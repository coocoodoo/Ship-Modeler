package paint

import (
	"image"
	"testing"
)

// Ordered dithering is how a two-colour ramp gets a middle (SPEC-UX §13.4).
//
// The matrices are the whole feature, so they are checked as matrices: a Bayer
// matrix of order n is a permutation of 0..n²-1, and that is what makes its
// thresholds spread evenly instead of clumping. Get one entry wrong and the
// ramp still looks like a ramp — it just has a stripe in it that nobody can
// explain.

func TestBayerMatricesArePermutations(t *testing.T) {
	for _, d := range []Dither{Dither2x2, Dither4x4, Dither8x8} {
		n := d.Size()
		seen := make([]bool, n*n)
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				v := d.rank(x, y)
				if v < 0 || v >= n*n {
					t.Fatalf("%s: rank at (%d,%d) is %d, outside 0..%d", d, x, y, v, n*n-1)
				}
				if seen[v] {
					t.Errorf("%s: rank %d appears more than once", d, v)
				}
				seen[v] = true
			}
		}
	}
}

func TestBayerThresholdsSpanTheRange(t *testing.T) {
	for _, d := range []Dither{Dither2x2, Dither4x4, Dither8x8} {
		n := d.Size()
		lo, hi := 1.0, 0.0
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				v := d.Threshold(x, y)
				if v < 0 || v >= 1 {
					t.Fatalf("%s: threshold at (%d,%d) is %v, want [0,1)", d, x, y, v)
				}
				if v < lo {
					lo = v
				}
				if v > hi {
					hi = v
				}
			}
		}
		// The extremes have to leave room at both ends: a threshold of exactly
		// zero would paint a texel at zero coverage, and one of exactly one
		// would leave a texel bare at full coverage.
		if lo <= 0 || hi >= 1 {
			t.Errorf("%s: thresholds run %v..%v, which touches an end", d, lo, hi)
		}
	}
}

// TestDitherTilesTheGrid is why the matrix is indexed by texel and not by
// position within a shape: a dithered gradient painted twice in two passes has
// to line up with itself, and a pattern that restarted at each shape's corner
// would seam.
func TestDitherTilesTheGrid(t *testing.T) {
	for _, d := range []Dither{Dither2x2, Dither4x4, Dither8x8} {
		n := d.Size()
		for _, at := range []image.Point{{X: 0, Y: 0}, {X: 3, Y: 5}, {X: -1, Y: -7}} {
			if got, want := d.Threshold(at.X+n, at.Y), d.Threshold(at.X, at.Y); got != want {
				t.Errorf("%s: shifting one tile in x changed the threshold at %v", d, at)
			}
			if got, want := d.Threshold(at.X, at.Y+n), d.Threshold(at.X, at.Y); got != want {
				t.Errorf("%s: shifting one tile in y changed the threshold at %v", d, at)
			}
		}
	}
}

// TestDitherCoverageIsMonotonic is the property a ramp depends on: raising the
// coverage can only ever turn texels on, never off.
func TestDitherCoverageIsMonotonic(t *testing.T) {
	for _, d := range []Dither{DitherNone, Dither2x2, Dither4x4, Dither8x8} {
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				on := false
				for step := 0; step <= 20; step++ {
					cov := float64(step) / 20
					now := d.Covers(x, y, cov)
					if on && !now {
						t.Fatalf("%s: texel (%d,%d) switched off again at coverage %v",
							d, x, y, cov)
					}
					on = now
				}
				if !on {
					t.Errorf("%s: texel (%d,%d) never paints, even at full coverage", d, x, y)
				}
			}
		}
	}
}

// TestDitherHalfCoverageIsHalfTheTexels is what "ordered" buys over a plain
// threshold: at half coverage a dithered fill is a checkerboard, not a
// coin flip and not a solid block.
func TestDitherHalfCoverageIsHalfTheTexels(t *testing.T) {
	for _, d := range []Dither{Dither2x2, Dither4x4, Dither8x8} {
		n := d.Size()
		on := 0
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				if d.Covers(x, y, 0.5) {
					on++
				}
			}
		}
		if want := n * n / 2; on != want {
			t.Errorf("%s: half coverage painted %d of %d texels, want %d",
				d, on, n*n, want)
		}
	}
}

func TestDitherNoneIsAPlainCut(t *testing.T) {
	if DitherNone.Size() != 0 {
		t.Errorf("DitherNone has size %d, want 0", DitherNone.Size())
	}
	if DitherNone.Covers(3, 4, 0.4) {
		t.Error("with no dithering, coverage below half must not paint")
	}
	if !DitherNone.Covers(3, 4, 0.6) {
		t.Error("with no dithering, coverage above half must paint")
	}
}

func TestParseDitherRoundTrips(t *testing.T) {
	for _, d := range Dithers {
		got, ok := ParseDither(d.String())
		if !ok || got != d {
			t.Errorf("%q parsed back as %v (ok=%v)", d.String(), got, ok)
		}
	}
	if _, ok := ParseDither("3x3"); ok {
		t.Error("3x3 is not a Bayer order we have")
	}
}
