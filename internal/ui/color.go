package ui

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The colour swatch and its HSV popover (SPEC-UX §4). The picker is our own
// rather than the OS one, so it matches the theme and works identically in
// headless tests.

// popoverState tracks the one open popover. Only one may be open at a time.
type popoverState struct {
	id     ID
	anchor rl.Rectangle
	// hsv is the live colour while the picker is open.
	h, s, v float64
	alpha   uint8
}

// SwatchOpts configures a colour chip.
type SwatchOpts struct {
	// Tooltip names what this chip is for. A swatch is a colour and nothing
	// else on screen, so the copy is the only thing that says whether clicking
	// it recolours a body or arms a brush.
	Tooltip string
	// Selected rings the chip: which of a page of colours is the live one.
	Selected bool
	// Empty draws the chip as an unfilled slot, for a recents strip that has
	// not filled up yet.
	Empty bool
}

// ColorSwatch draws a colour chip and reports a click on it.
func (c *Context) ColorSwatch(id ID, r rl.Rectangle, col color.RGBA, opts SwatchOpts) bool {
	it := c.interact(id, r, opts.Empty)
	if opts.Empty {
		c.StrokeRounded(r, 4, Fade(ColorStroke, 0.7))
		return false
	}
	c.FillRounded(r, 4, col)

	border := ColorStroke
	switch {
	case opts.Selected:
		border = ColorText
	case it.Hovered:
		border = ColorText
	}
	c.StrokeRounded(r, 4, border)
	if opts.Selected {
		// A second ring outside the first, so the armed colour still reads as
		// armed against a pale swatch where one white line would vanish.
		c.StrokeRounded(Inset(r, -c.hairline()*2), 5, ColorAccent)
	}
	c.queueTooltip(id, r, it, opts.Tooltip, "", "")
	return it.Clicked
}

// OpenColorPicker opens the HSV popover anchored to a rectangle.
func (c *Context) OpenColorPicker(id ID, anchor rl.Rectangle, col color.RGBA) {
	h, s, v := rgbToHSV(col)
	c.popover = popoverState{id: id, anchor: anchor, h: h, s: s, v: v, alpha: col.A}
}

// ColorPickerOpen reports whether a given picker is showing.
func (c *Context) ColorPickerOpen(id ID) bool { return c.popover.id == id }

// CloseColorPicker dismisses the popover.
func (c *Context) CloseColorPicker() {
	c.popover = popoverState{}
	c.popoverBox = rl.Rectangle{}
}

// ColorPickerResult reports the popover's outcome for a frame.
type ColorPickerResult struct {
	Color   color.RGBA
	Changed bool
	Closed  bool
}

// pickerSize is the popover's layout in logical pixels.
const (
	pickerWidth   = 200
	pickerSVSize  = 140
	pickerHueBar  = 16
	pickerPadding = 10
	pickerSwatch  = 18
)

// ColorPicker draws the open popover. It is deferred to the overlay layer so it
// is never clipped by the panel that owns the swatch.
func (c *Context) ColorPicker(id ID, presets []color.RGBA) ColorPickerResult {
	if c.popover.id != id {
		return ColorPickerResult{}
	}
	var out ColorPickerResult

	w := c.Px(pickerWidth)
	rows := float32(math.Ceil(float64(len(presets)) / 8))
	h := c.Px(pickerPadding*3+pickerSVSize+pickerHueBar) + rows*c.Px(pickerSwatch+4)
	box := Rect(c.popover.anchor.X, c.popover.anchor.Y+c.popover.anchor.Height+c.Px(4), w, h)

	// A popover must not run off the bottom or right of the window.
	if maxX := float32(rl.GetRenderWidth()) - c.Px(Spacing); box.X+box.Width > maxX {
		box.X = maxX - box.Width
	}
	if maxY := float32(rl.GetRenderHeight()) - c.Px(Spacing); box.Y+box.Height > maxY {
		box.Y = c.popover.anchor.Y - box.Height - c.Px(4)
	}

	c.popoverBox = box

	// Clicking outside closes it, which is the only way out besides Escape.
	outside := !rl.CheckCollisionPointRec(c.MousePos(), box) &&
		!rl.CheckCollisionPointRec(c.MousePos(), c.popover.anchor)
	if (c.In.Pressed[MouseLeft] && outside) || c.In.KeyPressed(rl.KeyEscape) {
		c.CloseColorPicker()
		return ColorPickerResult{Closed: true}
	}

	c.Defer(func() {
		c.Card(box)
		inner := Inset(box, c.Px(pickerPadding))

		// Saturation and value field.
		sv := Rect(inner.X, inner.Y, inner.Width, c.Px(pickerSVSize))
		c.drawSVField(sv, c.popover.h)
		if c.dragInside(id.Child("sv"), sv) {
			c.popover.s = clamp01(float64((float32(c.In.MouseX) - sv.X) / sv.Width))
			c.popover.v = 1 - clamp01(float64((float32(c.In.MouseY)-sv.Y)/sv.Height))
			out.Changed = true
		}
		cx := sv.X + float32(c.popover.s)*sv.Width
		cy := sv.Y + float32(1-c.popover.v)*sv.Height
		rl.DrawCircleLinesV(rl.Vector2{X: cx, Y: cy}, c.Px(5), ColorText)

		// Hue strip.
		hue := Rect(inner.X, sv.Y+sv.Height+c.Px(pickerPadding), inner.Width, c.Px(pickerHueBar))
		c.drawHueBar(hue)
		if c.dragInside(id.Child("hue"), hue) {
			c.popover.h = clamp01(float64((float32(c.In.MouseX)-hue.X)/hue.Width)) * 360
			out.Changed = true
		}
		hx := hue.X + float32(c.popover.h/360)*hue.Width
		rl.DrawRectangleLinesEx(Rect(hx-c.Px(2), hue.Y-c.Px(1), c.Px(4), hue.Height+c.Px(2)),
			c.hairline(), ColorText)

		// Preset swatches.
		y := hue.Y + hue.Height + c.Px(pickerPadding)
		size := c.Px(pickerSwatch)
		for i, p := range presets {
			col, row := i%8, i/8
			sr := Rect(inner.X+float32(col)*(size+c.Px(4)), y+float32(row)*(size+c.Px(4)), size, size)
			if c.ColorSwatch(id.Child("preset"+itoa(i)), sr, p, SwatchOpts{
				Tooltip: "#" + hexOf(p),
			}) {
				c.popover.h, c.popover.s, c.popover.v = rgbToHSV(p)
				out.Changed = true
			}
		}
	})

	out.Color = hsvToRGB(c.popover.h, c.popover.s, c.popover.v, c.popover.alpha)
	return out
}

