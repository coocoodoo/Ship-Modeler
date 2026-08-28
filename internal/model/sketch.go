package model

import (
	"fmt"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/sketch2d"
)

// Sketch entities (SPEC-GEOMETRY §3). A sketch stores what the user drew in its
// logical form — a rectangle stays a rectangle — and expands to segments only
// when the region engine needs them. That is what lets a whole rectangle be
// selected, moved and deleted as one thing.

// EntityKind is which of the three drawing tools produced an entity.
type EntityKind uint8

// The kinds are serialized by value, so they are append-only and never
// reordered (Sketch_func.md §3).
const (
	// EntLine is a single straight segment.
	EntLine EntityKind = iota
	// EntRect is an axis-aligned rectangle, stored by opposite corners and
	// expanded to four segments.
	EntRect
	// EntCircle is a regular polygon inscribed in a radius: a circle in this
	// app is an n-gon, which is what keeps the result crisp and low-poly (D-06).
	EntCircle
	// EntPoint is a bare position. It makes no segments and so never joins a
	// region; it exists to be snapped to and measured from.
	EntPoint
)

func (k EntityKind) String() string {
	switch k {
	case EntRect:
		return "rectangle"
	case EntCircle:
		return "circle"
	case EntPoint:
		return "point"
	default:
		return "line"
	}
}

// Circle segment-count limits (SPEC-UX §8.3).
const (
	MinCircleSegs     = 3
	MaxCircleSegs     = 64
	DefaultCircleSegs = 16
)

// Entity is one drawn thing in a sketch. The fields it uses depend on Kind:
// a line uses A and B, a rectangle uses A and B as opposite corners, a circle
// uses C, R and Segs, and a point uses A alone. The later kinds of
// Sketch_func.md §3 draw on D, W and Pts.
//
// One flat struct rather than an interface per kind: an entity is data that
// gets saved, compared, translated and JSON round-tripped, and the only
// behaviour that varies is Points(). A sum type would buy nothing and cost a
// serialization scheme.
type Entity struct {
	Kind EntityKind `json:"kind"`
	A    geom.Vec2i `json:"a,omitempty"`
	B    geom.Vec2i `json:"b,omitempty"`
	C    geom.Vec2i `json:"c,omitempty"`
	// D is a fourth anchor, for the kinds that need one.
	D    geom.Vec2i `json:"d,omitempty"`
	R    int64      `json:"r,omitempty"`
	// W is a second scalar: a slot's half-width, an ellipse's semi-minor axis.
	W    int64 `json:"w,omitempty"`
	Segs int   `json:"segs,omitempty"`
	// Pts carries a variable-length point list for splines and beziers.
	Pts []geom.Vec2i `json:"pts,omitempty"`

	// Construction marks a guide: something to snap to and measure against
	// that is deliberately not part of the shape. Construction entities are
	// skipped when the sketch expands to segments, so they never close a
	// region and never show as an open end (Sketch_func.md §3).
	Construction bool `json:"cx,omitempty"`
}

// NewLine builds a line entity.
func NewLine(a, b geom.Vec2i) Entity { return Entity{Kind: EntLine, A: a, B: b} }

// NewRect builds a rectangle from two opposite corners.
func NewRect(a, b geom.Vec2i) Entity { return Entity{Kind: EntRect, A: a, B: b} }

// NewPoint builds a bare position.
func NewPoint(a geom.Vec2i) Entity { return Entity{Kind: EntPoint, A: a} }

// NewCircle builds an n-gon. The segment count is clamped to the legal range.
func NewCircle(c geom.Vec2i, r int64, segs int) Entity {
	return Entity{Kind: EntCircle, C: c, R: r, Segs: clampSegs(segs)}
}

func clampSegs(n int) int {
	if n < MinCircleSegs {
		return MinCircleSegs
	}
	if n > MaxCircleSegs {
		return MaxCircleSegs
	}
	return n
}

// Degenerate reports whether an entity has collapsed to nothing and should not
// be kept.
func (e Entity) Degenerate() bool {
	switch e.Kind {
	case EntRect:
		return e.A.X == e.B.X || e.A.Y == e.B.Y
	case EntCircle:
		return e.R <= 0
	case EntPoint:
		// A point is its own reason for existing, and one at the origin — where
		// A happens to equal the unset B — is a real place to put one.
		return false
	default:
		return e.A == e.B
	}
}

