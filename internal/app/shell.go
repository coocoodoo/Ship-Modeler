package app

import (
	"fmt"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/extrude"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/sketch"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// The window chrome of SPEC-UX §2: toolbar, tree panel and hint bar. Every
// widget here runs twice per frame — once for input during Update and once for
// pixels during Draw — from this one function, so what is interactive is always
// exactly what is drawn.

// treeState is the left panel's own state: which sections are open, whether the
// panel is collapsed, and which row is being renamed inline.
type treeState struct {
	width     float64
	collapsed bool

	planesOpen   bool
	sketchesOpen bool
	bodiesOpen   bool

	// renaming identifies the row with an open inline editor.
	renaming model.Ref
	// pickerFor identifies the body whose colour popover is open.
	pickerFor uint32
	// hovered is the row under the cursor this frame.
	hovered model.Ref
}

func (t *treeState) init(s *io.Settings) {
	t.width = float64(s.TreePanelWidth)
	if t.width < 160 {
		t.width = ui.TreeWidth
	}
	t.collapsed = s.TreeCollapsed
	t.planesOpen, t.sketchesOpen, t.bodiesOpen = true, true, true
}

// buildShell lays out and runs the whole chrome, handling input and painting in
// the one pass an immediate-mode kit expects.
func (a *App) buildShell(l Layout) {
	a.tree.hovered = model.Ref{}

	if a.InSketch() {
		a.buildSketchToolbar(l.Toolbar)
	} else {
		a.buildToolbar(l.Toolbar)
	}
	if !a.tree.collapsed {
		a.buildTree(l.Tree)
	}
	a.buildTreeHandle(l.Handle)
	switch {
	case a.InExtrude():
		a.buildExtrudeCard(l.Viewport)
	case a.InSketch():
		a.buildSketchCard(l.Viewport)
	}
	a.UI.HintBar(l.HintBar, a.HintText(), Version)

	// The tree's hover feeds the viewport highlight, and the viewport's hover
	// feeds the tree's — whichever one is live this frame wins.
	if a.tree.hovered.Kind != model.SelNone {
		a.TreeHover = a.tree.hovered
	} else if a.Hover.Hit && a.Hover.Kind == 0 {
		a.TreeHover = model.Ref{}
	} else {
		a.TreeHover = a.viewportHoverRef()
	}
}

// toolbarTool describes one tool button.
type toolbarTool struct {
	mode     Mode
	label    string
	shortcut string
	icon     ui.IconFunc
	// milestone names the release that turns this tool on, which is what its
	// disabled tooltip says. Empty means the tool has shipped.
	milestone string
	// start is what clicking the button does, and is the same entry point the
	// shortcut key uses. Nil for a tool that has not shipped.
	start func(*App)
}

func toolbarTools() []toolbarTool {
	return []toolbarTool{
		{mode: ModeSketch, label: "Sketch", shortcut: "S", icon: ui.DrawSketchToolIcon,
			start: (*App).beginSketchFromSelection},
		{mode: ModeExtrude, label: "Extrude", shortcut: "E", icon: ui.DrawExtrudeIcon,
			start: func(a *App) { a.BeginExtrude() }},
		{mode: ModeBoolean, label: "Boolean", shortcut: "B", icon: ui.DrawBooleanIcon,
			milestone: "M4"},
		{mode: ModeIdle, label: "Move", shortcut: "M", icon: ui.DrawMoveIcon,
			milestone: "M6"},
		{mode: ModePaint, label: "Paint", shortcut: "P", icon: ui.DrawPaintIcon,
			milestone: "M7"},
	}
}

// active reports whether this tool is the one currently running, which is what
// gives the button its accent underline (SPEC-UX §4).
func (t toolbarTool) active(a *App) bool {
	switch t.mode {
	case ModeSketch:
		return a.InSketch()
	case ModeExtrude:
		return a.InExtrude()
	default:
		return false
	}
}

func (a *App) buildToolbar(r rl.Rectangle) {
	a.UI.Panel(r)
	a.UI.HairlineH(r.X, r.Y+r.Height-a.px(1), r.Width, ui.ColorStroke)

	inner := ui.InsetXY(r, a.px(ui.Spacing), a.px(4))
	btnH := inner.Height
	rest := inner

	for _, tool := range toolbarTools() {
		w := a.px(ui.IconSize+ui.Spacing) + a.UI.TextWidth(tool.label, ui.FontSizeUI) + a.px(ui.Spacing)
		var box rl.Rectangle
		box, rest = ui.SplitLeft(rest, w)
		box.Height = btnH

		var why string
		if tool.start == nil {
			why = tool.label + " arrives with milestone " + tool.milestone
		}
		clicked := a.UI.IconButton(ui.MakeID("tool."+tool.label), box, tool.icon, ui.IconOpts{
			Label:       tool.label,
			Active:      tool.active(a),
			Disabled:    tool.start == nil,
			Shortcut:    tool.shortcut,
			DisabledWhy: why,
		})
		if clicked && tool.start != nil {
			tool.start(a)
		}
		var gap rl.Rectangle
		gap, rest = ui.SplitLeft(rest, a.px(2))
		_ = gap
	}

	// Undo and redo sit after a separator, as in the SPEC-UX §2 mock.
	sepX := rest.X + a.px(ui.Spacing)
	a.UI.HairlineV(sepX, r.Y+a.px(8), r.Height-a.px(16), ui.ColorStroke)
	rest.X = sepX + a.px(ui.Spacing)
	rest.Width -= a.px(ui.Spacing * 2)

	sq := a.px(ui.ToolbarHeight - 10)
	var undoBox, redoBox rl.Rectangle
	undoBox, rest = ui.SplitLeft(rest, sq)
	undoBox.Height = btnH
	if a.UI.IconButton(ui.MakeID("tool.undo"), undoBox, ui.DrawUndoIcon, ui.IconOpts{
		Disabled:    !a.Bus.CanUndo(),
		Tooltip:     undoTooltip("Undo", a.Bus.UndoName()),
		Shortcut:    "Ctrl+Z",
		DisabledWhy: "Nothing to undo yet",
	}) {
		a.Undo()
	}
	redoBox, rest = ui.SplitLeft(rest, sq)
	redoBox.Height = btnH
	if a.UI.IconButton(ui.MakeID("tool.redo"), redoBox, ui.DrawRedoIcon, ui.IconOpts{
		Disabled:    !a.Bus.CanRedo(),
		Tooltip:     undoTooltip("Redo", a.Bus.RedoName()),
		Shortcut:    "Ctrl+Y",
		DisabledWhy: "Nothing to redo",
	}) {
		a.Redo()
	}

	// Settings gear, right-aligned.
	gear, _ := ui.SplitRight(rest, sq)
	gear.Height = btnH
	a.UI.IconButton(ui.MakeID("tool.settings"), gear, ui.DrawSettingsIcon, ui.IconOpts{
		Disabled:    true,
		DisabledWhy: "Settings arrive with milestone M8",
	})
}

func undoTooltip(verb, name string) string {
	if name == "" {
		return verb
	}
	return verb + ": " + name
}

// buildTreeHandle draws the collapse chevron on the panel's edge.
func (a *App) buildTreeHandle(r rl.Rectangle) {
	id := ui.MakeID("tree.handle")
	// Only the middle of the strip is a button, so the whole panel edge does not
	// look clickable.
	btnH := a.px(48)
	btn := ui.Rect(r.X, r.Y+(r.Height-btnH)/2, r.Width, btnH)

	dir := 2 // pointing left: click to collapse
	tip := "Collapse the panel"
	if a.tree.collapsed {
		dir = 0
		tip = "Show the panel"
	}
	icon := func(cx, cy, size float64, col color.RGBA) {
		ui.DrawChevron(cx, cy, size, dir, col)
	}
	if a.UI.IconButton(id, btn, icon, ui.IconOpts{Tooltip: tip}) {
		a.tree.collapsed = !a.tree.collapsed
	}
}

func (a *App) px(v float64) float32 { return float32(v * a.Scale) }

// buildTree draws the Planes / Sketches / Bodies panel (SPEC-UX §7).
func (a *App) buildTree(r rl.Rectangle) {
	a.UI.Panel(r)
	a.UI.HairlineV(r.X+r.Width-a.px(1), r.Y, r.Height, ui.ColorStroke)

	doc := a.Doc()
	rowH := a.px(ui.TreeRowHeight)
	// Leave room for the collapse handle and the footer stats line.
	content := ui.Rect(r.X, r.Y+a.px(4), r.Width-a.px(TreeHandleWidth), r.Height-a.px(4))
	footer, content := ui.SplitBottom(content, a.px(22))

	rest := content
	section := func(key, label string, count int, open *bool) bool {
		var head rl.Rectangle
		head, rest = ui.SplitTop(rest, rowH)
		if a.UI.TreeSection(ui.MakeID("tree.section."+key), head, label, count, *open) {
			*open = !*open
		}
		return *open
	}

	if section("planes", "Planes", geom.PlaneCount, &a.tree.planesOpen) {
		for i := 0; i < geom.PlaneCount; i++ {
			var row rl.Rectangle
			row, rest = ui.SplitTop(rest, rowH)
			a.planeRow(row, geom.PlaneKind(i))
		}
	}
	if section("sketches", "Sketches", len(doc.Sketches), &a.tree.sketchesOpen) {
		if len(doc.Sketches) == 0 {
			var row rl.Rectangle
			row, rest = ui.SplitTop(rest, rowH)
			a.emptyRow(row, "No sketches yet")
		}
		for _, s := range doc.Sketches {
			var row rl.Rectangle
			row, rest = ui.SplitTop(rest, rowH)
			a.sketchRow(row, s)
		}
	}
	if section("bodies", "Bodies", len(doc.Bodies), &a.tree.bodiesOpen) {
		if len(doc.Bodies) == 0 {
			var row rl.Rectangle
			row, rest = ui.SplitTop(rest, rowH)
			a.emptyRow(row, "No bodies yet")
		}
		for _, b := range doc.Bodies {
			var row rl.Rectangle
			row, rest = ui.SplitTop(rest, rowH)
			a.bodyRow(row, b)
		}
	}

	a.UI.HairlineH(footer.X, footer.Y, footer.Width, ui.ColorStroke)
	a.UI.Text(ui.InsetXY(footer, a.px(ui.Spacing), 0), doc.Stats().String(),
		ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.8))
}

