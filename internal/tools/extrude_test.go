package tools

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/csg"
	"modeler/internal/geom/extrude"
)

func newTool() *ExtrudeTool {
	return NewExtrudeTool(1, []int{0}, geom.Vec3{}, geom.AxisZ)
}

func TestExtrudeToolOpensWithAVisiblePreview(t *testing.T) {
	tool := newTool()
	if tool.DepthUnits != DefaultDepthUnits {
		t.Errorf("opening depth = %v, want %v", tool.DepthUnits, DefaultDepthUnits)
	}
	if ok, why := tool.Valid(); !ok {
		t.Errorf("a freshly opened tool is invalid: %s", why)
	}
	if tool.Flipped() {
		t.Error("a fresh tool starts flipped")
	}
}

func TestExtrudeToolNeedsARegion(t *testing.T) {
	tool := NewExtrudeTool(1, nil, geom.Vec3{}, geom.AxisZ)
	ok, why := tool.Valid()
	if ok {
		t.Fatal("extruding nothing was accepted")
	}
	if why == "" {
		t.Error("the refusal gave no reason, and silence is never the answer")
	}
}

func TestExtrudeToolNeedsDepth(t *testing.T) {
	tool := newTool()
	tool.SetDepth(0)
	if ok, why := tool.Valid(); ok {
		t.Error("a zero-depth extrude was accepted")
	} else if why == "" {
		t.Error("the refusal gave no reason")
	}
}

// TestDragThroughZeroFlipsTheExtrusion is the behaviour SPEC-UX §9.2 asks for:
// pull the arrow back past its origin and the solid grows the other way rather
// than inverting.
func TestDragThroughZeroFlipsTheExtrusion(t *testing.T) {
	tool := newTool()
	tool.Dir = extrude.Normal
	tool.SetDepth(3)

	p := tool.BuildParams(geom.PlaneFrame(geom.PlaneFront))
	if p.Dir != extrude.Normal {
		t.Errorf("a positive depth built %v, want Normal", p.Dir)
	}
	if p.Depth != geom.ToSubunits(3) {
		t.Errorf("depth = %d subunits, want %d", p.Depth, geom.ToSubunits(3))
	}

	tool.SetDepth(-2)
	if !tool.Flipped() {
		t.Fatal("a negative depth does not report as flipped")
	}
	p = tool.BuildParams(geom.PlaneFrame(geom.PlaneFront))
	if p.Dir != extrude.Reverse {
		t.Errorf("a negative depth built %v, want Reverse", p.Dir)
	}
	// The geometry only ever runs forwards, so the depth handed to it is
	// positive even though the drag was backwards.
	if p.Depth != geom.ToSubunits(2) {
		t.Errorf("depth = %d subunits, want a positive %d", p.Depth, geom.ToSubunits(2))
	}

	// The arrow visibly turns round.
	if got := tool.ArrowDirection(); got != (geom.Vec3{Z: -1}) {
		t.Errorf("flipped arrow points %v, want -Z", got)
	}
}

func TestFlippingReverseGivesNormal(t *testing.T) {
	tool := newTool()
	tool.Dir = extrude.Reverse
	tool.SetDepth(-2)
	if got := tool.BuildParams(geom.PlaneFrame(geom.PlaneFront)).Dir; got != extrude.Normal {
		t.Errorf("flipping a reverse extrude gave %v, want Normal", got)
	}
}

func TestSymmetricIsNotFlippedByASignChange(t *testing.T) {
	// Symmetric straddles the plane, so which way the drag went is irrelevant.
	tool := newTool()
	tool.Dir = extrude.Symmetric
	tool.SetDepth(-4)
	p := tool.BuildParams(geom.PlaneFrame(geom.PlaneFront))
	if p.Dir != extrude.Symmetric {
		t.Errorf("symmetric became %v after a negative drag", p.Dir)
	}
	if p.Depth != geom.ToSubunits(4) {
		t.Errorf("depth = %d, want %d", p.Depth, geom.ToSubunits(4))
	}
}

