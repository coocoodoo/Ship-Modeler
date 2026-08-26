package model

import (
	"math"
	"strings"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Direct edits and transforms (R10, R11, SPEC-GEOMETRY §7), written before the
// commands they test.
//
// The thing that makes these worth writing first is the planarity policy. A
// vertex move is trivial arithmetic; what is not trivial is what happens to the
// four-sided face that vertex belonged to, which is now bent. The answer has to
// be the same every time and has to reverse cleanly, because the alternative is
// a model that slowly fills up with faces nobody can sketch on any more.

// selVerts resolves a selection to vertex indices, which is what every direct
// edit ultimately moves.
func TestSelectionResolvesToVertices(t *testing.T) {
	doc, b, top := boxDoc(t)
	fi := faceIndex(b.Mesh, top)
	loop := b.Mesh.Faces[fi].Outer()

	cases := []struct {
		name string
		ref  Ref
		want int
	}{
		{"a vertex is itself", VertRef(b.ID, loop[0]), 1},
		{"an edge is its two ends", EdgeRef(b.ID, 0), 2},
		{"a face is its loop", FaceRef(b.ID, top), len(loop)},
		{"a body is all of it", BodyRef(b.ID), len(b.Mesh.Verts)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sel Selection
			sel.Set(c.ref)
			got := sel.VertIndices(doc)[b.ID]
			if len(got) != c.want {
				t.Errorf("resolved to %d vertices, want %d", len(got), c.want)
			}
		})
	}

	// Overlapping selections resolve to a set, not a list: a vertex shared by
	// two selected faces moves once, or the shape tears.
	var sel Selection
	sel.Set(FaceRef(b.ID, top))
	for i := range b.Mesh.Faces {
		if b.Mesh.FaceNormal(i).Dot(geom.AxisX) > 0.99 {
			sel.Add(FaceRef(b.ID, b.Mesh.Faces[i].ID))
		}
	}
	got := sel.VertIndices(doc)[b.ID]
	if len(got) != 6 {
		t.Errorf("two adjacent faces of a box share an edge: %d vertices, want 6", len(got))
	}
}

// TestMovingAFaceMovesTheWholeFace is R10 at its simplest: the four corners of
// a box's top go up together, and the box gets taller by exactly that much.
func TestMovingAFaceMovesTheWholeFace(t *testing.T) {
	doc, b, top := boxDoc(t)
	var sel Selection
	sel.Set(FaceRef(b.ID, top))

	cmd := NewMoveVerts(sel.VertIndices(doc), geom.Vec3{Y: 2}, "face")
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("MoveVerts: %v", err)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("the result is invalid: %v", err)
	}
	if got := mesh.Volume(b.Mesh); math.Abs(got-96) > 1e-9 {
		t.Errorf("volume = %v, want 4*4*6", got)
	}
	if box := b.Mesh.AABB(); math.Abs(box.Max.Y-6) > 1e-9 {
		t.Errorf("the top is at %v, want 6", box.Max.Y)
	}
	if n := b.Mesh.RecheckPlanarity(); n != 0 {
		t.Errorf("%d faces went non-planar moving a whole face straight up", n)
	}

	cmd.Undo(doc)
	if got := mesh.Volume(b.Mesh); math.Abs(got-64) > 1e-9 {
		t.Errorf("after undo, volume = %v, want 64", got)
	}
}

// TestMovingOneVertexBendsTheFacesAroundIt is the planarity policy of
// SPEC-GEOMETRY §7.2. Pulling one corner of a box out of plane bends the three
// faces that meet there, and the model has to say so rather than pretend.
func TestMovingOneVertexBendsTheFacesAroundIt(t *testing.T) {
	doc, b, _ := boxDoc(t)
	var sel Selection
	sel.Set(VertRef(b.ID, 0))

	cmd := NewMoveVerts(sel.VertIndices(doc), geom.Vec3{X: 1, Y: 1, Z: 1}, "vertex")
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("MoveVerts: %v", err)
	}
	bent := 0
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].NonPlanar {
			bent++
		}
	}
	if bent != 3 {
		t.Errorf("%d faces are flagged bent, want the 3 that meet at a corner", bent)
	}
	if cmd.Bent() != bent {
		t.Errorf("the command reports %d bent faces, the mesh says %d", cmd.Bent(), bent)
	}

	// Moving it back clears the flags: the policy is about where the vertices
	// are now, not about what has been done to them.
	cmd.Undo(doc)
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].NonPlanar {
			t.Fatalf("face %d is still flagged bent after undo", i)
		}
	}
}

