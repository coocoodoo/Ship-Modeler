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
