package csg

import (
	"encoding/json"
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"testing"
)

func TestRepeatedChamferKeepsNewEdgesSelectable(t *testing.T) {
	m := mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	first, err := Chamfer(m, []int{0}, .75)
	if err != nil {
		t.Fatal(err)
	}
	for ei, e := range first.Mesh.Topo().Edges {
		angle := math.Acos(first.Mesh.FaceNormal(e.Uses[0].Face).Dot(first.Mesh.FaceNormal(e.Uses[1].Face))) * 180 / math.Pi
		if angle < 44 || angle > 46 {
			continue
		}
		second, err := Chamfer(first.Mesh, []int{ei}, .25)
		if err != nil {
			t.Fatal(err)
		}
		shallow := 0
		for j, edge := range second.Mesh.Topo().Edges {
			a := math.Acos(math.Max(-1, math.Min(1, second.Mesh.FaceNormal(edge.Uses[0].Face).Dot(second.Mesh.FaceNormal(edge.Uses[1].Face))))) * 180 / math.Pi
			if a > 1 && a < 25 {
				shallow++
				if second.Mesh.ClassifyEdge(j) == mesh.EdgeSmooth {
					t.Errorf("second chamfer hid edge %d at %.2f degrees", j, a)
				}
				if _, err := Chamfer(second.Mesh, []int{j}, .125); err != nil {
					t.Errorf("third chamfer on edge %d: %v", j, err)
				}
			}
		}
		if shallow == 0 {
			t.Fatal("no repeated bevel edges exercised")
		}
		// Match a document saved by the earlier build, which only wrote lineage.
		legacy := second.Mesh.Clone()
		for i := range legacy.Faces {
			legacy.Faces[i].KeepEdges = false
		}
		data, err := json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		var loaded mesh.Mesh
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		if len(loaded.DrawnEdges()) != len(second.Mesh.DrawnEdges()) {
			t.Fatal("saved legacy chamfer lost editable edges")
		}
	}
}

func TestConstructionPreviewDoesNotAcquireAuthoredEdges(t *testing.T) {
	m := mesh.Box(geom.Vec3{X: 10, Y: 10, Z: 10}, geom.Vec3{X: 12, Y: 12, Z: 12}, 1)
	tool := mesh.NGonPrism(geom.PlaneFrame(geom.PlaneTop), 2.5, 16, 6, 0)
	r, err := Boolean(Union, m, tool)
	if err != nil {
		t.Fatal(err)
	}
	smooth := 0
	for ei, e := range r.Mesh.Topo().Edges {
		a := math.Acos(math.Max(-1, math.Min(1, r.Mesh.FaceNormal(e.Uses[0].Face).Dot(r.Mesh.FaceNormal(e.Uses[1].Face))))) * 180 / math.Pi
		if a > 1 && a < 25 {
			smooth++
			if r.Mesh.ClassifyEdge(ei) != mesh.EdgeSmooth {
				t.Fatal("extrude preview received chamfer-only edge marking")
			}
		}
	}
	if smooth != 16 {
		t.Fatal("missing faceted preview sides", smooth)
	}
}
