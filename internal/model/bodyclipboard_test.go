package model

import (
	"image"
	"image/color"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"testing"
)

func TestBodyClipboardOwnsGeometryPaintAndUndo(t *testing.T) {
	doc, b, _ := boxDoc(t)
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	red := color.RGBA{R: 255, A: 255}
	img.SetRGBA(0, 0, red)
	b.Mesh.Faces[0].Paint = &mesh.FacePaint{Img: img, Texel: 1, Frame: b.Mesh.FaceFrame(0)}
	b.Mesh.Faces[0].KeepEdges = true
	snapshot := SnapshotBody(b)
	img.SetRGBA(0, 0, color.RGBA{})
	b.Mesh.Verts[0].X = 99
	// The clipboard survives deleting its source, and works repeatedly.
	doc.Bodies = nil
	cmd := &PasteBodies{Sources: []*Body{snapshot}, Offset: geom.Vec3{X: 1}}
	bus := NewBus(doc)
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	copy := cmd.Copies()[0]
	face := copy.Mesh.Faces[0]
	if face.Paint.Img.RGBAAt(0, 0) != red || !face.KeepEdges || copy.Mesh.Verts[0].X != snapshot.Mesh.Verts[0].X+1 {
		t.Fatal("paste lost snapshot data")
	}
	for _, f := range copy.Mesh.Faces {
		if f.ID.BodyID() != copy.ID {
			t.Fatal("face identity was shared")
		}
	}
	face.Paint.Img.SetRGBA(0, 0, color.RGBA{})
	if snapshot.Mesh.Faces[0].Paint.Img.RGBAAt(0, 0) != red {
		t.Fatal("paste shares clipboard pixels")
	}
	id := copy.ID
	bus.Undo()
	if len(doc.Bodies) != 0 {
		t.Fatal("undo left pasted body")
	}
	bus.Redo()
	if len(doc.Bodies) != 1 || doc.Bodies[0].ID != id {
		t.Fatal("redo changed identities")
	}
	other := &PasteBodies{Sources: []*Body{snapshot}}
	if err := bus.Run(other); err != nil {
		t.Fatal(err)
	}
	if other.Copies()[0].ID == id || other.Copies()[0].Mesh.Faces[0].Paint.Img.RGBAAt(0, 0) != red {
		t.Fatal("repeated paste lost independent snapshot")
	}
}
