package sketch

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// SK5's modify tools (Sketch_func.md §5 SK5), tested as pure geometry before
// any of them is wired to a button.
//
// These are the operations that edit what is already drawn, and they are the
// ones where a small error is invisible: a fillet whose arc is a subunit off
// its lines leaves two hairline gaps that turn a closed profile into four open
// ends, and nothing on screen says so until an extrude refuses.

// --- Fillet ---------------------------------------------------------------

func TestFilletMeetsBothLinesExactly(t *testing.T) {
	// A right angle at the origin: one line coming in along +X, one going up.
	a := model.NewLine(at(-4, 0), at(0, 0))
	b := model.NewLine(at(0, 0), at(0, 4))

	res, err := Fillet(a, b, sub(1))
	if err != nil {
		t.Fatalf("Fillet: %v", err)
	}
	if len(res.Lines) != 2 || res.Arc.Kind != model.EntArc {
		t.Fatalf("got %d lines and a %v, want 2 lines and an arc",
			len(res.Lines), res.Arc.Kind)
	}

	// The arc's ends are exactly the trimmed lines' ends: no gap, no overlap.
	arcPts := res.Arc.Points()
	start, end := arcPts[0], arcPts[len(arcPts)-1]
	if res.Lines[0].B != start {
		t.Errorf("the first line ends at %v and the arc starts at %v", res.Lines[0].B, start)
	}
	if res.Lines[1].A != end {
		t.Errorf("the arc ends at %v and the second line starts at %v", end, res.Lines[1].A)
	}
	// The far ends are untouched: a fillet trims the corner, not the lines.
	if res.Lines[0].A != at(-4, 0) || res.Lines[1].B != at(0, 4) {
		t.Errorf("the far ends moved: %v and %v", res.Lines[0].A, res.Lines[1].B)
	}
	// A radius-1 fillet on a right angle trims exactly 1 unit off each leg.
	if res.Lines[0].B != at(-1, 0) {
		t.Errorf("the first line was trimmed to %v, want %v", res.Lines[0].B, at(-1, 0))
	}
	if res.Lines[1].A != at(0, 1) {
		t.Errorf("the second line starts at %v, want %v", res.Lines[1].A, at(0, 1))
	}
}

// The arc has to be tangent — that is what makes it a fillet and not a
// chamfer with extra steps. Its centre sits a radius from both legs.
func TestFilletArcIsTangentToBothLegs(t *testing.T) {
	a := model.NewLine(at(-6, 0), at(0, 0))
	b := model.NewLine(at(0, 0), at(0, 6))
	res, err := Fillet(a, b, sub(2))
	if err != nil {
		t.Fatal(err)
	}
	// The legs run out to −X and +Y, so the corner opens into that quadrant and
	// the centre sits there: a radius in from each leg, at (−2,2).
	if d := res.Arc.C.Sub(at(-2, 2)).Len(); d > 1.5 {
		t.Errorf("the arc centre is %v, want %v — inside the corner the legs make",
			res.Arc.C, at(-2, 2))
	}
	// Tangency stated directly: the centre is exactly a radius from each leg's
	// line, which for these two is |x| and |y|.
	if got := absI(res.Arc.C.Y); got != sub(2) {
		t.Errorf("the centre is %d from the horizontal leg, want %d", got, sub(2))
	}
	if got := absI(res.Arc.C.X); got != sub(2) {
		t.Errorf("the centre is %d from the vertical leg, want %d", got, sub(2))
	}
	for i, p := range res.Arc.Points() {
		if d := math.Abs(p.Sub(res.Arc.C).Len() - float64(sub(2))); d > 1.5 {
			t.Errorf("arc point %d is %.1f subunits off the radius", i, d)
		}
	}
}

// A radius bigger than the legs cannot fit, and saying so with the size that
// would is more use than silently clamping or drawing a wrong shape.
func TestFilletRefusesARadiusThatDoesNotFit(t *testing.T) {
	a := model.NewLine(at(-1, 0), at(0, 0))
	b := model.NewLine(at(0, 0), at(0, 1))
	_, err := Fillet(a, b, sub(5))
	if err == nil {
		t.Fatal("a fillet larger than its legs was accepted")
	}
	if got := err.Error(); got == "" {
		t.Error("the refusal said nothing")
	}
}

