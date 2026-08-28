package model

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// SK2's model layer: arcs and ellipses (Sketch_func.md §5 SK2). Written before
// the code they describe.
//
// The rule that runs through all of it: an open curve's tessellation must
// begin and end *exactly* on the points it was built from. Snapping,
// region-closing and every later tool that joins to a curve depend on those
// two positions being the ones the user clicked, not the nearest the cosine
// happened to land on (Sketch_func.md §6).

// --- Arc ------------------------------------------------------------------

func TestArcStartsAndEndsExactlyWhereItWasBuilt(t *testing.T) {
	c := at(0, 0)
	start := at(4, 0)
	end := at(0, 4)
	arc := NewArc(c, start, end, true, 32)

	pts := arc.Points()
	if len(pts) < 3 {
		t.Fatalf("a quarter arc tessellated to %d points", len(pts))
	}
	if pts[0] != start {
		t.Errorf("first point = %v, want the start %v exactly", pts[0], start)
	}
	if pts[len(pts)-1] != end {
		t.Errorf("last point = %v, want the end %v exactly", pts[len(pts)-1], end)
	}
	if arc.Closed() {
		t.Error("an arc is not a closed loop")
	}
}

// Every point in between sits on the circle, which is what makes it an arc
// rather than a polyline that happens to share two endpoints.
func TestArcPointsLieOnItsCircle(t *testing.T) {
	c := at(1, 1)
	arc := NewArc(c, at(5, 1), at(1, 5), true, 24)
	want := float64(sub(4))
	for i, p := range arc.Points() {
		got := p.Sub(c).Len()
		if math.Abs(got-want) > 1.5 { // subunits: a rounding's worth
			t.Errorf("point %d is %.2f from the centre, want %.2f", i, got, want)
		}
	}
}

// The sweep direction is the whole difference between the short way round and
// the long way, and a user who asked for one and got the other has drawn the
// wrong shape.
func TestArcSweepsBothWays(t *testing.T) {
	c, start, end := at(0, 0), at(4, 0), at(0, 4)

	ccw := NewArc(c, start, end, true, 64).Points()
	cw := NewArc(c, start, end, false, 64).Points()

	// Counter-clockwise from +X to +Y is the quarter through (+x,+y); the
	// clockwise one is the three quarters the other way round.
	if len(cw) <= len(ccw) {
		t.Errorf("clockwise gave %d points and counter-clockwise %d; "+
			"the long way round should need more", len(cw), len(ccw))
	}
	midCCW := ccw[len(ccw)/2]
	if midCCW.X <= 0 || midCCW.Y <= 0 {
		t.Errorf("the counter-clockwise midpoint %v is not in the +x+y quadrant", midCCW)
	}
	midCW := cw[len(cw)/2]
	if midCW.X >= 0 && midCW.Y >= 0 {
		t.Errorf("the clockwise midpoint %v took the short way round", midCW)
	}
	// Both still land where they were told to.
	for _, pts := range [][]geom.Vec2i{ccw, cw} {
		if pts[0] != start || pts[len(pts)-1] != end {
			t.Errorf("arc runs %v..%v, want %v..%v", pts[0], pts[len(pts)-1], start, end)
		}
	}
}

// A full sweep is a circle drawn as an arc: start and end coincide, and it
// must not collapse to a single point or to nothing.
func TestArcOfAFullTurnIsARing(t *testing.T) {
	c := at(0, 0)
	arc := NewArc(c, at(3, 0), at(3, 0), true, 32)
	pts := arc.Points()
	if len(pts) < 8 {
		t.Fatalf("a full-turn arc gave %d points", len(pts))
	}
	var minX, maxX int64 = 1 << 40, -(1 << 40)
	for _, p := range pts {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
	}
	if maxX-minX < sub(5) {
		t.Errorf("the ring spans %d subunits across, want about %d", maxX-minX, sub(6))
	}
}

func TestArcDegeneracy(t *testing.T) {
	c := at(0, 0)
	if NewArc(c, c, c, true, 16).Degenerate() != true {
		t.Error("an arc of no radius should be degenerate")
	}
	if NewArc(c, at(4, 0), at(4, 0), true, 16).Degenerate() {
		t.Error("a full-turn arc is a real shape, not a degenerate one")
	}
	if NewArc(c, at(4, 0), at(0, 4), true, 16).Degenerate() {
		t.Error("an ordinary quarter arc was called degenerate")
	}
}

// Segment count follows the sweep: a quarter arc at the same smoothness as a
// full circle needs a quarter of the segments, not the same number crammed in.
func TestArcSegmentsFollowTheSweep(t *testing.T) {
	c := at(0, 0)
	quarter := NewArc(c, at(4, 0), at(0, 4), true, 64)
	full := NewCircle(c, sub(4), 64)
	q, f := len(quarter.Points()), len(full.Points())
	if q >= f {
		t.Errorf("a quarter arc used %d points and a whole circle %d", q, f)
	}
	if q < f/8 {
		t.Errorf("a quarter arc used only %d points against the circle's %d — too coarse", q, f)
	}
}

// --- Ellipse --------------------------------------------------------------

