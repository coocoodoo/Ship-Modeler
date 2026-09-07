package app

import (
	"fmt"
	"sort"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// Orientation markers in the app (the user's request, 2026-08-28).
//
// Three dots a game engine reads off a .pxm: the front of the ship, its top,
// and one per thruster. Placing one is the arm-then-pick gesture the program
// already speaks (V-63a): press the row in the tree, then click the model —
// the dot lands on the face under the click, and carries that face's outward
// normal, which for a thruster is the direction the exhaust plays.

// markerState is the placement machinery.
type markerState struct {
	// awaiting is the kind the next viewport click will place.
	awaiting model.MarkerKind
	armed    bool
	// hover is the index of the dot under the pointer, or -1. Markers are
	// overlay glyphs rather than geometry, so the ID pass cannot report them
	// and this is resolved in screen space instead — the same answer, and the
	// same reason, as a sketch under the cursor (V-12).
	hover            int
	attachmentOpen   bool
	slot, appendText string
	editIndex        int
	page             int
}

// AwaitingMarker reports whether a marker pick is armed.
func (a *App) AwaitingMarker() bool { return a.markers.armed }

// BeginMarkerPick arms placement: the next click on a face places the dot.
func (a *App) BeginMarkerPick(kind model.MarkerKind) {
	if kind == model.MarkerAttachment {
		a.beginAttachmentDialog(-1)
		return
	}
	a.markers.armed = true
	a.markers.awaiting = kind
}

// CancelMarkerPick disarms it, reporting whether there was anything to disarm.
func (a *App) CancelMarkerPick() bool {
	if !a.markers.armed {
		return false
	}
	a.markers.armed = false
	return true
}

// placeMarkerAt spends an armed pick on a picked face: the dot lands where the
// cursor ray meets the face's plane, and keeps the face's outward normal.
func (a *App) placeMarkerAt(hit render.PickResult, in InputFrame, vp render.Viewport) {
	f, ok := a.resolveFace(hit.BodyID, hit.FaceUID)
	if !ok {
		a.Toast(ui.Toast{Text: "That face is no longer there", Kind: ui.ToastWarn})
		return
	}
	frame := f.body.Mesh.FaceFrame(f.face)
	at, hitPlane := a.pointOnFace(in.MouseX, in.MouseY, vp, frame)
	if !hitPlane {
		return
	}
	kind := a.markers.awaiting
	a.markers.armed = false
	cmd := &model.PlaceMarker{Marker: model.Marker{
		Kind: kind,
		At:   at,
		Dir:  f.body.Mesh.FaceNormal(f.face),
	}}
	if kind == model.MarkerAttachment {
		cmd.Marker.Slot, cmd.Marker.AppendText = a.markers.slot, a.markers.appendText
	}
	if !a.Run(cmd) {
		return
	}
	if kind == model.MarkerAttachment {
		a.Sel.Clear()
		a.selectMarker(len(a.Doc().Markers) - 1)
		a.toastWithUndo(model.AttachmentName(cmd.Marker.Slot, cmd.Marker.AppendText) + " placed")
	} else {
		a.toastWithUndo(fmt.Sprintf("%s dot placed on %s", kind.Label(), f.body.Name))
	}
}

// markerHint is the hint bar while a pick is armed.
func (a *App) markerHint() string {
	switch a.markers.awaiting {
	case model.MarkerFront:
		return "Click the model where the FRONT of the ship is · Esc cancels"
	case model.MarkerTop:
		return "Click the model where the TOP of the ship is · Esc cancels"
	case model.MarkerAttachment:
		return "Click a face to place " + model.AttachmentName(a.markers.slot, a.markers.appendText) + " · Esc cancels"
	default:
		return "Click the model where a thruster fires from · Esc cancels"
	}
}

// markerColor is each kind's dot colour, shared by the overlay and the tree.
func markerColor(k model.MarkerKind) rl.Color {
	switch k {
	case model.MarkerFront:
		return ui.ColorSuccess
	case model.MarkerTop:
		return ui.ColorAccent
	case model.MarkerAttachment:
		return rl.Color{R: 196, G: 145, B: 255, A: 255}
	default:
		return ui.ColorWarn
	}
}

// MarkerPickRadiusPx is how close the pointer must come to a dot's centre to
// grab it. Generous, because a dot is a few pixels across and a target you have
// to hunt for is a target that feels broken.
const MarkerPickRadiusPx = 9.0

// markerAt reports the index of the dot under the cursor, or -1.
//
// Dots are drawn in the overlay pass with no depth test, so one behind the hull
// is still visible and must still be clickable: what you can see is what you
// get. Nearest-to-the-cursor wins, and ties go to the one nearest the camera.
func (a *App) markerAt(mouseX, mouseY float64, vp render.Viewport) int {
	if a.Mode != ModeIdle || len(a.Doc().Markers) == 0 {
		return -1
	}
	radius := MarkerPickRadiusPx * a.Scale
	local := geom.Vec2{X: mouseX - float64(vp.X), Y: mouseY - float64(vp.Y)}
	eye := a.Camera.Eye()

	best, bestScore, bestDepth := -1, radius*radius, 0.0
	for i, m := range a.Doc().Markers {
		p, ok := a.Camera.WorldToViewport(m.At, float64(vp.W), float64(vp.H))
		if !ok {
			continue
		}
		d := p.Sub(local).LenSq()
		if d > bestScore {
			continue
		}
		depth := m.At.Sub(eye).LenSq()
		if best >= 0 && d == bestScore && depth >= bestDepth {
			continue
		}
		best, bestScore, bestDepth = i, d, depth
	}
	return best
}

// selectMarker puts the selection on a dot, honouring the click modifiers the
// tree and the viewport already share.
func (a *App) selectMarker(i int) {
	a.selectRef(model.MarkerRef(i))
	if len(a.Doc().Markers) > 4 {
		a.markers.page = i / 4
	}
}

// selectedMarkers is the dots the gizmo is anchored to.
func (a *App) selectedMarkers() []int { return a.Sel.MarkerIndices(a.Doc()) }

// deleteSelectedMarkers removes the selected dots, highest index first so the
// earlier ones do not shift out from under the loop.
func (a *App) deleteSelectedMarkers() bool {
	idx := a.selectedMarkers()
	if len(idx) == 0 {
		return false
	}
	sort.Sort(sort.Reverse(sort.IntSlice(idx)))
	names := a.Sel.Describe(a.Doc())
	for _, i := range idx {
		if !a.Run(&model.DeleteMarker{Index: i}) {
			return true
		}
	}
	a.Sel.Clear()
	a.toastWithUndo("Deleted " + names)
	return true
}

// buildMarkerOverlay draws the placed dots: a filled square each, in the
// kind's colour, with a short tick along a thruster's exhaust direction.
// Idle only — the working modes have their own overlays to keep legible.
func (a *App) buildMarkerOverlay() *render.Overlay {
	if a.Mode != ModeIdle || len(a.Doc().Markers) == 0 {
		return nil
	}
	d := &render.Overlay{}
	for i, m := range a.Doc().Markers {
		col := placedMarkerColor(m)
		size := 9.0
		if m.Kind == model.MarkerThruster {
			size = 8
		}
		// A selected dot wears a ring in the accent, so it reads as "this is
		// what the gizmo will move" without losing the colour that says which
		// kind it is. Hovering swells it, which is the affordance that tells
		// you it can be grabbed at all.
		selected := a.Sel.Contains(model.MarkerRef(i))
		if selected {
			d.Markers = append(d.Markers, render.OverlayMarker{
				P: m.At, Kind: render.MarkerVertex,
				Color: ui.ColorAccent, SizePx: size + 7,
			})
		} else if a.markers.hover == i {
			size += 3
		}
		d.Markers = append(d.Markers, render.OverlayMarker{
			P: m.At, Kind: render.MarkerVertex, Color: col, SizePx: size,
		})
		if m.Kind == model.MarkerThruster || m.Kind == model.MarkerAttachment {
			d.Lines = append(d.Lines, render.OverlayLine{
				A: m.At, B: m.At.Add(m.Dir.Mul(1.2)),
				Color: ui.Fade(col, 0.8), WidthPx: 2,
			})
		}
	}
	return d
}

func (a *App) drawAttachmentLabels(vp render.Viewport) {
	if a.Mode != ModeIdle {
		return
	}
	for i, m := range a.Doc().Markers {
		if m.Kind != model.MarkerAttachment {
			continue
		}
		p, ok := a.Camera.WorldToViewport(m.At, float64(vp.W), float64(vp.H))
		if !ok || p.X < 0 || p.Y < 0 || p.X > float64(vp.W) || p.Y > float64(vp.H) {
			continue
		}
		label := m.Slot
		if a.Sel.Contains(model.MarkerRef(i)) || a.markers.hover == i {
			label = a.Doc().MarkerLabel(i)
		}
		x, y := float32(vp.X)+float32(p.X)+a.px(10), float32(vp.Y)+float32(p.Y)-a.px(10)
		w := min(a.px(260), a.UI.TextWidth(label, ui.FontSizeSmall)+a.px(10))
		x = max(float32(vp.X), min(x, float32(vp.X+vp.W)-w))
		r := ui.Rect(x, y, w, a.px(20))
		a.UI.FillRounded(r, 3, ui.ColorCard)
		a.UI.Text(ui.InsetXY(r, a.px(5), 0), label, ui.FontSizeSmall, placedMarkerColor(m))
	}
}

// buildMarkerRows is the tree's Markers section: the placed dots with delete
// buttons, then one arming row per kind.
func (a *App) buildMarkerRows(row func() rl.Rectangle, open bool) {
	if !open {
		return
	}
	doc := a.Doc()
	start, end := 0, len(doc.Markers)
	pages := max(1, (len(doc.Markers)+3)/4)
	if len(doc.Markers) > 4 {
		a.markers.page = min(a.markers.page, pages-1)
		start = a.markers.page * 4
		end = min(start+4, len(doc.Markers))
	}
	for i := start; i < end; i++ {
		m := doc.Markers[i]
		label := m.Kind.Label()
		if m.Kind == model.MarkerAttachment {
			label = doc.MarkerLabel(i)
		}
		if m.Kind == model.MarkerThruster {
			label = doc.MarkerLabel(i)
		}
		iconColor := ui.AccentMarker
		labelColor := rl.Color{}
		if m.Kind == model.MarkerAttachment {
			iconColor = placedMarkerColor(m)
			labelColor = iconColor
		}
		ref := model.MarkerRef(i)
		res := a.UI.TreeRow(ui.MakeID("tree.marker."+itoa(i)), row(), ui.TreeRowSpec{
			Label:         label,
			Icon:          ui.DrawMarkerIcon,
			IconColor:     iconColor,
			LabelColor:    labelColor,
			KeepIconColor: m.Kind == model.MarkerAttachment,
			CanDelete:     true,
			Selected:      a.Sel.Contains(ref),
			Indent:        1,
		})
		if res.Hovered {
			a.tree.hovered = ref
		}
		switch {
		case res.DoubleClicked && m.Kind == model.MarkerAttachment:
			a.beginAttachmentDialog(i)
		case res.ClickedDelete:
			idx := i
			if a.Run(&model.DeleteMarker{Index: idx}) {
				a.Sel.Remove(ref)
				a.toastWithUndo("Deleted the " + m.Kind.String() + " dot")
			}
		case res.Clicked:
			// Selecting the row arms the gizmo on that dot, which is the same
			// thing clicking it in the viewport does.
			a.selectMarker(i)
		}
	}
	if pages > 1 {
		// Keep the navigation and add controls stationary on the last page.
		for i := end; i < start+4; i++ {
			row()
		}
		nav := row()
		prev, rest := ui.SplitLeft(nav, a.px(54))
		next, label := ui.SplitRight(rest, a.px(54))
		if a.UI.Button(ui.MakeID("marker.prev"), prev, "Previous", ui.ButtonOpts{Disabled: a.markers.page == 0}) {
			a.markers.page--
		}
		a.UI.TextCentered(label, fmt.Sprintf("%d / %d", a.markers.page+1, pages), ui.FontSizeSmall, ui.ColorTextDim)
		if a.UI.Button(ui.MakeID("marker.next"), next, "Next", ui.ButtonOpts{Disabled: a.markers.page+1 == pages}) {
			a.markers.page++
		}
	}

	arm := func(kind model.MarkerKind, label, tip string) {
		armed := a.markers.armed && a.markers.awaiting == kind
		if armed {
			label = "Click the model…"
		}
		res := a.UI.TreeRow(ui.MakeID("tree.marker.add."+kind.String()), row(), ui.TreeRowSpec{
			Label:     label,
			Icon:      ui.DrawMarkerIcon,
			IconColor: ui.AccentMarker,
			Selected:  armed,
			Indent:    1,
			Dim:       !armed,
		})
		if res.Clicked {
			if armed {
				a.CancelMarkerPick()
			} else {
				a.BeginMarkerPick(kind)
			}
		}
		_ = tip
	}
	_, hasFront := doc.FrontMarker()
	_, hasTop := doc.TopMarker()
	frontLabel, topLabel := "Set front dot…", "Set top dot…"
	if hasFront {
		frontLabel = "Move front dot…"
	}
	if hasTop {
		topLabel = "Move top dot…"
	}
	arm(model.MarkerFront, frontLabel, "The direction the ship flies")
	arm(model.MarkerTop, topLabel, "Pins the ship's roll")
	arm(model.MarkerThruster, "Add thruster dot…", "Where an exhaust effect plays")
	arm(model.MarkerAttachment, "Add ship part…", "A named attachment point for modular parts")
}
