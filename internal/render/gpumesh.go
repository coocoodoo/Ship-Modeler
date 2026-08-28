package render

import (
	"image"

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
	g.TriCount = len(tris)
	n := len(tris) * 3
	g.VertCount = n
	g.positions = make([]float32, 0, n*3)
	g.texcoords = make([]float32, 0, n*2)
	g.normals = make([]float32, 0, n*3)
	g.colors = make([]uint8, 0, n*4)

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
		nrm := m.FaceNormal(t.Face)
		// The UVs are the face's own paint mapping, which is affine within the
		// face's plane — so interpolating it across the face's triangles is
		// exact, and a stroke never has to touch the vertex buffer. Vertices
		// are already duplicated per face for flat shading, so each face
		// carries its own. An unpainted face samples the atlas nowhere and the
		// shader is told to ignore it.
		fp := m.Faces[t.Face].Paint
		for _, vi := range [3]int{t.A, t.B, t.C} {
			p := m.Verts[vi]
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
		}
	}

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
