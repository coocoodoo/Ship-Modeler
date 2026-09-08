package apptest

import (
	"regexp"
	"testing"
)

func TestUVViewerPaintsLiveModel(t *testing.T) {
	out, dir := runScript(t, "uv_view")
	sums := regexp.MustCompile(`facepaint body=1 face=7 .* sum=([0-9a-f]+)`).FindAllStringSubmatch(out, -1)
	if len(sums) != 4 {
		t.Fatalf("expected four paint snapshots:\n%s", out)
	}
	if sums[0][1] == sums[1][1] || sums[0][1] != sums[2][1] || sums[1][1] != sums[3][1] {
		t.Fatal("UV mouse stroke, undo, and redo did not update the model texture")
	}
	zooms := regexp.MustCompile(`UV body=1 zoom=([0-9.]+)`).FindAllStringSubmatch(out, -1)
	if len(zooms) != 4 || zooms[0][1] != zooms[1][1] || zooms[1][1] != zooms[2][1] || zooms[2][1] == zooms[3][1] {
		t.Fatal("painting moved the sheet or wheel failed to zoom")
	}
	checkGolden(t, "uv_view", dir)
}
