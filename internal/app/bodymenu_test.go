package app

import (
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
