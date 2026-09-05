package app

import (
	"image"
	"image/color"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/render"
	"testing"
)

func TestPasteEachCornerKeepsWholeImageOnFace(t *testing.T) {
	for rot := 0; rot < 4; rot++ {
		for corner := pasteBottomLeft; corner <= pasteTopRight; corner++ {
			a := bareApp()
			a.initPaint()
			add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
			if err := a.Bus.Run(add); err != nil {
				t.Fatal(err)
			}
			b := add.AddedBody()
			src := image.NewRGBA(image.Rect(0, 0, 3, 2))
			for i := 0; i < 6; i++ {
				src.SetRGBA(i%3, i/3, color.RGBA{R: uint8(i + 1), A: 255})
			}
			a.paint.pixels.clipboard = src
			a.paint.pixels.corner = corner
			a.setPixelPasteRotation(rot)
			a.togglePixelPasteMirror(false)
			oriented := a.orientedPastePixels()
			p, err := paint.Allocate(b.Mesh, 0, a.paint.res)
			if err != nil {
				t.Fatal(err)
			}
			face := paint.FaceRect(b.Mesh, 0, p)
			cursor, want := face.Min, face.Min
			if corner&1 != 0 {
				cursor.X = face.Max.X - 1
				want.X = face.Max.X - oriented.Bounds().Dx()
			}
			if corner&2 != 0 {
				cursor.Y = face.Max.Y - 1
				want.Y = face.Max.Y - oriented.Bounds().Dy()
			}
			if got := a.pixelPasteOrigin(cursor); got != want {
				t.Fatalf("rotation %d corner %d: origin %v want %v", rot, corner, got, want)
			}
			if !a.PastePixelsAt(b.ID, b.Mesh.Faces[0].ID, cursor) {
				t.Fatal("paste")
			}
			for y := 0; y < oriented.Bounds().Dy(); y++ {
				for x := 0; x < oriented.Bounds().Dx(); x++ {
					if paint.At(b.Mesh.Faces[0].Paint, want.Add(image.Pt(x, y))) != oriented.RGBAAt(x, y) {
						t.Fatal("corner paste clipped or shifted pixels")
					}
				}
			}
			a.Bus.Undo()
			if b.Mesh.Faces[0].Paint != nil {
				t.Fatal("undo")
			}
		}
	}
}

func TestPasteRightClickAndDragAreDistinct(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	a.paint.tool = paint.ToolPaste
	a.paint.pixels.clipboard = image.NewRGBA(image.Rect(0, 0, 3, 2))
	a.Camera = render.DefaultCamera()
	a.Scale = 1
	a.layout = ComputeLayout(1280, 720, 1, 240, false, 236)
	vp := a.layout.RenderViewport()
	down := InputFrame{MouseX: 600, MouseY: 380}
	down.Pressed[MouseRight] = true
	down.Down[MouseRight] = true
	before := a.Camera
	a.handleCameraInput(down, vp)
	if a.orbiting || a.pixelPasteMenuOpen() {
		t.Fatal("press should wait for click/drag")
	}
	up := InputFrame{MouseX: 600, MouseY: 380}
	up.Released[MouseRight] = true
	a.handleCameraInput(up, vp)
	if !a.pixelPasteMenuOpen() || a.Camera != before {
		t.Fatal("right click should open menu without moving camera")
	}
	a.setPixelPasteRotation(3)
	a.paint.pixels.corner = pasteTopRight
	depth := a.Bus.UndoDepth()
	a.choosePixelPasteMenu(8)
	if a.pixelPasteMenuOpen() || a.paint.pixels.rotation != 3 || a.paint.pixels.corner != pasteTopRight || a.Bus.UndoDepth() != depth {
		t.Fatal("Cancel changed paste or history")
	}
	a.handleCameraInput(down, vp)
	drag := InputFrame{MouseX: 630, MouseY: 390, MouseDX: 30, MouseDY: 10}
	drag.Down[MouseRight] = true
	a.handleCameraInput(drag, vp)
	if !a.orbiting || a.Camera == before {
		t.Fatal("right drag no longer orbits")
	}
	up.MouseX, up.MouseY = 630, 390
	a.handleCameraInput(up, vp)
	if a.pixelPasteMenuOpen() || a.orbiting {
		t.Fatal("drag release opened menu or stuck orbit")
	}
}

func TestPasteMenuChoicesAndWindowClipping(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	a.paint.tool = paint.ToolPaste
	a.paint.pixels.clipboard = image.NewRGBA(image.Rect(0, 0, 3, 2))
	a.Scale = 1
	a.layout = ComputeLayout(800, 600, 1, 240, false, 236)
	for i, want := range []pasteCorner{pasteTopLeft, pasteTopRight, pasteBottomLeft, pasteBottomRight} {
		a.paint.pixels.menu.open = true
		a.choosePixelPasteMenu(i)
		if a.paint.pixels.corner != want || a.pixelPasteMenuOpen() {
			t.Fatal("corner choice")
		}
	}
	for i := 0; i < 4; i++ {
		a.paint.pixels.menu.open = true
		a.choosePixelPasteMenu(4 + i)
		if int(a.paint.pixels.rotation) != i {
			t.Fatal("rotation choice")
		}
	}
	a.paint.pixels.menu = pasteMenuState{open: true, x: 799, y: 599}
	box := a.pixelPasteMenuBox()
	if box.X < 0 || box.Y < 0 || box.X+box.Width > 800 || box.Y+box.Height > 600 {
		t.Fatal("menu escaped window")
	}
	if !a.chromeOwnsPointer(InputFrame{MouseX: 300, MouseY: 200}) || a.cardOnlyOwnsPointer(InputFrame{MouseX: 300, MouseY: 200}) {
		t.Fatal("menu did not block background")
	}
}
