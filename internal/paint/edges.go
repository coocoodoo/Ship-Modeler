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

// EdgeBand paints a stripe of the brush's width along an edge, on one face,
// and returns the texel rectangle it wrote.
//
// The band is laid *inside* the face rather than centred on the edge. An edge
// is the boundary between two faces, so a brush centred on it spends half its
// width on texels that are not on this face at all — invisible, and half the
// thickness that was asked for. Offsetting inward by half the width gives the
// full band on each face, and the two together read as one line round the
// corner.
func EdgeBand(m *mesh.Mesh, fi int, p *mesh.FacePaint, b Brush, worldA, worldB geom.Vec3) image.Rectangle {
	if m == nil || p == nil || fi < 0 || fi >= len(m.Faces) {
		return image.Rectangle{}
	}
	size := b.Size
	if size < 1 {
		size = 1
	}
	ta, tb := edgeBandLine(m, fi, p, size, worldA, worldB)

	// Nothing may land off the face: the margin around a face's picture exists
	// so a brush can overrun without checking, but paint out there is paint
	// nobody sees, and here it would be the half of the band the user asked
	// for and did not get.
	return strokeClipped(p, b, ta, tb, FaceRect(m, fi, p))
}

// edgeBandLine is where the band's dabs are anchored: the edge, moved into the
// face by half the width and pulled back by the dab's own offset.
func edgeBandLine(m *mesh.Mesh, fi int, p *mesh.FacePaint, size int, worldA, worldB geom.Vec3) (image.Point, image.Point) {
	if size < 1 {
		size = 1
	}

	// The direction from the edge into this face, along the face's own plane.
	// Taken in world space, where the face has a frame and a normal to project
	// against; texel space would need the same work with worse names.
	mid := worldA.Add(worldB).Mul(0.5)
	toCentre := m.FaceCentroid(fi).Sub(mid)
	along, ok := worldB.Sub(worldA).NormalizeOK()
	if ok {
		// Only the part of it square to the edge: the rest slides along the
		// band rather than moving it inward.
		toCentre = toCentre.Sub(along.Mul(toCentre.Dot(along)))
	}
	inward, ok := toCentre.NormalizeOK()
	if !ok {
		// A degenerate face: nothing sensible to offset toward, so lay the
		// band on the edge and let it fall where it may.
		inward = geom.Vec3{}
	}

	// Half the band's width, in world units. FacePaint.Texel is how much world
	// one texel spans, which is what turns a thickness in texels into a
	// distance to move.
	shift := inward.Mul(p.Texel * float64(size) / 2)
	ta := Texel(p, worldA.Add(shift))
	tb := Texel(p, worldB.Add(shift))

	// A dab covers [t, t+size), so its centre sits half a width past its
	// anchor. Pulling the anchor back puts the band's centre on the line just
	// computed instead of past it.
	back := (size - 1) / 2
	return ta.Sub(image.Point{X: back, Y: back}), tb.Sub(image.Point{X: back, Y: back})
}

// strokeClipped is Stroke with every dab confined to a rectangle.
func strokeClipped(p *mesh.FacePaint, b Brush, from, to image.Point, clip image.Rectangle) image.Rectangle {
	size := b.Size
	if size < 1 {
		size = 1
	}
	dirty := image.Rectangle{}
	for _, t := range walk(from, to) {
		for y := t.Y; y < t.Y+size; y++ {
			for x := t.X; x < t.X+size; x++ {
				at := image.Point{X: x, Y: y}
				if !at.In(clip) {
					continue
				}
				coverage := 1.0
				if b.Soft {
					coverage = softCoverage(at, t, size)
				}
				dirty = union(dirty, put(p, b, at, coverage))
			}
		}
	}
	return dirty
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
