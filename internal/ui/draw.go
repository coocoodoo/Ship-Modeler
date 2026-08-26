package ui

import (
	"image/color"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Shared drawing primitives. Every widget builds from these, so corner radii,
// hairlines and text clipping stay consistent across the kit (SPEC-UX §3).

// Rect builds a rectangle from floats, which reads better than the struct
// literal at call sites.
func Rect(x, y, w, h float32) rl.Rectangle {
	return rl.Rectangle{X: x, Y: y, Width: w, Height: h}
}

// Inset shrinks a rectangle on every side.
func Inset(r rl.Rectangle, d float32) rl.Rectangle {
	return rl.Rectangle{X: r.X + d, Y: r.Y + d, Width: r.Width - 2*d, Height: r.Height - 2*d}
}

// InsetXY shrinks a rectangle horizontally and vertically by different amounts.
func InsetXY(r rl.Rectangle, dx, dy float32) rl.Rectangle {
	return rl.Rectangle{X: r.X + dx, Y: r.Y + dy, Width: r.Width - 2*dx, Height: r.Height - 2*dy}
}

// SplitLeft cuts w pixels off the left of a rectangle and returns both parts.
func SplitLeft(r rl.Rectangle, w float32) (left, rest rl.Rectangle) {
	if w > r.Width {
		w = r.Width
	}
	left = rl.Rectangle{X: r.X, Y: r.Y, Width: w, Height: r.Height}
	rest = rl.Rectangle{X: r.X + w, Y: r.Y, Width: r.Width - w, Height: r.Height}
	return
}

// SplitRight cuts w pixels off the right of a rectangle.
func SplitRight(r rl.Rectangle, w float32) (right, rest rl.Rectangle) {
	if w > r.Width {
		w = r.Width
	}
	right = rl.Rectangle{X: r.X + r.Width - w, Y: r.Y, Width: w, Height: r.Height}
	rest = rl.Rectangle{X: r.X, Y: r.Y, Width: r.Width - w, Height: r.Height}
	return
}

// SplitTop cuts h pixels off the top of a rectangle.
func SplitTop(r rl.Rectangle, h float32) (top, rest rl.Rectangle) {
	if h > r.Height {
		h = r.Height
	}
	top = rl.Rectangle{X: r.X, Y: r.Y, Width: r.Width, Height: h}
	rest = rl.Rectangle{X: r.X, Y: r.Y + h, Width: r.Width, Height: r.Height - h}
	return
}

// SplitBottom cuts h pixels off the bottom of a rectangle.
func SplitBottom(r rl.Rectangle, h float32) (bottom, rest rl.Rectangle) {
	if h > r.Height {
		h = r.Height
	}
	bottom = rl.Rectangle{X: r.X, Y: r.Y + r.Height - h, Width: r.Width, Height: h}
	rest = rl.Rectangle{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height - h}
	return
}

// Center returns the midpoint of a rectangle.
func Center(r rl.Rectangle) rl.Vector2 {
	return rl.Vector2{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
}

// roundness converts a corner radius in device pixels into raylib's
// roundness fraction, which is relative to the shorter side.
func roundness(r rl.Rectangle, radiusPx float32) float32 {
	short := r.Width
	if r.Height < short {
		short = r.Height
	}
	if short <= 0 {
		return 0
	}
	v := 2 * radiusPx / short
	if v > 1 {
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}

// FillRect paints a plain rectangle.
func FillRect(r rl.Rectangle, c color.RGBA) {
	rl.DrawRectangleRec(r, c)
}

// FillRounded paints a rounded rectangle with a radius in logical pixels.
func (c *Context) FillRounded(r rl.Rectangle, radius float64, col color.RGBA) {
	rp := c.Px(radius)
	if rp <= 0.5 || r.Width < 2 || r.Height < 2 {
		rl.DrawRectangleRec(r, col)
		return
	}
	rl.DrawRectangleRounded(r, roundness(r, rp), 6, col)
}

// StrokeRounded outlines a rounded rectangle.
func (c *Context) StrokeRounded(r rl.Rectangle, radius float64, col color.RGBA) {
	rp := c.Px(radius)
	if rp <= 0.5 || r.Width < 2 || r.Height < 2 {
		rl.DrawRectangleLinesEx(r, c.hairline(), col)
		return
	}
	rl.DrawRectangleRoundedLinesEx(r, roundness(r, rp), 6, c.hairline(), col)
}

// hairline is the one-device-pixel border width at the current scale.
func (c *Context) hairline() float32 {
	w := float32(c.Scale)
	if w < 1 {
		w = 1
	}
	return w
}

// Panel paints a background panel: the toolbar, tree and hint bar.
func (c *Context) Panel(r rl.Rectangle) {
	FillRect(r, ColorPanel)
}

// Card paints a floating card: rounded, slightly lighter, with a border.
func (c *Context) Card(r rl.Rectangle) {
	c.FillRounded(r, CardRadius, ColorCard)
	c.StrokeRounded(r, CardRadius, ColorStroke)
}

// HairlineH draws a horizontal separator across a rectangle's top edge.
func (c *Context) HairlineH(x, y, w float32, col color.RGBA) {
	rl.DrawRectangleRec(Rect(x, y, w, c.hairline()), col)
}

// HairlineV draws a vertical separator.
func (c *Context) HairlineV(x, y, h float32, col color.RGBA) {
	rl.DrawRectangleRec(Rect(x, y, c.hairline(), h), col)
}

// Text draws left-aligned text vertically centred in a rectangle, truncating
// with an ellipsis when it does not fit. Every label in the kit goes through
// here so nothing ever spills out of its row.
func (c *Context) Text(r rl.Rectangle, s string, size float64, col color.RGBA) {
	c.textIn(r, c.fontFor(size), s, size, col, false)
}

// TextCentered draws text centred in both axes.
func (c *Context) TextCentered(r rl.Rectangle, s string, size float64, col color.RGBA) {
	c.textIn(r, c.fontFor(size), s, size, col, true)
}

func (c *Context) fontFor(size float64) rl.Font {
	switch {
	case size >= FontSizeHeader:
		return c.Fonts.Header
	case size <= FontSizeSmall:
		return c.Fonts.Small
	default:
		return c.Fonts.UI
	}
}

func (c *Context) textIn(r rl.Rectangle, font rl.Font, s string, size float64, col color.RGBA, centered bool) {
	if s == "" || r.Width <= 0 {
		return
	}
	s = c.Truncate(s, size, r.Width)
	w, h := c.Fonts.Measure(font, s, size)
	y := r.Y + (r.Height-h)/2
	x := r.X
	if centered {
		x = r.X + (r.Width-w)/2
	}
	c.Fonts.Draw(font, s, x, y, size, col)
}

// Truncate shortens text with a trailing ellipsis so it fits maxWidth.
func (c *Context) Truncate(s string, size float64, maxWidth float32) string {
	font := c.fontFor(size)
	if w, _ := c.Fonts.Measure(font, s, size); w <= maxWidth {
		return s
	}
	const ellipsis = "…"
	runes := []rune(s)
	// Binary search the longest prefix that fits with the ellipsis appended.
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		w, _ := c.Fonts.Measure(font, string(runes[:mid])+ellipsis, size)
		if w <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return strings.TrimRight(string(runes[:lo]), " ") + ellipsis
}

// TextWidth measures a string at a logical size.
func (c *Context) TextWidth(s string, size float64) float32 {
	w, _ := c.Fonts.Measure(c.fontFor(size), s, size)
	return w
}

// Overlay dims everything behind a modal.
func (c *Context) Overlay(r rl.Rectangle) {
	FillRect(r, color.RGBA{R: 0, G: 0, B: 0, A: 0x9A})
}
