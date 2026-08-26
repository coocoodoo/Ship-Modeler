package sketch2d

import (
	"math"

	"modeler/internal/geom"
)

// Miter offset (SPEC-GEOMETRY §5.3), the operation behind an extrude's draft
// angle. Every edge slides along its own left normal by delta and the new
// vertices are where consecutive offset edges meet.
//
// One rule covers both loop kinds. A counter-clockwise outer loop has its
// material on the left, so a positive delta shrinks it; a clockwise hole has
// its material on the left too — outside the hole — so the same positive delta
// grows the hole. That is exactly "material thins consistently".
//
// The vertex positions are computed in float64 and rounded back to subunits.
// That rounding is unavoidable — a mitre lands on an irrational point — and it
// is why every offset is followed by a validity check rather than trusted.

// MaxMiterSpread bounds how far a mitre may throw a vertex before the result is
// rejected outright, as a multiple of delta. A near-straight corner sends the
// intersection toward infinity; this catches it before the arithmetic does.
const MaxMiterSpread = 64.0

// OffsetLoop slides every edge of a loop along its left normal by delta.
//
// The second result is false when the offset is structurally broken: an edge
// reversed direction, or a mitre threw a vertex impossibly far. It does not
// check for self-intersection — that is ValidOffset's job, because it needs the
// original to compare against.
func OffsetLoop(l Loop, delta int64) (Loop, bool) {
	n := len(l.Pts)
	if n < 3 {
		return Loop{}, false
	}
	if delta == 0 {
		return l.clone(), true
	}

	d := float64(delta)
	// Each edge, shifted: a point on it and its direction.
	type lineSeg struct{ px, py, dx, dy float64 }
	lines := make([]lineSeg, n)
	for i := 0; i < n; i++ {
		a, b := l.Pts[i], l.Pts[(i+1)%n]
		ex, ey := float64(b.X-a.X), float64(b.Y-a.Y)
		length := math.Hypot(ex, ey)
		if length == 0 {
			return Loop{}, false
		}
		// The left normal of a to b, normalised.
		nx, ny := -ey/length, ex/length
		lines[i] = lineSeg{px: float64(a.X) + nx*d, py: float64(a.Y) + ny*d, dx: ex, dy: ey}
	}

	out := Loop{Pts: make([]geom.Vec2i, n), Src: make([]Source, n)}
	limit := math.Abs(d) * MaxMiterSpread
	for i := 0; i < n; i++ {
		prev := lines[(i-1+n)%n]
		cur := lines[i]

		x, y, ok := intersectLines(prev, cur)
		if !ok {
			// The two edges are parallel, so the corner is straight and the
			// offset vertex is simply the shifted point.
			x, y = cur.px, cur.py
		}
		if math.Abs(x-float64(l.Pts[i].X)) > limit || math.Abs(y-float64(l.Pts[i].Y)) > limit {
			return Loop{}, false
		}
		p := geom.Vec2i{X: int64(math.Round(x)), Y: int64(math.Round(y))}
		if !geom.InSketchRange(p) {
			return Loop{}, false
		}
		out.Pts[i] = p
		// The edge leaving this vertex is the same profile edge, moved, so it
		// keeps naming the entity it came from (SPEC-GEOMETRY §5.5).
		if i < len(l.Src) {
			out.Src[i] = l.Src[i]
		}
	}

	// Every offset edge must still run the same way as the edge it came from.
	// A reversed one means the corner folded through itself.
	for i := 0; i < n; i++ {
		oldE := l.Pts[(i+1)%n].Sub(l.Pts[i])
		newE := out.Pts[(i+1)%n].Sub(out.Pts[i])
		if newE.Dot(oldE) <= 0 {
			return Loop{}, false
		}
	}
	return out, true
}

// intersectLines solves two parametric lines, reporting false when parallel.
func intersectLines(a, b struct{ px, py, dx, dy float64 }) (x, y float64, ok bool) {
	den := a.dx*b.dy - a.dy*b.dx
	if math.Abs(den) < 1e-9 {
		return 0, 0, false
	}
	t := ((b.px-a.px)*b.dy - (b.py-a.py)*b.dx) / den
	return a.px + t*a.dx, a.py + t*a.dy, true
}

func (l Loop) clone() Loop {
	return Loop{
		Pts: append([]geom.Vec2i(nil), l.Pts...),
		Src: append([]Source(nil), l.Src...),
	}
}

// ValidOffset reports whether an offset loop is usable: it kept its winding,
// has real area, and does not cross itself.
func ValidOffset(original, offset Loop) bool {
	if len(offset.Pts) < 3 {
		return false
	}
	oa, na := original.Area2(), offset.Area2()
	if na == 0 || (oa > 0) != (na > 0) {
		return false
	}
	return !loopSelfIntersects(offset.Pts)
}