func TestFilletRefusesLinesThatDoNotMeet(t *testing.T) {
	a := model.NewLine(at(-4, 0), at(-1, 0))
	b := model.NewLine(at(1, 0), at(4, 0))
	if _, err := Fillet(a, b, sub(1)); err == nil {
		t.Error("two lines with no shared corner were filleted")
	}
}

func TestFilletRefusesParallelLines(t *testing.T) {
	a := model.NewLine(at(-4, 0), at(0, 0))
	b := model.NewLine(at(0, 0), at(4, 0))
	if _, err := Fillet(a, b, sub(1)); err == nil {
		t.Error("a straight line was filleted")
	}
}

// --- Chamfer --------------------------------------------------------------

func TestChamferCutsBothLegsEqually(t *testing.T) {
	a := model.NewLine(at(-4, 0), at(0, 0))
	b := model.NewLine(at(0, 0), at(0, 4))

	res, err := Chamfer(a, b, sub(1))
	if err != nil {
		t.Fatalf("Chamfer: %v", err)
	}
	if len(res.Lines) != 3 {
		t.Fatalf("got %d lines, want 2 trimmed legs and the cut across", len(res.Lines))
	}
	// Both legs trimmed by the same distance, and the third line joins them.
	if res.Lines[0].B != at(-1, 0) || res.Lines[1].A != at(0, 1) {
		t.Errorf("legs trimmed to %v and %v", res.Lines[0].B, res.Lines[1].A)
	}
	cut := res.Lines[2]
	if cut.A != at(-1, 0) || cut.B != at(0, 1) {
		t.Errorf("the chamfer runs %v..%v, want %v..%v", cut.A, cut.B, at(-1, 0), at(0, 1))
	}
}

// --- Mirror ---------------------------------------------------------------

// Reflection is exact in integers, which is the whole reason a mirrored hull
// half meets its original without a seam.
func TestMirrorIsExact(t *testing.T) {
	// Mirror about the vertical axis through the origin.
	axisA, axisB := at(0, -5), at(0, 5)
	e := model.NewLine(at(2, 1), at(4, 3))

	got := MirrorEntity(e, axisA, axisB)
	if got.A != at(-2, 1) || got.B != at(-4, 3) {
		t.Errorf("mirrored line = %v..%v, want %v..%v", got.A, got.B, at(-2, 1), at(-4, 3))
	}
	// Mirroring twice is the identity: no drift, no rounding creep.
	back := MirrorEntity(got, axisA, axisB)
	if !back.Equal(e) {
		t.Errorf("mirroring twice gave %+v, want the original %+v", back, e)
	}
}

// Every kind mirrors, including the ones that carry a point list or a sweep
// direction. An arc that mirrors without flipping its winding comes out
// bulging the wrong way.
func TestMirrorHandlesEveryKind(t *testing.T) {
	axisA, axisB := at(0, -5), at(0, 5)
	cases := []model.Entity{
		model.NewLine(at(1, 1), at(3, 2)),
		model.NewRect(at(1, 1), at(3, 3)),
		model.NewCircle(at(2, 2), sub(1), 16),
		model.NewPoint(at(3, 1)),
		model.NewArc(at(2, 0), at(4, 0), at(2, 2), true, 16),
		model.NewEllipse(at(3, 0), at(5, 0), sub(1), 16),
		model.NewPolygon(at(2, 2), at(4, 2), 6),
		model.NewSlot(at(1, 0), at(4, 0), sub(1), 8),
		model.NewSpline([]geom.Vec2i{at(1, 0), at(3, 2), at(5, 0)}, false, 8),
		model.NewBezier(at(1, 0), at(2, 2), at(4, 2), at(5, 0), 8),
	}
	for _, e := range cases {
		m := MirrorEntity(e, axisA, axisB)
		if m.Kind != e.Kind {
			t.Errorf("%v mirrored into a %v", e.Kind, m.Kind)
			continue
		}
		// Every point of the mirrored shape is the reflection of a point of the
		// original, in reverse order for the closed ones.
		orig, got := e.Points(), m.Points()
		if len(orig) != len(got) {
			t.Errorf("%v: %d points became %d", e.Kind, len(orig), len(got))
			continue
		}
		// The mirror of the whole shape spans the same distance from the axis.
		var maxOrig, maxGot int64
		for i := range orig {
			if v := orig[i].X; v > maxOrig {
				maxOrig = v
			}
			if v := -got[i].X; v > maxGot {
				maxGot = v
			}
		}
		if maxOrig != maxGot {
			t.Errorf("%v reaches %d from the axis, its mirror %d", e.Kind, maxOrig, maxGot)
		}
		// Mirroring twice returns the original exactly.
		if back := MirrorEntity(m, axisA, axisB); !back.Equal(e) {
			t.Errorf("%v did not survive two mirrors", e.Kind)
		}
	}
}

