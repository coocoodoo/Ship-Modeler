package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"image"
	"math"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// Face paint's U/V axes grow right/up, so its minimum texel is bottom-left.
type pasteCorner uint8

const (
	pasteBottomLeft pasteCorner = iota
	pasteBottomRight
	pasteTopLeft
	pasteTopRight
)

type pasteMenuState struct {
	open, pending bool
	x, y          float64
	hover         paintHover
}

func pasteOrigin(at image.Point, size image.Point, corner pasteCorner) image.Point {
	if corner&1 != 0 {
		at.X -= max(0, size.X-1)
	}
	if corner&2 != 0 {
		at.Y -= max(0, size.Y-1)
	}
	return at
}

func (a *App) pixelPasteOrigin(at image.Point) image.Point {
	p := a.orientedPastePixels()
	if p == nil {
		return at
	}
	return pasteOrigin(at, p.Bounds().Size(), a.paint.pixels.corner)
}

func (a *App) pixelPasteMenuOpen() bool {
	return a.InPaint() && a.paint.tool == paint.ToolPaste && a.paint.pixels.menu.open
}

// A short right-click opens the menu. Crossing the drag threshold hands the
// same press back to camera navigation, preserving right-drag orbit and pan.
func (a *App) handlePixelPasteRightClick(in *InputFrame, vp render.Viewport) bool {
	if a.material.open {
		return false
	}
	st := &a.paint.pixels.menu
	if !a.InPaint() || a.paint.tool != paint.ToolPaste || a.paint.awaitingLock || a.paint.pixels.clipboard == nil {
		*st = pasteMenuState{}
		return false
	}
	if st.open {
		return true
	}
	if !st.pending && in.Pressed[MouseRight] && vp.Contains(int(in.MouseX), int(in.MouseY)) && !a.Cube.Contains(in.MouseX, in.MouseY) {
		st.pending = true
		st.x, st.y = in.MouseX, in.MouseY
		a.orbiting, a.panning = false, false
	}
	if !st.pending {
		return false
	}
	threshold := 4 * math.Max(1, a.Scale)
	dx, dy := in.MouseX-st.x, in.MouseY-st.y
	if dx*dx+dy*dy > threshold*threshold {
		st.pending = false
		if in.Down[MouseRight] {
			in.Pressed[MouseRight] = true
		}
		return false
	}
	if !in.Down[MouseRight] {
		st.pending = false
		if in.Released[MouseRight] {
			st.open = true
			st.hover = a.paint.hover
		}
	}
	return true
}

func (a *App) pixelPasteMenuItems() []ui.MenuItem {
	c, r := a.paint.pixels.corner, a.paint.pixels.rotation
	return []ui.MenuItem{
		{Label: "Anchor: top left", Selected: c == pasteTopLeft},
		{Label: "Anchor: top right", Selected: c == pasteTopRight},
		{Label: "Anchor: bottom left", Selected: c == pasteBottomLeft},
		{Label: "Anchor: bottom right", Selected: c == pasteBottomRight},
		{Label: "Rotate 0°", Selected: r == 0},
		{Label: "Rotate 90°", Selected: r == 1},
		{Label: "Rotate 180°", Selected: r == 2},
		{Label: "Rotate 270°", Selected: r == 3},
		{Label: "Cancel", Icon: ui.DrawCrossIcon},
	}
}

func (a *App) pixelPasteMenuBox() rl.Rectangle {
	st := a.paint.pixels.menu
	w, h := a.px(244), a.px(8+26*9)
	screen := a.layout.Screen
	w, h = min(w, screen.Width), min(h, screen.Height)
	x := max(screen.X, min(float32(st.x), screen.X+screen.Width-w))
	y := max(screen.Y, min(float32(st.y), screen.Y+screen.Height-h))
	return ui.Rect(x, y, w, h)
}

func (a *App) choosePixelPasteMenu(chosen int) {
	switch chosen {
	case 0, 1, 2, 3:
		a.paint.pixels.corner = []pasteCorner{pasteTopLeft, pasteTopRight, pasteBottomLeft, pasteBottomRight}[chosen]
	case 4, 5, 6, 7:
		a.setPixelPasteRotation(chosen - 4)
	}
	// Cancel, outside-click and Escape only dismiss; none puts paint down.
	a.paint.pixels.menu = pasteMenuState{}
}

func (a *App) buildPixelPasteMenu() {
	if !a.pixelPasteMenuOpen() {
		return
	}
	result := a.UI.Menu(ui.MakeID("paint.paste.menu"), a.pixelPasteMenuBox(), a.pixelPasteMenuItems())
	// Hit-test the menu first and then block the entire background for this
	// frame. Even the press that dismisses it cannot activate a control below.
	a.UI.ClaimPointer(a.layout.Screen)
	if result.Dismissed || a.UI.In.Pressed[MouseRight] {
		a.choosePixelPasteMenu(-1)
	} else if result.Chosen >= 0 {
		a.choosePixelPasteMenu(result.Chosen)
	}
}
