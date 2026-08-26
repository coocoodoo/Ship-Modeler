package ui

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Icons are drawn as vector strokes rather than shipped as art, so they stay
// crisp at every DPI scale and there is no binary asset to regenerate (D-11).
// Each takes a centre, a nominal size in device pixels, and a colour.

// IconFunc is the shape every icon shares, so widgets can take one as a value.
type IconFunc func(cx, cy, size float64, col color.RGBA)

// strokeWidth scales the 1.5 px nominal stroke to the current size.
func strokeWidth(size float64) float32 {
	w := size / IconSize * IconStroke
	if w < 1 {
		w = 1
	}
	return float32(w)
}

func v2(x, y float64) rl.Vector2 { return rl.Vector2{X: float32(x), Y: float32(y)} }

// line is the shared stroke primitive.
func line(a, b rl.Vector2, w float32, col color.RGBA) { rl.DrawLineEx(a, b, w, col) }

// poly strokes an open path.
func poly(w float32, col color.RGBA, pts ...rl.Vector2) {
	for i := 0; i+1 < len(pts); i++ {
		line(pts[i], pts[i+1], w, col)
	}
}

// closedPoly strokes a closed path.
func closedPoly(w float32, col color.RGBA, pts ...rl.Vector2) {
	poly(w, col, pts...)
	if len(pts) > 1 {
		line(pts[len(pts)-1], pts[0], w, col)
	}
}

// DrawHomeIcon draws the house glyph used by the view cube's home button.
func DrawHomeIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	poly(w, col,
		v2(cx-h*0.9, cy-h*0.05), v2(cx, cy-h*0.85), v2(cx+h*0.9, cy-h*0.05))
	poly(w, col, v2(cx-h*0.62, cy-h*0.05), v2(cx-h*0.62, cy+h*0.8),
		v2(cx+h*0.62, cy+h*0.8), v2(cx+h*0.62, cy-h*0.05))
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
	poly(w, col, a, b, c)
}

// DrawCheckIcon draws the confirm tick.
func DrawCheckIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	poly(w, col, v2(cx-h*0.7, cy), v2(cx-h*0.15, cy+h*0.55), v2(cx+h*0.7, cy-h*0.55))
}

// DrawCrossIcon draws the cancel cross.
func DrawCrossIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx-h*0.6, cy-h*0.6), v2(cx+h*0.6, cy+h*0.6), w, col)
	line(v2(cx+h*0.6, cy-h*0.6), v2(cx-h*0.6, cy+h*0.6), w, col)
}

// DrawEyeIcon draws the visibility toggle: an open eye when visible, and the
// same eye struck through when hidden.
func DrawEyeIcon(cx, cy, size float64, visible bool, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// Two arcs meeting at the corners approximate the lens shape.
	const steps = 10
	for _, sign := range []float64{-1, 1} {
		var pts []rl.Vector2
		for i := 0; i <= steps; i++ {
			t := -1 + 2*float64(i)/steps
			x := cx + t*h*0.85
			y := cy + sign*(1-t*t)*h*0.5
			pts = append(pts, v2(x, y))
		}
		poly(w, col, pts...)
	}
	if visible {
		rl.DrawCircleLinesV(v2(cx, cy), float32(h*0.28), col)
	} else {
		line(v2(cx-h*0.85, cy+h*0.7), v2(cx+h*0.85, cy-h*0.7), w, col)
	}
}

// DrawPencilIcon draws the rename affordance.
func DrawPencilIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	tip := v2(cx-h*0.7, cy+h*0.7)
	poly(w, col, tip, v2(cx-h*0.5, cy+h*0.1), v2(cx+h*0.5, cy-h*0.9), v2(cx+h*0.75, cy-h*0.6))
	line(v2(cx+h*0.75, cy-h*0.6), v2(cx-h*0.25, cy+h*0.4), w, col)
	line(v2(cx-h*0.25, cy+h*0.4), tip, w, col)
}

// DrawTrashIcon draws the delete affordance.
func DrawTrashIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx-h*0.8, cy-h*0.5), v2(cx+h*0.8, cy-h*0.5), w, col)
	poly(w, col, v2(cx-h*0.55, cy-h*0.5), v2(cx-h*0.42, cy+h*0.8),
		v2(cx+h*0.42, cy+h*0.8), v2(cx+h*0.55, cy-h*0.5))
	poly(w, col, v2(cx-h*0.3, cy-h*0.5), v2(cx-h*0.3, cy-h*0.85),
		v2(cx+h*0.3, cy-h*0.85), v2(cx+h*0.3, cy-h*0.5))
}

// DrawPlaneIcon draws the tree icon for a default plane: a parallelogram.
func DrawPlaneIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	closedPoly(w, col,
		v2(cx-h*0.9, cy+h*0.35), v2(cx-h*0.2, cy-h*0.6),
		v2(cx+h*0.9, cy-h*0.35), v2(cx+h*0.2, cy+h*0.6))
}

// DrawBodyIcon draws the tree icon for a body: a small wireframe cube.
func DrawBodyIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// Front face.
	fl, ft := cx-h*0.8, cy-h*0.4
	fr, fb := cx+h*0.3, cy+h*0.75
	closedPoly(w, col, v2(fl, ft), v2(fr, ft), v2(fr, fb), v2(fl, fb))
	// Top and side, drawn as the two visible offset edges.
	dx, dy := h*0.5, -h*0.45
	poly(w, col, v2(fl, ft), v2(fl+dx, ft+dy), v2(fr+dx, ft+dy), v2(fr, ft))
	line(v2(fr+dx, ft+dy), v2(fr+dx, fb+dy), w, col)
	line(v2(fr+dx, fb+dy), v2(fr, fb), w, col)
}