// emptyRow is the placeholder an empty section shows instead of a blank stare
// (SPEC-UX §14).
func (a *App) emptyRow(r rl.Rectangle, text string) {
	box := r
	box.X += a.px(ui.TreeRowHeight)
	a.UI.Text(box, text, ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.6))
}

// planeRow draws one default plane. It has an eye toggle and nothing else:
// planes can never be renamed or deleted, and the missing buttons are the
// explanation (R1, SPEC-UX §5).
func (a *App) planeRow(r rl.Rectangle, k geom.PlaneKind) {
	doc := a.Doc()
	ref := model.PlaneRef(k)
	visible := doc.PlaneVisible(k)

	res := a.UI.TreeRow(ui.MakeID("tree.plane."+k.String()), r, ui.TreeRowSpec{
		Label:    k.String(),
		Icon:     ui.DrawPlaneIcon,
		Visible:  visible,
		HasEye:   true,
		Selected: a.Sel.Contains(ref),
		Indent:   1,
		Dim:      !visible,
	})
	if res.Hovered {
		a.tree.hovered = ref
	}
	if res.ToggledVisible {
		a.Run(&model.SetPlaneVisible{Plane: k, Visible: !visible})
	}
	if res.Clicked {
		a.selectRef(ref)
	}
}

func (a *App) sketchRow(r rl.Rectangle, s *model.Sketch) {
	ref := model.SketchRef(s.ID)
	editing := a.tree.renaming == ref

	res := a.UI.TreeRow(ui.MakeID("tree.sketch."+itoa(int(s.ID))), r, ui.TreeRowSpec{
		Label:     s.Name,
		Icon:      ui.DrawSketchIcon,
		Visible:   s.Visible,
		HasEye:    true,
		CanRename: true,
		CanDelete: true,
		Selected:  a.Sel.Contains(ref),
		Indent:    1,
		Editing:   editing,
		Dim:       !s.Visible,
	})
	if res.Hovered {
		a.tree.hovered = ref
	}
	switch {
	case res.ToggledVisible:
		a.Run(&model.SetSketchVisible{ID: s.ID, Visible: !s.Visible})
	case res.ClickedDelete:
		a.deleteRef(ref)
	case res.DoubleClicked:
		// Double-clicking a sketch row re-enters editing (SPEC-UX §7); renaming
		// is the pencil, so the two never fight over the same gesture.
		a.EditSketch(s)
	case res.ClickedRename:
		a.beginRename(ref, s.Name)
	case res.Clicked:
		a.selectRef(ref)
	}
	a.finishRename(ref, res.Name, func(name string) {
		a.Run(&model.RenameSketch{ID: s.ID, To: name})
	})
}

