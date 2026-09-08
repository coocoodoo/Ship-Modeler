package app

import (
	"fmt"
	"image/color"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

type notePinState struct {
	open, editing, armed, hidden bool
	clearing                     bool
	draft                        model.NotePin
	page                         int
}

func drawNotePinIcon(cx, cy, size float64, col color.RGBA) {
	rl.DrawLineEx(rl.Vector2{X: float32(cx), Y: float32(cy)}, rl.Vector2{X: float32(cx), Y: float32(cy + size*.45)}, float32(size*.12), col)
	rl.DrawCircleV(rl.Vector2{X: float32(cx), Y: float32(cy - size*.12)}, float32(size*.29), col)
	rl.DrawCircleV(rl.Vector2{X: float32(cx), Y: float32(cy - size*.12)}, float32(size*.1), ui.ColorPanel)
}

func (a *App) buildPinsToggle(r rl.Rectangle) {
	a.UI.Panel(r)
	a.UI.HairlineH(r.X, r.Y, r.Width, ui.ColorStroke)
	if a.UI.IconButton(ui.MakeID("view.pins"), ui.InsetXY(r, a.px(5), a.px(3)), drawNotePinIcon, ui.IconOpts{
		Label: "Pins", Tooltip: "Place text notes on the model for your next AI request", Active: a.notePins.open || a.notePins.armed,
		Disabled: a.Mode != ModeIdle && !a.InPaint(),
	}) {
		a.notePins.open = !a.notePins.open
		a.notePins.armed = false
		a.notePins.editing = false
		a.UI.ClearFocus()
	}
}

func (a *App) pinScreen(p model.NotePin, vp render.Viewport) (rl.Rectangle, bool) {
	if b := a.Doc().BodyByID(p.Body); b != nil && !b.Visible {
		return rl.Rectangle{}, false
	}
	at, _ := p.Position(a.Doc())
	xy, ok := a.Camera.WorldToViewport(at, float64(vp.W), float64(vp.H))
	if !ok || xy.X < 0 || xy.Y < 0 || xy.X > float64(vp.W) || xy.Y > float64(vp.H) {
		return rl.Rectangle{}, false
	}
	return ui.Rect(float32(vp.X)+float32(xy.X)-a.px(12), float32(vp.Y)+float32(xy.Y)-a.px(32), a.px(24), a.px(24)), true
}

func (a *App) pinAt(in InputFrame, vp render.Viewport) *model.NotePin {
	if a.notePins.hidden {
		return nil
	}
	var best *model.NotePin
	distance := float64(1e30)
	for i := range a.Doc().NotePins {
		p := &a.Doc().NotePins[i]
		box, ok := a.pinScreen(*p, vp)
		if !ok || !rl.CheckCollisionPointRec(rl.Vector2{X: float32(in.MouseX), Y: float32(in.MouseY)}, box) {
			continue
		}
		at, _ := p.Position(a.Doc())
		d := at.Sub(a.Camera.Eye()).LenSq()
		if d < distance {
			best = p
			distance = d
		}
	}
	return best
}

// Runs before painting and gizmos so a pin click never alters the surface.
func (a *App) updateNotePinInput(in InputFrame, vp render.Viewport) bool {
	if a.Mode != ModeIdle && !a.InPaint() {
		a.notePins.armed = false
		return false
	}
	if a.notePins.armed {
		if in.KeyPressed(rl.KeyEscape) {
			a.notePins.armed = false
			return true
		}
		if in.Pressed[MouseLeft] && !a.cubeOwnsPointer(in) {
			s := a.BuildScene()
			a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
			hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
			if hit.Hit && hit.Kind == render.PickFace {
				f, ok := a.resolveFace(hit.BodyID, hit.FaceUID)
				if ok {
					at, ok := a.pointOnFace(in.MouseX, in.MouseY, vp, f.body.Mesh.FaceFrame(f.face))
					if ok {
						p, err := model.AnchorNotePin(f.body, f.face, at)
						if err == nil {
							a.editNotePin(p)
							a.notePins.armed = false
							return true
						}
					}
				}
			}
			a.Toast(ui.Toast{Text: "Click a model face to place a note pin", Kind: ui.ToastWarn})
		}
		return true
	}
	if p := a.pinAt(in, vp); p != nil && in.Pressed[MouseLeft] {
		a.editNotePin(*p)
		return true
	}
	return false
}

func (a *App) editNotePin(p model.NotePin) {
	a.notePins.draft = p
	a.notePins.open = true
	a.notePins.editing = true
	a.UI.ClearFocus()
}

func (a *App) drawNotePins(vp render.Viewport) {
	if a.notePins.hidden || (a.Mode != ModeIdle && !a.InPaint()) {
		return
	}
	for _, p := range a.Doc().NotePins {
		r, ok := a.pinScreen(p, vp)
		if !ok {
			continue
		}
		col := ui.ColorWarn
		if model.PinStatus(p) == "In progress" {
			col = ui.ColorAccent
		}
		if model.PinStatus(p) == "Needs review" {
			col = ui.ColorWarn
		}
		if p.Done {
			col = ui.ColorSuccess
		}
		_, attached := p.Position(a.Doc())
		if !attached {
			col = ui.ColorTextDim
		}
		rl.DrawLineEx(rl.Vector2{X: r.X + r.Width/2, Y: r.Y + r.Height}, rl.Vector2{X: r.X + r.Width/2, Y: r.Y + r.Height + a.px(8)}, a.px(2), col)
		a.UI.FillRounded(r, 9, ui.ColorCard)
		a.UI.StrokeRounded(r, 9, col)
		a.UI.TextCentered(r, fmt.Sprint(p.ID), ui.FontSizeSmall, col)
	}
}

func (a *App) buildNotePinsDialog() {
	st := &a.notePins
	if !st.open || a.UI.ModalOpen() {
		return
	}
	title := "Model Notes"
	if st.editing {
		title = "Edit Note Pin"
		if st.draft.ID == 0 {
			title = "New Note Pin"
		}
	}
	_, err := model.CleanNoteText(st.draft.Text)
	why := ""
	if err != nil {
		why = err.Error()
	}
	card := a.UI.FloatingCard(ui.MakeID("pins.dialog"), a.libraryDialogBox(540, 640), title,
		ui.FloatingCardOpts{Footer: st.editing, ConfirmLabel: "Save note", CancelLabel: "Cancel", ConfirmDisabled: err != nil, ConfirmWhy: why})
	body := card.Body
	row := func(h float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, a.px(float64(h)))
		return r
	}
	a.UI.TextWrapped(row(42), "Notes stay with this project. Ask the AI to work on your model when you are ready.", ui.FontSizeSmall, ui.ColorTextDim)
	if st.editing {
		if st.draft.ID != 0 && a.UI.Button(ui.MakeID("pins.start"), row(29), "Start work on this pin", ui.ButtonOpts{Disabled: a.Bus.Review() != nil}) {
			a.workflowError(a.startPinWork(st.draft.ID))
			return
		}
		label := "Model face"
		if b := a.Doc().BodyByID(st.draft.Body); b != nil {
			label = b.Name
		}
		if _, attached := st.draft.Position(a.Doc()); !attached {
			label += " · face no longer available"
		}
		a.UI.Text(row(24), label, ui.FontSizeSmall, ui.ColorTextDim)
		a.UI.Text(row(22), "What would you like changed?", ui.FontSizeUI, ui.ColorText)
		editorHeight := min(100.0, max(48.0, float64(body.Height)/a.Scale-290))
		text := a.UI.TextArea(ui.MakeID("pins.text"), row(float32(editorHeight)), st.draft.Text, "e.g. Paint this panel copper and add two vents")
		st.draft.Text = text.Text
		a.UI.Text(row(22), fmt.Sprintf("%d / 2,000 characters", len([]rune(st.draft.Text))), ui.FontSizeSmall, ui.ColorTextDim)
		label = "Mark as done"
		if st.draft.Done {
			label = "Done — reopen request"
		}
		if a.UI.Button(ui.MakeID("pins.done"), row(28), label, ui.ButtonOpts{}) {
			st.draft.Done = !st.draft.Done
			st.draft.Status = "Open"
			if st.draft.Done {
				st.draft.Status = "Done"
			}
		}
		statuses := []string{"Open", "In progress", "Needs review", "Done"}
		statusIndex := 0
		for i, v := range statuses {
			if v == model.PinStatus(st.draft) {
				statusIndex = i
			}
		}
		if v, changed := a.UI.ChipGroup(ui.MakeID("pins.status"), row(28), statuses, statusIndex, ui.ChipGroupOpts{}); changed {
			st.draft.Status = statuses[v]
			st.draft.Done = v == 3
		}
		if len(st.draft.BeforeImage) > 0 || len(st.draft.AfterImage) > 0 {
			r := row(84)
			l, rr := ui.SplitLeft(r, r.Width/2)
			a.reviewImage(0, st.draft.BeforeImage, l, "Before")
			a.reviewImage(1, st.draft.AfterImage, rr, "After")
			a.UI.TextWrapped(row(34), st.draft.Changes, ui.FontSizeSmall, ui.ColorTextDim)
		}
		if st.draft.ID != 0 && a.UI.Button(ui.MakeID("pins.delete"), row(28), "Delete pin", ui.ButtonOpts{}) {
			if a.Run(&model.DeleteNotePin{ID: st.draft.ID}) {
				st.editing = false
				a.UI.ClearFocus()
			}
		}
		if card.Confirmed {
			if a.Run(&model.SetNotePin{Pin: st.draft}) {
				st.open = false
				st.editing = false
				a.UI.ClearFocus()
			}
		}
	} else {
		if a.UI.Button(ui.MakeID("pins.workshop"), row(29), "AI Review / Material Workshop", ui.ButtonOpts{}) {
			a.openWorkflow(a.workflow.face, 0)
			return
		}
		controls := row(32)
		add, rest := ui.SplitLeft(controls, controls.Width/3)
		hide, clear := ui.SplitLeft(rest, controls.Width/3)
		if a.UI.Button(ui.MakeID("pins.add"), add, "Drop a pin", ui.ButtonOpts{Style: ui.ButtonPrimary}) {
			st.armed = true
			st.open = false
			st.hidden = false
			a.CancelMarkerPick()
			a.Sel.Clear()
			a.dropPushPull()
			a.transform.tool = nil
			a.UI.ClearFocus()
		}
		label := "Hide pins"
		if st.hidden {
			label = "Show pins"
		}
		if a.UI.Button(ui.MakeID("pins.visibility"), hide, label, ui.ButtonOpts{}) {
			st.hidden = !st.hidden
		}
		if a.UI.Button(ui.MakeID("pins.clear"), clear, "Clear all pins", ui.ButtonOpts{Disabled: len(a.Doc().NotePins) == 0 && a.Doc().Seq.NotePin == 0}) {
			st.clearing = true
			a.UI.ShowModal(ui.ModalState{Title: "Clear all pins?", Body: "Are you sure you want to clear all pins?", ConfirmText: "Yes", CancelText: "No", Danger: true})
		}
		row(10)
		pins := a.Doc().NotePins
		perPage := max(1, min(5, int((float64(body.Height)/a.Scale-62)/44)))
		pages := max(1, (len(pins)+perPage-1)/perPage)
		st.page = max(0, min(st.page, pages-1))
		if len(pins) == 0 {
			a.UI.TextWrapped(row(80), "No notes yet. Choose Drop a pin, click the model, then write your request.", ui.FontSizeUI, ui.ColorTextDim)
		}
		for i := st.page * perPage; i < min(len(pins), (st.page+1)*perPage); i++ {
			p := pins[i]
			text := strings.Join(strings.Fields(p.Text), " ")
			rs := []rune(text)
			if len(rs) > 48 {
				text = string(rs[:45]) + "…"
			}
			status := model.PinStatus(p) + " · "
			if p.Done {
				status = "✓ "
			}
			if a.UI.Button(ui.MakeID(fmt.Sprintf("pins.note.%d", p.ID)), row(44), fmt.Sprintf("%s%d · %s", status, p.ID, text), ui.ButtonOpts{}) {
				a.editNotePin(p)
			}
		}
		nav := row(30)
		prev, nav := ui.SplitLeft(nav, a.px(100))
		next, mid := ui.SplitRight(nav, a.px(100))
		if a.UI.Button(ui.MakeID("pins.prev"), prev, "Previous", ui.ButtonOpts{Disabled: st.page == 0}) {
			st.page--
		}
		a.UI.TextCentered(mid, fmt.Sprintf("%d / %d", st.page+1, pages), ui.FontSizeSmall, ui.ColorTextDim)
		if a.UI.Button(ui.MakeID("pins.next"), next, "Next", ui.ButtonOpts{Disabled: st.page+1 >= pages}) {
			st.page++
		}
		if a.UI.Button(ui.MakeID("pins.close"), row(30), "Close", ui.ButtonOpts{}) {
			st.open = false
			a.UI.ClearFocus()
		}
	}
	a.UI.ClaimPointer(a.layout.Screen)
	if card.Cancelled || a.UI.In.KeyPressed(rl.KeyEscape) {
		st.open = false
		st.editing = false
		a.UI.ClearFocus()
	}
}

func (a *App) aiNotePins() []any {
	result := make([]any, 0, len(a.Doc().NotePins))
	for _, p := range a.Doc().NotePins {
		at, attached := p.Position(a.Doc())
		name := ""
		fi := -1
		if b := a.Doc().BodyByID(p.Body); b != nil {
			name = b.Name
			if b.Mesh != nil {
				for i, f := range b.Mesh.Faces {
					if f.ID == p.Face {
						fi = i
						break
					}
				}
			}
		}
		result = append(result, map[string]any{"status": model.PinStatus(p), "changes": p.Changes, "hasBefore": len(p.BeforeImage) > 0, "hasAfter": len(p.AfterImage) > 0, "id": p.ID, "text": p.Text, "done": p.Done, "body": p.Body, "bodyName": name, "faceID": fmt.Sprint(p.Face), "faceIndex": fi, "at": at, "attached": attached})
	}
	return result
}