// DrawSketchIcon draws the tree icon for a sketch: an open profile with nodes.
func DrawSketchIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	a := v2(cx-h*0.8, cy+h*0.6)
	b := v2(cx-h*0.2, cy-h*0.7)
	c := v2(cx+h*0.8, cy-h*0.1)
	poly(w, col, a, b, c)
	for _, p := range []rl.Vector2{a, b, c} {
		rl.DrawCircleV(p, float32(h*0.16), col)
	}
}

// Toolbar tool icons (SPEC-UX §2).

// DrawSketchToolIcon is the pencil-on-plane mark for the Sketch tool.
func DrawSketchToolIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	closedPoly(w, col,
		v2(cx-h*0.95, cy+h*0.5), v2(cx-h*0.35, cy-h*0.35),
		v2(cx+h*0.55, cy-h*0.15), v2(cx-h*0.05, cy+h*0.7))
	poly(w, col, v2(cx+h*0.1, cy+h*0.15), v2(cx+h*0.75, cy-h*0.8), v2(cx+h*0.95, cy-h*0.55))
}

// DrawExtrudeIcon is a face with an arrow pulling out of it.
func DrawExtrudeIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	closedPoly(w, col,
		v2(cx-h*0.9, cy+h*0.1), v2(cx-h*0.2, cy+h*0.55),
		v2(cx-h*0.2, cy-h*0.25), v2(cx-h*0.9, cy-h*0.7))
	line(v2(cx+h*0.05, cy+h*0.05), v2(cx+h*0.85, cy+h*0.05), w, col)
	poly(w, col, v2(cx+h*0.55, cy-h*0.25), v2(cx+h*0.9, cy+h*0.05), v2(cx+h*0.55, cy+h*0.35))
}

// DrawBooleanIcon is two overlapping circles.
func DrawBooleanIcon(cx, cy, size float64, col color.RGBA) {
	h := size / 2
	rl.DrawCircleLinesV(v2(cx-h*0.3, cy), float32(h*0.6), col)
	rl.DrawCircleLinesV(v2(cx+h*0.3, cy), float32(h*0.6), col)
}

// DrawMoveIcon is the four-way arrow of the transform tool.
func DrawMoveIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx-h*0.9, cy), v2(cx+h*0.9, cy), w, col)
	line(v2(cx, cy-h*0.9), v2(cx, cy+h*0.9), w, col)
	for _, a := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
		tx, ty := cx+math.Cos(a)*h*0.9, cy+math.Sin(a)*h*0.9
		nx, ny := math.Cos(a+2.5)*h*0.32, math.Sin(a+2.5)*h*0.32
		mx, my := math.Cos(a-2.5)*h*0.32, math.Sin(a-2.5)*h*0.32
		poly(w, col, v2(tx+nx, ty+ny), v2(tx, ty), v2(tx+mx, ty+my))
	}
}

// DrawPaintIcon is a brush.
func DrawPaintIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	poly(w, col, v2(cx-h*0.75, cy+h*0.85), v2(cx-h*0.75, cy+h*0.2),
		v2(cx+h*0.2, cy+h*0.2), v2(cx+h*0.2, cy+h*0.85))
	line(v2(cx-h*0.75, cy+h*0.85), v2(cx+h*0.2, cy+h*0.85), w, col)
	line(v2(cx-h*0.28, cy+h*0.2), v2(cx+h*0.72, cy-h*0.85), w, col)
	line(v2(cx-h*0.02, cy+h*0.2), v2(cx+h*0.95, cy-h*0.62), w, col)
}

// DrawUndoIcon and DrawRedoIcon are the curved history arrows.
func DrawUndoIcon(cx, cy, size float64, col color.RGBA) { drawHistoryArrow(cx, cy, size, col, false) }
func DrawRedoIcon(cx, cy, size float64, col color.RGBA) { drawHistoryArrow(cx, cy, size, col, true) }

func drawHistoryArrow(cx, cy, size float64, col color.RGBA, mirrored bool) {
	w := strokeWidth(size)
	h := size / 2
	dir := 1.0
	if mirrored {
		dir = -1
	}
	var pts []rl.Vector2
	const steps = 12
	for i := 0; i <= steps; i++ {
		a := math.Pi*0.15 + math.Pi*0.95*float64(i)/steps
		pts = append(pts, v2(cx+dir*math.Cos(a)*h*0.8, cy-math.Sin(a)*h*0.6+h*0.2))
	}
	poly(w, col, pts...)
	tip := pts[len(pts)-1]
	poly(w, col,
		v2(float64(tip.X)+dir*h*0.05, float64(tip.Y)-h*0.45),
		v2(float64(tip.X), float64(tip.Y)),
		v2(float64(tip.X)+dir*h*0.5, float64(tip.Y)-h*0.1))
}

// DrawSettingsIcon is the gear in the toolbar's right corner.
func DrawSettingsIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	rl.DrawCircleLinesV(v2(cx, cy), float32(h*0.38), col)
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		line(v2(cx+math.Cos(a)*h*0.55, cy+math.Sin(a)*h*0.55),
			v2(cx+math.Cos(a)*h*0.92, cy+math.Sin(a)*h*0.92), w, col)
	}
}
