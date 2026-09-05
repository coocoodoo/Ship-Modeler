package paint

import (
	"image"
	"image/color"
	"math"

	"modeler/internal/geom/mesh"
)

// The two-point tools of SPEC-UX §13.4: line, rectangle, ellipse and gradient.
//
// Every one of them is decided by exactly two texels — where the press landed
// and where the pointer is now — which is what lets the drag rubber-band for
// free: the stroke command re-runs from those two points each frame, so the
// shape you let go of is the shape you were looking at.

// DrawRect paints an axis-aligned rectangle spanning two texels, inclusive of
// both. Dragging up or to the left gives the same box as dragging down and
// right, because a rectangle is its corners and not its order.
func DrawRect(p *mesh.FacePaint, b Brush, a, z image.Point, filled bool) image.Rectangle {
	box := boxOf(a, z)
	if filled {
		dirty := image.Rectangle{}
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				dirty = union(dirty, put(p, b, image.Point{X: x, Y: y}, 1))
			}
		}
		return dirty
	}
	// The outline is drawn with the brush, so a thick rectangle is four thick
	// walls rather than a hairline with a wide dirty rect.
	tl := image.Point{X: box.Min.X, Y: box.Min.Y}
	tr := image.Point{X: box.Max.X - 1, Y: box.Min.Y}
	br := image.Point{X: box.Max.X - 1, Y: box.Max.Y - 1}
	bl := image.Point{X: box.Min.X, Y: box.Max.Y - 1}
	dirty := Stroke(p, b, tl, tr)
	dirty = union(dirty, Stroke(p, b, tr, br))
	dirty = union(dirty, Stroke(p, b, br, bl))
	return union(dirty, Stroke(p, b, bl, tl))
}

// DrawEllipse paints the ellipse inscribed in the box the two texels span.
//
// It is rasterised a scanline at a time from the ellipse equation rather than
// by a midpoint walk, because the walk has to be special-cased at both axis
// crossings and this does not: solving for x at each y gives a run that is
// symmetric by construction, is trivially fillable, and cannot leave the gaps a
// mis-stepped walk does.
func DrawEllipse(p *mesh.FacePaint, b Brush, a, z image.Point, filled bool) image.Rectangle {
	box := boxOf(a, z)
	w, h := box.Dx(), box.Dy()
	if w <= 0 || h <= 0 {
		return image.Rectangle{}
	}
	if w <= 2 || h <= 2 {
		// Too small to have an inside; the box itself is the honest answer.
		return DrawRect(p, b, a, z, filled || w <= 1 || h <= 1)
	}

	spans := ellipseSpans(box)
	inside := func(x, y int) bool {
		i := y - box.Min.Y
		if i < 0 || i >= len(spans) || spans[i].empty {
			return false
		}
		return x >= spans[i].lo && x <= spans[i].hi
	}

	var pts []image.Point
	for y := box.Min.Y; y < box.Max.Y; y++ {
		sp := spans[y-box.Min.Y]
		if sp.empty {
			continue
		}
		for x := sp.lo; x <= sp.hi; x++ {
			// Filled takes the whole span; an outline takes the texels of it
			// that have a neighbour outside the shape. Asking the shape rather
			// than tracking a walk is what makes the outline symmetric for
			// free, and closed even where the curve runs nearly flat.
			if filled || x == sp.lo || x == sp.hi ||
				!inside(x, y-1) || !inside(x, y+1) {
				pts = append(pts, image.Point{X: x, Y: y})
			}
		}
	}
	return applyPoints(p, b, pts, filled)
}

// span is one scanline's run of an ellipse.
type span struct {
	lo, hi int
	empty  bool
}

// ellipseSpans solves the ellipse for x at each row of its box.
func ellipseSpans(box image.Rectangle) []span {
	cx := float64(box.Min.X+box.Max.X-1) / 2
	cy := float64(box.Min.Y+box.Max.Y-1) / 2
	rx, ry := float64(box.Dx()-1)/2, float64(box.Dy()-1)/2

	out := make([]span, box.Dy())
	for i := range out {
		y := box.Min.Y + i
		dy := (float64(y) - cy) / ry
		s := 1 - dy*dy
		if s < 0 {
			out[i] = span{empty: true}
			continue
		}
		half := rx * math.Sqrt(s)
		lo := int(math.Ceil(cx - half - 1e-9))
		hi := int(math.Floor(cx + half + 1e-9))
		if lo > hi {
			out[i] = span{empty: true}
			continue
		}
		out[i] = span{lo: lo, hi: hi}
	}
	return out
}

