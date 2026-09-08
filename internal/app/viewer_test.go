package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/render"
	"testing"
)

func TestViewerBlocksEditsAndRetainsDocumentOnUnlock(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Scale = 1
	a.Camera = render.DefaultCamera()
	a.files.path = "my ship.pxm"
	a.files.readOnly = true // Forward-version protection is independent of viewer mode.
	doc := a.Doc()
	a.enterViewer()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1)}
	if a.Run(add) || a.Save() || a.SaveAs() {
		t.Fatal("viewer allowed a write")
	}
	r := &ScriptRunner{App: a, Size: ShotSize{W: 1200, H: 800}}
	for _, name := range []string{"delete", "paint.fill", "file.save", "body.add", "undo", "view.uv", "pin.add"} {
		if err := r.runOp(io.Op{Op: name}); err == nil {
			t.Fatalf("viewer allowed %s", name)
		}
	}
	a.editViewer()
	if a.Doc() != doc || a.files.path != "my ship.pxm" || !a.files.readOnly {
		t.Fatal("unlock changed the loaded project or format protection")
	}
	if a.Viewer || !a.Run(add) || len(a.Doc().Bodies) != 1 {
		t.Fatal("Edit did not unlock modeling")
	}
}

func TestViewerInputOnlyNavigates(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Scale = 1
	a.Camera = render.DefaultCamera()
	a.enterViewer()
	a.layout = a.Layout(1200, 800)
	if a.layout.Viewport.X != 0 || a.layout.Viewport.Width != 1200 || a.layout.Tree.Width != 0 || a.layout.PaintBar.Width != 0 {
		t.Fatal("viewer is not full width")
	}
	in := NewInputFrame()
	in.WindowW, in.WindowH = 1200, 800
	in.MouseX, in.MouseY = 600, 400
	in.Pressed[MouseLeft] = true
	in.KeysPressed = []int32{rl.KeyDelete, rl.KeyP, rl.KeyE, rl.KeyS}
	a.update(in)
	if a.Mode != ModeIdle || a.Doc().DirtySinceSave || a.Sel.Len() != 0 {
		t.Fatal("viewer input edited the model")
	}
	before := a.Camera
	in.Pressed[MouseLeft] = false
	in.Pressed[MouseRight], in.Down[MouseRight] = true, true
	in.MouseDX = 30
	a.update(in)
	if a.Camera == before {
		t.Fatal("viewer cannot orbit")
	}
	if a.library.menu.open {
		t.Fatal("orbit opened a body edit menu")
	}
}
