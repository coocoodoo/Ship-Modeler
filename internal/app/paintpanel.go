package app

import (
	"fmt"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/paint"
	"modeler/internal/ui"
)

// The palette panel of SPEC-UX §13.1. It replaces the contextual card while
// paint mode is on, and it is taller than the other cards because it is the one
// panel you work out of continuously rather than dismiss.
//
// The spec draws it with emoji. None of them are in the font atlas, and a
// missing glyph is a box, so every one is a stroke icon instead (D-11).

// Panel metrics in logical pixels.
const (
	paintPanelWidth   = 236
	paintSwatchSize   = 22
	paintSwatchGap    = 4
	paintPaletteCols  = 8
	paintPaletteRows  = 4
	paintRecentsCount = 8
)

// buildPaintPanel lays out and runs the whole palette panel.
func (a *App) buildPaintPanel(viewport rl.Rectangle) {
	st := &a.paint
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	swatch := a.px(paintSwatchSize)
	gap := a.px(paintSwatchGap)

	// The prompt and the oblique chip are the two things that come and go, so
	// the panel is measured rather than fixed: a card with a hole in it where a
	// warning sometimes goes looks broken.
	mismatch, mismatchRes := a.paintResMismatch()
	oblique := a.paintOblique()
	// The dither modes and the fill toggle only mean anything to some tools, so
	// they only appear for those tools. A panel that showed every control every
	// tool might ever want would be a panel mostly full of greyed-out rows.
	showDither := st.tool == paint.ToolBrush || st.tool == paint.ToolGradient
	showFill := st.tool.Shape()

	h := a.px(38) + // title
		line + a.px(26)*2 + a.px(4) + a.px(6) + // two rows of tools
		line + a.px(24) + a.px(6) + // size
		line + a.px(24) + a.px(8) + // res
		line + float32(paintPaletteRows)*(swatch+gap) + a.px(6) + // palette
		line + swatch + a.px(8) + // recents
		a.px(26) + a.px(6) + // current colour + custom
		a.px(26) + a.px(4) // import + textures
	if len(st.custom) > 0 {
		h += a.px(24) + a.px(4)
	}
	if mismatch {
		h += line*2 + a.px(26) + a.px(6)
	}
	if oblique {
		h += a.px(26) + a.px(6)
	}
	// The lock row is always there. It is the one control whose state you have
	// to be able to read at a glance — "am I about to paint the face I think I
	// am" is not a question worth hunting for an answer to.
	h += a.px(26) + a.px(6)
	if st.locked {
		h += line
	}
	if showDither {
		h += line + a.px(24) + a.px(6)
	}
	if showFill {
		h += a.px(24) + a.px(6)
	}

	box := ui.Rect(
		viewport.X+viewport.Width-a.px(paintPanelWidth)-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		a.px(paintPanelWidth), h)
	// The panel is taller than the others, and on a short window it would run
	// off the bottom into the hint bar. Sliding it up is better than clipping
	// it: every control stays reachable.
	if bottom := viewport.Y + viewport.Height - a.px(ui.Spacing); box.Y+box.Height > bottom {
		box.Y = bottom - box.Height
		if box.Y < viewport.Y+a.px(ui.Spacing) {
			box.Y = viewport.Y + a.px(ui.Spacing)
		}
	}

	card := a.UI.FloatingCard(ui.MakeID("paint.card"), box, "Paint", ui.FloatingCardOpts{})
	body := card.Body
	row := func(height float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, height)
		return r
	}
	space := func(v float64) {
		body.Y += a.px(v)
		body.Height -= a.px(v)
	}

	a.UI.Text(row(line), "Tools", ui.FontSizeSmall, ui.ColorTextDim)
	a.paintToolRow(row(a.px(26)), paintPixelTools())
	space(4)
	a.paintToolRow(row(a.px(26)), paintShapeTools())
	space(6)

	a.UI.Text(row(line), "Size", ui.FontSizeSmall, ui.ColorTextDim)
	sizeLabels := make([]string, len(paint.BrushSizes))
	sizeSel := 0
	for i, s := range paint.BrushSizes {
		sizeLabels[i] = itoa(s)
		if s == st.size {
			sizeSel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.size"), row(a.px(24)),
		sizeLabels, sizeSel, ui.ChipGroupOpts{
			Tooltip: "Brush square, in texels",
		}); changed {
		st.size = paint.BrushSizes[pick]
	}
	space(6)

	if showFill {
		if a.UI.Toggle(ui.MakeID("paint.fillshape"), row(a.px(24)), "Fill the shape",
			st.fillShape, ui.ButtonOpts{
				Tooltip: "Solid, or just the outline",
			}) {
			st.fillShape = !st.fillShape
		}
		space(6)
	}

	if showDither {
		a.paintDitherRow(row(line), row(a.px(24)))
		space(6)
	}

	a.paintResRow(row(line), row(a.px(24)))
	if mismatch {
		space(6)
		a.paintMismatchPrompt(row(line*2), row(a.px(26)), mismatchRes)
	}
	space(8)

	if len(st.custom) > 0 {
		pages := []string{"Built-in", "Imported"}
		if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.page"), row(a.px(24)),
			pages, st.page, ui.ChipGroupOpts{
				Tooltip: "Which page of colours the grid shows",
			}); changed {
			st.page = pick
		}
		space(4)
	}

	a.UI.Text(row(line), "Palette", ui.FontSizeSmall, ui.ColorTextDim)
	a.paintPaletteGrid(row(float32(paintPaletteRows)*(swatch+gap)), swatch, gap)
	space(6)

	a.UI.Text(row(line), "Recents", ui.FontSizeSmall, ui.ColorTextDim)
	a.paintRecentsStrip(row(swatch), swatch, gap)
	space(8)

	a.paintColorRow(row(a.px(26)))
	space(6)
	a.paintFooterRow(row(a.px(26)))

	space(6)
	a.paintLockRow(row, line)

	if oblique {
		space(6)
		if a.UI.Button(ui.MakeID("paint.faceview"), row(a.px(26)), "Face view", ui.ButtonOpts{
			Tooltip: "Turn the camera square-on to this face",
		}) {
			a.FaceView()
		}
	}
}

