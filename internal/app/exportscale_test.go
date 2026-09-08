package app

import (
	"modeler/internal/geom"
	"modeler/internal/io"
	"path/filepath"
	"testing"
)

func TestExportUsesShipScaleWithoutEditingProject(t *testing.T) {
	a, b := uvTestApp(t)
	depth, dirty, bounds := a.Bus.UndoDepth(), a.Doc().DirtySinceSave, b.Mesh.AABB()
	if a.modelExportScale() != 1 {
		t.Fatal("default scale must be 1")
	}
	if err := a.SetModelExportScale(.5); err != nil {
		t.Fatal(err)
	}
	if err := a.SetModelExportScale(0); err == nil || a.modelExportScale() != .5 {
		t.Fatal("invalid scale changed the chosen value")
	}
	path := filepath.Join(t.TempDir(), "ship.stl")
	if err := a.ExportTo(path, 4, false); err != nil {
		t.Fatal(err)
	}
	tris, err := io.ReadMeshFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := geom.Empty()
	for _, tri := range tris {
		got = got.AddPoint(tri.A).AddPoint(tri.B).AddPoint(tri.C)
	}
	if got.Size() != bounds.Size().Mul(.5) {
		t.Fatal("export ignored ship scale or used PNG scale instead")
	}
	if a.Bus.UndoDepth() != depth || a.Doc().DirtySinceSave != dirty || b.Mesh.AABB() != bounds {
		t.Fatal("export edited the project")
	}
}
