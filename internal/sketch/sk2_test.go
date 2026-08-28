package sketch

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// SK2's session layer: the 3-point circle, the three arc gestures and the
// ellipse (Sketch_func.md §5 SK2). Written before the code they describe.

// --- Circumcentre ---------------------------------------------------------

// The shared arithmetic behind the 3-point circle and the 3-point arc. It is
// tested on its own because both gestures are only as good as it is, and a
// wrong centre is a shape that looks nearly right.
func TestCircumcentreOfThreePoints(t *testing.T) {
	cases := []struct {
		name    string
		a, b, c geom.Vec2i
		want    geom.Vec2i
	}{
		{"right angle at the origin", at(4, 0), at(0, 4), at(-4, 0), at(0, 0)},
		{"offset circle", at(5, 1), at(1, 5), at(-3, 1), at(1, 1)},
		{"order does not matter", at(0, 4), at(-4, 0), at(4, 0), at(0, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Circumcentre(tc.a, tc.b, tc.c)
			if !ok {
				t.Fatal("three points on a circle were called collinear")
			}
			if d := got.Sub(tc.want).Len(); d > 1.5 {
				t.Errorf("centre = %v, want %v (%.2f subunits out)", got, tc.want, d)
			}
		})
	}
}

// Three points in a line have no circle, and the tool must say so rather than
// dividing by nearly zero and drawing something enormous.
func TestCircumcentreRefusesCollinearPoints(t *testing.T) {
	if _, ok := Circumcentre(at(0, 0), at(2, 0), at(5, 0)); ok {
		t.Error("three collinear points produced a centre")
	}
	if _, ok := Circumcentre(at(0, 0), at(2, 2), at(4, 4)); ok {
		t.Error("three points on a diagonal produced a centre")
	}
	if _, ok := Circumcentre(at(1, 1), at(1, 1), at(3, 3)); ok {
		t.Error("two coincident points produced a centre")
	}
}

// --- 3-point circle -------------------------------------------------------

func TestThreePointCircleUsesTheCircumcentre(t *testing.T) {
	s := session(ToolCircle3)
	if got := s.Click(at(4, 0)); got.Commit {
		t.Fatal("click 1 committed")
	}
	if got := s.Click(at(0, 4)); got.Commit {
		t.Fatal("click 2 committed")
	}
	ents := s.Click(at(-4, 0)).Committed()
	if len(ents) != 1 || ents[0].Kind != model.EntCircle {
		t.Fatalf("click 3 produced %+v, want one circle", ents)
	}
	e := ents[0]
	if d := e.C.Sub(at(0, 0)).Len(); d > 1.5 {
		t.Errorf("centre = %v, want the origin", e.C)
	}
	if math.Abs(float64(e.R-sub(4))) > 1.5 {
		t.Errorf("radius = %d, want %d", e.R, sub(4))
	}
}

func TestThreePointCircleRefusesALine(t *testing.T) {
	s := session(ToolCircle3)
	s.Click(at(0, 0))
	s.Click(at(2, 0))
	got := s.Click(at(4, 0))
	if got.Commit {
		t.Fatal("three points in a line made a circle")
	}
	if got.Rejected == "" {
		t.Error("the refusal said nothing about why")
	}
	if s.Drawing() {
		t.Error("a refused third click left the gesture open")
	}
}

// --- Centre arc -----------------------------------------------------------

func TestCentreArcSweepsTowardTheThirdClick(t *testing.T) {
	s := session(ToolArcCenter)
	s.Click(at(0, 0)) // centre
	s.Click(at(4, 0)) // start
	ents := s.Click(at(0, 4)).Committed()
	if len(ents) != 1 || ents[0].Kind != model.EntArc {
		t.Fatalf("produced %+v, want one arc", ents)
	}
	e := ents[0]
	if e.C != at(0, 0) || e.A != at(4, 0) {
		t.Errorf("arc centre/start = %v/%v, want %v/%v", e.C, e.A, at(0, 0), at(4, 0))
	}
	// Clicking into the +x+y quadrant means the short way round, which is
	// counter-clockwise from +X.
	if !e.CCW() {
		t.Error("the arc swept clockwise, the long way round to the third click")
	}
	pts := e.Points()
	if pts[len(pts)-1].Sub(at(0, 4)).Len() > 1.5 {
		t.Errorf("the arc ends at %v, want near the third click %v", pts[len(pts)-1], at(0, 4))
	}
}

