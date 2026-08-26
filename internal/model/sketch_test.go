package model

import (
	"testing"

	"modeler/internal/geom"
)

func sub(v float64) int64        { return geom.ToSubunits(v) }
func at(x, y float64) geom.Vec2i { return geom.Vec2i{X: sub(x), Y: sub(y)} }

func sketchWith(t *testing.T, ents ...Entity) (*Document, *Bus, *Sketch) {
	t.Helper()
	doc, bus := testDoc(t)
	cmd := &AddSketch{Plane: geom.PlaneFront}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	s := cmd.AddedSketch()
	for _, e := range ents {
		if err := bus.Run(&AddEntity{Sketch: s.ID, Entity: e}); err != nil {
			t.Fatalf("adding %v: %v", e.Kind, err)
		}
	}
	return doc, bus, s
}

func TestEntityPointsAndSegments(t *testing.T) {
	line := NewLine(at(0, 0), at(4, 0))
	if got := len(line.Points()); got != 2 {
		t.Errorf("line has %d points, want 2", got)
	}
	if got := len(line.AppendSegments(nil, 0)); got != 1 {
		t.Errorf("line expands to %d segments, want 1", got)
	}

	// A rectangle stores two corners but draws four sides.
	r := NewRect(at(0, 0), at(4, 3))
	pts := r.Points()
	if len(pts) != 4 {
		t.Fatalf("rect has %d points, want 4", len(pts))
	}
	segs := r.AppendSegments(nil, 7)
	if len(segs) != 4 {
		t.Fatalf("rect expands to %d segments, want 4", len(segs))
	}
	for i, s := range segs {
		if s.Src.Entity != 7 || s.Src.Seg != i {
			t.Errorf("segment %d is attributed to %+v", i, s.Src)
		}
	}

	// A circle is an n-gon, so its segment count is its side count.
	c := NewCircle(at(2, 2), sub(1), 16)
	if got := len(c.AppendSegments(nil, 0)); got != 16 {
		t.Errorf("16-gon expands to %d segments", got)
	}
	if got := len(NewCircle(at(0, 0), sub(1), 3).Points()); got != 3 {
		t.Errorf("triangle circle has %d points", got)
	}
}

func TestCircleSegsAreClamped(t *testing.T) {
	if got := NewCircle(at(0, 0), sub(1), 1).Segs; got != MinCircleSegs {
		t.Errorf("segs clamped to %d, want %d", got, MinCircleSegs)
	}
	if got := NewCircle(at(0, 0), sub(1), 999).Segs; got != MaxCircleSegs {
		t.Errorf("segs clamped to %d, want %d", got, MaxCircleSegs)
	}
}

