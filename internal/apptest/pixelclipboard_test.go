package apptest

import (
	"image"
	"path/filepath"
	"testing"
)

func TestPixelClipboardPointerWorkflow(t *testing.T) {
	_, dir := runScript(t, "pixel_clipboard")
	checkGolden(t, "pixel_clipboard", dir)
	load := func(name string) image.Image {
		im, err := LoadPNG(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		return im
	}
	pasted, undone, redone := load("pixels_pasted"), load("pixels_undo"), load("pixels_redo")
	preview, quarter, restored, wrapped := load("pixels_preview"), load("pixels_preview90"), load("pixels_preview_restored"), load("pixels_preview_wrapped")
	rotated := 0
	changed := 0
	// Compare the model area; undo/redo intentionally have different toast text.
	for y := 100; y < 520; y++ {
		for x := 240; x < 1040; x++ {
			if preview.At(x, y) != restored.At(x, y) || preview.At(x, y) != wrapped.At(x, y) {
				t.Fatalf("rotation moved camera or failed to return at %d,%d", x, y)
			}
			if preview.At(x, y) != quarter.At(x, y) {
				rotated++
			}
			p, u, r := pasted.At(x, y), undone.At(x, y), redone.At(x, y)
			if p != u {
				changed++
			}
			if p != r {
				t.Fatalf("redo differs at %d,%d", x, y)
			}
		}
	}
	if changed < 100 {
		t.Fatalf("paste changed only %d visible pixels; selection/copy/paste controls failed", changed)
	}
	if rotated < 100 {
		t.Fatal("Ctrl+wheel did not rotate the preview")
	}
	for _, pair := range [][2]string{{"pixels_button180", "pixels_preview180"}, {"pixels_button270", "pixels_preview270"}, {"pixels_button0", "pixels_preview"}, {"pixels_mirror_hv", "pixels_preview180"}, {"pixels_mirror_restored", "pixels_preview"}} {
		button, wheel := load(pair[0]), load(pair[1])
		for y := 100; y < 520; y++ {
			for x := 240; x < 1040; x++ {
				if button.At(x, y) != wheel.At(x, y) {
					t.Fatalf("%s does not match wheel rotation at %d,%d", pair[0], x, y)
				}
			}
		}
	}
	for _, name := range []string{"pixels_mirror_h", "pixels_mirror_v"} {
		mirrored := load(name)
		different := 0
		for y := 100; y < 520; y++ {
			for x := 240; x < 1040; x++ {
				if mirrored.At(x, y) != preview.At(x, y) {
					different++
				}
			}
		}
		if different < 100 {
			t.Fatalf("%s did not change the preview", name)
		}
	}
}
