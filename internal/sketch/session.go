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
	// ToolCircle3 fits a circle through three points.
	ToolCircle3
	// ToolEllipse draws an oval from its centre and two axes.
	ToolEllipse
	// ToolArcCenter sweeps an arc about a centre.
	ToolArcCenter
	// ToolArc3 fits an arc through three points.
	ToolArc3
	// ToolArcTangent continues an existing entity smoothly.
	ToolArcTangent
	// ToolPolygon draws a regular n-gon with a corner where you click.
	ToolPolygon
	// ToolPolygonCirc draws one whose flat sides touch where you click.
	ToolPolygonCirc
	// ToolSlot draws a capsule: a track with rounded ends.
	ToolSlot
	// ToolSpline draws a smooth curve through the points you place.
	ToolSpline
	// ToolBezier draws one cubic from four control points.
	ToolBezier
)

// AllTools lists every tool in toolbar order, which is what the group layout
// and the shortcut sheet are built from.
func AllTools() []Tool {
	return []Tool{
		ToolSelect, ToolLine, ToolMidLine, ToolRect, ToolCenterRect,
		ToolAlignedRect, ToolCircle, ToolCircle3, ToolEllipse,
		ToolArcCenter, ToolArc3, ToolArcTangent,
		ToolPolygon, ToolPolygonCirc, ToolSlot,
		ToolSpline, ToolBezier, ToolPoint,
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
	case ToolCircle3:
		return "3 point circle"
	case ToolEllipse:
		return "Ellipse"
	case ToolArcCenter:
		return "Centre point arc"
	case ToolArc3:
		return "3 point arc"
	case ToolArcTangent:
		return "Tangent arc"
	case ToolPolygon:
		return "Inscribed polygon"
	case ToolPolygonCirc:
		return "Circumscribed polygon"
	case ToolSlot:
		return "Slot"
	case ToolSpline:
		return "Spline"
	case ToolBezier:
		return "Bezier"
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
	case ToolCircle, ToolCircle3, ToolEllipse:
		return "C"
	case ToolArcCenter, ToolArc3, ToolArcTangent:
		return "A"
	case ToolPolygon, ToolPolygonCirc:
		return "P"
	case ToolSlot:
		return "O"
	case ToolSpline, ToolBezier:
		return "S"
	case ToolPoint:
		return "."
	default:
		return "V"
	}
}

// staged reports whether the tool builds one shape from several clicks, which
// is what makes Esc step back a point instead of dropping the gesture.
func (t Tool) staged() bool {
	switch t {
	case ToolAlignedRect, ToolCircle3, ToolEllipse, ToolArcCenter, ToolArc3,
		ToolSlot, ToolSpline, ToolBezier:
		return true
	}
	return false
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
	case ToolCircle3:
		return "Click three points on the circle"
	case ToolEllipse:
		return "Click the centre, then the long axis, then how far across"
	case ToolArcCenter:
		return "Click the centre, then the start, then sweep to the end"
	case ToolArc3:
		return "Click the arc's start, then its end, then a point it passes through"
	case ToolArcTangent:
		return "Click a loose endpoint to continue from, then where the arc ends"
	case ToolPolygon:
		return "Click the centre, then a corner - the side count is in the card"
	case ToolPolygonCirc:
		return "Click the centre, then the middle of a flat side"
	case ToolSlot:
		return "Click the two ends of the track, then how wide it is"
	case ToolSpline:
		return "Click points for the curve to run through — click the first to close it, or double-click to finish · Esc steps back one point"
	case ToolBezier:
		return "Click the start, two handles, then the end"
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
	// Sides is how many a new polygon gets.
	Sides int
	// Subdivisions is how finely a new spline or bezier is tessellated, per
	// span. Zero means the default.
	Subdivisions int

	// Selected holds the entity indices the Select tool has picked.
	Selected []int

	// chain is the line tool's placed points, oldest first.
	chain []geom.Vec2i
	// anchor is a rectangle's first corner or a circle's centre.
	anchor geom.Vec2i
	// hasAnchor distinguishes "no first click yet" from "anchored at the
	// origin", which is a real place a user can click.
	hasAnchor bool
	// tangentDir is the direction the tangent arc must leave its anchor in,
	// taken from the entity it was hung off.
	tangentDir geom.Vec2i

	// context is what is already drawn, which the tangent arc needs in order
	// to find an endpoint to continue from. The session is otherwise ignorant
	// of the sketch's contents on purpose — it owns what is half-drawn, not
	// what is finished — so this is set by the app each frame rather than
	// held as a reference that could go stale.
	context []model.Entity
}

