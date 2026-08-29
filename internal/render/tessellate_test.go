package render

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Shading tessellation (the user's report, 2026-08-28): "I really don't see
// ambient occlusion."
//
// It was baking correctly and had nowhere to land. Openness is a corner value
// interpolated across the face, and a CAD face is a handful of big flat
// polygons with no interior corners — so on the inside floor of a box, all
// four corners are equally occluded and the interpolation of four equal
// numbers is a constant. The face came out uniformly dimmer, which reads as a
// darker shade rather than as a shadow in a corner. These tests pin the fix:
// the render mesh gets interior vertices for the falloff to live on.

func TestAFaceWithSomethingInFrontOfItGetsInteriorVertices(t *testing.T) {
	m := stepMesh()
	g := BuildBodyGPU(m)

	// The step's ledge is 8 x 4 units with the tower standing on it. Before
	// this it had four corners and two triangles, and every point on it took
	// its shading from those four — which is why the whole ledge came out one
	// flat tone instead of dark against the wall.
	fi := faceAlong(m, geom.Vec3{Y: 1})
	interior := 0
	lo, hi := faceBoundsIn(m, fi)
	for i := 0; i < g.VertCount; i++ {
		if g.vertFace[i] != fi {
			continue
		}
		p := vertAt(g, i)
		if p.X > lo.X+1e-6 && p.X < hi.X-1e-6 && p.Z > lo.Z+1e-6 && p.Z < hi.Z-1e-6 {
			interior++
		}
	}
	if interior == 0 {
		t.Fatal("the ledge still has no vertex strictly inside it, so any " +
			"corner falloff across it can only be a straight ramp between corners")
	}
}

func TestTessellationLeavesEveryVertexOnItsOwnFace(t *testing.T) {
	m := stepMesh()
	g := BuildBodyGPU(m)
	for i := 0; i < g.VertCount; i++ {
		fi := g.vertFace[i]
		p, n := vertAt(g, i), m.FaceNormal(fi)
		// Distance from the face's plane: subdividing must not move the surface.
		if d := math.Abs(p.Sub(m.FaceCentroid(fi)).Dot(n)); d > 1e-9 {
			t.Fatalf("vertex %d sits %v off the plane of the face it belongs to", i, d)
		}
	}
}

// A subdivided face must still cover exactly the same surface — same area, no
// more and no less, or the tessellation is a modelling change in disguise.
func TestTessellationCoversTheSameSurface(t *testing.T) {
	m := stepMesh()
	g := BuildBodyGPU(m)
	var got float64
	for i := 0; i < g.VertCount; i += 3 {
		a, b, c := vertAt(g, i), vertAt(g, i+1), vertAt(g, i+2)
		got += b.Sub(a).Cross(c.Sub(a)).Len() / 2
	}
	var want float64
	for _, tr := range m.Triangulate() {
		a, b, c := m.Verts[tr.A], m.Verts[tr.B], m.Verts[tr.C]
		want += b.Sub(a).Cross(c.Sub(a)).Len() / 2
	}
	if math.Abs(got-want) > 1e-6*want {
		t.Errorf("the tessellated surface is %v, the mesh is %v", got, want)
	}
}

// The other half of the bargain. Subdividing every face of every body would
// have made an ordinary box cost a hundred times what it did, for shading that
// is uniformly "fully open" — so a face with nothing in front of it is left as
// the two triangles it always was. On a convex body that is every face.
func TestAConvexBodyIsNotSubdividedAtAll(t *testing.T) {
	m := mesh.Box(geom.Vec3{X: -6, Y: 0, Z: -4}, geom.Vec3{X: 6, Y: 4, Z: 4}, 1)
	g := BuildBodyGPU(m)
	if want := len(m.Triangulate()); g.TriCount != want {
		t.Errorf("a plain box renders as %d triangles, want the %d it triangulates to — "+
			"nothing stands in front of any of its faces", g.TriCount, want)
	}
}

// The point of the whole exercise. On the ledge of a step, openness has to
// climb as you walk away from the wall — not sit at one value for the whole
// face, which is what a four-corner face gave.
func TestOpennessClimbsAwayFromAWall(t *testing.T) {
	m := stepMesh()
	g := BuildBodyGPU(m)
	BakeAO(g, m)

	up := geom.Vec3{Y: 1}
	var last byte
	var lastZ float64
	for i, z := range []float64{0.25, 1, 2, 3} {
		got, ok := openness(g, geom.Vec3{X: 0, Y: 1, Z: z}, up)
		if !ok {
			t.Fatalf("no rendered corner near z=%v on the ledge", z)
		}
		if i > 0 && got <= last {
			t.Errorf("openness at z=%v is %d, no lighter than %d at z=%v — "+
				"the falloff away from the wall is flat", z, got, last, lastZ)
		}
		last, lastZ = got, z
	}
}

// faceAlong is in ao_test.go's fixture family; these helpers keep the reads
// above short.
func vertAt(g *BodyGPU, i int) geom.Vec3 {
	return geom.Vec3{
		X: float64(g.positions[i*3]),
		Y: float64(g.positions[i*3+1]),
		Z: float64(g.positions[i*3+2]),
	}
}

func faceBoundsIn(m *mesh.Mesh, fi int) (lo, hi geom.Vec3) {
	lo = geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi = geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, loop := range m.Faces[fi].Loops {
		for _, vi := range loop {
			p := m.Verts[vi]
			lo = geom.Vec3{X: math.Min(lo.X, p.X), Y: math.Min(lo.Y, p.Y), Z: math.Min(lo.Z, p.Z)}
			hi = geom.Vec3{X: math.Max(hi.X, p.X), Y: math.Max(hi.Y, p.Y), Z: math.Max(hi.Z, p.Z)}
		}
	}
	return lo, hi
}

func faceAlong(m *mesh.Mesh, n geom.Vec3) int {
	for i := range m.Faces {
		if m.FaceNormal(i).Dot(n) > 0.999 {
			return i
		}
	}
	panic("no face along that normal")
}
