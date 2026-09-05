package paint

import (
	"bytes"
	"image"
	"image/color"
	"modeler/internal/geom"
	"testing"
)

func TestPastePreservesAlphaTransparencyClipAndUndo(t *testing.T) {
	bus, b, uid := stampDoc(t)
	seed := &StampFace{Body: b.ID, Face: uid, Res: 8, Tile: gradTile(), Cells: []image.Point{image.Pt(0, 0)}}
	if err := bus.Run(seed); err != nil {
		t.Fatal(err)
	}
	p := seed.Painted()
	before := append([]byte(nil), p.Img.Pix...)
	clip := image.NewRGBA(image.Rect(0, 0, 3, 2))
	soft := color.RGBA{R: 15, G: 22, B: 31, A: 60}
	clip.SetRGBA(1, 0, soft)
	clip.SetRGBA(2, 1, color.RGBA{R: 255, A: 255})
	c := &StampFace{Body: b.ID, Face: uid, Tile: clip, Cells: []image.Point{image.Pt(-1, 0)}, PreserveAlpha: true}
	if err := bus.Run(c); err != nil {
		t.Fatal(err)
	}
	if At(p, image.Pt(0, 0)) != soft {
		t.Fatal("partial alpha was thresholded")
	}
	if At(p, image.Pt(0, 1)) != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
		t.Fatal("transparent pixel erased destination")
	}
	if At(p, image.Pt(-1, 0)).A != 0 {
		t.Fatal("paste escaped face")
	}
	bus.Undo()
	if !bytes.Equal(before, p.Img.Pix) {
		t.Fatal("undo did not restore original pixels exactly")
	}
	bus.Redo()
	if At(p, image.Pt(0, 0)) != soft {
		t.Fatal("redo failed")
	}
}

func TestCopyFacePixelsClipsAndDetaches(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 8)
	if err != nil {
		t.Fatal(err)
	}
	red := color.RGBA{R: 220, A: 255}
	Set(p, image.Pt(0, 0), red)
	Set(p, image.Pt(-1, 0), red)
	clip := CopyFacePixels(m, fi, p, image.Rect(-2, -2, 2, 2))
	if clip == nil || clip.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatal("copy bounds")
	}
	Set(p, image.Pt(0, 0), color.RGBA{})
	if clip.RGBAAt(0, 0) != red {
		t.Fatal("clipboard aliases source")
	}
	if CopyFacePixels(m, fi, p, image.Rect(0, 0, 2, 2)) != nil {
		t.Fatal("empty copy should be refused")
	}
}

func TestCopyFacePixelsExcludesCutHoles(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 8)
	if err != nil {
		t.Fatal(err)
	}
	red := color.RGBA{R: 220, A: 255}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			Set(p, image.Pt(x, y), red)
		}
	}
	// A cut leaves the shared source texture in place under the new hole.
	hole := []int{}
	for _, uv := range []image.Point{image.Pt(2, 2), image.Pt(2, 4), image.Pt(4, 4), image.Pt(4, 2)} {
		hole = append(hole, len(m.Verts))
		m.Verts = append(m.Verts, p.Frame.ToWorld(geom.Vec2{X: float64(uv.X) * p.Texel, Y: float64(uv.Y) * p.Texel}))
	}
	m.Faces[fi].Loops = append(m.Faces[fi].Loops, hole)
	clip := CopyFacePixels(m, fi, p, image.Rect(0, 0, 8, 8))
	if clip == nil {
		t.Fatal("copy failed")
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want := red
			if x >= 2 && x < 4 && y >= 2 && y < 4 {
				want = color.RGBA{}
			}
			if clip.RGBAAt(x, y) != want {
				t.Fatalf("wrong copy at %d,%d", x, y)
			}
		}
	}
}