// TestArrowDragSnapsToTheGrid covers the snapping of SPEC-UX §9.2: whole units
// by default, quarter units with Ctrl, free with Alt.
func TestArrowDragSnapsToTheGrid(t *testing.T) {
	const unitsPerPixel = 0.05 // 20 px to the unit

	cases := []struct {
		name    string
		snap    geom.SnapStep
		movedPx float64
		want    float64
	}{
		{"grid rounds to whole units", geom.SnapGrid, 62, 4},         // 1 + 3.1 -> 4
		{"grid rounds down too", geom.SnapGrid, 48, 3},               // 1 + 2.4 -> 3
		{"fine snaps to quarters", geom.SnapFine, 65, 4.25},          // 1 + 3.25 exactly
		{"fine rounds to the nearest quarter", geom.SnapFine, 62, 4}, // 4.1 -> 4
		// Free is not truly free: it still lands on the subunit lattice,
		// because no tool may put an off-lattice value into the document
		// (SPEC-GEOMETRY §1.2). A tenth of a unit is not a subunit multiple.
		{"free keeps the lattice", geom.SnapNone, 62, 1050.0 / geom.Unit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := newTool() // opens at depth 1
			tool.BeginDrag(0)
			if !tool.Dragging() {
				t.Fatal("BeginDrag did not start a drag")
			}
			tool.UpdateDrag(c.movedPx, unitsPerPixel, c.snap)
			if math.Abs(tool.DepthUnits-c.want) > 1e-9 {
				t.Errorf("depth = %v, want %v", tool.DepthUnits, c.want)
			}
			tool.EndDrag()
			if tool.Dragging() {
				t.Error("EndDrag did not stop the drag")
			}
		})
	}
}

// TestDragIsAbsoluteNotAccumulated keeps the drag from drifting: the depth is
// always the anchor plus the total travel, never a running sum of steps.
func TestDragIsAbsoluteNotAccumulated(t *testing.T) {
	tool := newTool()
	tool.BeginDrag(100)

	// Wander out and come back to where the drag began.
	for _, px := range []float64{140, 220, 60, 100} {
		tool.UpdateDrag(px, 0.05, geom.SnapNone)
	}
	if math.Abs(tool.DepthUnits-DefaultDepthUnits) > 1e-9 {
		t.Errorf("returning to the start left the depth at %v, want %v",
			tool.DepthUnits, DefaultDepthUnits)
	}
}

func TestUpdateDragDoesNothingWithoutABeginDrag(t *testing.T) {
	tool := newTool()
	before := tool.DepthUnits
	tool.UpdateDrag(500, 0.05, geom.SnapGrid)
	if tool.DepthUnits != before {
		t.Error("a drag update landed without a drag having started")
	}
}

func TestFlipButton(t *testing.T) {
	tool := newTool()
	tool.SetDepth(3)
	tool.Flip()
	if tool.DepthUnits != -3 {
		t.Errorf("after flip, depth = %v, want -3", tool.DepthUnits)
	}
	tool.Flip()
	if tool.DepthUnits != 3 {
		t.Errorf("flipping twice gave %v, want 3", tool.DepthUnits)
	}
}

// TestThroughAllTakesOverTheDepth covers SPEC-UX §9.3: the toggle replaces the
// dragged depth with one the scene decided, without disturbing the drag itself,
// so turning it off again returns to exactly the depth you had.
func TestThroughAllTakesOverTheDepth(t *testing.T) {
	tool := newTool()
	tool.SetDepth(3)
	tool.ThroughDepth = 40

	if got := tool.EffectiveDepth(); got != 3 {
		t.Errorf("with the toggle off, depth = %v, want the dragged 3", got)
	}

	tool.ThroughAll = true
	if got := tool.EffectiveDepth(); got != 40 {
		t.Errorf("through-all depth = %v, want the scene's 40", got)
	}
	if got := tool.BuildParams(geom.PlaneFrame(geom.PlaneFront)).Depth; got != geom.ToSubunits(40) {
		t.Errorf("built depth = %d subunits, want %d", got, geom.ToSubunits(40))
	}
	if got := tool.DepthLabel(); got != "40" {
		t.Errorf("the field reads %q, want the scene's depth", got)
	}

	// Symmetric splits the run either side of the plane, so clearing the scene
	// both ways takes twice the reach.
	tool.Dir = extrude.Symmetric
	if got := tool.EffectiveDepth(); got != 80 {
		t.Errorf("symmetric through-all depth = %v, want 80", got)
	}

	tool.Dir = extrude.Normal
	tool.ThroughAll = false
	if tool.EffectiveDepth() != 3 {
		t.Error("turning the toggle off lost the dragged depth")
	}
}

// TestThroughAllStillFlips: the toggle decides how far, never which way.
func TestThroughAllStillFlips(t *testing.T) {
	tool := newTool()
	tool.ThroughDepth = 40
	tool.ThroughAll = true
	tool.SetDepth(-1)

	if !tool.Flipped() {
		t.Fatal("a negative drag with through-all on does not report as flipped")
	}
	p := tool.BuildParams(geom.PlaneFrame(geom.PlaneFront))
	if p.Dir != extrude.Reverse {
		t.Errorf("direction = %v, want Reverse", p.Dir)
	}
	if p.Depth != geom.ToSubunits(40) {
		t.Errorf("depth = %d, want the full through-all run", p.Depth)
	}
	if got := tool.ArrowDirection(); got != (geom.Vec3{Z: -1}) {
		t.Errorf("arrow points %v, want -Z", got)
	}
}

