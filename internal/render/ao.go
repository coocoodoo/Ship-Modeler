package render

import (
	"math"
	"runtime"
	"sync"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Baked ambient occlusion (the user's request, 2026-08-28).
//
// Simple by design: at build time, every rendered corner casts a fixed fan of
// rays into the hemisphere above its face; the fraction they hit, weighted by
// how close the hit is, becomes an openness byte in the vertex colour's blue
// channel (red and green belong to the pick pass, blue was spare). The shader
// multiplies it in, scaled by a strength the settings own — so restrength is
// free and rebake only follows geometry.
//
// It is deterministic — a fixed pattern, no noise — which is what lets the
// golden shots pin it, and it is skipped while a drag is live: a bake per
// frame would turn a vertex drag into a slideshow, and shading that pops in
// on release is the honest trade.

// AORadius is the reference reach that shadingSpacing is derived from. The
// bake itself uses aoRadiusFor, which scales with the body.
const AORadius = 2.5

// AO reach scales with the body it is shading (V-142).
//
// A fixed 2.5 units was the other half of "I don't see any ambient occlusion":
// on a twelve-unit hull with a three-unit cavity, the ceiling was out of range
// of its own floor and the whole inside came back fully open. Occlusion is
// local, but "local" only means anything relative to how big the thing is —
// so the reach is a fraction of the body's own diagonal, floored so a tiny
// greeble still gets corner shading and capped so a huge hull does not turn
// into a fog bank.
const (
	aoRadiusOfDiagonal = 0.35
	aoRadiusMin        = 1.5
	aoRadiusMax        = 12.0
)

// aoRadiusFor is the reach used for one body. Bodies are baked against their
// own triangles only, so taking the size from the body being shaded is
// self-consistent.
func aoRadiusFor(b geom.AABB) float64 {
	r := aoRadiusOfDiagonal * b.Max.Sub(b.Min).Len()
	if r < aoRadiusMin {
		return aoRadiusMin
	}
	if r > aoRadiusMax {
		return aoRadiusMax
	}
	return r
}

// aoRayBias lifts a ray's origin off its own face so coplanar triangles of
// the very face being shaded never count as occluders.
const aoRayBias = 1e-4

// aoInset pulls each corner's sample point toward its face's interior before
// casting. From the exact corner, an abutting wall is edge-on — a
// zero-thickness plane subtends nothing from inside its own plane — so the
// mathematically darkest point samples brightest. A third of a unit in is
// where the corner's darkness actually lives, and interpolation carries it
// back out to the edge.
const aoInset = 0.35

// aoDirs is the fixed hemisphere fan, built once: two rings of eight
// directions at 30 and 60 degrees above the face plane. Sixteen rays is
// enough for corner shading and few enough to bake a whole body in
// milliseconds.
var aoDirs = buildAODirs()

func buildAODirs() []geom.Vec3 {
	var out []geom.Vec3
	for _, elev := range []float64{30, 60} {
		e := elev * math.Pi / 180
		for k := 0; k < 8; k++ {
			a := float64(k) * math.Pi / 4
			out = append(out, geom.Vec3{
				X: math.Cos(e) * math.Cos(a),
				Y: math.Cos(e) * math.Sin(a),
				Z: math.Sin(e),
			})
		}
	}
	return out
}

// BakeAO fills the openness byte for every rendered corner of a body. Call it
// after BuildBodyGPU and before Upload.
func BakeAO(g *BodyGPU, m *mesh.Mesh) {
	if g == nil || m == nil || g.VertCount == 0 {
		return
	}
	tris := m.Triangulate()
	ts := make([]aoTri, len(tris))
	for i, t := range tris {
		a, b, c := m.Verts[t.A], m.Verts[t.B], m.Verts[t.C]
		centre := a.Add(b).Add(c).Mul(1.0 / 3)
		r := math.Sqrt(math.Max(centre.Sub(a).LenSq(),
			math.Max(centre.Sub(b).LenSq(), centre.Sub(c).LenSq())))
		ts[i] = aoTri{a: a, b: b, c: c, centre: centre, radius: r}
	}

	radius := aoRadiusFor(m.AABB())
	// Candidate occluders per face, not per corner.
	//
	// The filter used to run down every triangle for every corner, which cost
	// nothing when a face had four of them and became the whole bill once the
	// shading tessellation gave it hundreds (V-142). A face's corners all sit
	// inside its own bounding box, so the triangles that could possibly reach
	// any of them can be found once and then narrowed per corner from that
	// short list instead of from the whole body.
	cand := make(map[int][]aoTri, len(m.Faces))
	for _, fi := range g.MeshFaces {
		lo, hi := facePlaneBounds(m, fi)
		var out []aoTri
		for _, t := range ts {
			d := 0.0
			for _, ax := range [3]int{0, 1, 2} {
				c, l, h := axis(t.centre, ax), axis(lo, ax), axis(hi, ax)
				if c < l {
					d += (l - c) * (l - c)
				} else if c > h {
					d += (c - h) * (c - h)
				}
			}
			if math.Sqrt(d) <= radius+t.radius {
				out = append(out, t)
			}
		}
		cand[fi] = out
	}

	// One corner's answer depends on nothing another corner writes, so the
	// bake splits across cores. The result is byte-identical either way — the
	// ray pattern is fixed and every corner is computed from the mesh alone,
	// which is what lets the goldens keep pinning it.
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 || g.VertCount < 4096 {
		workers = 1
	}
	var wg sync.WaitGroup
	chunk := (g.VertCount + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo, hi := w*chunk, (w+1)*chunk
		if hi > g.VertCount {
			hi = g.VertCount
		}
		if lo >= hi {
			continue
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			bakeRange(g, m, cand, radius, lo, hi)
		}(lo, hi)
	}
	wg.Wait()
}

// bakeRange fills the openness byte for corners [lo, hi).
func bakeRange(g *BodyGPU, m *mesh.Mesh, cand map[int][]aoTri,
	radius float64, lo, hi int) {
	var near []aoTri
	for i := lo; i < hi; i++ {
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
		// The inset is for corners on the face's outline, where an abutting
		// wall shares the sample's own plane and so subtends nothing. A vertex
		// the shading tessellation put inside the face has no wall at it and
		// must not be moved: sliding every sample toward the centroid would
		// drag the contact shadow off the wall it belongs to and flatten the
		// falloff this exists to produce (V-142).
		fi := g.vertFace[i]
		sample := p
		if edge := distToFaceOutline(m, fi, p); edge < aoInset {
			if in := m.FaceCentroid(fi).Sub(p); in.Len() > 1e-9 {
				step := math.Min(aoInset-edge, in.Len()*0.5)
				sample = p.Add(in.Normalize().Mul(step))
			}
		}
		frame := geom.FrameFromNormal(sample, n)
		origin := sample.Add(n.Mul(aoRayBias))

		// The triangles within reach of this corner, narrowed from its face's
		// short list rather than from the whole body.
		near = near[:0]
		for _, t := range cand[fi] {
			if t.centre.Sub(sample).Len() <= radius+t.radius {
				near = append(near, t)
			}
		}

		occ := 0.0
		for _, d := range aoDirs {
			// The fan's z is the face normal; x and y lie in the face.
			dir := frame.U.Mul(d.X).Add(frame.V.Mul(d.Y)).Add(frame.N.Mul(d.Z))
			best := math.Inf(1)
			for _, t := range near {
				if hit, dist := rayTriangle(origin, dir, t.a, t.b, t.c); hit && dist < best {
					best = dist
				}
			}
			if best <= radius {
				occ += 1 - best/radius
			}
		}
		occ /= float64(len(aoDirs))

		open := 1 - occ
		if open < 0 {
			open = 0
		}
		g.colors[i*4+2] = byte(math.Round(open * 255))
	}
}

// aoTri is one occluder: its corners, and a bounding sphere so a sample can
// reject it without touching the ray test.
type aoTri struct {
	a, b, c geom.Vec3
	centre  geom.Vec3
	radius  float64
}

func axis(v geom.Vec3, i int) float64 {
	switch i {
	case 0:
		return v.X
	case 1:
		return v.Y
	}
	return v.Z
}

// rayTriangle is Moller-Trumbore: whether a ray hits a triangle, and how far
// along it.
func rayTriangle(o, d, a, b, c geom.Vec3) (bool, float64) {
	e1 := b.Sub(a)
	e2 := c.Sub(a)
	p := d.Cross(e2)
	det := e1.Dot(p)
	if det > -1e-12 && det < 1e-12 {
		return false, 0
	}
	inv := 1 / det
	t := o.Sub(a)
	u := t.Dot(p) * inv
	if u < 0 || u > 1 {
		return false, 0
	}
	q := t.Cross(e1)
	v := d.Dot(q) * inv
	if v < 0 || u+v > 1 {
		return false, 0
	}
	dist := e2.Dot(q) * inv
	if dist <= aoRayBias {
		return false, 0
	}
	return true, dist
}

// distToFaceOutline is how far a point on a face is from that face's own
// boundary — zero at a corner or along an edge, largest deep inside.
func distToFaceOutline(m *mesh.Mesh, fi int, p geom.Vec3) float64 {
	best := math.Inf(1)
	for _, loop := range m.Faces[fi].Loops {
		for k := range loop {
			a := m.Verts[loop[k]]
			b := m.Verts[loop[(k+1)%len(loop)]]
			if d := pointSegmentDist(p, a, b); d < best {
				best = d
			}
		}
	}
	return best
}

func pointSegmentDist(p, a, b geom.Vec3) float64 {
	ab := b.Sub(a)
	l2 := ab.LenSq()
	if l2 < 1e-18 {
		return p.Sub(a).Len()
	}
	t := p.Sub(a).Dot(ab) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return p.Sub(a.Add(ab.Mul(t))).Len()
}
