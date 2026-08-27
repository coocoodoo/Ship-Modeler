package paint

import (
	"image"
	"image/color"
	"testing"
)

// The shape and gradient tools of SPEC-UX §13.4.
//
// Every one of these is a two-point tool: press somewhere, release somewhere
// else, and what lands is decided entirely by those two texels. That makes them
// easy to test and easy to get subtly wrong — an ellipse that is a texel wider
// on one side, a rectangle whose outline is drawn twice at the corners, a
// gradient that reaches its second colour a texel before the end.

var hullGrey = color.RGBA{R: 142, G: 163, B: 176, A: 255}

func hardBrush(c color.RGBA, size int) Brush {
	return Brush{Color: c, Size: size, Under: hullGrey}
}

func TestRectOutlineDrawsFourWallsAndNoMiddle(t *testing.T) {
	p := testPaint(t, 32)
	DrawRect(p, hardBrush(red, 1), image.Point{X: 2, Y: 2}, image.Point{X: 8, Y: 6}, false)

	for x := 2; x <= 8; x++ {
		for _, y := range []int{2, 6} {
			if got := At(p, image.Point{X: x, Y: y}); got != red {
				t.Fatalf("(%d,%d) is %v, want the outline", x, y, got)
			}
		}
	}
	for y := 2; y <= 6; y++ {
		for _, x := range []int{2, 8} {
			if got := At(p, image.Point{X: x, Y: y}); got != red {
				t.Fatalf("(%d,%d) is %v, want the outline", x, y, got)
			}
		}
	}
	for y := 3; y <= 5; y++ {
		for x := 3; x <= 7; x++ {
			if got := At(p, image.Point{X: x, Y: y}); got.A != 0 {
				t.Fatalf("(%d,%d) is %v, but an outlined rectangle is hollow", x, y, got)
			}
		}
	}
}

func TestRectFilledCoversItsBox(t *testing.T) {
	p := testPaint(t, 32)
	r := DrawRect(p, hardBrush(red, 1), image.Point{X: 8, Y: 6}, image.Point{X: 2, Y: 2}, true)

	// Dragged up and to the left: the corners still come out the same box.
	if want := image.Rect(2, 2, 9, 7); r != want {
		t.Errorf("the dirty rect is %v, want %v", r, want)
	}
	for y := 2; y <= 6; y++ {
		for x := 2; x <= 8; x++ {
			if got := At(p, image.Point{X: x, Y: y}); got != red {
				t.Fatalf("(%d,%d) is %v inside a filled rectangle", x, y, got)
			}
		}
	}
	if got := At(p, image.Point{X: 9, Y: 6}); got.A != 0 {
		t.Error("the fill ran past its box")
	}
}

func TestEllipseIsSymmetricInBothAxes(t *testing.T) {
	p := testPaint(t, 128)
	// An odd-sized box so there is a true centre row and column.
	a, z := image.Point{X: 10, Y: 10}, image.Point{X: 30, Y: 24}
	DrawEllipse(p, hardBrush(red, 1), a, z, false)

	painted := func(x, y int) bool { return At(p, image.Point{X: x, Y: y}).A != 0 }
	for y := a.Y; y <= z.Y; y++ {
		for x := a.X; x <= z.X; x++ {
			mx := a.X + z.X - x
			my := a.Y + z.Y - y
			if painted(x, y) != painted(mx, y) {
				t.Fatalf("(%d,%d) and its mirror (%d,%d) disagree", x, y, mx, y)
			}
			if painted(x, y) != painted(x, my) {
				t.Fatalf("(%d,%d) and its mirror (%d,%d) disagree", x, y, x, my)
			}
		}
	}
}

func TestEllipseTouchesEachSideOfItsBoxExactlyOnce(t *testing.T) {
	p := testPaint(t, 128)
	a, z := image.Point{X: 10, Y: 10}, image.Point{X: 30, Y: 24}
	DrawEllipse(p, hardBrush(red, 1), a, z, false)

	// It has to reach every wall — an ellipse that stops a texel short reads as
	// a wonky circle — and it must not spill past one.
	count := func(pts []image.Point) int {
		n := 0
		for _, t := range pts {
			if At(p, t).A != 0 {
				n++
			}
		}
		return n
	}
	var left, top []image.Point
	for y := a.Y; y <= z.Y; y++ {
		left = append(left, image.Point{X: a.X, Y: y})
	}
	for x := a.X; x <= z.X; x++ {
		top = append(top, image.Point{X: x, Y: a.Y})
	}
	if n := count(left); n == 0 {
		t.Error("the ellipse never reaches its left wall")
	}
	if n := count(top); n == 0 {
		t.Error("the ellipse never reaches its top wall")
	}
	for y := a.Y - 1; y <= z.Y+1; y++ {
		if At(p, image.Point{X: a.X - 1, Y: y}).A != 0 {
			t.Fatalf("the ellipse spilled past its left wall at y=%d", y)
		}
		if At(p, image.Point{X: z.X + 1, Y: y}).A != 0 {
			t.Fatalf("the ellipse spilled past its right wall at y=%d", y)
		}
	}
}

