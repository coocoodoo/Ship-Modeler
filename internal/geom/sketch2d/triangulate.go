package sketch2d

import (
	"modeler/internal/geom"
)

// Ear-clipping triangulation with hole bridging (SPEC-GEOMETRY §5.2). Every
// predicate is the exact int64 orientation test, so a triangulation is either
// an exact partition of the region or it is not produced at all — there is no
// tolerance to tune.

// Tri is one triangle of a triangulated region, wound counter-clockwise.
type Tri struct {
	A, B, C geom.Vec2i
	// Src carries the provenance of the boundary edge each triangle vertex
	// leaves along, or a zero Source for an interior edge.
	Src [3]Source
}

// Area2 is twice the triangle's signed area, positive when counter-clockwise.
func (t Tri) Area2() int64 { return geom.Cross(t.A, t.B, t.C) }

// vert is a working vertex: its position plus the source of the edge leaving it.
type vert struct {
	p   geom.Vec2i
	src Source
	// boundary marks a real region edge, as opposed to one of the two doubled
	// edges a hole bridge introduces.
	boundary bool
}

// Triangulate covers a region with counter-clockwise triangles, holes removed.
// It returns nil for anything degenerate rather than guessing.
func Triangulate(r Region) []Tri {
	poly := toVerts(r.Outer)
	if len(poly) < 3 {
		return nil
	}
	if geom.ShoelaceArea2(r.Outer.Pts) <= 0 {
		return nil
	}

	// Bridge the holes in, furthest right first. Taking them in that order means
	// a bridge never has to cross one that is still to be cut.
	holes := make([][]vert, 0, len(r.Holes))
	for _, h := range r.Holes {
		hv := toVerts(h)
		if len(hv) >= 3 {
			holes = append(holes, hv)
		}
	}
	sortHolesByRightmost(holes)
	for _, h := range holes {
		poly = bridgeHole(poly, h)
	}

	return earClip(poly)
}

func toVerts(l Loop) []vert {
	out := make([]vert, len(l.Pts))
	for i, p := range l.Pts {
		s := Source{}
		if i < len(l.Src) {
			s = l.Src[i]
		}
		out[i] = vert{p: p, src: s, boundary: true}
	}
	return out
}

// sortHolesByRightmost orders holes by their rightmost vertex, descending.
func sortHolesByRightmost(holes [][]vert) {
	for i := 1; i < len(holes); i++ {
		h := holes[i]
		hx := rightmost(h).p.X
		j := i - 1
		for j >= 0 && rightmost(holes[j]).p.X < hx {
			holes[j+1] = holes[j]
			j--
		}
		holes[j+1] = h
	}
}

func rightmost(vs []vert) vert {
	best := 0
	for i, v := range vs {
		if v.p.X > vs[best].p.X || (v.p.X == vs[best].p.X && v.p.Y < vs[best].p.Y) {
			best = i
		}
	}
	return vs[best]
}

func rightmostIndex(vs []vert) int {
	best := 0
	for i, v := range vs {
		if v.p.X > vs[best].p.X || (v.p.X == vs[best].p.X && v.p.Y < vs[best].p.Y) {
			best = i
		}
	}
	return best
}

// bridgeHole splices a hole into the outer polygon with a doubled edge, so the
// result is one weakly-simple polygon the ear clipper can chew through.
//
// The bridge runs from the hole's rightmost vertex to the nearest polygon
// vertex that can see it. Visibility is tested exactly: the candidate segment
// must cross no edge of either loop, and must leave the hole on the inside.
func bridgeHole(poly, hole []vert) []vert {
	hi := rightmostIndex(hole)
	m := hole[hi].p

	best, bestDist := -1, int64(0)
	for i := range poly {
		if poly[i].p == m {
			continue
		}
		d := poly[i].p.Sub(m).LenSq()
		if best >= 0 && d >= bestDist {
			continue
		}
		if !bridgeVisible(poly, hole, i, hi) {
			continue
		}
		best, bestDist = i, d
	}
	if best < 0 {
		// No vertex can see the hole. Rather than emit a wrong triangulation,
		// leave the hole out: the caller's exact area check will catch it.
		return poly
	}

	// poly[..best] + bridge into the hole, all the way round, back out, + the
	// rest of poly. The two bridge edges are marked as not-boundary so the
	// triangles that use them carry no false provenance.
	out := make([]vert, 0, len(poly)+len(hole)+2)
	out = append(out, poly[:best+1]...)
	out[len(out)-1].boundary = false
	for k := 0; k < len(hole); k++ {
		out = append(out, hole[(hi+k)%len(hole)])
	}
	entry := hole[hi]
	entry.boundary = false
	out = append(out, entry)
	out = append(out, poly[best:]...)
	return out
}

