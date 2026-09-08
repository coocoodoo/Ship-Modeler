package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"testing"
)

func TestNotePinAIStateAndModalOwnership(t *testing.T) {
	a := bareApp()
	a.Scale = 1
	b := &model.Body{ID: 1, Name: "Hull", Visible: true, Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	a.Doc().Bodies = append(a.Doc().Bodies, b)
	tr := b.Mesh.FaceTris(0)[0]
	at := b.Mesh.Verts[tr.A].Add(b.Mesh.Verts[tr.B]).Add(b.Mesh.Verts[tr.C]).Mul(1.0 / 3)
	p, err := model.AnchorNotePin(b, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	p.Text = "Add a copper vent"
	if err := a.Bus.Run(&model.SetNotePin{Pin: p}); err != nil {
		t.Fatal(err)
	}
	notes := a.aiState()["notePins"].([]any)
	n := notes[0].(map[string]any)
	if n["text"] != p.Text || n["bodyName"] != "Hull" || n["attached"] != true || n["done"] != false {
		t.Fatalf("AI cannot resolve note: %v", n)
	}
	a.editNotePin(a.Doc().NotePins[0])
	if !a.chromeOwnsPointer(InputFrame{}) || !a.uvBlocked() {
		t.Fatal("note editor allows painting through dialog")
	}
	a.notePins.armed = true
	a.escape()
	if a.notePins.armed {
		t.Fatal("Escape left placement armed")
	}
}
