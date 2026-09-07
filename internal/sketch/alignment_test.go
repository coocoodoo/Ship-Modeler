package sketch

import (
	"modeler/internal/geom"
	"modeler/internal/model"
	"testing"
)

func TestEndpointAlignment(t *testing.T) {
	from := at(-5, -3)
	end := at(4.25, 4.5)
	ents := []model.Entity{model.NewLine(at(-2, 4.5), end)}
	for _, tc := range []struct {
		name      string
		raw, want geom.Vec2i
		axis      int
	}{
		{"vertical", at(4.4, -1.2), at(4.25, -1), 0},
		{"horizontal", at(7.2, 4.65), at(7, 4.5), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.raw, ents, &from, cfg())
			if got.Kind != SnapAlignment || got.Point != tc.want || got.Guides[tc.axis].From != end || !got.HasGuide() {
				t.Fatalf("alignment: %+v, want point %v tracking %v", got, tc.want, end)
			}
		})
	}
	// Tracking is opt-in to active placement, and Alt releases it entirely.
	raw := at(4.4, -1.2)
	if got := Resolve(raw, ents, nil, cfg()); got.Kind == SnapAlignment || got.HasGuide() {
		t.Fatalf("idle hover acquired a guide: %+v", got)
	}
	c := cfg()
	c.Suppressed = true
	if got := Resolve(raw, ents, &from, c); got.Point != raw || got.HasGuide() || got.Kind != SnapFree {
		t.Fatalf("Alt did not release alignment: %+v", got)
	}
}

func TestAlignmentCombinesAxesAndPreservesSnapPriority(t *testing.T) {
	from := at(-5, -3)
	x, y := at(4.25, 8), at(-2, 2.25)
	ents := []model.Entity{model.NewPoint(x), model.NewPoint(y)}
	got := Resolve(at(4.4, 2.4), ents, &from, cfg())
	if got.Point != at(4.25, 2.25) || got.Guides[0].From != x || got.Guides[1].From != y {
		t.Fatalf("crossing guides: %+v", got)
	}
	// A perpendicular remote guide can finish a horizontal current segment.
	got = Resolve(at(4.4, -2.95), ents, &from, cfg())
	if got.Point != at(4.25, -3) || got.Infer != InferHorizontal || got.Guides[0].From != x {
		t.Fatalf("current-segment inference: %+v", got)
	}
	for _, tc := range []struct {
		raw  geom.Vec2i
		kind SnapKind
	}{
		{at(4.3, 8.05), SnapEndpoint}, {at(1.2, 5.2), SnapMidpoint},
	} {
		got = Resolve(tc.raw, []model.Entity{model.NewLine(x, y)}, &from, cfg())
		if got.Kind != tc.kind || got.HasGuide() {
			t.Fatalf("point snap lost priority: %+v", got)
		}
	}
}

func TestAlignmentScreenRadiusAndFaceCorners(t *testing.T) {
	from := at(-5, -3)
	end := at(4, 8)
	for _, scale := range []float64{2, 10, 40} {
		c := DefaultConfig(scale)
		c.Reference = [][]geom.Vec2i{{end, at(9, 8), at(9, 12)}}
		for _, pixels := range []int64{5, 7} {
			raw := at(4, 0)
			raw.X += int64(float64(pixels) * scale)
			got := Resolve(raw, nil, &from, c)
			if (got.Guides[0].Axis == InferVertical) != (pixels == 5) {
				t.Fatalf("scale %v, offset %d pixels: %+v", scale, pixels, got)
			}
		}
	}
}
