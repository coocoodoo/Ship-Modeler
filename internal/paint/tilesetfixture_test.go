package paint

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The committed test sheet (Tile_paint.md TP2): testdata/tileset_test.png is
// generated, never hand-drawn, and this test keeps the committed bytes honest
// by decoding them and comparing every pixel against the generator. Setting
// MODELER_WRITE_FIXTURE=1 rewrites the file from the generator.
//
// The sheet is a 2x2 grid of 8 px tiles with margin 1 and spacing 1 — 19x19:
//
//	tile 0  solid red        tile 1  solid green
//	tile 2  blue, one white  tile 3  yellow left half,
//	        pixel top-left           transparent right half
//
// Tile 2 is the orientation witness, tile 3 the alpha-threshold one.
func tilesetFixture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 19, 19))
	fill := func(r image.Rectangle, c color.RGBA) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	// The margin and gutters stay transparent, which also proves the slicer
	// never reads them.
	fill(image.Rect(1, 1, 9, 9), color.RGBA{R: 220, G: 60, B: 60, A: 255})
	fill(image.Rect(10, 1, 18, 9), color.RGBA{R: 60, G: 200, B: 90, A: 255})
	fill(image.Rect(1, 10, 9, 18), color.RGBA{R: 60, G: 90, B: 220, A: 255})
	img.SetRGBA(1, 10, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	fill(image.Rect(10, 10, 14, 18), color.RGBA{R: 230, G: 210, B: 70, A: 255})
	return img
}

func fixturePath() string {
	return filepath.Join("..", "..", "testdata", "tileset_test.png")
}

func TestTilesetFixtureMatchesTheCommittedPNG(t *testing.T) {
	want := tilesetFixture()
	if os.Getenv("MODELER_WRITE_FIXTURE") == "1" {
		f, err := os.Create(fixturePath())
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, want); err != nil {
			t.Fatal(err)
		}
		t.Log("fixture rewritten")
	}
	f, err := os.Open(fixturePath())
	if err != nil {
		t.Fatalf("the committed fixture is missing: %v (regenerate with MODELER_WRITE_FIXTURE=1)", err)
	}
	defer f.Close()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 19 || got.Bounds().Dy() != 19 {
		t.Fatalf("fixture is %v, want 19x19", got.Bounds())
	}
	for y := 0; y < 19; y++ {
		for x := 0; x < 19; x++ {
			w := want.RGBAAt(x, y)
			g := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			if w != g {
				t.Fatalf("fixture pixel (%d,%d) = %v, want %v — regenerate it", x, y, g, w)
			}
		}
	}
	// And the slicer reads it as intended.
	ts, err := LoadTilesetFile(fixturePath())
	if err != nil {
		t.Fatal(err)
	}
	ts.TileW, ts.TileH, ts.Margin, ts.Spacing = 8, 8, 1, 1
	if ts.Count() != 4 {
		t.Fatalf("fixture slices into %d tiles, want 4", ts.Count())
	}
	if c := ts.Oriented(2, Orientation{}).RGBAAt(0, 0); c.R != 255 || c.G != 255 {
		t.Errorf("tile 2's witness pixel is %v, want white at its top-left", c)
	}
	opaque := 0
	t3 := ts.Oriented(3, Orientation{})
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if t3.RGBAAt(x, y).A >= StampAlphaThreshold {
				opaque++
			}
		}
	}
	if opaque != 32 {
		t.Errorf("tile 3 has %d opaque pixels, want its half of 32", opaque)
	}
}