// TestThroughAllWithNothingToRunThroughSaysSo keeps the disabled-control rule of
// SPEC-UX §15: refusing without a reason is never allowed.
func TestThroughAllWithNothingToRunThroughSaysSo(t *testing.T) {
	tool := newTool()
	tool.SetDepth(3)
	tool.ThroughAll = true
	tool.ThroughDepth = 0

	ok, why := tool.Valid()
	if ok {
		t.Fatal("a zero-length through-all extrude was accepted")
	}
	if !contains(why, "Through all") {
		t.Errorf("the reason %q does not name the toggle that caused it", why)
	}
}

func TestDepthAndDraftAreClamped(t *testing.T) {
	tool := newTool()
	tool.SetDepth(1e9)
	if tool.DepthUnits != MaxDepthUnits {
		t.Errorf("depth clamped to %v, want %v", tool.DepthUnits, MaxDepthUnits)
	}
	tool.SetDepth(-1e9)
	if tool.DepthUnits != -MaxDepthUnits {
		t.Errorf("depth clamped to %v, want %v", tool.DepthUnits, -MaxDepthUnits)
	}

	tool.SetDraft(90)
	if tool.Draft != extrude.MaxDraftDegrees {
		t.Errorf("draft clamped to %v, want %v", tool.Draft, extrude.MaxDraftDegrees)
	}
	tool.SetDraft(-90)
	if tool.Draft != -extrude.MaxDraftDegrees {
		t.Errorf("draft clamped to %v, want %v", tool.Draft, -extrude.MaxDraftDegrees)
	}
}

// TestResultsNeedSomethingToCombineWith is the chip-enabling rule of
// SPEC-UX §9.4: New always works, and the other three are only offered when the
// extrude actually reaches a body to combine with.
func TestResultsNeedSomethingToCombineWith(t *testing.T) {
	combining := []Result{ResultAdd, ResultSubtract, ResultIntersect}

	// Nothing in reach: only New.
	if !ResultNew.Available(0) {
		t.Error("New should be available with nothing to combine with")
	}
	if ResultNew.UnavailableReason(0) != "" {
		t.Error("an available result should have no disabled reason")
	}
	for _, r := range combining {
		if r.Available(0) {
			t.Errorf("%v is available with no body in reach", r)
		}
		why := r.UnavailableReason(0)
		if why == "" {
			t.Errorf("%v gives no reason for being disabled", r)
		}
		if !contains(why, r.String()) {
			t.Errorf("%v's reason %q does not name the chip it is about", r, why)
		}
	}

	// A body in reach turns them all on.
	for _, r := range append(combining, ResultNew) {
		if !r.Available(1) {
			t.Errorf("%v is still disabled with a body in reach", r)
		}
		if r.UnavailableReason(1) != "" {
			t.Errorf("%v still gives a disabled reason when it is enabled", r)
		}
	}
}

