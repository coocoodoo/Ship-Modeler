package app

import (
	"image"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/paint"
	"modeler/internal/render"
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

	// tex mirrors the sheet on the GPU for the panel's picker.
	tex      rl.Texture2D
	texReady bool
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

// beginTileStamp opens a stamp trail on the hovered face (Tile_paint.md §3).
func (a *App) beginTileStamp() {
	tile := a.armedTile()
	if tile == nil {
		a.Toast(ui.Toast{Text: "Import a tileset and pick a tile first", Kind: ui.ToastWarn})
		return
	}
	h := a.paint.hover
	if !h.ok || h.paint == nil {
		return
	}
	st := &a.paint.tiles
	st.stamping = true
	st.stampBody, st.stampFace = h.body, h.face
	st.stampPt = h.paint
	st.cells = append(st.cells[:0], a.tileCellFor(h.texel, st.free))
	a.applyTileStamp()
}

// trackTileStamp trails the pointer: each new cell it enters takes a stamp,
// and the face the trail started on is the only face it can touch — like a
// stroke, a body passing in front of the cursor mid-drag must not capture it.
func (a *App) trackTileStamp(in InputFrame, vp render.Viewport) {
	st := &a.paint.tiles
	if !in.Down[MouseLeft] {
		a.finishTileStamp()
		return
	}
	if st.stampPt == nil {
		return
	}
	p, ok := a.pointOnFace(in.MouseX, in.MouseY, vp, st.stampPt.Frame)
	if !ok {
		return
	}
	st.free = in.Alt
	cell := a.tileCellFor(paint.Texel(st.stampPt, p), st.free)
	for _, had := range st.cells {
		if had == cell {
			return
		}
	}
	st.cells = append(st.cells, cell)
	a.applyTileStamp()
}

// applyTileStamp pushes the trail so far through the bus as a live drag, so
// the history gets one entry per mouse-down (SPEC-DATA §3.2).
func (a *App) applyTileStamp() {
	st := &a.paint.tiles
	cmd := &paint.StampFace{
		Body: st.stampBody, Face: st.stampFace, Res: a.paint.res,
		Tile: a.armedTile(),
		// The command reruns the trail from its cells each frame, so it needs
		// its own copy.
		Cells: append([]image.Point(nil), st.cells...),
	}
	if a.Bus.Dragging() {
		_ = a.Bus.UpdateDrag(cmd)
		return
	}
	_ = a.Bus.BeginDrag(cmd)
}

// finishTileStamp commits the trail as one step.
func (a *App) finishTileStamp() {
	st := &a.paint.tiles
	if !st.stamping {
		return
	}
	st.stamping = false
	st.cells = st.cells[:0]
	st.stampPt = nil
	if a.Bus.Dragging() {
		a.Bus.CommitDrag()
	}
}

// cancelTileStamp reverts a live trail, which is Escape mid-drag.
func (a *App) cancelTileStamp() {
	st := &a.paint.tiles
	if !st.stamping {
		return
	}
	st.stamping = false
	st.cells = st.cells[:0]
	st.stampPt = nil
	a.Bus.CancelDrag()
}

// refreshTileTexture mirrors the sheet to the GPU for the panel's picker,
// nearest-filtered so the pixels stay pixels.
func (a *App) refreshTileTexture() {
	a.dropTilePickerTexture()
	t := &a.paint.tiles
	if t.set == nil || t.set.Img == nil {
		return
	}
	img := rl.NewImageFromImage(t.set.Img)
	t.tex = rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)
	rl.SetTextureFilter(t.tex, rl.FilterPoint)
	t.texReady = true
}

// dropTilePickerTexture releases the picker's GPU copy of the sheet.
func (a *App) dropTilePickerTexture() {
	t := &a.paint.tiles
	if t.texReady {
		rl.UnloadTexture(t.tex)
		t.texReady = false
	}
}

// restoreTileset quietly re-arms the sheet the last session used. Quiet on
// purpose: a missing file at startup is not the user's doing right now, and
// the panel's empty state says what to do about it.
func (a *App) restoreTileset() {
	ts := a.Settings.Tiles
	if ts.Path == "" {
		return
	}
	set, err := paint.LoadTilesetFile(ts.Path)
	if err != nil {
		return
	}
	if ok, _ := paint.ValidTileGrid(ts.TileW, ts.TileH); ok {
		set.TileW, set.TileH = ts.TileW, ts.TileH
		set.Margin, set.Spacing = ts.Margin, ts.Spacing
	}
	t := &a.paint.tiles
	t.set, t.path = set, ts.Path
	t.orient = paint.Orientation{Rot: ts.Rot % 4, FlipX: ts.FlipX}
	if ts.Selected >= 0 && ts.Selected < set.Count() {
		t.sel = ts.Selected
	}
	a.dropTileCache()
	a.refreshTileTexture()
}

// storeTileSettings writes the tile setup back to the preferences.
func (a *App) storeTileSettings() {
	t := &a.paint.tiles
	if t.set == nil {
		return
	}
	a.Settings.Tiles = io.TileSettings{
		Path: t.path, TileW: t.set.TileW, TileH: t.set.TileH,
		Margin: t.set.Margin, Spacing: t.set.Spacing,
		Selected: t.sel, Rot: t.orient.Rot, FlipX: t.orient.FlipX,
	}
}

// importTilesetWithDialog is the panel's Import button: ask for a PNG, copy
// it into the config dir so the original can move, and arm the copy.
func (a *App) importTilesetWithDialog() {
	path, ok, err := io.AskOpenTileset(a.Settings.LastDir)
	if err != nil || !ok {
		return
	}
	kept, err := io.CopyIntoConfig("tilesets", path)
	if err != nil {
		// The copy failing is not worth losing the import over: arm the
		// original and let the settings remember where it was.
		kept = path
	}
	a.ImportTileset(kept)
}
