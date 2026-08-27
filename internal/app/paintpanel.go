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

	h := a.px(38) + // title
		line + a.px(26) + a.px(6) + // tools
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
	a.paintToolRow(row(a.px(26)))
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

	if oblique {
		space(6)
		if a.UI.Button(ui.MakeID("paint.faceview"), row(a.px(26)), "Face view", ui.ButtonOpts{
			Tooltip: "Turn the camera square-on to this face",
		}) {
			a.FaceView()
		}
	}
}

// paintToolRow is the four tools, each with its shortcut on the tooltip.
func (a *App) paintToolRow(r rl.Rectangle) {
	tools := []struct {
		tool paint.Tool
		icon ui.IconFunc
		tip  string
	}{
		{paint.ToolPencil, ui.DrawPencilIcon, "Paint one texel at a time"},
		{paint.ToolEraser, ui.DrawEraserIcon, "Rub back to the body's own colour"},
		{paint.ToolFill, ui.DrawFillIcon, "Flood the matching texels around the one you click"},
		{paint.ToolPick, ui.DrawDropperIcon, "Pick up the colour under the cursor (or hold Alt)"},
	}
	gap := a.px(4)
	w := (r.Width - gap*float32(len(tools)-1)) / float32(len(tools))
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

// paintResRow is the resolution chips, with the density they would give the
// face under the cursor spelled out beside them.
func (a *App) paintResRow(label, chips rl.Rectangle) {
	st := &a.paint
	a.UI.Text(label, "Res", ui.FontSizeSmall, ui.ColorTextDim)
	if h := st.hover; h.ok && h.paint != nil {
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
	h := a.paint.hover
	if !h.ok || !h.allocated || h.paint == nil || h.paint.Res == a.paint.res {
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

	h := a.paint.hover
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
	h := a.paint.hover
	return h.ok && h.obliqueDeg > ObliqueWarnDegrees
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
			Selected: sameColor(c, a.paint.color),
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
			Selected: sameColor(c, a.paint.color),
		}) {
			a.setPaintColor(c)
		}
	}
}

// paintColorRow is the armed colour and the way to a custom one.
func (a *App) paintColorRow(r rl.Rectangle) {
	swatchBox, rest := ui.SplitLeft(r, a.px(paintSwatchSize+8))
	swatchBox = ui.Inset(swatchBox, a.px(2))
	swatchBox.Width = a.px(paintSwatchSize)
	if a.UI.ColorSwatch(ui.MakeID("paint.current"), swatchBox, a.paint.color, ui.SwatchOpts{
		Tooltip:  "The brush colour: #" + paint.Hex(a.paint.color),
		Selected: true,
	}) {
		a.openPaintPicker(r)
	}
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
	a.UI.OpenColorPicker(ui.MakeID("paint.picker"), anchor, a.paint.color)
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
		a.paint.color = c
	}
	if res.Closed {
		// The strip records what you settled on, not every colour the cursor
		// crossed on the way there.
		a.paint.recents.Add(a.paint.color)
	}
}

// sameColor compares two colours ignoring alpha, which the palette never uses.
func sameColor(a, b color.RGBA) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B
}
