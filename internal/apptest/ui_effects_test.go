package apptest

import "testing"

func TestUIEffects(t *testing.T) {
	_, out := runScript(t, "ui_effects")
	checkGolden(t, "ui_effects", out)
}
