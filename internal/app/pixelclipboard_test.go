package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"image"
	"image/color"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/render"
	"testing"
)

func TestPixelSelectionDragDirectionsAndSquare(t *testing.T) {
	b := image.Rect(0, 0, 16, 12)
	for _, tc := range []struct {
		a, z   image.Point
		square bool
		want   image.Rectangle
	}{
		{image.Pt(2, 3), image.Pt(6, 5), false, image.Rect(2, 3, 7, 6)},
		{image.Pt(6, 5), image.Pt(2, 3), false, image.Rect(2, 3, 7, 6)},
		{image.Pt(2, 3), image.Pt(6, 5), true, image.Rect(2, 3, 7, 8)},
		{image.Pt(6, 5), image.Pt(2, 3), true, image.Rect(2, 1, 7, 6)},
		{image.Pt(12, 9), image.Pt(100, 100), true, image.Rect(12, 9, 15, 12)},
		{image.Pt(2, 3), image.Pt(-100, -100), false, image.Rect(0, 0, 3, 4)},
		{image.Pt(2, 3), image.Pt(2, 3), true, image.Rect(2, 3, 3, 4)},
	} {
		if got := pixelSelectionRect(tc.a, tc.z, b, tc.square); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

func TestPixelClipboardTransfersFacesAndSurvivesUndo(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	source, dest := b.Mesh.Faces[0].ID, b.Mesh.Faces[2].ID
	p, err := paint.Allocate(b.Mesh, 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	b.Mesh.Faces[0].Paint = p
	red := color.RGBA{R: 240, G: 12, A: 255}
	soft := color.RGBA{R: 20, B: 44, A: 70}
	paint.Set(p, image.Pt(2, 3), red)
	paint.Set(p, image.Pt(3, 3), soft)
	if !a.selectPixelRect(b.ID, source, image.Rect(2, 3, 5, 5)) {
		t.Fatal("selection")
	}
	key := func(k int32, ctrl bool) {
		in := NewInputFrame()
		in.Ctrl = ctrl
		in.KeysPressed = append(in.KeysPressed, k)
		a.handlePaintKeys(in)
	}
	a.paint.pixels.flipH, a.paint.pixels.flipV = true, true
	key(rl.KeyC, true)
	if a.paint.pixels.flipH || a.paint.pixels.flipV {
		t.Fatal("new copy must reset mirrors")
	}
	if a.paint.tool == paint.ToolCircle || a.paint.pixels.clipboard == nil {
		t.Fatal("Ctrl+C did not copy")
	}
	paint.Set(p, image.Pt(2, 3), color.RGBA{G: 255, A: 255})
	a.paint.locked = true
	key(rl.KeyV, true)
	if a.paint.tool != paint.ToolPaste || a.paint.locked {
		t.Fatal("paste must allow choosing another face")
	}
	depth := a.Bus.UndoDepth()
	if !a.PastePixelsAt(b.ID, dest, image.Pt(8, 9)) {
		t.Fatal("paste")
	}
	out := b.Mesh.Faces[2].Paint
	if paint.At(out, image.Pt(8, 9)) != red || paint.At(out, image.Pt(9, 9)) != soft {
		t.Fatal("copied snapshot was stretched, altered or aliased")
	}
	if a.Bus.UndoDepth() != depth+1 {
		t.Fatal("paste must be one undo step")
	}
	a.Bus.Undo()
	if b.Mesh.Faces[2].Paint != nil {
		t.Fatal("undo did not restore bare target")
	}
	a.paint.pixels.clipboard.SetRGBA(0, 0, color.RGBA{})
	a.Bus.Redo()
	if paint.At(b.Mesh.Faces[2].Paint, image.Pt(8, 9)) != red {
		t.Fatal("redo consulted changed clipboard")
	}
	key(rl.KeyEscape, false)
	if !a.InPaint() || a.paint.tool != paint.ToolSelect {
		t.Fatal("Esc should cancel placement only")
	}
	a.ExitPaint()
	if a.paint.pixels.clipboard == nil || a.paint.pixels.selection != nil {
		t.Fatal("mode exit should preserve clipboard, clear selection")
	}
}

func TestPixelSelectionInvalidatedByResample(t *testing.T) {
	a := bareApp()
	a.initPaint()
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	uid := b.Mesh.Faces[0].ID
	if !a.selectPixelRect(b.ID, uid, image.Rect(0, 0, 4, 4)) {
		t.Fatal("select")
	}
	p, err := paint.Allocate(b.Mesh, 0, 16)
	if err != nil {
		t.Fatal(err)
	}
	b.Mesh.Faces[0].Paint = p
	if s, _ := a.currentPixelSelection(); s != nil {
		t.Fatal("stale selection survived density change")
	}
}

func TestPasteWheelRotationAndCameraRouting(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	a.paint.tool = paint.ToolPaste
	a.layout = ComputeLayout(1280, 720, 1, 240, false, 236)
	view := a.layout.RenderViewport()
	a.Camera = render.DefaultCamera()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		src.SetRGBA(i%3, i/3, color.RGBA{R: uint8(i + 1), A: uint8(30 + i)})
	}
	a.paint.pixels.clipboard = src
	wheel := func(n float64, ctrl bool) {
		in := NewInputFrame()
		in.MouseX, in.MouseY = 600, 380
		in.Wheel = n
		in.Ctrl = ctrl
		a.handleCameraInput(in, view)
	}
	before := a.Camera
	wheel(1, true)
	rot := a.orientedPastePixels()
	if a.Camera != before {
		t.Fatal("Ctrl+wheel zoomed during paste")
	}
	if rot.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatal("90 degrees must swap rectangular dimensions")
	}
	for i, want := range []uint8{4, 1, 5, 2, 6, 3} {
		got := rot.RGBAAt(i%2, i/2)
		if got.R != want || got.A != 29+want {
			t.Fatalf("rotated pixel %d: %v", i, got)
		}
	}
	if a.orientedPastePixels() != rot {
		t.Fatal("preview unnecessarily rebuilt")
	}
	wheel(-1, true)
	if a.orientedPastePixels() != src {
		t.Fatal("reverse wheel did not restore original")
	}
	wheel(4, true)
	if a.paint.pixels.rotation != 1 {
		t.Fatal("coalesced wheel movement must advance one visible step")
	}
	wheel(-2, true)
	if a.paint.pixels.rotation != 0 {
		t.Fatal("reverse wheel movement must go back one step")
	}
	wheel(1, false)
	if a.Camera == before || a.paint.pixels.rotation != 0 {
		t.Fatal("plain wheel should zoom without rotating")
	}
	a.paint.tool = paint.ToolSelect
	before = a.Camera
	wheel(1, true)
	if a.Camera == before {
		t.Fatal("Ctrl+wheel should zoom outside paste")
	}
}

func TestPasteWheelRepeatsFullTurnsWhileCtrlHeld(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	a.paint.tool = paint.ToolPaste
	a.layout = ComputeLayout(1280, 720, 1, 240, false, 236)
	a.Camera = render.DefaultCamera()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		src.SetRGBA(i%3, i/3, color.RGBA{R: uint8(i + 1), A: 255})
	}
	a.paint.pixels.clipboard = src
	// Keep Ctrl and the cursor held steady through repeated events and idle
	// frames. Asymmetric pixels prove each cached preview really changes.
	wants := [][]uint8{{1, 2, 3, 4, 5, 6}, {4, 1, 5, 2, 6, 3}, {6, 5, 4, 3, 2, 1}, {3, 6, 2, 5, 1, 4}}
	for _, delta := range []float64{1, 4, 120, 0.25, -1, -4, -120, -0.25} {
		a.paint.pixels.rotation = 0
		a.paint.pixels.oriented = nil
		for n := 1; n <= 12; n++ {
			in := InputFrame{MouseX: 600, MouseY: 380, Ctrl: true, Wheel: delta}
			before := a.Camera
			a.handleCameraInput(in, a.layout.RenderViewport())
			want := n % 4
			if delta < 0 {
				want = (4 - want) % 4
			}
			if int(a.paint.pixels.rotation) != want || a.Camera != before {
				t.Fatalf("delta %g event %d: angle=%d", delta, n, a.paint.pixels.rotation)
			}
			p := a.orientedPastePixels()
			for i, red := range wants[want] {
				if p.RGBAAt(i%p.Bounds().Dx(), i/p.Bounds().Dx()).R != red {
					t.Fatalf("delta %g event %d: stale/wrong preview", delta, n)
				}
			}
			in.Wheel = 0
			for idle := 0; idle < 3; idle++ {
				a.handleCameraInput(in, a.layout.RenderViewport())
			}
			if int(a.paint.pixels.rotation) != want {
				t.Fatal("idle frame reset orientation")
			}
		}
	}
}

