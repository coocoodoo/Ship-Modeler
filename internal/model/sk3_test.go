package model

import (
	"math"
	"testing"
)

// SK3's model layer: polygons and slots (Sketch_func.md §5 SK3).

// --- Polygon --------------------------------------------------------------

func TestPolygonHasTheSidesItWasAskedFor(t *testing.T) {
	for _, n := range []int{3, 5, 6, 8, 12} {
		p := NewPolygon(at(0, 0), at(4, 0), n)
		if got := len(p.Points()); got != n {
			t.Errorf("a %d-sided polygon has %d points", n, got)
		}
		if !p.Closed() {
			t.Errorf("a %d-gon is not closed", n)
		}
	}
}

// The first vertex is where the user clicked, exactly: it is a snap target
// like every other placed point (V-99).
func TestPolygonStartsAtItsClickedVertex(t *testing.T) {
	p := NewPolygon(at(1, 1), at(5, 1), 6)
	if got := p.Points()[0]; got != at(5, 1) {
		t.Errorf("first vertex = %v, want the clicked %v", got, at(5, 1))
	}
}

// Every vertex is the same distance from the centre — that is what makes it
// regular rather than merely closed.
func TestPolygonIsRegular(t *testing.T) {
	c := at(2, 3)
	p := NewPolygon(c, at(6, 3), 7)
	want := float64(sub(4))
	for i, v := range p.Points() {
		if got := v.Sub(c).Len(); math.Abs(got-want) > 1.5 {
			t.Errorf("vertex %d is %.2f from the centre, want %.2f", i, got, want)
		}
	}
}

// A polygon turns with its first vertex, which is how a hexagon gets drawn
// point-up or flat-top.
func TestPolygonRotatesWithItsFirstVertex(t *testing.T) {
	up := NewPolygon(at(0, 0), at(0, 4), 6)   // first vertex at the top
	side := NewPolygon(at(0, 0), at(4, 0), 6) // first vertex to the right
	if up.Points()[0] == side.Points()[0] {
		t.Fatal("two differently oriented hexagons start at the same place")
	}
	// Both are still the same size.
	for i := range up.Points() {
		a := up.Points()[i].Sub(at(0, 0)).Len()
		b := side.Points()[i].Sub(at(0, 0)).Len()
		if math.Abs(a-b) > 1.5 {
			t.Errorf("vertex %d: %.2f vs %.2f", i, a, b)
		}
	}
}

func TestPolygonSideCountIsClamped(t *testing.T) {
	if got := len(NewPolygon(at(0, 0), at(4, 0), 2).Points()); got != MinPolygonSides {
		t.Errorf("a 2-sided polygon gave %d points, want the %d floor", got, MinPolygonSides)
	}
	if got := len(NewPolygon(at(0, 0), at(4, 0), 999).Points()); got != MaxPolygonSides {
		t.Errorf("a 999-sided polygon gave %d points, want the %d ceiling", got, MaxPolygonSides)
	}
}

func TestPolygonDegeneracy(t *testing.T) {
	if !NewPolygon(at(2, 2), at(2, 2), 6).Degenerate() {
		t.Error("a polygon with no radius should be degenerate")
	}
	if NewPolygon(at(0, 0), at(4, 0), 6).Degenerate() {
		t.Error("an ordinary hexagon was called degenerate")
	}
}

