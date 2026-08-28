package paint

import (
	"image"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Painting a line along an edge (the user's request, 2026-08-27).
//
// Panel lines, plating seams and outlined hulls all want the same thing: a
// stripe that runs along an edge and turns the corner onto both faces that
// meet there. Doing it by hand means painting each face separately and lining
// the two halves up by eye, at which point the pixels no longer meet.
//
// The work is small because the pieces were already here: an edge knows which
// faces use it (mesh.Topo), and a face's paint knows where a world position
// lands in its own texels (Texel). This is the arithmetic that puts the band
// on the face rather than across the boundary.

// EdgeBand paints a stripe of the brush's width in texels along an edge, on
// one face, and returns the texel rectangle it wrote.
//
// The width is in texels because this is a pixel-art tool: a line one texel
// wide is the crispest a face can draw, and "how many pixels" is the question
// someone drawing panel seams is actually asking. It does mean the band is a
// different *physical* thickness on faces with different texel densities —
// a face's texel is its longest side over its resolution — which is the same
// thing every brush stroke in the program already does (V-126).
//
// The band is laid *inside* the face rather than centred on the edge. An edge
// is the boundary between two faces, so a brush centred on it spends half its
// width on texels that are not on this face at all — invisible, and half the
// thickness that was asked for.
//
// The band is rasterized geometrically: every texel whose square overlaps the
// edge swept inward by the width, however slightly, takes paint (V-135). It
// used to be a Bresenham walk of square dabs, and on a slanted edge the walk
// floored each dab to the grid — up to a whole texel away from the edge,
// always toward the same side — leaving a stair-stepped sliver of bare body
// colour exactly where the line was asked for. Overshoot past the edge is
// safe by construction: the renderer clips a face's picture to the face, so
// a partly-outside texel shows only its inside part, painted.
func EdgeBand(m *mesh.Mesh, fi int, p *mesh.FacePaint, b Brush, worldA, worldB geom.Vec3) image.Rectangle {
	g, ok := edgeBandGeom(m, fi, p, b.Size, worldA, worldB)
	if !ok {
		return image.Rectangle{}
	}
	// Nothing may land off the face's rectangle: the margin around a face's
	// picture exists so a brush can overrun without checking, but paint out
	// there is paint nobody sees.
	clip := g.bbox.Intersect(FaceRect(m, fi, p))

	dirty := image.Rectangle{}
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		for x := clip.Min.X; x < clip.Max.X; x++ {
			at := image.Point{X: x, Y: y}
			if !g.covers(at) {
				continue
			}
			coverage := 1.0
			if b.Soft {
				coverage = g.softAt(at)
			}
			dirty = union(dirty, put(p, b, at, coverage))
		}
	}
	return dirty
}

// bandGeom is an edge band as an oriented rectangle in continuous texel
// space: the edge segment swept inward across the face by the band's width.
type bandGeom struct {
	a        geom.Vec2 // the edge's A end, in texels
	dir, nrm geom.Vec2 // unit: along the edge, and inward across it
	s0, s1   float64   // extent along dir, after any border extension
	w        float64   // the width
	bbox     image.Rectangle
}

// edgeBandGeom lays the band out for one face.
func edgeBandGeom(m *mesh.Mesh, fi int, p *mesh.FacePaint, size int, worldA, worldB geom.Vec3) (bandGeom, bool) {
	if m == nil || p == nil || fi < 0 || fi >= len(m.Faces) {
		return bandGeom{}, false
	}
	if size < 1 {
		size = 1
	}
	ua, ub := UV(p, worldA), UV(p, worldB)
	d := ub.Sub(ua)
	length := d.Len()
	if length < 1e-9 {
		return bandGeom{}, false
	}
	dir := d.Mul(1 / length)
	// Inward is square to the edge, toward the face's centre.
	nrm := geom.Vec2{X: -dir.Y, Y: dir.X}
	if UV(p, m.FaceCentroid(fi)).Sub(ua).Dot(nrm) < 0 {
		nrm = nrm.Mul(-1)
	}
	g := bandGeom{a: ua, dir: dir, nrm: nrm, s0: 0, s1: length, w: float64(size)}

	// Extend an end that reaches the face's border by the band's own width,
	// and let the face-rect clip trim the spill. Two bands meeting at a corner
	// each stop where their edge does, which leaves the miter between them
	// bare — the notch at every corner of the screenshots. Border ends only:
	// an edge that ends *inside* a face — the base of a box unioned onto a
	// plate — has no boundary there to clip against, and an extension there
	// pokes out past the corner as a painted nub (found by screenshot,
	// 2026-08-28, twice).
	rect := FaceRect(m, fi, p)
	near := func(q geom.Vec2) bool {
		pad := g.w + 1
		return q.X <= float64(rect.Min.X)+pad || q.Y <= float64(rect.Min.Y)+pad ||
			q.X >= float64(rect.Max.X)-pad || q.Y >= float64(rect.Max.Y)-pad
	}
	if near(ua) {
		g.s0 = -g.w
	}
	if near(ub) {
		g.s1 = length + g.w
	}

	lo := geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}
	hi := geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, c := range g.corners() {
		lo.X, lo.Y = math.Min(lo.X, c.X), math.Min(lo.Y, c.Y)
		hi.X, hi.Y = math.Max(hi.X, c.X), math.Max(hi.Y, c.Y)
	}
	g.bbox = image.Rect(
		int(math.Floor(lo.X)), int(math.Floor(lo.Y)),
		int(math.Ceil(hi.X)), int(math.Ceil(hi.Y)))
	return g, true
}

