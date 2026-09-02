package app

import (
	"fmt"
	"image"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/paint"
	"modeler/internal/ui"
)

// The tile section of the paint panel (Tile_paint.md TP3): import, the grid
// chips, the sheet picker and the stamp's orientation. It stands where the
// palette normally does — tiles carry their own colours.

// tilePickerHeight is the box the sheet draws in, logical pixels. The sheet
// scales to fit it entirely; a 1024 px sheet gets small, but every tile stays
// clickable, and that beats a scroll region nobody discovers.
const tilePickerHeight = 150

// buildTileSection lays out and runs the tile controls.
func (a *App) buildTileSection(row func(float32) rl.Rectangle, space func(float64), line float32) {
	t := &a.paint.tiles

	hdr := row(line)
	a.UI.Text(hdr, "Tileset", ui.FontSizeSmall, ui.ColorTextDim)
	if t.set != nil {
		count, _ := ui.SplitRight(hdr, a.px(90))
		a.UI.Text(count, plural(t.set.Count(), "tile", "tiles"),
			ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.9))
	}

	if a.UI.Button(ui.MakeID("tile.import"), row(a.px(26)), "Import tileset PNG", ui.ButtonOpts{
		Tooltip: "Load a sheet; the grid below cuts it into tiles",
	}) {
		a.RequestFile(fileImportTileset)
	}
	space(4)

	// The grid chips: 8/16/32/64 or Custom (the user's spelling of "adjust
	// the tile pattern"). A preset is square with no margin or gutter; Custom
	// opens the four fields for sheets that have them.
	a.UI.Text(row(line), "Tile grid", ui.FontSizeSmall, ui.ColorTextDim)
	labels := make([]string, 0, len(paint.TileGridPresets)+1)
	sel := -1
	for i, p := range paint.TileGridPresets {
		labels = append(labels, itoa(p))
		if t.set != nil && !t.custom &&
			t.set.TileW == p && t.set.TileH == p && t.set.Margin == 0 && t.set.Spacing == 0 {
			sel = i
		}
	}
	labels = append(labels, "Custom")
	if t.custom {
		sel = len(labels) - 1
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("tile.grid"), row(a.px(24)),
		labels, sel, ui.ChipGroupOpts{Tooltip: "How the sheet slices into tiles"}); changed {
		if pick == len(labels)-1 {
			t.custom = true
		} else {
			t.custom = false
			a.SetTileGrid(paint.TileGridPresets[pick], paint.TileGridPresets[pick], 0, 0)
		}
	}
	space(4)

	if t.custom {
		r := row(a.px(24))
		fieldW := (r.Width - a.px(12)) / 4
		field := func(i int, label string, v int, min, max float64) int {
			f := ui.Rect(r.X+float32(i)*(fieldW+a.px(4)), r.Y, fieldW, r.Height)
			out, res := a.UI.DragNumber(ui.MakeID("tile.f"+label), f, float64(v), ui.NumberOpts{
				Step: 1, Decimals: 0, Min: min, Max: max,
				Tooltip: label,
			})
			if res.Changed {
				return int(out)
			}
			return v
		}
		if t.set != nil {
			w := field(0, "Tile width", t.set.TileW, paint.MinTileSize, paint.MaxTileSize)
			h := field(1, "Tile height", t.set.TileH, paint.MinTileSize, paint.MaxTileSize)
			m := field(2, "Margin", t.set.Margin, 0, 64)
			sp := field(3, "Spacing", t.set.Spacing, 0, 64)
			if w != t.set.TileW || h != t.set.TileH || m != t.set.Margin || sp != t.set.Spacing {
				a.SetTileGrid(w, h, m, sp)
			}
		}
		space(4)
	}

	if t.set == nil {
		a.UI.Text(row(line), "No sheet yet - import one to start",
			ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.7))
		return
	}

	a.buildTilePicker(row(a.px(tilePickerHeight)))
	space(4)

	// Orientation: rotate a quarter turn, mirror, and the readout.
	r := row(a.px(26))
	rot, r := ui.SplitLeft(r, a.px(32))
	flip, r := ui.SplitLeft(r, a.px(32)+a.px(4))
	flip.X += a.px(4)
	flip.Width -= a.px(4)
	if a.UI.IconButton(ui.MakeID("tile.rot"), rot, ui.DrawRotateCWIcon, ui.IconOpts{
		Tooltip: "Turn the stamp a quarter clockwise",
	}) {
		a.SetTileOrientation(t.orient.RotatedCW())
	}
	if a.UI.IconButton(ui.MakeID("tile.flip"), flip, ui.DrawFlipIcon, ui.IconOpts{
		Tooltip: "Mirror the stamp",
	}) {
		a.SetTileOrientation(t.orient.Flipped())
	}
	label := fmt.Sprintf("Tile %d", t.sel)
	if t.orient.Rot != 0 {
		label += fmt.Sprintf(" · %d deg", int(t.orient.Rot)*90)
	}
	if t.orient.FlipX {
		label += " · mirrored"
	}
	r.X += a.px(8)
	r.Width -= a.px(8)
	a.UI.Text(r, label, ui.FontSizeSmall, ui.ColorTextDim)
}

