package paint

import (
	"image"
	"image/color"
	"testing"
)

// Tileset slicing and stamp orientation (Tile_paint.md TP1).
//
// The slicing maths follows Tiled's convention — margin is the border around
// the whole sheet, spacing the gutter between tiles, partial columns dropped —
// and every case here is hand-computed, because margin/spacing off-by-ones
// are the classic way a picker shows tiles one pixel out of register.

// sheet builds a w x h RGBA filled with a colour.
func sheet(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestTilesetSlicingIsTiledConvention(t *testing.T) {
	cases := []struct {
		name                   string
		sheetW, sheetH         int
		tw, th, margin, space  int
		wantCols, wantRows     int
		wantFirst, wantSecond  image.Rectangle
		secondIsNextRowInstead bool
	}{
		{
			name: "clean 2x2 of 8px", sheetW: 16, sheetH: 16, tw: 8, th: 8,
			wantCols: 2, wantRows: 2,
			wantFirst:  image.Rect(0, 0, 8, 8),
			wantSecond: image.Rect(8, 0, 16, 8),
		},
		{
			name: "margin and spacing", sheetW: 19, sheetH: 19, tw: 8, th: 8, margin: 1, space: 2,
			// (19 - 2*1 + 2) / (8 + 2) = 19/10 = 1... hand-check: tiles at
			// x=1 and x=1+8+2=11, ending at 19-margin=18 -> 11+8=19 > 18? No:
			// 11+8=19, sheet minus margin is 18, so the second column does NOT
			// fit. cols = 1.
			wantCols: 1, wantRows: 1,
			wantFirst: image.Rect(1, 1, 9, 9),
		},
		{
			name: "margin 1 spacing 1 fits 2", sheetW: 19, sheetH: 10, tw: 8, th: 8, margin: 1, space: 1,
			// tiles at x=1 and x=10; 10+8=18 <= 19-1 ok. rows: y=1, 1+8=9 <= 9 ok.
			wantCols: 2, wantRows: 1,
			wantFirst:  image.Rect(1, 1, 9, 9),
			wantSecond: image.Rect(10, 1, 18, 9),
		},
		{
			name: "partial column dropped", sheetW: 20, sheetH: 8, tw: 8, th: 8,
			wantCols: 2, wantRows: 1,
			wantFirst:  image.Rect(0, 0, 8, 8),
			wantSecond: image.Rect(8, 0, 16, 8),
		},
		{
			name: "non-square tiles", sheetW: 32, sheetH: 8, tw: 16, th: 8,
			wantCols: 2, wantRows: 1,
			wantFirst:  image.Rect(0, 0, 16, 8),
			wantSecond: image.Rect(16, 0, 32, 8),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := &Tileset{
				Img:   sheet(tc.sheetW, tc.sheetH, color.RGBA{R: 9, A: 255}),
				TileW: tc.tw, TileH: tc.th, Margin: tc.margin, Spacing: tc.space,
			}
			if got := ts.Cols(); got != tc.wantCols {
				t.Errorf("cols = %d, want %d", got, tc.wantCols)
			}
			if got := ts.Rows(); got != tc.wantRows {
				t.Errorf("rows = %d, want %d", got, tc.wantRows)
			}
			if ts.Count() > 0 {
				if got := ts.TileRect(0); got != tc.wantFirst {
					t.Errorf("tile 0 = %v, want %v", got, tc.wantFirst)
				}
			}
			if ts.Count() > 1 && tc.wantSecond != (image.Rectangle{}) {
				if got := ts.TileRect(1); got != tc.wantSecond {
					t.Errorf("tile 1 = %v, want %v", got, tc.wantSecond)
				}
			}
		})
	}
}

func TestTilesetDegenerateSlicing(t *testing.T) {
	ts := &Tileset{Img: sheet(8, 8, color.RGBA{A: 255}), TileW: 16, TileH: 16}
	if ts.Count() != 0 {
		t.Errorf("a tile bigger than its sheet sliced into %d tiles, want 0", ts.Count())
	}
	ts = &Tileset{Img: sheet(8, 8, color.RGBA{A: 255}), TileW: 0, TileH: 8}
	if ts.Count() != 0 {
		t.Errorf("a zero-width tile sliced into %d tiles, want 0", ts.Count())
	}
	if ts.TileRect(5) != (image.Rectangle{}) {
		t.Error("an out-of-range tile index returned a rectangle")
	}
}

