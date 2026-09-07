package model

import (
	"image"
	"modeler/internal/geom/mesh"
	"testing"
)

func TestChamferPreviewPaintAndUndo(t *testing.T) {
	doc, b, _ := boxDoc(t)
	before, seq := b.Mesh, b.FaceSeq
	paint := &mesh.FacePaint{Res: 4, Texel: .25, Frame: b.Mesh.FaceFrame(0), Img: image.NewRGBA(image.Rect(0, 0, 16, 16))}
	b.Mesh.Faces[0].Paint = paint
	cmd := &ChamferEdges{Edges: map[uint32][]int{b.ID: {0, 1}}, Distance: .5}
	if err := cmd.Prepare(doc); err != nil {
		t.Fatal(err)
	}
	preview := cmd.Preview(b.ID)
	if preview == nil || preview == before || b.Mesh != before || b.FaceSeq != seq {
		t.Fatal("preview changed document")
	}
	bus := NewBus(doc)
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	if b.Mesh != preview || bus.UndoDepth() != 1 {
		t.Fatal("apply diverged from preview")
	}
	found := false
	for _, f := range b.Mesh.Faces {
		if f.Paint == paint {
			found = true
		}
	}
	if !found {
		t.Fatal("surviving face lost paint")
	}
	bus.Undo()
	if b.Mesh != before || b.FaceSeq != seq {
		t.Fatal("undo did not restore snapshot")
	}
	bus.Redo()
	if b.Mesh != preview {
		t.Fatal("redo differs from original result")
	}
}

func TestChamferMultiBodyFailureIsAtomic(t *testing.T) {
	doc, b, _ := boxDoc(t)
	before := b.Mesh
	cmd := &ChamferEdges{Edges: map[uint32][]int{b.ID: {0}, 999: {0}}, Distance: .5}
	bus := NewBus(doc)
	if err := bus.Run(cmd); err == nil {
		t.Fatal("accepted missing second body")
	}
	if b.Mesh != before || bus.UndoDepth() != 0 || cmd.Preview(b.ID) != nil {
		t.Fatal("partial chamfer escaped failure")
	}
	cmd = &ChamferEdges{Edges: map[uint32][]int{b.ID: {0}}, Distance: .5}
	if err := cmd.Prepare(doc); err != nil {
		t.Fatal(err)
	}
	b.Mesh = before.Clone()
	if err := bus.Run(cmd); err == nil {
		t.Fatal("accepted stale preview")
	}
}
