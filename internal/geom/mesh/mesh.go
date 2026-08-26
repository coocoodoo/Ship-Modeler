// Package mesh holds the body representation: vertices plus polygon faces with
// holes, each carrying a stable identity (SPEC-GEOMETRY §2, D-07).
//
// Faces, not triangles, are the user-facing unit: you sketch on a face, push or
// pull a face, and paint a face. Triangulation happens only at the edges of the
// system — for rendering and for handing geometry to the boolean kernel — and
// results are merged back into polygon faces.
//
// This package is raylib-free and cgo-free; its tests need no window.
package mesh

import (
	"image"
	"math"

	"modeler/internal/geom"
)

// FaceUID is globally unique within a document and never reused:
// uint64(bodyID)<<32 | faceSeq (SPEC-DATA §1). It is the anchor for paint,
// boolean provenance and pick references.
type FaceUID uint64

// NoFace is the zero FaceUID, meaning "no lineage" / "unset".
const NoFace FaceUID = 0

// MakeFaceUID composes a FaceUID from its parts.
func MakeFaceUID(bodyID, faceSeq uint32) FaceUID {
	return FaceUID(uint64(bodyID)<<32 | uint64(faceSeq))
}

// BodyID returns the owning body of a FaceUID.
func (u FaceUID) BodyID() uint32 { return uint32(u >> 32) }

// Seq returns the per-body face sequence number of a FaceUID.
func (u FaceUID) Seq() uint32 { return uint32(u) }

// FacePaint is the pixel-art texture anchored to one face
// (SPEC-GEOMETRY §8.1). The struct lives here because mesh.Face owns it;
// package paint owns the brush and mapping operations over it.
type FacePaint struct {
	Res   int         // 16|32|128|256|512, the chip chosen at creation
	Texel float64     // world units per texel, fixed at creation
	Frame geom.Frame  // persistent paint anchor
	Img   *image.RGBA // alpha 0 means unpainted: the body color shows through
	Off   image.Point // texel index of Img's origin, so the image can grow
}

// UV maps a world point to continuous texel coordinates on this face's texture
// (SPEC-GEOMETRY §8.3).
//
// It lives on the struct rather than in package paint so that the renderer,
// which builds vertex UVs, and the brush, which decides which texel the cursor
// is over, are running the same arithmetic rather than two copies of it. The
// normal component is discarded, so a point anywhere along the face's normal
// maps to the same texel — which is exactly what a cursor ray hitting the
// surface needs.
func (p *FacePaint) UV(world geom.Vec3) geom.Vec2 {
	if p == nil || p.Texel == 0 {
		return geom.Vec2{}
	}
	local := p.Frame.ToLocal(world)
	return geom.Vec2{X: local.X / p.Texel, Y: local.Y / p.Texel}
}

// TexelBounds is the texel rectangle the image currently covers. Texel
// coordinates and image coordinates differ by Off, and everything that reads or
// writes a texel goes through this rather than doing that sum by hand.
func (p *FacePaint) TexelBounds() image.Rectangle {
	if p == nil || p.Img == nil {
		return image.Rectangle{}
	}
	return p.Img.Bounds().Add(p.Off)
}

// Face is a planar polygon with optional holes.
//
// Loops[0] is the outer loop, counter-clockwise seen from outside the body;
// any further loops are holes, clockwise from the same viewpoint. Entries are
// indices into Mesh.Verts.
type Face struct {
	ID        FaceUID
	Loops     [][]int
	SrcFace   FaceUID // lineage through booleans, for paint and selection
	NonPlanar bool    // set by direct edits (SPEC-GEOMETRY §7.2)
	Paint     *FacePaint

	// plane cache; valid only when planeOK is set. Invalidated by any vertex
	// edit through Mesh.InvalidateCaches.
	planeN  geom.Vec3
	planeD  float64
	planeOK bool
}

// Outer returns the outer loop indices.
func (f *Face) Outer() []int {
	if len(f.Loops) == 0 {
		return nil
	}
	return f.Loops[0]
}

// Holes returns the hole loops, if any.
func (f *Face) Holes() [][]int {
	if len(f.Loops) < 2 {
		return nil
	}
	return f.Loops[1:]
}

// Mesh is a body's geometry: a shared vertex array and polygon faces.
type Mesh struct {
	Verts []geom.Vec3
	Faces []Face

	topo *Topology // built on demand, dropped by InvalidateCaches
}

