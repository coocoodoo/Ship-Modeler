package app

import (
	"fmt"
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/paint"
	"modeler/internal/ui"
)

// The palette panel of SPEC-UX §13.1. It is a sidebar down the right of the
// window rather than a card floating over the model (V-151): it is the one
// panel you work out of continuously rather than dismiss, and a panel you
// keep open all session has no business sitting on top of the thing you are
// painting. The layout gives it real space, so the viewport ends where it
// begins.
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

// The brush alpha's range (V-158). The floor is 1 and not 0: alpha zero is not
// faint paint but no paint, which is the eraser's job.
const (
	MinPaintAlpha = 1
	MaxPaintAlpha = 255
)

// buildPaintBar draws the sidebar and runs the palette inside it.
//
// While the bar is still moving its contents are laid out at the width they
// will finally have and clipped to however much has arrived, so the panel
// slides in as one piece instead of reflowing at every width on the way.
// Sliding shut it draws empty: the controls belong to paint mode, and the
// mode is already gone by then.
func (a *App) buildPaintBar(r rl.Rectangle) {
	if r.Width <= 0 {
		return
	}
	a.UI.Panel(r)
	a.UI.HairlineV(r.X, r.Y, r.Height, ui.ColorStroke)
	if !a.InPaint() {
		return
	}
	full := ui.Rect(r.X+r.Width-a.px(paintPanelWidth), r.Y, a.px(paintPanelWidth), r.Height)
	clipped := r.Width < full.Width-0.5
	if clipped {
		rl.BeginScissorMode(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height))
	}
	a.buildPaintPanel(full)
	if clipped {
		rl.EndScissorMode()
	}
}