// loopSelfIntersects tests every non-adjacent pair of edges exactly. O(n^2) is
// fine at sketch scale, and the spec says so.
func loopSelfIntersects(pts []geom.Vec2i) bool {
	n := len(pts)
	for i := 0; i < n; i++ {
		a1, a2 := pts[i], pts[(i+1)%n]
		for j := i + 1; j < n; j++ {
			// Adjacent edges share a vertex by construction.
			if j == i || (j+1)%n == i || (i+1)%n == j {
				continue
			}
			b1, b2 := pts[j], pts[(j+1)%n]
			if geom.SegSegIntersect(a1, a2, b1, b2).Rel != geom.SegNone {
				return true
			}
		}
	}
	return false
}

// OffsetRegion offsets a region's outer loop and every hole by one delta.
//
// It also checks the relationships the loops must keep: no hole may cross or
// escape the outer loop, and no two holes may meet.
func OffsetRegion(r Region, delta int64) (Region, bool) {
	outer, ok := OffsetLoop(r.Outer, delta)
	if !ok || !ValidOffset(r.Outer, outer) {
		return Region{}, false
	}
	out := Region{Outer: outer}

	for _, h := range r.Holes {
		oh, ok := OffsetLoop(h, delta)
		if !ok || !ValidOffset(h, oh) {
			return Region{}, false
		}
		out.Holes = append(out.Holes, oh)
	}

	for i, h := range out.Holes {
		if !loopStrictlyInside(h.Pts, outer.Pts) {
			return Region{}, false
		}
		if loopsCross(h.Pts, outer.Pts) {
			return Region{}, false
		}
		for j := i + 1; j < len(out.Holes); j++ {
			if loopsCross(h.Pts, out.Holes[j].Pts) {
				return Region{}, false
			}
		}
	}
	// A region whose holes have eaten it has nothing left to build from.
	if out.Area2() <= 0 {
		return Region{}, false
	}
	return out, true
}

// loopStrictlyInside reports whether every vertex of inner lies inside outer,
// touching neither its boundary nor its outside.
func loopStrictlyInside(inner, outer []geom.Vec2i) bool {
	for _, p := range inner {
		in, onEdge := geom.PointInPolygon(p, outer)
		if !in || onEdge {
			return false
		}
	}
	return true
}

// loopsCross reports whether any edge of one loop meets any edge of the other.
func loopsCross(a, b []geom.Vec2i) bool {
	for i := range a {
		a1, a2 := a[i], a[(i+1)%len(a)]
		for j := range b {
			b1, b2 := b[j], b[(j+1)%len(b)]
			if geom.SegSegIntersect(a1, a2, b1, b2).Rel != geom.SegNone {
				return true
			}
		}
	}
	return false
}

// ClampIterations is how many halvings the binary search takes. Twelve lands
// within a sixteenth of a subunit of the true limit for any realistic profile.
const ClampIterations = 12

// ClampOffsetRegion offsets by as much of delta as the profile can take, and
// reports how much that was.
//
// When the requested offset is legal the answer is exactly it. When it is not,
// a binary search finds the largest magnitude that still validates, which is
// what the UI shows as the clamped draft angle (SPEC-UX §9.3).
func ClampOffsetRegion(r Region, delta int64) (Region, int64) {
	if got, ok := OffsetRegion(r, delta); ok {
		return got, delta
	}

	sign := int64(1)
	if delta < 0 {
		sign = -1
	}
	lo, hi := int64(0), delta*sign // magnitudes, lo always valid

	best, bestDelta := r, int64(0)
	for i := 0; i < ClampIterations && lo < hi; i++ {
		mid := lo + (hi-lo)/2
		if mid == lo {
			break
		}
		if got, ok := OffsetRegion(r, mid*sign); ok {
			best, bestDelta = got, mid*sign
			lo = mid
		} else {
			hi = mid
		}
	}
	return best, bestDelta
}

// DeltaForDraft converts a draft angle into an offset distance for a given
// extrude depth: delta = depth * tan(theta), rounded to subunits
// (SPEC-GEOMETRY §5.3).
func DeltaForDraft(depthSubunits int64, degrees float64) int64 {
	if degrees == 0 {
		return 0
	}
	return int64(math.Round(float64(depthSubunits) * math.Tan(degrees*math.Pi/180)))
}

// DraftForDelta is the inverse, so a clamped offset can be reported back as the
// angle the user actually got.
func DraftForDelta(depthSubunits, delta int64) float64 {
	if depthSubunits == 0 {
		return 0
	}
	return math.Atan2(float64(delta), float64(depthSubunits)) * 180 / math.Pi
}
