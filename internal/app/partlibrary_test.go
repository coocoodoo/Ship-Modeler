package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/model"
	"testing"
)

func TestLibraryAcrossNewSessionsAndInsertUndo(t *testing.T) {
	a := bareApp()
	a.library.dir = t.TempDir()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	depth := a.Bus.UndoDepth()
	a.beginSaveToLibrary([]uint32{add.AddedBody().ID})
	a.library.name = "Engine"
	a.library.category = "Propulsion"
	if !a.saveLibraryPart() {
		t.Fatal(a.library.err)
	}
	if len(a.Doc().Bodies) != 1 || a.Bus.UndoDepth() != depth {
		t.Fatal("saving changed source project")
	}
	b := bareApp()
	b.library.dir = a.library.dir
	b.refreshPartLibrary()
	if len(b.library.parts) != 1 || b.library.parts[0].Category != "Propulsion" || len(b.Doc().Bodies) != 0 {
		t.Fatal("library did not survive session")
	}
	if !b.insertLibraryPart(b.library.parts[0].ID) {
		t.Fatal(b.library.err)
	}
	if b.Sel.Len() != 1 || len(b.Doc().Bodies) != 1 {
		t.Fatal("insert did not select copy")
	}
	b.Undo()
	if len(b.Doc().Bodies) != 0 || len(b.library.parts) != 1 {
		t.Fatal("undo affected library")
	}
	b.Redo()
	if len(b.Doc().Bodies) != 1 {
		t.Fatal("redo failed")
	}
}

func TestLibraryEditUpdatesOneEntryAndCanBeRepeated(t *testing.T) {
	a := bareApp()
	a.library.dir = t.TempDir()
	body := &model.Body{ID: 1, Name: "Wing", Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1), Visible: true}
	part, err := io.SaveLibraryPart(a.library.dir, "Wing", "Parts", []*model.Body{body})
	if err != nil {
		t.Fatal(err)
	}
	a.refreshPartLibrary()
	if !a.editLibraryPart(part.ID) || !a.libraryEditAvailable() {
		t.Fatal("edit did not open a working copy")
	}
	if !a.editLibraryPart(part.ID) || len(a.Doc().Bodies) != 1 {
		t.Fatal("reopening edit created another working copy")
	}
	for _, width := range []float64{3, 4} {
		b := a.Doc().Bodies[0]
		b.Mesh = mesh.Box(geom.Vec3{}, geom.Vec3{X: width, Y: 2, Z: 2}, b.ID)
		a.beginUpdateLibraryPart(a.library.editPart, a.library.editBodies)
		a.library.name = "Updated wing"
		if !a.saveLibraryPart() {
			t.Fatal(a.library.err)
		}
		if len(a.library.parts) != 1 || a.library.parts[0].ID != part.ID {
			t.Fatal("update duplicated entry")
		}
		fresh, err := io.LoadLibraryPart(a.library.dir, a.library.parts[0])
		if err != nil || geom.AABBOf(fresh[0].Mesh.Verts).Max.X != width {
			t.Fatal("edited geometry was not saved", err)
		}
	}
	a.beginSaveToLibrary(a.library.editBodies)
	a.library.name, a.library.category = "Updated wing", "Parts"
	if a.saveLibraryPart() || len(a.library.parts) != 1 {
		t.Fatal("same-name duplicate was created")
	}
	a.Bus = model.NewBus(model.NewDocument())
	if a.libraryEditAvailable() {
		t.Fatal("edit target survived document replacement")
	}
}