// Clone returns a deep copy. Bodies are small, so commands snapshot whole
// meshes for undo (SPEC-DATA §3.2).
func (m *Mesh) Clone() *Mesh {
	out := &Mesh{
		Verts: append([]geom.Vec3(nil), m.Verts...),
		Faces: make([]Face, len(m.Faces)),
	}
	for i := range m.Faces {
		src := &m.Faces[i]
		dst := &out.Faces[i]
		*dst = *src
		dst.Loops = make([][]int, len(src.Loops))
		for j, loop := range src.Loops {
			dst.Loops[j] = append([]int(nil), loop...)
		}
		// Paint is shared deliberately: boolean fragments reference one image
		// (SPEC-GEOMETRY §8.4). Undo restores the pointer, not the pixels;
		// paint strokes carry their own dirty-rect undo (SPEC-DATA §3.3).
	}
	return out
}

// InvalidateCaches drops derived data after any vertex or face edit.
func (m *Mesh) InvalidateCaches() {
	m.topo = nil
	for i := range m.Faces {
		m.Faces[i].planeOK = false
	}
}

// LoopPoints materialises a loop's vertex positions.
func (m *Mesh) LoopPoints(loop []int) []geom.Vec3 {
	pts := make([]geom.Vec3, len(loop))
	for i, vi := range loop {
		pts[i] = m.Verts[vi]
	}
	return pts
}

// FaceNormal returns the unit normal of a face using Newell's method, which is
// stable for near-degenerate polygons and correct for non-convex ones.
func (m *Mesh) FaceNormal(fi int) geom.Vec3 {
	n, _ := m.facePlane(fi)
	return n
}

// FacePlane returns the face's unit normal and plane offset d, where the plane
// is {p : dot(n,p) = d}.
func (m *Mesh) FacePlane(fi int) (n geom.Vec3, d float64) {
	return m.facePlane(fi)
}

func (m *Mesh) facePlane(fi int) (geom.Vec3, float64) {
	f := &m.Faces[fi]
	if f.planeOK {
		return f.planeN, f.planeD
	}
	loop := f.Outer()
	var acc geom.Vec3
	var centroid geom.Vec3
	n := len(loop)
	if n >= 3 {
		for i := 0; i < n; i++ {
			a := m.Verts[loop[i]]
			b := m.Verts[loop[(i+1)%n]]
			acc.X += (a.Y - b.Y) * (a.Z + b.Z)
			acc.Y += (a.Z - b.Z) * (a.X + b.X)
			acc.Z += (a.X - b.X) * (a.Y + b.Y)
			centroid = centroid.Add(a)
		}
		centroid = centroid.Mul(1 / float64(n))
	}
	unit, ok := acc.NormalizeOK()
	if !ok {
		unit = geom.AxisZ
	}
	f.planeN = unit
	f.planeD = unit.Dot(centroid)
	f.planeOK = true
	return f.planeN, f.planeD
}

// FaceArea returns the polygon area with holes subtracted.
func (m *Mesh) FaceArea(fi int) float64 {
	n := m.FaceNormal(fi)
	f := &m.Faces[fi]
	area := 0.0
	for li, loop := range f.Loops {
		a := m.loopSignedArea(loop, n)
		if li == 0 {
			area += a
		} else {
			area -= math.Abs(a)
		}
	}
	return area
}

// loopSignedArea projects a loop onto the plane with normal n and returns its
// signed area (positive when the loop winds counter-clockwise about n).
func (m *Mesh) loopSignedArea(loop []int, n geom.Vec3) float64 {
	if len(loop) < 3 {
		return 0
	}
	var acc geom.Vec3
	for i := range loop {
		a := m.Verts[loop[i]]
		b := m.Verts[loop[(i+1)%len(loop)]]
		acc = acc.Add(a.Cross(b))
	}
	return 0.5 * acc.Dot(n)
}

// FaceCentroid returns the area-weighted centroid of the outer loop, which is
// where gizmos and extrude arrows attach.
func (m *Mesh) FaceCentroid(fi int) geom.Vec3 {
	loop := m.Faces[fi].Outer()
	if len(loop) == 0 {
		return geom.Vec3{}
	}
	var c geom.Vec3
	for _, vi := range loop {
		c = c.Add(m.Verts[vi])
	}
	return c.Mul(1 / float64(len(loop)))
}

// FaceFrame returns the canonical frame of a face: origin at its centroid,
// basis from the face normal (SPEC-GEOMETRY §3).
func (m *Mesh) FaceFrame(fi int) geom.Frame {
	return geom.FrameFromNormal(m.FaceCentroid(fi), m.FaceNormal(fi))
}

