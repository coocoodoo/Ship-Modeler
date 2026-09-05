package paint

import (
	"image"
	"image/color"
	"testing"
)

// The translucent brush of V-158. Alpha is a brush setting rather than a
// property of the colour, so these tests are about what a stroke *lays down*:
// a colour you can see through, laid once however many dabs cross the texel.

var halfRed = color.RGBA{R: 220, G: 40, B: 40, A: 128}

func TestOverLeavesTheOpaquePathAlone(t *testing.T) {
	// The old behaviour, byte for byte: a full-alpha source replaces whatever
	// it lands on. Every golden in the repo depends on this staying true.
	got := Over(blue, red)
	if got != red {
		t.Fatalf("opaque paint over blue gave %v, want %v", got, red)
	}
}

func TestOverIsTheOrdinarySourceOverRule(t *testing.T) {
	// Half of red onto opaque blue: each channel lands halfway between.
	got := Over(color.RGBA{R: 0, G: 0, B: 200, A: 255}, color.RGBA{R: 200, A: 128})
	if got.A != 255 {
		t.Fatalf("compositing onto an opaque texel left alpha %d, want 255", got.A)
	}
	near := func(name string, got, want uint8) {
		t.Helper()
		if int(got) < int(want)-2 || int(got) > int(want)+2 {
			t.Errorf("%s is %d, want about %d", name, got, want)
		}
	}
	near("red", got.R, 100)
	near("blue", got.B, 100)
}

func TestOverOntoNothingKeepsTheAlpha(t *testing.T) {
	// Onto a bare texel there is nothing to blend with, so the paint stays as
	// transparent as it was laid — which is what lets the hull show through it.
	got := Over(color.RGBA{}, halfRed)
	if got.A != halfRed.A {
		t.Fatalf("alpha onto a bare texel became %d, want %d", got.A, halfRed.A)
	}
}

func TestATranslucentDabDoesNotCoverWhatIsUnderIt(t *testing.T) {
	p := testPaint(t, 4)
	at := image.Point{X: 3, Y: 3}
	Stroke(p, Brush{Color: blue, Size: 1}, at, at)
	Stroke(p, Brush{Color: halfRed, Size: 1, Once: NewStamp(halfRed)}, at, at)

	got := At(p, at)
	if got == blue {
		t.Fatal("the translucent dab did not change the texel at all")
	}
	if got.R <= blue.R || got.B >= blue.B {
		t.Fatalf("texel is %v: half-transparent red should have pulled it toward red", got)
	}
	if got.A != 255 {
		t.Errorf("paint over opaque paint left alpha %d, want it opaque", got.A)
	}
}

func TestATranslucentStrokeIsEvenAlongItsJoins(t *testing.T) {
	// Freehand paints one Stroke per pair of samples, so every interior texel
	// is written twice. Without the per-stroke stamp those texels composite
	// twice and the line comes out blotched at its own joins.
	p := testPaint(t, 8)
	b := Brush{Color: halfRed, Size: 1, Once: NewStamp(halfRed)}
	pts := []image.Point{{X: 2, Y: 2}, {X: 6, Y: 2}, {X: 10, Y: 2}}
	prev := pts[0]
	for _, t2 := range pts {
		Stroke(p, b, prev, t2)
		prev = t2
	}
	want := At(p, image.Point{X: 3, Y: 2}) // a texel only one segment covered
	for x := 2; x <= 10; x++ {
		if got := At(p, image.Point{X: x, Y: 2}); got != want {
			t.Fatalf("texel (%d,2) is %v but the rest of the line is %v", x, got, want)
		}
	}
}

func TestOverlappingDabsWithoutAStampWouldStack(t *testing.T) {
	// The negative control for the test above: the stamp is doing real work,
	// not papering over a difference that was never there.
	p := testPaint(t, 4)
	at := image.Point{X: 2, Y: 2}
	b := Brush{Color: halfRed, Size: 1}
	Stroke(p, b, at, at)
	once := At(p, at)
	Stroke(p, b, at, at)
	if At(p, at) == once {
		t.Fatal("two unstamped dabs landed the same as one: nothing accumulated")
	}
}

func TestATranslucentFillBlendsRatherThanReplaces(t *testing.T) {
	p := testPaint(t, 4)
	r := image.Rect(0, 0, 8, 8)
	Fill(p, r, image.Point{X: 1, Y: 1}, blue, nil)
	Fill(p, r, image.Point{X: 1, Y: 1}, halfRed, nil)

	got := At(p, image.Point{X: 4, Y: 4})
	if got == blue {
		t.Fatal("the translucent fill did not change the region")
	}
	if got.R <= blue.R {
		t.Fatalf("filled texel is %v: it should have moved toward red", got)
	}
}

func TestTheEyedropperReadsTheAlphaItFinds(t *testing.T) {
	// A translucent texel sampled as opaque would be a colour you cannot pick
	// back up: the dropper would quietly promote it to solid.
	p := testPaint(t, 4)
	at := image.Point{X: 1, Y: 1}
	Set(p, at, halfRed)
	got := Sample(p, at, blue)
	if got.A != halfRed.A {
		t.Fatalf("the dropper read alpha %d, want %d", got.A, halfRed.A)
	}
}

func TestATranslucentGradientStaysTranslucentEndToEnd(t *testing.T) {
	// Both ends of the ramp carry the brush's alpha (the command sets that up),
	// so a translucent gradient fades between two hues rather than between
	// thin paint and thick.
	p := testPaint(t, 8)
	from, to := halfRed, color.RGBA{R: 40, G: 80, B: 220, A: 128}
	Gradient(p, image.Rect(0, 0, 16, 16), image.Point{X: 0}, image.Point{X: 15}, from, to, DitherNone, nil)

	a := At(p, image.Point{X: 1, Y: 4})
	z := At(p, image.Point{X: 14, Y: 4})
	if a.A != z.A {
		t.Fatalf("the ramp ends differ in alpha: %d at one end, %d at the other", a.A, z.A)
	}
	if a.A == 255 {
		t.Fatal("the ramp came out opaque")
	}
	if a.B >= z.B {
		t.Fatalf("the ramp did not travel: %v to %v", a, z)
	}
}
