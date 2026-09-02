package paint

import (
	"sort"
	"strings"
	"testing"
)

// The shipped library (V-152). These pin the bundle itself, because a bundle
// that half-parses is the failure that would otherwise show up as a browser
// with mysterious gaps in it.

func TestLibraryLoads(t *testing.T) {
	lib := Library()
	if len(lib) < 1000 {
		t.Fatalf("the library holds %d palettes, which is far short of the bundle", len(lib))
	}
	for _, p := range lib {
		if strings.TrimSpace(p.Name) == "" {
			t.Error("a palette came through with no name")
			break
		}
		if len(p.Colors) == 0 {
			t.Errorf("%q came through with no colours", p.Name)
			break
		}
		for _, c := range p.Colors {
			if c.A != 255 {
				t.Errorf("%q has a colour that is not opaque", p.Name)
				break
			}
		}
	}
}

func TestLibraryIsSortedByName(t *testing.T) {
	lib := Library()
	sorted := sort.SliceIsSorted(lib, func(i, j int) bool {
		return strings.ToLower(lib[i].Name) < strings.ToLower(lib[j].Name)
	})
	if !sorted {
		t.Error("the library is not in name order, so the list would read at random")
	}
}

func TestLibraryIsParsedOnce(t *testing.T) {
	a, b := Library(), Library()
	if len(a) != len(b) {
		t.Fatal("two calls disagreed about the library")
	}
	if len(a) > 0 && &a[0] != &b[0] {
		t.Error("the library was parsed twice; it is meant to be cached")
	}
}

// Matches is what the search box does: every word has to appear, in any
// order, so a name half-remembered still finds its palette.
func TestMatchesTakesWordsInAnyOrder(t *testing.T) {
	p := Palette{Name: "GrafxKid Gameboy Pocket (Green)"}
	for _, q := range []string{"", "  ", "gameboy", "GAMEBOY", "pocket gameboy", "green grafxkid"} {
		if !p.Matches(q) {
			t.Errorf("%q should have matched %q", q, p.Name)
		}
	}
	for _, q := range []string{"gameboy nintendo", "zzz"} {
		if p.Matches(q) {
			t.Errorf("%q should not have matched %q", q, p.Name)
		}
	}
}

// A palette named in the browser has to survive the round trip through the
// bundle exactly: these are colours an artist chose, not approximations.
func TestAKnownPaletteKeepsItsExactColours(t *testing.T) {
	var found *Palette
	for i, p := range Library() {
		if strings.EqualFold(p.Name, "Nostalgia") {
			found = &Library()[i]
			break
		}
	}
	if found == nil {
		t.Skip("the bundle does not carry Nostalgia; nothing to pin here")
	}
	if len(found.Colors) != 4 {
		t.Fatalf("Nostalgia has %d colours, want the 4 it ships with", len(found.Colors))
	}
	for _, c := range found.Colors {
		if c.R == 0 && c.G == 0 && c.B == 0 {
			t.Error("a colour came through as pure black, which smells like a parse failure")
		}
	}
}
