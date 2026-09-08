package app

import (
	"archive/zip"
	modelio "modeler/internal/io"
	"os"
	"path/filepath"
	"testing"
)

func TestPXMExportUsesProjectArchiveAndCreatesNoCompanions(t *testing.T) {
	a, b := uvTestApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "model.pxm")
	depth, dirty, bounds := a.Bus.UndoDepth(), a.Doc().DirtySinceSave, b.Mesh.AABB()
	if err := a.SetModelExportScale(.5); err != nil {
		t.Fatal(err)
	}
	if err := a.ExportTo(path, 1, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "model.pxm" {
		t.Fatal("PXM export created a companion folder")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Method != zip.Deflate {
			t.Fatal("PXM export is not compressed")
		}
	}
	loaded, err := modelio.LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Doc.Bodies[0].Mesh.AABB() != bounds {
		t.Fatal("project export applied 3D geometry scale")
	}
	if a.Bus.UndoDepth() != depth || a.Doc().DirtySinceSave != dirty || a.files.path != "" {
		t.Fatal("export changed current project state")
	}
}
