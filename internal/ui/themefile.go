package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"strings"
)

// The theme file (SPEC-UX §3.2, V-149): every colour token the chrome uses,
// as "#RRGGBB" or "#RRGGBBAA" strings, so a player can retune the whole
// program from a text editor. A key left out keeps its built-in value; a key
// that will not parse is an error naming it, and the theme is left exactly as
// it was — a half-applied palette is worse than the default one.
//
// This package cannot know where the file lives (it imports nothing above
// std and raylib), so the app hands it a path.

// ThemeFile is the on-disk shape of the theme.
type ThemeFile struct {
	BG      string `json:"bg,omitempty"`
	Panel   string `json:"panel,omitempty"`
	Card    string `json:"card,omitempty"`
	Stroke  string `json:"stroke,omitempty"`
	Text    string `json:"text,omitempty"`
	TextDim string `json:"textDim,omitempty"`

	Warn    string `json:"warn,omitempty"`
	Error   string `json:"error,omitempty"`
	Success string `json:"success,omitempty"`

	ViewportTop    string `json:"viewportTop,omitempty"`
	ViewportBottom string `json:"viewportBottom,omitempty"`

	Accents ThemeAccents `json:"accents"`
	Axis    ThemeAxis    `json:"axis"`
}

// ThemeAccents are the per-mode accents of SPEC-UX §3.1.
type ThemeAccents struct {
	Model   string `json:"model,omitempty"`
	Sketch  string `json:"sketch,omitempty"`
	Extrude string `json:"extrude,omitempty"`
	Boolean string `json:"boolean,omitempty"`
	Paint   string `json:"paint,omitempty"`
	Marker  string `json:"marker,omitempty"`
}

// ThemeAxis are the world axis colours: triad, gizmo arrows, plane tints.
type ThemeAxis struct {
	X string `json:"x,omitempty"`
	Y string `json:"y,omitempty"`
	Z string `json:"z,omitempty"`
}

// themeSnapshot is every mutable token, so the built-in theme can be restored.
type themeSnapshot struct {
	bg, panel, card, stroke, text, textDim     color.RGBA
	warn, errorC, success                      color.RGBA
	viewportTop, viewportBottom                color.RGBA
	model, sketch, extrude, boolean, paint     color.RGBA
	marker, axisX, axisY, axisZ                color.RGBA
	accent, accentSoft                         color.RGBA
	gridMinor, gridMajor, hover, bevel, shadow color.RGBA
}

func snapshotTheme() themeSnapshot {
	return themeSnapshot{
		bg: ColorBG, panel: ColorPanel, card: ColorCard, stroke: ColorStroke,
		text: ColorText, textDim: ColorTextDim,
		warn: ColorWarn, errorC: ColorError, success: ColorSuccess,
		viewportTop: ColorViewportTop, viewportBottom: ColorViewportBottom,
		model: AccentModel, sketch: AccentSketch, extrude: AccentExtrude,
		boolean: AccentBoolean, paint: AccentPaint, marker: AccentMarker,
		axisX: ColorAxisX, axisY: ColorAxisY, axisZ: ColorAxisZ,
		accent: ColorAccent, accentSoft: ColorAccentSoft,
		gridMinor: ColorGridMinor, gridMajor: ColorGridMajor, hover: ColorHover,
		bevel: ColorBevel, shadow: ColorShadow,
	}
}

func (s themeSnapshot) restore() {
	ColorBG, ColorPanel, ColorCard, ColorStroke = s.bg, s.panel, s.card, s.stroke
	ColorText, ColorTextDim = s.text, s.textDim
	ColorWarn, ColorError, ColorSuccess = s.warn, s.errorC, s.success
	ColorViewportTop, ColorViewportBottom = s.viewportTop, s.viewportBottom
	AccentModel, AccentSketch, AccentExtrude = s.model, s.sketch, s.extrude
	AccentBoolean, AccentPaint, AccentMarker = s.boolean, s.paint, s.marker
	ColorAxisX, ColorAxisY, ColorAxisZ = s.axisX, s.axisY, s.axisZ
	ColorAccent, ColorAccentSoft = s.accent, s.accentSoft
	ColorGridMinor, ColorGridMajor, ColorHover = s.gridMinor, s.gridMajor, s.hover
	ColorBevel, ColorShadow = s.bevel, s.shadow
}

// builtinTheme is the palette as compiled, taken before anything could touch
// the tokens.
var builtinTheme = snapshotTheme()

// ResetTheme puts every token back to the built-in palette.
func ResetTheme() { builtinTheme.restore() }

