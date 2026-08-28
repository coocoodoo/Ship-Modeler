package sketch

import (
	"math"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// The sketch mode state machine (SPEC-UX §8.2, §8.3, §8.7). The session owns
// what the user has half-drawn: a line chain in progress, a rectangle's first
// corner, a circle's centre. Committing an entity is the caller's job, through
// the command bus, so undo covers drawing exactly like everything else.

// Tool is the active drawing tool.
type Tool uint8

// Tools are grouped in the toolbar by what they draw (Sketch_func.md §4.2);
// the enum is append-only because scripts name tools by value.
const (
	ToolSelect Tool = iota
	ToolLine
	ToolRect
	ToolCircle
	// ToolPoint places a bare position to snap to.
	ToolPoint
	// ToolMidLine draws a line outward from its middle.
	ToolMidLine
	// ToolCenterRect grows a rectangle from its centre.
	ToolCenterRect
	// ToolAlignedRect draws a rectangle at any angle, from a base edge.
	ToolAlignedRect
)

// AllTools lists every tool in toolbar order, which is what the group layout
// and the shortcut sheet are built from.
func AllTools() []Tool {
	return []Tool{
		ToolSelect, ToolLine, ToolMidLine, ToolRect, ToolCenterRect,
		ToolAlignedRect, ToolCircle, ToolPoint,
	}
}

func (t Tool) String() string {
	switch t {
	case ToolLine:
		return "Line"
	case ToolRect:
		return "Rectangle"
	case ToolCircle:
		return "Circle"
	case ToolPoint:
		return "Point"
	case ToolMidLine:
		return "Midpoint line"
	case ToolCenterRect:
		return "Centre rectangle"
	case ToolAlignedRect:
		return "Aligned rectangle"
	default:
		return "Select"
	}
}

// Shortcut is the key that activates the tool (SPEC-UX §8.2). Tools that share
// a key share a toolbar group, and the key cycles through the group.
func (t Tool) Shortcut() string {
	switch t {
	case ToolLine, ToolMidLine:
		return "L"
	case ToolRect, ToolCenterRect, ToolAlignedRect:
		return "R"
	case ToolCircle:
		return "C"
	case ToolPoint:
		return "."
	default:
		return "V"
	}
}

// staged reports whether the tool builds one shape from several clicks, which
// is what makes Esc step back a point instead of dropping the gesture.
func (t Tool) staged() bool {
	return t == ToolAlignedRect
}

// Hint is the standing hint-bar copy for the tool. Every tool says what to do
// next, because silence is never the answer (SPEC-UX §1).
func (t Tool) Hint() string {
	switch t {
	case ToolLine:
		return "Click to place points — click the first point or double-click to finish · Esc to cancel the chain"
	case ToolRect:
		return "Click two opposite corners"
	case ToolCircle:
		return "Click the centre, then the radius — segment count is in the card"
	case ToolPoint:
		return "Click to place a point to snap to — it draws no line and closes no region"
	case ToolMidLine:
		return "Click the middle of the line, then one end"
	case ToolCenterRect:
		return "Click the centre, then a corner"
	case ToolAlignedRect:
		return "Click the two ends of one edge, then the height — Esc steps back one point"
	default:
		return "Click an entity to select it · Del removes the selection · Esc clears it"
	}
}

// Session is the live state of one sketch being edited.
type Session struct {
	// SketchID is the document sketch this session edits.
	SketchID uint32
	// Tool is the active drawing tool.
	Tool Tool
	// CircleSegs is the segment count new circles are created with.
	CircleSegs int

	// Selected holds the entity indices the Select tool has picked.
	Selected []int

	// chain is the line tool's placed points, oldest first.
	chain []geom.Vec2i
	// anchor is a rectangle's first corner or a circle's centre.
	anchor geom.Vec2i
	// hasAnchor distinguishes "no first click yet" from "anchored at the
	// origin", which is a real place a user can click.
	hasAnchor bool
}

// NewSession starts editing a sketch with the Line tool ready, which is the
// tool a first-time user needs first.
func NewSession(sketchID uint32) *Session {
	return &Session{SketchID: sketchID, Tool: ToolLine, CircleSegs: model.DefaultCircleSegs}
}

// Drawing reports whether anything is half-placed, which is what Esc unwinds
// and what the rubber band previews.
func (s *Session) Drawing() bool { return len(s.chain) > 0 || s.hasAnchor }

// Chain returns the line tool's placed points.
func (s *Session) Chain() []geom.Vec2i { return s.chain }

// Anchor returns the pending rectangle corner or circle centre.
func (s *Session) Anchor() (geom.Vec2i, bool) { return s.anchor, s.hasAnchor }

// RubberFrom is the point a preview should be drawn from, or nil when nothing
// is in progress. It is also what the inference guides work against.
func (s *Session) RubberFrom() *geom.Vec2i {
	switch s.Tool {
	case ToolLine, ToolAlignedRect:
		if n := len(s.chain); n > 0 {
			return &s.chain[n-1]
		}
	case ToolRect, ToolCircle, ToolMidLine, ToolCenterRect:
		if s.hasAnchor {
			return &s.anchor
		}
	}
	return nil
}

// SetTool switches tools, abandoning anything half-drawn. Changing tool
// mid-chain and having the chain silently continue would be a surprise.
func (s *Session) SetTool(t Tool) {
	if s.Tool != t {
		s.CancelDraw()
	}
	s.Tool = t
}

// CancelDraw drops the in-progress geometry without touching the document.
func (s *Session) CancelDraw() {
	s.chain = s.chain[:0]
	s.hasAnchor = false
}

// ClearSelection empties the Select tool's picks.
func (s *Session) ClearSelection() { s.Selected = s.Selected[:0] }

// Escape implements one press of the Esc stack (SPEC-UX §1): abandon the
// pending draw first, then the selection, and only then report that the caller
// should leave sketch mode.
type EscapeResult uint8

const (
	// EscapeCancelledDraw means a half-drawn entity was dropped.
	EscapeCancelledDraw EscapeResult = iota
	// EscapeClearedSelection means the selection was emptied.
	EscapeClearedSelection
	// EscapeExitMode means there was nothing left to cancel.
	EscapeExitMode
)

// Escape unwinds one level and says what it did.
func (s *Session) Escape() EscapeResult {
	// A staged gesture steps back one point per press rather than abandoning
	// the lot: three clicks of care should not be undone by one reflex
	// (Sketch_func.md §1.2). The line chain is deliberately not staged — its
	// hint promises "Esc to cancel the chain", and a chain is a run of
	// committed lines, so there is nothing but the pending point to lose.
	if s.Tool.staged() && len(s.chain) > 0 {
		s.chain = s.chain[:len(s.chain)-1]
		return EscapeCancelledDraw
	}
	if s.Drawing() {
		s.CancelDraw()
		return EscapeCancelledDraw
	}
	if len(s.Selected) > 0 {
		s.ClearSelection()
		return EscapeClearedSelection
	}
	return EscapeExitMode
}

// ClickResult is what a click produced.
type ClickResult struct {
	// Entity, when Commit is set, is the finished entity to run through the bus.
	Entity model.Entity
	// Entities carries the result instead when one gesture produces several,
	// as the aligned rectangle's four lines do. Read the result through
	// Committed rather than either field, so there is one answer to "what did
	// this click make".
	Entities []model.Entity
	Commit   bool
	// ClosedChain reports that a line chain closed on its own start point,
	// which is what turns a run of clicks into a closed profile.
	ClosedChain bool
	// Rejected carries a reason the click could not do anything useful, for
	// the invalid-click feedback of SPEC-UX §1.
	Rejected string
}

// Committed is everything the click produced, in commit order. It is the one
// way to read a result: a caller that reached for Entity directly would
// silently drop the aligned rectangle's other three lines.
func (r ClickResult) Committed() []model.Entity {
	if !r.Commit {
		return nil
	}
	if len(r.Entities) > 0 {
		return r.Entities
	}
	return []model.Entity{r.Entity}
}

// Click places a point with the active tool. p is the already-snapped position.
//
// The line tool builds a chain and emits one line entity per placed segment, so
// each stroke is independently undoable and the region engine sees exactly what
// was drawn.
func (s *Session) Click(p geom.Vec2i) ClickResult {
	switch s.Tool {
	case ToolLine:
		return s.clickLine(p)
	case ToolRect:
		return s.clickRect(p)
	case ToolCircle:
		return s.clickCircle(p)
	case ToolPoint:
		return ClickResult{Entity: model.NewPoint(p), Commit: true}
	case ToolMidLine:
		return s.clickMidLine(p)
	case ToolCenterRect:
		return s.clickCenterRect(p)
	case ToolAlignedRect:
		return s.clickAlignedRect(p)
	default:
		return ClickResult{}
	}
}

// clickMidLine draws outward from the middle: click the centre, then one end,
// and the far end is the reflection.
func (s *Session) clickMidLine(p geom.Vec2i) ClickResult {
	if !s.hasAnchor {
		s.anchor, s.hasAnchor = p, true
		return ClickResult{}
	}
	far := reflect(s.anchor, p)
	s.hasAnchor = false
	e := model.NewLine(far, p)
	if e.Degenerate() {
		return ClickResult{Rejected: "A line needs length — click away from the midpoint"}
	}
	return ClickResult{Entity: e, Commit: true}
}

// clickCenterRect grows a rectangle both ways from its centre.
func (s *Session) clickCenterRect(p geom.Vec2i) ClickResult {
	if !s.hasAnchor {
		s.anchor, s.hasAnchor = p, true
		return ClickResult{}
	}
	e := model.NewRect(reflect(s.anchor, p), p)
	s.hasAnchor = false
	if e.Degenerate() {
		return ClickResult{Rejected: "A rectangle needs width and height"}
	}
	return ClickResult{Entity: e, Commit: true}
}

// clickAlignedRect lays a base edge with two clicks and takes the height from
// a third, which is how a rectangle gets drawn at an angle.
//
// It commits four lines rather than a rectangle entity, because EntRect is
// axis-aligned by definition and a rotated one is exactly its four edges
// (Sketch_func.md §2). The chain carries the staging so Esc can step back one
// point at a time.
func (s *Session) clickAlignedRect(p geom.Vec2i) ClickResult {
	if len(s.chain) < 2 {
		if n := len(s.chain); n > 0 && s.chain[n-1] == p {
			return ClickResult{Rejected: "That point is already placed"}
		}
		s.chain = append(s.chain, p)
		return ClickResult{}
	}
	a, b := s.chain[0], s.chain[1]
	lines, ok := alignedRectLines(a, b, p)
	if !ok {
		return ClickResult{Rejected: "An aligned rectangle needs height — click off the edge"}
	}
	s.chain = s.chain[:0]
	return ClickResult{Entities: lines, Commit: true}
}

// AlignedRectLines is alignedRectLines for callers outside the package: the
// script op builds the same rectangle the tool does, through the same code.
func AlignedRectLines(a, b, p geom.Vec2i) ([]model.Entity, bool) {
	return alignedRectLines(a, b, p)
}

// alignedRectLines builds the four edges of a rectangle whose base runs a→b
// and whose far side passes through p.
//
// The height is p's distance from the base line, taken along the perpendicular
// so the corners are square whatever direction p was clicked in. It is rounded
// once, here, and everything downstream stays exact (SPEC-GEOMETRY §3).
func alignedRectLines(a, b, p geom.Vec2i) ([]model.Entity, bool) {
	base := b.Sub(a)
	length := base.Len()
	if length <= 0 {
		return nil, false
	}
	// The unit perpendicular to the base, scaled by how far p lies off it.
	perpX, perpY := -float64(base.Y)/length, float64(base.X)/length
	off := float64(p.Sub(a).X)*perpX + float64(p.Sub(a).Y)*perpY
	d := geom.Vec2i{
		X: int64(roundHalf(perpX * off)),
		Y: int64(roundHalf(perpY * off)),
	}
	if d == (geom.Vec2i{}) {
		return nil, false
	}
	c, dPt := b.Add(d), a.Add(d)
	return []model.Entity{
		model.NewLine(a, b),
		model.NewLine(b, c),
		model.NewLine(c, dPt),
		model.NewLine(dPt, a),
	}, true
}

// reflect returns the point opposite p through centre.
func reflect(centre, p geom.Vec2i) geom.Vec2i {
	return geom.Vec2i{X: 2*centre.X - p.X, Y: 2*centre.Y - p.Y}
}

func roundHalf(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v + 0.5))
	}
	return float64(int64(v + 0.5))
}

