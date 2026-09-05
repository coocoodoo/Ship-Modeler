package render

import (
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"testing"
)

// Screen-space shading must not add triangles or change model geometry.
func TestConcaveMeshNeedsNoShadingTessellation(t *testing.T) {
	m := stepMesh()
	g := BuildBodyGPU(m)
	if g.TriCount != len(m.Triangulate()) {
		t.Fatal("AO still adds render triangles")
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

func vertAt(g *BodyGPU, i int) geom.Vec3 {
	return geom.Vec3{
		X: float64(g.positions[i*3]),
		Y: float64(g.positions[i*3+1]),
		Z: float64(g.positions[i*3+2]),
	}
}

func stepMesh() *mesh.Mesh {
	// A plate with a tower on its back half, stitched watertight: the ledge
	// top meets the step wall along y=1, z=0 - a genuine concave junction.
	m := &mesh.Mesh{
		Verts: []geom.Vec3{
			{X: -4, Y: 0, Z: -4}, {X: 4, Y: 0, Z: -4}, {X: 4, Y: 0, Z: 4}, {X: -4, Y: 0, Z: 4},
			{X: -4, Y: 1, Z: -4}, {X: 4, Y: 1, Z: -4},
			{X: 4, Y: 1, Z: 0}, {X: -4, Y: 1, Z: 0},
			{X: -4, Y: 1, Z: 4}, {X: 4, Y: 1, Z: 4},
			{X: -4, Y: 4, Z: -4}, {X: 4, Y: 4, Z: -4}, {X: 4, Y: 4, Z: 0}, {X: -4, Y: 4, Z: 0},
		},
		Faces: []mesh.Face{
			{ID: mesh.MakeFaceUID(1, 1), Loops: [][]int{{0, 1, 2, 3}}},            // bottom
			{ID: mesh.MakeFaceUID(1, 2), Loops: [][]int{{8, 9, 6, 7}}},            // ledge top
			{ID: mesh.MakeFaceUID(1, 3), Loops: [][]int{{7, 6, 12, 13}}},          // step wall +z
			{ID: mesh.MakeFaceUID(1, 4), Loops: [][]int{{10, 13, 12, 11}}},        // tower top
			{ID: mesh.MakeFaceUID(1, 5), Loops: [][]int{{3, 2, 9, 8}}},            // front +z
			{ID: mesh.MakeFaceUID(1, 6), Loops: [][]int{{0, 4, 10, 11, 5, 1}}},    // back -z
			{ID: mesh.MakeFaceUID(1, 7), Loops: [][]int{{0, 3, 8, 7, 13, 10, 4}}}, // left -x
			{ID: mesh.MakeFaceUID(1, 8), Loops: [][]int{{1, 5, 11, 12, 6, 9, 2}}}, // right +x
		},
	}
	return m
}
