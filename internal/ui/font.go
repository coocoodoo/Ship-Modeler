package ui

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
	"golang.org/x/image/font/gofont/goregular"
)

// Fonts holds the embedded UI typeface rasterised at the exact pixel sizes the
// current display scale needs (D-11, SPEC-UX §3). Loading at the scaled size
// rather than scaling a smaller atlas is what keeps text crisp at 1.25x-2x.
type Fonts struct {
	scale  float64
	Small  rl.Font // 11 px logical: hints and badges
	UI     rl.Font // 13 px logical: the default
	Header rl.Font // 15 px logical: section headers
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

// LoadFonts rasterises the embedded typeface for a display scale. Call
// ReloadFonts when the scale changes; the caller owns unloading.
func LoadFonts(scale float64) *Fonts {
	scale = Scale(scale)
	px := func(logical float64) int32 { return int32(logical*scale + 0.5) }
	load := func(logical float64) rl.Font {
		f := rl.LoadFontFromMemory(".ttf", goregular.TTF, px(logical), glyphRange)
		// Bilinear keeps scaled-up glyph edges smooth; the pixel-art crispness
		// rule applies to paint textures, not to UI text.
		rl.SetTextureFilter(f.Texture, rl.FilterBilinear)
		return f
	}
	return &Fonts{
		scale:  scale,
		Small:  load(FontSizeSmall),
		UI:     load(FontSizeUI),
		Header: load(FontSizeHeader),
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