func (s *Session) clickLine(p geom.Vec2i) ClickResult {
	if len(s.chain) == 0 {
		s.chain = append(s.chain, p)
		return ClickResult{}
	}
	last := s.chain[len(s.chain)-1]
	if p == last {
		return ClickResult{Rejected: "That point is already placed"}
	}

	// Clicking the chain's own start closes the profile and ends the run.
	if p == s.chain[0] && len(s.chain) >= 2 {
		s.chain = s.chain[:0]
		return ClickResult{
			Entity:      model.NewLine(last, p),
			Commit:      true,
			ClosedChain: true,
		}
	}

	s.chain = append(s.chain, p)
	return ClickResult{Entity: model.NewLine(last, p), Commit: true}
}

func (s *Session) clickRect(p geom.Vec2i) ClickResult {
	if !s.hasAnchor {
		s.anchor, s.hasAnchor = p, true
		return ClickResult{}
	}
	e := model.NewRect(s.anchor, p)
	s.hasAnchor = false
	if e.Degenerate() {
		return ClickResult{Rejected: "A rectangle needs width and height"}
	}
	return ClickResult{Entity: e, Commit: true}
}

func (s *Session) clickCircle(p geom.Vec2i) ClickResult {
	if !s.hasAnchor {
		s.anchor, s.hasAnchor = p, true
		return ClickResult{}
	}
	r := radiusOf(s.anchor, p)
	e := model.NewCircle(s.anchor, r, s.CircleSegs)
	s.hasAnchor = false
	if e.Degenerate() {
		return ClickResult{Rejected: "A circle needs a radius"}
	}
	return ClickResult{Entity: e, Commit: true}
}