func (a *App) bodyRow(r rl.Rectangle, b *model.Body) {
	ref := model.BodyRef(b.ID)
	editing := a.tree.renaming == ref
	swatch := b.Color

	res := a.UI.TreeRow(ui.MakeID("tree.body."+itoa(int(b.ID))), r, ui.TreeRowSpec{
		Label:     b.Name,
		Icon:      ui.DrawBodyIcon,
		Visible:   b.Visible,
		HasEye:    true,
		Swatch:    &swatch,
		CanRename: true,
		CanDelete: true,
		Selected:  a.Sel.Contains(ref),
		Indent:    1,
		Editing:   editing,
		Dim:       !b.Visible,
	})
	if res.Hovered {
		a.tree.hovered = ref
	}
	switch {
	case res.ToggledVisible:
		a.Run(&model.SetBodyVisible{ID: b.ID, Visible: !b.Visible})
	case res.ClickedDelete:
		a.deleteRef(ref)
	case res.ClickedSwatch:
		a.openColorPicker(b, r)
	case res.ClickedRename || res.DoubleClicked:
		a.beginRename(ref, b.Name)
	case res.Clicked:
		a.selectRef(ref)
	}
	a.finishRename(ref, res.Name, func(name string) {
		a.Run(&model.RenameBody{ID: b.ID, To: name})
	})
	a.drawColorPicker(b)
}

