package model

import (
	"encoding/json"
	"testing"

	"modeler/internal/geom"
)

// SK1's model layer: the point entity, construction geometry, and the generic
// ReplaceEntities command every modify tool will be built from
// (Sketch_func.md §5 SK1). Written before the code they describe.

// --- EntPoint -------------------------------------------------------------

func TestPointEntityIsOneNakedPosition(t *testing.T) {
	p := NewPoint(at(3, 4))
	pts := p.Points()
	if len(pts) != 1 || pts[0] != at(3, 4) {
		t.Fatalf("points = %v, want exactly [%v]", pts, at(3, 4))
	}
	if p.Closed() {
		t.Error("a point is not a closed loop")
	}
	// One position cannot make a segment, so it contributes none — and must not
	// panic reaching for a second point that is not there.
	if segs := p.AppendSegments(nil, 0); len(segs) != 0 {
		t.Errorf("a point produced %d segments, want none", len(segs))
	}
}

// A point at the origin is a real place to put one. Degeneracy is decided per
// kind, and the line rule (A == B) would condemn it, because a point never
// fills in B.
func TestPointAtTheOriginIsLegal(t *testing.T) {
	if NewPoint(geom.Vec2i{}).Degenerate() {
		t.Error("a point at the origin was called degenerate")
	}
	if NewPoint(at(1, 1)).Degenerate() {
		t.Error("a point away from the origin was called degenerate")
	}
}

func TestPointSurvivesTranslation(t *testing.T) {
	p := NewPoint(at(1, 2)).Translate(at(3, 0))
	if p.A != at(4, 2) {
		t.Errorf("translated point = %v, want %v", p.A, at(4, 2))
	}
}

// --- Construction ---------------------------------------------------------

// The whole reason construction geometry exists: it guides without becoming
// part of the shape. A construction rectangle around a profile must leave the
// region count exactly where it was.
func TestConstructionNeverMakesRegions(t *testing.T) {
	_, bus, s := sketchWith(t, NewRect(at(0, 0), at(4, 4)))
	if got := len(s.Arrangement().Regions); got != 1 {
		t.Fatalf("the ordinary rectangle made %d regions, want 1", got)
	}

	guide := NewRect(at(-2, -2), at(6, 6))
	guide.Construction = true
	if err := bus.Run(&AddEntity{Sketch: s.ID, Entity: guide}); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Arrangement().Regions); got != 1 {
		t.Errorf("a construction rectangle added %d regions; it must add none", got-1)
	}
	if got := len(s.Entities); got != 2 {
		t.Errorf("the sketch holds %d entities, want both", got)
	}
}

// Construction lines must not create open ends either: a loose guide line
// ringed in red would be telling the user to close something that is not
// meant to be closed.
func TestConstructionMakesNoOpenEnds(t *testing.T) {
	guide := NewLine(at(0, 0), at(5, 0))
	guide.Construction = true
	_, _, s := sketchWith(t, guide)
	if got := len(s.Arrangement().OpenEnds); got != 0 {
		t.Errorf("a construction line left %d open ends, want none", got)
	}
}

func TestSetConstructionIsUndoable(t *testing.T) {
	_, bus, s := sketchWith(t, NewRect(at(0, 0), at(4, 4)), NewLine(at(0, 0), at(1, 1)))

	if err := bus.Run(&SetConstruction{Sketch: s.ID, Indices: []int{0}, On: true}); err != nil {
		t.Fatalf("SetConstruction: %v", err)
	}
	if !s.Entities[0].Construction {
		t.Fatal("the entity was not marked construction")
	}
	if s.Entities[1].Construction {
		t.Error("an entity nobody named was marked too")
	}
	if got := len(s.Arrangement().Regions); got != 0 {
		t.Errorf("converting the only loop to construction left %d regions", got)
	}

	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if s.Entities[0].Construction {
		t.Error("undo left the entity as construction")
	}
	if got := len(s.Arrangement().Regions); got != 1 {
		t.Errorf("undo left %d regions, want the rectangle's 1 back", got)
	}
}

func TestSetConstructionRejectsBadIndices(t *testing.T) {
	_, bus, s := sketchWith(t, NewLine(at(0, 0), at(1, 1)))
	err := bus.Run(&SetConstruction{Sketch: s.ID, Indices: []int{7}, On: true})
	if err == nil {
		t.Fatal("an out-of-range index was accepted")
	}
	if len(s.Entities) != 1 || s.Entities[0].Construction {
		t.Error("a refused command still changed the sketch")
	}
}

// --- ReplaceEntities ------------------------------------------------------

