package app

import (
	"testing"

	"modeler/internal/ui"
)

// Palette names come from thousands of files by thousands of authors, and the
// font atlas is a curated codepoint list — anything outside it draws as a box
// (D-11). Every name has to survive that before it can reach a row or a toast.

func TestEveryLibraryNameIsRenderable(t *testing.T) {
	a := bareApp()
	lib := a.PaletteLibrary()
	if len(lib) < 1000 {
		t.Fatalf("the library only offered %d palettes", len(lib))
	}
	for _, p := range lib {
		if r, bad := ui.FirstUnrenderable(p.Name); bad {
			t.Errorf("palette %q carries %q, which the atlas cannot draw", p.Name, r)
			break
		}
		if p.Name == "" {
			t.Error("a name was emptied out entirely")
			break
		}
	}
}

func TestRenderableNameFoldsAndDrops(t *testing.T) {
	cases := map[string]string{
		"Gameboy Pocket": "Gameboy Pocket",
		"‚quoted‘":       ",quoted‘", // the low-9 folds to a comma, ‘ is in the atlas
		"grapes 🍇":       "grapes",
		"🍇":              "Untitled palette",
		"  spaced   out ": "spaced out",
	}
	for in, want := range cases {
		if got := renderableName(in); got != want {
			t.Errorf("renderableName(%q) = %q, want %q", in, got, want)
		}
	}
	// Whatever comes out, the atlas can always draw it.
	for _, in := range []string{"‚„‛‟ʼ´′″‐‑−", "🍇🎨", "Ünïcödé", "ok"} {
		if r, bad := ui.FirstUnrenderable(renderableName(in)); bad {
			t.Errorf("renderableName(%q) still carries %q", in, r)
		}
	}
}