// Points returns the entity's corner positions in draw order. For a line that
// is its two endpoints, for a rectangle its four corners, for a circle its
// n-gon vertices.
func (e Entity) Points() []geom.Vec2i {
	switch e.Kind {
	case EntPoint:
		return []geom.Vec2i{e.A}
	case EntRect:
		return []geom.Vec2i{
			{X: e.A.X, Y: e.A.Y},
			{X: e.B.X, Y: e.A.Y},
			{X: e.B.X, Y: e.B.Y},
			{X: e.A.X, Y: e.B.Y},
		}
	case EntCircle:
		n := clampSegs(e.Segs)
		pts := make([]geom.Vec2i, n)
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			// Rounding to subunits here is the only inexact step; exactness
			// resumes immediately afterwards (SPEC-GEOMETRY §3).
			pts[i] = geom.Vec2i{
				X: e.C.X + int64(math.Round(float64(e.R)*math.Cos(a))),
				Y: e.C.Y + int64(math.Round(float64(e.R)*math.Sin(a))),
			}
		}
		return pts
	default:
		return []geom.Vec2i{e.A, e.B}
	}
}

// Closed reports whether the entity's points form a loop.
func (e Entity) Closed() bool { return e.Kind != EntLine && e.Kind != EntPoint }

// Equal compares two entities field for field.
//
// It exists because Pts made Entity uncomparable with ==, and the modify tools
// of Sketch_func.md §5 need to ask "did this change" about whole entities. A
// method the compiler cannot silently accept a wrong answer from beats an ==
// that stops compiling one day and gets replaced by something laxer.
func (e Entity) Equal(o Entity) bool {
	if e.Kind != o.Kind || e.A != o.A || e.B != o.B || e.C != o.C || e.D != o.D ||
		e.R != o.R || e.W != o.W || e.Segs != o.Segs ||
		e.Construction != o.Construction || len(e.Pts) != len(o.Pts) {
		return false
	}
	for i := range e.Pts {
		if e.Pts[i] != o.Pts[i] {
			return false
		}
	}
	return true
}

// AppendSegments expands the entity into segments, attributing each to the
// given entity index so provenance survives into the region engine.
func (e Entity) AppendSegments(dst []sketch2d.Seg, entity int) []sketch2d.Seg {
	pts := e.Points()
	if len(pts) < 2 {
		return dst
	}
	n := len(pts)
	if !e.Closed() {
		n--
	}
	for i := 0; i < n; i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		if a == b {
			continue
		}
		dst = append(dst, sketch2d.Seg{A: a, B: b, Src: sketch2d.Source{Entity: entity, Seg: i}})
	}
	return dst
}

// Bounds returns the entity's bounding box in subunits.
func (e Entity) Bounds() (min, max geom.Vec2i) {
	pts := e.Points()
	if len(pts) == 0 {
		return
	}
	min, max = pts[0], pts[0]
	for _, p := range pts[1:] {
		if p.X < min.X {
			min.X = p.X
		}
		if p.Y < min.Y {
			min.Y = p.Y
		}
		if p.X > max.X {
			max.X = p.X
		}
		if p.Y > max.Y {
			max.Y = p.Y
		}
	}
	return
}

// Translate moves the whole entity.
func (e Entity) Translate(d geom.Vec2i) Entity {
	e.A = e.A.Add(d)
	e.B = e.B.Add(d)
	e.C = e.C.Add(d)
	return e
}

// Segments expands every entity of a sketch, in order.
//
// Construction entities are skipped: they are guides, and a guide that closed
// a region or rang as an open end would be doing the one thing it exists not
// to do. They are still drawn, still snapped to and still selectable — this is
// the only place they are absent (Sketch_func.md §3).
func (s *Sketch) Segments() []sketch2d.Seg {
	var out []sketch2d.Seg
	for i := range s.Entities {
		if s.Entities[i].Construction {
			continue
		}
		out = s.Entities[i].AppendSegments(out, i)
	}
	return out
}

// Arrangement returns the sketch's regions and open ends, rebuilding the cache
// only when the entities have changed.
//
// The engine is fast enough to run on every edit (SPEC-GEOMETRY §4), but the
// UI asks for the arrangement several times a frame — to fill regions, to draw
// open ends, to label the card — so caching it keeps that to one build.
func (s *Sketch) Arrangement() sketch2d.Arrangement {
	if s.arrCached && s.arrStamp == s.stamp {
		return s.arrangement
	}
	s.arrangement = sketch2d.Build(s.Segments())
	s.arrCached = true
	s.arrStamp = s.stamp
	return s.arrangement
}

// Touch marks the sketch's derived data stale. Every mutation goes through a
// command, and every such command calls this.
func (s *Sketch) Touch() { s.stamp++ }