// SetContext hands the session the entities already in the sketch, for the
// tools that build on what is there.
func (s *Session) SetContext(ents []model.Entity) { s.context = ents }

// NewSession starts editing a sketch with the Line tool ready, which is the
// tool a first-time user needs first.
func NewSession(sketchID uint32) *Session {
	return &Session{
		SketchID:   sketchID,
		Tool:       ToolLine,
		CircleSegs: model.DefaultCircleSegs,
		Sides:      model.DefaultPolygonSides,
	}
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
	case ToolLine, ToolAlignedRect, ToolCircle3, ToolEllipse, ToolArcCenter,
		ToolArc3, ToolSlot, ToolSpline, ToolBezier:
		if n := len(s.chain); n > 0 {
			return &s.chain[n-1]
		}
	case ToolRect, ToolCircle, ToolMidLine, ToolCenterRect, ToolArcTangent,
		ToolPolygon, ToolPolygonCirc:
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
	case ToolCircle3, ToolEllipse, ToolArcCenter, ToolArc3, ToolSlot:
		return s.clickThreePoint(p)
	case ToolPolygon, ToolPolygonCirc:
		return s.clickPolygon(p)
	case ToolSpline:
		return s.clickSpline(p)
	case ToolBezier:
		return s.clickBezier(p)
	case ToolArcTangent:
		return s.clickTangentArc(p)
	default:
		return ClickResult{}
	}
}

// clickThreePoint runs the four gestures that take three clicks and build one
// shape from them. They differ only in what the three points mean, which is
// the whole of buildThreePoint.
func (s *Session) clickThreePoint(p geom.Vec2i) ClickResult {
	if len(s.chain) < 2 {
		if n := len(s.chain); n > 0 && s.chain[n-1] == p {
			return ClickResult{Rejected: "That point is already placed"}
		}
		s.chain = append(s.chain, p)
		return ClickResult{}
	}
	e, ok := s.buildThreePoint(s.chain[0], s.chain[1], p)
	// Either way the attempt is over: a click that cannot make a shape ends
	// the gesture and says why, which is what the circle and rectangle tools
	// have done since M2. One rule for every tool beats a kinder one for some.
	s.chain = s.chain[:0]
	if !ok {
		return ClickResult{Rejected: s.Tool.refusal()}
	}
	return ClickResult{Entity: e, Commit: true}
}

// buildThreePoint turns three placed points into the shape the active tool
// makes of them.
func (s *Session) buildThreePoint(a, b, c geom.Vec2i) (model.Entity, bool) {
	switch s.Tool {
	case ToolCircle3:
		centre, ok := Circumcentre(a, b, c)
		if !ok {
			return model.Entity{}, false
		}
		e := model.NewCircle(centre, radiusOf(centre, a), s.CircleSegs)
		return e, !e.Degenerate()
	case ToolEllipse:
		return ellipseThrough(a, b, c, s.CircleSegs)
	case ToolArcCenter:
		return arcToward(a, b, c, s.CircleSegs)
	case ToolArc3:
		// Start, end, then a point on the way: the third click is the bulge.
		return arcThrough(a, c, b, s.CircleSegs)
	case ToolSlot:
		return slotThrough(a, b, c)
	}
	return model.Entity{}, false
}

