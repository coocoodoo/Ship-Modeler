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

// Sketch tool icons (SPEC-UX §8.2).

// DrawCursorIcon is the arrow of the Select tool.
func DrawCursorIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	tip := v2(cx-h*0.5, cy-h*0.85)
	closedPoly(w, col, tip, v2(cx+h*0.35, cy+h*0.25), v2(cx-h*0.05, cy+h*0.25),
		v2(cx-h*0.5, cy+h*0.75))
	line(v2(cx-h*0.02, cy+h*0.25), v2(cx+h*0.4, cy+h*0.9), w, col)
}

// DrawLineToolIcon is a stroke with a node at each end.
func DrawLineToolIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	a := v2(cx-h*0.75, cy+h*0.7)
	b := v2(cx+h*0.75, cy-h*0.7)
	line(a, b, w, col)
	rl.DrawCircleV(a, float32(h*0.22), col)
	rl.DrawCircleV(b, float32(h*0.22), col)
}

// DrawRectToolIcon is an outlined rectangle with corner nodes.
func DrawRectToolIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	l, t := cx-h*0.8, cy-h*0.6
	r, b := cx+h*0.8, cy+h*0.6
	closedPoly(w, col, v2(l, t), v2(r, t), v2(r, b), v2(l, b))
	rl.DrawCircleV(v2(l, t), float32(h*0.2), col)
	rl.DrawCircleV(v2(r, b), float32(h*0.2), col)
}

// DrawCircleToolIcon is a polygon-ish circle with a centre dot, because a
// circle in this app is a regular n-gon (D-06).
func DrawCircleToolIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	const sides = 8
	pts := make([]rl.Vector2, sides)
	for i := 0; i < sides; i++ {
		a := 2*math.Pi*float64(i)/sides + math.Pi/sides
		pts[i] = v2(cx+math.Cos(a)*h*0.82, cy+math.Sin(a)*h*0.82)
	}
	closedPoly(w, col, pts...)
	rl.DrawCircleV(v2(cx, cy), float32(h*0.16), col)
}

// DrawFlipIcon is the two-way arrow that reverses an extrude (SPEC-UX §9.2).
func DrawFlipIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx-h*0.8, cy-h*0.35), v2(cx+h*0.8, cy-h*0.35), w, col)
	poly(w, col, v2(cx+h*0.4, cy-h*0.75), v2(cx+h*0.85, cy-h*0.35), v2(cx+h*0.4, cy+h*0.05))
	line(v2(cx-h*0.8, cy+h*0.35), v2(cx+h*0.8, cy+h*0.35), w, col)
	poly(w, col, v2(cx-h*0.4, cy-h*0.05), v2(cx-h*0.85, cy+h*0.35), v2(cx-h*0.4, cy+h*0.75))
}

// Paint tool icons (SPEC-UX §13.1). The panel is specced with emoji, which the
// font atlas does not carry — every glyph in this program is a stroke drawing
// for exactly that reason (D-11).

// DrawEraserIcon is a rubber on its side, wiping right to left.
func DrawEraserIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// The block, leaning the way a held eraser does.
	closedPoly(w, col,
		v2(cx-h*0.35, cy+h*0.5), v2(cx+h*0.3, cy-h*0.7),
		v2(cx+h*0.85, cy-h*0.2), v2(cx+h*0.2, cy+h*0.5))
	// The worn edge it rubs with, and the line it has cleared.
	line(v2(cx-h*0.05, cy-h*0.1), v2(cx+h*0.55, cy+h*0.4), w, col)
	line(v2(cx-h*0.85, cy+h*0.75), v2(cx+h*0.6, cy+h*0.75), w, col)
}

// DrawFillIcon is a tipped bucket with a drop coming out of it.
func DrawFillIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	closedPoly(w, col,
		v2(cx-h*0.8, cy-h*0.15), v2(cx+h*0.15, cy-h*0.8),
		v2(cx+h*0.7, cy+h*0.1), v2(cx-h*0.25, cy+h*0.75))
	// The handle, and the drop that has already left.
	poly(w, col, v2(cx-h*0.55, cy-h*0.4), v2(cx-h*0.2, cy-h*0.85), v2(cx+h*0.1, cy-h*0.55))
	rl.DrawCircleV(v2(cx+h*0.72, cy+h*0.6), float32(h*0.2), col)
}

// DrawDropperIcon is the eyedropper: a slanted pipette with a bulb.
func DrawDropperIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx-h*0.8, cy+h*0.8), v2(cx+h*0.25, cy-h*0.25), w, col)
	closedPoly(w, col,
		v2(cx+h*0.05, cy-h*0.45), v2(cx+h*0.45, cy-h*0.85),
		v2(cx+h*0.85, cy-h*0.45), v2(cx+h*0.45, cy-h*0.05))
	// The tip, drawn solid: it is the part that touches the pixel.
	rl.DrawCircleV(v2(cx-h*0.72, cy+h*0.72), float32(h*0.2), col)
}

