package apptest

import (
	"math"
	"testing"
)

// Shift+F looks square-on at the selected plane or face and frames it
// (V-153). F alone frames from wherever you are standing, which is right for
// a body and wrong for a flat thing: a plane seen at an angle is a
// parallelogram, and what is on it can only be read straight on.

func TestGoldenLookAt(t *testing.T) {
	_, outDir := runScript(t, "m0_lookat")
	checkGolden(t, "m0_lookat", outDir)
}

func TestLookAtFacesThePlaneItIsGiven(t *testing.T) {
	stdout, _ := runScript(t, "m0_lookat")
	cams := parseCameraDumps(t, stdout)
	if len(cams) != 3 {
		t.Fatalf("expected 3 camera dumps, got %d:\n%s", len(cams), stdout)
	}
	start, onPlane, onFace := cams[0], cams[1], cams[2]

	// The starting view is deliberately oblique, or this proves nothing.
	if math.Abs(start[0]) > 0.95 {
		t.Fatalf("the script did not start off-axis: forward %v", start)
	}

	// The Right plane's normal is +X, so looking at it means forward is -X.
	if !squareOn(onPlane, [3]float64{-1, 0, 0}) {
		t.Errorf("after looking at the Right plane the camera faces %v, want -X", onPlane)
	}
	// The hull's +y face: straight down. Azimuth is free when looking down the
	// pole, so only the Y component is pinned.
	if math.Abs(onFace[1]+1) > 0.02 {
		t.Errorf("after looking at the top face the camera faces %v, want -Y", onFace)
	}
}

// squareOn reports whether two directions agree to within half a degree.
func squareOn(got, want [3]float64) bool {
	dot := got[0]*want[0] + got[1]*want[1] + got[2]*want[2]
	return dot > math.Cos(0.5*math.Pi/180)
}