// applyPoints puts the brush down on a rasterised shape: one texel each for a
// fill, a whole dab each for an outline drawn with a wide brush.
func applyPoints(p *mesh.FacePaint, b Brush, pts []image.Point, filled bool) image.Rectangle {
	dirty := image.Rectangle{}
	wide := !filled && b.Size > 1
	for _, t := range pts {
		if wide {
			dirty = union(dirty, dab(p, b, t, b.Size))
			continue
		}
		dirty = union(dirty, put(p, b, t, 1))
	}
	return dirty
}

// Gradient ramps between two colours across a region, along the axis the drag
// drew (SPEC-UX §13.4).
//
// With a Bayer mode the ramp spends its middle on a pattern of the two colours
// rather than on colours between them, which is what keeps a gradient inside
// the palette it was drawn from.
func Gradient(p *mesh.FacePaint, region image.Rectangle, a, z image.Point,
	from, to color.RGBA, d Dither, mask *Mask) image.Rectangle {

	if p == nil || region.Empty() {
		return image.Rectangle{}
	}
	// A caller that never set an alpha means an opaque ramp, which is the
	// same reading the brush gives a bare colour.
	if from.A == 0 {
		from.A = 255
	}
	if to.A == 0 {
		to.A = 255
	}
	dx, dy := float64(z.X-a.X), float64(z.Y-a.Y)
	lenSq := dx*dx + dy*dy

	dirty := image.Rectangle{}
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if mask != nil && !mask.Contains(image.Point{X: x, Y: y}) {
				continue
			}
			t := 1.0
			if lenSq > 0 {
				// The projection onto the drag axis, which is what makes the
				// bands perpendicular to the direction you dragged.
				t = (float64(x-a.X)*dx + float64(y-a.Y)*dy) / lenSq
				t = clamp01f(t)
			}
			c := to
			if d == DitherNone {
				c = lerpColor(from, to, t)
				// lerpColor stores an opaque result, which is what the
				// coverage blend it was written for wants and the opposite of
				// what a translucent ramp wants: the alpha is the brush's and
				// has to survive the mix (V-158).
				c.A = uint8(float64(from.A) + (float64(to.A)-float64(from.A))*t + 0.5)
			} else if !d.Covers(x, y, t) {
				c = from
			}
			at := image.Point{X: x, Y: y}
			if c.A < 255 {
				c = Over(At(p, at), c)
			} else {
				c.A = 255
			}
			if Set(p, at, c) {
				dirty = union(dirty, oneTexel(at))
			}
		}
	}
	return dirty
}

// Over composites src onto dst by src's alpha, the ordinary source-over rule.
//
// A texel's alpha is what the viewport mixes the body's own colour through, so
// a partly transparent result is paint you can see the hull beneath — which is
// exactly what an alpha below full is asking for. Fully opaque src replaces
// dst outright, which is the path every tool took before alpha existed and
// still takes whenever the slider is at the top (V-158).
func Over(dst, src color.RGBA) color.RGBA {
	sa := float64(src.A) / 255
	if sa >= 1 {
		return color.RGBA{R: src.R, G: src.G, B: src.B, A: 255}
	}
	if sa <= 0 {
		return dst
	}
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return color.RGBA{}
	}
	ch := func(s, d uint8) uint8 {
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / outA
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		return uint8(v + 0.5)
	}
	return color.RGBA{
		R: ch(src.R, dst.R), G: ch(src.G, dst.G), B: ch(src.B, dst.B),
		A: uint8(outA*255 + 0.5),
	}
}

