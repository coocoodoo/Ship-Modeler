package sketch

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
)

func sub(v float64) int64        { return geom.ToSubunits(v) }
func at(x, y float64) geom.Vec2i { return geom.Vec2i{X: sub(x), Y: sub(y)} }

// cfg is a snap config at a zoom where one screen pixel spans 10 subunits, so
// the 7 px endpoint radius reaches 70 subunits — a quarter of a unit.
func cfg() Config { return DefaultConfig(10) }

func TestOffsetGridInferenceAndFreeSnap(t *testing.T) {
	c := cfg()
	c.GridOrigin = at(-2.5, 1.25)
	from := at(0.5, 0.25)
	for _, cursor := range []geom.Vec2i{at(4.4, 0.3), at(0.55, 4.1)} {
		got := Resolve(cursor, nil, &from, c)
		if !got.HasGuide() || (got.Point.X-c.GridOrigin.X)%c.GridStep != 0 || (got.Point.Y-c.GridOrigin.Y)%c.GridStep != 0 {
			t.Fatalf("inference left the shifted grid: %+v", got)
		}
		c.Suppressed = true
		if free := Resolve(cursor, nil, &from, c); free.Kind != SnapFree || free.Point != cursor {
			t.Fatalf("Alt no longer allows free placement: %+v", free)
		}
		c.Suppressed = false
	}
}

func TestSnapToGridByDefault(t *testing.T) {
	got := Resolve(geom.Vec2i{X: sub(2) + 40, Y: sub(3) - 30}, nil, nil, cfg())
	if got.Kind != SnapGrid {
		t.Fatalf("kind = %v, want grid", got.Kind)
	}
	if got.Point != at(2, 3) {
		t.Errorf("point = %v, want %v", got.Point, at(2, 3))
	}
	if got.HasGuide() {
		t.Error("a plain grid snap should draw no guide")
	}
}

// TestAltSuppressesEverything is the escape hatch of SPEC-UX §8.4: holding Alt
// gets the raw cursor, grid included.
func TestAltSuppressesEverything(t *testing.T) {
	c := cfg()
	c.Suppressed = true
	raw := geom.Vec2i{X: sub(2) + 40, Y: sub(3) - 30}

	ents := []model.Entity{model.NewLine(at(2, 3), at(5, 3))}
	got := Resolve(raw, ents, nil, c)
	if got.Kind != SnapFree {
		t.Errorf("kind = %v, want free", got.Kind)
	}
	if got.Point != raw {
		t.Errorf("point = %v, want the raw cursor %v", got.Point, raw)
	}
}

// TestEndpointBeatsMidpointBeatsGrid is the priority SPEC-UX §8.4 fixes.
func TestEndpointBeatsMidpointBeatsGrid(t *testing.T) {
	ents := []model.Entity{model.NewLine(at(0, 0), at(4, 0))}

	// Near the endpoint at (4,0): the endpoint wins even though the grid point
	// is closer to the raw cursor.
	near := geom.Vec2i{X: sub(4) - 20, Y: 20}
	got := Resolve(near, ents, nil, cfg())
	if got.Kind != SnapEndpoint || got.Point != at(4, 0) {
		t.Errorf("near an endpoint: %v at %v", got.Kind, got.Point)
	}

	// Near the midpoint at (2,0), which is not an endpoint.
	mid := geom.Vec2i{X: sub(2) + 15, Y: 15}
	got = Resolve(mid, ents, nil, cfg())
	if got.Kind != SnapMidpoint || got.Point != at(2, 0) {
		t.Errorf("near a midpoint: %v at %v", got.Kind, got.Point)
	}

	// Far from both: the grid.
	far := geom.Vec2i{X: sub(9) + 30, Y: sub(9) + 30}
	got = Resolve(far, ents, nil, cfg())
	if got.Kind != SnapGrid || got.Point != at(9, 9) {
		t.Errorf("far from everything: %v at %v", got.Kind, got.Point)
	}
}

func TestSnapRadiiScaleWithZoom(t *testing.T) {
	ents := []model.Entity{model.NewLine(at(0, 0), at(4, 0))}
	// 60 subunits from the endpoint at (4,0).
	cursor := geom.Vec2i{X: sub(4) - 60, Y: 0}

	// Zoomed in, one pixel is one subunit, so 60 subunits is 60 px away: too far.
	zoomedIn := DefaultConfig(1)
	if got := Resolve(cursor, ents, nil, zoomedIn); got.Kind == SnapEndpoint {
		t.Error("the endpoint snapped from 60 pixels away when zoomed in")
	}
	// Zoomed out, one pixel is 20 subunits, so 60 subunits is 3 px: well inside.
	zoomedOut := DefaultConfig(20)
	if got := Resolve(cursor, ents, nil, zoomedOut); got.Kind != SnapEndpoint {
		t.Errorf("the endpoint did not snap from 3 pixels away when zoomed out: %v", got.Kind)
	}
}

