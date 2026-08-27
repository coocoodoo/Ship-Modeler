package apptest

import (
	"math"
	"testing"
)

// M9 fixes, each one reported by the user and each one kept honest by a test
// that fails on the code as it was.

func TestGoldenThroughCut(t *testing.T) {
	_, outDir := runScript(t, "m9_throughcut")
	checkGolden(t, "m9_throughcut", outDir)
}

// TestThroughAllCutsAllTheWayThrough is the bug the user reported: "the
// subtract function when extruding a sketch is buggy, not working".
//
// Through all measured its reach as the distance to the furthest corner of the
// scene and ran that far in one direction. On a sketch drawn on one of the three
// default planes — which all pass through the origin, and so through the middle
// of most ships — that starts the cut *inside* the material and leaves a blind
// pocket where a hole was asked for. It cut exactly half.
//
// The hull is 2 by 2 by 6 through the middle: a real hole takes 24 units of
// material, and the old behaviour took 12.
func TestThroughAllCutsAllTheWayThrough(t *testing.T) {
	stdout, _ := runScript(t, "m9_throughcut")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, pending, after := dumps[0], dumps[1], dumps[2]

	if !pending.extrude.present || !pending.extrude.through {
		t.Fatalf("the tool was not set to through all: %+v", pending.extrude)
	}
	if pending.extrude.result != "Subtract" {
		t.Fatalf("the tool is set to %q, not Subtract", pending.extrude.result)
	}

	removed := before.body(t, "Hull").vol - after.body(t, "Hull").vol
	if math.Abs(removed-24) > 1e-9 {
		t.Errorf("a through cut removed %v units of hull, want the whole 2x2x6 hole "+
			"of 24 — half of that means it only cut forward from the sketch plane",
			removed)
	}
	// And it really is a hole, not a slot taken out of one side: a through hole
	// leaves the body one shell with a tunnel in it.
	if after.body(t, "Hull").tris <= before.body(t, "Hull").tris {
		t.Errorf("the cut did not add any geometry (%d triangles, was %d)",
			after.body(t, "Hull").tris, before.body(t, "Hull").tris)
	}
}

func TestGoldenSampleShip(t *testing.T) {
	_, outDir := runScript(t, "m9_sample")
	checkGolden(t, "m9_sample", outDir)
}

// TestTheSampleShipBuildsEndToEnd is the full-app end-to-end of PLAN M9: every
// tool the program has, driven in order, on the one document a first-time user
// is shown.
//
// It is the same script the welcome screen runs, which is the point of the
// sample being a script at all — what a new user sees is built by the path a
// test drives, so it cannot rot without this going red.
func TestTheSampleShipBuildsEndToEnd(t *testing.T) {
	stdout, _ := runScript(t, "m9_sample")
	dumps := parseM3Dumps(t, stdout)
	paints := parseM7Dumps(t, stdout)
	if len(dumps) != 1 {
		t.Fatalf("expected 1 dump, got %d:\n%s", len(dumps), stdout)
	}
	built := dumps[0]

	// One body: the hull, the wings and both engines unioned into it, with the
	// cockpit added on top of a sketch drawn on its own face. Two bodies would
	// mean a boolean quietly fell back to New.
	if len(built.bodies) != 1 {
		t.Fatalf("the sample built %d bodies, want 1 — a union fell back to a new body",
			len(built.bodies))
	}
	ship := built.bodies[0]
	if ship.tris < 100 {
		t.Errorf("the ship has %d triangles, which is too few to be the whole thing",
			ship.tris)
	}
	// A solid with a real volume, not a shell or a sliver.
	if ship.vol < 900 || ship.vol > 1200 {
		t.Errorf("the ship's volume is %v, outside the range the script builds", ship.vol)
	}

	// Six sketches, all consumed and hidden, and the planes put away — the
	// state the welcome screen hands over.
	if len(built.sketchVisible) != 6 {
		t.Errorf("%d sketches were left behind, want the 6 the script draws",
			len(built.sketchVisible))
	}
	for name, visible := range built.sketchVisible {
		if visible {
			t.Errorf("%s is still shown after being extruded", name)
		}
	}

	// And it is painted: three faces, one of them the dithered ramp.
	if len(paints[0].pictures) != 3 {
		t.Errorf("%d faces came out painted, want 3", len(paints[0].pictures))
	}
	for _, p := range paints[0].pictures {
		if p.opaque == 0 {
			t.Errorf("face %d has a texture with nothing on it", p.face)
		}
	}
}

// TestGoldenGridStep pins the grid-step control (the user's request,
// 2026-08-27): the same sketch drawn over a half-unit grid and a two-unit one.
// The two shots must differ from each other — a step that changed nothing on
// screen would mean the chips are decoration.
func TestGoldenGridStep(t *testing.T) {
	_, outDir := runScript(t, "m9_grid")
	checkGolden(t, "m9_grid", outDir)

	half, err := LoadPNG(outDir + "/m9_grid_half.png")
	if err != nil {
		t.Fatal(err)
	}
	two, err := LoadPNG(outDir + "/m9_grid_two.png")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Compare(half, two)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Differs() {
		t.Error("a 0.5 u grid and a 2 u grid rendered identically — the step is not reaching the drawn grid")
	}
}
