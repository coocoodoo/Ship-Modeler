package apptest

import (
	"path/filepath"
	"testing"
)

func TestChamferEdgeWorkflow(t *testing.T) {
	_, dir := runScript(t, "chamfer_edges")
	checkGolden(t, "chamfer_edges", dir)
	for _, pair := range [][2]string{{"gizmo075", "typed075"}, {"gizmo050", "preview"}} {
		left, err := LoadPNG(filepath.Join(dir, pair[0]+".png"))
		if err != nil {
			t.Fatal(err)
		}
		right, err := LoadPNG(filepath.Join(dir, pair[1]+".png"))
		if err != nil {
			t.Fatal(err)
		}
		for y := 150; y < 540; y++ {
			for x := 320; x < 980; x++ {
				if left.At(x, y) != right.At(x, y) {
					t.Fatalf("%s differs from %s at %d,%d", pair[0], pair[1], x, y)
				}
			}
		}
	}
	a, err := LoadPNG(filepath.Join(dir, "applied.png"))
	if err != nil {
		t.Fatal(err)
	}
	u, err := LoadPNG(filepath.Join(dir, "undo.png"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := LoadPNG(filepath.Join(dir, "redo.png"))
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for y := 150; y < 540; y++ {
		for x := 320; x < 980; x++ {
			if a.At(x, y) != u.At(x, y) {
				changed++
			}
			if a.At(x, y) != r.At(x, y) {
				t.Fatalf("redo differs at %d,%d", x, y)
			}
		}
	}
	if changed < 100 {
		t.Fatalf("chamfer changed only %d model pixels", changed)
	}
	b, err := LoadPNG(filepath.Join(dir, "before.png"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadPNG(filepath.Join(dir, "cancelled.png"))
	if err != nil {
		t.Fatal(err)
	}
	for y := 150; y < 540; y++ {
		for x := 320; x < 980; x++ {
			if b.At(x, y) != c.At(x, y) {
				t.Fatalf("cancel changed model at %d,%d", x, y)
			}
		}
	}
}
