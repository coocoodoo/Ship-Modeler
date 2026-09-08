package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	modelio "modeler/internal/io"
	"modeler/internal/model"
	"path/filepath"
	"testing"
)

func TestExportUnpaintedMaterialDoesNotEditModel(t *testing.T) {
	a := bareApp()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	a.Doc().DirtySinceSave = false
	features := len(a.Doc().Features)
	path := filepath.Join(t.TempDir(), "base_color.png")
	if err := a.ExportMaterialImages(b.ID, b.Mesh.Faces[0].ID, "base_color", path); err != nil {
		t.Fatal(err)
	}
	img, err := modelio.ReadMaterialImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if img.RGBAAt(0, 0) != b.Color {
		t.Fatal("unpainted export lost body color")
	}
	if b.Mesh.Faces[0].Paint != nil || a.Doc().DirtySinceSave || len(a.Doc().Features) != features {
		t.Fatal("export edited the model or history")
	}
}
