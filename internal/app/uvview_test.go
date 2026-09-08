package app

import (
	"image"
	"image/color"
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
)

func uvTestApp(t *testing.T) (*App, *model.Body) {
	t.Helper()
	a := bareApp()
	a.Scale = 1
	a.initPaint()
	a.paint.res = 4
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	a.Bus.Events.Listen(a.onUVEvent)
	a.toggleUVView()
	a.paint.bar = 1
	a.layout = a.Layout(1280, 720)
	a.prepareUVView()
	if len(a.uv.islands) != 6 {
		t.Fatal("missing UV faces")
	}
	a.paint.size = 1
	a.paint.color = color.RGBA{R: 231, G: 40, B: 30, A: 255}
	return a, b
}

func uvTestPoint(a *App, face int, x, y float64) InputFrame {
	p := a.uvScreen(a.uv.islands[face].sheetUV(geom.Vec2{X: x + .5, Y: y + .5}))
	return InputFrame{MouseX: p.X, MouseY: p.Y}
}

func uvTestDrag(a *App, face int, from, to image.Point) {
	in := uvTestPoint(a, face, float64(from.X), float64(from.Y))
	in.Pressed[0], in.Down[0] = true, true
	a.updateUVView(in, a.layout.RenderViewport())
	a.prepareUVView()
	in = uvTestPoint(a, face, float64(to.X), float64(to.Y))
	in.Down[0] = true
	a.updateUVView(in, a.layout.RenderViewport())
	a.prepareUVView()
	in.Down[0], in.Released[0] = false, true
	a.updateUVView(in, a.layout.RenderViewport())
	a.prepareUVView()
}

func TestUVPaintToolsShareTexturesAndUndo(t *testing.T) {
	for _, tool := range []paint.Tool{paint.ToolPencil, paint.ToolLine, paint.ToolRect, paint.ToolCircle, paint.ToolFill, paint.ToolGradient, paint.ToolBrush} {
		t.Run(string(tool), func(t *testing.T) {
			a, b := uvTestApp(t)
			a.setPaintTool(tool)
			depth, zoom, pan := a.Bus.UndoDepth(), a.uv.zoom, a.uv.pan
			uvTestDrag(a, 0, image.Pt(3, 3), image.Pt(10, 10))
			p := b.Mesh.Faces[0].Paint
			if p == nil {
				t.Fatal("UV stroke did not paint model face")
			}
			n := 0
			for i := 3; i < len(p.Img.Pix); i += 4 {
				if p.Img.Pix[i] != 0 {
					n++
				}
			}
			if n == 0 {
				t.Fatal("empty stroke")
			}
			if a.uv.zoom != zoom || a.uv.pan != pan {
				t.Fatal("first dab moved the UV layout")
			}
			if a.uv.captured || a.Bus.UndoDepth() != depth+1 {
				t.Fatal("stroke must finish in one undo step")
			}
			a.Bus.Undo()
			a.prepareUVView()
			if b.Mesh.Faces[0].Paint != nil {
				t.Fatal("undo did not restore bare face")
			}
			a.Bus.Redo()
			a.prepareUVView()
			if b.Mesh.Faces[0].Paint == nil {
				t.Fatal("redo did not restore UV paint")
			}
		})
	}
}

func TestUVZoomPanAndCapture(t *testing.T) {
	a, b := uvTestApp(t)
	for _, f := range b.Mesh.Faces {
		if f.Paint != nil {
			t.Fatal("opening viewer allocated document paint")
		}
	}
	in := uvTestPoint(a, 0, 4, 5)
	before := a.uvSheet(in.MouseX, in.MouseY)
	camera := a.Camera
	in.Wheel = 3
	a.updateUVView(in, a.layout.RenderViewport())
	after := a.uvSheet(in.MouseX, in.MouseY)
	if math.Hypot(before.X-after.X, before.Y-after.Y) > 1e-8 || a.Camera != camera {
		t.Fatal("zoom moved cursor target or camera")
	}
	in.Wheel = 0
	in.Pressed[2], in.Down[2] = true, true
	in.MouseDX, in.MouseDY = 20, -11
	pan := a.uv.pan
	a.updateUVView(in, a.layout.RenderViewport())
	if a.uv.pan != pan.Add(geom.Vec2{X: 20, Y: -11}) || a.paint.stroking {
		t.Fatal("pan painted or failed")
	}
	a.updateUVView(InputFrame{}, a.layout.RenderViewport())
	a.fitUVView(false)
	in = uvTestPoint(a, 0, 4, 5)
	in.Pressed[0], in.Down[0] = true, true
	a.updateUVView(in, a.layout.RenderViewport())
	a.prepareUVView()
	a.updateUVView(InputFrame{MouseX: 1, MouseY: 1, Released: [3]bool{true}}, a.layout.RenderViewport())
	if a.paint.stroking || a.uv.captured {
		t.Fatal("release outside viewer left stroke captured")
	}
}

func TestUVClipboardPickAndErase(t *testing.T) {
	a, b := uvTestApp(t)
	uvTestDrag(a, 0, image.Pt(3, 3), image.Pt(5, 3))
	a.setPaintTool(paint.ToolSelect)
	uvTestDrag(a, 0, image.Pt(3, 3), image.Pt(5, 4))
	if !a.CopySelectedPixels() || !a.BeginPixelPaste() {
		t.Fatal("UV copy failed")
	}
	in := uvTestPoint(a, 1, 6, 6)
	in.Pressed[0] = true
	a.updateUVView(in, a.layout.RenderViewport())
	a.prepareUVView()
	p := b.Mesh.Faces[1].Paint
	if p == nil || paint.At(p, image.Pt(6, 6)) != a.paint.color {
		t.Fatal("UV paste differs from copied pixels")
	}
	a.setPaintTool(paint.ToolPick)
	a.paint.color = color.RGBA{}
	a.updateUVView(in, a.layout.RenderViewport())
	if a.paint.color.R != 231 {
		t.Fatal("UV eyedropper missed paint")
	}
	a.setPaintTool(paint.ToolEraser)
	uvTestDrag(a, 1, image.Pt(6, 6), image.Pt(6, 6))
	if paint.At(b.Mesh.Faces[1].Paint, image.Pt(6, 6)).A != 0 {
		t.Fatal("UV eraser failed")
	}
}