// The same first two clicks with the third on the other side must sweep the
// other way: the direction is the gesture's, not a fixed convention.
func TestCentreArcTakesTheOtherWayRound(t *testing.T) {
	s := session(ToolArcCenter)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	ents := s.Click(at(0, -4)).Committed()
	if len(ents) != 1 {
		t.Fatalf("produced %+v", ents)
	}
	if ents[0].CCW() {
		t.Error("a third click below the axis still swept counter-clockwise")
	}
}

// --- 3-point arc ----------------------------------------------------------

// Start, end, then a point the arc has to pass through: the bulge follows the
// third click, which is the only thing that distinguishes it from a line.
func TestThreePointArcPassesThroughTheThirdClick(t *testing.T) {
	s := session(ToolArc3)
	s.Click(at(4, 0))  // start
	s.Click(at(-4, 0)) // end
	ents := s.Click(at(0, 4)).Committed()
	if len(ents) != 1 || ents[0].Kind != model.EntArc {
		t.Fatalf("produced %+v, want one arc", ents)
	}
	e := ents[0]
	if e.A != at(4, 0) || e.B != at(-4, 0) {
		t.Errorf("arc runs %v..%v, want %v..%v", e.A, e.B, at(4, 0), at(-4, 0))
	}
	// The arc has to bulge through the third point, not away from it.
	var best float64 = 1 << 40
	for _, p := range e.Points() {
		if d := p.Sub(at(0, 4)).Len(); d < best {
			best = d
		}
	}
	if best > float64(sub(0.5)) {
		t.Errorf("the arc's closest approach to the through-point is %.0f subunits", best)
	}
}

func TestThreePointArcRefusesALine(t *testing.T) {
	s := session(ToolArc3)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	if got := s.Click(at(2, 0)); got.Commit || got.Rejected == "" {
		t.Error("three collinear points made an arc, or refused silently")
	}
}

// --- Tangent arc ----------------------------------------------------------

// A tangent arc leaves an existing entity smoothly, which means its first step
// runs along that entity's direction rather than at an angle to it.
func TestTangentArcLeavesTheLineSmoothly(t *testing.T) {
	// A line running east, ending at (4,0).
	ents := []model.Entity{model.NewLine(at(0, 0), at(4, 0))}
	s := session(ToolArcTangent)
	s.SetContext(ents)

	if got := s.Click(at(4, 0)); got.Commit {
		t.Fatal("the first click committed; it only picks the endpoint")
	}
	got := s.Click(at(8, 4))
	made := got.Committed()
	if len(made) != 1 || made[0].Kind != model.EntArc {
		t.Fatalf("produced %+v (%q), want one arc", made, got.Rejected)
	}
	arc := made[0]
	if arc.A != at(4, 0) {
		t.Errorf("the arc starts at %v, want the endpoint it was hung off %v", arc.A, at(4, 0))
	}
	if arc.B != at(8, 4) {
		t.Errorf("the arc ends at %v, want the second click %v", arc.B, at(8, 4))
	}
	// Tangency: the first step of the arc continues east, along the line.
	pts := arc.Points()
	if len(pts) < 3 {
		t.Fatalf("the arc tessellated to %d points", len(pts))
	}
	step := pts[1].Sub(pts[0])
	if step.X <= 0 {
		t.Errorf("the arc's first step is %v — it does not leave along the line", step)
	}
	if angle := math.Abs(math.Atan2(float64(step.Y), float64(step.X))); angle > 0.35 {
		t.Errorf("the arc leaves at %.2f rad to the line, want tangent", angle)
	}
}

