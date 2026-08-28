package app

import (
	"image"
	"image/color"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
)

// Switching the paint tool mid-stroke, tested without a GPU.
//
// The stroke machinery lives behind the update loop, but the wedge it guards
// against is pure state: a live stroke holds an open drag on the bus, and an
// open drag silently disables undo, redo and every command after it. The edge
// tool's update path never reaches the stroke code, so switching to it
// mid-drag used to leave that drag open until some later brush stroke happened
// to close it — with the bake button erroring in the meantime.

func TestSwitchingToolMidStrokeClosesTheDrag(t *testing.T) {
	a := bareApp()
	m := mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -2}, geom.Vec3{X: 4, Y: 1, Z: 2}, 1)
	add := &model.AddBody{Mesh: m}
	if err := a.Bus.Run(add); err != nil {
		t.Fatalf("AddBody: %v", err)
	}
	b := add.AddedBody()

	// A live stroke, the way updatePaint leaves one: drag open, flag up.
	a.paint.tool = paint.ToolPencil
	a.paint.res = paint.DefaultRes
	a.paint.color = color.RGBA{R: 255, A: 255}
	a.paint.strokeBody = b.ID
	a.paint.strokeFace = m.Faces[0].ID
	a.paint.stroking = true
	a.paint.points = []image.Point{{X: 1, Y: 1}}
	a.applyStroke()
	if !a.Bus.Dragging() {
		t.Fatal("the stroke did not open a drag")
	}

	a.setPaintTool(paint.ToolEdge)
	if a.Bus.Dragging() {
		t.Fatal("switching tools left the stroke's drag open — undo and every later command are dead until something closes it")
	}
	if a.paint.stroking {
		t.Error("the stroke flag survived the tool switch")
	}
	if !a.Bus.CanUndo() {
		t.Error("the committed stroke should be undoable after the switch")
	}
}

// Re-arming the same tool is not a switch and must not end a stroke.
func TestReArmingTheSameToolKeepsTheStroke(t *testing.T) {
	a := bareApp()
	a.paint.tool = paint.ToolPencil
	a.paint.stroking = true
	a.setPaintTool(paint.ToolPencil)
	if !a.paint.stroking {
		t.Error("re-picking the armed tool ended the stroke")
	}
}
