package sketch

import (
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// SK1's session layer: the point tool and the three variant gestures
// (Sketch_func.md §5 SK1). Written before the state machines they describe.
//
// Every one of these is a click sequence with an Esc story, and the Esc story
// is the part that is easy to get wrong: a multi-stage tool must unwind one
// stage per press, not abandon the whole gesture on the first.

func session(tool Tool) *Session {
	s := NewSession(1)
	s.SetTool(tool)
	return s
}

// --- Point ----------------------------------------------------------------

func TestPointToolCommitsOnEveryClick(t *testing.T) {
	s := session(ToolPoint)
	for i, p := range []geom.Vec2i{at(1, 1), at(2, 5), at(0, 0)} {
		got := s.Click(p)
		if !got.Commit {
			t.Fatalf("click %d placed nothing", i)
		}
		ents := got.Committed()
		if len(ents) != 1 || ents[0].Kind != model.EntPoint || ents[0].A != p {
			t.Errorf("click %d produced %+v, want a point at %v", i, ents, p)
		}
		if s.Drawing() {
			t.Errorf("click %d left the tool mid-gesture; a point is one click", i)
		}
	}
}

// --- Midpoint line --------------------------------------------------------

// The gesture: click the middle, then one end. The far end is the reflection,
// which is the whole point of the tool — a line centred where you said.
func TestMidpointLineMirrorsItsAnchor(t *testing.T) {
	s := session(ToolMidLine)

	if got := s.Click(at(3, 3)); got.Commit {
		t.Fatal("the first click committed; it only sets the midpoint")
	}
	if !s.Drawing() {
		t.Error("the tool does not report being mid-gesture after the midpoint")
	}

	got := s.Click(at(5, 3))
	ents := got.Committed()
	if !got.Commit || len(ents) != 1 {
		t.Fatalf("the second click produced %+v", ents)
	}
	e := ents[0]
	if e.Kind != model.EntLine {
		t.Fatalf("kind = %v, want a line", e.Kind)
	}
	// A at the far side, B where the user clicked: (1,3) → (5,3) about (3,3).
	if e.A != at(1, 3) || e.B != at(5, 3) {
		t.Errorf("line = %v..%v, want %v..%v", e.A, e.B, at(1, 3), at(5, 3))
	}
	// The midpoint of the result is exactly the point that was clicked.
	mid := geom.Vec2i{X: (e.A.X + e.B.X) / 2, Y: (e.A.Y + e.B.Y) / 2}
	if mid != at(3, 3) {
		t.Errorf("midpoint = %v, want %v", mid, at(3, 3))
	}
	if s.Drawing() {
		t.Error("the tool stayed armed after committing")
	}
}

func TestMidpointLineRefusesAZeroLengthReach(t *testing.T) {
	s := session(ToolMidLine)
	s.Click(at(2, 2))
	got := s.Click(at(2, 2))
	if got.Commit {
		t.Fatal("a midpoint line with no reach was committed")
	}
	if got.Rejected == "" {
		t.Error("the refusal said nothing about why")
	}
	if s.Drawing() {
		t.Error("a refused second click left the gesture half open")
	}
}

// --- Centre rectangle -----------------------------------------------------

func TestCentreRectangleGrowsBothWays(t *testing.T) {
	s := session(ToolCenterRect)

	if got := s.Click(at(3, 3)); got.Commit {
		t.Fatal("the first click committed; it only sets the centre")
	}
	got := s.Click(at(5, 4))
	ents := got.Committed()
	if !got.Commit || len(ents) != 1 || ents[0].Kind != model.EntRect {
		t.Fatalf("the second click produced %+v", ents)
	}
	e := ents[0]
	// Corner (5,4) about centre (3,3) puts the opposite corner at (1,2).
	lo, hi := corners(e)
	if lo != at(1, 2) || hi != at(5, 4) {
		t.Errorf("rectangle = %v..%v, want %v..%v", lo, hi, at(1, 2), at(5, 4))
	}
	// The centre of the result is exactly where the first click landed.
	mid := geom.Vec2i{X: (e.A.X + e.B.X) / 2, Y: (e.A.Y + e.B.Y) / 2}
	if mid != at(3, 3) {
		t.Errorf("centre = %v, want %v", mid, at(3, 3))
	}
}

func TestCentreRectangleRefusesAFlatCorner(t *testing.T) {
	s := session(ToolCenterRect)
	s.Click(at(2, 2))
	if got := s.Click(at(2, 5)); got.Commit || got.Rejected == "" {
		t.Error("a rectangle with no width was accepted, or refused silently")
	}
}

// --- Aligned rectangle ----------------------------------------------------

// Three clicks: two lay the base edge at any angle, the third gives the
// height. It commits four lines rather than an EntRect, because EntRect is
// axis-aligned by definition (Sketch_func.md §2).
func TestAlignedRectangleEmitsFourConnectedLines(t *testing.T) {
	s := session(ToolAlignedRect)

	if got := s.Click(at(0, 0)); got.Commit {
		t.Fatal("click 1 committed")
	}
	if got := s.Click(at(4, 0)); got.Commit {
		t.Fatal("click 2 committed; the base edge is not the whole rectangle")
	}
	got := s.Click(at(4, 2))
	ents := got.Committed()
	if !got.Commit {
		t.Fatal("click 3 committed nothing")
	}
	if len(ents) != 4 {
		t.Fatalf("got %d entities, want 4 lines", len(ents))
	}
	for i, e := range ents {
		if e.Kind != model.EntLine {
			t.Errorf("entity %d is a %v, want a line", i, e.Kind)
		}
	}
	// Head to tail, and back to the start: an open chain would be a profile
	// that cannot be extruded, which is the whole reason to draw a rectangle.
	for i := range ents {
		next := ents[(i+1)%len(ents)]
		if ents[i].B != next.A {
			t.Errorf("line %d ends at %v but line %d starts at %v",
				i, ents[i].B, (i+1)%len(ents), next.A)
		}
	}
	if s.Drawing() {
		t.Error("the tool stayed armed after committing")
	}
}

// The base edge decides the orientation; a diagonal one produces a rectangle
// nothing else in this program can draw.
func TestAlignedRectangleFollowsItsBaseEdge(t *testing.T) {
	s := session(ToolAlignedRect)
	s.Click(at(0, 0))
	s.Click(at(3, 3)) // a 45° base
	ents := s.Click(at(2, 4)).Committed()
	if len(ents) != 4 {
		t.Fatalf("got %d entities, want 4", len(ents))
	}
	// The first line is the base exactly as clicked.
	if ents[0].A != at(0, 0) || ents[0].B != at(3, 3) {
		t.Errorf("base edge = %v..%v, want %v..%v", ents[0].A, ents[0].B, at(0, 0), at(3, 3))
	}
	// Opposite sides are parallel and equal: the shape really is a rectangle
	// and not a general quadrilateral.
	base := ents[0].B.Sub(ents[0].A)
	far := ents[2].A.Sub(ents[2].B) // walked the other way round the loop
	if base != far {
		t.Errorf("the far side is %v, want the base's %v", far, base)
	}
	// And the sides meet the base square on.
	side := ents[1].B.Sub(ents[1].A)
	if dot := base.X*side.X + base.Y*side.Y; dot != 0 {
		t.Errorf("base·side = %d, want 0 — the corners are not square", dot)
	}
}

// Esc unwinds one stage per press (Sketch_func.md §1.2), so a mis-clicked
// height does not throw away the base edge you carefully placed.
func TestAlignedRectangleEscapeUnwindsOneStageAtATime(t *testing.T) {
	s := session(ToolAlignedRect)
	s.Click(at(0, 0))
	s.Click(at(4, 0))

	if got := s.Escape(); got != EscapeCancelledDraw {
		t.Fatalf("first Esc = %v, want EscapeCancelledDraw", got)
	}
	if !s.Drawing() {
		t.Fatal("the first Esc abandoned the whole gesture, not one stage")
	}
	// The first point is still placed, so the next click re-lays the base.
	if got := s.Escape(); got != EscapeCancelledDraw {
		t.Fatalf("second Esc = %v, want EscapeCancelledDraw", got)
	}
	if s.Drawing() {
		t.Error("the second Esc left something half-drawn")
	}
	if got := s.Escape(); got != EscapeExitMode {
		t.Errorf("third Esc = %v, want EscapeExitMode", got)
	}
}

func TestAlignedRectangleRefusesNoHeight(t *testing.T) {
	s := session(ToolAlignedRect)
	s.Click(at(0, 0))
	s.Click(at(4, 0))
	got := s.Click(at(2, 0)) // on the base line: no height at all
	if got.Commit || got.Rejected == "" {
		t.Error("a flat aligned rectangle was accepted, or refused silently")
	}
}

// --- Shared behaviour -----------------------------------------------------

// Switching tools mid-gesture must not leave the next tool holding the last
// one's half-placed points.
func TestSwitchingToolsClearsEveryNewGesture(t *testing.T) {
	for _, tool := range []Tool{ToolMidLine, ToolCenterRect, ToolAlignedRect} {
		s := session(tool)
		s.Click(at(1, 1))
		if !s.Drawing() {
			t.Fatalf("%v: nothing in progress after the first click", tool)
		}
		s.SetTool(ToolSelect)
		if s.Drawing() {
			t.Errorf("%v: switching away left the gesture in progress", tool)
		}
	}
}

// Every tool says what to do next; silence is never the answer (SPEC-UX §1).
func TestEveryToolHasCopy(t *testing.T) {
	for _, tool := range AllTools() {
		if tool.String() == "" {
			t.Errorf("tool %d has no name", tool)
		}
		if tool.Hint() == "" {
			t.Errorf("%v has no hint", tool)
		}
		if tool.Shortcut() == "" {
			t.Errorf("%v has no shortcut", tool)
		}
	}
}

// The preview is what the user is judging the click by, so a tool with a
// gesture in progress must have one.
func TestNewToolsPreviewWhatTheyWouldCommit(t *testing.T) {
	cases := []struct {
		tool  Tool
		first []geom.Vec2i
		at    geom.Vec2i
	}{
		{ToolMidLine, []geom.Vec2i{at(3, 3)}, at(5, 3)},
		{ToolCenterRect, []geom.Vec2i{at(3, 3)}, at(5, 4)},
		{ToolAlignedRect, []geom.Vec2i{at(0, 0), at(4, 0)}, at(4, 2)},
	}
	for _, tc := range cases {
		s := session(tc.tool)
		for _, p := range tc.first {
			s.Click(p)
		}
		if got := s.PreviewAt(tc.at); !got.Show {
			t.Errorf("%v shows no preview mid-gesture", tc.tool)
		}
	}
}

// corners normalizes a rectangle entity to (low, high) so a test can compare
// it without caring which corner was clicked first.
func corners(e model.Entity) (lo, hi geom.Vec2i) {
	lo, hi = e.A, e.B
	if lo.X > hi.X {
		lo.X, hi.X = hi.X, lo.X
	}
	if lo.Y > hi.Y {
		lo.Y, hi.Y = hi.Y, lo.Y
	}
	return lo, hi
}

// A click that cannot make a shape ends the attempt and says why — the rule
// every tool has followed since M2's circle, and the one the three-click
// gestures of SK2 follow too. Keeping the good points for a retry would be
// kinder for some tools and inconsistent across all of them.
func TestARefusedGestureIsOver(t *testing.T) {
	cases := []struct {
		tool   Tool
		clicks []geom.Vec2i
	}{
		{ToolMidLine, []geom.Vec2i{at(2, 2), at(2, 2)}},
		{ToolCenterRect, []geom.Vec2i{at(2, 2), at(2, 5)}},
		{ToolAlignedRect, []geom.Vec2i{at(0, 0), at(4, 0), at(2, 0)}},
	}
	for _, tc := range cases {
		s := session(tc.tool)
		var last ClickResult
		for _, p := range tc.clicks {
			last = s.Click(p)
		}
		if last.Commit {
			t.Errorf("%v: the refused gesture committed", tc.tool)
		}
		if last.Rejected == "" {
			t.Errorf("%v: refused without saying why", tc.tool)
		}
		if s.Drawing() {
			t.Errorf("%v: the refused gesture is still in progress", tc.tool)
		}
	}
}
