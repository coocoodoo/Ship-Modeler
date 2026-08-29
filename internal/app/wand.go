package app

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// The magic wand in the app (the user's request, 2026-08-28; V-145).
//
// Click a texel with the wand and everything within the tolerance of its
// colour, connected to it, becomes the selection; from then on every painting
// tool writes only inside that selection until it is cleared. Shift adds
// another region. The mask itself is enforced down in the paint package's
// single texel writer, so this file only decides *what* is selected and shows
// it — a crisp outline traced on the face, the same way the texel cursor is.

// DefaultWandTolerance is where the slider starts: wide enough to take a
// dithered ramp as one region, tight enough not to eat a panel line.
const DefaultWandTolerance = 32

// wandClick spends a wand click: select at the hovered texel, or add to the
// selection with Shift held.
func (a *App) wandClick(add bool) {
	h := a.paint.hover
	f, ok := a.resolveFace(h.body, h.face)
	if !ok || h.paint == nil {
		return
	}
	region := paint.FaceRect(f.body.Mesh, f.face, h.paint)
	mask := paint.WandSelect(h.paint, region, h.texel,
		uint8(clampInt(a.paint.wandTolerance, 0, 255)), f.body.Color)
	if mask == nil || mask.Count() == 0 {
		return
	}

	st := &a.paint
	sameFace := st.wandMask != nil && st.wandBody == h.body && st.wandFace == h.face
	if add && sameFace {
		mask = st.wandMask.Union(mask)
	}
	st.wandMask = mask
	st.wandBody, st.wandFace = h.body, h.face
	st.wandRes = h.paint.Res

	verb := "Selected"
	if add && sameFace {
		verb = "Selection grew to"
	}
	a.Toast(ui.Toast{
		Text:     fmt.Sprintf("%s %s — other tools now paint only inside it", verb, plural(mask.Count(), "texel", "texels")),
		Action:   "Clear",
		OnAction: func() { a.ClearWandSelection() },
	})
}

// ClearWandSelection drops the mask, reporting whether there was one.
func (a *App) ClearWandSelection() bool {
	if a.paint.wandMask == nil {
		return false
	}
	a.paint.wandMask = nil
	return true
}

// wandMaskFor is the mask a stroke on the given face must carry: the wand's
// selection when it belongs to that face and still describes its picture, and
// nil otherwise — a selection on one face never confines painting on another.
//
// The resolution check is the stale-coordinates guard: a resample rebuilds
// the face's picture at a different density, and a mask in the old texel
// coordinates would confine the brush to the wrong pixels. Rather than
// guessing a conversion, the selection simply no longer applies.
func (a *App) wandMaskFor(bodyID uint32, uid mesh.FaceUID) *paint.Mask {
	st := &a.paint
	if st.wandMask == nil || st.wandBody != bodyID || st.wandFace != uid {
		return nil
	}
	if f, ok := a.resolveFace(bodyID, uid); ok {
		if p := f.body.Mesh.Faces[f.face].Paint; p != nil && p.Res != st.wandRes {
			st.wandMask = nil
			return nil
		}
	}
	return st.wandMask
}

// wandOverlay traces the selection's boundary on the face, lifted the same
// hair the texel cursor floats at, so what is selected is never a guess.
func (a *App) wandOverlay() *render.Overlay {
	st := &a.paint
	if !a.InPaint() || st.wandMask == nil {
		return nil
	}
	f, ok := a.resolveFace(st.wandBody, st.wandFace)
	if !ok {
		// The face was cut away under the selection: nothing to outline, and
		// nothing the mask could confine any more.
		st.wandMask = nil
		return nil
	}
	p := f.body.Mesh.Faces[f.face].Paint
	if p == nil {
		// Selected on a bare face and nothing painted yet: the provisional
		// mapping the hover used still describes it.
		if st.prov != nil && st.provBody == st.wandBody && st.provFace == st.wandFace {
			p = st.prov
		} else {
			return nil
		}
	}
	if p.Res != st.wandRes {
		st.wandMask = nil // resampled underneath the selection
		return nil
	}

	corner := func(cx, cy int) geom.Vec3 {
		w := p.Frame.ToWorld(geom.Vec2{X: float64(cx) * p.Texel, Y: float64(cy) * p.Texel})
		return w.Add(p.Frame.N.Mul(p.Texel * 0.03))
	}
	d := &render.Overlay{}
	col := ui.ColorAccent
	for _, seg := range st.wandMask.Boundary() {
		d.Lines = append(d.Lines, render.OverlayLine{
			A: corner(seg[0].X, seg[0].Y), B: corner(seg[1].X, seg[1].Y),
			Color: col, WidthPx: 2,
		})
	}
	if d.Empty() {
		return nil
	}
	return d
}

// buildWandSection is the tolerance control, shown while the wand is armed.
func (a *App) buildWandSection(row func(float32) rl.Rectangle, line float32) {
	st := &a.paint
	head := row(line)
	note, label := ui.SplitRight(head, a.px(96))
	a.UI.Text(label, "Tolerance", ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.Text(note, describeTolerance(st.wandTolerance), ui.FontSizeSmall,
		ui.Fade(ui.ColorTextDim, 0.9))

	r := row(a.px(24))
	valueBox, sliderBox := ui.SplitRight(r, a.px(40))
	if v, changed := a.UI.Slider(ui.MakeID("paint.wandtol"), sliderBox,
		float64(st.wandTolerance), 0, 255, ui.ButtonOpts{
			Tooltip: "How far a colour may drift and still join the selection — 0 is exact, 255 takes the face",
		}); changed {
		st.wandTolerance = clampInt(int(v+0.5), 0, 255)
	}
	a.UI.Text(valueBox, itoa(st.wandTolerance), ui.FontSizeUI, ui.ColorText)
}

// describeTolerance names what a tolerance means, so the slider is a sentence
// rather than a bare number.
func describeTolerance(t int) string {
	switch {
	case t == 0:
		return "exact colour"
	case t <= 24:
		return "close shades"
	case t <= 80:
		return "a colour family"
	case t < 255:
		return "most of the face"
	default:
		return "everything"
	}
}

// buildWandSelectionRow is the standing "N texels selected · Clear" row shown
// under every tool while a selection constrains them.
func (a *App) buildWandSelectionRow(r rl.Rectangle) {
	st := &a.paint
	if _, ok := a.resolveFace(st.wandBody, st.wandFace); !ok {
		st.wandMask = nil
		return
	}
	clearBox, textBox := ui.SplitRight(r, a.px(64))
	a.UI.Text(textBox, plural(st.wandMask.Count(), "texel selected", "texels selected"),
		ui.FontSizeSmall, ui.ColorAccent)
	if a.UI.Button(ui.MakeID("paint.wandclear"), clearBox, "Clear", ui.ButtonOpts{
		Tooltip:  "Paint anywhere again",
		Shortcut: "Esc",
	}) {
		a.ClearWandSelection()
	}
}