// TestHorizontalInference covers the dashed guide of SPEC-UX §8.4.
func TestHorizontalInference(t *testing.T) {
	from := at(0, 0)

	// Two units right and a hair up: about 1.4 degrees, inside the 4 degree
	// window, so the guide flattens it.
	cursor := geom.Vec2i{X: sub(2), Y: 12}
	got := Resolve(cursor, nil, &from, cfg())
	if got.Infer != InferHorizontal {
		t.Fatalf("infer = %v, want horizontal", got.Infer)
	}
	if got.Point.Y != from.Y {
		t.Errorf("the inferred point is at y=%d, want the guide's %d", got.Point.Y, from.Y)
	}
	if got.Point.X != sub(2) {
		t.Errorf("the inferred point is at x=%d, want it grid-snapped to %d", got.Point.X, sub(2))
	}
	if !got.HasGuide() || got.From != from {
		t.Error("an inferred snap must carry its guide")
	}

	// Well off the axis: no inference.
	steep := geom.Vec2i{X: sub(2), Y: sub(1)}
	if got := Resolve(steep, nil, &from, cfg()); got.Infer != InferNone {
		t.Errorf("a 26 degree direction inferred %v", got.Infer)
	}
}

func TestVerticalInference(t *testing.T) {
	from := at(0, 0)
	cursor := geom.Vec2i{X: 12, Y: sub(3)}
	got := Resolve(cursor, nil, &from, cfg())
	if got.Infer != InferVertical {
		t.Fatalf("infer = %v, want vertical", got.Infer)
	}
	if got.Point.X != from.X || got.Point.Y != sub(3) {
		t.Errorf("inferred point = %v", got.Point)
	}
}

func TestInferenceYieldsToASnap(t *testing.T) {
	// A real endpoint just off the axis beats staying on the guide: latching
	// onto geometry is always more useful.
	from := at(0, 0)
	ents := []model.Entity{model.NewLine(at(3, 0.1), at(5, 0.1))}
	cursor := geom.Vec2i{X: sub(3) + 10, Y: sub(0.1) + 10}

	got := Resolve(cursor, ents, &from, cfg())
	if got.Kind != SnapEndpoint {
		t.Fatalf("kind = %v, want the endpoint to win", got.Kind)
	}
	if got.Infer != InferNone {
		t.Error("an endpoint snap should not also claim an inference guide")
	}
}

func TestInferAxisBoundary(t *testing.T) {
	from := geom.Vec2i{}
	// Exactly on the axis.
	if got := inferAxis(from, geom.Vec2i{X: 1000, Y: 0}); got != InferHorizontal {
		t.Errorf("a flat direction inferred %v", got)
	}
	// Just inside four degrees: tan(3.9 deg) * 1000 = 68.
	if got := inferAxis(from, geom.Vec2i{X: 1000, Y: 68}); got != InferHorizontal {
		t.Errorf("3.9 degrees inferred %v, want horizontal", got)
	}
	// Just outside: tan(4.1 deg) * 1000 = 72.
	if got := inferAxis(from, geom.Vec2i{X: 1000, Y: 72}); got != InferNone {
		t.Errorf("4.1 degrees inferred %v, want none", got)
	}
	// A zero direction has no axis.
	if got := inferAxis(from, from); got != InferNone {
		t.Errorf("a zero direction inferred %v", got)
	}
	// Diagonal is neither.
	if got := inferAxis(from, geom.Vec2i{X: 1000, Y: 1000}); got != InferNone {
		t.Errorf("45 degrees inferred %v", got)
	}
}

