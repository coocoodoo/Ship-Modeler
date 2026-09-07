package csg

import (
	"fmt"
	"math"
	"sort"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Chamfer builds equal offsets along the two planar faces of each edge.
// Tools are local wedges, bounded by the edge's end faces; a distant part of
// a concave body must never be sliced by an infinite chamfer plane.
// The target and its paint are untouched. The result is safe to preview.
func Chamfer(target *mesh.Mesh, edges []int, distance float64) (Result, error) {
	if target == nil || len(edges) == 0 {
		return Result{}, fmt.Errorf("select at least one solid edge")
	}
	if math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 1.0/geom.Unit {
		return Result{}, fmt.Errorf("enter a positive chamfer distance")
	}
	if err := mesh.Validate(target); err != nil {
		return Result{}, fmt.Errorf("chamfer needs a closed solid: %w", err)
	}
	var cuts, fills []*mesh.Mesh
	seen := map[int]bool{}
	for _, ei := range edges {
		if seen[ei] {
			continue
		}
		seen[ei] = true
		tool, concave, err := chamferWedge(target, ei, distance)
		if err != nil {
			return Result{}, fmt.Errorf("edge %d: %w", ei+1, err)
		}
		if concave {
			fills = append(fills, tool)
		} else {
			cuts = append(cuts, tool)
		}
	}
	result := Result{Mesh: target}
	var err error
	if len(fills) > 0 {
		result, err = Boolean(Union, result.Mesh, fills...)
	}
	if err == nil && len(cuts) > 0 {
		result, err = Boolean(Subtract, result.Mesh, cuts...)
	}
	if err != nil {
		return Result{}, err
	}
	if result.Empty || mesh.Check(result.Mesh).Shells != mesh.Check(target).Shells {
		return Result{}, fmt.Errorf("that distance removes or splits the solid — use a smaller distance")
	}
	if (len(fills) == 0 || len(cuts) == 0) && math.Abs(mesh.Volume(result.Mesh)-mesh.Volume(target)) < geom.WeldDist*geom.WeldDist*geom.WeldDist {
		return Result{}, fmt.Errorf("that distance is too small to form a chamfer")
	}
	return result, nil
}

type chamferPlane struct {
	n geom.Vec3
	d float64
}

func chamferWedge(m *mesh.Mesh, ei int, distance float64) (*mesh.Mesh, bool, error) {
	t := m.Topo()
	if ei < 0 || ei >= len(t.Edges) {
		return nil, false, fmt.Errorf("the edge is no longer there")
	}
	e := t.Edges[ei]
	if !e.Manifold() {
		return nil, false, fmt.Errorf("the edge must join two faces")
	}
	a, b := m.Verts[e.A], m.Verts[e.B]
	axis := b.Sub(a).Normalize()
	var n, inward [2]geom.Vec3
	for i, use := range e.Uses {
		if m.Faces[use.Face].NonPlanar || m.Planarity(use.Face) > geom.PlanarDist {
			return nil, false, fmt.Errorf("chamfer needs two flat faces")
		}
		n[i] = m.FaceNormal(use.Face)
		dir := axis
		if !use.Forward {
			dir = dir.Neg()
		}
		inward[i] = n[i].Cross(dir).Normalize()
		room := chamferFaceRoom(m, use.Face, a.Add(b).Mul(.5), inward[i])
		if distance >= room-geom.WeldDist {
			return nil, false, fmt.Errorf("the distance reaches the far side of a face — use less than %.3g u", room)
		}
	}
	turn := inward[0].Dot(n[1])
	if math.Abs(turn) < 1e-6 {
		return nil, false, fmt.Errorf("select a sharp edge between two different face planes")
	}
	concave := turn > 0

	// Incident end planes supply the miter on slanted caps. A collinear split
	// with no cap is bounded at its endpoint, so selecting the full edge chain
	// joins its wedges without extending into an unrelated edge.
	var caps []chamferPlane
	margin := distance + geom.WeldDist
	for end, vi := range []int{e.A, e.B} {
		found := false
		for _, fi := range t.VertFaces[vi] {
			if fi == e.Uses[0].Face || fi == e.Uses[1].Face {
				continue
			}
			normal, d := m.FacePlane(fi)
			along := normal.Dot(axis)
			if (end == 0 && along >= -1e-6) || (end == 1 && along <= 1e-6) {
				continue
			}
			if m.Planarity(fi) > geom.PlanarDist {
				return nil, false, fmt.Errorf("an end face is not flat")
			}
			caps = append(caps, chamferPlane{normal, d})
			for _, v := range inward {
				margin = math.Max(margin, math.Abs(normal.Dot(v)*distance/along)+distance)
			}
			found = true
		}
		if !found {
			normal := axis
			if end == 0 {
				normal = normal.Neg()
			}
			caps = append(caps, chamferPlane{normal, normal.Dot(m.Verts[vi])})
		}
	}
	start, finish := a.Sub(axis.Mul(margin)), b.Add(axis.Mul(margin))
	verts := []geom.Vec3{start, start.Add(inward[0].Mul(distance)), start.Add(inward[1].Mul(distance)), finish, finish.Add(inward[0].Mul(distance)), finish.Add(inward[1].Mul(distance))}
	center := start.Add(finish).Mul(.5).Add(inward[0].Add(inward[1]).Mul(distance / 3))
	var polys [][]geom.Vec3
	for _, indices := range [][]int{{0, 1, 2}, {3, 5, 4}, {0, 3, 4, 1}, {1, 4, 5, 2}, {2, 5, 3, 0}} {
		p := make([]geom.Vec3, len(indices))
		for i, vi := range indices {
			p[i] = verts[vi]
		}
		if p[1].Sub(p[0]).Cross(p[2].Sub(p[0])).Dot(p[0].Sub(center)) < 0 {
			reverseChamferLoop(p)
		}
		polys = append(polys, p)
	}
	for _, cap := range caps {
		polys = clipChamferTool(polys, cap)
	}
	out := &mesh.Mesh{}
	for _, p := range polys {
		loop := []int{}
		for _, v := range p {
			index := -1
			for i, existing := range out.Verts {
				if existing.Dist(v) < 1e-7 {
					index = i
					break
				}
			}
			if index < 0 {
				index = len(out.Verts)
				out.Verts = append(out.Verts, v)
			}
			if len(loop) == 0 || loop[len(loop)-1] != index {
				loop = append(loop, index)
			}
		}
		if len(loop) > 1 && loop[0] == loop[len(loop)-1] {
			loop = loop[:len(loop)-1]
		}
		if len(loop) >= 3 {
			out.Faces = append(out.Faces, mesh.Face{ID: mesh.MakeFaceUID(0, uint32(len(out.Faces)+1)), Loops: [][]int{loop}, KeepEdges: true})
		}
	}
	if err := mesh.Validate(out); err != nil {
		return nil, false, fmt.Errorf("the chamfer does not fit these end faces — reduce the distance")
	}
	return out, concave, nil
}

// Distance to the first boundary encountered inside a face, including holes.
func chamferFaceRoom(m *mesh.Mesh, fi int, p, dir geom.Vec3) float64 {
	f := m.FaceFrame(fi)
	q, v := f.ToLocal(p), geom.Vec2{X: dir.Dot(f.U), Y: dir.Dot(f.V)}
	room := math.Inf(1)
	for _, loop := range m.Faces[fi].Loops {
		for i, vi := range loop {
			a, b := f.ToLocal(m.Verts[vi]), f.ToLocal(m.Verts[loop[(i+1)%len(loop)]])
			s := b.Sub(a)
			denom := v.CrossZ(s)
			if math.Abs(denom) < 1e-9 {
				continue
			}
			d := a.Sub(q).CrossZ(s) / denom
			u := a.Sub(q).CrossZ(v) / denom
			if d > geom.WeldDist && u >= -1e-8 && u <= 1+1e-8 {
				room = math.Min(room, d)
			}
		}
	}
	return room
}

func reverseChamferLoop(p []geom.Vec3) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}