// --- Linear pattern -------------------------------------------------------

func TestLinearPatternSpacesCopiesEvenly(t *testing.T) {
	e := model.NewCircle(at(0, 0), sub(1), 16)
	got := LinearPattern([]model.Entity{e}, at(3, 0), 4)
	// Four copies means three new ones: the original stays put.
	if len(got) != 3 {
		t.Fatalf("a count of 4 produced %d new entities, want 3", len(got))
	}
	for i, c := range got {
		want := at(float64(3*(i+1)), 0)
		if c.C != want {
			t.Errorf("copy %d is centred at %v, want %v", i, c.C, want)
		}
	}
}

func TestLinearPatternOfOneMakesNothing(t *testing.T) {
	e := model.NewCircle(at(0, 0), sub(1), 16)
	if got := LinearPattern([]model.Entity{e}, at(3, 0), 1); len(got) != 0 {
		t.Errorf("a count of 1 produced %d copies, want none", len(got))
	}
}

// --- Circular pattern -----------------------------------------------------

func TestCircularPatternWalksTheFullTurn(t *testing.T) {
	// A circle 4 units out from the origin, repeated four times round.
	e := model.NewCircle(at(4, 0), sub(1), 16)
	got := CircularPattern([]model.Entity{e}, at(0, 0), 4, 360)
	if len(got) != 3 {
		t.Fatalf("4 round produced %d new entities, want 3", len(got))
	}
	// Quarter turns: (0,4), (-4,0), (0,-4).
	wants := []geom.Vec2i{at(0, 4), at(-4, 0), at(0, -4)}
	for i, c := range got {
		if d := c.C.Sub(wants[i]).Len(); d > 1.5 {
			t.Errorf("copy %d is centred at %v, want %v", i, c.C, wants[i])
		}
		// Every copy stays the same distance from the pivot.
		if d := math.Abs(c.C.Sub(at(0, 0)).Len() - float64(sub(4))); d > 1.5 {
			t.Errorf("copy %d drifted to %.1f from the pivot", i, c.C.Sub(at(0, 0)).Len())
		}
	}
}

// A partial sweep spreads the copies across it rather than round the whole
// circle, which is how a row of portholes along a curved hull is made.
func TestCircularPatternHonoursAPartialSweep(t *testing.T) {
	e := model.NewCircle(at(4, 0), sub(1), 16)
	got := CircularPattern([]model.Entity{e}, at(0, 0), 3, 90)
	if len(got) != 2 {
		t.Fatalf("3 across 90° produced %d new entities, want 2", len(got))
	}
	// 3 copies across 90° puts them at 0°, 45° and 90°.
	if d := got[1].C.Sub(at(0, 4)).Len(); d > 1.5 {
		t.Errorf("the last copy is at %v, want the far end of the sweep %v", got[1].C, at(0, 4))
	}
}

// --- Offset ---------------------------------------------------------------