// EntityCount, RegionCount and OpenEndCount are what the sketch card reports
// (SPEC-UX §8.1).
func (s *Sketch) EntityCount() int  { return len(s.Entities) }
func (s *Sketch) RegionCount() int  { return len(s.Arrangement().Regions) }
func (s *Sketch) OpenEndCount() int { return len(s.Arrangement().OpenEnds) }

// Frame returns the sketch's plane frame in world space.
func (s *Sketch) Frame() geom.Frame {
	if s.OnFace {
		return s.FrameSnap
	}
	return geom.PlaneFrame(s.Plane)
}

// Where names the sketch's plane for toasts and the tree.
func (s *Sketch) Where() string {
	if s.OnFace {
		return "a face"
	}
	return "the " + s.Plane.String() + " plane"
}

// Summary is the one-line description the sketch card shows.
func (s *Sketch) Summary() string {
	a := s.Arrangement()
	parts := fmt.Sprintf("%s · %s",
		plural(len(s.Entities), "entity", "entities"),
		plural(len(a.Regions), "region", "regions"))
	if n := len(a.OpenEnds); n > 0 {
		parts += fmt.Sprintf(" · %s", plural(n, "open end", "open ends"))
	}
	return parts
}

// ---------------------------------------------------------------------------
// Commands.

// AddEntity appends a drawn entity to a sketch.
type AddEntity struct {
	Sketch uint32
	Entity Entity

	index int
}

func (c *AddEntity) Name() string { return "Draw " + c.Entity.Kind.String() }

func (c *AddEntity) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if c.Entity.Degenerate() {
		return fmt.Errorf("that %s has no size", c.Entity.Kind)
	}
	c.index = len(s.Entities)
	s.Entities = append(s.Entities, c.Entity)
	s.Touch()
	return nil
}

func (c *AddEntity) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil || c.index >= len(s.Entities) {
		return
	}
	s.Entities = append(s.Entities[:c.index], s.Entities[c.index+1:]...)
	s.Touch()
}

func (c *AddEntity) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// DeleteEntities removes entities by index, restoring them in place on undo.
type DeleteEntities struct {
	Sketch  uint32
	Indices []int

	removed []Entity
	at      []int
}

func (c *DeleteEntities) Name() string {
	return "Delete " + plural(len(c.Indices), "entity", "entities")
}

func (c *DeleteEntities) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if len(c.Indices) == 0 {
		return fmt.Errorf("nothing selected to delete")
	}
	drop := map[int]bool{}
	for _, i := range c.Indices {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
		drop[i] = true
	}

	c.removed = c.removed[:0]
	c.at = c.at[:0]
	kept := make([]Entity, 0, len(s.Entities)-len(drop))
	for i, e := range s.Entities {
		if drop[i] {
			c.removed = append(c.removed, e)
			c.at = append(c.at, i)
			continue
		}
		kept = append(kept, e)
	}
	s.Entities = kept
	s.Touch()
	return nil
}

func (c *DeleteEntities) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	// Reinsert lowest index first so each lands back where it was.
	for k := range c.removed {
		i := c.at[k]
		if i > len(s.Entities) {
			i = len(s.Entities)
		}
		s.Entities = append(s.Entities, Entity{})
		copy(s.Entities[i+1:], s.Entities[i:])
		s.Entities[i] = c.removed[k]
	}
	s.Touch()
}

func (c *DeleteEntities) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// SetCircleSegs changes an n-gon's segment count, which the sketch card offers
// while a circle is selected (SPEC-UX §8.3).
type SetCircleSegs struct {
	Sketch uint32
	Index  int
	Segs   int

	prev int
}

func (c *SetCircleSegs) Name() string { return "Change circle segments" }

func (c *SetCircleSegs) Do(doc *Document) error {
	e, err := entityAt(doc, c.Sketch, c.Index)
	if err != nil {
		return err
	}
	if e.Kind != EntCircle {
		return fmt.Errorf("that entity is not a circle")
	}
	c.prev = e.Segs
	e.Segs = clampSegs(c.Segs)
	doc.SketchByID(c.Sketch).Touch()
	return nil
}

func (c *SetCircleSegs) Undo(doc *Document) {
	if e, err := entityAt(doc, c.Sketch, c.Index); err == nil {
		e.Segs = c.prev
		doc.SketchByID(c.Sketch).Touch()
	}
}

func (c *SetCircleSegs) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// MoveEntities shifts entities by a snapped delta, which is what dragging in
// the Select tool commits.
type MoveEntities struct {
	Sketch  uint32
	Indices []int
	Delta   geom.Vec2i
}

