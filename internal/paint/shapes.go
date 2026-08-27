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
	from, to color.RGBA, d Dither) image.Rectangle {

	if p == nil || region.Empty() {
		return image.Rectangle{}
	}
	dx, dy := float64(z.X-a.X), float64(z.Y-a.Y)
	lenSq := dx*dx + dy*dy

	dirty := image.Rectangle{}
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
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
			} else if !d.Covers(x, y, t) {
				c = from
			}
			c.A = 255
			at := image.Point{X: x, Y: y}
			if Set(p, at, c) {
				dirty = union(dirty, oneTexel(at))
			}
		}
	}
	return dirty
}

// put writes one texel at a coverage, which is the single place the brush's
// softness, its dithering and the eraser all resolve into a colour.
func put(p *mesh.FacePaint, b Brush, at image.Point, coverage float64) image.Rectangle {
	if coverage <= 0 {
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
	c.A = 255
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