// radiusOf measures a circle's radius from centre to rim, rounded to the
// nearest subunit so the value stays on the lattice (SPEC-UX §8.3).
func radiusOf(centre, rim geom.Vec2i) int64 {
	return int64(rim.Sub(centre).Len() + 0.5)
}

// FinishChain ends a line run without closing it, which is what a double-click
// does (SPEC-UX §8.3).
func (s *Session) FinishChain() {
	if s.Tool == ToolLine {
		s.chain = s.chain[:0]
	}
}

// Preview is the rubber-band shape under the cursor: what would be committed if
// the user clicked right now.
type Preview struct {
	// Kind is which shape to draw, or none.
	Kind model.EntityKind
	// Entity is the shape itself, valid when Show is set.
	Entity model.Entity
	// Entities carries a multi-part preview — the aligned rectangle's four
	// edges — the same way ClickResult does. Read through Shapes.
	Entities []model.Entity
	Show     bool
	// ClosesChain marks that clicking here would close the profile, which the
	// UI shows with a ring on the start point.
	ClosesChain bool
}

// Shapes is everything the preview draws, in order — the counterpart of
// ClickResult.Committed, so what is previewed and what is committed are read
// the same way.
func (p Preview) Shapes() []model.Entity {
	if !p.Show {
		return nil
	}
	if len(p.Entities) > 0 {
		return p.Entities
	}
	return []model.Entity{p.Entity}
}

