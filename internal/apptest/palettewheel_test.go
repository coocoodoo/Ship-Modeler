package apptest

import (
	"regexp"
	"strconv"
	"testing"
)

// One wheel notch over the palette list used to do two things at once: scroll
// the list and zoom the model behind it (reported 2026-09-04). A panel that
// scrolls claims the wheel over itself now, and gives it back when it closes.

var (
	zoomLine  = regexp.MustCompile(`zoom ortho=([\d.]+) dist=([\d.]+)`)
	firstLine = regexp.MustCompile(`palette open=\d .* first=(\d+)`)
)

func parseFloats(t *testing.T, re *regexp.Regexp, stdout string, group int) []float64 {
	t.Helper()
	var out []float64
	for _, m := range re.FindAllStringSubmatch(stdout, -1) {
		v, err := strconv.ParseFloat(m[group], 64)
		if err != nil {
			t.Fatalf("unparsable %q", m[0])
		}
		out = append(out, v)
	}
	return out
}

func TestTheWheelOverThePaletteScrollsWithoutZooming(t *testing.T) {
	stdout, _ := runScript(t, "palette_wheel")

	zooms := parseFloats(t, zoomLine, stdout, 1)
	if len(zooms) != 3 {
		t.Fatalf("expected 3 zoom dumps, got %d:\n%s", len(zooms), stdout)
	}
	rows := parseFloats(t, firstLine, stdout, 1)
	if len(rows) != 2 {
		t.Fatalf("expected 2 palette dumps, got %d:\n%s", len(rows), stdout)
	}

	// The list moved...
	if rows[1] <= rows[0] {
		t.Errorf("the wheel did not scroll the list: row %v then %v", rows[0], rows[1])
	}
	// ...and the camera did not.
	if zooms[1] != zooms[0] {
		t.Errorf("scrolling the palette zoomed the model: ortho %v became %v",
			zooms[0], zooms[1])
	}
	// With the panel gone the wheel is the camera's again, or the fix would
	// have cured the collision by breaking zoom.
	if zooms[2] == zooms[1] {
		t.Errorf("the wheel stopped zooming after the palette closed: ortho stuck at %v",
			zooms[2])
	}
}
