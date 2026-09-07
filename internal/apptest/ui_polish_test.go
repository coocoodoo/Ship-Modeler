package apptest

import "testing"

func TestUIPolish(t *testing.T) {
	_, out := runScript(t, "ui_polish")
	checkGolden(t, "ui_polish", out)
}
