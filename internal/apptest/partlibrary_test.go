package apptest

import (
	"modeler/internal/io"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedPartsLibraryUI(t *testing.T) {
	out, dir := runScript(t, "parts_library")
	parts, err := io.ReadPartLibrary(filepath.Join(dir, "parts"))
	if err != nil || len(parts) != 1 {
		t.Fatalf("library was not saved: %v %v\n%s", parts, err, out)
	}
	if parts[0].Name != "Modular hull" || parts[0].Category != "Structures" {
		t.Fatalf("form lost text: %+v", parts[0])
	}
	if !strings.Contains(out, `name="Modular hull copy 4"`) {
		t.Fatalf("insert did not create a model copy: %s", out)
	}
	before, err := LoadPNG(filepath.Join(dir, "library_browser.png"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := LoadPNG(filepath.Join(dir, "library_rotated.png"))
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for y := 338; y < 516; y++ {
		for x := 532; x < 988; x++ {
			if before.At(x, y) != after.At(x, y) {
				changed++
			}
		}
	}
	if changed < 500 {
		t.Fatalf("preview did not rotate: only %d changed pixels", changed)
	}
	for y := 420; y < 620; y++ {
		for x := 1010; x < 1250; x++ {
			if before.At(x, y) != after.At(x, y) {
				t.Fatal("preview changed the model view outside the library")
			}
		}
	}
	checkGolden(t, "parts_library", dir)
}