func TestPasteUsesVerticalWheelDespiteOppositeTilt(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	a.paint.tool = paint.ToolPaste
	a.layout = ComputeLayout(1280, 720, 1, 240, false, 236)
	a.Camera = render.DefaultCamera()
	a.paint.pixels.clipboard = image.NewRGBA(image.Rect(0, 0, 3, 2))
	// Upward scrolling stays positive even when a device alternately reports
	// a larger leftward tilt. The old dominant-axis helper gives +1,-2,+1,-2
	// here, bouncing the angle between 0 and 90 instead of completing a turn.
	for n, move := range []rl.Vector2{{Y: 1}, {X: -2, Y: 1}, {Y: 1}, {X: -2, Y: 1}, {X: -5}} {
		in := InputFrame{Ctrl: true, Wheel: verticalWheelNotches(move), MouseX: 600, MouseY: 380}
		before := a.Camera
		a.handleCameraInput(in, a.layout.RenderViewport())
		want := (n + 1) % 4
		if n == 4 {
			want = 0
		}
		if int(a.paint.pixels.rotation) != want || a.Camera != before {
			t.Fatalf("event %d: rotation=%d want=%d", n, a.paint.pixels.rotation, want)
		}
	}
}

func TestRotatedPasteUndoKeepsPlacedOrientation(t *testing.T) {
	a := bareApp()
	a.initPaint()
	a.Mode = ModePaint
	a.paint.tool = paint.ToolPaste
	add := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)}
	if err := a.Bus.Run(add); err != nil {
		t.Fatal(err)
	}
	b := add.AddedBody()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	red := color.RGBA{R: 220, A: 255}
	soft := color.RGBA{B: 24, A: 50}
	src.SetRGBA(0, 0, red)
	src.SetRGBA(2, 1, soft)
	a.paint.pixels.clipboard = src
	a.rotatePixelPasteWheel(InputFrame{Ctrl: true, Wheel: 1})
	a.togglePixelPasteMirror(false)
	if !a.PastePixelsAt(b.ID, b.Mesh.Faces[0].ID, image.Pt(2, 3)) {
		t.Fatal("paste")
	}
	check := func() {
		p := b.Mesh.Faces[0].Paint
		if paint.At(p, image.Pt(2, 3)) != red || paint.At(p, image.Pt(3, 5)) != soft {
			t.Fatal("placement did not match rotated pixels")
		}
	}
	check()
	a.Bus.Undo()
	a.togglePixelPasteMirror(true)
	a.rotatePixelPasteWheel(InputFrame{Ctrl: true, Wheel: 1})
	a.Bus.Redo()
	check()
	if src.RGBAAt(0, 0) != red || src.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatal("rotation mutated copied source")
	}
}