// A circumscribed polygon is stored as its inscribed equivalent, so there is
// one kind and no variant flag (Sketch_func.md §2). The test is the geometry
// that normalization has to preserve: the flat sides, not the corners, touch
// the radius the user gave.
func TestCircumscribedPolygonTouchesTheRadiusWithItsSides(t *testing.T) {
	const n = 6
	r := float64(sub(4))
	p := NewCircumscribedPolygon(at(0, 0), at(4, 0), n)

	// The midpoint of any side should sit at the requested radius.
	pts := p.Points()
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		mid := at(0, 0)
		mid.X = (a.X + b.X) / 2
		mid.Y = (a.Y + b.Y) / 2
		if got := mid.Sub(at(0, 0)).Len(); math.Abs(got-r) > 3 {
			t.Errorf("side %d's midpoint is %.2f from the centre, want %.2f", i, got, r)
		}
	}
	// And its corners reach further out than an inscribed polygon's, which is
	// the whole difference between the two. A polygon carries its size in its
	// first vertex, not in R.
	inscribed := NewPolygon(at(0, 0), at(4, 0), n)
	outer := p.A.Sub(at(0, 0)).Len()
	inner := inscribed.A.Sub(at(0, 0)).Len()
	if outer <= inner {
		t.Errorf("circumscribed corners reach %.2f, inscribed %.2f — want further out", outer, inner)
	}
	// Specifically, further by 1/cos(pi/n).
	if want := inner / math.Cos(math.Pi/n); math.Abs(outer-want) > 2 {
		t.Errorf("circumscribed corner radius %.2f, want %.2f", outer, want)
	}
}

// --- Slot -----------------------------------------------------------------

func TestSlotIsAClosedCapsule(t *testing.T) {
	s := NewSlot(at(0, 0), at(6, 0), sub(2), 16)
	if !s.Closed() {
		t.Error("a slot is a closed loop")
	}
	pts := s.Points()
	if len(pts) < 8 {
		t.Fatalf("a slot tessellated to %d points", len(pts))
	}
	// It reaches half-width above and below the line of centres...
	var maxY int64
	for _, p := range pts {
		if v := absInt64(p.Y); v > maxY {
			maxY = v
		}
	}
	if absInt64(maxY-sub(2)) > 3 {
		t.Errorf("the slot reaches %d across, want %d", maxY, sub(2))
	}
	// ...and half-width past each end, which is what makes the ends round.
	var minX, maxX int64 = 1 << 40, -(1 << 40)
	for _, p := range pts {
		minX, maxX = min64(minX, p.X), max64(maxX, p.X)
	}
	if absInt64(minX-(-sub(2))) > 3 {
		t.Errorf("the slot starts at %d, want %d", minX, -sub(2))
	}
	if absInt64(maxX-sub(8)) > 3 {
		t.Errorf("the slot ends at %d, want %d", maxX, sub(8))
	}
}

// A slot at an angle is the common case — a mounting track down a wing — and
// the caps have to follow the axis rather than staying upright.
func TestSlotFollowsItsAxis(t *testing.T) {
	s := NewSlot(at(0, 0), at(4, 4), sub(1), 16)
	c := at(2, 2)
	// Every point lies within the capsule: no further from the axis segment
	// than the half-width, and no further from the centre than half the length
	// plus the half-width.
	maxReach := float64(sub(2))*math.Sqrt2 + float64(sub(1)) + 3
	for i, p := range s.Points() {
		if d := p.Sub(c).Len(); d > maxReach {
			t.Errorf("point %d is %.1f from the middle, further than the capsule reaches (%.1f)",
				i, d, maxReach)
		}
	}
}

func TestSlotDegeneracy(t *testing.T) {
	if !NewSlot(at(0, 0), at(4, 0), 0, 16).Degenerate() {
		t.Error("a slot with no width should be degenerate")
	}
	if !NewSlot(at(2, 2), at(2, 2), sub(1), 16).Degenerate() {
		t.Error("a slot whose centres coincide should be degenerate")
	}
	if NewSlot(at(0, 0), at(4, 0), sub(1), 16).Degenerate() {
		t.Error("an ordinary slot was called degenerate")
	}
}

// The shape a slot exists for: cut through a plate and leave a track.
func TestSlotMakesOneRegion(t *testing.T) {
	_, _, sk := sketchWith(t, NewSlot(at(-3, 0), at(3, 0), sub(1), 16))
	arr := sk.Arrangement()
	if len(arr.Regions) != 1 {
		t.Errorf("a slot made %d regions, want 1", len(arr.Regions))
	}
	if len(arr.OpenEnds) != 0 {
		t.Errorf("a slot left %d open ends", len(arr.OpenEnds))
	}
}
