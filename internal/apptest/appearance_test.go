package apptest

import "testing"

func TestAppearance(t *testing.T) {
	_, out := runScript(t, "appearance")
	checkGolden(t, "appearance", out)
}
