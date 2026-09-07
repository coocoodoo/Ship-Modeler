package apptest

import "testing"

func TestUISize(t *testing.T) {
	_, out := runScript(t, "ui_size")
	checkGolden(t, "ui_size", out)
}
