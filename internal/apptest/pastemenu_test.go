package apptest

import (
	"image"
	"path/filepath"
	"testing"
)

func TestPasteCornerMenuWorkflow(t *testing.T) {
	_, dir := runScript(t, "paste_menu")
	checkGolden(t, "paste_menu", dir)
	load := func(name string) image.Image {
		p, err := LoadPNG(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	difference := func(a, b string) int {
		left, right := load(a), load(b)
		n := 0
		for y := 100; y < 520; y++ {
			for x := 240; x < 1040; x++ {
				if left.At(x, y) != right.At(x, y) {
					n++
				}
			}
		}
		return n
	}
	for _, pair := range [][2]string{{"pixels_preview", "menu_cancel"}, {"pixels_preview", "corner_bottom_left"}, {"menu_rotated", "menu_cancel_rotated"}, {"menu_rotated", "menu_outside_dismiss"}} {
		if n := difference(pair[0], pair[1]); n != 0 {
			t.Fatalf("%s changed %d model/preview pixels", pair[1], n)
		}
	}
	for _, name := range []string{"corner_top_right", "corner_top_left", "corner_bottom_right", "menu_rotated"} {
		if difference("pixels_preview", name) < 100 {
			t.Fatalf("%s did not change the preview", name)
		}
	}
}