// TestResultOps maps each chip onto the boolean it runs.
func TestResultOps(t *testing.T) {
	want := map[Result]struct {
		op    csg.Op
		needs bool
	}{
		ResultNew:       {csg.Union, false},
		ResultAdd:       {csg.Union, true},
		ResultSubtract:  {csg.Subtract, true},
		ResultIntersect: {csg.Intersect, true},
	}
	for r, w := range want {
		op, combining := r.Op()
		if combining != w.needs {
			t.Errorf("%v combining = %v, want %v", r, combining, w.needs)
		}
		if combining && op != w.op {
			t.Errorf("%v runs %v, want %v", r, op, w.op)
		}
		if r.NeedsTarget() != w.needs {
			t.Errorf("%v NeedsTarget = %v, want %v", r, r.NeedsTarget(), w.needs)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestAxisDistanceGrabsTheArrow covers the gizmo hit test: near the shaft is a
// grab, well off it is not, and the projection keeps running past the tip so a
// long drag keeps extruding.
func TestAxisDistanceGrabsTheArrow(t *testing.T) {
	// A 90 px arrow running up the screen from (100, 300).
	const ax, ay, bx, by = 100.0, 300.0, 100.0, 210.0

	dist, along := AxisDistancePx(104, 250, ax, ay, bx, by)
	if dist != 4 {
		t.Errorf("distance = %v, want 4", dist)
	}
	if math.Abs(along-50) > 1e-9 {
		t.Errorf("along = %v, want 50", along)
	}
	if dist > ArrowGrabRadiusPx {
		t.Error("a point 4 px from the shaft did not count as a grab")
	}

	if dist, _ := AxisDistancePx(140, 250, ax, ay, bx, by); dist <= ArrowGrabRadiusPx {
		t.Error("a point 40 px away counted as a grab")
	}

	// Past the tip the projection keeps growing rather than clamping, so the
	// drag can pull the extrusion far beyond the arrow's drawn length.
	if _, along := AxisDistancePx(100, 100, ax, ay, bx, by); along <= 90 {
		t.Errorf("dragging past the tip gave along = %v, want more than the arrow's 90", along)
	}
	// And behind the origin it goes negative, which is what flips the extrusion.
	if _, along := AxisDistancePx(100, 350, ax, ay, bx, by); along >= 0 {
		t.Errorf("dragging behind the origin gave along = %v, want negative", along)
	}

	// A degenerate arrow does not divide by zero.
	if dist, along := AxisDistancePx(105, 300, ax, ay, ax, ay); dist != 5 || along != 0 {
		t.Errorf("degenerate arrow gave dist=%v along=%v", dist, along)
	}
}

func TestResultNames(t *testing.T) {
	want := map[Result]string{
		ResultNew: "New", ResultAdd: "Add",
		ResultSubtract: "Subtract", ResultIntersect: "Intersect",
	}
	for r, name := range want {
		if r.String() != name {
			t.Errorf("%d.String() = %q, want %q", r, r.String(), name)
		}
	}
}

func TestDepthLabel(t *testing.T) {
	tool := newTool()
	tool.SetDepth(6)
	if got := tool.DepthLabel(); got != "6" {
		t.Errorf("depth label = %q, want \"6\"", got)
	}
	tool.SetDepth(2.5)
	if got := tool.DepthLabel(); got != "2.5" {
		t.Errorf("depth label = %q, want \"2.5\"", got)
	}
}

// TestSubtractOnAFaceSketchAimsIntoTheBody is the user's report of
// 2026-08-27: "subtract in extruding on a sketch on a model is not working".
//
// A sketch on a face opens with Result=Add and the arrow pointing outward,
// which is right for adding. Clicking Subtract left the arrow pointing the
// same way — so the solid sat against the outside of the body, the boolean
// ran, and it took nothing away. Choosing "cut this out of the body I am
// drawn on" has to mean cutting into it.
func TestSubtractOnAFaceSketchAimsIntoTheBody(t *testing.T) {
	frame := geom.PlaneFrame(geom.PlaneTop)

	faceTool := func() *ExtrudeTool {
		e := NewExtrudeTool(1, []int{0}, geom.Vec3{}, frame.N)
		e.OnFace = true
		e.Result = ResultAdd // what BeginExtrude sets for a face sketch
		return e
	}

	// Add points outward, away from the body: that is what growing means.
	add := faceTool()
	if got := add.BuildParams(frame).Dir; got != extrude.Normal {
		t.Errorf("Add on a face builds %v, want Normal (outward)", got)
	}

	// Switching to Subtract turns it round.
	cut := faceTool()
	cut.SetResult(ResultSubtract)
	if got := cut.BuildParams(frame).Dir; got != extrude.Reverse {
		t.Errorf("Subtract on a face builds %v, want Reverse (into the body)", got)
	}
	// The depth itself is untouched in size — only its sense changed.
	if got := cut.EffectiveDepth(); got != DefaultDepthUnits {
		t.Errorf("depth became %v, want the %v it was", got, DefaultDepthUnits)
	}

	// Switching back points it out again.
	cut.SetResult(ResultAdd)
	if got := cut.BuildParams(frame).Dir; got != extrude.Normal {
		t.Errorf("Add again builds %v, want Normal", got)
	}

	// The user's own direction is still theirs: an explicit flip after
	// choosing Subtract must survive.
	cut.SetResult(ResultSubtract)
	cut.Flip()
	if got := cut.BuildParams(frame).Dir; got != extrude.Normal {
		t.Errorf("after an explicit flip it builds %v, want the flip to hold", got)
	}
}

// A sketch on a default plane has no body to be inside, so nothing is guessed.
func TestPlaneSketchesKeepTheirDirection(t *testing.T) {
	frame := geom.PlaneFrame(geom.PlaneFront)
	e := NewExtrudeTool(1, []int{0}, geom.Vec3{}, frame.N)
	before := e.DepthUnits
	e.SetResult(ResultSubtract)
	if e.DepthUnits != before {
		t.Errorf("a plane sketch's depth changed from %v to %v on picking Subtract",
			before, e.DepthUnits)
	}
}
