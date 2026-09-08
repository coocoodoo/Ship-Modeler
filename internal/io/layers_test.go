package io

import (
	"bytes"
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"testing"
)

func TestPXMLayersMasksAndPinReviewRoundTrip(t *testing.T) {
	d := shipDoc(t)
	p := d.Bodies[0].Mesh.Faces[0].Paint
	mask := image.NewRGBA(p.Img.Bounds())
	mask.SetRGBA(1, 1, color.RGBA{255, 255, 255, 255})
	p.Layers = []mesh.PaintLayer{{Name: "Armor", Visible: true, Opacity: .75, Pixels: mesh.CopyImage(p.Img), Mask: mask}}
	p.ActiveLayer = 0
	p.PBRStale = true
	d.NotePins = []model.NotePin{{ID: 1, Body: d.Bodies[0].ID, Face: d.Bodies[0].Mesh.Faces[0].ID, Text: "Add armor", Status: "Needs review", Changes: "Painted an armor layer", BeforeImage: []byte{1, 2, 3}, AfterImage: []byte{4, 5, 6}}}
	path := saveTo(t, d)
	r, e := LoadShip(path)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Doc.NotePins) != 1 || r.Doc.NotePins[0].Status != "Needs review" || r.Doc.NotePins[0].Changes != "Painted an armor layer" || !bytes.Equal(r.Doc.NotePins[0].AfterImage, []byte{4, 5, 6}) {
		t.Fatal("PXM lost pin review history")
	}
	q := r.Doc.Bodies[0].Mesh.Faces[0].Paint
	if len(q.Layers) != 1 || q.Layers[0].Name != "Armor" || q.Layers[0].Opacity != .75 || !q.PBRStale || !bytes.Equal(q.Layers[0].Mask.Pix, mask.Pix) || !bytes.Equal(q.Layers[0].Pixels.Pix, p.Img.Pix) {
		t.Fatal("PXM lost editable layer data")
	}
}
