package apptest

import "testing"

// The shading toggle: the same painted box lit and then flat. The flat shot
// is the proof the uniform reaches the shader — lighting and AO gone, every
// texel of a colour identical — and the lit shot pins the default.
func TestGoldenShadingToggle(t *testing.T) {
	_, outDir := runScript(t, "view_shading")
	checkGolden(t, "view_shading", outDir)
}