// buildPaintPanel lays out and runs the whole palette panel inside the bar.
func (a *App) buildPaintPanel(bar rl.Rectangle) {
	st := &a.paint
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	swatch := a.px(paintSwatchSize)
	gap := a.px(paintSwatchGap)

	// The prompt and the oblique chip come and go with what is under the
	// pointer; the sidebar is full height either way, so they simply take
	// their row when they apply.
	mismatch, mismatchRes := a.paintResMismatch()
	oblique := a.paintOblique()
	// Both prompts describe "the face you are pointing at", and the edge tool
	// does not point at faces: under it the sticky face is whatever the last
	// brush tool touched, and a resample offer about that face would be an
	// offer about something invisible.
	if a.paint.tool == paint.ToolEdge {
		mismatch, oblique = false, false
	}
	// The dither modes and the fill toggle only mean anything to some tools, so
	// they only appear for those tools. A panel that showed every control every
	// tool might ever want would be a panel mostly full of greyed-out rows.
	showDither := st.tool == paint.ToolBrush || st.tool == paint.ToolGradient
	showFill := st.tool.Shape()
	// The edge tool replaces the brush's own controls with its own: size and
	// resolution mean nothing to it, and a panel offering them would be a
	// panel mostly full of things that do not apply.
	showEdges := st.tool == paint.ToolEdge
	// The tile tool swaps the palette for the sheet: tiles carry their own
	// colours, and a swatch grid under a stamp would be a grid of things that
	// do nothing (Tile_paint.md TP3).
	showTiles := st.tool == paint.ToolTile
	// The wand replaces the brush controls with its tolerance; the standing
	// selection row shows under every tool, because the selection constrains
	// every tool (V-145).
	showWand := st.tool == paint.ToolWand
	showClipboard := st.tool == paint.ToolSelect || st.tool == paint.ToolPaste
	haveWandSel := st.wandMask != nil
	// The brush's square is meaningless to the edge tool, which has its own
	// width, and to the tile tool, whose size is the tile's. A control that
	// does nothing is worse than an absent one.
	showSize := !showEdges && !showTiles && !showWand && !showClipboard

	inner := ui.InsetXY(bar, a.px(ui.Spacing+2), a.px(6))

	// The title wears the mode's colour with a stripe down the bar's edge, the
	// same thing every tool card does.
	titleBox, rest := ui.SplitTop(inner, a.UI.Fonts.LineHeight(ui.FontSizeHeader)+a.px(4))
	ui.FillRect(ui.Rect(bar.X, titleBox.Y, a.px(3), titleBox.Height), ui.ColorAccent)
	a.UI.Text(titleBox, "Paint", ui.FontSizeHeader, ui.ColorAccent)
	a.buildPixelClipboardActions(titleBox)
	a.UI.HairlineH(bar.X, titleBox.Y+titleBox.Height+a.px(4), bar.Width, ui.Fade(ui.ColorAccent, 0.35))
	rest.Y += a.px(10)
	rest.Height -= a.px(10)

	// The way out is pinned to the bottom, taken off before anything else, so
	// it is in the same place whatever the tool above it has grown into — and
	// so a panel of controls taller than the window can never bury it.
	footer, body := ui.SplitBottom(rest, a.px(32))
	footer.Y += a.px(6)
	footer.Height -= a.px(6)
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

	if showSize {
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
	}

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

	if showEdges {
		a.buildEdgeSection(row, space, line)
		space(6)
	}

	if showWand {
		a.buildWandSection(row, line)
		space(6)
	}
	if haveWandSel {
		a.buildWandSelectionRow(row(a.px(26)))
		space(6)
	}

	a.paintResRow(row(line), row(a.px(24)))
	if mismatch {
		space(6)
		a.paintMismatchPrompt(row(line*2), row(a.px(26)), mismatchRes)
	}
	space(8)

	if showTiles {
		a.buildTileSection(row, space, line)
		space(6)
	}
	if showClipboard {
		a.buildPixelClipboardSection(row, line)
		space(6)
	}

	if len(st.custom) > 0 && !showTiles && !showClipboard {
		// The second page carries the palette's own name once one has been
		// chosen from the library — "Imported" describes where it came from,
		// which is the one thing you already know.
		second := "Imported"
		if st.browser.applied != "" {
			second = st.browser.applied
		}
		chips := row(a.px(24))
		second = a.UI.Truncate(second, ui.FontSizeSmall, chips.Width/2-a.px(12))
		if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.page"), chips,
			[]string{"Built-in", second}, st.page, ui.ChipGroupOpts{
				Tooltip: "Which page of colours the grid shows",
			}); changed {
			st.page = pick
		}
		space(4)
	}

	if !showTiles && !showClipboard {
		a.UI.Text(row(line), "Palette", ui.FontSizeSmall, ui.ColorTextDim)
		a.paintPaletteGrid(row(float32(paintPaletteRows)*(swatch+gap)), swatch, gap)
		space(6)

		a.buildAlphaSection(row, line)
		space(6)

		a.UI.Text(row(line), "Recents", ui.FontSizeSmall, ui.ColorTextDim)
		a.paintRecentsStrip(row(swatch), swatch, gap)
		space(8)

		a.paintColorRow(row(a.px(26)))
		space(6)
	}
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

	// The way out, in the footer taken off the bottom above. The shortcut is
	// P rather than Esc: P always leaves the mode, while Esc backs out one
	// level at a time and only reaches the mode once nothing else is pending.
	if a.UI.Button(ui.MakeID("paint.stop"), footer, "Stop painting", ui.ButtonOpts{
		Tooltip:  "Leave paint mode and close this panel",
		Shortcut: "P",
	}) {
		a.ExitPaint()
	}
}

