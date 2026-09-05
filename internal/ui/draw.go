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

// CheckerCell is the checkerboard square's edge in logical pixels. Small
// enough that a 22px swatch shows several of them, because one square is a
// two-tone chip and four is a checkerboard.
const CheckerCell = 5

// FillChecker fills a rounded rectangle with the transparency checkerboard,
// which is the one convention every drawing program shares for "you can see
// through this" (V-158).
//
// Cells that would spill out of a rounded corner are simply not drawn: the
// light ground shows there instead. That reads as a light square at the
// corner, which the pattern already has plenty of, and it avoids the corner
// pixels poking outside the chip.
func (c *Context) FillChecker(r rl.Rectangle, radius float64) {
	// Mid greys rather than the white-and-silver a paint program on a white
	// canvas uses: the point is to be legible under a colour, not to be the
	// brightest thing on a dark panel.
	light := Shade(ColorText, 0.55)
	dark := Shade(ColorText, 0.34)
	c.FillRounded(r, radius, light)

	cell := c.Px(CheckerCell)
	if cell < 2 {
		cell = 2
	}
	rp := c.Px(radius)
	for iy := 0; float32(iy)*cell < r.Height; iy++ {
		for ix := 0; float32(ix)*cell < r.Width; ix++ {
			if (ix+iy)%2 == 0 {
				continue
			}
			x, y := r.X+float32(ix)*cell, r.Y+float32(iy)*cell
			w, h := cell, cell
			if x+w > r.X+r.Width {
				w = r.X + r.Width - x
			}
			if y+h > r.Y+r.Height {
				h = r.Y + r.Height - y
			}
			box := Rect(x, y, w, h)
			if !insideRounded(r, rp, box) {
				continue
			}
			FillRect(box, dark)
		}
	}
}

// insideRounded reports whether a box lies wholly within a rounded rectangle.
func insideRounded(r rl.Rectangle, radius float32, box rl.Rectangle) bool {
	if radius <= 0 {
		return true
	}
	// Only the four corner quadrants can fail, and within one the farthest
	// point of the box from the corner's centre is the one to test.
	corners := [4][2]float32{
		{r.X + radius, r.Y + radius},
		{r.X + r.Width - radius, r.Y + radius},
		{r.X + radius, r.Y + r.Height - radius},
		{r.X + r.Width - radius, r.Y + r.Height - radius},
	}
	for i, cn := range corners {
		cx, cy := cn[0], cn[1]
		px, py := box.X, box.Y
		if i == 1 || i == 3 {
			px = box.X + box.Width
		}
		if i == 2 || i == 3 {
			py = box.Y + box.Height
		}
		if (i%2 == 0 && px > cx) || (i%2 == 1 && px < cx) {
			continue // the box does not reach into this corner horizontally
		}
		if (i < 2 && py > cy) || (i >= 2 && py < cy) {
			continue // nor vertically
		}
		dx, dy := px-cx, py-cy
		if dx*dx+dy*dy > radius*radius {
			return false
		}
	}
	return true
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

// Shadow lays the soft drop shadow a raised surface sits on: a few expanding
// translucent layers standing in for a blur, offset downward because the light
// comes from above. Cheap enough to run under every card every frame, and the
// single biggest reason the chrome stopped looking pasted on (V-88).
func (c *Context) Shadow(r rl.Rectangle, radius, alpha float64) {
	ink := Fade(ColorShadow, alpha)
	if ink.A == 0 {
		return
	}
	// Layered rings standing in for a blur, with the density falling away as
	// they spread. Equal-strength rings pile into a flat dark band with a hard
	// outer rim — a sticker outline, not a shadow; the falloff is what makes
	// the ink thin out the way light does.
	weights := [5]float64{1, 0.65, 0.4, 0.22, 0.1}
	for i := 1; i <= 5; i++ {
		spread := c.Px(float64(i) * 1.8)
		drop := c.Px(float64(i) * 1.1)
		layer := rl.Rectangle{
			X: r.X - spread, Y: r.Y - spread + drop,
			Width: r.Width + 2*spread, Height: r.Height + 2*spread,
		}
		c.FillRounded(layer, radius+float64(i)*1.8, Fade(ink, weights[i-1]))
	}
}

// Bevel runs the one-pixel light along a raised surface's top edge, inset past
// the corner radius so it never pokes out of the rounding.
func (c *Context) Bevel(r rl.Rectangle, radius float64) {
	in := c.Px(radius)
	c.HairlineH(r.X+in, r.Y+c.hairline(), r.Width-2*in, ColorBevel)
}

// Card paints a floating card: shadowed, rounded, lit along its top edge, with
// a border.
func (c *Context) Card(r rl.Rectangle) {
	c.Shadow(r, CardRadius, 1)
	c.FillRounded(r, CardRadius, ColorCard)
	c.StrokeRounded(r, CardRadius, ColorStroke)
	c.Bevel(r, CardRadius)
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

// TextWrapped draws text broken across as many lines of r as it needs, up to
// as many as fit.
//
// The kit had no wrapping until something had a sentence to say rather than a
// label: the export card's caveat about a format is the sort of thing that has
// to be readable in the panel rather than truncated with an ellipsis.
func (c *Context) TextWrapped(r rl.Rectangle, s string, size float64, col color.RGBA) {
	if s == "" || r.Width <= 0 {
		return
	}
	lineH := c.Fonts.LineHeight(size)
	if lineH <= 0 {
		return
	}
	maxLines := int(r.Height / lineH)
	if maxLines < 1 {
		maxLines = 1
	}

	words := strings.Fields(s)
	var lines []string
	current := ""
	for _, w := range words {
		try := w
		if current != "" {
			try = current + " " + w
		}
		if c.TextWidth(try, size) <= r.Width || current == "" {
			current = try
			continue
		}
		lines = append(lines, current)
		current = w
		if len(lines) == maxLines {
			break
		}
	}
	if current != "" && len(lines) < maxLines {
		lines = append(lines, current)
	}

	y := r.Y
	for i, line := range lines {
		if i == maxLines-1 && i < len(lines)-1 {
			line = c.Truncate(line+" …", size, r.Width)
		}
		c.Text(Rect(r.X, y, r.Width, lineH), line, size, col)
		y += lineH
	}
}
