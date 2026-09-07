package app

import (
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/scene"
	"testing"
)

func TestChamferGizmoQuarterStepsAndReleaseOutside(t *testing.T) {
	a := bareApp()
	a.Scale, a.Camera = 1, render.DefaultCamera()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	a.Sel.Set(model.EdgeRef(b.ID, 0))
	a.BeginEdgeChamfer()
	vp := a.Viewport(1280, 720)
	v, ok := a.chamferArrow(vp)
	if !ok || a.buildChamferGizmo(vp) == nil {
		t.Fatal("no arrow")
	}
	e := b.Mesh.Topo().Edges[0]
	outward := b.Mesh.FaceNormal(e.Uses[0].Face).Add(b.Mesh.FaceNormal(e.Uses[1].Face))
	if v.Dir.Dot(outward) >= 0 {
		t.Fatal("outside-edge gizmo points away from the cut")
	}
	ax, ay, bx, by, _ := scene.ArrowScreenEnds(a.Camera, vp, v.Origin, v.Dir, v.LengthWorld)
	l := math.Hypot(bx-ax, by-ay)
	dx, dy := (bx-ax)/l, (by-ay)/l
	x, y := ax+dx*l*.65, ay+dy*l*.65
	in := InputFrame{MouseX: x, MouseY: y, WindowW: 1280, WindowH: 720, DeltaMillis: 16}
	in.Pressed[MouseLeft], in.Down[MouseLeft] = true, true
	before, depth := b.Mesh, a.Bus.UndoDepth()
	if !a.updateChamferGizmo(in, vp) || !a.chamfer.gizmo.dragging {
		t.Fatal("arrow not grabbed")
	}
	in.Pressed[MouseLeft] = false
	for _, c := range []struct{ pixels, want float64 }{{12, .5}, {24, .75}, {36, 1}, {12, .5}, {0, .25}, {-24, .25}, {24, .75}} {
		in.MouseX, in.MouseY = x+dx*c.pixels, y+dy*c.pixels
		in.Ctrl, in.Alt = true, true
		a.updateChamferGizmo(in, vp)
		if a.chamfer.distance != c.want {
			t.Fatalf("%g pixels: %g want %g", c.pixels, a.chamfer.distance, c.want)
		}
	}
	// Move perpendicular to the drag axis, outside the window. The same
	// release still ends the drag and validates the latest snapped preview.
	in.MouseX, in.MouseY = x+dx*24-dy*3000, y+dy*24+dx*3000
	in.Down[MouseLeft], in.Released[MouseLeft] = false, true
	a.update(in)
	if a.chamfer.gizmo.dragging || !a.chamfer.gizmo.captured || a.chamfer.dirty || a.chamfer.command == nil {
		t.Fatal("release did not finish preview")
	}
	if a.chamfer.distance != .75 || b.Mesh != before || a.Bus.UndoDepth() != depth || a.Sel.Len() != 1 {
		t.Fatal("drag changed geometry/selection/history")
	}
	if !a.CommitEdgeChamfer() {
		t.Fatal("commit failed")
	}
	if a.Bus.UndoDepth() != depth+1 {
		t.Fatal("drag created extra undo steps")
	}
}

func TestChamferGizmoSnapAtDifferentScales(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		if got := chamferDragDistance(.25, 24*scale, scale); got != .75 {
			t.Fatal(scale, got)
		}
		if got := chamferDragDistance(.31, 12*scale, scale); got != .5 {
			t.Fatal("typed value did not join quarter grid", got)
		}
	}
}
