package model

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// SK4's model layer: splines and beziers (Sketch_func.md §5 SK4).

// --- Spline ---------------------------------------------------------------

// A spline goes *through* the points you place. That is the whole difference
// between it and a bezier, and it is what makes the placed points snap targets
// like every other placed point (V-99).
func TestSplinePassesThroughEveryPlacedPoint(t *testing.T) {
	through := []geom.Vec2i{at(0, 0), at(3, 4), at(7, 1), at(10, 5)}
	s := NewSpline(through, false, 8)
	pts := s.Points()

	for _, want := range through {
		found := false
		for _, got := range pts {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the spline misses its placed point %v", want)
		}
	}
	if pts[0] != through[0] {
		t.Errorf("starts at %v, want %v", pts[0], through[0])
	}
	if pts[len(pts)-1] != through[len(through)-1] {
		t.Errorf("ends at %v, want %v", pts[len(pts)-1], through[len(through)-1])
	}
}

// Between the placed points it curves, or it is a polyline with extra steps.
func TestSplineCurvesBetweenItsPoints(t *testing.T) {
	// Three points making a corner: a spline should round it rather than
	// turning square.
	s := NewSpline([]geom.Vec2i{at(0, 0), at(4, 0), at(4, 4)}, false, 8)
	pts := s.Points()
	if len(pts) < 10 {
		t.Fatalf("the spline tessellated to %d points", len(pts))
	}
	// Curving means leaving the straight runs. A Catmull-Rom through a right
	// angle overshoots *outside* the corner rather than cutting inside it —
	// which way it bulges is the curve's business; that it bulges at all is
	// the difference between a spline and a polyline.
	onFirstRun := func(p geom.Vec2i) bool {
		return p.Y == 0 && p.X >= 0 && p.X <= sub(4)
	}
	onSecondRun := func(p geom.Vec2i) bool {
		return p.X == sub(4) && p.Y >= 0 && p.Y <= sub(4)
	}
	off := 0
	for _, p := range pts {
		if !onFirstRun(p) && !onSecondRun(p) {
			off++
		}
	}
	if off == 0 {
		t.Error("every point lies on the two straight runs — this is a polyline, not a spline")
	}
}

func TestSplineOfTwoPointsIsALine(t *testing.T) {
	s := NewSpline([]geom.Vec2i{at(0, 0), at(4, 0)}, false, 8)
	pts := s.Points()
	if pts[0] != at(0, 0) || pts[len(pts)-1] != at(4, 0) {
		t.Errorf("runs %v..%v", pts[0], pts[len(pts)-1])
	}
	// Every point sits on the straight run between them.
	for i, p := range pts {
		if p.Y != 0 {
			t.Errorf("point %d is at y=%d, off the line between two points", i, p.Y)
		}
	}
}

// A closed spline is a loop, which is what makes a curved profile extrudable.
func TestClosedSplineMakesARegion(t *testing.T) {
	s := NewSpline([]geom.Vec2i{at(0, 0), at(4, -2), at(8, 0), at(4, 3)}, true, 8)
	if !s.Closed() {
		t.Fatal("a closed spline does not report itself closed")
	}
	_, _, sk := sketchWith(t, s)
	arr := sk.Arrangement()
	if len(arr.Regions) != 1 {
		t.Errorf("a closed spline made %d regions, want 1", len(arr.Regions))
	}
	if len(arr.OpenEnds) != 0 {
		t.Errorf("it left %d open ends", len(arr.OpenEnds))
	}
}

func TestSplineDegeneracy(t *testing.T) {
	if !NewSpline(nil, false, 8).Degenerate() {
		t.Error("a spline with no points should be degenerate")
	}
	if !NewSpline([]geom.Vec2i{at(1, 1)}, false, 8).Degenerate() {
		t.Error("a spline of one point should be degenerate")
	}
	if !NewSpline([]geom.Vec2i{at(1, 1), at(1, 1)}, false, 8).Degenerate() {
		t.Error("a spline whose points all coincide should be degenerate")
	}
	if NewSpline([]geom.Vec2i{at(0, 0), at(4, 0)}, false, 8).Degenerate() {
		t.Error("an ordinary two-point spline was called degenerate")
	}
}

// --- Bezier ---------------------------------------------------------------

// A cubic runs from the first control point to the last and is pulled toward
// the middle two without reaching them.
func TestBezierRunsBetweenItsOuterControls(t *testing.T) {
	b := NewBezier(at(0, 0), at(0, 4), at(8, 4), at(8, 0), 12)
	pts := b.Points()
	if pts[0] != at(0, 0) {
		t.Errorf("starts at %v, want %v", pts[0], at(0, 0))
	}
	if pts[len(pts)-1] != at(8, 0) {
		t.Errorf("ends at %v, want %v", pts[len(pts)-1], at(8, 0))
	}
	// It is pulled up toward the handles, but never as far as them.
	var maxY int64
	for _, p := range pts {
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	if maxY <= 0 {
		t.Error("the curve was not pulled toward its handles at all")
	}
	if maxY >= sub(4) {
		t.Errorf("the curve reached y=%d, as far as its handles — a cubic stays inside them", maxY)
	}
	// Symmetric controls give a symmetric curve.
	mid := pts[len(pts)/2]
	if math.Abs(float64(mid.X-sub(4))) > float64(sub(0.5)) {
		t.Errorf("the middle of a symmetric curve is at x=%d, want about %d", mid.X, sub(4))
	}
}

func TestBezierDegeneracy(t *testing.T) {
	p := at(2, 2)
	if !NewBezier(p, p, p, p, 12).Degenerate() {
		t.Error("a bezier with every control in one place should be degenerate")
	}
	if NewBezier(at(0, 0), at(1, 2), at(3, 2), at(4, 0), 12).Degenerate() {
		t.Error("an ordinary bezier was called degenerate")
	}
}

// --- Both -----------------------------------------------------------------

func TestCurveControlPointsRoundTrip(t *testing.T) {
	cases := []Entity{
		NewSpline([]geom.Vec2i{at(0, 0), at(2, 3), at(5, 1)}, false, 8),
		NewSpline([]geom.Vec2i{at(0, 0), at(2, 3), at(5, 1)}, true, 8),
		NewBezier(at(0, 0), at(1, 2), at(3, 2), at(4, 0), 12),
	}
	for _, e := range cases {
		if len(e.Pts) == 0 {
			t.Errorf("%v carries no control points", e.Kind)
		}
		if !e.Equal(e) {
			t.Errorf("%v does not equal itself", e.Kind)
		}
	}
}

// Denser subdivision means a smoother curve, and the clamp keeps a spline from
// burying the region engine (measured in BenchmarkBuildLoop).
func TestSplineSubdivisionIsClamped(t *testing.T) {
	pts := []geom.Vec2i{at(0, 0), at(4, 4), at(8, 0)}
	coarse := len(NewSpline(pts, false, 2).Points())
	fine := len(NewSpline(pts, false, 16).Points())
	if fine <= coarse {
		t.Errorf("16 subdivisions gave %d points and 2 gave %d", fine, coarse)
	}
	if got := len(NewSpline(pts, false, 999).Points()); got != fine {
		t.Errorf("999 subdivisions gave %d points, want the same %d as the ceiling", got, fine)
	}
	if got := len(NewSpline(pts, false, 0).Points()); got != coarse {
		t.Errorf("0 subdivisions gave %d points, want the same %d as the floor", got, coarse)
	}
}
