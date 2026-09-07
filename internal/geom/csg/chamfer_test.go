package csg

import (
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"testing"
)

func TestChamferBoxEdges(t *testing.T) {
	m := mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	for ei := range m.Topo().Edges {
		r, err := Chamfer(m, []int{ei}, .5)
		if err != nil {
			t.Fatalf("edge %d: %v", ei, err)
		}
		if math.Abs(mesh.Volume(r.Mesh)-63.5) > .001 || len(r.Mesh.Faces) != 7 {
			t.Fatalf("edge %d: volume %g faces %d", ei, mesh.Volume(r.Mesh), len(r.Mesh.Faces))
		}
	}
	all := []int{}
	for i := range m.Topo().Edges {
		all = append(all, i)
	}
	r, err := Chamfer(m, all, .5)
	if err != nil {
		t.Fatal(err)
	}
	if mesh.Volume(r.Mesh) >= 60 || mesh.Volume(r.Mesh) < 57 {
		t.Fatalf("all edges: %g", mesh.Volume(r.Mesh))
	}
	if len(m.Faces) != 6 || mesh.Volume(m) != 64 {
		t.Fatal("modified input")
	}
}

func TestChamferSlantedEndAndConcaveEdge(t *testing.T) {
	m := mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	for i := range m.Verts {
		m.Verts[i].X += m.Verts[i].Z * .5
	}
	m.InvalidateCaches()
	for ei := range m.Topo().Edges {
		if _, err := Chamfer(m, []int{ei}, .25); err != nil {
			t.Fatalf("slanted edge %d: %v", ei, err)
		}
	}
	m = mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	cut := mesh.Box(geom.Vec3{X: 2, Y: 2, Z: -1}, geom.Vec3{X: 5, Y: 5, Z: 5}, 2)
	l, err := Boolean(Subtract, m, cut)
	if err != nil {
		t.Fatal(err)
	}
	for ei, e := range l.Mesh.Topo().Edges {
		a, b := l.Mesh.Verts[e.A], l.Mesh.Verts[e.B]
		if a.X == 2 && a.Y == 2 && b.X == 2 && b.Y == 2 {
			r, err := Chamfer(l.Mesh, []int{ei}, .5)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(mesh.Volume(r.Mesh)-48.5) > .001 {
				t.Fatalf("inside chamfer volume %g", mesh.Volume(r.Mesh))
			}
			return
		}
	}
	t.Fatal("missing concave edge")
}

func TestChamferRejectsInvalidDistance(t *testing.T) {
	m := mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	for _, d := range []float64{0, -1, 4, 100, math.NaN(), math.Inf(1)} {
		if _, err := Chamfer(m, []int{0}, d); err == nil {
			t.Fatalf("accepted %g", d)
		}
	}
	if _, err := Chamfer(m, []int{999}, .5); err == nil {
		t.Fatal("invalid edge accepted")
	}
}

func TestChamferLocalityAndFacetedRim(t *testing.T) {
	m := mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	other := mesh.Box(geom.Vec3{Z: 8}, geom.Vec3{X: 4, Y: 4, Z: 12}, 2)
	joined, err := Boolean(Union, m, other)
	if err != nil {
		t.Fatal(err)
	}
	ei := -1
	for i, e := range joined.Mesh.Topo().Edges {
		a, b := joined.Mesh.Verts[e.A], joined.Mesh.Verts[e.B]
		if a.X == 0 && b.X == 0 && a.Y == 0 && b.Y == 0 && a.Z < 5 && b.Z < 5 {
			ei = i
			break
		}
	}
	r, err := Chamfer(joined.Mesh, []int{ei}, .5)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(mesh.Volume(r.Mesh)-127.5) > .001 {
		t.Fatal("chamfer cut unrelated shell", mesh.Volume(r.Mesh))
	}
	m = mesh.NGonPrism(geom.PlaneFrame(geom.PlaneTop), 3, 16, 4, 1)
	var rim []int
	for i, e := range m.Topo().Edges {
		if math.Abs(m.Verts[e.A].Y-4) < 1e-6 && math.Abs(m.Verts[e.B].Y-4) < 1e-6 {
			rim = append(rim, i)
		}
	}
	if len(rim) != 16 {
		t.Fatal("missing rim")
	}
	if _, err := Chamfer(m, rim, .25); err != nil {
		t.Fatal("faceted rim:", err)
	}
}
