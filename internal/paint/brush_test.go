package paint

import (
	"image"
	"image/color"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The brush tests of SPEC-UX §13.2. A stroke is the only thing in the program
// that turns a run of pointer samples into document state, so the gaps between
// those samples are the whole problem: a brush that only paints where the mouse
// happened to be sampled draws a dotted line at any speed above a crawl.

var red = color.RGBA{R: 220, G: 40, B: 40, A: 255}
var blue = color.RGBA{R: 40, G: 80, B: 220, A: 255}

func testPaint(t *testing.T, res int) *mesh.FacePaint {
	t.Helper()
	m, fi := plate()
	p, err := Allocate(m, fi, res)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[fi].Paint = p
	return p
}

func TestStrokeJoinsUpItsSamples(t *testing.T) {
	p := testPaint(t, 4)
	b := Brush{Color: red, Size: 1}
	// Two samples ten texels apart: a stroke has to fill the gap.
	Stroke(p, b, image.Point{X: 2, Y: 2}, image.Point{X: 12, Y: 2})
	for x := 2; x <= 12; x++ {
		if got := At(p, image.Point{X: x, Y: 2}); got != red {
			t.Fatalf("texel (%d,2) is %v, want red — the stroke has a hole in it", x, got)
		}
	}
	if got := At(p, image.Point{X: 13, Y: 2}); got.A != 0 {
		t.Error("the stroke ran past its end sample")
	}
}

func TestStrokeInterpolatesDiagonally(t *testing.T) {
	p := testPaint(t, 4)
	Stroke(p, Brush{Color: red, Size: 1}, image.Point{X: 0, Y: 0}, image.Point{X: 6, Y: 3})
	// Every step of the walk is 8-connected to the last, so no row between the
	// endpoints may be empty.
	for y := 0; y <= 3; y++ {
		found := false
		for x := 0; x <= 6; x++ {
			if At(p, image.Point{X: x, Y: y}).A != 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("row %d is empty: the diagonal stroke skipped it", y)
		}
	}
}

func TestBrushSizePaintsASquare(t *testing.T) {
	for _, size := range BrushSizes {
		p := testPaint(t, 16)
		Stroke(p, Brush{Color: red, Size: size}, image.Point{X: 40, Y: 40}, image.Point{X: 40, Y: 40})
		painted := 0
		// The window has to hold the largest brush with room to spare, or it
		// counts a clipped square and calls the brush wrong.
		for y := 30; y < 60; y++ {
			for x := 30; x < 60; x++ {
				if At(p, image.Point{X: x, Y: y}).A != 0 {
					painted++
				}
			}
		}
		if painted != size*size {
			t.Errorf("brush %d painted %d texels, want %d", size, painted, size*size)
		}
	}
}

func TestEraserClearsToUnpainted(t *testing.T) {
	p := testPaint(t, 4)
	Stroke(p, Brush{Color: red, Size: 4}, image.Point{X: 8, Y: 8}, image.Point{X: 8, Y: 8})
	Stroke(p, Brush{Erase: true, Size: 1}, image.Point{X: 8, Y: 8}, image.Point{X: 8, Y: 8})
	if got := At(p, image.Point{X: 8, Y: 8}); got.A != 0 {
		t.Errorf("erased texel is %v, want fully transparent so the body colour shows", got)
	}
	// The eraser is a brush, not a bomb: the rest of the square survives.
	if got := At(p, image.Point{X: 10, Y: 10}); got != red {
		t.Errorf("texel outside the eraser is %v, want red", got)
	}
}

func TestFillSpreadsOverMatchingTexelsOnly(t *testing.T) {
	p := testPaint(t, 4)
	// A vertical wall of red splits the face in two.
	Stroke(p, Brush{Color: red, Size: 1}, image.Point{X: 10, Y: 0}, image.Point{X: 10, Y: 15})
	n := Fill(p, image.Rect(0, 0, 32, 16), image.Point{X: 4, Y: 8}, blue)
	if n == 0 {
		t.Fatal("fill painted nothing")
	}
	if got := At(p, image.Point{X: 0, Y: 0}); got != blue {
		t.Errorf("near corner is %v, want blue", got)
	}
	if got := At(p, image.Point{X: 10, Y: 8}); got != red {
		t.Error("fill overwrote the wall it should have stopped at")
	}
	if got := At(p, image.Point{X: 20, Y: 8}); got.A != 0 {
		t.Error("fill leaked past the wall to the far side")
	}
	// Filling with the colour that is already there is a no-op, not a hang.
	if again := Fill(p, image.Rect(0, 0, 32, 16), image.Point{X: 4, Y: 8}, blue); again != 0 {
		t.Errorf("re-filling the same region painted %d texels, want 0", again)
	}
}

func TestFillStaysInsideTheRegionItIsGiven(t *testing.T) {
	p := testPaint(t, 4)
	// The region is the face; the image is bigger because of its margin. A
	// fill that ignores the region floods the margin too, and the margin is
	// exactly the band that is not on the face at all.
	Fill(p, image.Rect(0, 0, 32, 16), image.Point{X: 4, Y: 8}, blue)
	if got := At(p, image.Point{X: -1, Y: -1}); got.A != 0 {
		t.Errorf("margin texel is %v, want untouched", got)
	}
	if got := At(p, image.Point{X: 31, Y: 15}); got != blue {
		t.Error("the last texel of the region was not filled")
	}
}

func TestPickSamplesWhatYouSee(t *testing.T) {
	p := testPaint(t, 4)
	body := color.RGBA{R: 0x8E, G: 0xA3, B: 0xB0, A: 255}
	Stroke(p, Brush{Color: red, Size: 1}, image.Point{X: 5, Y: 5}, image.Point{X: 5, Y: 5})

	if got := Sample(p, image.Point{X: 5, Y: 5}, body); got != red {
		t.Errorf("picked %v over a painted texel, want the paint", got)
	}
	// Over bare geometry the eyedropper reads the body colour, which is what
	// is actually on screen there (SPEC-UX §13.2).
	if got := Sample(p, image.Point{X: 6, Y: 5}, body); got != body {
		t.Errorf("picked %v over an unpainted texel, want the body colour %v", got, body)
	}
	if got := Sample(nil, image.Point{X: 0, Y: 0}, body); got != body {
		t.Errorf("picked %v on a face with no texture at all, want %v", got, body)
	}
}

func TestStrokeReportsTheRectItTouched(t *testing.T) {
	p := testPaint(t, 4)
	dirty := Stroke(p, Brush{Color: red, Size: 2}, image.Point{X: 4, Y: 4}, image.Point{X: 9, Y: 4})
	if dirty.Empty() {
		t.Fatal("a stroke that painted something reported an empty dirty rect")
	}
	// Everything the stroke changed has to be inside the rect the caller is
	// told to re-upload and to snapshot for undo. A texel outside it is a
	// pixel that never reaches the screen and never comes back on Ctrl+Z.
	for y := -1; y < 20; y++ {
		for x := -1; x < 20; x++ {
			tx := image.Point{X: x, Y: y}
			if At(p, tx).A != 0 && !tx.In(dirty) {
				t.Errorf("painted texel %v is outside the dirty rect %v", tx, dirty)
			}
		}
	}
}

func TestResampleRebuildsAtTheNewResolution(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[fi].Paint = p
	Stroke(p, Brush{Color: red, Size: 1}, image.Point{X: 0, Y: 0}, image.Point{X: 31, Y: 0})
	probe := World(p, image.Point{X: 10, Y: 0})

	out, err := Resample(m, fi, p, 16)
	if err != nil {
		t.Fatalf("resample: %v", err)
	}
	if out.Res != 16 {
		t.Errorf("res = %d, want 16", out.Res)
	}
	if want := 1.0 / 16; out.Texel != want {
		t.Errorf("texel = %v, want %v", out.Texel, want)
	}
	// Nearest resampling: the colour at a world point is what it was before.
	if got := At(out, Texel(out, probe)); got != red {
		t.Errorf("world point %v is %v after resampling, want red", probe, got)
	}
	if got := At(out, Texel(out, World(p, image.Point{X: 10, Y: 5}))); got.A != 0 {
		t.Error("resampling painted a texel that was empty before")
	}
}

func TestStrokeBetweenWorldPointsReachesBothEnds(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	// Two world points on the face, a long way apart.
	a := p.Frame.ToWorld(geom.Vec2{X: 0.1, Y: 0.1})
	b := p.Frame.ToWorld(geom.Vec2{X: 7.9, Y: 3.9})
	Stroke(p, Brush{Color: red, Size: 1}, Texel(p, a), Texel(p, b))
	if At(p, Texel(p, a)) != red || At(p, Texel(p, b)) != red {
		t.Error("the stroke did not reach both of its ends")
	}
}