// TestAMoveAlongAnAxisKeepsFacesFlat: sliding a whole box, or one of its
// vertical edges sideways, bends nothing.
func TestAMoveAlongAnAxisKeepsFacesFlat(t *testing.T) {
	doc, b, _ := boxDoc(t)
	var sel Selection
	sel.Set(BodyRef(b.ID))

	cmd := NewMoveVerts(sel.VertIndices(doc), geom.Vec3{X: 3}, "body")
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("MoveVerts: %v", err)
	}
	if cmd.Bent() != 0 {
		t.Errorf("moving the whole body bent %d faces", cmd.Bent())
	}
	if box := b.Mesh.AABB(); math.Abs(box.Min.X-3) > 1e-9 {
		t.Errorf("the body is at x %v, want 3", box.Min.X)
	}
	if got := mesh.Volume(b.Mesh); math.Abs(got-64) > 1e-9 {
		t.Errorf("moving changed the volume to %v", got)
	}
}

// TestNinetyDegreeRotationIsExact is SPEC-GEOMETRY §7.3: a quarter turn about a
// world axis is a permutation and a sign flip, so every vertex that was on the
// grid is still on it afterwards — no float rotation, no rounding, no drift.
func TestNinetyDegreeRotationIsExact(t *testing.T) {
	doc := NewDocument()
	id := doc.Seq.NextBody()
	b := &Body{
		ID: id, Name: "Bar", Visible: true,
		Mesh: mesh.Box(v3(1, 2, 3), v3(7, 4, 5), id),
	}
	b.FaceSeq = uint32(len(b.Mesh.Faces))
	doc.Bodies = append(doc.Bodies, b)

	var sel Selection
	sel.Set(BodyRef(id))
	pivot := geom.Vec3{X: 4, Y: 3, Z: 4}

	cmd := NewRotateVerts(sel.VertIndices(doc), pivot, geom.AxisY, 90, "body")
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("RotateVerts: %v", err)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("the result is invalid: %v", err)
	}
	if got := mesh.Volume(b.Mesh); math.Abs(got-6*2*2) > 1e-9 {
		t.Errorf("rotating changed the volume to %v", got)
	}
	// Exactness: every coordinate is still a whole number of subunits, and
	// bit-for-bit so — a float rotation by pi/2 would leave 6.000000000000001
	// scattered through the mesh.
	for i, p := range b.Mesh.Verts {
		for _, c := range []float64{p.X, p.Y, p.Z} {
			if c != math.Trunc(c*geom.Unit)/geom.Unit {
				t.Fatalf("vertex %d has an off-lattice coordinate %v", i, c)
			}
		}
	}
	// A quarter turn about Y sends X to -Z and Z to X, so the 6-long bar now
	// runs along Z.
	box := b.Mesh.AABB()
	if math.Abs(box.Size().Z-6) > 1e-9 || math.Abs(box.Size().X-2) > 1e-9 {
		t.Errorf("after a quarter turn the box is %v, want 6 long in Z", box.Size())
	}

	cmd.Undo(doc)
	if box := b.Mesh.AABB(); math.Abs(box.Size().X-6) > 1e-9 {
		t.Errorf("undo left the box %v, want 6 long in X again", box.Size())
	}
}

// TestFourQuarterTurnsReturnExactly: the strongest statement of exactness there
// is. Turn something all the way round and it must be where it started, to the
// bit, not to a tolerance.
func TestFourQuarterTurnsReturnExactly(t *testing.T) {
	doc := NewDocument()
	id := doc.Seq.NextBody()
	b := &Body{
		ID: id, Name: "Bar", Visible: true,
		Mesh: mesh.Box(v3(-3, -1, -2), v3(5, 1, 2), id),
	}
	doc.Bodies = append(doc.Bodies, b)
	want := append([]geom.Vec3(nil), b.Mesh.Verts...)

	var sel Selection
	sel.Set(BodyRef(id))
	for turn := 0; turn < 4; turn++ {
		cmd := NewRotateVerts(sel.VertIndices(doc), geom.Vec3{X: 1}, geom.AxisZ, 90, "body")
		if err := cmd.Do(doc); err != nil {
			t.Fatalf("turn %d: %v", turn, err)
		}
	}
	for i := range want {
		if b.Mesh.Verts[i] != want[i] {
			t.Fatalf("vertex %d came back to %v, want exactly %v",
				i, b.Mesh.Verts[i], want[i])
		}
	}
}