// DefaultThemeFile is the built-in palette written out, every key present, so
// a first theme.json shows what can be changed rather than being empty.
func DefaultThemeFile() ThemeFile {
	b := builtinTheme
	return ThemeFile{
		BG: themeHex(b.bg), Panel: themeHex(b.panel), Card: themeHex(b.card), Stroke: themeHex(b.stroke),
		Text: themeHex(b.text), TextDim: themeHex(b.textDim),
		Warn: themeHex(b.warn), Error: themeHex(b.errorC), Success: themeHex(b.success),
		ViewportTop: themeHex(b.viewportTop), ViewportBottom: themeHex(b.viewportBottom),
		Accents: ThemeAccents{
			Model: themeHex(b.model), Sketch: themeHex(b.sketch), Extrude: themeHex(b.extrude),
			Boolean: themeHex(b.boolean), Paint: themeHex(b.paint), Marker: themeHex(b.marker),
		},
		Axis: ThemeAxis{X: themeHex(b.axisX), Y: themeHex(b.axisY), Z: themeHex(b.axisZ)},
	}
}

// ApplyThemeFile sets every token the file names. All keys are parsed before
// any is applied, so a bad value leaves the theme untouched.
func ApplyThemeFile(t ThemeFile) error {
	type slot struct {
		key  string
		val  string
		dest *color.RGBA
	}
	slots := []slot{
		{"bg", t.BG, &ColorBG}, {"panel", t.Panel, &ColorPanel}, {"card", t.Card, &ColorCard},
		{"stroke", t.Stroke, &ColorStroke}, {"text", t.Text, &ColorText}, {"textDim", t.TextDim, &ColorTextDim},
		{"warn", t.Warn, &ColorWarn}, {"error", t.Error, &ColorError}, {"success", t.Success, &ColorSuccess},
		{"viewportTop", t.ViewportTop, &ColorViewportTop}, {"viewportBottom", t.ViewportBottom, &ColorViewportBottom},
		{"accents.model", t.Accents.Model, &AccentModel}, {"accents.sketch", t.Accents.Sketch, &AccentSketch},
		{"accents.extrude", t.Accents.Extrude, &AccentExtrude}, {"accents.boolean", t.Accents.Boolean, &AccentBoolean},
		{"accents.paint", t.Accents.Paint, &AccentPaint}, {"accents.marker", t.Accents.Marker, &AccentMarker},
		{"axis.x", t.Axis.X, &ColorAxisX}, {"axis.y", t.Axis.Y, &ColorAxisY}, {"axis.z", t.Axis.Z, &ColorAxisZ},
	}
	parsed := make([]color.RGBA, len(slots))
	for i, s := range slots {
		if strings.TrimSpace(s.val) == "" {
			continue
		}
		c, err := ParseHexColor(s.val)
		if err != nil {
			return fmt.Errorf("theme: %s: %w", s.key, err)
		}
		parsed[i] = c
	}
	for i, s := range slots {
		if strings.TrimSpace(s.val) == "" {
			continue
		}
		*s.dest = parsed[i]
	}
	// The live accent belongs to whatever mode is running; the app re-asserts
	// it every frame, so the idle colour is the right thing to leave here.
	SetAccent(AccentModel)
	return nil
}

// LoadTheme reads and applies a theme file. A missing file is not an error:
// it reports applied=false and the built-in palette stands.
func LoadTheme(path string) (applied bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	var t ThemeFile
	if err := json.Unmarshal(data, &t); err != nil {
		return false, fmt.Errorf("theme: %w", err)
	}
	t = upgradeLegacyDefault(t)
	if err := ApplyThemeFile(t); err != nil {
		return false, err
	}
	return true, nil
}

// WriteTheme saves a theme file, indented so it is pleasant to edit.
func WriteTheme(path string, t ThemeFile) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ParseHexColor reads "#RRGGBB" or "#RRGGBBAA" (the hash optional, case free).
func ParseHexColor(s string) (color.RGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) != 6 && len(h) != 8 {
		return color.RGBA{}, fmt.Errorf("%q is not #RRGGBB or #RRGGBBAA", s)
	}
	var v [4]uint8
	v[3] = 0xFF
	for i := 0; i < len(h)/2; i++ {
		hi, ok1 := hexNibble(h[2*i])
		lo, ok2 := hexNibble(h[2*i+1])
		if !ok1 || !ok2 {
			return color.RGBA{}, fmt.Errorf("%q is not #RRGGBB or #RRGGBBAA", s)
		}
		v[i] = hi<<4 | lo
	}
	return color.RGBA{R: v[0], G: v[1], B: v[2], A: v[3]}, nil
}

func hexNibble(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

// hexOf writes a colour the way ParseHexColor reads it, dropping the alpha
// when it is opaque.
func themeHex(c color.RGBA) string {
	if c.A == 0xFF {
		return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", c.R, c.G, c.B, c.A)
}
