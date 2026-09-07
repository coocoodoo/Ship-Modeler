package ui

import (
	"image/color"
	"math"
)

type hoverTransition struct {
	value    float64
	velocity float64
	target   float64
	finite   bool
	frame    uint64
}

// Patina's exponential easing, applied inside our existing immediate UI.
func (c *Context) hoverAmount(id ID, hovered bool) float64 {
	target := 0.0
	if hovered {
		target = 1
	}
	return c.easeAmount(id, target, 65, false)
}

func blendColor(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: lerp(a.A, b.A)}
}

func onAccent(accent color.RGBA) color.RGBA {
	channel := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= .04045 {
			return x / 12.92
		}
		return math.Pow((x+.055)/1.055, 2.4)
	}
	l := .2126*channel(accent.R) + .7152*channel(accent.G) + .0722*channel(accent.B)
	if l > .179 {
		return rgb(0x08, 0x0A, 0x10)
	}
	return rgb(0xFF, 0xFF, 0xFF)
}