// The simplest case that proves the maths: a rectangle offset outward is a
// bigger rectangle, by exactly the distance asked for on every side.
func TestOffsetGrowsARectangle(t *testing.T) {
	loop := []geom.Vec2i{at(0, 0), at(6, 0), at(6, 4), at(0, 4)}
	got, err := OffsetLoop(loop, sub(1))
	if err != nil {
		t.Fatalf("OffsetLoop: %v", err)
	}
	if len(got) != len(loop) {
		t.Fatalf("offset gave %d corners, want %d", len(got), len(loop))
	}
	want := []geom.Vec2i{at(-1, -1), at(7, -1), at(7, 5), at(-1, 5)}
	for i := range want {
		if d := got[i].Sub(want[i]).Len(); d > 1.5 {
			t.Errorf("corner %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestOffsetShrinksInward(t *testing.T) {
	loop := []geom.Vec2i{at(0, 0), at(6, 0), at(6, 4), at(0, 4)}
	got, err := OffsetLoop(loop, -sub(1))
	if err != nil {
		t.Fatalf("OffsetLoop: %v", err)
	}
	want := []geom.Vec2i{at(1, 1), at(5, 1), at(5, 3), at(1, 3)}
	for i := range want {
		if d := got[i].Sub(want[i]).Len(); d > 1.5 {
			t.Errorf("corner %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// An inward offset larger than the shape turns it inside out. Refusing beats
// emitting a bowtie that the region engine will make nonsense of
// (Sketch_func.md §6).
func TestOffsetRefusesWhenItWouldTurnInsideOut(t *testing.T) {
	loop := []geom.Vec2i{at(0, 0), at(6, 0), at(6, 4), at(0, 4)}
	if _, err := OffsetLoop(loop, -sub(3)); err == nil {
		t.Error("an offset that collapses the shape was accepted")
	}
}

// A fillet is the *minor* arc between its two tangent points — the short way
// across the corner. Taking the long way round produces an arc that sweeps
// 270° out past both legs: it looks absurd, and worse, it closes a region
// nobody drew, which turns a plain L into an extrudable shape.
func TestFilletTakesTheShortWayRound(t *testing.T) {
	// Every rotation of the same right-angle corner, so a winding rule that
	// happens to work for one orientation is caught in the others.
	corners := [][2]model.Entity{
		{model.NewLine(at(-4, 0), at(0, 0)), model.NewLine(at(0, 0), at(0, 4))},
		{model.NewLine(at(4, 0), at(0, 0)), model.NewLine(at(0, 0), at(0, 4))},
		{model.NewLine(at(-4, 0), at(0, 0)), model.NewLine(at(0, 0), at(0, -4))},
		{model.NewLine(at(4, 0), at(0, 0)), model.NewLine(at(0, 0), at(0, -4))},
		// And the orientation the sk5 script uses, which is where this was found.
		{model.NewLine(at(-4, -4), at(4, -4)), model.NewLine(at(4, -4), at(4, -1))},
	}
	for i, c := range corners {
		res, err := Fillet(c[0], c[1], sub(1))
		if err != nil {
			t.Fatalf("corner %d: %v", i, err)
		}
		pts := res.Arc.Points()
		// A quarter arc at 16 segments to the circle is 4 segments: 5 points.
		// The long way round would be 12 segments and 13 points.
		if len(pts) > 7 {
			t.Errorf("corner %d: the fillet arc has %d points — it took the long way round",
				i, len(pts))
		}
		// And it stays inside the box its own endpoints make, give or take the
		// bulge. A 270° arc leaves that box entirely.
		lo, hi := pts[0], pts[0]
		for _, p := range pts {
			lo.X, lo.Y = min64(lo.X, p.X), min64(lo.Y, p.Y)
			hi.X, hi.Y = max64(hi.X, p.X), max64(hi.Y, p.Y)
		}
		if hi.X-lo.X > sub(1.5) || hi.Y-lo.Y > sub(1.5) {
			t.Errorf("corner %d: the arc spans %dx%d subunits, far more than its radius",
				i, hi.X-lo.X, hi.Y-lo.Y)
		}
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
