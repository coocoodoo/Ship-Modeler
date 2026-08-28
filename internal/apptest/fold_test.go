package apptest

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Folding bent faces through the whole app (the user's request, 2026-08-28):
// select a corner vertex, drag it with the gizmo, and the faces that could not
// stay flat split into planar pieces along real creases — new faces, new
// edges — instead of remaining "bent" polygons folded wherever the
// triangulation happened to put the fold.

var foldBodyLine = regexp.MustCompile(
	`^body id=\d+ name="Wing pod" .* vol=([\d.]+) .* faces=(\d+) edges=(\d+) valid=(\d)$`)

// wingPodDumps returns the Wing pod's (volume, faces, edges, valid) at
// each dump. Edges counts what is drawn AND pickable — the two are one list.
func wingPodDumps(t *testing.T, stdout string) (vols []float64, faces, edges []int, valid []bool) {
	t.Helper()
	for _, raw := range strings.Split(stdout, "\n") {
		m := foldBodyLine.FindStringSubmatch(strings.TrimRight(raw, "\r"))
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("bad volume in %q: %v", raw, err)
		}
		f, _ := strconv.Atoi(m[2])
		e, _ := strconv.Atoi(m[3])
		vols = append(vols, v)
		faces, edges = append(faces, f), append(edges, e)
		valid = append(valid, m[4] == "1")
	}
	return
}

func TestBendingACornerFoldsTheFacesAlongRealCreases(t *testing.T) {
	stdout, _ := runScript(t, "fold_bend")
	vols, faces, _, valid := wingPodDumps(t, stdout)
	if len(faces) != 3 {
		t.Fatalf("got %d dumps of the Wing pod, want 3 — the script changed", len(faces))
	}
	for i, ok := range valid {
		if !ok {
			t.Fatalf("the Wing pod is not a valid solid at dump %d", i)
		}
	}

	// Before: a box, six faces. The pod is rotated so that a world-Y drag of
	// its top corner bends exactly two of the three faces meeting there — the
	// third is rotated about Y only, so its plane still contains the move.
	if faces[0] != 6 {
		t.Fatalf("the pod starts with %d faces, want 6", faces[0])
	}
	// After the move: each bent quad became two planar pieces.
	if faces[1] != 8 {
		t.Errorf("after the bend the pod has %d faces, want 8 — the bent faces did not fold", faces[1])
	}
	if !strings.Contains(stdout, `toast "Folded 2 bent faces along the crease"`) {
		t.Error("the fold never announced itself")
	}
	// Folding must not leave anything flagged bent, so the old warning has
	// nothing to say.
	if strings.Contains(stdout, "now bent") {
		t.Error("the bent-face warning fired even though the faces were folded flat")
	}
	// Pulling the corner up adds material.
	if vols[1] <= vols[0] {
		t.Errorf("volume went %v -> %v across an outward pull", vols[0], vols[1])
	}

	// One undo removes the whole thing: positions, pieces and creases.
	if faces[2] != 6 || vols[2] != vols[0] {
		t.Errorf("after undo the pod has %d faces and volume %v, want 6 and %v",
			faces[2], vols[2], vols[0])
	}
	if !strings.Contains(stdout, `toast "Undid: Move vertex"`) {
		t.Error("the undo did not name the move")
	}
}

// The mid-state shot: the pod with its corner pulled up and the two crease
// edges drawn where the folds happened. A fold that stopped producing crease
// edges, or that folded along a different chord, moves these pixels.
func TestGoldenFoldBend(t *testing.T) {
	_, outDir := runScript(t, "fold_bend")
	checkGolden(t, "fold_bend", outDir)
}

// The user's second report (2026-08-28): a gentle bend folded the face, but
// the crease "didn't process as an edge" — under CreaseAngleDeg it classified
// smooth, so it neither drew nor picked. Same-source pieces now crease at any
// real angle (V-134). The numbers say it all: a 0.6-unit corner nudge folds
// two faces (6 to 8) and the drawn-and-pickable edge list grows by exactly
// the two new creases (12 to 14).
func TestAGentleBendStillMakesGrabbableCreases(t *testing.T) {
	stdout, _ := runScript(t, "fold_shallow")
	_, faces, edges, valid := wingPodDumps(t, stdout)
	if len(faces) != 3 {
		t.Fatalf("got %d dumps, want 3", len(faces))
	}
	for i, ok := range valid {
		if !ok {
			t.Fatalf("invalid solid at dump %d", i)
		}
	}
	if faces[0] != 6 || edges[0] != 12 {
		t.Fatalf("the pod starts with %d faces and %d edges, want 6 and 12", faces[0], edges[0])
	}
	if faces[1] != 8 {
		t.Errorf("the gentle bend left %d faces, want 8 — it did not fold", faces[1])
	}
	if edges[1] != 14 {
		t.Errorf("after the fold %d edges draw, want 14 — the shallow creases are invisible and unpickable again", edges[1])
	}
	if faces[2] != 6 || edges[2] != 12 {
		t.Errorf("undo left %d faces and %d edges, want 6 and 12", faces[2], edges[2])
	}
}

func TestGoldenFoldShallow(t *testing.T) {
	_, outDir := runScript(t, "fold_shallow")
	checkGolden(t, "fold_shallow", outDir)
}
