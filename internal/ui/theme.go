// Package ui is the in-house immediate-mode widget kit (D-02) and the single
// home of the visual theme. It is standalone: nothing below it in the
// dependency order may import it, and it imports no document types.
package ui

import "image/color"

// Theme tokens — SPEC-UX §3. Nothing outside this file may invent a color.
var (
	ColorBG      = rgb(0x16, 0x18, 0x1D) // window background
	ColorPanel   = rgb(0x1E, 0x21, 0x28) // toolbar, tree, hint bar
	ColorCard    = rgb(0x26, 0x2A, 0x33) // floating cards, fields
	ColorStroke  = rgb(0x34, 0x39, 0x45) // hairlines, borders
	ColorText    = rgb(0xE8, 0xEA, 0xF0) // primary text
	ColorTextDim = rgb(0x9A, 0xA3, 0xB2) // secondary text, hints

	ColorAccent     = rgb(0x4C, 0x9A, 0xFF)        // selection, active tool
	ColorAccentSoft = rgba(0x4C, 0x9A, 0xFF, 0x33) // region fills, soft highlights
	ColorWarn       = rgb(0xFF, 0xB4, 0x54)        // clamped draft, bent face
	ColorError      = rgb(0xFF, 0x5D, 0x5D)        // open ends, failed ops
	ColorSuccess    = rgb(0x3D, 0xD6, 0x8C)        // confirm, valid states

	// The viewport background is a vertical gradient between these two.
	ColorViewportTop    = rgb(0x1A, 0x1D, 0x23)
	ColorViewportBottom = rgb(0x22, 0x26, 0x2E)

	ColorGridMinor = rgba(0xFF, 0xFF, 0xFF, 0x0F)
	ColorGridMajor = rgba(0xFF, 0xFF, 0xFF, 0x24)

	// ColorHover is overlaid on any hoverable element.
	ColorHover = rgba(0xFF, 0xFF, 0xFF, 0x14)
)

// Axis colors are shared by the triad, the gizmo arrows and the plane tints.
var (
	ColorAxisX = rgb(0xE5, 0x48, 0x4D)
	ColorAxisY = rgb(0x46, 0xA7, 0x58)
	ColorAxisZ = rgb(0x3E, 0x63, 0xDD)
)

// AxisColor returns the color of world axis i (0=X, 1=Y, 2=Z).
func AxisColor(i int) color.RGBA {
	switch i {
	case 0:
		return ColorAxisX
	case 1:
		return ColorAxisY
	default:
		return ColorAxisZ
	}
}

// BodyColors is the auto-assigned body palette (SPEC-UX §3): eight desaturated
// tones chosen so hand-painted pixels read clearly on top of them.
var BodyColors = []color.RGBA{
	rgb(0x8E, 0xA3, 0xB0),
	rgb(0xB0, 0x8E, 0x8E),
	rgb(0x8E, 0xB0, 0x9B),
	rgb(0xA3, 0x8E, 0xB0),
	rgb(0xB0, 0xA9, 0x8E),
	rgb(0x8E, 0x9B, 0xB0),
	rgb(0xB0, 0x8E, 0xA6),
	rgb(0x96, 0xB0, 0x8E),
}

// BodyColor returns the auto color for the n-th body created.
func BodyColor(n int) color.RGBA {
	return BodyColors[((n%len(BodyColors))+len(BodyColors))%len(BodyColors)]
}

// Layout constants — SPEC-UX §2, in unscaled pixels. Multiply by the UI scale.
const (
	ToolbarHeight = 40
	TreeWidth     = 240
	HintBarHeight = 26
	MinWindowW    = 1280
	MinWindowH    = 720
	Spacing       = 8
	CornerRadius  = 6
	CardRadius    = 8
	IconSize      = 18
	IconStroke    = 1.5

	// View cube and axis triad, SPEC-UX §6.
	ViewCubeSize   = 84
	ViewCubeMargin = 16
	TriadSize      = 72
	TriadMargin    = 16
)

// Font sizes — SPEC-UX §3.
const (
	FontSizeUI      = 13
	FontSizeHeader  = 15
	FontSizeSmall   = 11
	LineHeightRatio = 1.4
)

// EdgeLineTint is the multiplier applied to a body's color for its edge overlay
// (SPEC-UX §3: body color x 0.35 at 85% alpha).
const (
	EdgeLineTint  = 0.35
	EdgeLineAlpha = 0.85
)

// Fade returns c with its alpha scaled by a in [0,1].
func Fade(c color.RGBA, a float64) color.RGBA {
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: uint8(float64(c.A)*a + 0.5)}
}

// Shade multiplies the RGB channels of c by f, keeping alpha.
func Shade(c color.RGBA, f float64) color.RGBA {
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v + 0.5)
	}
	return color.RGBA{
		R: clamp(float64(c.R) * f),
		G: clamp(float64(c.G) * f),
		B: clamp(float64(c.B) * f),
		A: c.A,
	}
}

// EdgeColor is the overlay color for a body of the given color.
func EdgeColor(body color.RGBA) color.RGBA {
	return Fade(Shade(body, EdgeLineTint), EdgeLineAlpha)
}

// WithAlpha returns c with an explicit alpha byte.
func WithAlpha(c color.RGBA, a uint8) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func rgb(r, g, b uint8) color.RGBA     { return color.RGBA{R: r, G: g, B: b, A: 255} }
func rgba(r, g, b, a uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: a} }

// Scale rounds an OS display scale to the supported UI steps (SPEC-UX §3), so
// fonts are always loaded at an exact pixel size rather than smoothed.
func Scale(osScale float64) float64 {
	switch {
	case osScale >= 1.75:
		return 2.0
	case osScale >= 1.375:
		return 1.5
	case osScale >= 1.125:
		return 1.25
	default:
		return 1.0
	}
}
