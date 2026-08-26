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

const (
	// EntLine is a single straight segment.
	EntLine EntityKind = iota
	// EntRect is an axis-aligned rectangle, stored by opposite corners and
	// expanded to four segments.
	EntRect
	// EntCircle is a regular polygon inscribed in a radius: a circle in this
	// app is an n-gon, which is what keeps the result crisp and low-poly (D-06).
	EntCircle
)

func (k EntityKind) String() string {
	switch k {
	case EntRect:
		return "rectangle"
	case EntCircle:
		return "circle"
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
// a line uses A and B, a rectangle uses A and B as opposite corners, and a
// circle uses C, R and Segs.
type Entity struct {
	Kind EntityKind `json:"kind"`
	A    geom.Vec2i `json:"a,omitempty"`
	B    geom.Vec2i `json:"b,omitempty"`
	C    geom.Vec2i `json:"c,omitempty"`
	R    int64      `json:"r,omitempty"`
	Segs int        `json:"segs,omitempty"`
}

// NewLine builds a line entity.
func NewLine(a, b geom.Vec2i) Entity { return Entity{Kind: EntLine, A: a, B: b} }

// NewRect builds a rectangle from two opposite corners.
func NewRect(a, b geom.Vec2i) Entity { return Entity{Kind: EntRect, A: a, B: b} }

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
	default:
		return e.A == e.B
	}
}

// Points returns the entity's corner positions in draw order. For a line that
// is its two endpoints, for a rectangle its four corners, for a circle its
// n-gon vertices.
func (e Entity) Points() []geom.Vec2i {
	switch e.Kind {
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
func (e Entity) Closed() bool { return e.Kind != EntLine }

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
func (s *Sketch) Segments() []sketch2d.Seg {
	var out []sketch2d.Seg
	for i := range s.Entities {
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
