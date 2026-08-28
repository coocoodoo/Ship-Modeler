package model

import (
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The command half of folding (the user's request, 2026-08-28): a move that
// bends faces splits them into flat pieces when asked, as part of the same
// undo step, and one Ctrl+Z restores the document exactly — positions, faces,
// identities and the per-body face counter.

// foldBus is a document holding one unit box, with the body returned for
// convenience.
func foldBus(t *testing.T) (*Bus, *Body) {
	t.Helper()
	bus := NewBus(NewDocument())
	cmd := &AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1), Label: "Crate"}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	b := cmd.AddedBody()
	// The primitive minted six face ids; the body's counter must be past them
	// before anything else allocates.
	b.FaceSeq = uint32(len(b.Mesh.Faces))
	return bus, b
}

func TestMoveWithFoldSplitsTheBentFaces(t *testing.T) {
	bus, b := foldBus(t)
	cmd := NewMoveVerts(map[uint32][]int{b.ID: {7}}, geom.Vec3{X: 0.5, Y: 0.5, Z: 0.5}, "vertex")
	cmd.FoldBent = true
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Folded() != 3 {
		t.Errorf("folded %d faces, want the corner's 3", cmd.Folded())
	}
	if cmd.Bent() != 0 {
		t.Errorf("%d faces still count as bent after folding", cmd.Bent())
	}
	if len(b.Mesh.Faces) != 9 {
		t.Errorf("body has %d faces, want 9", len(b.Mesh.Faces))
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("folded body is not a valid solid: %v", err)
	}
	// Every face id is unique and the counter is past all of them, so the next
	// allocation anywhere cannot collide with a fold piece.
	seen := map[mesh.FaceUID]bool{}
	for i := range b.Mesh.Faces {
		id := b.Mesh.Faces[i].ID
		if seen[id] {
			t.Errorf("face id %v minted twice", id)
		}
		seen[id] = true
		if id.Seq() > b.FaceSeq {
			t.Errorf("face seq %d is past the body counter %d", id.Seq(), b.FaceSeq)
		}
	}
}

func TestUndoOfAFoldingMoveRestoresEverything(t *testing.T) {
	bus, b := foldBus(t)
	wantVert := b.Mesh.Verts[7]
	wantSeq := b.FaceSeq
	wantIDs := make([]mesh.FaceUID, len(b.Mesh.Faces))
	for i := range b.Mesh.Faces {
		wantIDs[i] = b.Mesh.Faces[i].ID
	}

	cmd := NewMoveVerts(map[uint32][]int{b.ID: {7}}, geom.Vec3{X: 0.5, Y: 0.5, Z: 0.5}, "vertex")
	cmd.FoldBent = true
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if b.Mesh.Verts[7] != wantVert {
		t.Errorf("vert 7 is at %v after undo, want %v", b.Mesh.Verts[7], wantVert)
	}
	if len(b.Mesh.Faces) != len(wantIDs) {
		t.Fatalf("body has %d faces after undo, want %d", len(b.Mesh.Faces), len(wantIDs))
	}
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].ID != wantIDs[i] {
			t.Errorf("face %d is %v after undo, want %v", i, b.Mesh.Faces[i].ID, wantIDs[i])
		}
		if b.Mesh.Faces[i].NonPlanar {
			t.Errorf("face %d is still flagged bent after undo", i)
		}
	}
	if b.FaceSeq != wantSeq {
		t.Errorf("face counter is %d after undo, want %d — redo would mint colliding ids", b.FaceSeq, wantSeq)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("undone body invalid: %v", err)
	}

	// And redo folds again, identically.
	if _, ok := bus.Redo(); !ok {
		t.Fatal("nothing to redo")
	}
	if len(b.Mesh.Faces) != 9 {
		t.Errorf("redo gave %d faces, want 9", len(b.Mesh.Faces))
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("redone body invalid: %v", err)
	}
}

func TestAFoldingDragIsStillOneUndoStep(t *testing.T) {
	bus, b := foldBus(t)
	depth := bus.UndoDepth()
	verts := map[uint32][]int{b.ID: {7}}

	// The drag frames run without folding, exactly as the gizmo does; the
	// commit swaps in the folding version of the final state.
	if err := bus.BeginDrag(NewMoveVerts(verts, geom.Vec3{X: 0.2}, "vertex")); err != nil {
		t.Fatal(err)
	}
	for _, x := range []float64{0.3, 0.4, 0.5} {
		if err := bus.UpdateDrag(NewMoveVerts(verts, geom.Vec3{X: x, Y: x, Z: x}, "vertex")); err != nil {
			t.Fatal(err)
		}
	}
	final := NewMoveVerts(verts, geom.Vec3{X: 0.5, Y: 0.5, Z: 0.5}, "vertex")
	final.FoldBent = true
	if err := bus.UpdateDrag(final); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.CommitDrag(); !ok {
		t.Fatal("commit failed")
	}
	if len(b.Mesh.Faces) != 9 {
		t.Errorf("the committed drag left %d faces, want 9", len(b.Mesh.Faces))
	}
	if got := bus.UndoDepth(); got != depth+1 {
		t.Errorf("the drag cost %d undo entries, want 1", got-depth)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if len(b.Mesh.Faces) != 6 || b.Mesh.Verts[7] != (geom.Vec3{X: 1, Y: 1, Z: 1}) {
		t.Errorf("one undo did not restore the box: %d faces, vert %v",
			len(b.Mesh.Faces), b.Mesh.Verts[7])
	}
}

func TestMoveWithoutFoldStillJustBends(t *testing.T) {
	bus, b := foldBus(t)
	cmd := NewMoveVerts(map[uint32][]int{b.ID: {7}}, geom.Vec3{X: 0.5, Y: 0.5, Z: 0.5}, "vertex")
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Folded() != 0 {
		t.Errorf("an unflagged move folded %d faces", cmd.Folded())
	}
	if cmd.Bent() != 3 {
		t.Errorf("bent = %d, want 3 — the flag must not change the default", cmd.Bent())
	}
	if len(b.Mesh.Faces) != 6 {
		t.Errorf("face count changed to %d without folding", len(b.Mesh.Faces))
	}
}