func clipChamferTool(polys [][]geom.Vec3, plane chamferPlane) [][]geom.Vec3 {
	outside := false
	for _, p := range polys {
		for _, v := range p {
			if plane.n.Dot(v)-plane.d > 1e-8 {
				outside = true
			}
		}
	}
	if !outside {
		return polys
	}
	var out [][]geom.Vec3
	var rim []geom.Vec3
	for _, p := range polys {
		var clipped []geom.Vec3
		for i, a := range p {
			b := p[(i+1)%len(p)]
			da, db := plane.n.Dot(a)-plane.d, plane.n.Dot(b)-plane.d
			if da <= 1e-8 {
				clipped = append(clipped, a)
			}
			if (da < -1e-8 && db > 1e-8) || (da > 1e-8 && db < -1e-8) {
				v := a.Add(b.Sub(a).Mul(da / (da - db)))
				clipped = append(clipped, v)
			}
		}
		if len(clipped) >= 3 {
			out = append(out, clipped)
			for _, v := range clipped {
				if math.Abs(plane.n.Dot(v)-plane.d) > 1e-7 {
					continue
				}
				unique := true
				for _, r := range rim {
					if r.Dist(v) < 1e-7 {
						unique = false
						break
					}
				}
				if unique {
					rim = append(rim, v)
				}
			}
		}
	}
	if len(rim) >= 3 {
		center := geom.Vec3{}
		for _, p := range rim {
			center = center.Add(p)
		}
		center = center.Mul(1 / float64(len(rim)))
		u := rim[0].Sub(center).Normalize()
		v := plane.n.Cross(u)
		sort.Slice(rim, func(i, j int) bool {
			a, b := rim[i].Sub(center), rim[j].Sub(center)
			return math.Atan2(a.Dot(v), a.Dot(u)) < math.Atan2(b.Dot(v), b.Dot(u))
		})
		out = append(out, rim)
	}
	return out
}