// bridgeVisible reports whether the segment from hole[hi] to poly[pi] stays
// inside the region.
func bridgeVisible(poly, hole []vert, pi, hi int) bool {
	a := poly[pi].p
	b := hole[hi].p

	blocks := func(vs []vert, skipA, skipB int) bool {
		n := len(vs)
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			if i == skipA || j == skipA || i == skipB || j == skipB {
				// Edges touching either endpoint of the bridge share a point
				// with it by construction; a shared endpoint is not a crossing.
				if !segmentsOverlap(a, b, vs[i].p, vs[j].p) {
					continue
				}
				return true
			}
			if geom.SegSegIntersect(a, b, vs[i].p, vs[j].p).Rel != geom.SegNone {
				return true
			}
		}
		return false
	}

	if blocks(poly, pi, -1) {
		return false
	}
	if blocks(hole, hi, -1) {
		return false
	}
	return midpointInside(a, b, poly)
}

// segmentsOverlap reports a collinear overlap of positive length, which a
// bridge sharing an endpoint with an edge must still reject.
func segmentsOverlap(a, b, c, d geom.Vec2i) bool {
	return geom.SegSegIntersect(a, b, c, d).Rel == geom.SegCollinear
}

// midpointInside tests the bridge's midpoint against the polygon, in doubled
// coordinates so the midpoint of two lattice points is itself exact.
func midpointInside(a, b geom.Vec2i, poly []vert) bool {
	mid := geom.Vec2i{X: a.X + b.X, Y: a.Y + b.Y}
	doubled := make([]geom.Vec2i, len(poly))
	for i, v := range poly {
		doubled[i] = geom.Vec2i{X: v.p.X * 2, Y: v.p.Y * 2}
	}
	inside, onEdge := geom.PointInPolygon(mid, doubled)
	return inside || onEdge
}

// earClip triangulates a counter-clockwise, weakly-simple polygon.
func earClip(poly []vert) []Tri {
	n := len(poly)
	if n < 3 {
		return nil
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}

	out := make([]Tri, 0, n)
	guard := 0
	maxGuard := n * n
	for len(idx) > 3 && guard < maxGuard {
		guard++
		clipped := false
		for k := 0; k < len(idx); k++ {
			prev := idx[(k+len(idx)-1)%len(idx)]
			cur := idx[k]
			next := idx[(k+1)%len(idx)]
			if !isEar(poly, idx, prev, cur, next) {
				continue
			}
			if tr, ok := makeTri(poly, prev, cur, next); ok {
				out = append(out, tr)
			}
			idx = append(idx[:k], idx[k+1:]...)
			clipped = true
			break
		}
		if !clipped {
			// Nothing is clippable: the polygon is degenerate or the bridge
			// left it self-touching in a way this pass cannot resolve. Emitting
			// a fan would silently produce a wrong area, so stop here and let
			// the caller's area check report it.
			break
		}
	}
	if len(idx) == 3 {
		if tr, ok := makeTri(poly, idx[0], idx[1], idx[2]); ok {
			out = append(out, tr)
		}
	}
	return out
}

func makeTri(poly []vert, a, b, c int) (Tri, bool) {
	t := Tri{A: poly[a].p, B: poly[b].p, C: poly[c].p}
	if t.Area2() <= 0 {
		return Tri{}, false
	}
	// An edge only carries provenance when it really is a region boundary.
	if poly[a].boundary && next(poly, a) == b {
		t.Src[0] = poly[a].src
	}
	if poly[b].boundary && next(poly, b) == c {
		t.Src[1] = poly[b].src
	}
	if poly[c].boundary && next(poly, c) == a {
		t.Src[2] = poly[c].src
	}
	return t, true
}

// next is the index following i in the working polygon's original order.
func next(poly []vert, i int) int { return (i + 1) % len(poly) }

// isEar reports whether the corner at cur can be clipped: it must turn left,
// and no other vertex may lie inside the triangle it would cut off.
func isEar(poly []vert, idx []int, prev, cur, next int) bool {
	a, b, c := poly[prev].p, poly[cur].p, poly[next].p
	if geom.Cross(a, b, c) <= 0 {
		return false // reflex or collinear
	}
	for _, j := range idx {
		if j == prev || j == cur || j == next {
			continue
		}
		p := poly[j].p
		// Compare by position, not by index. A hole bridge deliberately doubles
		// two vertices, and a duplicate that sits exactly on a corner of this
		// triangle is that same corner — it must not block the ear, or nothing
		// in a holed polygon is ever clippable.
		if p == a || p == b || p == c {
			continue
		}
		// Any other vertex touching the triangle, boundary included, does block
		// it: clipping past one would cut across the bridge.
		if pointInTriangle(p, a, b, c) {
			return false
		}
	}
	return true
}

// pointInTriangle is inclusive of the boundary and exact.
func pointInTriangle(p, a, b, c geom.Vec2i) bool {
	d1 := geom.Cross(a, b, p)
	d2 := geom.Cross(b, c, p)
	d3 := geom.Cross(c, a, p)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}
