package render

import (
	"image"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// BodyGPU is a body's GPU-resident geometry plus the CPU-side data the passes
// need: the face table for picking and the edge list for the overlay
// (SPEC-RENDER §2, §5).
//
// Vertices are duplicated per face so each carries its own face normal, which
// is what makes the standard pipeline shade flat.
type BodyGPU struct {
	// rlMesh is allocated on its own rather than embedded, because cgo's
	// pointer check scans the entire heap object a C pointer lands in: any
	// unpinned Go pointer sharing that object (our slices, edge list, ...)
	// would make UploadMesh panic. Alone, its only pointers are the vertex
	// arrays, which raylib-go pins for the call.
	rlMesh   *rl.Mesh
	uploaded bool

	// FaceUIDs maps a local face index (what the vertex color encodes) to the
	// face it came from; MeshFaces maps it to the index within mesh.Faces.
	FaceUIDs  []mesh.FaceUID
	MeshFaces []int

	// Verts mirrors the source mesh's vertex array so picking and gizmos can
	// refer to real vertex indices rather than positions.
	Verts []geom.Vec3

	// vertFace is the mesh face each emitted corner belongs to. Before the
	// shading tessellation a corner's face could be read off its triangle
	// index; now that one mesh triangle emits many, the answer has to be
	// carried (V-142).
	vertFace []int

	Edges     []EdgeSeg
	TriCount  int
	VertCount int
	Bounds    geom.AABB

	// Paint is the body's packed face textures, or nil when nothing on it is
	// painted (SPEC-GEOMETRY §8.1).
	Paint *atlas

	// Retained CPU arrays: rl.Mesh holds raw pointers into them, so they must
	// stay reachable for as long as the mesh is alive.
	positions []float32
	texcoords []float32
	normals   []float32
	colors    []uint8
}

// EdgeSeg is one drawn edge of a body, in world space.
type EdgeSeg struct {
	A, B geom.Vec3
	// VertA and VertB are the mesh vertex indices of the endpoints.
	VertA, VertB int
	Kind         mesh.EdgeKind
	// Index is the topology edge index, used as the pick reference.
	Index int
}

// BuildBodyGPU converts a body mesh into GPU form. The result is not uploaded
// yet; call Upload once a GL context exists.
func BuildBodyGPU(m *mesh.Mesh) *BodyGPU {
	g := &BodyGPU{Bounds: m.AABB()}
	g.Verts = append(g.Verts, m.Verts...)
	g.Paint = buildAtlas(m)

	tris := m.Triangulate()
	// A rough reservation: the tessellation below multiplies this, and append
	// grows it the rest of the way.
	n := len(tris) * 3
	g.positions = make([]float32, 0, n*3)
	g.texcoords = make([]float32, 0, n*2)
	g.normals = make([]float32, 0, n*3)
	g.colors = make([]uint8, 0, n*4)

	// How finely each face is cut for shading, worked out once per face so
	// that two triangles sharing an edge inside a face split it identically
	// and leave no T-junction (V-142).
	splits := make(map[int]int, len(m.Faces))
	radius := aoRadiusFor(g.Bounds)

	// Local face indices are assigned in mesh face order so they stay stable
	// for as long as the mesh does.
	localOf := make(map[int]int, len(m.Faces))
	for _, t := range tris {
		local, ok := localOf[t.Face]
		if !ok {
			local = len(g.FaceUIDs)
			localOf[t.Face] = local
			g.FaceUIDs = append(g.FaceUIDs, m.Faces[t.Face].ID)
			g.MeshFaces = append(g.MeshFaces, t.Face)
		}
		split, seen := splits[t.Face]
		if !seen {
			split = shadingSplit(m, tris, t.Face, radius)
			splits[t.Face] = split
		}
		nrm := m.FaceNormal(t.Face)
		// The UVs are the face's own paint mapping, which is affine within the
		// face's plane — so interpolating it across the face's triangles is
		// exact, and a stroke never has to touch the vertex buffer. Vertices
		// are already duplicated per face for flat shading, so each face
		// carries its own. An unpainted face samples the atlas nowhere and the
		// shader is told to ignore it.
		fp := m.Faces[t.Face].Paint
		emit := func(p geom.Vec3) {
			g.positions = append(g.positions, float32(p.X), float32(p.Y), float32(p.Z))
			if u, v, ok := g.Paint.uvOf(fp, p); ok {
				g.texcoords = append(g.texcoords, u, v)
			} else {
				g.texcoords = append(g.texcoords, 0, 0)
			}
			g.normals = append(g.normals, float32(nrm.X), float32(nrm.Y), float32(nrm.Z))
			// Blue is the baked AO openness, fully open until BakeAO runs —
			// previews and mid-drag rebuilds skip the bake and must not
			// render darkened.
			g.colors = append(g.colors,
				uint8(local&0xFF), uint8((local>>8)&0xFF), 255, 255)
			g.vertFace = append(g.vertFace, t.Face)
		}
		subdivide(m.Verts[t.A], m.Verts[t.B], m.Verts[t.C], split, emit)
	}
	g.VertCount = len(g.vertFace)
	g.TriCount = g.VertCount / 3

	topo := m.Topo()
	for _, ei := range m.DrawnEdges() {
		e := &topo.Edges[ei]
		g.Edges = append(g.Edges, EdgeSeg{
			A: m.Verts[e.A], B: m.Verts[e.B],
			VertA: e.A, VertB: e.B,
			Kind: m.ClassifyEdge(ei), Index: ei,
		})
	}
	return g
}

// HasPaint reports whether this body has any painted face to sample.
func (g *BodyGPU) HasPaint() bool { return g.Paint != nil && g.Paint.ready }

// PaintOverflowed reports that some of the body's textures did not fit its
// atlas, so those faces are rendering bare.
func (g *BodyGPU) PaintOverflowed() bool { return g.Paint != nil && g.Paint.Overflow }

// UpdatePaint re-uploads the changed texels of one face texture, and reports
// false when the atlas layout no longer fits and the body must be rebuilt.
func (g *BodyGPU) UpdatePaint(p *mesh.FacePaint, texels image.Rectangle) bool {
	return g.Paint.updateRect(p, texels)
}

// Upload sends the geometry to the GPU. Safe to call more than once.
func (g *BodyGPU) Upload() {
	g.Paint.upload()
	if g.uploaded || g.VertCount == 0 {
		return
	}
	m := new(rl.Mesh)
	m.VertexCount = int32(g.VertCount)
	m.TriangleCount = int32(g.TriCount)
	m.Vertices = &g.positions[0]
	m.Texcoords = &g.texcoords[0]
	m.Normals = &g.normals[0]
	m.Colors = &g.colors[0]
	rl.UploadMesh(m, false)
	g.rlMesh = m
	g.uploaded = true
}

// Unload frees GPU buffers.
func (g *BodyGPU) Unload() {
	g.Paint.unload()
	if !g.uploaded {
		return
	}
	rl.UnloadMesh(g.rlMesh)
	g.rlMesh = nil
	g.uploaded = false
}

// LocalFaceIndex returns the local index of a mesh face, or -1.
func (g *BodyGPU) LocalFaceIndex(meshFace int) int {
	for i, f := range g.MeshFaces {
		if f == meshFace {
			return i
		}
	}
	return -1
}

// Shading tessellation (the user's report, 2026-08-28).
//
// Baked openness is a corner value the shader interpolates across a face, and
// a CAD face is a big flat polygon whose only corners are its outline. On the
// inside floor of a box all four are equally occluded, so interpolating them
// gives a constant: the face came out uniformly dimmer rather than dark in
// its corners, which is not what ambient occlusion looks like and is why the
// feature read as absent (V-142).
//
// The fix is to give the falloff somewhere to live. The render mesh — not the
// document's — cuts each face into a grid so shading has interior samples.
// Nothing else changes: every new corner lies in the face's own plane, its uv
// comes from the same affine paint mapping, and it carries the same face
// index, so picking and painting never learn this happened.

// shadingSpacing is how far apart shading samples should fall on a large face,
// in units. A fifth of the AO radius: fine enough that a contact shadow has
// several steps to fade over, coarse enough that a hull face is a few hundred
// triangles rather than a few thousand.
const shadingSpacing = AORadius / 5

// maxShadingSplit caps the cut of any one triangle edge, so a single enormous
// face costs a bounded number of triangles instead of an unbounded one. Past
// it the samples simply spread out.
const maxShadingSplit = 12

// shadingSplit is how many pieces each edge of a face's triangles is cut into.
// It is decided per face rather than per triangle: two triangles sharing an
// edge inside one face must cut that edge the same way, or the shading breaks
// along a seam that is not there.
//
// A face with nothing in front of it is left alone. That is not an
// optimisation bolted on afterwards — it is the same statement as "this face
// is fully lit everywhere", and on a convex body it is every face, which is
// why an ordinary box costs exactly what it always did.
func shadingSplit(m *mesh.Mesh, tris []mesh.Tri, fi int, radius float64) int {
	lo, hi := facePlaneBounds(m, fi)
	if !faceHasOccluders(m, tris, fi, lo, hi, radius) {
		return 1
	}
	longest := math.Max(hi.X-lo.X, math.Max(hi.Y-lo.Y, hi.Z-lo.Z))
	n := int(math.Ceil(longest/shadingSpacing - 1e-9))
	if n < 1 {
		return 1
	}
	if n > maxShadingSplit {
		return maxShadingSplit
	}
	return n
}

// subdivide cuts a triangle into n x n similar triangles on a barycentric
// grid and hands each corner to emit, in the original winding.
//
// The grid is what keeps edges matched: every edge is cut into n equal parts,
// so a neighbour cutting its shared edge the same way lands on the same
// points.
func subdivide(a, b, c geom.Vec3, n int, emit func(geom.Vec3)) {
	if n <= 1 {
		emit(a)
		emit(b)
		emit(c)
		return
	}
	ab, ac := b.Sub(a), c.Sub(a)
	step := 1 / float64(n)
	at := func(iu, iv int) geom.Vec3 {
		return a.Add(ab.Mul(float64(iu) * step)).Add(ac.Mul(float64(iv) * step))
	}
	for iv := 0; iv < n; iv++ {
		for iu := 0; iu+iv < n; iu++ {
			emit(at(iu, iv))
			emit(at(iu+1, iv))
			emit(at(iu, iv+1))
			if iu+iv < n-1 {
				emit(at(iu+1, iv))
				emit(at(iu+1, iv+1))
				emit(at(iu, iv+1))
			}
		}
	}
}

// facePlaneBounds is a face's axis-aligned bounding box.
func facePlaneBounds(m *mesh.Mesh, fi int) (lo, hi geom.Vec3) {
	lo = geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi = geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, loop := range m.Faces[fi].Loops {
		for _, vi := range loop {
			p := m.Verts[vi]
			lo = geom.Vec3{X: math.Min(lo.X, p.X), Y: math.Min(lo.Y, p.Y), Z: math.Min(lo.Z, p.Z)}
			hi = geom.Vec3{X: math.Max(hi.X, p.X), Y: math.Max(hi.Y, p.Y), Z: math.Max(hi.Z, p.Z)}
		}
	}
	return lo, hi
}

// faceHasOccluders reports whether any geometry stands in front of a face and
// close enough to shade it. Coplanar and behind do not count: a face can only
// be darkened by something on the side it looks at.
func faceHasOccluders(m *mesh.Mesh, tris []mesh.Tri, fi int, lo, hi geom.Vec3, radius float64) bool {
	n := m.FaceNormal(fi)
	c := m.FaceCentroid(fi)
	lo = geom.Vec3{X: lo.X - radius, Y: lo.Y - radius, Z: lo.Z - radius}
	hi = geom.Vec3{X: hi.X + radius, Y: hi.Y + radius, Z: hi.Z + radius}
	for _, t := range tris {
		if t.Face == fi {
			continue
		}
		for _, vi := range [3]int{t.A, t.B, t.C} {
			p := m.Verts[vi]
			if p.Sub(c).Dot(n) <= 1e-9 {
				continue // coplanar with the face, or behind it
			}
			if p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y &&
				p.Z >= lo.Z && p.Z <= hi.Z {
				return true
			}
		}
	}
	return false
}