func TestFilledEllipseHasNoHoles(t *testing.T) {
	p := testPaint(t, 128)
	a, z := image.Point{X: 10, Y: 10}, image.Point{X: 30, Y: 24}
	DrawEllipse(p, hardBrush(red, 1), a, z, true)

	// Every painted row is one unbroken run: that is what "filled" means, and a
	// span fill that took its extremes from the wrong scanline leaves gaps.
	for y := a.Y; y <= z.Y; y++ {
		runs, inRun := 0, false
		for x := a.X - 1; x <= z.X+1; x++ {
			on := At(p, image.Point{X: x, Y: y}).A != 0
			if on && !inRun {
				runs++
			}
			inRun = on
		}
		if runs > 1 {
			t.Fatalf("row %d has %d separate runs in a filled ellipse", y, runs)
		}
	}
}

func TestShapesOfOneTexelDegradeToADab(t *testing.T) {
	for _, name := range []string{"rect", "ellipse"} {
		p := testPaint(t, 32)
		at := image.Point{X: 5, Y: 5}
		if name == "rect" {
			DrawRect(p, hardBrush(red, 1), at, at, false)
		} else {
			DrawEllipse(p, hardBrush(red, 1), at, at, false)
		}
		if got := At(p, at); got != red {
			t.Errorf("%s: a zero-sized shape left %v at its own texel", name, got)
		}
		if got := At(p, image.Point{X: 6, Y: 5}); got.A != 0 {
			t.Errorf("%s: a zero-sized shape painted its neighbour", name)
		}
	}
}

// TestGradientRunsFromOneColourToTheOther is the tool's whole contract: the
// texel you pressed on is the first colour, the texel you released on is the
// second, and everything between is on the way.
func TestGradientRunsFromOneColourToTheOther(t *testing.T) {
	p := testPaint(t, 32)
	region := image.Rect(0, 0, 32, 11)
	a, z := image.Point{X: 0, Y: 5}, image.Point{X: 31, Y: 5}
	Gradient(p, region, a, z, red, blue, DitherNone)

	if got := At(p, a); got != red {
		t.Errorf("the start texel is %v, want the first colour %v", got, red)
	}
	if got := At(p, z); got != blue {
		t.Errorf("the end texel is %v, want the second colour %v", got, blue)
	}
	// Monotonic along the axis: no band goes backwards.
	last := -1
	for x := 0; x <= 31; x++ {
		c := At(p, image.Point{X: x, Y: 5})
		if c.A == 0 {
			t.Fatalf("the gradient left texel (%d,5) unpainted", x)
		}
		if int(c.B) < last {
			t.Fatalf("the ramp went backwards at x=%d", x)
		}
		last = int(c.B)
	}
}

func TestGradientIsPerpendicularToItsDrag(t *testing.T) {
	p := testPaint(t, 32)
	region := image.Rect(0, 0, 32, 11)
	Gradient(p, region, image.Point{X: 0, Y: 5}, image.Point{X: 31, Y: 5}, red, blue, DitherNone)

	// A horizontal drag means every column is one colour top to bottom.
	for x := 0; x < 32; x++ {
		want := At(p, image.Point{X: x, Y: 0})
		for y := 1; y < 11; y++ {
			if got := At(p, image.Point{X: x, Y: y}); got != want {
				t.Fatalf("column %d changes down its height: (%d,%d) is %v, not %v",
					x, x, y, got, want)
			}
		}
	}
}

// TestDitheredGradientUsesOnlyTheTwoColours is the difference between the modes.
// A smooth ramp invents colours; a dithered one is not allowed to, which is the
// entire reason a pixel artist reaches for it.
func TestDitheredGradientUsesOnlyTheTwoColours(t *testing.T) {
	for _, d := range []Dither{Dither2x2, Dither4x4, Dither8x8} {
		p := testPaint(t, 32)
		region := image.Rect(0, 0, 32, 11)
		Gradient(p, region, image.Point{X: 0, Y: 5}, image.Point{X: 31, Y: 5}, red, blue, d)

		mixed := 0
		for y := 0; y < 11; y++ {
			for x := 0; x < 32; x++ {
				switch At(p, image.Point{X: x, Y: y}) {
				case red, blue:
				default:
					mixed++
				}
			}
		}
		if mixed != 0 {
			t.Errorf("%s: %d texels are neither of the two colours", d, mixed)
		}
		// And it does mix them, rather than being a hard edge down the middle.
		mid := 0
		for x := 12; x < 20; x++ {
			if At(p, image.Point{X: x, Y: 5}) == blue {
				mid++
			}
		}
		if mid == 0 || mid == 8 {
			t.Errorf("%s: the middle of the ramp is solid, so nothing was dithered", d)
		}
	}
}