// clickSpline collects points for a curve to run through, the way the line
// tool collects a chain — but the whole run becomes one entity, because a
// spline is one curve and not a series of them.
//
// Clicking the first point again closes the loop; FinishSpline ends it open,
// which is what a double-click does.
func (s *Session) clickSpline(p geom.Vec2i) ClickResult {
	if n := len(s.chain); n >= 2 && p == s.chain[0] {
		e := model.NewSpline(s.chain, true, s.SplineSegs())
		s.chain = s.chain[:0]
		if e.Degenerate() {
			return ClickResult{Rejected: "A closed spline needs points that are not all in one place"}
		}
		return ClickResult{Entity: e, Commit: true, ClosedChain: true}
	}
	if n := len(s.chain); n > 0 && s.chain[n-1] == p {
		return ClickResult{Rejected: "That point is already placed"}
	}
	s.chain = append(s.chain, p)
	return ClickResult{}
}

// FinishSpline ends an open curve, which is what double-clicking does. A run
// of fewer than two points is not a curve, so it simply clears.
func (s *Session) FinishSpline() ClickResult {
	if s.Tool != ToolSpline {
		return ClickResult{}
	}
	pts := s.chain
	if len(pts) < 2 {
		s.chain = s.chain[:0]
		return ClickResult{}
	}
	e := model.NewSpline(pts, false, s.SplineSegs())
	s.chain = s.chain[:0]
	if e.Degenerate() {
		return ClickResult{Rejected: "A spline needs points that are not all in one place"}
	}
	return ClickResult{Entity: e, Commit: true}
}

// clickBezier takes four controls: the two ends and the two handles that pull
// the curve between them.
func (s *Session) clickBezier(p geom.Vec2i) ClickResult {
	if len(s.chain) < 3 {
		s.chain = append(s.chain, p)
		return ClickResult{}
	}
	e := model.NewBezier(s.chain[0], s.chain[1], s.chain[2], p, s.SplineSegs())
	s.chain = s.chain[:0]
	if e.Degenerate() {
		return ClickResult{Rejected: "A bezier needs its controls in more than one place"}
	}
	return ClickResult{Entity: e, Commit: true}
}

// SplineSegs is the subdivision count new curves get.
func (s *Session) SplineSegs() int {
	if s.Subdivisions == 0 {
		return model.DefaultSplineSegs
	}
	return s.Subdivisions
}

// clickPolygon takes a centre and then a size, the second click meaning a
// corner or the middle of a flat side depending on the variant.
func (s *Session) clickPolygon(p geom.Vec2i) ClickResult {
	if !s.hasAnchor {
		s.anchor, s.hasAnchor = p, true
		return ClickResult{}
	}
	e, ok := s.buildPolygon(s.anchor, p)
	s.hasAnchor = false
	if !ok {
		return ClickResult{Rejected: s.Tool.refusal()}
	}
	return ClickResult{Entity: e, Commit: true}
}

// buildPolygon makes the n-gon the active variant asks for.
func (s *Session) buildPolygon(c, p geom.Vec2i) (model.Entity, bool) {
	sides := s.Sides
	if sides == 0 {
		sides = model.DefaultPolygonSides
	}
	var e model.Entity
	if s.Tool == ToolPolygonCirc {
		e = model.NewCircumscribedPolygon(c, p, sides)
	} else {
		e = model.NewPolygon(c, p, sides)
	}
	return e, !e.Degenerate()
}

// clickTangentArc hangs an arc off a loose endpoint so it leaves smoothly.
//
// The first click must land on an endpoint: without one there is no direction
// to be tangent to, and inventing one would draw a shape nobody asked for
// (Sketch_func.md §6).
func (s *Session) clickTangentArc(p geom.Vec2i) ClickResult {
	if !s.hasAnchor {
		at, dir, ok := endpointNear(p, s.context, s.tangentReach())
		if !ok {
			return ClickResult{
				Rejected: "Click a loose endpoint to continue from — a tangent arc needs one",
			}
		}
		s.anchor, s.hasAnchor = at, true
		s.tangentDir = dir
		return ClickResult{}
	}
	e, ok := arcTangent(s.anchor, s.tangentDir, p, s.CircleSegs)
	s.hasAnchor = false
	if !ok {
		return ClickResult{Rejected: "That is straight on from the endpoint — an arc needs to curve"}
	}
	return ClickResult{Entity: e, Commit: true}
}