// mark builds the asymmetric 2x2 probe tile:
//
//	R G
//	B .
//
// where . is transparent. Every one of the eight orientations of this tile is
// distinct, which is what makes it a complete witness for the D4 group.
func markTile() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	img.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	return img
}

func TestOrientationCoversTheD4Group(t *testing.T) {
	ts := &Tileset{Img: markTile(), TileW: 2, TileH: 2}
	if ts.Count() != 1 {
		t.Fatalf("probe sheet sliced into %d tiles, want 1", ts.Count())
	}
	// at reads the oriented tile as a compact string, row-major, one letter
	// per pixel: R, G, B or . for transparent.
	at := func(o Orientation) string {
		img := ts.Oriented(0, o)
		s := ""
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				switch c := img.RGBAAt(x, y); {
				case c.A < 128:
					s += "."
				case c.R > 128:
					s += "R"
				case c.G > 128:
					s += "G"
				default:
					s += "B"
				}
			}
		}
		return s
	}
	// Flip first, then rotate clockwise — the documented order. Each string
	// is derived by hand: the base tile RG/B., its mirror GR/.B, and their
	// quarter turns.
	want := map[Orientation]string{
		{Rot: 0, FlipX: false}: "RGB.",
		{Rot: 1, FlipX: false}: "BR.G",
		{Rot: 2, FlipX: false}: ".BGR",
		{Rot: 3, FlipX: false}: "G.RB",
		{Rot: 0, FlipX: true}:  "GR.B",
		{Rot: 1, FlipX: true}:  ".GBR",
		{Rot: 2, FlipX: true}:  "B.RG",
		{Rot: 3, FlipX: true}:  "RBG.",
	}
	seen := map[string]Orientation{}
	for o, w := range want {
		got := at(o)
		if got != w {
			t.Errorf("orientation %+v = %q, want %q", o, got, w)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("orientations %+v and %+v produce the same tile %q", prev, o, got)
		}
		seen[got] = o
	}
	if len(seen) != 8 {
		t.Errorf("the eight orientations produced %d distinct tiles", len(seen))
	}
}

func TestOrientationSwapsDimensionsOfNonSquareTiles(t *testing.T) {
	ts := &Tileset{Img: sheet(16, 8, color.RGBA{R: 1, A: 255}), TileW: 16, TileH: 8}
	up := ts.Oriented(0, Orientation{Rot: 0})
	turned := ts.Oriented(0, Orientation{Rot: 1})
	if up.Bounds().Dx() != 16 || up.Bounds().Dy() != 8 {
		t.Fatalf("unrotated tile is %v", up.Bounds())
	}
	if turned.Bounds().Dx() != 8 || turned.Bounds().Dy() != 16 {
		t.Errorf("a quarter-turned 16x8 tile is %v, want 8x16", turned.Bounds())
	}
}

func TestSnapToTileGridFloorsNegatives(t *testing.T) {
	cases := []struct {
		in   image.Point
		w, h int
		want image.Point
	}{
		{image.Point{X: 0, Y: 0}, 8, 8, image.Point{X: 0, Y: 0}},
		{image.Point{X: 7, Y: 7}, 8, 8, image.Point{X: 0, Y: 0}},
		{image.Point{X: 8, Y: 15}, 8, 8, image.Point{X: 8, Y: 8}},
		// The margin ring lives at texel -1: floor, never truncate toward zero.
		{image.Point{X: -1, Y: -1}, 8, 8, image.Point{X: -8, Y: -8}},
		{image.Point{X: -8, Y: -9}, 8, 8, image.Point{X: -8, Y: -16}},
		{image.Point{X: 5, Y: 5}, 16, 8, image.Point{X: 0, Y: 0}},
	}
	for _, tc := range cases {
		if got := SnapToTileGrid(tc.in, tc.w, tc.h); got != tc.want {
			t.Errorf("snap(%v, %dx%d) = %v, want %v", tc.in, tc.w, tc.h, got, tc.want)
		}
	}
}
