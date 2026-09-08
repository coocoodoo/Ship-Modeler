package mesh

import (
	"math"

	"modeler/internal/geom"
)

// Primitive builders. These exist for tests, for the M0 render harness and for
// the boolean acceptance matrix; the real modelling path builds bodies through
// extrude (SPEC-GEOMETRY §5).

// Box returns an axis-aligned box with six quad faces, wound counter-clockwise
// seen from outside. Face sequence numbers start at 1 so FaceUID zero stays
// reserved for "no face".
func Box(min, max geom.Vec3, bodyID uint32) *Mesh {
	b := geom.AABB{Min: min, Max: max}
	corners := b.Corners()
	m := &Mesh{Verts: corners[:]}

	// Corner bit layout matches geom.AABB.Corners: bit0=X, bit1=Y, bit2=Z.
	loops := [6][4]int{
		{4, 5, 7, 6}, // +Z
		{0, 2, 3, 1}, // -Z
		{1, 3, 7, 5}, // +X
		{0, 4, 6, 2}, // -X
		{2, 6, 7, 3}, // +Y
		{0, 1, 5, 4}, // -Y
	}
	for i, l := range loops {
		loop := []int{l[0], l[1], l[2], l[3]}
		m.Faces = append(m.Faces, Face{
			ID:    MakeFaceUID(bodyID, uint32(i+1)),
			Loops: [][]int{loop},
		})
	}
	return m
}

// NGonPrism returns a prism whose cross-section is a regular n-gon inscribed in
// radius r on the given frame, extruded by depth along the frame normal.
// A circle in this app is a regular n-gon (D-06, SPEC-UX §8.3).
func NGonPrism(frame geom.Frame, r float64, segs int, depth float64, bodyID uint32) *Mesh {
	if segs < 3 {
		segs = 3
	}
	m := &Mesh{}
	// Near cap vertices then far cap vertices.
	for i := 0; i < segs; i++ {
		a := 2 * math.Pi * float64(i) / float64(segs)
		p := geom.Vec2{X: r * math.Cos(a), Y: r * math.Sin(a)}
		m.Verts = append(m.Verts, frame.ToWorld(p))
	}
	for i := 0; i < segs; i++ {
		a := 2 * math.Pi * float64(i) / float64(segs)
		p := geom.Vec2{X: r * math.Cos(a), Y: r * math.Sin(a)}
		m.Verts = append(m.Verts, frame.ToWorldAt(p, depth))
	}

	seq := uint32(1)
	next := func() FaceUID { id := MakeFaceUID(bodyID, seq); seq++; return id }

	// Near cap faces away from the extrusion direction, so it winds backwards.
	near := make([]int, segs)
	for i := 0; i < segs; i++ {
		near[i] = segs - 1 - i
	}
	m.Faces = append(m.Faces, Face{ID: next(), Loops: [][]int{near}})

	far := make([]int, segs)
	for i := 0; i < segs; i++ {
		far[i] = segs + i
	}
	m.Faces = append(m.Faces, Face{ID: next(), Loops: [][]int{far}})

	for i := 0; i < segs; i++ {
		j := (i + 1) % segs
		m.Faces = append(m.Faces, Face{
			ID:    next(),
			Loops: [][]int{{i, j, segs + j, segs + i}},
		})
	}

	if depth < 0 {
		FlipAll(m)
	}
	return m
}

// FlipAll reverses every loop, turning a mesh inside out. Used when a
// construction produced negative depth.
func FlipAll(m *Mesh) {
	for fi := range m.Faces {
		for _, loop := range m.Faces[fi].Loops {
			reverse(loop)
		}
	}
	m.InvalidateCaches()
}

// Translate shifts every vertex, keeping paint frames glued to the geometry
// (SPEC-GEOMETRY §8.4).
func Translate(m *Mesh, d geom.Vec3) {
	for i := range m.Verts {
		m.Verts[i] = m.Verts[i].Add(d)
	}
	mapPaintFrames(m, func(f geom.Frame) geom.Frame { return f.Translated(d) })
	m.InvalidateCaches()
}

// Transform applies a rigid motion to the mesh and its paint frames.
func Transform(m *Mesh, x geom.Mat4) {
	for i := range m.Verts {
		m.Verts[i] = x.TransformPoint(m.Verts[i])
	}
	mapPaintFrames(m, func(f geom.Frame) geom.Frame { return f.Transformed(x) })
	m.InvalidateCaches()
}

