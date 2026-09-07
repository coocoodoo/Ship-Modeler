package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"testing"
)

func TestChamferModeCancelInvalidAndCommit(t *testing.T) {
	a := bareApp()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	a.Bus.Events.Listen(a.onDocumentEvent)
	a.Sel.Set(model.EdgeRef(b.ID, 0))
	before, depth := b.Mesh, a.Bus.UndoDepth()
	if !a.BeginEdgeChamfer() || a.chamfer.command == nil {
		t.Fatal("preview not prepared", a.chamfer.err)
	}
	if b.Mesh != before || a.Bus.UndoDepth() != depth {
		t.Fatal("preview is destructive")
	}
	a.CancelEdgeChamfer()
	if a.Mode != ModeIdle || b.Mesh != before || a.Bus.UndoDepth() != depth {
		t.Fatal("cancel changed document")
	}
	a.BeginEdgeChamfer()
	a.setChamferDistance(99)
	if a.CommitEdgeChamfer() || b.Mesh != before || !a.InChamfer() || a.chamfer.err == "" {
		t.Fatal("invalid chamfer was applied")
	}
	a.setChamferDistance(.5)
	if !a.CommitEdgeChamfer() || a.InChamfer() || a.Bus.UndoDepth() != depth+1 || len(b.Mesh.Faces) != 7 {
		t.Fatal("apply failed")
	}
	a.Undo()
	if b.Mesh != before {
		t.Fatal("undo lost source mesh")
	}
}