// paintLockRow is the face lock (SPEC-UX §13.5): one button that becomes the
// thing you press to get out of it, plus a line naming what you are locked to.
func (a *App) paintLockRow(row func(float32) rl.Rectangle, line float32) {
	st := &a.paint
	if !st.locked {
		_, hovering := a.stickyFace()
		if a.UI.Button(ui.MakeID("paint.lock"), row(a.px(26)), "Lock to this face",
			ui.ButtonOpts{
				Disabled:    !hovering,
				Tooltip:     "Face the camera at it, and paint nothing else",
				DisabledWhy: "Hover the face you want to lock to first",
			}) {
			a.LockToFace()
		}
		return
	}

	label := "this face"
	if f, ok := a.resolveFace(st.lockBody, st.lockFace); ok {
		label = fmt.Sprintf("face %d of %s", st.lockFace.Seq(), f.body.Name)
	}
	a.UI.Text(row(line), "Locked to "+label, ui.FontSizeSmall, ui.ColorAccent)

	r := row(a.px(26))
	recentre, unlock := ui.SplitLeft(r, r.Width*0.5)
	unlock.X += a.px(6)
	unlock.Width -= a.px(6)
	if a.UI.Button(ui.MakeID("paint.recentre"), recentre, "Recentre", ui.ButtonOpts{
		Tooltip: "Point the camera back at the locked face",
	}) {
		a.FaceView()
	}
	if a.UI.Button(ui.MakeID("paint.unlock"), unlock, "Unlock", ui.ButtonOpts{
		Style:    ui.ButtonPrimary,
		Tooltip:  "Paint any face again",
		Shortcut: "Esc",
	}) {
		a.UnlockFace()
	}
}

// paintTool is one entry in the tool rows.
type paintTool struct {
	tool paint.Tool
	icon ui.IconFunc
	tip  string
}

// paintPixelTools are the ones that paint where the pointer goes.
func paintPixelTools() []paintTool {
	return []paintTool{
		{paint.ToolPencil, ui.DrawPencilIcon, "Paint one texel at a time"},
		{paint.ToolBrush, ui.DrawPaintIcon, "A round brush that fades out at its edge"},
		{paint.ToolEraser, ui.DrawEraserIcon, "Rub back to the body's own colour"},
		{paint.ToolFill, ui.DrawFillIcon, "Flood the matching texels around the one you click"},
		{paint.ToolPick, ui.DrawDropperIcon, "Pick up the colour under the cursor (or hold Alt)"},
	}
}