// DrawImportIcon is an arrow landing in a tray, used by the palette import.
func DrawImportIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx, cy-h*0.85), v2(cx, cy+h*0.15), w, col)
	poly(w, col, v2(cx-h*0.4, cy-h*0.25), v2(cx, cy+h*0.2), v2(cx+h*0.4, cy-h*0.25))
	poly(w, col, v2(cx-h*0.8, cy+h*0.35), v2(cx-h*0.8, cy+h*0.8),
		v2(cx+h*0.8, cy+h*0.8), v2(cx+h*0.8, cy+h*0.35))
}

// DrawGradientIcon is a ramp: a box whose fill steps from dense to sparse,
// drawn as bands because that is what an ordered-dither ramp looks like.
func DrawGradientIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	l, t := cx-h*0.85, cy-h*0.75
	r, b := cx+h*0.85, cy+h*0.75
	closedPoly(w, col, v2(l, t), v2(r, t), v2(r, b), v2(l, b))
	// Four bands thinning left to right: solid, three-quarters, half, a dash.
	span := (r - l) / 5
	for i := 0; i < 4; i++ {
		x := l + span*(float64(i)+0.5)
		frac := 1 - float64(i)*0.28
		line(v2(x, cy-h*0.55*frac), v2(x, cy+h*0.55*frac), w, col)
	}
}

// DrawSwapIcon is the two-way arrow that exchanges the near and far colours.
func DrawSwapIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	line(v2(cx-h*0.7, cy-h*0.35), v2(cx+h*0.7, cy-h*0.35), w, col)
	poly(w, col, v2(cx+h*0.3, cy-h*0.7), v2(cx+h*0.75, cy-h*0.35), v2(cx+h*0.3, cy))
	line(v2(cx-h*0.7, cy+h*0.35), v2(cx+h*0.7, cy+h*0.35), w, col)
	poly(w, col, v2(cx-h*0.3, cy), v2(cx-h*0.75, cy+h*0.35), v2(cx-h*0.3, cy+h*0.7))
}

// DrawSaveIcon is the floppy every program still uses, because everyone still
// reads it.
func DrawSaveIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	closedPoly(w, col, v2(cx-h*0.8, cy-h*0.8), v2(cx+h*0.5, cy-h*0.8),
		v2(cx+h*0.8, cy-h*0.5), v2(cx+h*0.8, cy+h*0.8), v2(cx-h*0.8, cy+h*0.8))
	// The shutter at the top and the label at the bottom.
	closedPoly(w, col, v2(cx-h*0.4, cy-h*0.8), v2(cx+h*0.3, cy-h*0.8),
		v2(cx+h*0.3, cy-h*0.25), v2(cx-h*0.4, cy-h*0.25))
	closedPoly(w, col, v2(cx-h*0.5, cy+h*0.15), v2(cx+h*0.5, cy+h*0.15),
		v2(cx+h*0.5, cy+h*0.8), v2(cx-h*0.5, cy+h*0.8))
}

// DrawOpenIcon is a folder with its lid lifted.
func DrawOpenIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	poly(w, col, v2(cx-h*0.85, cy+h*0.6), v2(cx-h*0.85, cy-h*0.6),
		v2(cx-h*0.2, cy-h*0.6), v2(cx+h*0.05, cy-h*0.25), v2(cx+h*0.6, cy-h*0.25))
	// The front flap, tilted, which is what says "open".
	poly(w, col, v2(cx-h*0.85, cy+h*0.6), v2(cx+h*0.85, cy+h*0.6),
		v2(cx+h*0.6, cy-h*0.05), v2(cx-h*0.6, cy-h*0.05))
}

// --- Sketch tool variants (Sketch_func.md §4.2) ---------------------------

// DrawMidLineIcon is a stroke with its node in the middle, which is where the
// midpoint line is drawn from.
func DrawMidLineIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	a := v2(cx-h*0.75, cy+h*0.7)
	b := v2(cx+h*0.75, cy-h*0.7)
	line(a, b, w, col)
	rl.DrawCircleV(v2(cx, cy), float32(h*0.24), col)
}

// DrawCenterRectIcon is a rectangle with its centre marked.
func DrawCenterRectIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	l, t := cx-h*0.8, cy-h*0.6
	r, b := cx+h*0.8, cy+h*0.6
	closedPoly(w, col, v2(l, t), v2(r, t), v2(r, b), v2(l, b))
	rl.DrawCircleV(v2(cx, cy), float32(h*0.2), col)
}