func TestEllipseClosesAndSpansItsAxes(t *testing.T) {
	e := NewEllipse(at(0, 0), at(4, 0), sub(2), 32)
	if !e.Closed() {
		t.Error("an ellipse is a closed loop")
	}
	pts := e.Points()
	if len(pts) != 32 {
		t.Fatalf("got %d points, want the 32 asked for", len(pts))
	}
	var minX, maxX, minY, maxY int64 = 1 << 40, -(1 << 40), 1 << 40, -(1 << 40)
	for _, p := range pts {
		minX, maxX = min64(minX, p.X), max64(maxX, p.X)
		minY, maxY = min64(minY, p.Y), max64(maxY, p.Y)
	}
	if got := maxX - minX; absInt64(got-2*sub(4)) > 4 {
		t.Errorf("major span = %d subunits, want %d", got, 2*sub(4))
	}
	if got := maxY - minY; absInt64(got-2*sub(2)) > 4 {
		t.Errorf("minor span = %d subunits, want %d", got, 2*sub(2))
	}
}

// The major-axis endpoint is a point on the curve and a snap target, so it has
// to be exactly where it was placed.
func TestEllipsePassesThroughItsMajorEndpoint(t *testing.T) {
	e := NewEllipse(at(1, 1), at(5, 1), sub(2), 16)
	if got := e.Points()[0]; got != at(5, 1) {
		t.Errorf("first point = %v, want the major endpoint %v", got, at(5, 1))
	}
}

// A tilted ellipse is what the tool draws whenever the major axis is not
// horizontal, and getting the rotation wrong is invisible until something is
// extruded from it.
func TestEllipseRotatesWithItsMajorAxis(t *testing.T) {
	// Major axis along +Y: the wide direction should now be vertical.
	e := NewEllipse(at(0, 0), at(0, 4), sub(2), 32)
	var maxX, maxY int64
	for _, p := range e.Points() {
		maxX, maxY = max64(maxX, absInt64(p.X)), max64(maxY, absInt64(p.Y))
	}
	if maxY <= maxX {
		t.Errorf("reach = %d across, %d up; a +Y major axis should be taller than wide", maxX, maxY)
	}
	if absInt64(maxY-sub(4)) > 4 {
		t.Errorf("major reach = %d, want %d", maxY, sub(4))
	}
	if absInt64(maxX-sub(2)) > 4 {
		t.Errorf("minor reach = %d, want %d", maxX, sub(2))
	}
}

func TestEllipseDegeneracy(t *testing.T) {
	if !NewEllipse(at(0, 0), at(0, 0), sub(2), 16).Degenerate() {
		t.Error("an ellipse with no major axis should be degenerate")
	}
	if !NewEllipse(at(0, 0), at(4, 0), 0, 16).Degenerate() {
		t.Error("an ellipse with no minor axis should be degenerate")
	}
	if NewEllipse(at(0, 0), at(4, 0), sub(2), 16).Degenerate() {
		t.Error("an ordinary ellipse was called degenerate")
	}
}

// An ellipse whose axes are equal is a circle, and it must tessellate as one
// rather than drifting by a rounding.
func TestEqualAxesGiveACircle(t *testing.T) {
	e := NewEllipse(at(0, 0), at(3, 0), sub(3), 24)
	for i, p := range e.Points() {
		if got := p.Sub(at(0, 0)).Len(); math.Abs(got-float64(sub(3))) > 1.5 {
			t.Errorf("point %d is %.2f from the centre, want %d", i, got, sub(3))
		}
	}
}

// --- Both -----------------------------------------------------------------

// Curves are made of segments like everything else, and their provenance has
// to survive into the region engine or selection stops working on them.
func TestCurvesExpandToAttributedSegments(t *testing.T) {
	cases := []struct {
		name string
		ent  Entity
		want int // segments for an entity at index 3
	}{
		{"arc", NewArc(at(0, 0), at(4, 0), at(0, 4), true, 32), 0},
		{"ellipse", NewEllipse(at(0, 0), at(4, 0), sub(2), 16), 16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segs := tc.ent.AppendSegments(nil, 3)
			if len(segs) == 0 {
				t.Fatal("no segments")
			}
			if tc.want > 0 && len(segs) != tc.want {
				t.Errorf("got %d segments, want %d", len(segs), tc.want)
			}
			// An open curve has one fewer segment than points; a closed one has
			// the same number.
			pts := len(tc.ent.Points())
			wantSegs := pts
			if !tc.ent.Closed() {
				wantSegs = pts - 1
			}
			if len(segs) != wantSegs {
				t.Errorf("got %d segments for %d points (closed=%v), want %d",
					len(segs), pts, tc.ent.Closed(), wantSegs)
			}
			for _, s := range segs {
				if s.Src.Entity != 3 {
					t.Fatalf("a segment claims entity %d, want 3", s.Src.Entity)
				}
			}
		})
	}
}

// An arc joining two lines has to close the profile, which is the only thing
// that makes a curved outline extrudable.
func TestAnArcClosesAProfileWithLines(t *testing.T) {
	// A square with its top-right corner replaced by a quarter arc.
	_, _, s := sketchWith(t,
		NewLine(at(0, 0), at(4, 0)),
		NewLine(at(4, 0), at(4, 2)),
		NewArc(at(2, 2), at(4, 2), at(2, 4), true, 16),
		NewLine(at(2, 4), at(0, 4)),
		NewLine(at(0, 4), at(0, 0)),
	)
	arr := s.Arrangement()
	if len(arr.OpenEnds) != 0 {
		t.Errorf("the profile has %d open ends: an arc did not meet its lines", len(arr.OpenEnds))
	}
	if len(arr.Regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(arr.Regions))
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