// Planarity returns the largest distance from any loop vertex to the face
// plane. Compare against geom.PlanarDist (SPEC-GEOMETRY §7.2).
func (m *Mesh) Planarity(fi int) float64 {
	n, d := m.FacePlane(fi)
	worst := 0.0
	for _, loop := range m.Faces[fi].Loops {
		for _, vi := range loop {
			if dist := math.Abs(n.Dot(m.Verts[vi]) - d); dist > worst {
				worst = dist
			}
		}
	}
	return worst
}

// RecheckPlanarity updates the NonPlanar flag of every face after an edit and
// returns how many faces are now flagged.
func (m *Mesh) RecheckPlanarity() int {
	count := 0
	for i := range m.Faces {
		m.Faces[i].NonPlanar = m.Planarity(i) > geom.PlanarDist
		if m.Faces[i].NonPlanar {
			count++
		}
	}
	return count
}

// AABB returns the bounding box of all vertices.
func (m *Mesh) AABB() geom.AABB {
	return geom.AABBOf(m.Verts)
}

// Tri is a triangle of vertex indices tagged with the face it came from.
type Tri struct {
	A, B, C int
	Face    int // index into Mesh.Faces
}

// Triangulate returns the whole mesh as tagged triangles, for rendering and for
// the boolean kernel handoff.
//
// v1 uses a fan for convex, hole-free loops and the ear-clipping triangulator
// for everything else, so callers never need to know which is which.
func (m *Mesh) Triangulate() []Tri {
	tris := make([]Tri, 0, len(m.Faces)*2)
	for fi := range m.Faces {
		tris = m.appendFaceTris(tris, fi)
	}
	return tris
}

// FaceTris triangulates a single face.
func (m *Mesh) FaceTris(fi int) []Tri {
	return m.appendFaceTris(nil, fi)
}

func (m *Mesh) appendFaceTris(dst []Tri, fi int) []Tri {
	f := &m.Faces[fi]
	loop := f.Outer()
	if len(loop) < 3 {
		return dst
	}
	if len(f.Loops) == 1 && m.loopIsConvex(loop) {
		for i := 1; i+1 < len(loop); i++ {
			dst = append(dst, Tri{A: loop[0], B: loop[i], C: loop[i+1], Face: fi})
		}
		return dst
	}
	return m.appendEarTris(dst, fi)
}

// loopIsConvex reports whether a loop is convex when projected onto its plane.
func (m *Mesh) loopIsConvex(loop []int) bool {
	n := m.faceNormalOfLoop(loop)
	cnt := len(loop)
	for i := 0; i < cnt; i++ {
		a := m.Verts[loop[i]]
		b := m.Verts[loop[(i+1)%cnt]]
		c := m.Verts[loop[(i+2)%cnt]]
		if b.Sub(a).Cross(c.Sub(b)).Dot(n) < -1e-12 {
			return false
		}
	}
	return true
}

func (m *Mesh) faceNormalOfLoop(loop []int) geom.Vec3 {
	var acc geom.Vec3
	cnt := len(loop)
	for i := 0; i < cnt; i++ {
		a := m.Verts[loop[i]]
		b := m.Verts[loop[(i+1)%cnt]]
		acc.X += (a.Y - b.Y) * (a.Z + b.Z)
		acc.Y += (a.Z - b.Z) * (a.X + b.X)
		acc.Z += (a.X - b.X) * (a.Y + b.Y)
	}
	return acc.Normalize()
}

// appendEarTris triangulates a face with holes or concavities by projecting to
// the face frame and ear-clipping, bridging holes to the outer loop first.
//
// M2 adds the integer-exact triangulator in geom/sketch2d for sketch regions
// (SPEC-GEOMETRY §5.2). This float version stays for mesh faces, whose
// vertices may legitimately be off-grid after booleans.
func (m *Mesh) appendEarTris(dst []Tri, fi int) []Tri {
	f := &m.Faces[fi]
	frame := geom.FrameFromNormal(m.Verts[f.Outer()[0]], m.FaceNormal(fi))
	outer := m.projectLoop(f.Outer(), frame)
	if signedAreaProj(outer) < 0 {
		reverse(outer)
	}

	// Bridge each hole into the outer polygon; holes must wind the other way.
	for _, hole := range f.Holes() {
		hp := m.projectLoop(hole, frame)
		if len(hp) < 3 {
			continue
		}
		if signedAreaProj(hp) > 0 {
			reverse(hp)
		}
		outer = spliceHole(outer, hp)
	}

	return earClip(dst, outer, fi)
}

// projPt is a loop vertex projected into a face frame, keeping its mesh index.
type projPt struct {
	p  geom.Vec2
	vi int
}

func (m *Mesh) projectLoop(loop []int, frame geom.Frame) []projPt {
	out := make([]projPt, len(loop))
	for i, vi := range loop {
		out[i] = projPt{frame.ToLocal(m.Verts[vi]), vi}
	}
	return out
}