// PreviewAt builds the rubber band for a cursor position.
func (s *Session) PreviewAt(p geom.Vec2i) Preview {
	switch s.Tool {
	case ToolLine:
		if len(s.chain) == 0 {
			return Preview{}
		}
		last := s.chain[len(s.chain)-1]
		if last == p {
			return Preview{}
		}
		return Preview{
			Kind:        model.EntLine,
			Entity:      model.NewLine(last, p),
			Show:        true,
			ClosesChain: len(s.chain) >= 2 && p == s.chain[0],
		}
	case ToolRect:
		if !s.hasAnchor {
			return Preview{}
		}
		e := model.NewRect(s.anchor, p)
		return Preview{Kind: model.EntRect, Entity: e, Show: !e.Degenerate()}
	case ToolCircle:
		if !s.hasAnchor {
			return Preview{}
		}
		e := model.NewCircle(s.anchor, radiusOf(s.anchor, p), s.CircleSegs)
		return Preview{Kind: model.EntCircle, Entity: e, Show: !e.Degenerate()}
	case ToolMidLine:
		if !s.hasAnchor {
			return Preview{}
		}
		e := model.NewLine(reflect(s.anchor, p), p)
		return Preview{Kind: model.EntLine, Entity: e, Show: !e.Degenerate()}
	case ToolCenterRect:
		if !s.hasAnchor {
			return Preview{}
		}
		e := model.NewRect(reflect(s.anchor, p), p)
		return Preview{Kind: model.EntRect, Entity: e, Show: !e.Degenerate()}
	case ToolAlignedRect:
		switch len(s.chain) {
		case 1:
			// The base edge being laid: a plain rubber-band line.
			e := model.NewLine(s.chain[0], p)
			return Preview{Kind: model.EntLine, Entity: e, Show: !e.Degenerate()}
		case 2:
			lines, ok := alignedRectLines(s.chain[0], s.chain[1], p)
			if !ok {
				return Preview{}
			}
			return Preview{Kind: model.EntLine, Entities: lines, Show: true}
		}
	}
	return Preview{}
}

