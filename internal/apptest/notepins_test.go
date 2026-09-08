package apptest

import (
	"strings"
	"testing"
)

func TestNotePinsRightClickMenu(t *testing.T) {
	out, _ := runScript(t, "note_pins_menu")
	if strings.Count(out, "note-pin id=1 done=false attached=true at=-0.372,2.000,0.822") != 2 || strings.Contains(out, "note-pin id=2") {
		t.Fatalf("right-click pin lost its target or Cancel created a pin:\n%s", out)
	}
	if !strings.Contains(out, "Add a copper vent at this exact point.") {
		t.Fatal("right-click note text was not saved")
	}
}

func TestNotePinsWorkflow(t *testing.T) {
	out, _ := runScript(t, "note_pins")
	if strings.Count(out, "note-pin id=1") != 5 || strings.Count(out, "note-pin id=1 done=true attached=true") != 3 {
		t.Fatalf("pin creation, completion or history failed:\n%s", out)
	}
}

func TestNotePinsMousePlacement(t *testing.T) {
	out, _ := runScript(t, "note_pins_mouse")
	if strings.Count(out, "note-pin id=1 done=false attached=true") != 2 || strings.Contains(out, "note-pin id=2") || !strings.Contains(out, "Make this panel copper, with two narrow cyan vents.") {
		t.Fatalf("mouse placement, note entry, or cancel failed:\n%s", out)
	}
}
