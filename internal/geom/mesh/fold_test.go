package mesh

import (
	"image"
	"reflect"
	"testing"

	"modeler/internal/geom"
)

// Folding bent faces (the user's request, 2026-08-28).
//
// When a direct edit leaves a face non-planar, the face should split into flat
// pieces along a real crease — new edges, new faces — instead of staying one
// "bent" polygon whose fold lands wherever the triangulation happens to put
// it. The crease people expect is the line separating what moved from what
// did not, which is exactly the information the edit has.

// seamBox is a 2x1x1 box with a vertical seam of vertices at x=1: the front
// and back faces are six-vertex loops through the seam, the top and bottom are
// split into two quads each. It is the shape from the user's screenshots — a
// face whose neighbours already have a crease vertex mid-edge.
//
//	verts: 0..7 the outer corners, 8..11 the seam.
func seamBox() *Mesh {
	m := &Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0, Z: 0}, {X: 2, Y: 0, Z: 0}, {X: 2, Y: 1, Z: 0}, {X: 0, Y: 1, Z: 0},
			{X: 0, Y: 0, Z: 1}, {X: 2, Y: 0, Z: 1}, {X: 2, Y: 1, Z: 1}, {X: 0, Y: 1, Z: 1},
			{X: 1, Y: 0, Z: 1}, {X: 1, Y: 1, Z: 1}, // seam, front
			{X: 1, Y: 0, Z: 0}, {X: 1, Y: 1, Z: 0}, // seam, back
		},
	}
	loops := [][]int{
		{4, 8, 5, 6, 9, 7},   // front +Z, through the seam
		{0, 3, 11, 2, 1, 10}, // back -Z, through the seam
		{7, 9, 11, 3},        // top left
		{9, 6, 2, 11},        // top right
		{4, 0, 10, 8},        // bottom left
		{8, 10, 1, 5},        // bottom right
		{0, 4, 7, 3},         // left -X
		{1, 2, 6, 5},         // right +X
	}
	for i, l := range loops {
		m.Faces = append(m.Faces, Face{ID: MakeFaceUID(1, uint32(i+1)), Loops: [][]int{l}})
	}
	return m
}

// uidCounter is the identity mint a test hands to Fold.
func uidCounter(start uint32) func() FaceUID {
	seq := start
	return func() FaceUID {
		seq++
		return MakeFaceUID(1, seq)
	}
}

func TestSeamBoxIsAValidSolid(t *testing.T) {
	if err := Validate(seamBox()); err != nil {
		t.Fatalf("the fixture itself is broken: %v", err)
	}
}

func TestFoldSplitsAlongTheSeamBetweenMovedAndStill(t *testing.T) {
	m := seamBox()
	// Bend the front-left edge outward, as in the screenshots: verts 4 and 7
	// move along +Z. Only the front face bends — the left, top-left and
	// bottom-left faces all contain the move direction in their own planes.
	d := geom.Vec3{Z: 0.75}
	moved := map[int]bool{4: true, 7: true}
	m.Verts[4] = m.Verts[4].Add(d)
	m.Verts[7] = m.Verts[7].Add(d)
	m.InvalidateCaches()

	folded := FoldBent(m, moved, uidCounter(100))
	if folded != 1 {
		t.Fatalf("folded %d faces, want exactly 1 (the front)", folded)
	}
	if len(m.Faces) != 9 {
		t.Fatalf("mesh has %d faces after the fold, want 9", len(m.Faces))
	}
	if bent := m.RecheckPlanarity(); bent != 0 {
		t.Errorf("%d faces are still bent after folding", bent)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("the folded mesh is not a valid solid: %v", err)
	}

	// The crease runs along the seam: the two pieces both border verts 8 and 9,
	// and the moved pair sits together in one piece.
	both := 0
	movedPiece := false
	for fi := range m.Faces {
		has := map[int]bool{}
		for _, vi := range m.Faces[fi].Outer() {
			has[vi] = true
		}
		if has[8] && has[9] {
			both++
			if has[4] && has[7] {
				movedPiece = true
			}
		}
	}
	if both != 2 {
		t.Errorf("%d faces border the seam 8-9, want the 2 fold pieces", both)
	}
	if !movedPiece {
		t.Error("no fold piece holds the moved edge — the crease is somewhere else")
	}
	// The fold added material: the front-left panel now leans outward.
	if v := Volume(m); v <= 2.0 {
		t.Errorf("volume is %v after leaning a panel outward, want more than the box's 2", v)
	}
}

func TestFoldOfACornerPullMakesTriangles(t *testing.T) {
	m := Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1)
	// Pull the max corner outward off every plane it belongs to. All three of
	// its faces bend; each must fold along the diagonal that skips the moved
	// vertex, leaving two triangles apiece.
	moved := map[int]bool{7: true}
	m.Verts[7] = m.Verts[7].Add(geom.Vec3{X: 0.3, Y: 0.4, Z: 0.5})
	m.InvalidateCaches()

	folded := FoldBent(m, moved, uidCounter(100))
	if folded != 3 {
		t.Fatalf("folded %d faces, want 3", folded)
	}
	if len(m.Faces) != 9 {
		t.Fatalf("mesh has %d faces, want 9 (three quads kept, three split in two)", len(m.Faces))
	}
	if bent := m.RecheckPlanarity(); bent != 0 {
		t.Errorf("%d faces are still bent", bent)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("folded mesh invalid: %v", err)
	}
	if v := Volume(m); v <= 1.0 {
		t.Errorf("volume is %v after pulling a corner out, want more than 1", v)
	}
}