// ChainStart returns the first point of an in-progress chain, which the UI
// rings so the user can see where to click to close (SPEC-UX §8.3).
func (s *Session) ChainStart() (geom.Vec2i, bool) {
	if s.Tool == ToolLine && len(s.chain) >= 2 {
		return s.chain[0], true
	}
	return geom.Vec2i{}, false
}

// EntityAt returns the index of the entity nearest a point within a pick
// radius, or -1. This is the Select tool's hit test (SPEC-UX §8.5).
func EntityAt(p geom.Vec2i, ents []model.Entity, radius int64) int {
	best, bestD := -1, int64(0)
	for i := range ents {
		d := distanceToEntity(p, ents[i])
		if d > radius {
			continue
		}
		if best < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// distanceToEntity is the distance from a point to an entity's nearest edge, in
// subunits.
func distanceToEntity(p geom.Vec2i, e model.Entity) int64 {
	pts := e.Points()
	// A point has no segment to measure to, and something you can place has to
	// be something you can pick: measure to the position itself.
	if len(pts) == 1 {
		return int64(p.Sub(pts[0]).Len() + 0.5)
	}
	if len(pts) < 2 {
		return 1 << 62
	}
	n := len(pts)
	if !e.Closed() {
		n--
	}
	best := int64(1) << 62
	for i := 0; i < n; i++ {
		if d := distanceToSegment(p, pts[i], pts[(i+1)%len(pts)]); d < best {
			best = d
		}
	}
	return best
}

// distanceToSegment measures a point against a segment. The projection uses
// float64 only for the final distance; the clamp itself is exact.
func distanceToSegment(p, a, b geom.Vec2i) int64 {
	ab := b.Sub(a)
	den := ab.Dot(ab)
	if den == 0 {
		return int64(p.Sub(a).Len() + 0.5)
	}
	t := float64(p.Sub(a).Dot(ab)) / float64(den)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	cx := float64(a.X) + t*float64(ab.X)
	cy := float64(a.Y) + t*float64(ab.Y)
	dx := float64(p.X) - cx
	dy := float64(p.Y) - cy
	return int64(math.Hypot(dx, dy) + 0.5)
}
