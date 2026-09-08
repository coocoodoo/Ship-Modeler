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
	// Snapshot the surface hit, since the menu's clamped screen position is
	// not necessarily the point that was right-clicked.
	pin *model.NotePin
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
		if f, ok := a.resolveFace(hit.BodyID, hit.FaceUID); ok {
			if at, ok := a.pointOnFace(in.MouseX, in.MouseY, vp, f.body.Mesh.FaceFrame(f.face)); ok {
				if pin, err := model.AnchorNotePin(f.body, f.face, at); err == nil {
					st.pin = &pin
				}
			}
		}
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
	items := []ui.MenuItem{{Label: "Drop Pin", Icon: drawNotePinIcon}, {Label: "Save to Library", Icon: ui.DrawSaveIcon}}
	canUpdate := false
	if a.libraryEditAvailable() {
		for _, id := range a.library.editBodies {
			canUpdate = canUpdate || id == st.body
		}
	}
	if canUpdate {
		items = append(items, ui.MenuItem{Label: "Save library changes", Icon: ui.DrawRefreshIcon})
	}
	moveIndex := -1
	if st.pin != nil {
		moveIndex = len(items)
		items = append(items, ui.MenuItem{Label: "Move Texture", Icon: ui.DrawMoveIcon})
	}
	workIndex := -1
	if st.pin != nil {
		workIndex = len(items)
		items = append(items, ui.MenuItem{Label: "Layers / Match", Icon: ui.DrawMoveIcon})
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
	if result.Chosen >= 0 && result.Chosen == workIndex {
		a.openWorkflow(model.FaceScope{Body: st.body, Face: st.pin.Face}, 1)
	} else if result.Chosen >= 0 && result.Chosen == moveIndex {
		a.beginMoveTexture(st.body, st.pin.Face)
	} else if result.Chosen == 0 {
		a.dropBodyMenuPin()
	} else if result.Chosen == 1 {
		id := st.body
		a.beginSaveToLibrary([]uint32{id})
	} else if result.Chosen == 2 && canUpdate {
		a.beginUpdateLibraryPart(a.library.editPart, a.library.editBodies)
	} else if result.Chosen >= 0 {
		*st = bodyMenuState{}
	}
}

func (a *App) dropBodyMenuPin() {
	pin := a.library.menu.pin
	a.library.menu = bodyMenuState{}
	a.CancelMarkerPick()
	a.Sel.Clear()
	a.dropPushPull()
	a.transform.tool = nil
	a.notePins.hidden = false
	a.notePins.armed = false
	if pin != nil {
		if _, attached := pin.Position(a.Doc()); !attached {
			a.Toast(ui.Toast{Text: "That face is no longer there. Right-click a model face again.", Kind: ui.ToastWarn})
			return
		}
		a.editNotePin(*pin)
		return
	}
	// A body-tree row has no surface location: let the next click choose it.
	a.notePins.open, a.notePins.editing, a.notePins.armed = false, false, true
	a.UI.ClearFocus()
}
