package render

import (
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Baked ambient occlusion (the user's request, 2026-08-28).
//
// Simple on purpose: a fixed fan of rays out of each rendered corner, into
// the hemisphere above its face, counting how much nearby geometry blocks
// them. The result is a per-corner openness byte the shader multiplies in —
// no screen-space pass, no noise, and the same answer every build, which is
// what lets goldens pin it.

// openness reads the baked byte for the rendered corner nearest a world
// position on the face with the given normal.
func openness(g *BodyGPU, at, normal geom.Vec3) (byte, bool) {
	best, dist := -1, 1e18
	for i := 0; i < g.VertCount; i++ {
		p := geom.Vec3{
			X: float64(g.positions[i*3]),
			Y: float64(g.positions[i*3+1]),
			Z: float64(g.positions[i*3+2]),
		}
		n := geom.Vec3{
			X: float64(g.normals[i*3]),
			Y: float64(g.normals[i*3+1]),
			Z: float64(g.normals[i*3+2]),
		}
		if n.Dot(normal) < 0.99 {
			continue
		}
		if d := p.Sub(at).LenSq(); d < dist {
			dist, best = d, i
		}
	}
	if best < 0 {
		return 0, false
	}
	return g.colors[best*4+2], true
}

func TestALonePlateIsFullyOpen(t *testing.T) {
	m := mesh.Box(geom.Vec3{X: -4, Y: 0, Z: -4}, geom.Vec3{X: 4, Y: 1, Z: 4}, 1)
	g := BuildBodyGPU(m)
	BakeAO(g, m)
	for i := 0; i < g.VertCount; i++ {
		if got := g.colors[i*4+2]; got < 250 {
			t.Fatalf("corner %d of a lone convex plate bakes openness %d, want ~255 — "+
				"nothing is there to occlude it", i, got)
		}
	}
}

// The user's own shape: a step. The inside corner — where the ledge meets the
// step wall — is the textbook AO case, and the whole reason to have it.
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

func TestTheInsideCornerOfAStepIsDarker(t *testing.T) {
	m := stepMesh()
	if err := mesh.Validate(m); err != nil {
		t.Fatalf("the step fixture is broken: %v", err)
	}
	g := BuildBodyGPU(m)
	BakeAO(g, m)

	up := geom.Vec3{Y: 1}
	// On the ledge top: hugging the wall vs out at the open front edge.
	inner, ok1 := openness(g, geom.Vec3{X: 0, Y: 1, Z: 0}, up)
	outer, ok2 := openness(g, geom.Vec3{X: 0, Y: 1, Z: 4}, up)
	if !ok1 || !ok2 {
		t.Fatal("could not find the ledge corners")
	}
	if inner >= outer {
		t.Errorf("the wall-hugging corner bakes %d and the open edge %d — "+
			"the inside of a step must be darker", inner, outer)
	}
	if outer < 240 {
		t.Errorf("the open front edge bakes %d, want nearly fully open", outer)
	}
	if inner > 220 {
		t.Errorf("the concave junction bakes %d — barely darker than open; "+
			"the corner should read", inner)
	}

	// The wall's own base darkens too, symmetric with the ledge.
	wallN := geom.Vec3{Z: 1}
	base, ok3 := openness(g, geom.Vec3{X: 0, Y: 1, Z: 0}, wallN)
	top, ok4 := openness(g, geom.Vec3{X: 0, Y: 4, Z: 0}, wallN)
	if !ok3 || !ok4 {
		t.Fatal("could not find the wall corners")
	}
	if base >= top {
		t.Errorf("the wall base bakes %d and its top %d — the base sits in the corner", base, top)
	}
}

func TestGeometryBeyondTheRadiusDoesNotDarken(t *testing.T) {
	// Two plates far apart: each bakes as if alone.
	m := mesh.Box(geom.Vec3{X: -2, Y: 0, Z: -2}, geom.Vec3{X: 2, Y: 1, Z: 2}, 1)
	far := mesh.Box(geom.Vec3{X: AORadius * 4, Y: 0, Z: -2},
		geom.Vec3{X: AORadius*4 + 4, Y: 8, Z: 2}, 1)
	mesh.Merge(m, far)
	g := BuildBodyGPU(m)
	BakeAO(g, m)
	got, ok := openness(g, geom.Vec3{X: 2, Y: 1, Z: 0}, geom.Vec3{Y: 1})
	if !ok {
		t.Fatal("no corner found")
	}
	if got < 250 {
		t.Errorf("a tower beyond the AO radius darkened a corner to %d", got)
	}
}

func TestTheBakeIsDeterministic(t *testing.T) {
	a := BuildBodyGPU(stepMesh())
	b := BuildBodyGPU(stepMesh())
	BakeAO(a, stepMesh())
	BakeAO(b, stepMesh())
	if len(a.colors) != len(b.colors) {
		t.Fatal("two identical builds disagree on size")
	}
	for i := range a.colors {
		if a.colors[i] != b.colors[i] {
			t.Fatalf("byte %d differs between two identical bakes", i)
		}
	}
}
