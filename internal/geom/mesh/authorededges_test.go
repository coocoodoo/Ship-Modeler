package mesh

import (
	"encoding/json"
	"modeler/internal/geom"
	"testing"
)

func TestAuthoredEdgesSurviveSaveAndClone(t *testing.T) {
	m := NGonPrism(geom.PlaneFrame(geom.PlaneTop), 2.5, 16, 6, 1)
	m.Faces[2].KeepEdges = true
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Mesh
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []*Mesh{m.Clone(), &loaded} {
		authored, smooth := 0, 0
		for ei, e := range candidate.Topo().Edges {
			if deg := dihedralDeg(candidate, ei); deg > 1 && deg < 25 {
				if e.Uses[0].Face == 2 || e.Uses[1].Face == 2 {
					authored++
					if candidate.ClassifyEdge(ei) != EdgeCrease {
						t.Fatal("authored shallow boundary hidden")
					}
				} else {
					smooth++
					if candidate.ClassifyEdge(ei) != EdgeSmooth {
						t.Fatal("ordinary cylinder faceting changed")
					}
				}
			}
		}
		if authored != 2 || smooth != 14 {
			t.Fatal(authored, smooth)
		}
	}
}

func TestAuthoredFacesDoNotOutlineCoplanarSeams(t *testing.T) {
	m := seamBox()
	for i := range m.Faces {
		m.Faces[i].KeepEdges = true
	}
	found := false
	for ei := range m.Topo().Edges {
		if dihedralDeg(m, ei) < .0001 {
			found = true
			if m.ClassifyEdge(ei) != EdgeSmooth {
				t.Fatal("coplanar triangulation seam became pickable")
			}
		}
	}
	if !found {
		t.Fatal("no coplanar seam exercised")
	}
}