func TestFoldLeavesPlanarFacesAlone(t *testing.T) {
	m := Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1)
	// Slide the whole top face upward: every face stays planar (the sides
	// stretch within their own planes), so there is nothing to fold.
	moved := map[int]bool{2: true, 3: true, 6: true, 7: true}
	for vi := range moved {
		m.Verts[vi] = m.Verts[vi].Add(geom.Vec3{Y: 2})
	}
	m.InvalidateCaches()

	if folded := FoldBent(m, moved, uidCounter(100)); folded != 0 {
		t.Fatalf("folded %d faces of a stretch that bent nothing", folded)
	}
	if len(m.Faces) != 6 {
		t.Errorf("face count changed to %d on a no-op fold", len(m.Faces))
	}
}

func TestFoldPiecesInheritPaintAndLineage(t *testing.T) {
	m := seamBox()
	paint := &FacePaint{Res: 8, Texel: 1.0 / 8, Frame: m.FaceFrame(0), Img: image.NewRGBA(image.Rect(0, 0, 4, 4))}
	m.Faces[0].Paint = paint
	// A face that is itself a boolean fragment carries its source's identity;
	// the fold pieces must keep pointing at the original source, not at the
	// intermediate fragment.
	src := MakeFaceUID(1, 77)
	m.Faces[0].SrcFace = src

	moved := map[int]bool{4: true, 7: true}
	m.Verts[4] = m.Verts[4].Add(geom.Vec3{Z: 0.75})
	m.Verts[7] = m.Verts[7].Add(geom.Vec3{Z: 0.75})
	m.InvalidateCaches()

	if folded := FoldBent(m, moved, uidCounter(100)); folded != 1 {
		t.Fatalf("folded %d, want 1", folded)
	}
	pieces := 0
	seen := map[FaceUID]bool{}
	for fi := range m.Faces {
		f := &m.Faces[fi]
		if seen[f.ID] {
			t.Errorf("face id %v appears twice", f.ID)
		}
		seen[f.ID] = true
		if f.Paint == paint {
			pieces++
			if f.SrcFace != src {
				t.Errorf("piece lineage is %v, want the original source %v", f.SrcFace, src)
			}
		}
	}
	if pieces != 2 {
		t.Errorf("%d faces share the source paint, want the 2 fold pieces", pieces)
	}
}

func TestFoldPiecesFallBackToTheirOwnIDAsLineage(t *testing.T) {
	m := seamBox()
	orig := m.Faces[0].ID
	moved := map[int]bool{4: true, 7: true}
	m.Verts[4] = m.Verts[4].Add(geom.Vec3{Z: 0.75})
	m.Verts[7] = m.Verts[7].Add(geom.Vec3{Z: 0.75})
	m.InvalidateCaches()

	if folded := FoldBent(m, moved, uidCounter(100)); folded != 1 {
		t.Fatalf("folded %d, want 1", folded)
	}
	pieces := 0
	for fi := range m.Faces {
		if m.Faces[fi].SrcFace == orig {
			pieces++
		}
	}
	if pieces != 2 {
		t.Errorf("%d pieces trace back to the original face, want 2", pieces)
	}
}

func TestFoldSkipsFacesWithHoles(t *testing.T) {
	// A lone annulus, bent at one corner. Folding a holed face means chords
	// that must dodge the hole; v1 declines rather than guessing, and the face
	// simply stays flagged as bent (SPEC-GEOMETRY §7.2).
	m := &Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0, Z: 0}, {X: 4, Y: 0, Z: 0}, {X: 4, Y: 4, Z: 0}, {X: 0, Y: 4, Z: 0.9},
			{X: 1, Y: 1, Z: 0}, {X: 1, Y: 3, Z: 0}, {X: 3, Y: 3, Z: 0}, {X: 3, Y: 1, Z: 0},
		},
		Faces: []Face{{
			ID:    MakeFaceUID(1, 1),
			Loops: [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}},
		}},
	}
	if folded := FoldBent(m, map[int]bool{3: true}, uidCounter(100)); folded != 0 {
		t.Fatalf("folded %d holed faces, want 0", folded)
	}
	if len(m.Faces) != 1 {
		t.Errorf("the holed face was rewritten anyway: %d faces", len(m.Faces))
	}
}

func TestFoldIsDeterministic(t *testing.T) {
	build := func() *Mesh {
		m := Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1)
		m.Verts[7] = m.Verts[7].Add(geom.Vec3{X: 0.3, Y: 0.4, Z: 0.5})
		m.InvalidateCaches()
		FoldBent(m, map[int]bool{7: true}, uidCounter(100))
		return m
	}
	a, b := build(), build()
	if len(a.Faces) != len(b.Faces) {
		t.Fatalf("two identical folds produced %d and %d faces", len(a.Faces), len(b.Faces))
	}
	for i := range a.Faces {
		if a.Faces[i].ID != b.Faces[i].ID || !reflect.DeepEqual(a.Faces[i].Loops, b.Faces[i].Loops) {
			t.Fatalf("face %d differs between two identical folds", i)
		}
	}
}
