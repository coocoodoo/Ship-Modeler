package apptest

import (
	"math"
	"testing"
)

// SK2: arcs, ellipses and the 3-point circle (Sketch_func.md §5 SK2).

func TestGoldenSketchCurves(t *testing.T) {
	_, outDir := runScript(t, "sk2_curves")
	checkGolden(t, "sk2_curves", outDir)
}

// polygonArea is the area of a regular n-gon of radius r — which is what every
// "circle" in this program actually is (D-06). For an ellipse, pass its two
// semi-axes: the same formula with r² replaced by a·b.
func polygonArea(n int, a, b float64) float64 {
	return 0.5 * float64(n) * a * b * math.Sin(2*math.Pi/float64(n))
}

// The acceptance for the milestone, and the reason arcs are worth having: a
// profile of straight lines closed by an arc extrudes, and the solid is the
// size the geometry says it should be.
//
// The script draws three profiles and pulls them all 2 units deep:
//
//	a D-shape   6×3 rectangle + half a 32-gon of radius 3
//	a circle    32-gon of radius 2, fitted through three clicked points
//	an ellipse  32-gon with semi-axes 5 and 2
//
// Every number below comes from the polygon formula rather than from a
// recorded run, so a tessellation that quietly changed density or dropped a
// segment fails here rather than passing on a stale baseline.
func TestCurvedProfilesExtrudeToTheirTrueSize(t *testing.T) {
	stdout, _ := runScript(t, "sk2_curves")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	after := dumps[1]
	if len(after.bodies) != 4 {
		t.Fatalf("the extrude produced %d bodies, want 4", len(after.bodies))
	}

	const depth = 2.0
	dShape := 6*3 + polygonArea(32, 3, 3)/2
	circle := polygonArea(32, 2, 2)
	ellipse := polygonArea(32, 5, 2)
	want := (dShape + circle + ellipse) * depth

	got := after.bodies[3].vol
	if math.Abs(got-want) > 0.05 {
		t.Errorf("extruded volume = %.4f, want %.4f\n"+
			"  D-shape %.4f + circle %.4f + ellipse %.4f, doubled",
			got, want, dShape, circle, ellipse)
	}
}

// The contract that makes arcs usable at all: an arc's tessellation starts and
// ends exactly on the points it was built from, so lines drawn to those points
// meet it rather than nearly meeting it.
//
// A profile with three straight sides and one arc closes into a region only if
// that is true. One subunit of drift at either end and this is four open ends
// and no region — which is what a curved hull outline would silently become.
func TestAnArcClosesAProfileWithItsLines(t *testing.T) {
	stdout, _ := runScript(t, "sk2_curves")
	dumps := parseSketchDumps(t, stdout)
	if len(dumps) == 0 || !dumps[0].present {
		t.Fatalf("expected an active sketch in the first dump:\n%s", stdout)
	}
	s := dumps[0]

	if s.openEnds != 0 {
		t.Errorf("the sketch has %d open ends — an arc did not land on its lines", s.openEnds)
	}
	if s.regions != 3 {
		t.Errorf("the region engine found %d regions, want 3 "+
			"(the arc-closed D, the fitted circle and the ellipse)", s.regions)
	}
	if s.entities != 6 {
		t.Errorf("the sketch holds %d entities, want 6", s.entities)
	}
}
