package ui

import (
	"image/color"
	"math"
	"testing"
)

func TestLightAppearanceContrastAndDarkRestore(t *testing.T) {
	ResetTheme()
	defer ResetTheme()
	dark := snapshotTheme()
	body := BodyColor(0)
	ApplyAppearance("light")
	luminance := func(c color.RGBA) float64 {
		channel := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= .04045 {
				return x / 12.92
			}
			return math.Pow((x+.055)/1.055, 2.4)
		}
		return .2126*channel(c.R) + .7152*channel(c.G) + .0722*channel(c.B)
	}
	contrast := func(a, b color.RGBA) float64 {
		x, y := luminance(a), luminance(b)
		return (math.Max(x, y) + .05) / (math.Min(x, y) + .05)
	}
	for _, surface := range []color.RGBA{ColorPanel, ColorCard, ColorBG} {
		for _, ink := range []color.RGBA{ColorText, ColorTextDim} {
			if ratio := contrast(surface, ink); ratio < 4.5 {
				t.Errorf("text contrast %.2f is too low", ratio)
			}
		}
	}
	for _, accent := range []color.RGBA{AccentModel, AccentSketch, AccentExtrude, AccentBoolean, AccentPaint, AccentMarker} {
		if ratio := contrast(accent, onAccent(accent)); ratio < 4.5 {
			t.Errorf("button contrast %.2f is too low", ratio)
		}
	}
	if ColorGridMinor.R > 128 || BodyColor(0) != body {
		t.Fatal("light grid or body colors are wrong")
	}
	ApplyAppearance("dark")
	if snapshotTheme() != dark {
		t.Fatal("switching back failed to restore all dark tokens")
	}
}
