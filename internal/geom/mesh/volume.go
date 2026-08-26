package mesh

import "modeler/internal/geom"

// Volume returns the signed volume of a closed mesh by the divergence theorem:
// the sum of signed tetrahedron volumes from the origin over every triangle
// (SPEC-GEOMETRY §9). Kahan compensation keeps large-coordinate models honest.
//
// A correctly oriented body returns a positive value; tests compare against
// analytic prisms and against the boolean kernel's own property within
// geom.VolumeRelTol.
func Volume(m *Mesh) float64 {
	var sum, comp float64
	for _, t := range m.Triangulate() {
		a, b, c := m.Verts[t.A], m.Verts[t.B], m.Verts[t.C]
		v := a.Dot(b.Cross(c)) / 6
		y := v - comp
		s := sum + y
		comp = (s - sum) - y
		sum = s
	}
	return sum
}

// SurfaceArea sums the absolute area of every face, holes subtracted.
func SurfaceArea(m *Mesh) float64 {
	var sum float64
	for fi := range m.Faces {
		a := m.FaceArea(fi)
		if a < 0 {
			a = -a
		}
		sum += a
	}
	return sum
}

// Centroid returns the volume-weighted centroid of a closed mesh.
func Centroid(m *Mesh) geom.Vec3 {
	var num geom.Vec3
	var den float64
	for _, t := range m.Triangulate() {
		a, b, c := m.Verts[t.A], m.Verts[t.B], m.Verts[t.C]
		v := a.Dot(b.Cross(c)) / 6
		num = num.Add(a.Add(b).Add(c).Mul(v / 4))
		den += v
	}
	if den == 0 {
		return m.AABB().Center()
	}
	return num.Mul(1 / den)
}

// TriangleCount reports how many triangles the mesh renders as, for the tree
// panel's stats line (SPEC-UX §7).
func (m *Mesh) TriangleCount() int {
	n := 0
	for fi := range m.Faces {
		f := &m.Faces[fi]
		total := 0
		for _, loop := range f.Loops {
			total += len(loop)
		}
		if total < 3 {
			continue
		}
		// A polygon with h holes and n total loop vertices triangulates into
		// n + 2h - 2 triangles.
		n += total + 2*len(f.Holes()) - 2
	}
	return n
}
