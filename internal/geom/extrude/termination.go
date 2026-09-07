package extrude

import (
	"fmt"
	"math"
	"modeler/internal/geom/mesh"
)

// Intersect each side-wall generator with the target plane. This preserves
// planar side walls, holes and draft while giving a sloped face a sloped cap.
func terminateShell(m *mesh.Mesh, start int, rings []ring, p Params) error {
	if p.Dir == Symmetric || len(rings) != 2 {
		return fmt.Errorf("up-to targets require a one-way extrusion")
	}
	n := (len(m.Verts) - start) / 2
	near, far := start, start+n
	if p.Dir == Reverse {
		near, far = far, near
	}
	normal := p.EndNormal
	if normal.Len() < 1e-9 {
		return fmt.Errorf("target plane has no normal")
	}
	for i := 0; i < n; i++ {
		base := m.Verts[near+i]
		run := m.Verts[far+i].Sub(base)
		den := normal.Dot(run)
		if math.Abs(den) < 1e-9 {
			return fmt.Errorf("target face is parallel to an extrusion side")
		}
		ratio := normal.Dot(p.EndPoint.Sub(base)) / den
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 1e-8 {
			return fmt.Errorf("target must lie beyond the entire profile")
		}
		m.Verts[far+i] = base.Add(run.Mul(ratio))
	}
	return nil
}
