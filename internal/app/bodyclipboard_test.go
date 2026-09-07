package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"testing"
)

func TestBodyClipboardKeysMultiSelection(t *testing.T) {
	a := bareApp()
	for i := 0; i < 2; i++ {
		cmd := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, uint32(i+1))}
		if err := a.Bus.Run(cmd); err != nil {
			t.Fatal(err)
		}
		a.Sel.Add(model.BodyRef(cmd.AddedBody().ID))
	}
	a.handleTransformKeys(InputFrame{Ctrl: true, KeysPressed: []int32{rl.KeyC}})
	a.Sel.Clear()
	before := a.Bus.UndoDepth()
	a.handleTransformKeys(InputFrame{Ctrl: true, KeysPressed: []int32{rl.KeyV}})
	if len(a.Doc().Bodies) != 4 || a.Sel.Len() != 2 || a.Bus.UndoDepth() != before+1 {
		t.Fatal("group paste failed")
	}
	a.Undo()
	if len(a.Doc().Bodies) != 2 {
		t.Fatal("undo did not remove whole group")
	}
	a.Redo()
	if len(a.Doc().Bodies) != 4 {
		t.Fatal("redo failed")
	}
}
