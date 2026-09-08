package app

import (
	"image"
	"modeler/internal/paint"
	"testing"
)

func TestMaterialInspectionSelectsWithoutPainting(t *testing.T) {
	for _, tool := range []paint.Tool{paint.ToolPencil, paint.ToolFill, paint.ToolBrush, paint.ToolTile, paint.ToolEdge, paint.ToolSelect, paint.ToolPaste} {
		t.Run(string(tool), func(t *testing.T) {
			a, b := uvTestApp(t)
			a.setPaintTool(tool)
			a.paint.locked, a.paint.lockBody, a.paint.lockFace = true, b.ID, b.Mesh.Faces[0].ID
			a.openMaterialPanel()
			depth := a.Bus.UndoDepth()
			dirty := a.Doc().DirtySinceSave
			uvTestDrag(a, 1, image.Pt(3, 3), image.Pt(8, 8))
			if a.material.body != b.ID || a.material.face != b.Mesh.Faces[1].ID {
				t.Fatal("inspection did not select the clicked face through the paint lock")
			}
			if a.Bus.UndoDepth() != depth || a.Doc().DirtySinceSave != dirty || a.Bus.Dragging() {
				t.Fatal("inspection changed document history")
			}
			for _, f := range b.Mesh.Faces {
				if f.Paint != nil {
					t.Fatal("inspection painted the model")
				}
			}
			if a.paintCursorOverlay() != nil {
				t.Fatal("inspection still shows a brush preview")
			}
		})
	}
}

func TestLeavingMaterialInspectionRestoresPainting(t *testing.T) {
	a, b := uvTestApp(t)
	a.openMaterialPanel()
	a.closeMaterialPanel()
	uvTestDrag(a, 0, image.Pt(3, 3), image.Pt(4, 4))
	if b.Mesh.Faces[0].Paint == nil {
		t.Fatal("Back to Paint did not restore painting")
	}
}
