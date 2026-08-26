package paint

import (
	"image/color"
	"strings"
	"testing"
)

func TestDefaultPaletteIsThirtyTwoOpaqueColours(t *testing.T) {
	p := DefaultPalette()
	if len(p) != PaletteSize {
		t.Fatalf("palette has %d colours, want %d", len(p), PaletteSize)
	}
	seen := map[color.RGBA]bool{}
	for i, c := range p {
		if c.A != 255 {
			t.Errorf("colour %d is not opaque: %v", i, c)
		}
		if seen[c] {
			t.Errorf("colour %d (%v) is a duplicate", i, c)
		}
		seen[c] = true
	}
}

func TestParseHexReadsLospecFiles(t *testing.T) {
	in := "# a comment\n1A1C2C\r\n#5D275D\n\nef7d57\n"
	got, err := ParseHex(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []color.RGBA{
		{R: 0x1A, G: 0x1C, B: 0x2C, A: 255},
		{R: 0x5D, G: 0x27, B: 0x5D, A: 255},
		{R: 0xEF, G: 0x7D, B: 0x57, A: 255},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d colours, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("colour %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseHexRejectsRubbish(t *testing.T) {
	for _, in := range []string{"", "not a colour\n", "12345\n", "GGGGGG\n"} {
		if _, err := ParseHex(strings.NewReader(in)); err == nil {
			t.Errorf("input %q parsed without complaint", in)
		}
	}
}

func TestRecentsKeepTheLastEightMostRecentFirst(t *testing.T) {
	var r Recents
	for i := 0; i < 10; i++ {
		r.Add(color.RGBA{R: uint8(i), A: 255})
	}
	list := r.List()
	if len(list) != RecentsSize {
		t.Fatalf("recents holds %d, want %d", len(list), RecentsSize)
	}
	if list[0].R != 9 {
		t.Errorf("most recent is %v, want R=9", list[0])
	}
	// Re-picking a colour promotes it rather than duplicating it.
	r.Add(color.RGBA{R: 5, A: 255})
	list = r.List()
	if list[0].R != 5 {
		t.Errorf("re-picked colour did not move to the front: %v", list[0])
	}
	seen := map[uint8]int{}
	for _, c := range list {
		seen[c.R]++
		if seen[c.R] > 1 {
			t.Errorf("colour %d appears twice in recents", c.R)
		}
	}
}