func (a *App) openColorPicker(b *model.Body, anchor rl.Rectangle) {
	a.tree.pickerFor = b.ID
	a.UI.OpenColorPicker(ui.MakeID("body.color"), anchor, b.Color)
}

// drawColorPicker runs the open popover and commits its result as one command
// per released drag, so scrubbing the hue does not fill the history.
func (a *App) drawColorPicker(b *model.Body) {
	if a.tree.pickerFor != b.ID {
		return
	}
	id := ui.MakeID("body.color")
	res := a.UI.ColorPicker(id, model.PaletteForBodies())
	if res.Closed {
		a.tree.pickerFor = 0
		return
	}
	if res.Changed && res.Color != b.Color {
		// Live-preview through the bus so the change is undoable as one step.
		if a.Bus.Dragging() {
			_ = a.Bus.UpdateDrag(&model.SetBodyColor{ID: b.ID, To: res.Color})
		} else {
			_ = a.Bus.BeginDrag(&model.SetBodyColor{ID: b.ID, To: res.Color})
		}
	}
	if a.Bus.Dragging() && !a.UI.In.Down[ui.MouseLeft] {
		a.Bus.CommitDrag()
	}
}

func (a *App) beginRename(ref model.Ref, current string) {
	a.tree.renaming = ref
	a.UI.SetFocus(ui.MakeID("tree.rename").Child(current), current)
}

