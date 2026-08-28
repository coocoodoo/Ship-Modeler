package apptest

import "testing"

// Baked ambient occlusion (the user's request, 2026-08-28): the stepped
// shape with the strength at its default and at zero. The pair is the
// assertion — the concave junction reads darker on the first shot and the
// two must differ, which the baselines pin without any tolerance games.
func TestGoldenAmbientOcclusion(t *testing.T) {
	_, outDir := runScript(t, "ao_step")
	checkGolden(t, "ao_step", outDir)
}
