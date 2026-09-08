package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/ui"
	"reflect"
	"testing"
)

func TestMoveTexturePreviewCancelApply(t *testing.T) {
	a := bareApp()
	b := &model.Body{ID: 1, Visible: true, Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	a.Doc().Bodies = append(a.Doc().Bodies, b)
	p, e := paint.Allocate(b.Mesh, 0, 32)
	if e != nil {
		t.Fatal(e)
	}
	b.Mesh.Faces[0].Paint = p
	b.Mesh.Faces[1].Paint = p
	uid := b.Mesh.Faces[0].ID
	a.beginMoveTexture(1, uid)
	a.nudgeMoveTexture(2, -3)
	if !a.moveTexture.open || a.moveTexture.preview == nil || b.Mesh.Faces[0].Paint != p {
		t.Fatal("preview edited document")
	}
	a.finishMoveTexture(false)
	if b.Mesh.Faces[0].Paint != p {
		t.Fatal("cancel changed paint")
	}
	a.beginMoveTexture(1, uid)
	a.nudgeMoveTexture(2, -3)
	a.finishMoveTexture(true)
	if b.Mesh.Faces[0].Paint == p || b.Mesh.Faces[1].Paint != p {
		t.Fatal("apply isolation")
	}
	a.Undo()
	if b.Mesh.Faces[0].Paint != p {
		t.Fatal("one undo did not restore entire move")
	}
}
func TestClearPinsConfirmationAndUndo(t *testing.T) {
	a := bareApp()
	pins := []model.NotePin{{ID: 3, Text: "Keep this"}, {ID: 7, Text: "Done", Done: true}}
	a.Doc().NotePins = append([]model.NotePin(nil), pins...)
	a.Doc().Seq.NotePin = 7
	for _, answer := range []ui.ModalResult{{Cancelled: true}, {Dismissed: true}} {
		a.notePins.clearing = true
		a.routeModalAnswer(answer)
		if len(a.Doc().NotePins) != 2 || a.Doc().Seq.NotePin != 7 {
			t.Fatal("cancel cleared pins")
		}
	}
	a.notePins.clearing = true
	a.routeModalAnswer(ui.ModalResult{Confirmed: true})
	if len(a.Doc().NotePins) != 0 || a.Doc().Seq.NotePin != 0 {
		t.Fatal("yes did not clear")
	}
	a.Undo()
	if len(a.Doc().NotePins) != 2 || !reflect.DeepEqual(a.Doc().NotePins[1], pins[1]) || a.Doc().Seq.NotePin != 7 {
		t.Fatal("undo lost pins or IDs")
	}
	a.Redo()
	if len(a.Doc().NotePins) != 0 || a.Doc().Seq.NotePin != 0 {
		t.Fatal("redo did not clear")
	}
}
