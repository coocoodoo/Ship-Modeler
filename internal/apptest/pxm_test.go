package apptest

import "testing"

// The orientation dots (V-131): placed by script, drawn in the viewport,
// listed in the tree. The golden is the whole assertion — a dot that stopped
// rendering, or a tree that lost its Markers section, moves pixels.
func TestGoldenMarkers(t *testing.T) {
	_, outDir := runScript(t, "pxm_markers")
	checkGolden(t, "pxm_markers", outDir)
}