// corners is the band rectangle's four corners in texel space.
func (g *bandGeom) corners() [4]geom.Vec2 {
	at := func(s, u float64) geom.Vec2 {
		return g.a.Add(g.dir.Mul(s)).Add(g.nrm.Mul(u))
	}
	return [4]geom.Vec2{at(g.s0, 0), at(g.s1, 0), at(g.s1, g.w), at(g.s0, g.w)}
}

// covers reports whether a texel's unit square genuinely overlaps the band —
// separating axes over the square's two and the band's two. Strictly: a
// square that only touches the boundary takes no paint, which is what keeps a
// lattice-aligned band exactly as many texels wide as asked.
func (g *bandGeom) covers(t image.Point) bool {
	const eps = 1e-6
	// The square in the band's own coordinates: project its corners on dir
	// and nrm and compare against [s0,s1] x [0,w].
	sMin, sMax := math.Inf(1), math.Inf(-1)
	uMin, uMax := math.Inf(1), math.Inf(-1)
	for _, c := range [4]geom.Vec2{
		{X: float64(t.X), Y: float64(t.Y)},
		{X: float64(t.X) + 1, Y: float64(t.Y)},
		{X: float64(t.X) + 1, Y: float64(t.Y) + 1},
		{X: float64(t.X), Y: float64(t.Y) + 1},
	} {
		rel := c.Sub(g.a)
		s, u := rel.Dot(g.dir), rel.Dot(g.nrm)
		sMin, sMax = math.Min(sMin, s), math.Max(sMax, s)
		uMin, uMax = math.Min(uMin, u), math.Max(uMax, u)
	}
	if math.Min(sMax, g.s1)-math.Max(sMin, g.s0) <= eps {
		return false
	}
	if math.Min(uMax, g.w)-math.Max(uMin, 0) <= eps {
		return false
	}
	// The band's corners against the square's own axes.
	xMin, xMax := math.Inf(1), math.Inf(-1)
	yMin, yMax := math.Inf(1), math.Inf(-1)
	for _, c := range g.corners() {
		xMin, xMax = math.Min(xMin, c.X), math.Max(xMax, c.X)
		yMin, yMax = math.Min(yMin, c.Y), math.Max(yMax, c.Y)
	}
	if math.Min(xMax, float64(t.X)+1)-math.Max(xMin, float64(t.X)) <= eps {
		return false
	}
	if math.Min(yMax, float64(t.Y)+1)-math.Max(yMin, float64(t.Y)) <= eps {
		return false
	}
	return true
}

// softAt is the soft brush's coverage for a texel of the band: full through
// the middle, fading toward both long sides, the same shape a soft dab gives
// a stroke.
func (g *bandGeom) softAt(t image.Point) float64 {
	centre := geom.Vec2{X: float64(t.X) + 0.5, Y: float64(t.Y) + 0.5}
	u := centre.Sub(g.a).Dot(g.nrm)
	r := g.w/2 + 0.5
	d := math.Abs(u - g.w/2)
	if d >= r {
		return 0
	}
	core := r * SoftCore
	if d <= core {
		return 1
	}
	return (r - d) / (r - core)
}

