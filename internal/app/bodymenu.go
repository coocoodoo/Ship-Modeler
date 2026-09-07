package app

import (
	"math"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

type bodyMenuState struct {
	open, pending bool
	x, y          float64
	body          uint32
}

func (a *App) handleBodyRightClick(in *InputFrame, vp render.Viewport) bool {
	st := &a.library.menu
	if a.Mode != ModeIdle || in.Shift {
		st.pending = false
		return false
	}
	if st.open {
		return true
	}
	if !st.pending && in.Pressed[MouseRight] && vp.Contains(int(in.MouseX), int(in.MouseY)) && !a.cubeOwnsPointer(*in) {
		s := a.BuildScene()
		s.Planes = nil
		s.PickFacesOnly = true
		a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
		hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
		if !hit.Hit || hit.BodyID == 0 {
			return false
		}
		*st = bodyMenuState{pending: true, x: in.MouseX, y: in.MouseY, body: hit.BodyID}
		a.orbiting, a.panning = false, false
	}
	if !st.pending {
		return false
	}
	dx, dy := in.MouseX-st.x, in.MouseY-st.y
	threshold := 4 * math.Max(1, a.Scale)
	if dx*dx+dy*dy > threshold*threshold {
		st.pending = false
		in.MouseDX, in.MouseDY = dx, dy
		if in.Down[MouseRight] {
			in.Pressed[MouseRight] = true
		}
		return false
	}
	if !in.Down[MouseRight] {
		st.pending = false
		if in.Released[MouseRight] {
			st.open = true
			a.Sel.Set(model.BodyRef(st.body))
		}
	}
	return true
}
func (a *App) buildBodyMenu() {
	st := &a.library.menu
	if !st.open || a.UI.ModalOpen() {
		return
	}
	items := []ui.MenuItem{{Label: "Save to Library", Icon: ui.DrawSaveIcon}}
	canUpdate := false
	if a.libraryEditAvailable() {
		for _, id := range a.library.editBodies {
			canUpdate = canUpdate || id == st.body
		}
	}
	if canUpdate {
		items = append(items, ui.MenuItem{Label: "Save library changes", Icon: ui.DrawRefreshIcon})
	}
	items = append(items, ui.MenuItem{Label: "Cancel", Icon: ui.DrawCrossIcon})
	w, h := a.px(210), a.px(float64(8+26*len(items)))
	screen := a.layout.Screen
	x := max(screen.X, min(float32(st.x), screen.X+screen.Width-w))
	y := max(screen.Y, min(float32(st.y), screen.Y+screen.Height-h))
	result := a.UI.Menu(ui.MakeID("body.menu"), ui.Rect(x, y, w, h), items)
	a.UI.ClaimPointer(screen)
	if result.Dismissed || a.UI.In.Pressed[ui.MouseRight] {
		*st = bodyMenuState{}
		return
	}
	if result.Chosen == 0 {
		id := st.body
		a.beginSaveToLibrary([]uint32{id})
	} else if result.Chosen == 1 && canUpdate {
		a.beginUpdateLibraryPart(a.library.editPart, a.library.editBodies)
	} else if result.Chosen >= 0 {
		*st = bodyMenuState{}
	}
}
