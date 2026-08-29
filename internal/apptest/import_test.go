package apptest

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Importing a mesh file (the user's request, 2026-08-28).
//
// The point of this feature is not "the triangles loaded" — that part is
// trivial and the io tests cover it. It is that what arrives is something this
// program can work with: a body whose faces are polygons, closed, and the
// right size. An import that produced six hundred triangular faces would pass
// a loading test and be useless in the app.

var importedBlock = regexp.MustCompile(
	`^body id=\d+ name="block" .* vol=([\d.]+) .* faces=(\d+) edges=(\d+) valid=(\d)$`)

func TestGoldenMeshImport(t *testing.T) {
	_, outDir := runScript(t, "import_mesh")
	checkGolden(t, "import_mesh", outDir)
}

func TestImportingAnSTLMergesItBackIntoPolygonFaces(t *testing.T) {
	stdout, _ := runScript(t, "import_mesh")

	var vol float64
	var faces, edges, valid int
	found := false
	for _, line := range strings.Split(stdout, "\n") {
		m := importedBlock.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		vol, _ = strconv.ParseFloat(m[1], 64)
		faces, _ = strconv.Atoi(m[2])
		edges, _ = strconv.Atoi(m[3])
		valid, _ = strconv.Atoi(m[4])
		found = true
	}
	if !found {
		t.Fatalf("the imported body never appeared:\n%s", stdout)
	}

	// Twelve triangles in, six rectangles out. This is the whole feature: a
	// face here is what paint paints on, what push/pull drags, and what a
	// sketch is started on, and none of those want a triangle.
	if faces != 6 || edges != 12 {
		t.Errorf("the imported block has %d faces and %d edges, want 6 and 12 — "+
			"the coplanar triangles were not merged", faces, edges)
	}
	if valid != 1 {
		t.Error("the import is not a valid closed solid, so booleans will refuse it")
	}
	// 40 x 20 x 10 file units at the script's scale of 0.4 is 16 x 8 x 4.
	if vol != 512 {
		t.Errorf("the imported block is %v units cubed, want 512 — the scale was "+
			"not applied as asked", vol)
	}
	if !strings.Contains(stdout, `toast "Imported block: 6 faces from 12 triangles"`) {
		t.Errorf("the import did not report what it did:\n%s", stdout)
	}
}
