package ui

import (
	_ "embed"
	"encoding/json"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"strings"
)

// Palette surface colors from Patina core/src/palettes.rs (MIT). CAD mode and
// semantic accents retain Modeler's contrast-tuned light/dark variants.
//
//go:embed patina/palettes.json
var paletteData []byte

type Palette struct {
	Name                                                  string
	Dark                                                  bool
	Background, Surface, Variant, Text, Secondary, Accent string
}

var appearancePalettes = func() []Palette {
	var result []Palette
	if err := json.Unmarshal(paletteData, &result); err != nil {
		panic(err)
	}
	return result
}()

func Palettes(dark bool) []Palette {
	var out []Palette
	for _, p := range appearancePalettes {
		if p.Dark == dark {
			out = append(out, p)
		}
	}
	return out
}

func FindPalette(name string) (Palette, bool) {
	for _, p := range appearancePalettes {
		if strings.EqualFold(strings.TrimSpace(name), p.Name) {
			return p, true
		}
	}
	return Palette{}, false
}

func paletteColor(hex string) color.RGBA { c, _ := ParseHexColor(hex); return c }

func ApplyPalette(name string) bool {
	p, ok := FindPalette(name)
	if !ok {
		return false
	}
	mode := "light"
	if p.Dark {
		mode = "dark"
	}
	ApplyAppearance(mode)
	ColorBG, ColorPanel, ColorCard = paletteColor(p.Background), paletteColor(p.Surface), paletteColor(p.Variant)
	ColorText, ColorTextDim = paletteColor(p.Text), paletteColor(p.Secondary)
	ColorStroke = blendColor(ColorPanel, ColorText, .20)
	AccentModel = paletteColor(p.Accent)
	// Keep the 3D stage neutral for paint work; only the chrome uses palette hues.
	SetAccent(AccentModel)
	return true
}

// PaletteTile previews the actual surfaces before a palette is selected.
func (c *Context) PaletteTile(id ID, r rl.Rectangle, p Palette, selected bool) bool {
	it := c.interact(id, r, false)
	c.describeControl(id, "palette", p.Name)
	c.FillRounded(r, CornerRadius, paletteColor(p.Surface))
	border := blendColor(paletteColor(p.Surface), paletteColor(p.Text), .22)
	if selected {
		border = ColorAccent
	}
	c.StrokeRounded(r, CornerRadius, border)
	swatches := []string{p.Background, p.Variant, p.Accent}
	for i, hex := range swatches {
		c.FillRounded(Rect(r.X+c.Px(9+float64(i)*14), r.Y+c.Px(8), c.Px(10), c.Px(10)), 3, paletteColor(hex))
	}
	if selected {
		DrawCheckIcon(float64(r.X+r.Width-c.Px(13)), float64(r.Y+c.Px(13)), 12*c.Scale, paletteColor(p.Text))
	}
	c.Text(Rect(r.X+c.Px(9), r.Y+c.Px(24), r.Width-c.Px(16), c.Px(18)), p.Name, FontSizeSmall, paletteColor(p.Text))
	if it.Hovered {
		c.StrokeRounded(Inset(r, c.Px(1)), CornerRadius-1, Fade(paletteColor(p.Accent), .65))
	}
	return it.Clicked
}
