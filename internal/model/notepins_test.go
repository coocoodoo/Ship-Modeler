package model

import (
	"modeler/internal/geom"
	"testing"
)

func TestNotePinFollowsGeometryAndHistory(t *testing.T) {
	d, b, _ := boxDoc(t)
	tr := b.Mesh.FaceTris(0)[0]
	at := b.Mesh.Verts[tr.A].Add(b.Mesh.Verts[tr.B]).Add(b.Mesh.Verts[tr.C]).Mul(1.0 / 3)
	p, err := AnchorNotePin(b, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	p.Text = "Paint this panel copper"
	bus := NewBus(d)
	cmd := &SetNotePin{Pin: p}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	id := cmd.Pin.ID
	var sel Selection
	sel.Set(BodyRef(b.ID))
	delta := geom.Vec3{X: 4, Y: 2, Z: -3}
	if err := bus.Run(NewMoveVerts(sel.VertIndices(d), delta, "body")); err != nil {
		t.Fatal(err)
	}
	got, ok := d.NotePinByID(id).Position(d)
	if !ok || got.Sub(at.Add(delta)).Len() > 1e-8 {
		t.Fatalf("pin did not follow move: %v", got)
	}
	bus.Undo()
	got, ok = d.NotePinByID(id).Position(d)
	if !ok || got.Sub(at).Len() > 1e-8 {
		t.Fatal("pin did not follow undo")
	}
	edit := *d.NotePinByID(id)
	edit.Text = "Add two vents"
	edit.Done = true
	if err := bus.Run(&SetNotePin{Pin: edit}); err != nil {
		t.Fatal(err)
	}
	bus.Undo()
	if d.NotePinByID(id).Text != p.Text || d.NotePinByID(id).Done {
		t.Fatal("edit undo lost note")
	}
	bus.Redo()
	if !d.NotePinByID(id).Done {
		t.Fatal("redo lost completion")
	}
	if err := bus.Run(&DeleteNotePin{ID: id}); err != nil {
		t.Fatal(err)
	}
	bus.Undo()
	if d.NotePinByID(id) == nil {
		t.Fatal("delete undo lost pin")
	}
	b.Mesh.Faces[0].ID = b.NextFaceUID()
	if _, ok := d.NotePinByID(id).Position(d); ok {
		t.Fatal("stale pin silently attached to replacement face")
	}
}

func TestNotePinRejectsInvalidLocationAndText(t *testing.T) {
	_, b, _ := boxDoc(t)
	if _, err := AnchorNotePin(b, 0, geom.Vec3{X: 100, Y: 100, Z: 100}); err == nil {
		t.Fatal("accepted off-surface pin")
	}
	if _, err := CleanNoteText(" \n "); err == nil {
		t.Fatal("accepted empty note")
	}
}

func TestClearPinsRestartsNumberingThroughHistory(t *testing.T) {
	d, b, _ := boxDoc(t)
	tr := b.Mesh.FaceTris(0)[0]
	at := b.Mesh.Verts[tr.A].Add(b.Mesh.Verts[tr.B]).Add(b.Mesh.Verts[tr.C]).Mul(1.0 / 3)
	p, err := AnchorNotePin(b, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	p.Text = "A note"
	bus := NewBus(d)
	d.Seq.NotePin = 20
	old := &SetNotePin{Pin: p}
	if err = bus.Run(old); err != nil {
		t.Fatal(err)
	}
	if old.Pin.ID != 21 {
		t.Fatal("fixture counter")
	}
	if err = bus.Run(&ClearNotePins{}); err != nil {
		t.Fatal(err)
	}
	if d.Seq.NotePin != 0 || len(d.NotePins) != 0 {
		t.Fatal("counter not reset")
	}
	next := &SetNotePin{Pin: p}
	if err = bus.Run(next); err != nil {
		t.Fatal(err)
	}
	if next.Pin.ID != 1 || d.Seq.NotePin != 1 {
		t.Fatal("new sequence did not start at 1")
	}
	bus.Undo()
	bus.Undo()
	if d.Seq.NotePin != 21 || len(d.NotePins) != 1 || d.NotePins[0].ID != 21 {
		t.Fatal("undo did not restore old sequence")
	}
	bus.Redo()
	bus.Redo()
	if d.Seq.NotePin != 1 || len(d.NotePins) != 1 || d.NotePins[0].ID != 1 {
		t.Fatal("redo did not restore new sequence")
	}
	second := &SetNotePin{Pin: p}
	if err = bus.Run(second); err != nil {
		t.Fatal(err)
	}
	if second.Pin.ID != 2 {
		t.Fatal("numbering after redo")
	}
	// Empty projects cleared by older builds can reset their stale counter.
	d.NotePins = nil
	d.Seq.NotePin = 30
	c := &ClearNotePins{}
	if err = c.Do(d); err != nil {
		t.Fatal(err)
	}
	if d.Seq.NotePin != 0 {
		t.Fatal("empty reset")
	}
	c.Undo(d)
	if d.Seq.NotePin != 30 {
		t.Fatal("empty reset undo")
	}
}