// TestFreeRotationIsAllowedToLeaveTheGrid: fifteen degrees is not a
// permutation, and SPEC-GEOMETRY §7.3 says the honest answer is off-grid
// coordinates rather than a rounding that quietly deforms the shape.
func TestFreeRotationIsAllowedToLeaveTheGrid(t *testing.T) {
	doc, b, _ := boxDoc(t)
	var sel Selection
	sel.Set(BodyRef(b.ID))

	cmd := NewRotateVerts(sel.VertIndices(doc),
		geom.Vec3{X: 2, Y: 2, Z: 2}, geom.AxisY, 15, "body")
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("RotateVerts: %v", err)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("the result is invalid: %v", err)
	}
	if got := mesh.Volume(b.Mesh); math.Abs(got-64) > 1e-9 {
		t.Errorf("a rigid rotation changed the volume to %v", got)
	}
	if !cmd.LeftTheGrid() {
		t.Error("a 15 degree turn did not report leaving the grid")
	}

	exact := NewRotateVerts(sel.VertIndices(doc), geom.Vec3{}, geom.AxisY, 90, "body")
	if err := exact.Do(doc); err != nil {
		t.Fatal(err)
	}
	if exact.LeftTheGrid() {
		t.Error("a quarter turn reported leaving the grid")
	}
}

// TestDuplicateOffsetsAndNames is SPEC-UX §12.4: Ctrl+D gives a copy one unit
// along X with a fresh name and its own identity.
func TestDuplicateOffsetsAndNames(t *testing.T) {
	doc, b, _ := boxDoc(t)

	cmd := &DuplicateBody{ID: b.ID}
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("DuplicateBody: %v", err)
	}
	if len(doc.Bodies) != 2 {
		t.Fatalf("%d bodies, want 2", len(doc.Bodies))
	}
	copy := cmd.Copy()
	if copy.ID == b.ID {
		t.Error("the copy shares the original's id")
	}
	if copy.Name == b.Name {
		t.Errorf("both bodies are called %q", copy.Name)
	}
	if math.Abs(copy.Mesh.AABB().Min.X-(b.Mesh.AABB().Min.X+1)) > 1e-9 {
		t.Error("the copy is not offset one unit along X")
	}
	// Editing the copy must not reach back into the original.
	copy.Mesh.Verts[0] = geom.Vec3{X: 99}
	if b.Mesh.Verts[0].X == 99 {
		t.Error("the copy shares the original's vertex data")
	}
	// Face identities belong to the new body, or paint and selection would
	// follow the copy back to its source.
	for i := range copy.Mesh.Faces {
		if copy.Mesh.Faces[i].ID.BodyID() != copy.ID {
			t.Fatalf("face %d of the copy still claims body %d",
				i, copy.Mesh.Faces[i].ID.BodyID())
		}
	}

	cmd.Undo(doc)
	if len(doc.Bodies) != 1 {
		t.Errorf("after undo there are %d bodies, want 1", len(doc.Bodies))
	}
}

// TestMoveRefusesWhatItCannotDo keeps the refusals honest.
func TestMoveRefusesWhatItCannotDo(t *testing.T) {
	doc, b, _ := boxDoc(t)
	before := mesh.Volume(b.Mesh)

	if err := NewMoveVerts(nil, geom.Vec3{X: 1}, "").Do(doc); err == nil {
		t.Error("moving nothing was accepted")
	}
	bad := map[uint32][]int{b.ID: {0, 999}}
	err := NewMoveVerts(bad, geom.Vec3{X: 1}, "").Do(doc)
	if err == nil {
		t.Fatal("moving a vertex that is not there was accepted")
	}
	if !strings.Contains(err.Error(), "vert") {
		t.Errorf("the refusal %q does not mention the vertex", err)
	}
	if got := mesh.Volume(b.Mesh); got != before {
		t.Errorf("a refused move changed the document: %v, was %v", got, before)
	}
}
