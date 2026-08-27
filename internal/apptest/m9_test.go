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
