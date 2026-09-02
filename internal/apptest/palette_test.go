package apptest

import (
	"regexp"
	"strconv"
	"testing"
)

// The palette library (V-152): a searchable list of the shipped collection,
// and clicking a row swaps the custom page to it.

func TestGoldenPaletteLibrary(t *testing.T) {
	_, outDir := runScript(t, "palette_library")
	checkGolden(t, "palette_library", outDir)
}

type paletteDump struct {
	open    bool
	shown   int
	total   int
	query   string
	applied string
	colours int
}

var paletteLine = regexp.MustCompile(
	`palette open=(\d) shown=(\d+) of (\d+) query="([^"]*)" applied="([^"]*)" colours=(\d+)`)

func parsePaletteDumps(t *testing.T, stdout string) []paletteDump {
	t.Helper()
	var out []paletteDump
	for _, m := range paletteLine.FindAllStringSubmatch(stdout, -1) {
		n := func(s string) int { v, _ := strconv.Atoi(s); return v }
		out = append(out, paletteDump{
			open: m[1] == "1", shown: n(m[2]), total: n(m[3]),
			query: m[4], applied: m[5], colours: n(m[6]),
		})
	}
	return out
}

// The whole point of a search box over four thousand rows: it narrows, and
// what it narrows to is still a real set.
func TestSearchNarrowsTheLibrary(t *testing.T) {
	stdout, _ := runScript(t, "palette_library")
	d := parsePaletteDumps(t, stdout)
	if len(d) != 3 {
		t.Fatalf("expected 3 palette dumps, got %d:\n%s", len(d), stdout)
	}
	all, searched := d[0], d[1]

	if all.total < 1000 {
		t.Fatalf("the library only offered %d palettes", all.total)
	}
	if all.shown != all.total {
		t.Errorf("with no query the list showed %d of %d", all.shown, all.total)
	}
	if searched.query != "gameboy" {
		t.Fatalf("the search never reached the browser: query is %q", searched.query)
	}
	if searched.shown == 0 {
		t.Error("searching for gameboy found nothing, which cannot be right")
	}
	if searched.shown >= all.total {
		t.Errorf("the search narrowed nothing: %d of %d", searched.shown, all.total)
	}
	// The total never changes: a search hides rows, it does not unload them.
	if searched.total != all.total {
		t.Errorf("the library shrank from %d to %d while searching",
			all.total, searched.total)
	}
}

// Applying a row puts its colours on the brush's page and says which one is
// in use, so the list can point at it when it is opened again.
func TestApplyingAPaletteSwapsTheCustomPage(t *testing.T) {
	stdout, _ := runScript(t, "palette_library")
	d := parsePaletteDumps(t, stdout)
	if len(d) != 3 {
		t.Fatalf("expected 3 palette dumps, got %d", len(d))
	}
	before, after := d[0], d[2]

	if after.applied != "Nostalgia" {
		t.Errorf("the applied palette is %q, want Nostalgia", after.applied)
	}
	if after.colours == 0 {
		t.Error("applying a palette left no colours on the page")
	}
	if after.colours == before.colours && before.applied != "" {
		t.Error("the page did not change when a palette was applied")
	}
	// Closing the list leaves the choice standing: it is the panel's page now,
	// not something the browser was holding open.
	if after.open {
		t.Error("the browser is still open after the script closed it")
	}
}