// paintShapeTools are the ones decided by where you pressed and where you let
// go. Shift keeps a line straight and a box square.
func paintShapeTools() []paintTool {
	return []paintTool{
		{paint.ToolLine, ui.DrawLineToolIcon, "Drag a straight line — Shift keeps it square on"},
		{paint.ToolRect, ui.DrawRectToolIcon, "Drag a rectangle — Shift makes it a square"},
		{paint.ToolCircle, ui.DrawCircleToolIcon, "Drag an ellipse — Shift makes it a circle"},
		{paint.ToolGradient, ui.DrawGradientIcon,
			"Drag to ramp from the near colour to the far one"},
	}
}

// paintToolRow draws one row of tools, each with its shortcut on the tooltip.
//
// Both rows are laid out on the same five-column grid so the icons line up
// under each other even though one row is a tool shorter.
func (a *App) paintToolRow(r rl.Rectangle, tools []paintTool) {
	const columns = 5
	gap := a.px(4)
	w := (r.Width - gap*float32(columns-1)) / float32(columns)
	for i, t := range tools {
		box := ui.Rect(r.X+float32(i)*(w+gap), r.Y, w, r.Height)
		if a.UI.IconButton(ui.MakeID("paint.tool."+t.tool.String()), box, t.icon, ui.IconOpts{
			Active:   a.paint.tool == t.tool,
			Tooltip:  t.tip,
			Shortcut: t.tool.Shortcut(),
		}) {
			a.paint.tool = t.tool
		}
	}
}

// paintDitherRow is the Bayer modes (SPEC-UX §13.4). They apply to the soft
// brush and to the gradient, which are the two tools with a coverage between
// nothing and everything to spend.
func (a *App) paintDitherRow(label, chips rl.Rectangle) {
	st := &a.paint
	a.UI.Text(label, "Dither", ui.FontSizeSmall, ui.ColorTextDim)
	note, _ := ui.SplitRight(label, a.px(130))
	text := "blends"
	if st.dither != paint.DitherNone {
		text = "two colours only"
	}
	a.UI.Text(note, text, ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.9))

	labels := make([]string, len(paint.Dithers))
	sel := 0
	for i, d := range paint.Dithers {
		labels[i] = d.String()
		if d == st.dither {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.dither"), chips, labels, sel,
		ui.ChipGroupOpts{
			Tooltip: "Spread a half-shade over whole texels instead of blending",
		}); changed {
		st.dither = paint.Dithers[pick]
	}
}