func signedAreaProj(s []projPt) float64 {
	var a float64
	for i := range s {
		p, q := s[i].p, s[(i+1)%len(s)].p
		a += p.X*q.Y - q.X*p.Y
	}
	return a * 0.5
}

// spliceHole joins a hole into the outer loop with a doubled bridge edge. It
// picks the hole's rightmost vertex and the nearest outer vertex that can see
// it without crossing either loop.
func spliceHole(outer, hole []projPt) []projPt {
	// Rightmost hole vertex.
	hi := 0
	for i := range hole {
		if hole[i].p.X > hole[hi].p.X {
			hi = i
		}
	}
	m := hole[hi].p

	best, bestDist := -1, math.Inf(1)
	for i := range outer {
		d := outer[i].p.Sub(m).LenSq()
		if d >= bestDist {
			continue
		}
		if !bridgeVisible(outer, hole, i, hi) {
			continue
		}
		best, bestDist = i, d
	}
	if best < 0 {
		// Degenerate input: attach to the rightmost outer vertex anyway so the
		// face still renders as something rather than vanishing.
		best = 0
		for i := range outer {
			if outer[i].p.X > outer[best].p.X {
				best = i
			}
		}
	}

	out := make([]projPt, 0, len(outer)+len(hole)+2)
	out = append(out, outer[:best+1]...)
	for k := 0; k <= len(hole); k++ {
		out = append(out, hole[(hi+k)%len(hole)])
	}
	out = append(out, outer[best:]...)
	return out
}

// bridgeVisible reports whether the segment outer[oi] to hole[hi] crosses any
// edge of either loop.
func bridgeVisible(outer, hole []projPt, oi, hi int) bool {
	a, b := outer[oi].p, hole[hi].p
	crosses := func(loop []projPt, skip ...int) bool {
		for i := range loop {
			j := (i + 1) % len(loop)
			if containsInt(skip, i) || containsInt(skip, j) {
				continue
			}
			if segsProperlyCross(a, b, loop[i].p, loop[j].p) {
				return true
			}
		}
		return false
	}
	if crosses(outer, oi) {
		return false
	}
	return !crosses(hole, hi)
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// segsProperlyCross reports a strict crossing of segment interiors.
func segsProperlyCross(p1, p2, q1, q2 geom.Vec2) bool {
	d1 := cross2(p2.Sub(p1), q1.Sub(p1))
	d2 := cross2(p2.Sub(p1), q2.Sub(p1))
	d3 := cross2(q2.Sub(q1), p1.Sub(q1))
	d4 := cross2(q2.Sub(q1), p2.Sub(q1))
	return ((d1 > 0) != (d2 > 0)) && ((d3 > 0) != (d4 > 0))
}

// earClip triangulates a simple polygon given counter-clockwise.
func earClip(dst []Tri, poly []projPt, fi int) []Tri {
	idx := make([]int, len(poly))
	for i := range idx {
		idx[i] = i
	}
	guard := 0
	maxGuard := len(poly)*len(poly) + 16
	for len(idx) > 3 && guard < maxGuard {
		guard++
		clipped := false
		for k := 0; k < len(idx); k++ {
			i0 := idx[(k+len(idx)-1)%len(idx)]
			i1 := idx[k]
			i2 := idx[(k+1)%len(idx)]
			a, b, c := poly[i0].p, poly[i1].p, poly[i2].p
			if cross2(b.Sub(a), c.Sub(b)) <= 0 {
				continue // reflex or degenerate corner
			}
			blocked := false
			for _, j := range idx {
				if j == i0 || j == i1 || j == i2 {
					continue
				}
				if pointInTri(poly[j].p, a, b, c) {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
			dst = append(dst, Tri{A: poly[i0].vi, B: poly[i1].vi, C: poly[i2].vi, Face: fi})
			idx = append(idx[:k], idx[k+1:]...)
			clipped = true
			break
		}
		if !clipped {
			break // degenerate input: fan whatever remains
		}
	}
	for i := 1; i+1 < len(idx); i++ {
		dst = append(dst, Tri{A: poly[idx[0]].vi, B: poly[idx[i]].vi, C: poly[idx[i+1]].vi, Face: fi})
	}
	return dst
}

func reverse[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func cross2(a, b geom.Vec2) float64 { return a.X*b.Y - a.Y*b.X }

func pointInTri(p, a, b, c geom.Vec2) bool {
	d1 := cross2(b.Sub(a), p.Sub(a))
	d2 := cross2(c.Sub(b), p.Sub(b))
	d3 := cross2(a.Sub(c), p.Sub(c))
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}
