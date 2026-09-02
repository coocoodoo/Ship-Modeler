// Package ui is the in-house immediate-mode widget kit (D-02) and the single
// home of the visual theme. It is standalone: nothing below it in the
// dependency order may import it, and it imports no document types.
package ui

import "image/color"

// Theme tokens — SPEC-UX §3. Nothing outside this file may invent a color.
//
// The 2026-08-27 pass deepened the whole ladder one step and widened the gaps
// between its rungs. The old palette kept background, panel and card within a
// few points of value of each other, which with no shadows anywhere read as one
// flat sheet; depth needs both the contrast here and the elevation cues the
// draw layer now adds (V-88).
var (
	ColorBG      = rgb(0x11, 0x13, 0x19) // window background
	ColorPanel   = rgb(0x1A, 0x1D, 0x25) // toolbar, tree, hint bar
	ColorCard    = rgb(0x25, 0x2A, 0x35) // floating cards, fields
	ColorStroke  = rgb(0x3A, 0x41, 0x50) // hairlines, borders
	ColorText    = rgb(0xE8, 0xEA, 0xF0) // primary text
	ColorTextDim = rgb(0x9C, 0xA6, 0xB6) // secondary text, hints

	ColorAccent     = rgb(0x53, 0xA4, 0xFF)        // selection, active tool
	ColorAccentSoft = rgba(0x53, 0xA4, 0xFF, 0x33) // region fills, soft highlights
	ColorWarn       = rgb(0xFF, 0xA6, 0x3C)        // clamped draft, bent face
	ColorError      = rgb(0xFF, 0x5D, 0x5D)        // open ends, failed ops
	ColorSuccess    = rgb(0x3D, 0xD6, 0x8C)        // confirm, valid states

	// The viewport background is a vertical gradient between these two: light
	// falls from above, so the top is the bright end. The old ramp ran the
	// other way and was narrow enough to pass for flat.
	ColorViewportTop    = rgb(0x2A, 0x30, 0x3C)
	ColorViewportBottom = rgb(0x12, 0x14, 0x1A)

	ColorGridMinor = rgba(0xFF, 0xFF, 0xFF, 0x0F)
	ColorGridMajor = rgba(0xFF, 0xFF, 0xFF, 0x24)

	// ColorHover is overlaid on any hoverable element.
	ColorHover = rgba(0xFF, 0xFF, 0xFF, 0x14)

	// ColorBevel is the one-pixel light along a raised surface's top edge, and
	// ColorShadow the ink its drop shadow is layered from. Together they are
	// what makes a card sit above the viewport instead of being pasted on it.
	ColorBevel  = rgba(0xFF, 0xFF, 0xFF, 0x16)
	ColorShadow = rgba(0x00, 0x00, 0x08, 0x12)
)

// Mode accents (SPEC-UX §3.1, V-149). ColorAccent above is not a fixed colour:
// it is whichever of these the app's current mode owns, swapped in by
// SetAccent at the top of every frame. That one indirection is what makes the
// selection glow, the active-tool underline, the chips, the sliders and the
// sketch overlay all change colour with the mode without any of them knowing.
//
// The hues are chosen far apart and away from the three semantic colours
// (warn amber, error red, success green) so a warning still reads as one
// inside any mode — TestModeAccentsAreDistinctAndClearOfTheSemanticColours
// holds them to that, which is how the first marker orange was caught
// leaning on the warn amber. The viewport itself is never tinted — a pixel
// artist needs it colour-true — so the accents live in the chrome only.
var (
	AccentModel   = rgb(0x53, 0xA4, 0xFF) // blue: idle, selection, move, files
	AccentSketch  = rgb(0xFF, 0xD9, 0x4A) // gold: pencil on paper
	AccentExtrude = rgb(0x2E, 0xD0, 0xCC) // teal: things growing
	AccentBoolean = rgb(0xB4, 0x8C, 0xFF) // violet: things combining
	AccentPaint   = rgb(0xFF, 0x6F, 0xB5) // pink: paint
	AccentMarker  = rgb(0xA6, 0xF0, 0x4E) // lime: highlighter, for the dots you place
)

// SetAccent makes c the live accent: ColorAccent and its soft wash both follow.
func SetAccent(c color.RGBA) {
	ColorAccent = WithAlpha(c, 0xFF)
	ColorAccentSoft = WithAlpha(c, 0x33)
}

// Soft is the wash version of an accent — the alpha ColorAccentSoft carries.
func Soft(c color.RGBA) color.RGBA {
	return WithAlpha(c, 0x33)
}

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