// Without an endpoint to hang off, there is nothing to be tangent to. Saying
// so beats guessing a direction (Sketch_func.md §6).
func TestTangentArcNeedsAnEndpoint(t *testing.T) {
	s := session(ToolArcTangent)
	s.SetContext([]model.Entity{model.NewLine(at(0, 0), at(4, 0))})
	got := s.Click(at(2, 3)) // nowhere near either end
	if got.Commit {
		t.Fatal("a tangent arc started in mid-air")
	}
	if got.Rejected == "" {
		t.Error("the refusal said nothing about why")
	}
	if s.Drawing() {
		t.Error("the refused click armed the gesture anyway")
	}
}

// --- Ellipse --------------------------------------------------------------

func TestEllipseTakesCentreThenAxes(t *testing.T) {
	s := session(ToolEllipse)
	if got := s.Click(at(0, 0)); got.Commit {
		t.Fatal("click 1 committed")
	}
	if got := s.Click(at(4, 0)); got.Commit {
		t.Fatal("click 2 committed")
	}
	ents := s.Click(at(0, 2)).Committed()
	if len(ents) != 1 || ents[0].Kind != model.EntEllipse {
		t.Fatalf("produced %+v, want one ellipse", ents)
	}
	e := ents[0]
	if e.C != at(0, 0) || e.A != at(4, 0) {
		t.Errorf("centre/major = %v/%v", e.C, e.A)
	}
	if math.Abs(float64(e.W-sub(2))) > 1.5 {
		t.Errorf("semi-minor = %d, want %d", e.W, sub(2))
	}
}

// The third click's distance is measured across the major axis, so clicking
// anywhere along a line parallel to it gives the same ellipse.
func TestEllipseMinorIsMeasuredPerpendicular(t *testing.T) {
	mk := func(third geom.Vec2i) model.Entity {
		s := session(ToolEllipse)
		s.Click(at(0, 0))
		s.Click(at(4, 0))
		return s.Click(third).Committed()[0]
	}
	a := mk(at(0, 2))
	b := mk(at(3, 2)) // further along the major axis, same distance across
	if a.W != b.W {
		t.Errorf("semi-minor came out %d and %d for the same perpendicular reach", a.W, b.W)
	}
}

func TestEllipseRefusesAFlatMinor(t *testing.T) {
	s := session(ToolEllipse)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	if got := s.Click(at(2, 0)); got.Commit || got.Rejected == "" {
		t.Error("an ellipse with no minor axis was accepted, or refused silently")
	}
}

// --- Shared ---------------------------------------------------------------

// Every SK2 gesture is multi-click, so every one has to unwind a click at a
// time and preview what it would commit.
func TestSK2GesturesStageAndPreview(t *testing.T) {
	cases := []struct {
		tool   Tool
		clicks []geom.Vec2i
		at     geom.Vec2i
	}{
		{ToolCircle3, []geom.Vec2i{at(4, 0), at(0, 4)}, at(-4, 0)},
		{ToolArcCenter, []geom.Vec2i{at(0, 0), at(4, 0)}, at(0, 4)},
		{ToolArc3, []geom.Vec2i{at(4, 0), at(-4, 0)}, at(0, 4)},
		{ToolEllipse, []geom.Vec2i{at(0, 0), at(4, 0)}, at(0, 2)},
	}
	for _, tc := range cases {
		s := session(tc.tool)
		for _, p := range tc.clicks {
			s.Click(p)
		}
		if !s.Drawing() {
			t.Errorf("%v: not mid-gesture after %d clicks", tc.tool, len(tc.clicks))
		}
		if got := s.PreviewAt(tc.at); !got.Show {
			t.Errorf("%v: no preview before the last click", tc.tool)
		}
		// One Esc per placed point, then the tool is idle.
		for i := len(tc.clicks); i > 0; i-- {
			if got := s.Escape(); got != EscapeCancelledDraw {
				t.Errorf("%v: Esc %d returned %v", tc.tool, i, got)
			}
		}
		if s.Drawing() {
			t.Errorf("%v: still drawing after unwinding every point", tc.tool)
		}
	}
}