func TestPasteMirrorsAtEveryAngle(t *testing.T) {
	a := bareApp()
	a.initPaint()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		src.SetRGBA(i%3, i/3, color.RGBA{R: uint8(i + 1), G: uint8(i * 4), A: uint8(30 + i)})
	}
	a.paint.pixels.clipboard = src
	for rotation := 0; rotation < 4; rotation++ {
		base := paint.OrientPixels(src, paint.Orientation{Rot: uint8(rotation)})
		for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			a.paint.pixels.flipH, a.paint.pixels.flipV = false, false
			a.setPixelPasteRotation(rotation)
			a.orientedPastePixels() // materialize the cache before changing flags
			if flags[0] {
				a.togglePixelPasteMirror(false)
			}
			if flags[1] {
				a.togglePixelPasteMirror(true)
			}
			got := a.orientedPastePixels()
			if got.Bounds() != base.Bounds() {
				t.Fatal("mirror changed dimensions")
			}
			for y := 0; y < base.Bounds().Dy(); y++ {
				for x := 0; x < base.Bounds().Dx(); x++ {
					sx, sy := x, y
					if flags[0] {
						sx = base.Bounds().Dx() - 1 - x
					}
					if flags[1] {
						sy = base.Bounds().Dy() - 1 - y
					}
					if got.RGBAAt(x, y) != base.RGBAAt(sx, sy) {
						t.Fatalf("rotation %d flips %v pixel %d,%d", rotation, flags, x, y)
					}
				}
			}
			if flags[0] {
				a.togglePixelPasteMirror(false)
			}
			if flags[1] {
				a.togglePixelPasteMirror(true)
			}
			restored := a.orientedPastePixels()
			for y := 0; y < base.Bounds().Dy(); y++ {
				for x := 0; x < base.Bounds().Dx(); x++ {
					if restored.RGBAAt(x, y) != base.RGBAAt(x, y) {
						t.Fatal("second flip did not restore pixels")
					}
				}
			}
		}
	}
	for i := 0; i < 6; i++ {
		if src.RGBAAt(i%3, i/3).R != uint8(i+1) {
			t.Fatal("mirror mutated copied pixels")
		}
	}
}