func TestEntityDegenerate(t *testing.T) {
	cases := []struct {
		name string
		e    Entity
		want bool
	}{
		{"zero length line", NewLine(at(1, 1), at(1, 1)), true},
		{"real line", NewLine(at(0, 0), at(1, 0)), false},
		{"flat rect", NewRect(at(0, 0), at(4, 0)), true},
		{"thin rect", NewRect(at(0, 0), at(4, 1)), false},
		{"zero radius circle", NewCircle(at(0, 0), 0, 16), true},
		{"real circle", NewCircle(at(0, 0), sub(1), 16), false},
	}
	for _, c := range cases {
		if got := c.e.Degenerate(); got != c.want {
			t.Errorf("%s: Degenerate = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestArrangementIsTheAcceptanceShape covers M2's acceptance directly: a
// rectangle with a circle inside it gives two filled regions.
func TestArrangementIsTheAcceptanceShape(t *testing.T) {
	_, _, s := sketchWith(t,
		NewRect(at(0, 0), at(8, 6)),
		NewCircle(at(4, 3), sub(1.5), 16))

	a := s.Arrangement()
	if len(a.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(a.Regions))
	}
	if len(a.OpenEnds) != 0 {
		t.Errorf("open ends = %d, want none", len(a.OpenEnds))
	}
	if !a.Closed() {
		t.Error("the profile should report as closed")
	}
	if got := s.RegionCount(); got != 2 {
		t.Errorf("RegionCount = %d", got)
	}
	if got := s.EntityCount(); got != 2 {
		t.Errorf("EntityCount = %d", got)
	}
}

// TestArrangementCacheRebuildsOnEdit is what keeps the live fill honest: the
// cache must not outlive the edit that invalidated it.
func TestArrangementCacheRebuildsOnEdit(t *testing.T) {
	_, bus, s := sketchWith(t, NewRect(at(0, 0), at(4, 4)))
	if s.RegionCount() != 1 {
		t.Fatalf("regions = %d, want 1", s.RegionCount())
	}

	if err := bus.Run(&AddEntity{Sketch: s.ID, Entity: NewRect(at(6, 6), at(9, 9))}); err != nil {
		t.Fatal(err)
	}
	if got := s.RegionCount(); got != 2 {
		t.Errorf("after adding a second rect the cache still reports %d regions", got)
	}

	bus.Undo()
	if got := s.RegionCount(); got != 1 {
		t.Errorf("after undo the cache still reports %d regions", got)
	}
}

func TestOpenEndsGateTheProfile(t *testing.T) {
	_, bus, s := sketchWith(t,
		NewLine(at(0, 0), at(4, 0)),
		NewLine(at(4, 0), at(4, 4)),
		NewLine(at(4, 4), at(0, 4)))

	if s.RegionCount() != 0 {
		t.Errorf("an open profile has %d regions", s.RegionCount())
	}
	if got := s.OpenEndCount(); got != 2 {
		t.Errorf("open ends = %d, want 2", got)
	}

	// Closing the loop turns it into a region with nothing loose.
	if err := bus.Run(&AddEntity{Sketch: s.ID, Entity: NewLine(at(0, 4), at(0, 0))}); err != nil {
		t.Fatal(err)
	}
	if s.RegionCount() != 1 || s.OpenEndCount() != 0 {
		t.Errorf("after closing: %d regions, %d open ends", s.RegionCount(), s.OpenEndCount())
	}
}

func TestAddEntityRejectsDegenerate(t *testing.T) {
	_, bus, s := sketchWith(t)
	depth := bus.UndoDepth()
	if err := bus.Run(&AddEntity{Sketch: s.ID, Entity: NewLine(at(2, 2), at(2, 2))}); err == nil {
		t.Error("a zero-length line was accepted")
	}
	if len(s.Entities) != 0 {
		t.Error("a rejected entity was still added")
	}
	if bus.UndoDepth() != depth {
		t.Error("a rejected entity landed on the undo stack")
	}
}

func TestDeleteEntitiesRestoresPositions(t *testing.T) {
	_, bus, s := sketchWith(t,
		NewLine(at(0, 0), at(1, 0)),
		NewLine(at(1, 0), at(2, 0)),
		NewLine(at(2, 0), at(3, 0)),
		NewLine(at(3, 0), at(4, 0)))

	if err := bus.Run(&DeleteEntities{Sketch: s.ID, Indices: []int{1, 2}}); err != nil {
		t.Fatal(err)
	}
	if len(s.Entities) != 2 {
		t.Fatalf("after deleting two, %d entities remain", len(s.Entities))
	}
	if s.Entities[0].A != at(0, 0) || s.Entities[1].A != at(3, 0) {
		t.Errorf("the wrong entities survived: %v", s.Entities)
	}

	bus.Undo()
	if len(s.Entities) != 4 {
		t.Fatalf("undo restored %d entities", len(s.Entities))
	}
	for i, want := range []geom.Vec2i{at(0, 0), at(1, 0), at(2, 0), at(3, 0)} {
		if s.Entities[i].A != want {
			t.Errorf("entity %d came back as %v, want %v", i, s.Entities[i].A, want)
		}
	}
}

func TestDeleteEntitiesRejectsBadIndices(t *testing.T) {
	_, bus, s := sketchWith(t, NewLine(at(0, 0), at(1, 0)))
	if err := bus.Run(&DeleteEntities{Sketch: s.ID, Indices: []int{5}}); err == nil {
		t.Error("an out-of-range index was accepted")
	}
	if len(s.Entities) != 1 {
		t.Error("a rejected delete still removed something")
	}
	if err := bus.Run(&DeleteEntities{Sketch: s.ID}); err == nil {
		t.Error("deleting nothing was accepted")
	}
}

func TestMoveEntitiesIsUndoable(t *testing.T) {
	_, bus, s := sketchWith(t, NewRect(at(0, 0), at(4, 4)))
	d := at(2, 3)

	if err := bus.Run(&MoveEntities{Sketch: s.ID, Indices: []int{0}, Delta: d}); err != nil {
		t.Fatal(err)
	}
	if s.Entities[0].A != d {
		t.Errorf("after the move the corner is %v, want %v", s.Entities[0].A, d)
	}
	// The region moves with it and keeps its area.
	if s.RegionCount() != 1 {
		t.Errorf("after moving, regions = %d", s.RegionCount())
	}

	bus.Undo()
	if s.Entities[0].A != at(0, 0) {
		t.Errorf("undo left the corner at %v", s.Entities[0].A)
	}
}

func TestSetCircleSegs(t *testing.T) {
	_, bus, s := sketchWith(t, NewCircle(at(0, 0), sub(2), 16))

	if err := bus.Run(&SetCircleSegs{Sketch: s.ID, Index: 0, Segs: 32}); err != nil {
		t.Fatal(err)
	}
	if s.Entities[0].Segs != 32 {
		t.Errorf("segs = %d, want 32", s.Entities[0].Segs)
	}
	if got := len(s.Segments()); got != 32 {
		t.Errorf("the sketch now expands to %d segments", got)
	}
	bus.Undo()
	if s.Entities[0].Segs != 16 {
		t.Errorf("undo left segs at %d", s.Entities[0].Segs)
	}

	// Only circles have a segment count.
	_, bus2, s2 := sketchWith(t, NewLine(at(0, 0), at(1, 0)))
	if err := bus2.Run(&SetCircleSegs{Sketch: s2.ID, Index: 0, Segs: 8}); err == nil {
		t.Error("a line accepted a segment count")
	}
}

func TestSketchSummary(t *testing.T) {
	_, _, s := sketchWith(t, NewRect(at(0, 0), at(4, 4)))
	if got := s.Summary(); got != "1 entity · 1 region" {
		t.Errorf("summary = %q", got)
	}

	_, _, open := sketchWith(t, NewLine(at(0, 0), at(4, 0)))
	if got := open.Summary(); got != "1 entity · 0 regions · 2 open ends" {
		t.Errorf("open summary = %q", got)
	}
}

func TestEntityBoundsAndTranslate(t *testing.T) {
	r := NewRect(at(1, 2), at(5, 7))
	min, max := r.Bounds()
	if min != at(1, 2) || max != at(5, 7) {
		t.Errorf("bounds = %v..%v", min, max)
	}
	moved := r.Translate(at(2, 2))
	min, max = moved.Bounds()
	if min != at(3, 4) || max != at(7, 9) {
		t.Errorf("translated bounds = %v..%v", min, max)
	}

	c := NewCircle(at(0, 0), sub(2), 4)
	min, max = c.Bounds()
	if min.X != -sub(2) || max.X != sub(2) {
		t.Errorf("circle bounds = %v..%v", min, max)
	}
}
