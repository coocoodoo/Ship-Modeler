package ui

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Icons are drawn as vector strokes rather than shipped as art, so they stay
// crisp at every DPI scale and there is no binary asset to regenerate (D-11).
// Each takes a centre, a nominal size in device pixels, and a colour.

// strokeWidth scales the 1.5 px nominal stroke to the current size.
func strokeWidth(size float64) float32 {
	w := size / IconSize * IconStroke
	if w < 1 {
		w = 1
	}
	return float32(w)
}

func v2(x, y float64) rl.Vector2 { return rl.Vector2{X: float32(x), Y: float32(y)} }

// DrawHomeIcon draws the house glyph used by the view cube's home button.
func DrawHomeIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// Roof: two strokes meeting at the apex.
	apex := v2(cx, cy-h*0.85)
	left := v2(cx-h*0.9, cy-h*0.05)
	right := v2(cx+h*0.9, cy-h*0.05)
	rl.DrawLineEx(left, apex, w, col)
	rl.DrawLineEx(apex, right, w, col)
	// Walls.
	wl := v2(cx-h*0.62, cy-h*0.05)
	wr := v2(cx+h*0.62, cy-h*0.05)
	bl := v2(cx-h*0.62, cy+h*0.8)
	br := v2(cx+h*0.62, cy+h*0.8)
	rl.DrawLineEx(wl, bl, w, col)
	rl.DrawLineEx(wr, br, w, col)
	rl.DrawLineEx(bl, br, w, col)
}

// DrawChevron draws a chevron pointing in a cardinal direction: 0 right,
// 1 down, 2 left, 3 up. Used by collapsible sections and panel handles.
func DrawChevron(cx, cy, size float64, dir int, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	var a, b, c rl.Vector2
	switch dir {
	case 1: // down
		a, b, c = v2(cx-h*0.6, cy-h*0.3), v2(cx, cy+h*0.35), v2(cx+h*0.6, cy-h*0.3)
	case 2: // left
		a, b, c = v2(cx+h*0.3, cy-h*0.6), v2(cx-h*0.35, cy), v2(cx+h*0.3, cy+h*0.6)
	case 3: // up
		a, b, c = v2(cx-h*0.6, cy+h*0.3), v2(cx, cy-h*0.35), v2(cx+h*0.6, cy+h*0.3)
	default: // right
		a, b, c = v2(cx-h*0.3, cy-h*0.6), v2(cx+h*0.35, cy), v2(cx-h*0.3, cy+h*0.6)
	}
	rl.DrawLineEx(a, b, w, col)
	rl.DrawLineEx(b, c, w, col)
}

// DrawCheckIcon draws the confirm tick.
func DrawCheckIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	rl.DrawLineEx(v2(cx-h*0.7, cy), v2(cx-h*0.15, cy+h*0.55), w, col)
	rl.DrawLineEx(v2(cx-h*0.15, cy+h*0.55), v2(cx+h*0.7, cy-h*0.55), w, col)
}

// DrawCrossIcon draws the cancel cross.
func DrawCrossIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	rl.DrawLineEx(v2(cx-h*0.6, cy-h*0.6), v2(cx+h*0.6, cy+h*0.6), w, col)
	rl.DrawLineEx(v2(cx+h*0.6, cy-h*0.6), v2(cx-h*0.6, cy+h*0.6), w, col)
}
