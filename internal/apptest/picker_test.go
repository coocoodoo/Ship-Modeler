package apptest

import (
	"regexp"
	"testing"
)

// The custom colour picker (V-156). Its widgets run in the deferred pass,
// after ColorPicker has already returned, so the result it handed back never
// carried what they did: dragging the field and clicking a preset both left
// the brush on whatever colour it started with.

var brushColor = regexp.MustCompile(`paint mode=\d .*? color="([0-9A-Fa-f]{6})"`)

func brushColors(t *testing.T, stdout string) []string {
	t.Helper()
	var out []string
	for _, m := range brushColor.FindAllStringSubmatch(stdout, -1) {
		out = append(out, m[1])
	}
	return out
}

// The picker's own appearance: the field tinted to the live hue, the marker
// on the hue bar, the ring on the chosen preset.
func TestGoldenPaintPicker(t *testing.T) {
	_, outDir := runScript(t, "paint_picker")
	checkGolden(t, "paint_picker", outDir)
}

func TestThePickerActuallyChangesTheBrushColour(t *testing.T) {
	stdout, _ := runScript(t, "paint_picker")
	cols := brushColors(t, stdout)
	if len(cols) != 4 {
		t.Fatalf("expected 4 dumps, got %d:\n%s", len(cols), stdout)
	}
	start, opened, dragged, preset := cols[0], cols[1], cols[2], cols[3]

	if start != "FF0000" {
		t.Fatalf("the script did not arm the colour it meant to: %s", start)
	}
	// Opening the picker changes nothing by itself.
	if opened != start {
		t.Errorf("merely opening the picker changed the colour to %s", opened)
	}
	// Dragging in the saturation/value field must move the brush colour.
	if dragged == start {
		t.Errorf("dragging the picker's field left the colour at %s", dragged)
	}
	// ...and so must clicking one of its preset swatches.
	if preset == dragged {
		t.Errorf("clicking a preset left the colour at %s", preset)
	}
}
