package render

import (
	"math"

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

// AORadius is how far occlusion reaches, in units. Past it geometry does not
// darken a corner: ambient occlusion is about corners and crevices, not
// shadows.
const AORadius = 2.5

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
	// Per-triangle corners and a bounding sphere each, so a corner only ray
	// tests the triangles near it.
	type aoTri struct {
		a, b, c geom.Vec3
		centre  geom.Vec3
		radius  float64
	}
	ts := make([]aoTri, len(tris))
	for i, t := range tris {
		a, b, c := m.Verts[t.A], m.Verts[t.B], m.Verts[t.C]
		centre := a.Add(b).Add(c).Mul(1.0 / 3)
		r := math.Sqrt(math.Max(centre.Sub(a).LenSq(),
			math.Max(centre.Sub(b).LenSq(), centre.Sub(c).LenSq())))
		ts[i] = aoTri{a: a, b: b, c: c, centre: centre, radius: r}
	}

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
		// The corner's own face, via the triangle it was emitted from.
		centroid := m.FaceCentroid(tris[i/3].Face)
		sample := p
		if in := centroid.Sub(p); in.Len() > 1e-9 {
			step := math.Min(aoInset, in.Len()*0.5)
			sample = p.Add(in.Normalize().Mul(step))
		}
		frame := geom.FrameFromNormal(sample, n)
		origin := sample.Add(n.Mul(aoRayBias))

		// The triangles within reach of this corner.
		var near []aoTri
		for _, t := range ts {
			if t.centre.Sub(sample).Len() <= AORadius+t.radius {
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
			if best <= AORadius {
				occ += 1 - best/AORadius
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