// Mesh clones share paint with snapshots. Copy the mapping before changing
// it, and transform shared allocations just once even across split faces.
func mapPaintFrames(m *Mesh, transform func(geom.Frame) geom.Frame) {
	mapped := map[*FacePaint]*FacePaint{}
	for fi := range m.Faces {
		if p := m.Faces[fi].Paint; p != nil {
			cp := mapped[p]
			if cp == nil {
				clone := *p
				clone.Frame = transform(p.Frame)
				cp = &clone
				mapped[p] = cp
			}
			m.Faces[fi].Paint = cp
		}
	}
}

// Merge appends src's geometry into dst, renumbering vertex indices. The result
// is a multi-shell mesh, which is legal (SPEC-GEOMETRY §6.5).
// Merge concatenates two meshes into one.
//
// It is a concatenation, not a union: the shells arrive side by side and are
// not joined, welded or checked against each other. If they overlap, or meet
// face to face, the result is a mesh describing two solids in the same place —
// which passes every check in Validate, because each shell is closed and
// manifold on its own and they enclose no shared volume, and which Manifold
// then cannot union onto at all. It answers by doing nothing.
//
// So: use this for shells that are genuinely apart, and csg.Boolean for shells
// that are meant to become one solid. The test scene learned this the hard way
// (PROGRESS, M5).
func Merge(dst, src *Mesh) {
	off := len(dst.Verts)
	dst.Verts = append(dst.Verts, src.Verts...)
	for fi := range src.Faces {
		f := src.Faces[fi]
		loops := make([][]int, len(f.Loops))
		for li, loop := range f.Loops {
			nl := make([]int, len(loop))
			for i, vi := range loop {
				nl[i] = vi + off
			}
			loops[li] = nl
		}
		f.Loops = loops
		f.planeOK = false
		dst.Faces = append(dst.Faces, f)
	}
	dst.InvalidateCaches()
}

// Weld merges vertices closer than geom.WeldDist and drops the loop entries
// that collapse, which every boolean and rounding step must be followed by.
func Weld(m *Mesh) {
	const cell = geom.WeldDist * 2
	type key struct{ x, y, z int64 }
	at := func(v geom.Vec3) key {
		return key{
			int64(math.Round(v.X / cell)),
			int64(math.Round(v.Y / cell)),
			int64(math.Round(v.Z / cell)),
		}
	}

	remap := make([]int, len(m.Verts))
	newVerts := make([]geom.Vec3, 0, len(m.Verts))
	lookup := make(map[key]int, len(m.Verts))
	for i, v := range m.Verts {
		k := at(v)
		found := -1
		// Check the 27 neighbouring cells so a pair straddling a cell boundary
		// still welds.
		for dx := int64(-1); dx <= 1 && found < 0; dx++ {
			for dy := int64(-1); dy <= 1 && found < 0; dy++ {
				for dz := int64(-1); dz <= 1 && found < 0; dz++ {
					if j, ok := lookup[key{k.x + dx, k.y + dy, k.z + dz}]; ok {
						if newVerts[j].Dist(v) <= geom.WeldDist {
							found = j
						}
					}
				}
			}
		}
		if found < 0 {
			found = len(newVerts)
			newVerts = append(newVerts, v)
			lookup[k] = found
		}
		remap[i] = found
	}

	m.Verts = newVerts
	faces := m.Faces[:0]
	for fi := range m.Faces {
		f := m.Faces[fi]
		loops := make([][]int, 0, len(f.Loops))
		for _, loop := range f.Loops {
			nl := make([]int, 0, len(loop))
			for _, vi := range loop {
				r := remap[vi]
				if len(nl) > 0 && nl[len(nl)-1] == r {
					continue
				}
				nl = append(nl, r)
			}
			for len(nl) > 1 && nl[0] == nl[len(nl)-1] {
				nl = nl[:len(nl)-1]
			}
			if len(nl) >= 3 {
				loops = append(loops, nl)
			}
		}
		if len(loops) == 0 {
			continue // the whole face collapsed
		}
		f.Loops = loops
		f.planeOK = false
		faces = append(faces, f)
	}
	m.Faces = faces
	m.InvalidateCaches()
}
