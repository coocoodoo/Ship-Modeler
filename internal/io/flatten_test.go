package io

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"modeler/internal/model"
)

// What leaves the program when a face is painted (V-158).
//
// A face's texture only holds the texels that were painted; the viewport shows
// the body's colour through the rest. An exported material has no shader to do
// that with, so an unpainted texel under an opaque material is black — which
// meant one painted pixel used to blacken the rest of its face in every viewer
// that read the file correctly. The fix is to send the hull out inside the
// picture, and these are the tests that say so.

func TestFlattenPutsTheHullBehindThePaint(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255}) // painted
	// (1,0) left bare.
	hull := color.RGBA{R: 140, G: 160, B: 175, A: 255}

	out := flattenPaint(img, hull)
	if got := out.RGBAAt(0, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("the painted texel came out as %v, want the paint unchanged", got)
	}
	if got := out.RGBAAt(1, 0); got != hull {
		t.Errorf("the bare texel came out as %v, want the hull's own colour %v", got, hull)
	}
}

func TestFlattenResolvesTranslucentPaintAgainstTheHull(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 128}) // a glaze of red
	hull := color.RGBA{R: 0, G: 200, B: 0, A: 255}

	got := flattenPaint(img, hull).RGBAAt(0, 0)
	if got.A != 255 {
		t.Fatalf("the flattened texel kept alpha %d, want it opaque", got.A)
	}
	near := func(name string, got, want uint8) {
		t.Helper()
		if int(got) < int(want)-2 || int(got) > int(want)+2 {
			t.Errorf("%s is %d, want about %d", name, got, want)
		}
	}
	near("red", got.R, 128)
	near("green", got.G, 100)
}

func TestFlattenLeavesTheDocumentAlone(t *testing.T) {
	// The picture it is handed is the document's own. Writing a file must not
	// change the ship.
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	flattenPaint(img, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	if got := img.RGBAAt(0, 0); got != (color.RGBA{}) {
		t.Fatalf("flattening wrote back into the source picture: %v", got)
	}
}

// readPNGTexture pulls one exported texture back off disk.
func readPNGTexture(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open the exported texture: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode the exported texture: %v", err)
	}
	return img
}

func assertOpaqueTexture(t *testing.T, img image.Image, hull color.RGBA) {
	t.Helper()
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xFFFF {
				t.Fatalf("texel (%d,%d) exported with alpha %d: a viewer would draw it black", x, y, a>>8)
			}
		}
	}
	// The corner was never painted, so it has to be the hull's own colour.
	r, g, bl, _ := img.At(b.Min.X, b.Min.Y).RGBA()
	got := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255}
	if got != hull {
		t.Errorf("an unpainted texel exported as %v, want the hull's %v", got, hull)
	}
}

func TestExportedOBJTextureIsOpaque(t *testing.T) {
	dir := t.TempDir()
	doc := exportDoc(t)
	if err := ExportOBJ(filepath.Join(dir, "ship.obj"), doc); err != nil {
		t.Fatalf("export: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "paint", "*.png"))
	if len(matches) == 0 {
		t.Fatal("the export wrote no texture at all")
	}
	assertOpaqueTexture(t, readPNGTexture(t, matches[0]), hullColorOf(t, doc))
}

func TestExportedGLTFTextureIsOpaque(t *testing.T) {
	dir := t.TempDir()
	doc := exportDoc(t)
	if err := ExportGLTF(filepath.Join(dir, "ship.gltf"), doc); err != nil {
		t.Fatalf("export: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "paint", "*.png"))
	if len(matches) == 0 {
		t.Fatal("the export wrote no texture at all")
	}
	assertOpaqueTexture(t, readPNGTexture(t, matches[0]), hullColorOf(t, doc))
}

func hullColorOf(t *testing.T, doc *model.Document) color.RGBA {
	t.Helper()
	for _, b := range doc.Bodies {
		if b.Name == "Hull" {
			return b.Color
		}
	}
	t.Fatal("the fixture has no hull")
	return color.RGBA{}
}