func TestUVHolesAndImageClipping(t *testing.T) {
	is := uvIsland{loops: [][]geom.Vec2{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}, {{X: 3, Y: 3}, {X: 7, Y: 3}, {X: 7, Y: 7}, {X: 3, Y: 7}}}}
	if !uvInside(&is, geom.Vec2{X: 1, Y: 1}) || uvInside(&is, geom.Vec2{X: 5, Y: 5}) {
		t.Fatal("face hole is paintable")
	}
	poly := uvClip([]geom.Vec2{{X: -4, Y: 0}, {X: 12, Y: 0}, {X: 4, Y: 14}}, image.Rect(0, 0, 8, 8))
	if len(poly) < 3 {
		t.Fatal("lost clipped triangle")
	}
	for _, p := range poly {
		if p.X < 0 || p.X > 8 || p.Y < 0 || p.Y > 8 {
			t.Fatal("texture extends past image")
		}
	}
}

func TestUVTileWandEdgeAndPasteRotation(t *testing.T) {
	a, b := uvTestApp(t)
	tile := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			tile.SetRGBA(x, y, a.paint.color)
		}
	}
	a.paint.tiles.set = &paint.Tileset{Img: tile, TileW: 2, TileH: 2}
	a.setPaintTool(paint.ToolTile)
	uvTestDrag(a, 0, image.Pt(2, 2), image.Pt(6, 2))
	if p := b.Mesh.Faces[0].Paint; p == nil || paint.At(p, image.Pt(6, 2)).R != 231 {
		t.Fatal("UV tile trail failed")
	}
	a.setPaintTool(paint.ToolWand)
	in := uvTestPoint(a, 0, 6, 2)
	in.Pressed[0] = true
	a.updateUVView(in, a.layout.RenderViewport())
	if a.paint.wandMask == nil || !a.paint.wandMask.Contains(image.Pt(6, 2)) || a.paint.wandMask.Contains(image.Pt(9, 9)) {
		t.Fatal("UV wand selected wrong region")
	}
	a.ClearWandSelection()
	a.setPaintTool(paint.ToolEdge)
	is := &a.uv.islands[0]
	p := is.loops[0][0].Add(is.loops[0][1]).Mul(.5)
	screen := a.uvScreen(is.sheetUV(p))
	a.updateUVView(InputFrame{MouseX: screen.X, MouseY: screen.Y, Pressed: [3]bool{true}}, a.layout.RenderViewport())
	if len(a.paint.edges) == 0 {
		t.Fatal("UV edge cannot be selected")
	}
	a.paint.pixels.clipboard = tile
	a.BeginPixelPaste()
	zoom := a.uv.zoom
	in = uvTestPoint(a, 0, 5, 5)
	in.Ctrl, in.Wheel = true, 1
	for n := 1; n <= 8; n++ {
		a.updateUVView(in, a.layout.RenderViewport())
		if int(a.paint.pixels.rotation) != n%4 || a.uv.zoom != zoom {
			t.Fatal("Ctrl wheel lost a quarter turn or zoomed")
		}
	}
	in.Ctrl, in.Wheel = false, 0
	in.Down[MouseRight], in.Pressed[MouseRight] = true, true
	a.updateUVView(in, a.layout.RenderViewport())
	in.Down[MouseRight], in.Pressed[MouseRight], in.Released[MouseRight] = false, false, true
	a.updateUVView(in, a.layout.RenderViewport())
	if !a.pixelPasteMenuOpen() {
		t.Fatal("UV right click did not open placement menu")
	}
}

func TestUVBodySwitchResolutionAndDragHover(t *testing.T) {
	a, b := uvTestApp(t)
	in := uvTestPoint(a, 0, 2, 2)
	in.Down[0], in.Pressed[0] = true, true
	a.updateUVView(in, a.layout.RenderViewport())
	a.prepareUVView()
	in = uvTestPoint(a, 0, 7, 8)
	in.Down[0] = true
	a.updateUVView(in, a.layout.RenderViewport())
	if a.paint.hover.texel != image.Pt(7, 8) {
		t.Fatal("UV brush cursor stayed at stroke origin")
	}
	a.updateUVView(InputFrame{}, a.layout.RenderViewport())
	a.closeUVView()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 2)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	second := add.AddedBody()
	a.Sel.Set(model.BodyRef(second.ID))
	a.toggleUVView()
	a.prepareUVView()
	if a.uv.body != second.ID || a.uv.body == b.ID {
		t.Fatal("opening UV ignored selected body")
	}
	a.SetPaintRes(8)
	a.prepareUVView()
	if a.uv.res != 8 {
		t.Fatal("UV mapping missed density change")
	}
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		a.Scale = scale
		a.layout = a.Layout(1280, 720)
		panel, canvas := a.uvRects()
		if canvas.Width <= 0 || canvas.Height <= 0 || canvas.Y+canvas.Height > panel.Y+panel.Height {
			t.Fatal("UV canvas escapes at UI scale", scale)
		}
	}
}
