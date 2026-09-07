package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// The colour system (SPEC-UX §3.1-3.2): the accent is whichever mode owns it,
// and the theme file can retune every token or be refused whole.

func TestSetAccentMovesBothTokens(t *testing.T) {
	defer ResetTheme()
	SetAccent(color.RGBA{R: 1, G: 2, B: 3, A: 9})
	if ColorAccent != (color.RGBA{R: 1, G: 2, B: 3, A: 0xFF}) {
		t.Errorf("ColorAccent = %+v, want the colour, opaque", ColorAccent)
	}
	if ColorAccentSoft != (color.RGBA{R: 1, G: 2, B: 3, A: 0x33}) {
		t.Errorf("ColorAccentSoft = %+v, want the same colour at the soft alpha", ColorAccentSoft)
	}
}

func TestModeAccentsAreDistinctAndClearOfTheSemanticColours(t *testing.T) {
	accents := map[string]color.RGBA{
		"model": AccentModel, "sketch": AccentSketch, "extrude": AccentExtrude,
		"boolean": AccentBoolean, "paint": AccentPaint, "marker": AccentMarker,
	}
	semantic := map[string]color.RGBA{"warn": ColorWarn, "error": ColorError, "success": ColorSuccess}
	dist := func(a, b color.RGBA) int {
		d := func(x, y uint8) int {
			if x > y {
				return int(x - y)
			}
			return int(y - x)
		}
		return d(a.R, b.R) + d(a.G, b.G) + d(a.B, b.B)
	}
	for na, a := range accents {
		for nb, b := range accents {
			if na < nb && dist(a, b) < 90 {
				t.Errorf("accents %s and %s are too close to tell apart (%d)", na, nb, dist(a, b))
			}
		}
		for ns, s := range semantic {
			if dist(a, s) < 60 {
				t.Errorf("accent %s could be mistaken for %s (%d)", na, ns, dist(a, s))
			}
		}
	}
}

func TestParseHexColor(t *testing.T) {
	cases := map[string]color.RGBA{
		"#53A4FF":    {R: 0x53, G: 0xA4, B: 0xFF, A: 0xFF},
		"53a4ff":     {R: 0x53, G: 0xA4, B: 0xFF, A: 0xFF},
		" #FF000080": {R: 0xFF, G: 0, B: 0, A: 0x80},
	}
	for in, want := range cases {
		got, err := ParseHexColor(in)
		if err != nil || got != want {
			t.Errorf("ParseHexColor(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "#12345", "#GGGGGG", "blue", "#1234567"} {
		if _, err := ParseHexColor(bad); err == nil {
			t.Errorf("ParseHexColor(%q) accepted nonsense", bad)
		}
	}
}

func TestThemeFileRoundTripsAndKeepsDefaultsForMissingKeys(t *testing.T) {
	defer ResetTheme()
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")

	// No file: nothing applied, nothing wrong.
	if applied, err := LoadTheme(path); applied || err != nil {
		t.Fatalf("missing file: applied=%v err=%v", applied, err)
	}

	// The built-in palette written out and read back is the built-in palette.
	if err := WriteTheme(path, DefaultThemeFile()); err != nil {
		t.Fatal(err)
	}
	before := ColorBG
	if applied, err := LoadTheme(path); !applied || err != nil {
		t.Fatalf("round trip: applied=%v err=%v", applied, err)
	}
	if ColorBG != before || AccentSketch != builtinTheme.sketch {
		t.Error("the default file changed the palette")
	}

	// A file naming one key changes that key and no other.
	if err := os.WriteFile(path, []byte(`{"accents":{"paint":"#123456"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTheme(path); err != nil {
		t.Fatal(err)
	}
	if AccentPaint != (color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF}) {
		t.Errorf("paint accent = %+v, want the file's", AccentPaint)
	}
	if ColorBG != builtinTheme.bg || AccentSketch != builtinTheme.sketch {
		t.Error("an unnamed key moved")
	}
	// ...and the live accent falls back to the idle one for the app to reassert.
	if ColorAccent != AccentModel {
		t.Errorf("live accent = %+v after apply, want the model accent", ColorAccent)
	}
}

func TestThemeFileWithABadValueChangesNothing(t *testing.T) {
	defer ResetTheme()
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"bg":"#000000","accents":{"sketch":"gold"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := LoadTheme(path)
	if applied || err == nil {
		t.Fatalf("a bad value was accepted: applied=%v err=%v", applied, err)
	}
	if ColorBG != builtinTheme.bg {
		t.Error("the good key before the bad one was applied — the theme must apply whole or not at all")
	}
	if err.Error() == "" || !contains(err.Error(), "accents.sketch") {
		t.Errorf("error %q does not name the bad key", err)
	}
}

func TestResetThemeRestoresEverything(t *testing.T) {
	ColorBG = color.RGBA{R: 1}
	AccentPaint = color.RGBA{G: 1}
	ColorAxisZ = color.RGBA{B: 1}
	ResetTheme()
	if ColorBG != builtinTheme.bg || AccentPaint != builtinTheme.paint || ColorAxisZ != builtinTheme.axisZ {
		t.Error("ResetTheme left a token changed")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