// finishRename commits or abandons an inline edit.
func (a *App) finishRename(ref model.Ref, res ui.FieldResult, apply func(string)) {
	if a.tree.renaming != ref {
		return
	}
	switch {
	case res.Committed:
		a.tree.renaming = model.Ref{}
		if res.Changed {
			apply(res.Text)
		}
	case res.Cancelled:
		a.tree.renaming = model.Ref{}
	}
}

// selectRef applies the click modifiers of SPEC-UX §7: plain click replaces,
// Shift adds, Ctrl toggles.
func (a *App) selectRef(ref model.Ref) {
	switch {
	case a.UI.In.Shift:
		a.Sel.Add(ref)
	case a.UI.In.Ctrl:
		a.Sel.Toggle(ref)
	default:
		a.Sel.Set(ref)
	}
}

// deleteRef removes what a row points at, with the undo affordance of
// SPEC-UX §7 — a toast carrying Undo, never a modal.
func (a *App) deleteRef(ref model.Ref) {
	switch ref.Kind {
	case model.SelPlane:
		// The default planes are permanent; say so instead of doing nothing.
		a.Toast(ui.Toast{
			Text: "Default planes can't be deleted — you can hide them",
			Kind: ui.ToastWarn,
		})
	case model.SelBody:
		name := a.bodyName(ref.Body)
		if a.Run(&model.DeleteBody{ID: ref.Body}) {
			a.toastWithUndo("Deleted " + name)
		}
	case model.SelSketch:
		name := ""
		if s := a.Doc().SketchByID(ref.Sketch); s != nil {
			name = s.Name
		}
		if a.Run(&model.DeleteSketch{ID: ref.Sketch}) {
			a.toastWithUndo("Deleted " + name)
		}
	}
}

