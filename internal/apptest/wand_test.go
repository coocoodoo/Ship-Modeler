package apptest

import "testing"

// The magic wand (V-145): a whole-face gradient stroke confined to the
// wand-selected rectangle, with the selection outline visible. The golden is
// the proof that the mask reaches the tools and the outline reaches the eye.
func TestGoldenWand(t *testing.T) {
	_, outDir := runScript(t, "wand")
	checkGolden(t, "wand", outDir)
}
