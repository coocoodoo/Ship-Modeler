package app

import (
	"math"

	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// Box select (SPEC-UX §12.1, SPEC-RENDER §6.2).
//
// Drag on empty space and a rectangle follows the pointer; let go and whatever
// the filter says is inside becomes the selection. The default filter is
// vertices, because the workflow this exists for is grabbing the nose of a hull
// and stretching it.

// BoxSelectMinPx is how far the pointer must travel before a drag counts as a
// box rather than a click that wobbled.
const BoxSelectMinPx = 4

// boxSelectState is the app's half of the rectangle.
type boxSelectState struct {
	// active is true between pressing on empty space and releasing.
	active bool
	// Filter is which kind of element the box collects (SPEC-UX §12.1).
	Filter render.PickKind
	rect   render.BoxRect
}

// initBoxSelect sets the default filter, which is verts.
func (b *boxSelectState) init() { b.Filter = render.PickVert }

// BoxSelecting reports whether a rectangle is being dragged right now.
func (a *App) BoxSelecting() bool {
	return a.box.active && a.boxIsBigEnough()
}

func (a *App) boxIsBigEnough() bool {
	return math.Abs(a.box.rect.Width()) >= BoxSelectMinPx ||
		math.Abs(a.box.rect.Height()) >= BoxSelectMinPx
}

// BoxRect is the rectangle to draw, in window pixels.
func (a *App) BoxRect() render.BoxRect { return a.box.rect.Normalized() }

// beginBoxSelect starts a rectangle at a point.
func (a *App) beginBoxSelect(x, y float64) {
	a.box.active = true
	a.box.rect = render.BoxRect{X0: x, Y0: y, X1: x, Y1: y}
}

// updateBoxSelect follows the pointer, and finishes on release.
func (a *App) updateBoxSelect(in InputFrame, vp render.Viewport) {
	if !a.box.active {
		return
	}
	a.box.rect.X1, a.box.rect.Y1 = in.MouseX, in.MouseY
	if in.Down[MouseLeft] {
		return
	}
	a.box.active = false
	if !a.boxIsBigEnough() {
		return
	}
	a.applyBoxSelect(in, vp)
}

// applyBoxSelect runs the ID render and turns what it found into a selection.
func (a *App) applyBoxSelect(in InputFrame, vp render.Viewport) {
	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	found := a.Renderer.BoxPick(&s, vp, a.box.rect, a.box.Filter)

	refs := make([]model.Ref, 0, len(found))
	seen := map[model.Ref]bool{}
	for _, p := range found {
		ref, ok := boxRefOf(p, a.box.Filter)
		if !ok || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		if !in.Shift && !in.Ctrl {
			a.Sel.Clear()
		}
		a.Toast(ui.Toast{Text: "Nothing " + boxFilterWord(a.box.Filter) + " in that box"})
		return
	}

	switch {
	case in.Shift:
		for _, r := range refs {
			a.Sel.Add(r)
		}
	case in.Ctrl:
		for _, r := range refs {
			a.Sel.Toggle(r)
		}
	default:
		a.Sel.SetAll(refs)
	}
	a.Toast(ui.Toast{Text: "Selected " + plural(len(refs), boxFilterNoun(a.box.Filter),
		boxFilterNounPlural(a.box.Filter))})
}

// boxRefOf turns a pick reference into a selection reference. A body filter is
// the odd one out: it collects the bodies its faces belong to, because there is
// no such thing as a body pixel.
func boxRefOf(p render.PickRef, filter render.PickKind) (model.Ref, bool) {
	switch filter {
	case render.PickVert:
		return model.VertRef(p.BodyID, p.Vert), true
	case render.PickEdge:
		return model.EdgeRef(p.BodyID, p.Edge), true
	case render.PickFace:
		return model.FaceRef(p.BodyID, p.FaceUID), true
	}
	return model.Ref{}, false
}

// BoxFilters are the chips the card offers, in order.
func BoxFilters() []render.PickKind {
	return []render.PickKind{render.PickVert, render.PickEdge, render.PickFace}
}

func boxFilterWord(k render.PickKind) string {
	switch k {
	case render.PickEdge:
		return "edge-like"
	case render.PickFace:
		return "face-like"
	}
	return "vertex-like"
}

func boxFilterNoun(k render.PickKind) string {
	switch k {
	case render.PickEdge:
		return "edge"
	case render.PickFace:
		return "face"
	}
	return "vertex"
}

func boxFilterNounPlural(k render.PickKind) string {
	switch k {
	case render.PickEdge:
		return "edges"
	case render.PickFace:
		return "faces"
	}
	return "vertices"
}

// BoxFilterLabel names a filter for the chip group.
func BoxFilterLabel(k render.PickKind) string {
	switch k {
	case render.PickEdge:
		return "Edges"
	case render.PickFace:
		return "Faces"
	}
	return "Verts"
}