func (a *App) toastWithUndo(text string) {
	a.Toast(ui.Toast{
		Text:     text,
		Action:   "Undo",
		OnAction: func() { a.Undo() },
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// buildSketchToolbar replaces the modelling tools with the drawing ones while a
// sketch is being edited (SPEC-UX §8.2).
func (a *App) buildSketchToolbar(r rl.Rectangle) {
	a.UI.Panel(r)
	a.UI.HairlineH(r.X, r.Y+r.Height-a.px(1), r.Width, ui.ColorStroke)

	sess := a.sketch.session
	inner := ui.InsetXY(r, a.px(ui.Spacing), a.px(4))
	btnH := inner.Height
	rest := inner

	tools := []struct {
		tool sketch.Tool
		icon ui.IconFunc
	}{
		{sketch.ToolSelect, ui.DrawCursorIcon},
		{sketch.ToolLine, ui.DrawLineToolIcon},
		{sketch.ToolRect, ui.DrawRectToolIcon},
		{sketch.ToolCircle, ui.DrawCircleToolIcon},
	}
	for _, t := range tools {
		label := t.tool.String()
		w := a.px(ui.IconSize+ui.Spacing) + a.UI.TextWidth(label, ui.FontSizeUI) + a.px(ui.Spacing)
		var box rl.Rectangle
		box, rest = ui.SplitLeft(rest, w)
		box.Height = btnH
		if a.UI.IconButton(ui.MakeID("sketchtool."+label), box, t.icon, ui.IconOpts{
			Label:    label,
			Active:   sess.Tool == t.tool,
			Tooltip:  label,
			Shortcut: t.tool.Shortcut(),
		}) {
			sess.SetTool(t.tool)
		}
		var gap rl.Rectangle
		gap, rest = ui.SplitLeft(rest, a.px(2))
		_ = gap
	}

	// Extrude sits after a separator: it is not a drawing tool, it is what you
	// do with what you have drawn (SPEC-UX §9.1).
	sepX := rest.X + a.px(ui.Spacing)
	a.UI.HairlineV(sepX, r.Y+a.px(8), r.Height-a.px(16), ui.ColorStroke)
	rest.X = sepX + a.px(ui.Spacing)
	rest.Width -= a.px(ui.Spacing * 2)

	closed := len(a.ActiveSketch().Arrangement().Regions) > 0
	exW := a.px(ui.IconSize+ui.Spacing) + a.UI.TextWidth("Extrude", ui.FontSizeUI) + a.px(ui.Spacing)
	var exBox rl.Rectangle
	exBox, rest = ui.SplitLeft(rest, exW)
	exBox.Height = btnH
	if a.UI.IconButton(ui.MakeID("sketchtool.extrude"), exBox, ui.DrawExtrudeIcon, ui.IconOpts{
		Label:       "Extrude",
		Disabled:    !closed,
		Shortcut:    "E",
		Tooltip:     "Pull the selected region into a solid",
		DisabledWhy: "Close the red endpoints first — extrude needs a closed region",
	}) {
		a.BeginExtrude()
	}

	// Finish and cancel sit on the right, mirroring the card's footer.
	btnW := a.px(96)
	var doneBox, cancelBox rl.Rectangle
	doneBox, rest = ui.SplitRight(rest, btnW)
	doneBox.Height = btnH
	if a.UI.Button(ui.MakeID("sketch.done"), doneBox, "Finish", ui.ButtonOpts{
		Style:   ui.ButtonPrimary,
		Tooltip: "Keep this sketch and go back",
	}) {
		a.ExitSketch(true)
	}
	cancelBox, _ = ui.SplitRight(rest, btnW+a.px(6))
	cancelBox.Width -= a.px(6)
	cancelBox.Height = btnH
	if a.UI.Button(ui.MakeID("sketch.cancel"), cancelBox, "Close", ui.ButtonOpts{
		Style:   ui.ButtonGhost,
		Tooltip: "Leave sketch mode — the sketch is kept either way",
	}) {
		a.ExitSketch(true)
	}
}

// buildSketchCard is the contextual panel on the right of the viewport: what
// the sketch contains, and the circle segment count (SPEC-UX §8.1, §8.3).
func (a *App) buildSketchCard(viewport rl.Rectangle) {
	s := a.ActiveSketch()
	sess := a.sketch.session
	if s == nil || sess == nil {
		return
	}

	w := a.px(232)
	h := a.px(150)
	if s.Consumed {
		h += a.px(44)
	}
	// Below the view cube, never covering it (SPEC-UX §2).
	box := ui.Rect(
		viewport.X+viewport.Width-w-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		w, h)

	card := a.UI.FloatingCard(ui.MakeID("sketch.card"), box, s.Name, ui.FloatingCardOpts{})
	body := card.Body
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)

	arr := s.Arrangement()
	var row rl.Rectangle
	row, body = ui.SplitTop(body, line)
	a.UI.Text(row, s.Summary(), ui.FontSizeSmall, ui.ColorTextDim)

	// Open ends are the thing that blocks extrude, so they get their own line
	// in the error colour (SPEC-UX §8.6).
	row, body = ui.SplitTop(body, line)
	if n := len(arr.OpenEnds); n > 0 {
		a.UI.Text(row, plural(n, "open end", "open ends")+" — close them to extrude",
			ui.FontSizeSmall, ui.ColorError)
	} else if len(arr.Regions) > 0 {
		a.UI.Text(row, "Profile is closed", ui.FontSizeSmall, ui.ColorSuccess)
	}

	body.Y += a.px(4)
	body.Height -= a.px(4)

	// The segment count applies to new circles, and to a selected one.
	row, body = ui.SplitTop(body, line)
	a.UI.Text(row, "Circle segments", ui.FontSizeSmall, ui.ColorTextDim)

	row, body = ui.SplitTop(body, a.px(24))
	segs := sess.CircleSegs
	if i, ok := a.selectedCircle(s, sess); ok {
		segs = s.Entities[i].Segs
	}
	labels := []string{"8", "16", "32", "64"}
	values := []int{8, 16, 32, 64}
	selected := -1
	for i, v := range values {
		if v == segs {
			selected = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("sketch.segs"), row, labels, selected,
		ui.ChipGroupOpts{Tooltip: "Sides of a circle"}); changed {
		sess.CircleSegs = values[pick]
		if i, ok := a.selectedCircle(s, sess); ok {
			a.Run(&model.SetCircleSegs{Sketch: s.ID, Index: i, Segs: values[pick]})
		}
	}

	// Re-editing a consumed sketch says so, rather than pretending the bodies
	// will follow along (SPEC-UX §8.8).
	if s.Consumed {
		row, _ = ui.SplitTop(body, line*2)
		a.UI.Text(row, "Editing this sketch won't change existing bodies.",
			ui.FontSizeSmall, ui.ColorWarn)
	}
}

// selectedCircle returns the index of the single selected circle, if that is
// what the selection is.
func (a *App) selectedCircle(s *model.Sketch, sess *sketch.Session) (int, bool) {
	if len(sess.Selected) != 1 {
		return 0, false
	}
	i := sess.Selected[0]
	if i < 0 || i >= len(s.Entities) || s.Entities[i].Kind != model.EntCircle {
		return 0, false
	}
	return i, true
}

// buildExtrudeCard is the options panel of SPEC-UX §9.3: depth, direction,
// draft, result and through-all, with a footer that confirms or cancels.
func (a *App) buildExtrudeCard(viewport rl.Rectangle) {
	t := a.extrude.tool
	if t == nil {
		return
	}
	w := a.px(248)
	h := a.px(298)
	box := ui.Rect(
		viewport.X+viewport.Width-w-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		w, h)

	// The confirm button is only live when a solid actually built. A preview
	// that failed is the most reliable answer there is to "will this work",
	// and it comes with the sentence explaining why not.
	valid, why := t.Valid()
	if valid && a.extrude.previewErr != "" {
		valid, why = false, a.extrude.previewErr
	}
	card := a.UI.FloatingCard(ui.MakeID("extrude.card"), box, "Extrude", ui.FloatingCardOpts{
		Footer:          true,
		ConfirmLabel:    "Extrude",
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

	// Depth: a drag-number field kept in sync with the arrow both ways.
	r := row(a.px(24))
	labelBox, fieldBox := ui.SplitLeft(r, a.px(64))
	a.UI.Text(labelBox, "Depth", ui.FontSizeSmall, ui.ColorTextDim)
	flipBox, fieldBox := ui.SplitRight(fieldBox, a.px(28))
	// Through-All owns the depth while it is on, so the field shows what the
	// scene measured rather than pretending the number is still yours to set.
	shown := t.DepthUnits
	if t.ThroughAll {
		shown = t.EffectiveDepth()
	}
	if v, res := a.UI.DragNumber(ui.MakeID("extrude.depth"), fieldBox, shown, ui.NumberOpts{
		Unit: "u", Step: 1, FineStep: 0.25, Decimals: 2,
		Min: -tools.MaxDepthUnits, Max: tools.MaxDepthUnits,
		Disabled:    t.ThroughAll,
		Tooltip:     "Drag to scrub, click to type",
		DisabledWhy: "Through all sets the depth from the scene",
	}); res.Changed {
		t.SetDepth(v)
		a.rebuildExtrudePreview()
	}
	if a.UI.IconButton(ui.MakeID("extrude.flip"), flipBox, ui.DrawFlipIcon, ui.IconOpts{
		Tooltip: "Flip the direction",
	}) {
		t.Flip()
		a.rebuildExtrudePreview()
	}

	body.Y += a.px(4)
	body.Height -= a.px(4)

	// Direction chips.
	a.UI.Text(row(line), "Direction", ui.FontSizeSmall, ui.ColorTextDim)
	dirs := []extrude.Direction{extrude.Normal, extrude.Reverse, extrude.Symmetric}
	dirLabels := []string{"Normal", "Reverse", "Symmetric"}
	sel := 0
	for i, d := range dirs {
		if d == t.Dir {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("extrude.dir"), row(a.px(24)),
		dirLabels, sel, ui.ChipGroupOpts{}); changed {
		t.Dir = dirs[pick]
		a.rebuildExtrudePreview()
	}

	body.Y += a.px(4)
	body.Height -= a.px(4)

	// Draft, with the clamp warning of SPEC-UX §9.3.
	r = row(line)
	a.UI.Text(r, "Draft", ui.FontSizeSmall, ui.ColorTextDim)
	if t.Clamped {
		wb, _ := ui.SplitRight(r, a.px(120))
		a.UI.Text(wb, fmt.Sprintf("clamped to %.1f°", t.AchievedDraft),
			ui.FontSizeSmall, ui.ColorWarn)
	}
	r = row(a.px(24))
	sliderBox, draftBox := ui.SplitRight(r, a.px(64))
	if v, changed := a.UI.Slider(ui.MakeID("extrude.draftslider"), draftBox, t.Draft,
		-extrude.MaxDraftDegrees, extrude.MaxDraftDegrees, ui.ButtonOpts{}); changed {
		t.SetDraft(v)
		a.rebuildExtrudePreview()
	}
	if v, res := a.UI.DragNumber(ui.MakeID("extrude.draft"), sliderBox, t.Draft, ui.NumberOpts{
		Unit: "°", Step: 1, FineStep: 0.5, Decimals: 1,
		Min: -extrude.MaxDraftDegrees, Max: extrude.MaxDraftDegrees,
		Warn:        t.Clamped,
		WarnTooltip: "Clamped — the profile is too tight for more draft",
	}); res.Changed {
		t.SetDraft(v)
		a.rebuildExtrudePreview()
	}

	body.Y += a.px(4)
	body.Height -= a.px(4)

	// Result: only New works until the boolean kernel lands, and the others say
	// so rather than silently doing nothing.
	a.UI.Text(row(line), "Result", ui.FontSizeSmall, ui.ColorTextDim)
	results := []tools.Result{tools.ResultNew, tools.ResultAdd, tools.ResultSubtract, tools.ResultIntersect}
	resLabels := make([]string, len(results))
	disabled := make([]bool, len(results))
	reasons := make([]string, len(results))
	resSel := 0
	for i, res := range results {
		resLabels[i] = res.String()
		disabled[i] = !res.Available()
		reasons[i] = res.UnavailableReason()
		if res == t.Result {
			resSel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("extrude.result"), row(a.px(24)),
		resLabels, resSel, ui.ChipGroupOpts{
			PerChipDisabled: disabled, PerChipWhy: reasons,
		}); changed {
		t.Result = results[pick]
	}

	// Through all: the depth stops being something you drag and becomes
	// whatever clears the scene.
	if a.UI.Toggle(ui.MakeID("extrude.through"), row(a.px(22)), "Through all",
		t.ThroughAll, ui.ButtonOpts{
			Tooltip: "Run past everything in the scene instead of a set depth",
		}) {
		t.ThroughAll = !t.ThroughAll
		a.rebuildExtrudePreview()
	}

	if a.extrude.previewErr != "" {
		a.UI.Text(row(line), a.extrude.previewErr, ui.FontSizeSmall, ui.ColorError)
	}

	if card.Confirmed {
		a.CommitExtrude()
	}
	if card.Cancelled {
		a.CancelExtrude()
	}
}