// put writes one texel at a coverage, which is the single place the brush's
// softness, its dithering and the eraser all resolve into a colour.
func put(p *mesh.FacePaint, b Brush, at image.Point, coverage float64) image.Rectangle {
	if coverage <= 0 {
		return image.Rectangle{}
	}
	// The wand's selection, honoured here and nowhere else: every tool funnels
	// through this one writer, so a texel outside the mask is untouchable by
	// construction rather than by each tool remembering to check.
	if b.Mask != nil && !b.Mask.Contains(at) {
		return image.Rectangle{}
	}
	if b.Erase {
		// Erasing outside the image has nothing to rub out, and growing the
		// image to store transparency would be pure waste.
		if !at.In(Bounds(p)) || !b.Dither.Covers(at.X, at.Y, coverage) {
			return image.Rectangle{}
		}
		if !Set(p, at, color.RGBA{}) {
			return image.Rectangle{}
		}
		return oneTexel(at)
	}

	c := b.Color
	if c.A == 0 {
		c.A = 255 // a caller that never set an alpha means an opaque brush
	}
	// A translucent brush lays paint you can see through, so it composites
	// onto the texel rather than replacing it, and its coverage rides on the
	// same alpha: half a dab of half-transparent paint is a quarter laid down.
	if c.A < 255 {
		// Dithering spends the dab's *coverage* on whole texels rather than on
		// a blend, which is what keeps a soft edge or a ramp inside the
		// palette. It must not spend the alpha with it. The two are different
		// questions - coverage is how much of this texel the brush is over,
		// alpha is how much of the colour is being laid down - and putting
		// alpha through the threshold turns an even glaze into paint on every
		// other texel, which is what a hard dab at half alpha came out as
		// while any dither was armed (V-158).
		if b.Dither != DitherNone {
			if !b.Dither.Covers(at.X, at.Y, coverage) {
				return image.Rectangle{}
			}
			coverage = 1
		}
		a := float64(c.A) / 255 * coverage
		if b.Once != nil {
			// How much this texel has already taken from this stroke. Going
			// from that to a total of a means adding only the difference the
			// stroke has left to give, which is what keeps the joins between
			// overlapping dabs from stacking up darker than the dabs.
			had := b.Once[at]
			if a <= had {
				return image.Rectangle{}
			}
			b.Once[at] = a
			a = (a - had) / (1 - had)
		}
		src := c
		src.A = uint8(a*255 + 0.5)
		if src.A == 0 {
			return image.Rectangle{}
		}
		out := Over(At(p, at), src)
		if !Set(p, at, out) {
			return image.Rectangle{}
		}
		return oneTexel(at)
	}
	switch {
	case coverage >= 1:
	case b.Dither != DitherNone:
		if !b.Dither.Covers(at.X, at.Y, coverage) {
			return image.Rectangle{}
		}
	default:
		// Blend into whatever is under this texel — the paint already there, or
		// the body's own colour where there is none — and store the result
		// opaque. A partial alpha would composite against the body instead,
		// which is the body showing through paint that is supposed to be on top
		// of it.
		under := At(p, at)
		if under.A == 0 {
			under = b.Under
		}
		c = lerpColor(under, c, coverage)
		c.A = 255
	}
	if !Set(p, at, c) {
		return image.Rectangle{}
	}
	return oneTexel(at)
}

// SoftCore is how much of a soft brush's radius is solid before the falloff
// starts. A brush with no solid middle is never fully its own colour anywhere,
// which reads as a weak brush rather than a soft one.
const SoftCore = 0.4

// softCoverage is a round brush's falloff: solid in the middle, nothing at the
// rim of its box.
//
// The radius runs half a texel past the box so the outermost ring still gets
// something; without that slack a size-4 brush would be a two-texel dot with a
// faint halo.
func softCoverage(at, anchor image.Point, size int) float64 {
	r := float64(size)/2 + 0.5
	dx := float64(at.X-anchor.X) - float64(size-1)/2
	dy := float64(at.Y-anchor.Y) - float64(size-1)/2
	d := math.Hypot(dx, dy)
	if d >= r {
		return 0
	}
	core := r * SoftCore
	if d <= core {
		return 1
	}
	return (r - d) / (r - core)
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01f(t)
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

func clamp01f(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func oneTexel(t image.Point) image.Rectangle {
	return image.Rectangle{Min: t, Max: t.Add(image.Point{X: 1, Y: 1})}
}

// boxOf is the inclusive box two texels span, as a half-open rectangle.
func boxOf(a, z image.Point) image.Rectangle {
	r := image.Rectangle{Min: a, Max: z}
	if r.Min.X > r.Max.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Min.Y > r.Max.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	r.Max = r.Max.Add(image.Point{X: 1, Y: 1})
	return r
}
