package apptest

import (
	"regexp"
	"strings"
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
	// One press, one report, and the report says what it did — including what
	// the chosen pixel count came to in real size. That number is the answer to
	// "one pixel is still too thick": the face's resolution decides how big a
	// pixel is, and the tool cannot draw a finer one (V-126).
	if !hasToast(after.toasts, "Painted 12 edges, 2 pixels wide (0.5 u)") {
		t.Errorf("the bake did not report its width in pixels and in units:\n%q",
			after.toasts)
	}
}

// The user's report (2026-08-28): 3 px edge bands left stair-stepped gaps
// along slanted edges — the band never reached the edge it was asked to
// line. The stepped-wedge prism has diagonal profile edges on its front and
// back faces; the golden pins their bands flush against the silhouette, with
// the stair-step on the inner side only (V-135).
func TestGoldenSlantedEdgeBands(t *testing.T) {
	_, outDir := runScript(t, "edge_slant")
	checkGolden(t, "edge_slant", outDir)
}

// The click path headlessly: paint.pickedge routes through toggleEdge — the
// same entry a real click uses — so the chain pick (V-136) is on this path.
// On a body with no segmented lines the chain is the edge itself, and the
// bake names one edge.
func TestPickedgeRoutesThroughTheClickPath(t *testing.T) {
	stdout, _ := runScript(t, "edge_chain")
	if !strings.Contains(stdout, `toast "Painted 1 edge, 3 pixels wide (0.75 u)"`) {
		t.Error("picking one edge by index did not bake one edge")
	}
}

func TestGoldenEdgeChain(t *testing.T) {
	_, outDir := runScript(t, "edge_chain")
	checkGolden(t, "edge_chain", outDir)
}

// The user's file, rebuilt from its own feature history (2026-08-28, "still,
// didnt paint that face side"): the stepped shape, the ledge-end edge picked,
// and the band due on BOTH its faces — the ledge and the L-shaped right wall.
// The wall's vertex-average centroid sits exactly on that edge's line, which
// is what sent its band to the invisible side (V-137). The edge is 2 u at
// 8 px/u and 3 wide: 48 texels on each face, exactly.
func TestTheBandLandsOnBothFacesOfTheLedgeEndEdge(t *testing.T) {
	stdout, _ := runScript(t, "edge_lwall")
	for _, want := range []string{
		`facepaint body=1 face=8 faces=1 res=8 .* opaque=48`,  // the ledge
		`facepaint body=1 face=10 faces=1 res=8 .* opaque=48`, // the L-shaped wall
	} {
		if !regexp.MustCompile(want).MatchString(stdout) {
			t.Errorf("no match for %q in the paint dump — one side of the edge took no band", want)
		}
	}
}

func TestGoldenLWallBand(t *testing.T) {
	_, outDir := runScript(t, "edge_lwall")
	checkGolden(t, "edge_lwall", outDir)
}