// tangentReach is how close a click must come to an endpoint to grab it. The
// session has no zoom, so this is generous in sketch units rather than exact
// in pixels; the app snaps the click to the endpoint first anyway.
func (s *Session) tangentReach() int64 { return geom.SubunitsPerUnit }

// refusal is what a tool says when its points make no shape.
func (t Tool) refusal() string {
	switch t {
	case ToolCircle3:
		return "Those three points are in a line — a circle needs a bend"
	case ToolArc3:
		return "Those three points are in a line — an arc needs a bend"
	case ToolArcCenter:
		return "An arc needs a centre, a start and a sweep"
	case ToolEllipse:
		return "An ellipse needs width across its long axis"
	case ToolSlot:
		return "A slot needs two ends and a width across them"
	case ToolPolygon, ToolPolygonCirc:
		return "A polygon needs a size - click away from the centre"
	}
	return "That does not make a shape"
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
	s.chain = s.chain[:0]
	if !ok {
		return ClickResult{Rejected: "An aligned rectangle needs height — click off the edge"}
	}
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
	case ToolSpline:
		if len(s.chain) == 0 {
			return Preview{}
		}
		// The preview is the real curve through the points placed so far plus
		// the cursor, so what is shown is what would land.
		pts := append(append([]geom.Vec2i(nil), s.chain...), p)
		closes := len(s.chain) >= 2 && p == s.chain[0]
		e := model.NewSpline(pts, closes, s.SplineSegs())
		if closes {
			e = model.NewSpline(s.chain, true, s.SplineSegs())
		}
		return Preview{
			Kind: model.EntSpline, Entity: e,
			Show: !e.Degenerate(), ClosesChain: closes,
		}
	case ToolBezier:
		switch len(s.chain) {
		case 1, 2:
			// The control cage, so the handles being placed are visible.
			e := model.NewLine(s.chain[len(s.chain)-1], p)
			return Preview{Kind: model.EntLine, Entity: e, Show: !e.Degenerate()}
		case 3:
			e := model.NewBezier(s.chain[0], s.chain[1], s.chain[2], p, s.SplineSegs())
			return Preview{Kind: model.EntBezier, Entity: e, Show: !e.Degenerate()}
		}
	case ToolPolygon, ToolPolygonCirc:
		if !s.hasAnchor {
			return Preview{}
		}
		e, ok := s.buildPolygon(s.anchor, p)
		if !ok {
			return Preview{}
		}
		return Preview{Kind: model.EntPolygon, Entity: e, Show: true}
	case ToolCircle3, ToolEllipse, ToolArcCenter, ToolArc3, ToolSlot:
		switch len(s.chain) {
		case 1:
			// One point placed: a guide line to the cursor, so the gesture is
			// visibly in progress even before it has a shape to show.
			e := model.NewLine(s.chain[0], p)
			return Preview{Kind: model.EntLine, Entity: e, Show: !e.Degenerate()}
		case 2:
			e, ok := s.buildThreePoint(s.chain[0], s.chain[1], p)
			if !ok {
				return Preview{}
			}
			return Preview{Kind: e.Kind, Entity: e, Show: true}
		}
	case ToolArcTangent:
		if !s.hasAnchor {
			return Preview{}
		}
		e, ok := arcTangent(s.anchor, s.tangentDir, p, s.CircleSegs)
		if !ok {
			return Preview{}
		}
		return Preview{Kind: model.EntArc, Entity: e, Show: true}
	}
	return Preview{}
}

// ChainStart returns the first point of an in-progress chain, which the UI
// rings so the user can see where to click to close (SPEC-UX §8.3).
func (s *Session) ChainStart() (geom.Vec2i, bool) {
	if (s.Tool == ToolLine || s.Tool == ToolSpline) && len(s.chain) >= 2 {
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