// paintLockRow is the face lock (SPEC-UX §13.5): one button that becomes the
// thing you press to get out of it, plus a line naming what you are locked to.
func (a *App) paintLockRow(row func(float32) rl.Rectangle, line float32) {
	st := &a.paint
	if !st.locked {
		// Always live. The button arms the pick; the face is chosen by the click
		// that follows. A button that needed a face already under the pointer
		// could not be reached, because reaching for it is what takes the
		// pointer off the face.
		label := "Lock to a face…"
		opts := ui.ButtonOpts{
			Tooltip: "Then click a face: the camera turns to it and nothing else takes paint",
		}
		if st.awaitingLock {
			label = "Click a face…"
			opts.Style = ui.ButtonPrimary
			opts.Tooltip = "Click the face to lock to, or press again to cancel"
		}
		if a.UI.Button(ui.MakeID("paint.lock"), row(a.px(26)), label, opts) {
			if !a.CancelLockPick() {
				a.BeginLockPick()
			}
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
		{paint.ToolWand, ui.DrawWandIcon,
			"Select similar colours around a click — the other tools then paint only inside"},
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
		{paint.ToolEdge, ui.DrawEdgeLineIcon,
			"Click edges, then bake a line along them onto both faces"},
		{paint.ToolTile, ui.DrawTileIcon,
			"Stamp tiles from an imported sheet, snapped to a tile grid"},
	}
}

// paintToolRow draws one row of tools, each with its shortcut on the tooltip.
//
// Both rows are laid out on the same five-column grid so the icons line up
// under each other even though one row is a tool shorter.
func (a *App) paintToolRow(r rl.Rectangle, tools []paintTool) {
	const columns = 6
	gap := a.px(4)
	w := (r.Width - gap*float32(columns-1)) / float32(columns)
	for i, t := range tools {
		box := ui.Rect(r.X+float32(i)*(w+gap), r.Y, w, r.Height)
		if a.UI.IconButton(ui.MakeID("paint.tool."+t.tool.String()), box, t.icon, ui.IconOpts{
			Active:   a.paint.tool == t.tool,
			Tooltip:  t.tip,
			Shortcut: t.tool.Shortcut(),
			Accent:   paintToolAccent(t.tool),
		}) {
			a.setPaintTool(t.tool)
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
		if h.allocated && !sameDensity(h.paint.Texel, st.res) {
			text = fmt.Sprintf("this face is %.3g px/u", paint.Density(h.paint))
			col = ui.ColorWarn
		}
		a.UI.Text(note, text, ui.FontSizeSmall, ui.Fade(col, 0.9))
	}

	// The highlighted chip is the model's actual pixel size once anything is
	// painted — the truth, not the last thing clicked (V-140).
	shown := st.res
	if r, ok := a.documentPaintRes(); ok {
		shown = r
	}
	labels := make([]string, len(paint.Resolutions))
	sel := -1
	for i, res := range paint.Resolutions {
		labels[i] = itoa(res)
		if res == shown {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.res"), chips, labels, sel,
		ui.ChipGroupOpts{
			Tooltip: "The model's pixel size — changing it resamples every painted face, one undoable step",
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
	if !ok || h.paint == nil {
		return false, 0
	}
	if !h.allocated {
		// A bare face cannot mismatch any more: new paint always lands at the
		// body's own density (V-140), so there is nothing to warn about. The
		// prompt survives only for painted faces of legacy mixed documents.
		return false, 0
	}
	if sameDensity(h.paint.Texel, a.allocResFor(h.body)) {
		return false, 0
	}
	return true, nearestRes(paint.Density(h.paint))
}

// bodyPaintedRes is the density the body's painted faces use, as a chip, or
// false when nothing on the body is painted. Mixed densities answer with the
// first painted face's — documents rarely mix, and one honest offer beats a
// survey.
func (a *App) bodyPaintedRes(bodyID uint32) (int, bool) {
	b := a.Doc().BodyByID(bodyID)
	if b == nil || b.Mesh == nil {
		return 0, false
	}
	for i := range b.Mesh.Faces {
		if p := b.Mesh.Faces[i].Paint; p != nil && p.Texel > 0 {
			return nearestRes(paint.Density(p)), true
		}
	}
	return 0, false
}

// nearestRes is the chip closest to a density, which for anything painted since
// V-128 is that density exactly. A ship saved before it can hold a face at a
// density no chip produces, and the offer to match it has to name a chip the
// brush can actually be set to.
func nearestRes(d float64) int {
	best, gap := paint.Resolutions[0], math.Inf(1)
	for _, r := range paint.Resolutions {
		if g := math.Abs(float64(r) - d); g < gap {
			best, gap = r, g
		}
	}
	return best
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
	h, _ := a.stickyFace()
	shown := float64(faceRes)
	if h.paint != nil {
		shown = paint.Density(h.paint)
	}
	a.UI.Text(first, fmt.Sprintf("This face is %.3g px/u — switch the", shown),
		ui.FontSizeSmall, ui.ColorWarn)
	a.UI.Text(second, fmt.Sprintf("brush to %d, or resample it to %d?", faceRes, a.paint.res),
		ui.FontSizeSmall, ui.ColorWarn)

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

	// The armed slots, unlike the palette grid, show the brush's alpha: these
	// two are what the next stroke actually lays down, and a chip that says
	// solid while the slider says glaze is a chip that lies (V-158).
	tipAlpha := ""
	if st.alpha < 255 {
		tipAlpha = fmt.Sprintf(" at %d%%", alphaPercent(st.alpha))
	}
	slot := func(i int, c color.RGBA, tip string, x float32) {
		box := ui.Rect(x, r.Y+(r.Height-sw)/2, sw, sw)
		if a.UI.ColorSwatch(ui.MakeID("paint.slot"+itoa(i)), box, c, ui.SwatchOpts{
			Tooltip:  tip + ": #" + paint.Hex(c) + tipAlpha,
			Selected: st.slot == i,
			Alpha:    st.alpha,
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

	// The library is the way in now: the file dialog is still there, inside
	// it, for a palette that is not in the collection — but a list you can
	// search beats a dialog you have to already know the answer to.
	if a.UI.Button(ui.MakeID("paint.library"), importBox, "Palettes…", ui.ButtonOpts{
		Tooltip: "Browse the palette library, or import a .hex file",
	}) {
		a.OpenPaletteBrowser()
	}

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

// buildAlphaSection is the brush's opacity, between the palette and the
// recents because it is the other half of choosing a colour: the grid says
// which colour, this says how much of it (SPEC-UX §13.1, V-158).
//
// The value reads as a percentage rather than as 0..255. "How much of this
// colour lands" is the question being asked, and 0..255 is only how the answer
// is stored.
//
// The slider stops at 1 rather than 0. Alpha zero is not faint paint, it is no
// paint at all — that is what the eraser is for, and a brush that silently
// became an eraser at the bottom of its own range would be a trap.
func (a *App) buildAlphaSection(row func(float32) rl.Rectangle, line float32) {
	st := &a.paint
	head := row(line)
	note, label := ui.SplitRight(head, a.px(104))
	a.UI.Text(label, "Alpha", ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.Text(note, describeAlpha(st.alpha), ui.FontSizeSmall,
		ui.Fade(ui.ColorTextDim, 0.9))

	r := row(a.px(24))
	// Wider than the wand's readout and with a gap before it: "100%" is four
	// glyphs, and at the top of the range the knob sits at the very end of the
	// track, where it would otherwise sit on top of them.
	valueBox, sliderBox := ui.SplitRight(r, a.px(46))
	sliderBox.Width -= a.px(8)
	if v, changed := a.UI.Slider(ui.MakeID("paint.alpha"), sliderBox,
		float64(st.alpha), MinPaintAlpha, MaxPaintAlpha, ui.ButtonOpts{
			Tooltip: "How much of the colour a stroke lays down — full is solid, less lets what is under it show through",
		}); changed {
		st.alpha = uint8(clampInt(int(v+0.5), MinPaintAlpha, MaxPaintAlpha))
	}
	a.UI.Text(valueBox, fmt.Sprintf("%d%%", alphaPercent(st.alpha)),
		ui.FontSizeUI, ui.ColorText)
}

// alphaPercent is an alpha as the percentage the readout shows. It rounds to
// nearest so the top of the slider says 100 and the bottom does not say 0 —
// a stroke that lands is never nothing.
func alphaPercent(v uint8) int {
	p := int(float64(v)/255*100 + 0.5)
	if p < 1 {
		p = 1
	}
	return p
}

// describeAlpha names what an alpha means, so the slider is a sentence rather
// than a bare number — the same courtesy the wand's tolerance gets.
func describeAlpha(v uint8) string {
	switch {
	case v >= 255:
		return "solid"
	case v >= 208:
		return "nearly solid"
	case v >= 144:
		return "translucent"
	case v >= 72:
		return "a glaze"
	default:
		return "a faint tint"
	}
}