func TestGradientOfZeroLengthFillsWithTheSecondColour(t *testing.T) {
	p := testPaint(t, 32)
	region := image.Rect(0, 0, 8, 8)
	at := image.Point{X: 4, Y: 4}
	Gradient(p, region, at, at, red, blue, DitherNone)
	// A drag that went nowhere has no direction to ramp along. Filling with the
	// colour under the pointer is the answer that matches what the release
	// looked like.
	if got := At(p, image.Point{X: 0, Y: 0}); got != blue {
		t.Errorf("a zero-length gradient left %v, want a flat fill of %v", got, blue)
	}
}

// TestSoftBrushFadesIntoWhatIsUnderIt covers the soft brush of SPEC-UX §13.4.
//
// The blend happens here, against the body's own colour, and the result is
// stored fully opaque. Storing a partial alpha instead would look identical on
// an unpainted face and wrong everywhere else: the texture composites over the
// body colour, not over the paint already on the face, so a half-alpha texel
// laid over existing paint would show the body through it.
func TestSoftBrushFadesIntoWhatIsUnderIt(t *testing.T) {
	p := testPaint(t, 128)
	b := hardBrush(red, 16)
	b.Soft = true
	centre := image.Point{X: 40, Y: 40}
	Stroke(p, b, centre, centre)

	mid := At(p, image.Point{X: centre.X + 8, Y: centre.Y + 8})
	if mid.A != 255 {
		t.Errorf("the centre texel is %v; a soft brush still stores opaque texels", mid)
	}
	if mid != red {
		t.Errorf("the centre of a soft dab is %v, want the full brush colour %v", mid, red)
	}

	edge := At(p, image.Point{X: centre.X + 1, Y: centre.Y + 8})
	if edge.A == 0 {
		t.Fatal("the soft dab painted nothing near its edge")
	}
	if edge == red {
		t.Error("the edge of a soft dab is the full brush colour, so it is not soft")
	}
	if edge == hullGrey {
		t.Error("the edge of a soft dab is exactly the body colour, so nothing landed")
	}
	// Between the two, and nearer the body colour than the centre is.
	if !between(edge.R, red.R, hullGrey.R) {
		t.Errorf("the edge texel %v is not between the brush and the body colour", edge)
	}
}

func TestSoftBrushIsRoundAndHardBrushIsSquare(t *testing.T) {
	corner := image.Point{X: 40, Y: 40}

	hard := testPaint(t, 128)
	Stroke(hard, hardBrush(red, 8), corner, corner)
	if got := At(hard, corner); got != red {
		t.Fatalf("the hard brush left %v in its own corner", got)
	}

	soft := testPaint(t, 128)
	b := hardBrush(red, 8)
	b.Soft = true
	Stroke(soft, b, corner, corner)
	if got := At(soft, corner); got.A != 0 {
		t.Errorf("the soft brush painted the corner of its box (%v), so it is square", got)
	}
	if got := At(soft, image.Point{X: corner.X + 4, Y: corner.Y + 4}); got.A == 0 {
		t.Error("the soft brush painted nothing at the centre of its box")
	}
}

// TestDitheredSoftBrushStaysOnOneColour is the pixel-native half of softness:
// the falloff becomes a pattern of whole texels rather than a blend.
func TestDitheredSoftBrushStaysOnOneColour(t *testing.T) {
	p := testPaint(t, 128)
	b := hardBrush(red, 16)
	b.Soft, b.Dither = true, Dither4x4
	centre := image.Point{X: 40, Y: 40}
	Stroke(p, b, centre, centre)

	painted, blended := 0, 0
	for y := 36; y < 60; y++ {
		for x := 36; x < 60; x++ {
			c := At(p, image.Point{X: x, Y: y})
			if c.A == 0 {
				continue
			}
			painted++
			if c != red {
				blended++
			}
		}
	}
	if painted == 0 {
		t.Fatal("the dithered soft brush painted nothing")
	}
	if blended != 0 {
		t.Errorf("%d of %d painted texels are a blend, but a dithered brush "+
			"only ever writes its own colour", blended, painted)
	}
}

func between(v, a, b uint8) bool {
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	return v >= lo && v <= hi
}
