package ui

import (
	"image/color"
	"os"
	"path/filepath"

	rl "github.com/gen2brain/raylib-go/raylib"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
)

// Fonts holds the UI typeface rasterised at the exact pixel sizes the
// current display scale needs (D-11, SPEC-UX §3). Loading at the scaled size
// rather than scaling a smaller atlas is what keeps text crisp at 1.25x-2x.
type Fonts struct {
	scale  float64
	Small  rl.Font // hints and badges
	UI     rl.Font // default labels and controls
	Header rl.Font // semibold section and card headers
}

// glyphRange is the codepoint set baked into each atlas: ASCII plus the few
// symbols the UI draws as text rather than as vector icons.
var glyphRange = buildGlyphRange()

func buildGlyphRange() []rune {
	var rs []rune
	for c := rune(32); c < 127; c++ {
		rs = append(rs, c)
	}
	// The punctuation the UI copy actually uses, all verified present in the
	// Go Regular typeface. The symbols it does NOT carry — ticks, crosses,
	// chevrons, the undo arrows — are stroke icons instead (D-11), so nothing
	// here can fall back to a missing-glyph box.
	rs = append(rs, '·', '×', '°', '—', '–', '‹', '›', '“', '”', '‘', '’', '…', '±', '→', '↔')
	return rs
}

// CanRender reports whether a rune is in the atlas. Text containing anything
// else draws as a missing-glyph box, so every string the UI can show has to
// pass this — see the apptest that checks the toasts and hints of every script.
func CanRender(r rune) bool {
	for _, c := range glyphRange {
		if c == r {
			return true
		}
	}
	return false
}

// FirstUnrenderable returns the first rune of s the atlas cannot draw.
func FirstUnrenderable(s string) (rune, bool) {
	for _, r := range s {
		if !CanRender(r) {
			return r, true
		}
	}
	return 0, false
}

// LoadFonts rasterises the embedded typeface for a display scale. Call
// ReloadFonts when the scale changes; the caller owns unloading.
func LoadFonts(scale float64) *Fonts {
	scale = Scale(scale)
	px := func(logical float64) int32 { return int32(logical*scale + 0.5) }
	regular, medium := goregular.TTF, gomedium.TTF
	// Native Windows typography, with embedded fallbacks for portable builds.
	// System font files stay on the user's machine and are never redistributed.
	if windows := os.Getenv("WINDIR"); windows != "" {
		if data, err := os.ReadFile(filepath.Join(windows, "Fonts", "segoeui.ttf")); err == nil {
			regular = data
		}
		if data, err := os.ReadFile(filepath.Join(windows, "Fonts", "seguisb.ttf")); err == nil {
			medium = data
		}
	}
	load := func(logical float64, data []byte) rl.Font {
		f := rl.LoadFontFromMemory(".ttf", data, px(logical), glyphRange)
		// Bilinear keeps scaled-up glyph edges smooth; the pixel-art crispness
		// rule applies to paint textures, not to UI text.
		rl.SetTextureFilter(f.Texture, rl.FilterBilinear)
		return f
	}
	return &Fonts{
		scale:  scale,
		Small:  load(FontSizeSmall, regular),
		UI:     load(FontSizeUI, regular),
		Header: load(FontSizeHeader, medium),
	}
}

// Scale reports the display scale these fonts were rasterised for.
func (f *Fonts) Scale() float64 { return f.scale }

// Unload frees the GPU atlases.
func (f *Fonts) Unload() {
	if f == nil {
		return
	}
	rl.UnloadFont(f.Small)
	rl.UnloadFont(f.UI)
	rl.UnloadFont(f.Header)
}

// PixelSize returns the device pixel size for a logical font size.
func (f *Fonts) PixelSize(logical float64) float32 {
	return float32(logical * f.scale)
}

// Draw renders text with its top-left corner at (x, y) in device pixels.
func (f *Fonts) Draw(font rl.Font, text string, x, y float32, logicalSize float64, tint color.RGBA) {
	rl.DrawTextEx(font, text, rl.Vector2{X: x, Y: y}, f.PixelSize(logicalSize), 0, tint)
}

// Measure returns the rendered width and height of text in device pixels.
func (f *Fonts) Measure(font rl.Font, text string, logicalSize float64) (w, h float32) {
	v := rl.MeasureTextEx(font, text, f.PixelSize(logicalSize), 0)
	return v.X, v.Y
}

// DrawCentered renders text centred on (cx, cy) in device pixels.
func (f *Fonts) DrawCentered(font rl.Font, text string, cx, cy float32, logicalSize float64, tint color.RGBA) {
	w, h := f.Measure(font, text, logicalSize)
	f.Draw(font, text, cx-w/2, cy-h/2, logicalSize, tint)
}

// LineHeight is the baseline-to-baseline distance for a logical font size.
func (f *Fonts) LineHeight(logicalSize float64) float32 {
	return float32(logicalSize * LineHeightRatio * f.scale)
}
