package apptest

import (
	"testing"
)

// The edge-line tool (the user's request, 2026-08-27): pick edges, choose a
// thickness, press the button, and a band is baked along them onto both faces
// that meet at each one.

func TestGoldenEdgePaint(t *testing.T) {
	_, outDir := runScript(t, "edge_paint")
	checkGolden(t, "edge_paint", outDir)
}

// The acceptance: every sharp edge of a box picked and painted in one press.
//
// A box has twelve of them, and each is shared by two faces — so the band has
// to reach all six pictures. What makes it worth a test rather than a
// screenshot is that the line is *baked*: it becomes ordinary paint in the
// faces' own images the moment it lands, which is what lets it survive saving,
// exporting, and being painted over.
func TestPaintingEveryEdgeOfABox(t *testing.T) {
	stdout, _ := runScript(t, "edge_paint")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) < 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	after := dumps[len(dumps)-1]

	if !hasToast(after.toasts, "Picked 12 edges") {
		t.Errorf("the crease pick did not find a box's 12 edges:\n%q", after.toasts)
	}
	// One press, one report, and the report says what it did.
	if !hasToast(after.toasts, "Painted 12 edges, 2 texels wide") {
		t.Errorf("the bake did not report what it did:\n%q", after.toasts)
	}
}
