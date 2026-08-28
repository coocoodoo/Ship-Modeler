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

// The giant-stamp trap (the user's report, 2026-08-28, with their .pxm and
// settings attached): the res chip had drifted to 1, painted faces kept
// their own 8 px/u, and the first stamp on a BARE face silently allocated at
// the chip — a 32 px tile, 32 units wide. Painted faces already prompted on a
// density mismatch; bare faces on a painted body now do too, and the tile
// hint names the trap at the moment it matters.
func TestABareFaceOnAPaintedBodyWarnsBeforeTheFirstStamp(t *testing.T) {
	stdout, _ := runScript(t, "tile_resguard")
	if !strings.Contains(stdout, "resprompt offer=8 armed=1") {
		t.Error("hovering a bare face with the chip at 1 raised no prompt for the body's 8")
	}
	if !strings.Contains(stdout,
		`hint "Stamps here land at 1 px/u but the body is 8 — the panel offers the switch"`) {
		t.Error("the tile hint does not name the density trap")
	}
}

func TestGoldenTileResGuard(t *testing.T) {
	_, outDir := runScript(t, "tile_resguard")
	checkGolden(t, "tile_resguard", outDir)
}