// TestLineChainClosesOnItsStart is the golden path of SPEC-UX §8.3: click round
// and click the first point again to close the profile.
func TestLineChainClosesOnItsStart(t *testing.T) {
	s := NewSession(1)
	s.SetTool(ToolLine)

	corners := []geom.Vec2i{at(0, 0), at(4, 0), at(4, 4), at(0, 4)}

	// The first click only anchors: nothing to commit yet.
	if got := s.Click(corners[0]); got.Commit {
		t.Fatal("the first click of a chain committed something")
	}
	if !s.Drawing() {
		t.Error("the session does not report a chain in progress")
	}

	var committed []model.Entity
	for _, c := range corners[1:] {
		r := s.Click(c)
		if !r.Commit {
			t.Fatalf("clicking %v committed nothing", c)
		}
		committed = append(committed, r.Entity)
	}
	if len(committed) != 3 {
		t.Fatalf("three clicks produced %d lines", len(committed))
	}

	// Closing on the start point emits the last edge and ends the chain.
	closing := s.Click(corners[0])
	if !closing.Commit || !closing.ClosedChain {
		t.Fatalf("closing click = %+v", closing)
	}
	committed = append(committed, closing.Entity)
	if s.Drawing() {
		t.Error("the chain is still live after closing")
	}

	// The four lines really do close: the region engine finds one region and no
	// loose ends.
	sk := &model.Sketch{Entities: committed}
	a := sk.Arrangement()
	if len(a.Regions) != 1 || len(a.OpenEnds) != 0 {
		t.Errorf("the drawn chain gives %d regions and %d open ends",
			len(a.Regions), len(a.OpenEnds))
	}
}

func TestChainStartOnlyRingsOnceItCanClose(t *testing.T) {
	s := NewSession(1)
	s.Click(at(0, 0))
	if _, ok := s.ChainStart(); ok {
		t.Error("a one-point chain offers a close ring")
	}
	s.Click(at(4, 0))
	if _, ok := s.ChainStart(); !ok {
		t.Error("a two-point chain should offer a close ring")
	}
	p, _ := s.ChainStart()
	if p != at(0, 0) {
		t.Errorf("the close ring is at %v, want the chain start", p)
	}
}

func TestDoubleClickFinishesWithoutClosing(t *testing.T) {
	s := NewSession(1)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	if !s.Drawing() {
		t.Fatal("the chain is not live")
	}
	s.FinishChain()
	if s.Drawing() {
		t.Error("FinishChain left the chain live")
	}
}

func TestRectangleTakesTwoCorners(t *testing.T) {
	s := NewSession(1)
	s.SetTool(ToolRect)

	if got := s.Click(at(1, 1)); got.Commit {
		t.Fatal("the first corner committed a rectangle")
	}
	if a, ok := s.Anchor(); !ok || a != at(1, 1) {
		t.Errorf("anchor = %v ok=%v", a, ok)
	}
	got := s.Click(at(5, 4))
	if !got.Commit || got.Entity.Kind != model.EntRect {
		t.Fatalf("the second corner produced %+v", got)
	}
	if got.Entity.A != at(1, 1) || got.Entity.B != at(5, 4) {
		t.Errorf("rectangle corners = %v..%v", got.Entity.A, got.Entity.B)
	}
	if s.Drawing() {
		t.Error("the rectangle is still pending after being committed")
	}
}

func TestFlatRectangleIsRejectedWithAReason(t *testing.T) {
	s := NewSession(1)
	s.SetTool(ToolRect)
	s.Click(at(1, 1))
	got := s.Click(at(5, 1))
	if got.Commit {
		t.Error("a zero-height rectangle was committed")
	}
	if got.Rejected == "" {
		t.Error("the rejection gave no reason, and silence is never the answer")
	}
	if s.Drawing() {
		t.Error("the rejected rectangle is still pending")
	}
}

func TestCircleTakesCentreThenRadius(t *testing.T) {
	s := NewSession(1)
	s.SetTool(ToolCircle)
	s.CircleSegs = 16

	s.Click(at(3, 3))
	got := s.Click(at(5, 3))
	if !got.Commit || got.Entity.Kind != model.EntCircle {
		t.Fatalf("the radius click produced %+v", got)
	}
	if got.Entity.C != at(3, 3) {
		t.Errorf("centre = %v", got.Entity.C)
	}
	if got.Entity.R != sub(2) {
		t.Errorf("radius = %d, want %d", got.Entity.R, sub(2))
	}
	if got.Entity.Segs != 16 {
		t.Errorf("segments = %d, want 16", got.Entity.Segs)
	}
}

func TestZeroRadiusCircleIsRejected(t *testing.T) {
	s := NewSession(1)
	s.SetTool(ToolCircle)
	s.Click(at(3, 3))
	got := s.Click(at(3, 3))
	if got.Commit || got.Rejected == "" {
		t.Errorf("a zero-radius circle gave %+v", got)
	}
}

// TestEscapeUnwindsOneLevel covers the Esc stack of SPEC-UX §1.
func TestEscapeUnwindsOneLevel(t *testing.T) {
	s := NewSession(1)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	s.Selected = []int{0}

	// First: the half-drawn chain.
	if got := s.Escape(); got != EscapeCancelledDraw {
		t.Fatalf("first escape = %v", got)
	}
	if s.Drawing() {
		t.Error("the chain survived escape")
	}
	// Then: the selection.
	if got := s.Escape(); got != EscapeClearedSelection {
		t.Fatalf("second escape = %v", got)
	}
	if len(s.Selected) != 0 {
		t.Error("the selection survived escape")
	}
	// Then: leave the mode.
	if got := s.Escape(); got != EscapeExitMode {
		t.Fatalf("third escape = %v", got)
	}
}

