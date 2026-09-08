package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/render"
	"testing"
)

func TestBodyRightClickPreservesOrbitAndClickCancel(t *testing.T) {
	a := bareApp()
	a.Scale = 1
	vp := render.Viewport{W: 1280, H: 720}
	a.library.menu = bodyMenuState{pending: true, x: 500, y: 300, body: 1}
	in := InputFrame{MouseX: 520, MouseY: 310}
	in.Down[MouseRight] = true
	if a.handleBodyRightClick(&in, vp) || !in.Pressed[MouseRight] || in.MouseDX != 20 || in.MouseDY != 10 || a.library.menu.open {
		t.Fatal("right drag did not hand off to orbit")
	}
	a.library.menu = bodyMenuState{pending: true, x: 500, y: 300, body: 1}
	in = InputFrame{MouseX: 501, MouseY: 300}
	in.Released[MouseRight] = true
	if !a.handleBodyRightClick(&in, vp) || !a.library.menu.open {
		t.Fatal("short right click did not open menu")
	}
}

func TestBodyMenuPinUsesOriginalSurfaceHit(t *testing.T) {
	a := bareApp()
	b := &model.Body{ID: 1, Visible: true, Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	a.Doc().Bodies = append(a.Doc().Bodies, b)
	tri := b.Mesh.FaceTris(0)[0]
	at := b.Mesh.Verts[tri.A].Add(b.Mesh.Verts[tri.B]).Add(b.Mesh.Verts[tri.C]).Mul(1.0 / 3)
	p, err := model.AnchorNotePin(b, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	a.library.menu = bodyMenuState{open: true, body: 1, pin: &p, x: 1275, y: 715}
	a.dropBodyMenuPin()
	if a.library.menu.open || !a.notePins.open || !a.notePins.editing || a.notePins.armed || a.notePins.draft.At != at {
		t.Fatal("menu pin did not retain its surface hit")
	}
	if len(a.Doc().NotePins) != 0 {
		t.Fatal("opening the editor saved an unfinished pin")
	}
	a.library.menu = bodyMenuState{open: true, body: 1}
	a.dropBodyMenuPin()
	if !a.notePins.armed || a.notePins.open {
		t.Fatal("tree menu did not arm surface placement")
	}
}
