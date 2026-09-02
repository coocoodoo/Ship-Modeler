package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The icon set is reviewed by eye, so this writes a contact sheet of every
// icon at the sizes they are actually drawn at. It needs no GPU: the
// rasterizer produces an image, and only the upload needs a context.
//
// Set ICON_SHEET to a directory to get the sheets written there.

func TestEveryIconRasterizes(t *testing.T) {
	names := IconNames()
	if len(names) == 0 {
		t.Fatal("no icon assets found")
	}
	for _, n := range names {
		if s := svgShapeFor(n); s == nil {
			t.Errorf("%s.svg did not parse", n)
			continue
		}
		for _, px := range []int{14, 18, 24} {
			m := IconMask(n, px)
			if m == nil {
				t.Fatalf("%s produced no mask at %d px", n, px)
			}
			ink := 0
			for i := 3; i < len(m.Pix); i += 4 {
				if m.Pix[i] > 8 {
					ink++
				}
			}
			// An icon that rasterizes to nothing is a file that silently does
			// not draw — the exact failure the fallback would hide.
			if ink < px {
				t.Errorf("%s at %d px is nearly blank (%d lit pixels)", n, px, ink)
			}
		}
	}
}

// Antialiasing is the whole point of the rasterizer: a diagonal edge has to
// land on a ramp of partial coverage, not a staircase of on/off pixels.
func TestEdgesAreAntialiased(t *testing.T) {
	m := IconMask("linetool", 24)
	if m == nil {
		t.Skip("no linetool icon to measure")
	}
	partial := 0
	for i := 3; i < len(m.Pix); i += 4 {
		if a := m.Pix[i]; a > 8 && a < 247 {
			partial++
		}
	}
	if partial < 20 {
		t.Errorf("only %d partially covered pixels; the edges are not being antialiased", partial)
	}
}

func TestIconContactSheet(t *testing.T) {
	dir := os.Getenv("ICON_SHEET")
	if dir == "" {
		t.Skip("set ICON_SHEET to write the contact sheets")
	}
	names := IconNames()
	for _, px := range []int{18, 48} {
		const cols = 12
		pad := px / 2
		cell := px + pad
		rows := (len(names) + cols - 1) / cols
		w, h := cols*cell+pad, rows*cell+pad
		sheet := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.RGBA{0x14, 0x16, 0x1C, 0xFF}},
			image.Point{}, draw.Src)
		for i, n := range names {
			m := IconMask(n, px)
			if m == nil {
				continue
			}
			x := pad + (i%cols)*cell
			y := pad + (i/cols)*cell
			// The mask is white with alpha; composite it the way the app does.
			draw.DrawMask(sheet, image.Rect(x, y, x+px, y+px),
				&image.Uniform{color.RGBA{0xE8, 0xEA, 0xF0, 0xFF}}, image.Point{},
				m, image.Point{}, draw.Over)
		}
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("icons_%d.png", px)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, sheet); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	t.Logf("wrote contact sheets for %d icons to %s", len(names), dir)
}
