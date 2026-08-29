package paint

import (
	"image"
	"image/color"

	"modeler/internal/geom/mesh"
)

// The magic wand's selection (the user's request, 2026-08-28).
//
// A mask is a set of texels on one face. The wand builds one by flooding from
// a clicked texel across everything within a colour tolerance of it, and from
// then on every painting tool writes only inside it: the mask rides the brush
// into put(), the single writer every tool already funnels through, so
// obedience is structural rather than per-tool.
//
// It is app state, not document state: a selection is scaffolding around an
// edit, not part of the ship, so it neither saves nor undoes. What it
// *constrains* undoes exactly like any stroke, because the stroke commands
// snapshot what they actually wrote.

// Mask is a set of texels, stored as a bitmap over its bounding rectangle.
type Mask struct {
	// Rect bounds the bitmap in texel coordinates.
	Rect image.Rectangle
	// Bits is Rect.Dx()*Rect.Dy() flags, row-major from Rect.Min.
	Bits []bool

	count int
}

// Contains reports whether a texel is selected.
func (m *Mask) Contains(t image.Point) bool {
	if m == nil || !t.In(m.Rect) {
		return false
	}
	return m.Bits[(t.Y-m.Rect.Min.Y)*m.Rect.Dx()+(t.X-m.Rect.Min.X)]
}

// Count is how many texels are selected.
func (m *Mask) Count() int {
	if m == nil {
		return 0
	}
	return m.count
}

// set marks one texel, growing nothing: t must be inside Rect.
func (m *Mask) set(t image.Point) {
	i := (t.Y-m.Rect.Min.Y)*m.Rect.Dx() + (t.X - m.Rect.Min.X)
	if !m.Bits[i] {
		m.Bits[i] = true
		m.count++
	}
}

// newMask is an empty mask over a rectangle.
func newMask(r image.Rectangle) *Mask {
	return &Mask{Rect: r, Bits: make([]bool, r.Dx()*r.Dy())}
}

// Union merges another mask in, which is what Shift-clicking does. The two
// must describe the same picture; the result covers both rectangles.
func (m *Mask) Union(o *Mask) *Mask {
	if m == nil {
		return o
	}
	if o == nil {
		return m
	}
	out := newMask(m.Rect.Union(o.Rect))
	for _, src := range []*Mask{m, o} {
		for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
			for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
				t := image.Point{X: x, Y: y}
				if src.Contains(t) {
					out.set(t)
				}
			}
		}
	}
	return out
}

// WandSelect floods from a seed texel across every 4-connected texel whose
// colour is within tolerance of the seed's, bounded by the face's rectangle.
//
// A bare texel reads as the body's own colour — the same rule the eyedropper
// and the flood fill live by — so the wand works on unpainted hull exactly
// like painted: click the bare metal and the selection is the bare metal.
//
// Tolerance is the largest per-channel difference allowed, 0..255. Zero means
// exactly the seed's colour; 255 selects the whole connected face. Per-channel
// rather than a distance formula because this is a pixel-art tool: palettes
// are ramps, and "within N steps on every channel" is how a ramp neighbours.
func WandSelect(p *mesh.FacePaint, region image.Rectangle, seed image.Point, tolerance uint8, under color.RGBA) *Mask {
	if p == nil || region.Empty() || !seed.In(region) {
		return nil
	}
	sample := func(t image.Point) color.RGBA {
		c := At(p, t)
		if c.A == 0 {
			c = under
			c.A = 255
		}
		return c
	}
	within := func(a, b color.RGBA) bool {
		return absDiff(a.R, b.R) <= tolerance &&
			absDiff(a.G, b.G) <= tolerance &&
			absDiff(a.B, b.B) <= tolerance
	}

	want := sample(seed)
	m := newMask(region)
	m.set(seed)
	queue := []image.Point{seed}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		for _, n := range [4]image.Point{
			{X: t.X + 1, Y: t.Y}, {X: t.X - 1, Y: t.Y},
			{X: t.X, Y: t.Y + 1}, {X: t.X, Y: t.Y - 1},
		} {
			if !n.In(region) || m.Contains(n) || !within(sample(n), want) {
				continue
			}
			m.set(n)
			queue = append(queue, n)
		}
	}
	return m
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// Boundary returns the selection's outline as unit segments in texel-corner
// coordinates: every edge between a selected texel and an unselected (or
// outside) neighbour. It is what the viewport draws, so the selection is
// visible as a crisp border rather than a wash over the paint being judged.
func (m *Mask) Boundary() [][2]image.Point {
	if m == nil {
		return nil
	}
	var out [][2]image.Point
	for y := m.Rect.Min.Y; y < m.Rect.Max.Y; y++ {
		for x := m.Rect.Min.X; x < m.Rect.Max.X; x++ {
			t := image.Point{X: x, Y: y}
			if !m.Contains(t) {
				continue
			}
			if !m.Contains(image.Point{X: x, Y: y - 1}) { // top edge
				out = append(out, [2]image.Point{{X: x, Y: y}, {X: x + 1, Y: y}})
			}
			if !m.Contains(image.Point{X: x, Y: y + 1}) { // bottom
				out = append(out, [2]image.Point{{X: x, Y: y + 1}, {X: x + 1, Y: y + 1}})
			}
			if !m.Contains(image.Point{X: x - 1, Y: y}) { // left
				out = append(out, [2]image.Point{{X: x, Y: y}, {X: x, Y: y + 1}})
			}
			if !m.Contains(image.Point{X: x + 1, Y: y}) { // right
				out = append(out, [2]image.Point{{X: x + 1, Y: y}, {X: x + 1, Y: y + 1}})
			}
		}
	}
	return out
}