func TestSwitchingToolAbandonsTheDraw(t *testing.T) {
	s := NewSession(1)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	s.SetTool(ToolRect)
	if s.Drawing() {
		t.Error("switching tools left the chain live")
	}
	if s.Tool != ToolRect {
		t.Errorf("tool = %v", s.Tool)
	}
	// Switching to the same tool is a no-op, not a cancel.
	s.SetTool(ToolRect)
	s.Click(at(1, 1))
	s.SetTool(ToolRect)
	if !s.Drawing() {
		t.Error("re-selecting the active tool cancelled the pending corner")
	}
}

func TestPreviewFollowsTheTool(t *testing.T) {
	s := NewSession(1)

	// Nothing pending: nothing to preview.
	if got := s.PreviewAt(at(1, 1)); got.Show {
		t.Error("an idle line tool previewed something")
	}
	s.Click(at(0, 0))
	p := s.PreviewAt(at(4, 0))
	if !p.Show || p.Kind != model.EntLine {
		t.Fatalf("line preview = %+v", p)
	}
	if p.Entity.A != at(0, 0) || p.Entity.B != at(4, 0) {
		t.Errorf("line preview spans %v..%v", p.Entity.A, p.Entity.B)
	}

	// Hovering the chain start says the click would close it.
	s.Click(at(4, 0))
	if got := s.PreviewAt(at(0, 0)); !got.ClosesChain {
		t.Error("hovering the chain start did not offer to close it")
	}

	s.SetTool(ToolRect)
	s.Click(at(1, 1))
	if got := s.PreviewAt(at(4, 4)); !got.Show || got.Kind != model.EntRect {
		t.Errorf("rect preview = %+v", got)
	}
	// A degenerate preview is not drawn.
	if got := s.PreviewAt(at(4, 1)); got.Show {
		t.Error("a flat rectangle was previewed")
	}
}

func TestEntityHitTest(t *testing.T) {
	ents := []model.Entity{
		model.NewLine(at(0, 0), at(10, 0)),
		model.NewRect(at(0, 5), at(4, 9)),
	}
	radius := int64(30)

	if got := EntityAt(geom.Vec2i{X: sub(5), Y: 10}, ents, radius); got != 0 {
		t.Errorf("a point on the line hit entity %d, want 0", got)
	}
	if got := EntityAt(geom.Vec2i{X: sub(2), Y: sub(5) + 10}, ents, radius); got != 1 {
		t.Errorf("a point on the rectangle's edge hit entity %d, want 1", got)
	}
	if got := EntityAt(at(20, 20), ents, radius); got != -1 {
		t.Errorf("a point far from everything hit entity %d, want none", got)
	}
	// Inside the rectangle but away from any edge is a miss: entities are
	// picked by their strokes, not their area.
	if got := EntityAt(at(2, 7), ents, radius); got != -1 {
		t.Errorf("a point inside the rectangle hit entity %d, want none", got)
	}
}

func TestReadouts(t *testing.T) {
	if got := LengthUnits(at(0, 0), at(3, 4)); math.Abs(got-5) > 1e-9 {
		t.Errorf("length = %v, want 5", got)
	}
	cases := map[float64]geom.Vec2i{
		0:   at(1, 0),
		90:  at(0, 1),
		180: at(-1, 0),
		270: at(0, -1),
		45:  at(1, 1),
	}
	for want, dir := range cases {
		if got := AngleDegrees(geom.Vec2i{}, dir); math.Abs(got-want) > 1e-9 {
			t.Errorf("angle to %v = %v, want %v", dir, got, want)
		}
	}
	if got := AngleDegrees(geom.Vec2i{}, geom.Vec2i{}); got != 0 {
		t.Errorf("a zero direction reported %v degrees", got)
	}
}

func TestToolMetadata(t *testing.T) {
	for _, tool := range []Tool{ToolSelect, ToolLine, ToolRect, ToolCircle} {
		if tool.String() == "" {
			t.Errorf("tool %d has no name", tool)
		}
		if tool.Shortcut() == "" {
			t.Errorf("%v has no shortcut", tool)
		}
		if tool.Hint() == "" {
			t.Errorf("%v has no hint, and silence is never the answer", tool)
		}
	}
}
