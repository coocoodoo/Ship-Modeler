package apptest

import (
	"math"
	"testing"
)

// SK1: the tool groups, construction geometry, points and the rectangle and
// line variants (Sketch_func.md §5 SK1), end to end through the real app.

func TestGoldenSketchToolVariants(t *testing.T) {
	_, outDir := runScript(t, "sk1_tools")
	checkGolden(t, "sk1_tools", outDir)
}

// The acceptance number for the whole milestone.
//
// The script draws a midpoint line, a centre rectangle, an aligned rectangle,
// a point, a construction rectangle and an ordinary one, then extrudes every
// region 2 units deep. What comes out says all of it at once:
//
//	centre rectangle   4 × 3            = 12
//	aligned rectangle  √20 × √5         = 10   (exactly: the offset lands on the lattice)
//	ordinary rectangle 4 × 3            = 12
//	                                      34 × 2 deep = 68
//
// The construction rectangle is 16 × 5 = 80 more area. If guides ever start
// closing regions this number becomes 228 and the test says so — which is the
// one thing about construction geometry that must never quietly change.
func TestSketchToolsProduceExtrudableRegions(t *testing.T) {
	stdout, _ := runScript(t, "sk1_tools")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, after := dumps[0], dumps[1]

	if len(before.bodies) != 3 {
		t.Fatalf("the sketch dump has %d bodies, want the 3 hidden test ones", len(before.bodies))
	}
	if len(after.bodies) != 4 {
		t.Fatalf("the extrude produced %d bodies, want 4", len(after.bodies))
	}

	got := after.bodies[3]
	const want = 68.0
	if math.Abs(got.vol-want) > 1e-6 {
		t.Errorf("extruded volume = %.4f, want %.4f.\n"+
			"228 would mean the construction rectangle closed a region;\n"+
			"46 or 56 would mean a rectangle variant did not.", got.vol, want)
	}
	if got.tris < 12 {
		t.Errorf("the solid has %d triangles — too few for three prisms", got.tris)
	}
}

// A point draws, saves and picks, but it is not geometry, and neither is a
// guide: neither may close a region or ring as an open end. The volume above
// proves they make no solid; this proves they do not spoil the profile either.
func TestPointsAndGuidesLeaveTheProfileAlone(t *testing.T) {
	stdout, _ := runScript(t, "sk1_tools")
	dumps := parseSketchDumps(t, stdout)
	if len(dumps) == 0 || !dumps[0].present {
		t.Fatalf("expected an active sketch in the first dump:\n%s", stdout)
	}
	s := dumps[0]

	// 4 lines of the aligned rectangle + midline + centre rect + point +
	// construction rect + ordinary rect.
	if s.entities != 9 {
		t.Errorf("the sketch holds %d entities, want 9", s.entities)
	}
	if s.construction != 1 {
		t.Errorf("%d entities are construction, want the one guide rectangle", s.construction)
	}
	if s.regions != 3 {
		t.Errorf("the region engine found %d regions, want 3 — "+
			"the two rectangle variants and the ordinary one, and no guide", s.regions)
	}
	// The midpoint line is the only thing in the sketch with loose ends: two of
	// them. A point ringed as an open end, or a construction rectangle counted
	// as one, would push this number up.
	if s.openEnds != 2 {
		t.Errorf("the sketch reports %d open ends, want the midpoint line's 2", s.openEnds)
	}
}