// DrawAlignedRectIcon is a rectangle turned off axis, which is the one thing
// this variant does that the others cannot.
func DrawAlignedRectIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	const a = 0.42 // radians of tilt
	corner := func(dx, dy float64) rl.Vector2 {
		return v2(cx+dx*math.Cos(a)-dy*math.Sin(a), cy+dx*math.Sin(a)+dy*math.Cos(a))
	}
	closedPoly(w, col,
		corner(-h*0.8, -h*0.5), corner(h*0.8, -h*0.5),
		corner(h*0.8, h*0.5), corner(-h*0.8, h*0.5))
}

// DrawPointToolIcon is a dot in a ring: a position, marked.
func DrawPointToolIcon(cx, cy, size float64, col color.RGBA) {
	h := size / 2
	rl.DrawCircleLinesV(v2(cx, cy), float32(h*0.72), col)
	rl.DrawCircleV(v2(cx, cy), float32(h*0.22), col)
}

// DrawConstructionIcon is a dashed diagonal: geometry that guides without
// being part of the shape.
func DrawConstructionIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	const dashes = 3
	for i := 0; i < dashes; i++ {
		t0 := float64(i) / dashes
		t1 := t0 + 0.62/dashes
		lerp := func(t float64) rl.Vector2 {
			return v2(cx-h*0.8+t*h*1.6, cy+h*0.8-t*h*1.6)
		}
		line(lerp(t0), lerp(t1), w, col)
	}
}

// DrawCircle3Icon is a circle with three points marked on it.
func DrawCircle3Icon(cx, cy, size float64, col color.RGBA) {
	h := size / 2
	rl.DrawCircleLinesV(v2(cx, cy), float32(h*0.8), col)
	for _, a := range []float64{-math.Pi / 2, math.Pi / 6, 5 * math.Pi / 6} {
		rl.DrawCircleV(v2(cx+math.Cos(a)*h*0.8, cy+math.Sin(a)*h*0.8), float32(h*0.2), col)
	}
}

// DrawEllipseIcon is an oval, wider than it is tall.
func DrawEllipseIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	const sides = 16
	pts := make([]rl.Vector2, sides)
	for i := 0; i < sides; i++ {
		a := 2 * math.Pi * float64(i) / sides
		pts[i] = v2(cx+math.Cos(a)*h*0.9, cy+math.Sin(a)*h*0.5)
	}
	closedPoly(w, col, pts...)
}

// arcSweep strokes a partial circle, which every arc icon is built from.
func arcSweep(cx, cy, radius, from, to float64, w float32, col color.RGBA) {
	const steps = 12
	prev := v2(cx+math.Cos(from)*radius, cy+math.Sin(from)*radius)
	for i := 1; i <= steps; i++ {
		a := from + (to-from)*float64(i)/steps
		next := v2(cx+math.Cos(a)*radius, cy+math.Sin(a)*radius)
		line(prev, next, w, col)
		prev = next
	}
}

// DrawArc3Icon is an arc with its three defining points marked.
func DrawArc3Icon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	r := h * 0.85
	from, to := math.Pi, 2*math.Pi
	arcSweep(cx, cy+h*0.35, r, from, to, w, col)
	for _, a := range []float64{from, (from + to) / 2, to} {
		rl.DrawCircleV(v2(cx+math.Cos(a)*r, cy+h*0.35+math.Sin(a)*r), float32(h*0.2), col)
	}
}

// DrawArcTangentIcon is an arc leaving a straight line smoothly.
func DrawArcTangentIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// The line runs in along the bottom, and the arc curls up off its end.
	line(v2(cx-h*0.9, cy+h*0.6), v2(cx, cy+h*0.6), w, col)
	arcSweep(cx, cy-h*0.15, h*0.75, math.Pi/2, -math.Pi/6, w, col)
}

// DrawArcCenterIcon is an arc with its centre marked and radii drawn to it.
func DrawArcCenterIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	r := h * 0.85
	cyy := cy + h*0.35
	from, to := math.Pi, 2*math.Pi
	arcSweep(cx, cyy, r, from, to, w, col)
	rl.DrawCircleV(v2(cx, cyy), float32(h*0.2), col)
	line(v2(cx, cyy), v2(cx+math.Cos(from)*r, cyy+math.Sin(from)*r), w, Fade(col, 0.5))
	line(v2(cx, cyy), v2(cx+math.Cos(to)*r, cyy+math.Sin(to)*r), w, Fade(col, 0.5))
}

// DrawPolygonIcon is a hexagon with a corner marked: the inscribed variant is
// measured to a corner.
func DrawPolygonIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	pts := make([]rl.Vector2, 6)
	for i := 0; i < 6; i++ {
		a := 2*math.Pi*float64(i)/6 - math.Pi/2
		pts[i] = v2(cx+math.Cos(a)*h*0.85, cy+math.Sin(a)*h*0.85)
	}
	closedPoly(w, col, pts...)
	rl.DrawCircleV(pts[0], float32(h*0.2), col)
}