// buildTilePicker draws the sheet, nearest-filtered, scaled to fit its box,
// with the armed tile outlined; clicking a tile arms it.
func (a *App) buildTilePicker(r rl.Rectangle) {
	t := &a.paint.tiles
	if !t.texReady || t.set == nil {
		return
	}
	sw := float32(t.set.Img.Bounds().Dx())
	sh := float32(t.set.Img.Bounds().Dy())
	scale := r.Width / sw
	if s := r.Height / sh; s < scale {
		scale = s
	}
	// Integer zoom when the sheet fits with room: crisper pixels; fractional
	// only when the sheet is bigger than the box.
	if scale > 1 {
		scale = float32(int(scale))
	}
	dw, dh := sw*scale, sh*scale
	ox := r.X + (r.Width-dw)/2
	oy := r.Y + (r.Height-dh)/2

	// The well behind the sheet, so its transparent regions read as empty
	// rather than as panel.
	a.UI.FillRounded(r, 4, ui.Fade(ui.ColorBG, 0.65))
	rl.DrawTexturePro(t.tex,
		rl.Rectangle{X: 0, Y: 0, Width: sw, Height: sh},
		rl.Rectangle{X: ox, Y: oy, Width: dw, Height: dh},
		rl.Vector2{}, 0, rl.White)

	// The grid's tile outlines, faint, so the slicing is visible on the sheet
	// itself — this is "adjust the tile pattern" showing its work.
	faint := ui.WithAlpha(ui.ColorText, 0x2E)
	for i := 0; i < t.set.Count(); i++ {
		tr := t.set.TileRect(i)
		box := ui.Rect(ox+float32(tr.Min.X)*scale, oy+float32(tr.Min.Y)*scale,
			float32(tr.Dx())*scale, float32(tr.Dy())*scale)
		a.UI.StrokeRounded(box, 0, faint)
	}
	// The armed tile in the accent, over the faint grid.
	if sel := t.set.TileRect(t.sel); !sel.Empty() {
		box := ui.Rect(ox+float32(sel.Min.X)*scale, oy+float32(sel.Min.Y)*scale,
			float32(sel.Dx())*scale, float32(sel.Dy())*scale)
		a.UI.StrokeRounded(box, 0, ui.ColorAccent)
		box.X--
		box.Y--
		box.Width += 2
		box.Height += 2
		a.UI.StrokeRounded(box, 0, ui.WithAlpha(ui.ColorAccent, 0x90))
	}

	// Click to arm. The UI kit's buttons own their own hit tests, but the
	// picker is one surface with many targets, so it reads the pointer
	// directly, exactly as the palette grid does.
	mx, my := float32(a.lastMouseX), float32(a.lastMouseY)
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) &&
		mx >= ox && mx < ox+dw && my >= oy && my < oy+dh {
		px := int((mx - ox) / scale)
		py := int((my - oy) / scale)
		for i := 0; i < t.set.Count(); i++ {
			if (image.Point{X: px, Y: py}).In(t.set.TileRect(i)) {
				a.SelectTile(i)
				break
			}
		}
	}
}