// paintResRow is the resolution chips, with the density they would give the
// face under the cursor spelled out beside them.
func (a *App) paintResRow(label, chips rl.Rectangle) {
	st := &a.paint
	a.UI.Text(label, "Res", ui.FontSizeSmall, ui.ColorTextDim)
	// The sticky face again, so the note and the prompt under it tell one story
	// rather than one of them vanishing when the pointer reaches the panel.
	if h, ok := a.stickyFace(); ok && h.paint != nil {
		note, _ := ui.SplitRight(label, a.px(120))
		text := fmt.Sprintf("%.3f u / texel", h.paint.Texel)
		col := ui.ColorTextDim
		if h.allocated && h.paint.Res != st.res {
			text = fmt.Sprintf("this face is %d px", h.paint.Res)
			col = ui.ColorWarn
		}
		a.UI.Text(note, text, ui.FontSizeSmall, ui.Fade(col, 0.9))
	}

	labels := make([]string, len(paint.Resolutions))
	sel := -1
	for i, res := range paint.Resolutions {
		labels[i] = itoa(res)
		if res == st.res {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.res"), chips, labels, sel,
		ui.ChipGroupOpts{
			Tooltip: "How many texels across a face is at its widest, set when it is first painted",
		}); changed {
		a.SetPaintRes(paint.Resolutions[pick])
	}
}

// paintResMismatch reports whether the face under the cursor already has a
// texture at a different resolution, and what that resolution is.
func (a *App) paintResMismatch() (bool, int) {
	// The sticky face, not the live one: this prompt's own buttons are in the
	// panel, and reading the live hover would take them away as you reached for
	// them.
	h, ok := a.stickyFace()
	if !ok || !h.allocated || h.paint == nil || h.paint.Res == a.paint.res {
		return false, 0
	}
	return true, h.paint.Res
}

// paintMismatchPrompt is the inline offer of SPEC-UX §13.2: match the face, or
// rebuild the face to match the brush. Painting is allowed either way — the
// prompt is an offer, not a gate, and the stroke keeps the face's own density
// because texel size never changes implicitly.
func (a *App) paintMismatchPrompt(text, buttons rl.Rectangle, faceRes int) {
	// Two calls rather than one string with a newline in it: the text drawer
	// measures and truncates a whole string, so a second line inside one would
	// be cut off rather than wrapped.
	first, second := ui.SplitTop(text, text.Height/2)
	a.UI.Text(first, fmt.Sprintf("This face is %d px — switch the", faceRes),
		ui.FontSizeSmall, ui.ColorWarn)
	a.UI.Text(second, fmt.Sprintf("brush to %d, or resample it to %d?", faceRes, a.paint.res),
		ui.FontSizeSmall, ui.ColorWarn)

	h, _ := a.stickyFace()
	gap := a.px(6)
	w := (buttons.Width - gap) / 2
	left := ui.Rect(buttons.X, buttons.Y, w, buttons.Height)
	right := ui.Rect(buttons.X+w+gap, buttons.Y, w, buttons.Height)

	if a.UI.Button(ui.MakeID("paint.matchface"), left, fmt.Sprintf("Use %d", faceRes),
		ui.ButtonOpts{Tooltip: "Set the brush to this face's resolution"}) {
		a.SetPaintRes(faceRes)
	}
	if a.UI.Button(ui.MakeID("paint.resample"), right, fmt.Sprintf("Resample %d", a.paint.res),
		ui.ButtonOpts{
			Style:   ui.ButtonPrimary,
			Tooltip: "Rebuild this face's picture at the brush's resolution, nearest sampled",
		}) {
		a.ResampleFace(h.body, h.face, a.paint.res)
	}
}

// paintOblique reports whether the face under the cursor is steep enough to be
// worth offering to turn the camera at.
func (a *App) paintOblique() bool {
	h, ok := a.stickyFace()
	return ok && h.obliqueDeg > ObliqueWarnDegrees
}

// paintPaletteGrid draws the 8x4 page of swatches.
func (a *App) paintPaletteGrid(r rl.Rectangle, swatch, gap float32) {
	cols := a.paintPalette()
	for i, c := range cols {
		if i >= paintPaletteCols*paintPaletteRows {
			break
		}
		col, row := i%paintPaletteCols, i/paintPaletteCols
		box := ui.Rect(r.X+float32(col)*(swatch+gap), r.Y+float32(row)*(swatch+gap), swatch, swatch)
		if a.UI.ColorSwatch(ui.MakeID("paint.swatch"+itoa(i)), box, c, ui.SwatchOpts{
			Tooltip:  "#" + paint.Hex(c),
			Selected: sameColor(c, a.activeColor()),
		}) {
			a.setPaintColor(c)
		}
	}
}

// paintRecentsStrip draws the eight most recently used colours, newest first.
func (a *App) paintRecentsStrip(r rl.Rectangle, swatch, gap float32) {
	recents := a.paint.recents.List()
	for i := 0; i < paintRecentsCount; i++ {
		box := ui.Rect(r.X+float32(i)*(swatch+gap), r.Y, swatch, swatch)
		if i >= len(recents) {
			a.UI.ColorSwatch(ui.MakeID("paint.recent"+itoa(i)), box, color.RGBA{},
				ui.SwatchOpts{Empty: true})
			continue
		}
		c := recents[i]
		if a.UI.ColorSwatch(ui.MakeID("paint.recent"+itoa(i)), box, c, ui.SwatchOpts{
			Tooltip:  "#" + paint.Hex(c),
			Selected: sameColor(c, a.activeColor()),
		}) {
			a.setPaintColor(c)
		}
	}
}

// paintColorRow is the two armed colours and the way to a custom one.
//
// Two rather than one because a gradient needs both ends, and they are ordinary
// armed colours rather than a setting buried in the gradient tool: picking the
// far end up with the eyedropper has to be the same gesture as picking the near
// one. Clicking a swatch points the palette, the dropper and the picker at it.
func (a *App) paintColorRow(r rl.Rectangle) {
	st := &a.paint
	sw := a.px(paintSwatchSize)
	gap := a.px(4)

	slot := func(i int, c color.RGBA, tip string, x float32) {
		box := ui.Rect(x, r.Y+(r.Height-sw)/2, sw, sw)
		if a.UI.ColorSwatch(ui.MakeID("paint.slot"+itoa(i)), box, c, ui.SwatchOpts{
			Tooltip:  tip + ": #" + paint.Hex(c),
			Selected: st.slot == i,
		}) {
			if st.slot == i {
				// A second click on the slot already armed opens the mixer,
				// which is where you were heading anyway.
				a.openPaintPicker(r)
			}
			st.slot = i
		}
	}
	slot(0, st.color, "Near colour", r.X)
	slot(1, st.colorB, "Far colour", r.X+sw+gap)

	swapBox := ui.Rect(r.X+2*(sw+gap), r.Y+(r.Height-sw)/2, sw, sw)
	if a.UI.IconButton(ui.MakeID("paint.swap"), swapBox, ui.DrawSwapIcon, ui.IconOpts{
		Tooltip:  "Swap the two colours",
		Shortcut: "X",
	}) {
		a.swapPaintColors()
	}

	rest := r
	rest.X = swapBox.X + sw + a.px(6)
	rest.Width = r.X + r.Width - rest.X
	if a.UI.Button(ui.MakeID("paint.custom"), rest, "Custom colour…", ui.ButtonOpts{
		Tooltip: "Mix a colour that is not on the page",
	}) {
		a.openPaintPicker(r)
	}
	a.drawPaintPicker()
}

// paintFooterRow is the import affordance and the textures eye.
func (a *App) paintFooterRow(r rl.Rectangle) {
	importBox, rest := ui.SplitLeft(r, r.Width*0.58)
	rest.X += a.px(6)
	rest.Width -= a.px(6)

	// The dialog itself belongs to M8 (SPEC-DATA §5). Until then the import is
	// real and reachable — drop the file on the window — and the button says so
	// rather than pretending nothing exists (SPEC-UX §15).
	a.UI.Button(ui.MakeID("paint.import"), importBox, "Import .hex", ui.ButtonOpts{
		Disabled: true,
		DisabledWhy: "File dialogs arrive with M8 — for now, " +
			"drop a .hex palette onto the window",
	})

	label := "Textures"
	if a.UI.Toggle(ui.MakeID("paint.textures"), rest, label, !a.paint.hideTextures,
		ui.ButtonOpts{
			Tooltip: "Show the paint, or the bare geometry under it",
		}) {
		a.paint.hideTextures = !a.paint.hideTextures
	}
}

// openPaintPicker opens the HSV popover on the brush colour.
func (a *App) openPaintPicker(anchor rl.Rectangle) {
	a.UI.OpenColorPicker(ui.MakeID("paint.picker"), anchor, a.activeColor())
}

// drawPaintPicker runs the open popover. Unlike a body's colour this is not a
// document edit, so it commits nothing: the palette is settings (SPEC-DATA §6).
func (a *App) drawPaintPicker() {
	id := ui.MakeID("paint.picker")
	if !a.UI.ColorPickerOpen(id) {
		return
	}
	res := a.UI.ColorPicker(id, paint.DefaultPalette()[:16])
	if res.Changed {
		c := res.Color
		c.A = 255
		if a.paint.slot == 1 {
			a.paint.colorB = c
		} else {
			a.paint.color = c
		}
	}
	if res.Closed {
		// The strip records what you settled on, not every colour the cursor
		// crossed on the way there.
		a.paint.recents.Add(a.activeColor())
	}
}

// sameColor compares two colours ignoring alpha, which the palette never uses.
func sameColor(a, b color.RGBA) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B
}
