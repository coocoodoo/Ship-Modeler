package io

import (
	"archive/zip"
	"encoding/json"
	"io"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotePinProjectRoundTripAndExportIsolation(t *testing.T) {
	d := model.NewDocument()
	b := &model.Body{ID: 1, Name: "Hull", Visible: true, Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	d.Bodies = append(d.Bodies, b)
	tr := b.Mesh.FaceTris(0)[0]
	at := b.Mesh.Verts[tr.A].Add(b.Mesh.Verts[tr.B]).Add(b.Mesh.Verts[tr.C]).Mul(1.0 / 3)
	p, err := model.AnchorNotePin(b, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	p.Text = "Pin-only request: add copper vents"
	if err := model.NewBus(d).Run(&model.SetNotePin{Pin: p}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pins.pxm")
	if err := SaveShip(path, d, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Doc.NotePins) != 1 || loaded.Doc.NotePins[0].Text != p.Text || loaded.Doc.Seq.NotePin != 1 {
		t.Fatal("project lost pin metadata")
	}
	if _, ok := loaded.Doc.NotePins[0].Position(loaded.Doc); !ok {
		t.Fatal("loaded pin lost anchor")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "game/") {
			r, _ := f.Open()
			data, _ := io.ReadAll(r)
			r.Close()
			if strings.Contains(string(data), p.Text) {
				t.Fatal("authoring note leaked into game payload")
			}
		}
	}
	data, _ := json.Marshal(BuildGameMarkers(d))
	if strings.Contains(string(data), "copper vents") {
		t.Fatal("pin exported as game marker")
	}
}