func (c *MoveEntities) Name() string {
	return "Move " + plural(len(c.Indices), "entity", "entities")
}

func (c *MoveEntities) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	for _, i := range c.Indices {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
	}
	for _, i := range c.Indices {
		s.Entities[i] = s.Entities[i].Translate(c.Delta)
	}
	s.Touch()
	return nil
}

func (c *MoveEntities) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	for _, i := range c.Indices {
		if i >= 0 && i < len(s.Entities) {
			s.Entities[i] = s.Entities[i].Translate(c.Delta.Neg())
		}
	}
	s.Touch()
}

func (c *MoveEntities) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// SetConstruction flips entities between real geometry and guides.
type SetConstruction struct {
	Sketch  uint32
	Indices []int
	On      bool

	prev []bool
}

func (c *SetConstruction) Name() string {
	verb := "Convert to construction"
	if !c.On {
		verb = "Convert to geometry"
	}
	return verb
}

func (c *SetConstruction) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if len(c.Indices) == 0 {
		return fmt.Errorf("nothing selected to convert")
	}
	// Validate every index before touching one, so a bad list changes nothing.
	for _, i := range c.Indices {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
	}
	c.prev = c.prev[:0]
	for _, i := range c.Indices {
		c.prev = append(c.prev, s.Entities[i].Construction)
		s.Entities[i].Construction = c.On
	}
	s.Touch()
	return nil
}

func (c *SetConstruction) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	for k, i := range c.Indices {
		if i >= 0 && i < len(s.Entities) && k < len(c.prev) {
			s.Entities[i].Construction = c.prev[k]
		}
	}
	s.Touch()
}

func (c *SetConstruction) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// ReplaceEntities removes and adds entities in one atomic, undoable step.
//
// It is the shape of every modify tool: fillet trims two lines and adds an
// arc, mirror adds copies, offset replaces a chain with a parallel one. Doing
// each as a remove-then-add pair would put two entries in the history for one
// gesture and leave a half-applied sketch if the second failed
// (Sketch_func.md §3).
type ReplaceEntities struct {
	Sketch uint32
	// Remove indexes the entities to drop, in any order.
	Remove []int
	Add    []Entity
	// Label is the undo name: what the user asked for, not what it did.
	Label string

	removed []Entity
	at      []int
	added   int
}

func (c *ReplaceEntities) Name() string {
	if c.Label != "" {
		return c.Label
	}
	return "Edit sketch"
}

func (c *ReplaceEntities) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if len(c.Remove) == 0 && len(c.Add) == 0 {
		return fmt.Errorf("that would change nothing")
	}
	// Everything that can fail is checked before anything moves.
	drop := map[int]bool{}
	for _, i := range c.Remove {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
		drop[i] = true
	}
	for _, e := range c.Add {
		if e.Degenerate() {
			return fmt.Errorf("that %s has no size", e.Kind)
		}
	}

	// Nothing below can fail.
	c.removed = c.removed[:0]
	c.at = c.at[:0]
	kept := make([]Entity, 0, len(s.Entities)-len(drop)+len(c.Add))
	for i, e := range s.Entities {
		if drop[i] {
			c.removed = append(c.removed, e)
			c.at = append(c.at, i)
			continue
		}
		kept = append(kept, e)
	}
	c.added = len(c.Add)
	s.Entities = append(kept, c.Add...)
	s.Touch()
	return nil
}

func (c *ReplaceEntities) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	// The additions went on the end, so they come off the end.
	if c.added > 0 && len(s.Entities) >= c.added {
		s.Entities = s.Entities[:len(s.Entities)-c.added]
	}
	// Then the removals go back where they were, lowest index first, which is
	// the order they were collected in.
	for k := range c.removed {
		i := c.at[k]
		if i > len(s.Entities) {
			i = len(s.Entities)
		}
		s.Entities = append(s.Entities, Entity{})
		copy(s.Entities[i+1:], s.Entities[i:])
		s.Entities[i] = c.removed[k]
	}
	s.Touch()
}

func (c *ReplaceEntities) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

func entityAt(doc *Document, sketch uint32, index int) (*Entity, error) {
	s := doc.SketchByID(sketch)
	if s == nil {
		return nil, fmt.Errorf("no sketch %d", sketch)
	}
	if index < 0 || index >= len(s.Entities) {
		return nil, fmt.Errorf("no entity %d in that sketch", index)
	}
	return &s.Entities[index], nil
}