// EdgeEndsOf returns an edge's two world positions, and whether the index
// names a real edge of the mesh.
func EdgeEndsOf(m *mesh.Mesh, edge int) (geom.Vec3, geom.Vec3, bool) {
	if m == nil {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	t := m.Topo()
	if edge < 0 || edge >= len(t.Edges) {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	e := t.Edges[edge]
	if e.A < 0 || e.A >= len(m.Verts) || e.B < 0 || e.B >= len(m.Verts) {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	return m.Verts[e.A], m.Verts[e.B], true
}

// FacesOfEdge lists the faces that meet along an edge, without repeats.
func FacesOfEdge(m *mesh.Mesh, edge int) []int {
	if m == nil {
		return nil
	}
	t := m.Topo()
	if edge < 0 || edge >= len(t.Edges) {
		return nil
	}
	var out []int
	for _, use := range t.Edges[edge].Uses {
		dup := false
		for _, had := range out {
			if had == use.Face {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, use.Face)
		}
	}
	return out
}

// EdgeIsCrease reports whether the faces along an edge meet at more than the
// given angle, in degrees. It is what "only the corners" means when a whole
// body's edges are offered at once: the flat joins inside a face's plane are
// not edges anybody wants a line along.
func EdgeIsCrease(m *mesh.Mesh, edge int, degrees float64) bool {
	faces := FacesOfEdge(m, edge)
	if len(faces) < 2 {
		return true // a boundary edge is always worth drawing
	}
	a, b := m.FaceNormal(faces[0]), m.FaceNormal(faces[1])
	d := a.Dot(b)
	if d > 1 {
		d = 1
	}
	if d < -1 {
		d = -1
	}
	return math.Acos(d)*180/math.Pi >= degrees
}

// EdgeChain returns the run of edges that continue straight through edge's
// endpoints, edge included (the user's report, 2026-08-28: "it skipped this
// end").
//
// A boundary that history has split at a vertex — a union seam, a fold chord
// landing on it — is several topology edges but one line to the eye, and a
// person who clicks a line means the line. The chain walks out of each
// endpoint while exactly one other edge leaves the vertex within maxDeg of
// straight ahead; a real corner offers none and a junction offers several,
// and both stop the walk. Deliberately no crease check on the continuation:
// half a line whose face has leaned until the crease went shallow is still
// the same line.
func EdgeChain(m *mesh.Mesh, edge int, maxDeg float64) []int {
	if m == nil {
		return nil
	}
	t := m.Topo()
	if edge < 0 || edge >= len(t.Edges) {
		return nil
	}
	minDot := math.Cos(maxDeg * math.Pi / 180)

	// next is the unique straight continuation of cur out of vertex v, or -1.
	next := func(cur, v int) int {
		e := t.Edges[cur]
		from := e.A
		if v == e.A {
			from = e.B
		}
		dir, ok := m.Verts[v].Sub(m.Verts[from]).NormalizeOK()
		if !ok {
			return -1
		}
		best := -1
		for i := range t.Edges {
			if i == cur {
				continue
			}
			o := t.Edges[i]
			var far int
			switch v {
			case o.A:
				far = o.B
			case o.B:
				far = o.A
			default:
				continue
			}
			cont, ok := m.Verts[far].Sub(m.Verts[v]).NormalizeOK()
			if !ok || cont.Dot(dir) < minDot {
				continue
			}
			if best >= 0 {
				return -1 // two candidates: a junction, not a line
			}
			best = i
		}
		return best
	}

	chain := []int{edge}
	seen := map[int]bool{edge: true}
	for _, start := range [2]int{t.Edges[edge].A, t.Edges[edge].B} {
		cur, v := edge, start
		for {
			n := next(cur, v)
			if n < 0 || seen[n] {
				break
			}
			seen[n] = true
			chain = append(chain, n)
			e := t.Edges[n]
			if e.A == v {
				v = e.B
			} else {
				v = e.A
			}
			cur = n
		}
	}
	return chain
}
