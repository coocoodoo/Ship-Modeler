package paint

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
)

// Tilesets (Tile_paint.md TP1, the user's request 2026-08-28).
//
// A tileset is a sheet of pixels and a grid that separates it into tiles. The
// grid follows Tiled's convention, because that is what the sheets people
// already have expect: Margin is the border around the whole sheet, Spacing
// the gutter between tiles, and a partial column at the sheet's edge is
// dropped rather than stretched. This file is raylib-free — the sheet decodes
// with the standard library, and only the app layer ever uploads it to the
// GPU for the panel's picker.

// Sheet and tile caps (Tile_paint.md §3). Over-cap imports are refused with
// the numbers in the message, not clamped: a silently cropped sheet is a
// mystery, a named limit is a fact.
const (
	MaxSheetSize = 1024
	MaxTileSize  = 64
	MinTileSize  = 2
)

// TileGridPresets are the chip row: square tiles, no margin, no gutter.
// Custom opens the four fields for everything else.
var TileGridPresets = []int{8, 16, 32, 64}

// DefaultTileSize is the grid a fresh import starts on.
const DefaultTileSize = 16

// Tileset is a sheet plus its slicing grid.
type Tileset struct {
	Img             *image.RGBA
	TileW, TileH    int
	Margin, Spacing int
}

// Cols is how many whole tiles fit across the sheet.
func (t *Tileset) Cols() int { return t.fit(t.sheetW(), t.TileW) }

// Rows is how many whole tiles fit down it.
func (t *Tileset) Rows() int { return t.fit(t.sheetH(), t.TileH) }

// Count is the number of tiles the grid yields.
func (t *Tileset) Count() int { return t.Cols() * t.Rows() }

func (t *Tileset) sheetW() int {
	if t.Img == nil {
		return 0
	}
	return t.Img.Bounds().Dx()
}

func (t *Tileset) sheetH() int {
	if t.Img == nil {
		return 0
	}
	return t.Img.Bounds().Dy()
}

// fit counts whole tiles along one axis: tiles start at Margin and step by
// tile+Spacing, and the last one must end inside the far margin.
func (t *Tileset) fit(sheet, tile int) int {
	if tile <= 0 || sheet <= 0 {
		return 0
	}
	span := sheet - 2*t.Margin + t.Spacing
	if span < tile+t.Spacing {
		return 0
	}
	return span / (tile + t.Spacing)
}

// TileRect is tile i's rectangle in sheet coordinates, row-major from the
// top-left, or the zero rectangle for an index outside the grid.
func (t *Tileset) TileRect(i int) image.Rectangle {
	cols := t.Cols()
	if cols == 0 || i < 0 || i >= t.Count() {
		return image.Rectangle{}
	}
	cx, cy := i%cols, i/cols
	x := t.Margin + cx*(t.TileW+t.Spacing)
	y := t.Margin + cy*(t.TileH+t.Spacing)
	return image.Rect(x, y, x+t.TileW, y+t.TileH)
}

// Orientation is a stamp's member of the dihedral group: flip horizontally
// first, then rotate clockwise by Rot quarter turns. Eight members, one
// documented order — composing them ad hoc is how the fourth click ends up
// mirrored.
type Orientation struct {
	Rot   uint8 // 0..3 quarter turns clockwise
	FlipX bool
}

// RotatedCW is the orientation after one more clockwise quarter turn.
func (o Orientation) RotatedCW() Orientation {
	o.Rot = (o.Rot + 1) % 4
	return o
}

// Flipped is the orientation after mirroring what is currently shown. A
// mirror of a rotated tile is not "toggle the flag": in flip-then-rotate
// order, mirroring the visible result flips the flag AND reverses the
// rotation.
func (o Orientation) Flipped() Orientation {
	o.FlipX = !o.FlipX
	o.Rot = (4 - o.Rot) % 4
	return o
}

// Oriented returns tile i's pixels under an orientation, as a fresh image
// with bounds at the origin. Rot 1 and 3 swap the tile's dimensions.
func (t *Tileset) Oriented(i int, o Orientation) *image.RGBA {
	r := t.TileRect(i)
	if r.Empty() {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	w, h := r.Dx(), r.Dy()
	ow, oh := w, h
	if o.Rot%2 == 1 {
		ow, oh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := x
			if o.FlipX {
				sx = w - 1 - x
			}
			c := t.Img.RGBAAt(r.Min.X+sx, r.Min.Y+y)
			// Where (x, y) of the flipped tile lands after Rot clockwise
			// quarter turns.
			var dx, dy int
			switch o.Rot {
			case 0:
				dx, dy = x, y
			case 1:
				dx, dy = h-1-y, x
			case 2:
				dx, dy = w-1-x, h-1-y
			default:
				dx, dy = y, w-1-x
			}
			out.SetRGBA(dx, dy, c)
		}
	}
	return out
}

// SnapToTileGrid floors a texel to its tile cell's min corner. Floor, never
// truncate: the margin ring lives at texel -1, and -1/8 rounding toward zero
// would snap it to cell 0 instead of cell -1.
func SnapToTileGrid(t image.Point, tw, th int) image.Point {
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	return image.Point{X: floorDiv(t.X, tw) * tw, Y: floorDiv(t.Y, th) * th}
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// LoadTileset decodes a sheet from a reader and checks it against the caps.
// The grid starts at the default preset; the caller re-slices as the user
// adjusts.
func LoadTileset(r io.Reader) (*Tileset, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("read the tileset: %w", err)
	}
	b := img.Bounds()
	if b.Dx() > MaxSheetSize || b.Dy() > MaxSheetSize {
		return nil, fmt.Errorf("the sheet is %dx%d px — the limit is %dx%d",
			b.Dx(), b.Dy(), MaxSheetSize, MaxSheetSize)
	}
	if b.Dx() < MinTileSize || b.Dy() < MinTileSize {
		return nil, fmt.Errorf("the sheet is %dx%d px — too small to hold a tile", b.Dx(), b.Dy())
	}
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x-b.Min.X, y-b.Min.Y, img.At(x, y))
		}
	}
	return &Tileset{Img: rgba, TileW: DefaultTileSize, TileH: DefaultTileSize}, nil
}

// LoadTilesetFile is LoadTileset over a path.
func LoadTilesetFile(path string) (*Tileset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open the tileset: %w", err)
	}
	defer f.Close()
	return LoadTileset(f)
}

// ValidTileGrid reports whether a grid is inside the caps, with the reason
// when it is not.
func ValidTileGrid(w, h int) (bool, string) {
	if w < MinTileSize || h < MinTileSize {
		return false, fmt.Sprintf("tiles must be at least %d px", MinTileSize)
	}
	if w > MaxTileSize || h > MaxTileSize {
		return false, fmt.Sprintf("tiles top out at %d px", MaxTileSize)
	}
	return true, ""
}
