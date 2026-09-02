package apptest

import "testing"

// The live cut (V-150): a Subtract pending on the hull's top face draws the
// hull with the pocket already in it and the leaving material as a red ghost
// sitting in the hole. The golden is the proof the preview shows the result
// rather than a plug hidden inside the hull.
func TestGoldenCutPreview(t *testing.T) {
	_, outDir := runScript(t, "m3_cutpreview")
	checkGolden(t, "m3_cutpreview", outDir)
}
