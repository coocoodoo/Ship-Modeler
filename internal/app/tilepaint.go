package app

import (
	"image"

	"modeler/internal/geom/mesh"
	"modeler/internal/paint"
	"modeler/internal/ui"
)

// Tile stamping in the app (Tile_paint.md TP2/TP3).
//
// The paint package owns the sheet, the slicing and the stamp; this file owns
// the state the panel and the pointer share — which sheet is loaded, which
// tile is armed, how it is turned — and the one entry point that puts a tile
// onto a face through the command bus.

// tilePaintState is the tile tool's half of paintState.
type tilePaintState struct {
	set    *paint.Tileset
	path   string
	sel    int
	orient paint.Orientation

	// custom is true when the grid chips sit on Custom, showing the four
	// slicing fields. A preset chip is just those fields set for you.
	custom bool

	// oriented caches the armed tile's pixels under the armed orientation, so
	// the ghost and a stamp trail do not rebuild the image every frame.
	oriented *image.RGBA

	// stamping tracks a pointer trail: the cells stamped this mouse-down, and
	// the face they are confined to.
	stamping  bool
	stampBody uint32
	stampFace mesh.FaceUID
	stampPt   *mesh.FacePaint
	cells     []image.Point
	// free is live while Alt suppresses the snap, refreshed every frame the
	// tool updates.
	free bool
}

// TileReady reports whether a sheet is loaded and slicing yields tiles.
func (a *App) TileReady() bool {
	t := &a.paint.tiles
	return t.set != nil && t.set.Count() > 0
}

// armedTile is the selected tile under the armed orientation, rebuilt when
// stale and cached until the selection, grid or orientation changes.
func (a *App) armedTile() *image.RGBA {
	t := &a.paint.tiles
	if !a.TileReady() {
		return nil
	}
	if t.sel >= t.set.Count() {
		t.sel = 0
	}
	if t.oriented == nil {
		t.oriented = t.set.Oriented(t.sel, t.orient)
	}
	return t.oriented
}

// dropTileCache forgets the cached oriented pixels after any change to what
// they are built from.
func (a *App) dropTileCache() { a.paint.tiles.oriented = nil }

// ImportTileset loads a sheet from a path and arms it with the current grid.
// The panel's Import button copies the file into the config dir first and
// hands the copy's path here; scripts hand any path directly.
func (a *App) ImportTileset(path string) bool {
	set, err := paint.LoadTilesetFile(path)
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	t := &a.paint.tiles
	// Keep the grid the user had if it fits the new sheet; otherwise the
	// default. A re-import of the same sheet must not forget its slicing.
	if t.set != nil {
		set.TileW, set.TileH = t.set.TileW, t.set.TileH
		set.Margin, set.Spacing = t.set.Margin, t.set.Spacing
	}
	t.set, t.path, t.sel = set, path, 0
	t.orient = paint.Orientation{}
	a.dropTileCache()
	a.refreshTileTexture()
	if set.Count() == 0 {
		a.Toast(ui.Toast{
			Text: "Sheet loaded, but the grid cuts no tiles — adjust the tile size",
			Kind: ui.ToastWarn,
		})
		return true
	}
	a.Toast(ui.Toast{Text: "Tileset loaded: " + plural(set.Count(), "tile", "tiles")})
	return true
}

// SetTileGrid re-slices the sheet.
func (a *App) SetTileGrid(w, h, margin, spacing int) bool {
	if ok, why := paint.ValidTileGrid(w, h); !ok {
		a.Toast(ui.Toast{Text: capitalize(why), Kind: ui.ToastWarn})
		return false
	}
	t := &a.paint.tiles
	if t.set == nil {
		a.Toast(ui.Toast{Text: "Import a tileset first", Kind: ui.ToastWarn})
		return false
	}
	if margin < 0 {
		margin = 0
	}
	if spacing < 0 {
		spacing = 0
	}
	t.set.TileW, t.set.TileH = w, h
	t.set.Margin, t.set.Spacing = margin, spacing
	if t.sel >= t.set.Count() {
		t.sel = 0
	}
	a.dropTileCache()
	return true
}

// SelectTile arms one tile of the sheet.
func (a *App) SelectTile(i int) bool {
	t := &a.paint.tiles
	if t.set == nil || i < 0 || i >= t.set.Count() {
		a.Toast(ui.Toast{Text: "No such tile in the sheet", Kind: ui.ToastWarn})
		return false
	}
	t.sel = i
	a.dropTileCache()
	return true
}

// SetTileOrientation turns the armed stamp.
func (a *App) SetTileOrientation(o paint.Orientation) {
	a.paint.tiles.orient = paint.Orientation{Rot: o.Rot % 4, FlipX: o.FlipX}
	a.dropTileCache()
}

// tileCellFor is where a stamp at a texel lands: the tile-grid cell, or the
// texel itself while Alt holds the snap off. The grid uses the ORIENTED
// dimensions — what is about to land on the surface — so a quarter-turned
// 16x8 tile snaps on an 8x16 grid.
func (a *App) tileCellFor(texel image.Point, free bool) image.Point {
	tile := a.armedTile()
	if tile == nil || free {
		return texel
	}
	return paint.SnapToTileGrid(texel, tile.Bounds().Dx(), tile.Bounds().Dy())
}

// StampTileAt places the armed tile on a face as one command, which is what
// the script op drives. The pointer path trails through the same command via
// the bus's drag coalescing instead.
func (a *App) StampTileAt(body uint32, face mesh.FaceUID, texel image.Point, free bool) bool {
	tile := a.armedTile()
	if tile == nil {
		a.Toast(ui.Toast{Text: "Import a tileset and pick a tile first", Kind: ui.ToastWarn})
		return false
	}
	return a.Run(&paint.StampFace{
		Body: body, Face: face, Res: a.paint.res,
		Tile:  tile,
		Cells: []image.Point{a.tileCellFor(texel, free)},
	})
}

// refreshTileTexture mirrors the sheet to the GPU for the panel's picker.
// TP3 owns the actual texture; until then this is the seam it hangs off.
func (a *App) refreshTileTexture() { a.dropTilePickerTexture() }

// dropTilePickerTexture releases the picker's GPU copy of the sheet. TP3
// replaces this stub with the real unload.
func (a *App) dropTilePickerTexture() {}
