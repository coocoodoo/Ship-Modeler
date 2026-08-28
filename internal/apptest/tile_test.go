package apptest

import (
	"regexp"
	"strings"
	"testing"
)

// Tile stamping through the whole app (Tile_paint.md TP2): import the
// committed sheet, slice it on the user's grid, stamp snapped, stamp turned,
// stamp a half-transparent tile, and undo. Every count is hand-computed.

var tilesetLine = regexp.MustCompile(`^tileset sheet=(\S+) grid=(\S+) tiles=(\d+) sel=(\d+) rot=(\d+) flip=(\d)$`)

func TestTileStampsSnapTurnAndUndo(t *testing.T) {
	stdout, _ := runScript(t, "tile_stamp")

	// The sheet slices on the user's grid chips: 19x19 at 8x8 with a 1 px
	// margin and gutter is exactly the four tiles the fixture draws.
	if !strings.Contains(stdout, "tileset sheet=19x19 grid=8x8+1+1 tiles=4 sel=2 rot=0 flip=0") {
		t.Error("the tileset did not slice to the fixture's four tiles")
	}
	if !strings.Contains(stdout, "tileset sheet=19x19 grid=8x8+1+1 tiles=4 sel=2 rot=2 flip=1") {
		t.Error("the orientation op did not arm rot 2 + flip")
	}

	// The opaque-texel counts across the four dumps that have paint:
	//   two snapped stamps of the 64-px tile . 128 (adjacent, no overlap)
	//   one turned stamp ................... 192
	//   the half-transparent tile .......... 224 (32 of its 64 pixels land)
	//   undo ............................... 192
	var opaque []string
	var sums []string
	re := regexp.MustCompile(`^facepaint .* opaque=(\d+) sum=([0-9a-f]+)$`)
	for _, raw := range strings.Split(stdout, "\n") {
		if m := re.FindStringSubmatch(strings.TrimRight(raw, "\r")); m != nil {
			opaque = append(opaque, m[1])
			sums = append(sums, m[2])
		}
	}
	want := []string{"128", "192", "224", "192"}
	if strings.Join(opaque, ",") != strings.Join(want, ",") {
		t.Fatalf("opaque counts %v, want %v", opaque, want)
	}
	// Undo is exact: the picture after undo hashes identically to the one
	// before the last stamp.
	if sums[3] != sums[1] {
		t.Errorf("undo hash %s differs from the pre-stamp hash %s", sums[3], sums[1])
	}
	// The first hash encodes the snapped POSITIONS, not just the counts: two
	// stamps aimed at uv (3,2) and (11,2) land at cells (0,0) and (8,0). A
	// broken snap keeps the counts and moves the pixels, and this catches it.
	if sums[0] != "9e2fcbf5" {
		t.Errorf("first stamp pair hashed %s, want 9e2fcbf5 — the stamps are not landing on the snapped cells", sums[0])
	}
	if !strings.Contains(stdout, `toast "Tileset loaded: 1 tile"`) {
		t.Error("the import toast is missing or counts wrongly")
	}
}

func TestGoldenTileStamp(t *testing.T) {
	_, outDir := runScript(t, "tile_stamp")
	checkGolden(t, "tile_stamp", outDir)
}

// The pointer path (Tile_paint.md TP3): with the Tile tool armed, a click on
// the model stamps through beginTileStamp -> the bus drag -> commit, exactly
// as a person would. The 8x8 blue tile is 64 opaque texels.
func TestAClickStampsThroughTheRealPointerPath(t *testing.T) {
	stdout, _ := runScript(t, "tile_panel")
	if !regexp.MustCompile(`facepaint body=1 face=\d+ faces=1 res=8 .* opaque=64`).MatchString(stdout) {
		t.Error("the click did not stamp 64 texels through the pointer path")
	}
	if !strings.Contains(stdout, `hint "Click to stamp · drag for a trail · Alt places free of the grid"`) {
		t.Error("the tile tool's hint is missing")
	}
}

// The panel and the ghost: the picker with the armed tile outlined, the grid
// chips, and the tile's own pixels previewed on the face under the cursor.
func TestGoldenTilePanel(t *testing.T) {
	_, outDir := runScript(t, "tile_panel")
	checkGolden(t, "tile_panel", outDir)
}

// One model, one pixel size (V-140, the user's demand: "I want them to be
// just 1 size, 1px... Not SCALE"). The flow drives the whole contract: the
// chips resample the entire model as one undoable step, and a stale chip —
// here deliberately desynced by undoing a resample — cannot make a bare
// face paint at a foreign density, because new paint always matches the
// paint that exists.
func TestOneModelOnePixelSize(t *testing.T) {
	stdout, _ := runScript(t, "tile_resguard")

	if !strings.Contains(stdout, `toast "Model resampled to 1 px/u"`) {
		t.Error("changing the chip with paint present did not resample the model")
	}
	// The resample really happened, wholesale, and the undo was exact.
	if !regexp.MustCompile(`facepaint body=1 face=3 faces=1 res=1 texel=1\.0+ `).MatchString(stdout) {
		t.Error("the painted face did not rebuild at 1 px/u")
	}
	sums := regexp.MustCompile(`facepaint body=1 face=3 .* sum=([0-9a-f]+)`).FindAllStringSubmatch(stdout, -1)
	if len(sums) < 3 || sums[0][1] != sums[2][1] {
		t.Error("undoing the resample did not restore the exact picture")
	}

	// The chip is now stale at 1 (the undo desynced it) — and the bare top
	// face still previews AND stamps at the model's 8, which is the whole
	// point: the chip cannot diverge the model's pixel size.
	hovers := regexp.MustCompile(`painthover body=1 face=2 [^
]*`).FindAllString(stdout, -1)
	if len(hovers) != 2 {
		t.Fatalf("want 2 hover lines, got %d", len(hovers))
	}
	if !strings.Contains(hovers[0], "res=8 allocated=0") {
		t.Errorf("the bare face previews as %q, want res=8 with the chip stale at 1", hovers[0])
	}
	if !strings.Contains(hovers[1], "res=8 allocated=1") {
		t.Errorf("the stamp landed as %q, want res=8", hovers[1])
	}
}

func TestGoldenTileResGuard(t *testing.T) {
	_, outDir := runScript(t, "tile_resguard")
	checkGolden(t, "tile_resguard", outDir)
}
