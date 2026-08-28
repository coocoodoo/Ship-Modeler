package sketch

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// SK3's session layer: polygons and slots (Sketch_func.md §5 SK3).

func TestPolygonToolTakesCentreThenVertex(t *testing.T) {
	s := session(ToolPolygon)
	s.Sides = 6
	if got := s.Click(at(0, 0)); got.Commit {
		t.Fatal("the first click committed; it only sets the centre")
	}
	ents := s.Click(at(4, 0)).Committed()
	if len(ents) != 1 || ents[0].Kind != model.EntPolygon {
		t.Fatalf("produced %+v, want one polygon", ents)
	}
	e := ents[0]
	if e.C != at(0, 0) || e.A != at(4, 0) {
		t.Errorf("centre/vertex = %v/%v", e.C, e.A)
	}
	if e.Segs != 6 {
		t.Errorf("sides = %d, want the session's 6", e.Segs)
	}
}

// The circumscribed variant is a different gesture producing the same kind:
// the click gives the distance to a flat side rather than to a corner.
func TestCircumscribedPolygonToolMeasuresToTheSide(t *testing.T) {
	build := func(tool Tool) model.Entity {
		s := session(tool)
		s.Sides = 6
		s.Click(at(0, 0))
		return s.Click(at(4, 0)).Committed()[0]
	}
	inscribed := build(ToolPolygon)
	circumscribed := build(ToolPolygonCirc)

	if circumscribed.Kind != model.EntPolygon {
		t.Fatalf("kind = %v, want a polygon — one kind, no variant flag", circumscribed.Kind)
	}
	in := inscribed.A.Sub(at(0, 0)).Len()
	out := circumscribed.A.Sub(at(0, 0)).Len()
	if out <= in {
		t.Errorf("circumscribed reach %.2f is not beyond inscribed %.2f", out, in)
	}
}

func TestPolygonRefusesNoRadius(t *testing.T) {
	s := session(ToolPolygon)
	s.Click(at(2, 2))
	if got := s.Click(at(2, 2)); got.Commit || got.Rejected == "" {
		t.Error("a polygon with no radius was accepted, or refused silently")
	}
}

// --- Slot -----------------------------------------------------------------

func TestSlotTakesTwoCentresThenAWidth(t *testing.T) {
	s := session(ToolSlot)
	if got := s.Click(at(0, 0)); got.Commit {
		t.Fatal("click 1 committed")
	}
	if got := s.Click(at(6, 0)); got.Commit {
		t.Fatal("click 2 committed; a slot needs a width too")
	}
	ents := s.Click(at(3, 2)).Committed()
	if len(ents) != 1 || ents[0].Kind != model.EntSlot {
		t.Fatalf("produced %+v, want one slot", ents)
	}
	e := ents[0]
	if e.A != at(0, 0) || e.B != at(6, 0) {
		t.Errorf("centres = %v..%v", e.A, e.B)
	}
	if math.Abs(float64(e.W-sub(2))) > 1.5 {
		t.Errorf("half-width = %d, want %d", e.W, sub(2))
	}
}

// The width is how far the third click lies across the axis, so clicking
// further along the slot does not make it fatter.
func TestSlotWidthIsMeasuredAcrossTheAxis(t *testing.T) {
	mk := func(third geom.Vec2i) model.Entity {
		s := session(ToolSlot)
		s.Click(at(0, 0))
		s.Click(at(6, 0))
		return s.Click(third).Committed()[0]
	}
	if a, b := mk(at(1, 2)), mk(at(5, 2)); a.W != b.W {
		t.Errorf("half-width came out %d and %d for the same reach across", a.W, b.W)
	}
}

func TestSlotRefusesNoWidth(t *testing.T) {
	s := session(ToolSlot)
	s.Click(at(0, 0))
	s.Click(at(6, 0))
	if got := s.Click(at(3, 0)); got.Commit || got.Rejected == "" {
		t.Error("a slot with no width was accepted, or refused silently")
	}
}

func TestSlotRefusesCoincidentCentres(t *testing.T) {
	s := session(ToolSlot)
	s.Click(at(2, 2))
	got := s.Click(at(2, 2))
	if got.Commit {
		t.Fatal("a slot with one centre was committed")
	}
	if got.Rejected == "" {
		t.Error("the refusal said nothing about why")
	}
}

// Both tools preview and stage like every other multi-click gesture.
func TestSK3GesturesStageAndPreview(t *testing.T) {
	cases := []struct {
		tool   Tool
		clicks []geom.Vec2i
		at     geom.Vec2i
	}{
		{ToolPolygon, []geom.Vec2i{at(0, 0)}, at(4, 0)},
		{ToolPolygonCirc, []geom.Vec2i{at(0, 0)}, at(4, 0)},
		{ToolSlot, []geom.Vec2i{at(0, 0), at(6, 0)}, at(3, 2)},
	}
	for _, tc := range cases {
		s := session(tc.tool)
		for _, p := range tc.clicks {
			s.Click(p)
		}
		if !s.Drawing() {
			t.Errorf("%v: not mid-gesture", tc.tool)
		}
		if got := s.PreviewAt(tc.at); !got.Show {
			t.Errorf("%v: no preview before the last click", tc.tool)
		}
	}
}
