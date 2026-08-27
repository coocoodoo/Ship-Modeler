package app

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom/csg"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// Boolean mode (R12, SPEC-UX §11): pick the body that survives, pick what to
// combine into it, press Enter.

// booleanState is the app's half of the tool.
type booleanState struct {
	tool *tools.BooleanTool
}

// InBoolean reports whether the boolean tool is open.
func (a *App) InBoolean() bool { return a.Mode == ModeBoolean && a.boolean.tool != nil }

// BeginBoolean opens the tool, seeding it from whatever is already selected so
// picking a body and pressing B does the obvious thing.
func (a *App) BeginBoolean() bool {
	if len(a.Doc().Bodies) < 2 {
		a.Toast(ui.Toast{
			Text: "A boolean needs two bodies — there is only one to work with",
			Kind: ui.ToastWarn,
		})
		return false
	}
	a.ExitPaint()
	t := tools.NewBooleanTool()
	for _, ref := range a.Sel.Refs() {
		if ref.Kind == model.SelBody {
			t.Pick(ref.Body)
		}
	}
	a.boolean.tool = t
	a.Mode = ModeBoolean
	a.Sel.Clear()
	return true
}

// CancelBoolean closes the tool without changing anything.
func (a *App) CancelBoolean() {
	a.boolean.tool = nil
	a.Mode = ModeIdle
}

// CommitBoolean runs the picked operation through the bus as one step.
func (a *App) CommitBoolean() bool {
	t := a.boolean.tool
	if t == nil {
		return false
	}
	if ok, why := t.Valid(); !ok {
		a.Toast(ui.Toast{Text: why, Kind: ui.ToastWarn})
		return false
	}

	cmd := &model.Boolean{
		Op:        t.Op,
		Target:    t.Target,
		Tools:     append([]uint32(nil), t.Tools...),
		KeepTools: t.KeepTools,
	}
	if err := a.Bus.Run(cmd); err != nil {
		// SPEC-UX §11.3: the document is untouched, and the message says so
		// rather than repeating the solver's own words.
		a.Toast(ui.Toast{Text: csgFailureToast(err), Kind: ui.ToastError})
		a.dumpCSGRepro("boolean", nil, append([]uint32{t.Target}, t.Tools...))
		return false
	}

	a.Toast(ui.Toast{Text: cmd.Summary()})
	if cmd.Emptied() {
		a.Sel.Clear()
	} else {
		a.Sel.Set(model.BodyRef(t.Target))
	}
	a.boolean.tool = nil
	a.Mode = ModeIdle
	return true
}

// updateBoolean turns viewport clicks into picks.
func (a *App) updateBoolean(in InputFrame, vp render.Viewport) {
	t := a.boolean.tool
	if t == nil || !in.Pressed[MouseLeft] {
		return
	}
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) {
		return
	}
	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	a.Hover = hit
	if !hit.Hit || hit.BodyID == 0 {
		return
	}
	t.Pick(hit.BodyID)
}

// handleBooleanKeys implements the tool's shortcuts.
func (a *App) handleBooleanKeys(in InputFrame) {
	if in.Ctrl {
		return
	}
	if in.KeyPressed(rl.KeyEscape) {
		a.CancelBoolean()
	}
	if in.KeyPressed(rl.KeyEnter) || in.KeyPressed(rl.KeyKpEnter) {
		a.CommitBoolean()
	}
}

// booleanHint is what the hint bar says while the tool is open.
func (a *App) booleanHint() string {
	if a.boolean.tool == nil {
		return ""
	}
	return a.boolean.tool.Hint()
}

// booleanTint colours a body according to its part in the pending operation:
// the survivor in the accent, the tools in error red, as SPEC-UX §11 asks. The
// alpha is the blend strength, so a picked body still reads as its own colour
// underneath.
func (a *App) booleanTint(bodyID uint32) (color.RGBA, bool) {
	t := a.boolean.tool
	if t == nil {
		return color.RGBA{}, false
	}
	picked, isTarget := t.Picked(bodyID)
	if !picked {
		return color.RGBA{}, false
	}
	if isTarget {
		return ui.Fade(ui.ColorAccent, 0.35), true
	}
	return ui.Fade(ui.ColorError, 0.3), true
}

// buildBooleanCard is the options panel of SPEC-UX §11.
func (a *App) buildBooleanCard(viewport rl.Rectangle) {
	t := a.boolean.tool
	if t == nil {
		return
	}
	w := a.px(248)
	h := a.px(196)
	box := ui.Rect(
		viewport.X+viewport.Width-w-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		w, h)

	valid, why := t.Valid()
	card := a.UI.FloatingCard(ui.MakeID("boolean.card"), box, "Boolean", ui.FloatingCardOpts{
		Footer:          true,
		ConfirmLabel:    "Apply",
		CancelLabel:     "Cancel",
		ConfirmDisabled: !valid,
		ConfirmWhy:      why,
	})
	body := card.Body
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	row := func(h float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, h)
		return r
	}

	a.UI.Text(row(line), "Operation", ui.FontSizeSmall, ui.ColorTextDim)
	ops := []csg.Op{csg.Union, csg.Subtract, csg.Intersect}
	labels := []string{"Union", "Subtract", "Intersect"}
	sel := 0
	for i, op := range ops {
		if op == t.Op {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("boolean.op"), row(a.px(24)),
		labels, sel, ui.ChipGroupOpts{}); changed {
		t.Op = ops[pick]
	}

	body.Y += a.px(6)
	body.Height -= a.px(6)

	if a.UI.Toggle(ui.MakeID("boolean.keep"), row(a.px(22)), "Keep original bodies",
		t.KeepTools, ui.ButtonOpts{
			Tooltip: "Leave the tool bodies in the document instead of consuming them",
		}) {
		t.KeepTools = !t.KeepTools
	}

	body.Y += a.px(6)
	body.Height -= a.px(6)
	a.UI.Text(row(line), a.booleanPickSummary(), ui.FontSizeSmall, ui.ColorTextDim)

	if card.Confirmed {
		a.CommitBoolean()
	}
	if card.Cancelled {
		a.CancelBoolean()
	}
}

// booleanPickSummary names what is picked, so the card says the same thing the
// tinting does.
func (a *App) booleanPickSummary() string {
	t := a.boolean.tool
	if t == nil || t.Target == 0 {
		return "Nothing picked yet"
	}
	doc := a.Doc()
	name := func(id uint32) string {
		if b := doc.BodyByID(id); b != nil {
			return b.Name
		}
		return "?"
	}
	out := "Keep " + name(t.Target)
	for i, id := range t.Tools {
		if i == 0 {
			out += " · with "
		} else {
			out += ", "
		}
		out += name(id)
	}
	return out
}

// canBoolean reports whether the Boolean button would do anything, which needs
// two bodies to combine.
func (a *App) canBoolean() (bool, string) {
	if len(a.Doc().Bodies) < 2 {
		return false, "A boolean needs two bodies — there is only one to work with"
	}
	return true, ""
}
