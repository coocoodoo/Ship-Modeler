package apptest

import (
	"math"
	"testing"
)

// SK3: polygons and slots (Sketch_func.md §5 SK3), in the shape they exist
// for — cut through a plate.

func TestGoldenSlotAndPolygons(t *testing.T) {
	_, outDir := runScript(t, "sk3_shapes")
	checkGolden(t, "sk3_shapes", outDir)
}

// The acceptance: a slot, a hexagon and a circumscribed octagon cut clean
// through a 16×8×2 plate.
//
// Every area below comes from the polygon formula, so the numbers say what the
// shapes are rather than what a previous run happened to produce:
//
//	slot     a 10×3 track, plus a 16-gon of radius 1.5 split between its caps
//	hexagon  6 sides, corner radius 0.75
//	octagon  8 sides, circumscribed on radius 1 — corners at 1/cos(π/8)
//
// The octagon is the one that would fail quietly if circumscribed
// normalization were wrong: it would still be an octagon, just the wrong size,
// and only a number catches that.
func TestSlotAndPolygonsCutTheirTrueArea(t *testing.T) {
	stdout, _ := runScript(t, "sk3_shapes")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, after := dumps[0], dumps[1]

	plate := before.body(t, "Body 4")
	const plateVol = 16.0 * 8.0 * 2.0
	if math.Abs(plate.vol-plateVol) > 1e-6 {
		t.Fatalf("the plate is %.4f before the cut, want %.4f", plate.vol, plateVol)
	}

	slot := 10*3.0 + polygonArea(16, 1.5, 1.5)
	hexagon := polygonArea(6, 0.75, 0.75)
	octagonR := 1.0 / math.Cos(math.Pi/8)
	octagon := polygonArea(8, octagonR, octagonR)

	const thickness = 2.0
	want := plateVol - (slot+hexagon+octagon)*thickness

	got := after.body(t, "Body 4").vol
	if math.Abs(got-want) > 0.05 {
		t.Errorf("after the cut the plate is %.4f, want %.4f\n"+
			"  removed: slot %.4f + hexagon %.4f + octagon %.4f, through %.0f of plate",
			got, want, slot, hexagon, octagon, thickness)
	}
	// A cut that went through leaves walls behind: a plate that was 12
	// triangles is not one any more.
	if after.body(t, "Body 4").tris <= 12 {
		t.Errorf("the plate still has %d triangles — nothing was cut out of it",
			after.body(t, "Body 4").tris)
	}
}

// Three closed shapes, three regions, no loose ends: a slot's caps meet its
// sides and a polygon closes on its first vertex.
func TestSlotAndPolygonsAreClosedProfiles(t *testing.T) {
	stdout, _ := runScript(t, "sk3_shapes")
	dumps := parseSketchDumps(t, stdout)
	var cutting sketchDump
	for _, d := range dumps {
		if d.present && d.name == "Sketch 2" {
			cutting = d
		}
	}
	if !cutting.present {
		t.Fatalf("the cutting sketch was never dumped:\n%s", stdout)
	}
	if cutting.entities != 3 {
		t.Errorf("the cutting sketch holds %d entities, want 3", cutting.entities)
	}
	if cutting.regions != 3 {
		t.Errorf("it made %d regions, want 3 — each shape closes on its own", cutting.regions)
	}
	if cutting.openEnds != 0 {
		t.Errorf("it left %d open ends, want none", cutting.openEnds)
	}
}
