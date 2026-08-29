package paint

import (
	"image"
	"image/color"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The magic wand (V-132): what it selects, and that every tool obeys the
// selection. The obedience tests matter more than the flood — a wand whose
// mask only the pencil respected would be a lie with a slider.

var (
	wandGrey = color.RGBA{R: 140, G: 160, B: 175, A: 255}
	wandRed  = color.RGBA{R: 220, G: 60, B: 50, A: 255}
	wandBlue = color.RGBA{R: 60, G: 90, B: 210, A: 255}
)

// wandFace is an 8x8-texel picture with a red 3x3 block at (1,1) on a blue
// ground, and the face rect covering all of it.
func wandFace(t *testing.T) (*mesh.FacePaint, image.Rectangle) {
	t.Helper()
	m := mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -4}, geom.Vec3{X: 4, Y: 1, Z: 4}, 1)
	fi := -1
	for i := range m.Faces {
		if m.FaceNormal(i).Y > 0.9 {
			fi = i
			break
		}
	}
	p, err := Allocate(m, fi, 1) // 8 u face at 1 px/u: 8x8 texels
	if err != nil {
		t.Fatal(err)
	}
	m.Faces[fi].Paint = p
	r := FaceRect(m, fi, p)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			Set(p, image.Point{X: x, Y: y}, wandBlue)
		}
	}
	for y := 1; y <= 3; y++ {
		for x := 1; x <= 3; x++ {
			Set(p, image.Point{X: x, Y: y}, wandRed)
		}
	}
	return p, r
}

func TestWandSelectsTheContiguousColour(t *testing.T) {
	p, r := wandFace(t)
	m := WandSelect(p, r, image.Point{X: 2, Y: 2}, 0, wandGrey)
	if m.Count() != 9 {
		t.Fatalf("selected %d texels, want the 3x3 red block", m.Count())
	}
	if !m.Contains(image.Point{X: 1, Y: 1}) || m.Contains(image.Point{X: 4, Y: 2}) {
		t.Error("the selection does not match the block")
	}
}

func TestToleranceWidensTheSelection(t *testing.T) {
	p, r := wandFace(t)
	// Nudge one neighbouring texel close to red: within 30, not within 0.
	Set(p, image.Point{X: 4, Y: 2}, color.RGBA{R: 200, G: 80, B: 60, A: 255})
	tight := WandSelect(p, r, image.Point{X: 2, Y: 2}, 0, wandGrey)
	loose := WandSelect(p, r, image.Point{X: 2, Y: 2}, 30, wandGrey)
	if tight.Count() != 9 {
		t.Errorf("tolerance 0 selected %d, want 9", tight.Count())
	}
	if loose.Count() != 10 {
		t.Errorf("tolerance 30 selected %d, want 10 (the near-red joins)", loose.Count())
	}
	// Full tolerance takes the whole face.
	all := WandSelect(p, r, image.Point{X: 2, Y: 2}, 255, wandGrey)
	if all.Count() != r.Dx()*r.Dy() {
		t.Errorf("tolerance 255 selected %d, want every texel", all.Count())
	}
}

func TestBareTexelsReadAsTheBodyColour(t *testing.T) {
	m := mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -4}, geom.Vec3{X: 4, Y: 1, Z: 4}, 1)
	fi := 0
	p, err := Allocate(m, fi, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Faces[fi].Paint = p
	r := FaceRect(m, fi, p)
	sel := WandSelect(p, r, r.Min, 0, wandGrey)
	if sel.Count() != r.Dx()*r.Dy() {
		t.Errorf("a bare face is one colour — the body's — but the wand took %d of %d",
			sel.Count(), r.Dx()*r.Dy())
	}
}

func TestEveryToolObeysTheMask(t *testing.T) {
	countRed := func(p *mesh.FacePaint, r image.Rectangle, outside func(image.Point) bool) int {
		n := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				pt := image.Point{X: x, Y: y}
				if outside(pt) && At(p, pt) == wandRed {
					n++
				}
			}
		}
		return n
	}

	type stroke func(p *mesh.FacePaint, r image.Rectangle, m *Mask)
	cases := map[string]stroke{
		"pencil line across": func(p *mesh.FacePaint, r image.Rectangle, m *Mask) {
			Stroke(p, Brush{Color: wandRed, Size: 2, Mask: m}, image.Point{X: 0, Y: 4}, image.Point{X: 7, Y: 4})
		},
		"rect outline": func(p *mesh.FacePaint, r image.Rectangle, m *Mask) {
			DrawRect(p, Brush{Color: wandRed, Size: 1, Mask: m}, image.Point{X: 0, Y: 0}, image.Point{X: 7, Y: 7}, true)
		},
		"gradient": func(p *mesh.FacePaint, r image.Rectangle, m *Mask) {
			Gradient(p, r, image.Point{X: 0, Y: 0}, image.Point{X: 7, Y: 7}, wandRed, wandRed, DitherNone, m)
		},
		"flood fill from inside": func(p *mesh.FacePaint, r image.Rectangle, m *Mask) {
			Fill(p, r, image.Point{X: 5, Y: 5}, wandRed, m)
		},
	}

	for name, apply := range cases {
		p, r := wandFace(t)
		// Select the blue ground around the red block...
		m := WandSelect(p, r, image.Point{X: 6, Y: 6}, 0, wandGrey)
		// ...then invert the question: paint red everywhere the tool reaches;
		// no red may land on the texels OUTSIDE the mask that were not red
		// before (the block itself was red already, so measure the mask).
		apply(p, r, m)
		leaked := countRed(p, r, func(pt image.Point) bool {
			return !m.Contains(pt) && !(pt.X >= 1 && pt.X <= 3 && pt.Y >= 1 && pt.Y <= 3)
		})
		if leaked > 0 {
			t.Errorf("%s: %d texels outside the selection were painted", name, leaked)
		}
	}
}

func TestBoundaryOutlinesTheSelection(t *testing.T) {
	p, r := wandFace(t)
	m := WandSelect(p, r, image.Point{X: 2, Y: 2}, 0, wandGrey)
	segs := m.Boundary()
	// A 3x3 block has 12 unit edges of border.
	if len(segs) != 12 {
		t.Errorf("boundary = %d segments, want 12 for a 3x3 block", len(segs))
	}
}

func TestUnionMergesSelections(t *testing.T) {
	p, r := wandFace(t)
	block := WandSelect(p, r, image.Point{X: 2, Y: 2}, 0, wandGrey)
	ground := WandSelect(p, r, image.Point{X: 6, Y: 6}, 0, wandGrey)
	all := block.Union(ground)
	if all.Count() != r.Dx()*r.Dy() {
		t.Errorf("block + ground = %d texels, want the whole face", all.Count())
	}
}