// DrawPolygonCircIcon is a hexagon with a flat side marked: the circumscribed
// variant is measured to a side.
func DrawPolygonCircIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	pts := make([]rl.Vector2, 6)
	for i := 0; i < 6; i++ {
		a := 2*math.Pi*float64(i)/6 - math.Pi/2
		pts[i] = v2(cx+math.Cos(a)*h*0.85, cy+math.Sin(a)*h*0.85)
	}
	closedPoly(w, col, pts...)
	mid := rl.Vector2{X: (pts[0].X + pts[1].X) / 2, Y: (pts[0].Y + pts[1].Y) / 2}
	rl.DrawCircleV(mid, float32(h*0.2), col)
}

// DrawSlotIcon is a capsule lying on its side.
func DrawSlotIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	r := h * 0.45
	left, right := cx-h*0.45, cx+h*0.45
	line(v2(left, cy-r), v2(right, cy-r), w, col)
	line(v2(left, cy+r), v2(right, cy+r), w, col)
	arcSweep(right, cy, r, -math.Pi/2, math.Pi/2, w, col)
	arcSweep(left, cy, r, math.Pi/2, 3*math.Pi/2, w, col)
}

// DrawSplineIcon is a curve through three marked points.
func DrawSplineIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// An S-curve, sampled from a sine so it reads as smooth at icon size.
	const steps = 16
	sample := func(t float64) rl.Vector2 {
		x := cx + (t-0.5)*h*1.8
		y := cy - math.Sin(t*2*math.Pi)*h*0.5
		return v2(x, y)
	}
	prev := sample(0)
	for i := 1; i <= steps; i++ {
		next := sample(float64(i) / steps)
		line(prev, next, w, col)
		prev = next
	}
	for _, t := range []float64{0, 0.5, 1} {
		rl.DrawCircleV(sample(t), float32(h*0.18), col)
	}
}

// DrawBezierIcon is a curve with its control cage.
func DrawBezierIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	p0 := v2(cx-h*0.85, cy+h*0.6)
	p1 := v2(cx-h*0.5, cy-h*0.8)
	p2 := v2(cx+h*0.5, cy-h*0.8)
	p3 := v2(cx+h*0.85, cy+h*0.6)
	// The cage, faint.
	faint := Fade(col, 0.45)
	line(p0, p1, w, faint)
	line(p2, p3, w, faint)
	// The curve itself.
	const steps = 16
	at := func(t float64) rl.Vector2 {
		u := 1 - t
		a, b := float32(u*u*u), float32(3*u*u*t)
		c, d := float32(3*u*t*t), float32(t*t*t)
		return rl.Vector2{
			X: a*p0.X + b*p1.X + c*p2.X + d*p3.X,
			Y: a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y,
		}
	}
	prev := at(0)
	for i := 1; i <= steps; i++ {
		next := at(float64(i) / steps)
		line(prev, next, w, col)
		prev = next
	}
	rl.DrawCircleV(p1, float32(h*0.16), faint)
	rl.DrawCircleV(p2, float32(h*0.16), faint)
}

// DrawEdgeLineIcon is a corner with a stripe running along it: the edge-line
// tool paints a band that turns the corner onto both faces.
func DrawEdgeLineIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	h := size / 2
	// Two faces meeting at a vertical edge, drawn as a shallow V from above.
	apexTop := v2(cx, cy-h*0.85)
	apexBot := v2(cx, cy+h*0.6)
	left := v2(cx-h*0.85, cy-h*0.35)
	right := v2(cx+h*0.85, cy-h*0.35)
	faint := Fade(col, 0.5)
	line(left, apexTop, w, faint)
	line(apexTop, right, w, faint)
	line(left, rl.Vector2{X: left.X, Y: left.Y + float32(h*0.95)}, w, faint)
	line(right, rl.Vector2{X: right.X, Y: right.Y + float32(h*0.95)}, w, faint)
	// The band itself, down the shared edge.
	line(apexTop, apexBot, w*2.2, col)
}

// DrawMarkerIcon is an orientation dot: a small filled centre in a ring, the
// glyph for the front/top/thruster markers a game engine reads (V-131).
func DrawMarkerIcon(cx, cy, size float64, col color.RGBA) {
	r := size * 0.36
	rl.DrawCircleLinesV(rl.Vector2{X: float32(cx), Y: float32(cy)}, float32(r), col)
	rl.DrawCircleV(rl.Vector2{X: float32(cx), Y: float32(cy)}, float32(size*0.14), col)
}