// dragInside reports whether the pointer is pressing within a rectangle,
// including continuing a drag that started there.
func (c *Context) dragInside(id ID, r rl.Rectangle) bool {
	inside := rl.CheckCollisionPointRec(c.MousePos(), r)
	if inside && c.In.Pressed[MouseLeft] {
		c.active = id
		c.wantMouse = true
	}
	if c.active == id && c.In.Down[MouseLeft] {
		c.wantMouse = true
		return true
	}
	return false
}

// drawSVField paints the saturation/value square for a hue as a coarse grid of
// quads, which is plenty at this size and needs no shader.
func (c *Context) drawSVField(r rl.Rectangle, hue float64) {
	const steps = 24
	cw := r.Width / steps
	ch := r.Height / steps
	for i := 0; i < steps; i++ {
		for j := 0; j < steps; j++ {
			s := (float64(i) + 0.5) / steps
			v := 1 - (float64(j)+0.5)/steps
			cell := Rect(r.X+float32(i)*cw, r.Y+float32(j)*ch, cw+1, ch+1)
			FillRect(cell, hsvToRGB(hue, s, v, 255))
		}
	}
	c.StrokeRounded(r, 0, ColorStroke)
}

func (c *Context) drawHueBar(r rl.Rectangle) {
	const steps = 48
	cw := r.Width / steps
	for i := 0; i < steps; i++ {
		h := float64(i) / steps * 360
		FillRect(Rect(r.X+float32(i)*cw, r.Y, cw+1, r.Height), hsvToRGB(h, 1, 1, 255))
	}
	c.StrokeRounded(r, 0, ColorStroke)
}

// rgbToHSV converts a colour into hue (0-360), saturation and value (0-1).
func rgbToHSV(c color.RGBA) (h, s, v float64) {
	r := float64(c.R) / 255
	g := float64(c.G) / 255
	b := float64(c.B) / 255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	v = max
	d := max - min
	if max > 0 {
		s = d / max
	}
	if d == 0 {
		return 0, s, v
	}
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// hsvToRGB is the inverse of rgbToHSV.
func hsvToRGB(h, s, v float64, alpha uint8) color.RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s, v = clamp01(s), clamp01(v)
	cc := v * s
	x := cc * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - cc
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = cc, x, 0
	case h < 120:
		r, g, b = x, cc, 0
	case h < 180:
		r, g, b = 0, cc, x
	case h < 240:
		r, g, b = 0, x, cc
	case h < 300:
		r, g, b = x, 0, cc
	default:
		r, g, b = cc, 0, x
	}
	to8 := func(f float64) uint8 { return uint8(math.Round(clamp01(f+m) * 255)) }
	return color.RGBA{R: to8(r), G: to8(g), B: to8(b), A: alpha}
}

// hexOf spells a colour the way the palette files and the tooltips do. It is a
// copy of paint.Hex rather than a call to it, because the widget kit sits below
// everything and imports none of it (PLAN §4).
func hexOf(c color.RGBA) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 6)
	for i, v := range [3]uint8{c.R, c.G, c.B} {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0F]
	}
	return string(out)
}
