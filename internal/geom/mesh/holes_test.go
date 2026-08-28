package mesh

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// Faces with holes (2026-08-27).
//
// A pocket cut into a flat face leaves that face pierced: an outer boundary
// and a hole. Every such face was being triangulated as though the hole were
// not there, which drew a lid straight across the opening — so a pocket looked
// exactly like an untouched face, and "subtract is not working" was a
// perfectly reasonable thing to conclude from the picture.
//
// The material really was gone; only the drawing was wrong. That is why these
// tests measure area rather than counting triangles: a wrong triangulation
// produces exactly the right *number* of triangles.

// triArea sums the unsigned area of a triangulation, which is what a renderer
// actually paints. Signed area hides the fault: a fan across a bridged hole
// sums to the correct total while covering the hole with overlapping,
// oppositely wound triangles.
func triArea(m *Mesh, tris []Tri) float64 {
	var a float64
	for _, t := range tris {
		p, q, r := m.Verts[t.A], m.Verts[t.B], m.Verts[t.C]
		a += q.Sub(p).Cross(r.Sub(p)).Len() / 2
	}
	return a
}

// A slab with a pocket cut into one face: that face is pierced, and the
// triangles must cover the material and not the hole.
func TestAPiercedFaceLeavesItsHoleOpen(t *testing.T) {
	slab := Box(geom.Vec3{X: -9, Y: -4, Z: -4}, geom.Vec3{X: 9, Y: 4, Z: 4}, 1)
	// The pocket's opening on the +Z face: 12 x 4, inside the face's 18 x 8.
	pierced := -1
	for i := range slab.Faces {
		if slab.FaceNormal(i).Z > 0.9 {
			pierced = i
		}
	}
	if pierced < 0 {
		t.Fatal("no +Z face on a box")
	}

	// Add the hole by hand, so this tests the triangulator and not the solver.
	hole := addSquareLoop(slab, 4, 6, 2)
	slab.Faces[pierced].Loops = append(slab.Faces[pierced].Loops, hole)
	slab.InvalidateCaches()

	if got := len(slab.Faces[pierced].Loops); got != 2 {
		t.Fatalf("the face has %d loops, want an outer and a hole", got)
	}

	const outer, cut = 18.0 * 8.0, 12.0 * 4.0
	want := outer - cut
	got := triArea(slab, slab.FaceTris(pierced))
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("the pierced face triangulates to %.2f of area, want %.2f\n"+
			"  %.2f means the hole was covered over; the pocket would be invisible",
			got, want, outer)
	}
}

// Every triangle has to wind the same way as the face it belongs to. A fan
// across a bridged hole produces backwards ones whose signed areas cancel, so
// only checking the total would pass.
func TestAPiercedFaceWindsEveryTriangleTheSameWay(t *testing.T) {
	slab := Box(geom.Vec3{X: -9, Y: -4, Z: -4}, geom.Vec3{X: 9, Y: 4, Z: 4}, 1)
	pierced := -1
	for i := range slab.Faces {
		if slab.FaceNormal(i).Z > 0.9 {
			pierced = i
		}
	}
	slab.Faces[pierced].Loops = append(slab.Faces[pierced].Loops, addSquareLoop(slab, 4, 6, 2))
	slab.InvalidateCaches()

	n := slab.FaceNormal(pierced)
	for i, tr := range slab.FaceTris(pierced) {
		p, q, r := slab.Verts[tr.A], slab.Verts[tr.B], slab.Verts[tr.C]
		if d := q.Sub(p).Cross(r.Sub(p)).Dot(n); d <= 0 {
			t.Errorf("triangle %d faces backwards (%.2f) — the fan fallback ran", i, d)
		}
	}
}

// A face with no hole is unaffected, convex or not.
func TestOrdinaryFacesStillTriangulate(t *testing.T) {
	slab := Box(geom.Vec3{X: -3, Y: -2, Z: -1}, geom.Vec3{X: 3, Y: 2, Z: 1}, 1)
	for i := range slab.Faces {
		got := triArea(slab, slab.FaceTris(i))
		want := slab.FaceArea(i)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("face %d triangulates to %.4f, want its area %.4f", i, got, want)
		}
	}
}

// addSquareLoop appends four vertices making a hole loop on the +Z face, wound
// clockwise seen from outside so it reads as a hole.
func addSquareLoop(m *Mesh, z, hx, hy float64) []int {
	pts := []geom.Vec3{
		{X: -hx, Y: -hy, Z: z},
		{X: -hx, Y: hy, Z: z},
		{X: hx, Y: hy, Z: z},
		{X: hx, Y: -hy, Z: z},
	}
	loop := make([]int, len(pts))
	for i, p := range pts {
		loop[i] = len(m.Verts)
		m.Verts = append(m.Verts, p)
	}
	return loop
}