func TestReplaceEntitiesSwapsInOneStep(t *testing.T) {
	_, bus, s := sketchWith(t,
		NewLine(at(0, 0), at(4, 0)),
		NewLine(at(4, 0), at(4, 4)),
		NewLine(at(4, 4), at(0, 0)))

	cmd := &ReplaceEntities{
		Sketch: s.ID,
		Remove: []int{0, 2},
		Add:    []Entity{NewCircle(at(2, 2), sub(1), 16)},
		Label:  "Fillet",
	}
	if err := bus.Run(cmd); err != nil {
		t.Fatalf("ReplaceEntities: %v", err)
	}
	if cmd.Name() != "Fillet" {
		t.Errorf("undo name = %q, want the label", cmd.Name())
	}
	if got := len(s.Entities); got != 2 {
		t.Fatalf("%d entities left, want 2 (one survivor + one addition)", got)
	}
	if s.Entities[0].Kind != EntLine || s.Entities[0].A != at(4, 0) {
		t.Errorf("the wrong line survived: %+v", s.Entities[0])
	}
	if s.Entities[1].Kind != EntCircle {
		t.Errorf("the addition is %v, want a circle", s.Entities[1].Kind)
	}
}

// Undo has to put the removed entities back at their original indices, in
// order — every later tool's undo depends on it, and a selection held across
// an undo points at indices.
func TestReplaceEntitiesUndoRestoresExactOrder(t *testing.T) {
	before := []Entity{
		NewLine(at(0, 0), at(4, 0)),
		NewLine(at(4, 0), at(4, 4)),
		NewLine(at(4, 4), at(0, 0)),
	}
	_, bus, s := sketchWith(t, before...)

	if err := bus.Run(&ReplaceEntities{
		Sketch: s.ID,
		Remove: []int{0, 2},
		Add:    []Entity{NewLine(at(9, 9), at(9, 8))},
		Label:  "Mirror",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if len(s.Entities) != len(before) {
		t.Fatalf("undo left %d entities, want %d", len(s.Entities), len(before))
	}
	for i := range before {
		if !s.Entities[i].Equal(before[i]) {
			t.Errorf("entity %d came back as %+v, want %+v", i, s.Entities[i], before[i])
		}
	}
}

// Atomicity: a command that cannot complete must change nothing at all, the
// same contract every other command on the bus keeps (SPEC-DATA §3.1).
func TestReplaceEntitiesIsAllOrNothing(t *testing.T) {
	_, bus, s := sketchWith(t, NewLine(at(0, 0), at(4, 0)))

	err := bus.Run(&ReplaceEntities{
		Sketch: s.ID,
		Remove: []int{0, 5}, // 5 does not exist
		Add:    []Entity{NewLine(at(1, 1), at(2, 2))},
		Label:  "Offset",
	})
	if err == nil {
		t.Fatal("a replace naming a missing entity was accepted")
	}
	if len(s.Entities) != 1 || s.Entities[0].A != at(0, 0) {
		t.Errorf("the refused replace still edited the sketch: %+v", s.Entities)
	}
}

func TestReplaceEntitiesRejectsDegenerateAdditions(t *testing.T) {
	_, bus, s := sketchWith(t, NewLine(at(0, 0), at(4, 0)))
	err := bus.Run(&ReplaceEntities{
		Sketch: s.ID,
		Add:    []Entity{NewLine(at(1, 1), at(1, 1))},
		Label:  "Pattern",
	})
	if err == nil {
		t.Fatal("a zero-length line was accepted")
	}
	if len(s.Entities) != 1 {
		t.Error("the refused replace still edited the sketch")
	}
}

// --- Serialization --------------------------------------------------------

// Every new field has to survive a save and a load, or the tools built on them
// draw shapes that vanish when the file comes back.
func TestNewEntityFieldsRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		ent  Entity
	}{
		{"line", NewLine(at(0, 0), at(4, 2))},
		{"rect", NewRect(at(0, 0), at(4, 2))},
		{"circle", NewCircle(at(1, 1), sub(2), 32)},
		{"point", NewPoint(at(3, 3))},
		{"construction line", func() Entity {
			e := NewLine(at(0, 0), at(1, 1))
			e.Construction = true
			return e
		}()},
		{"entity carrying the new scalars", Entity{
			Kind: EntLine, A: at(0, 0), B: at(1, 1),
			D: at(2, 2), W: sub(3), Pts: []geom.Vec2i{at(4, 4), at(5, 5)},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ent)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back Entity
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !back.Equal(tc.ent) {
				t.Errorf("round trip changed the entity:\n got %+v\nwant %+v", back, tc.ent)
			}
		})
	}
}

// An old file has none of the new fields. Reading one must leave them at their
// zero values rather than failing, which is what keeps every .ship written
// before SK1 openable.
func TestOldEntitiesStillRead(t *testing.T) {
	const old = `{"kind":0,"a":[256,512],"b":[768,1024]}`
	var e Entity
	if err := json.Unmarshal([]byte(old), &e); err != nil {
		t.Fatalf("an SK0-era entity would not read: %v", err)
	}
	if e.Kind != EntLine || e.A != at(1, 2) || e.B != at(3, 4) {
		t.Errorf("old entity read as %+v", e)
	}
	if e.Construction || e.W != 0 || len(e.Pts) != 0 || (e.D != geom.Vec2i{}) {
		t.Errorf("absent fields did not default to zero: %+v", e)
	}
}
