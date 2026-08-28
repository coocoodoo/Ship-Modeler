package mesh

import (
	"math"
	"sort"

	"modeler/internal/geom"
)

// Topology is the half-edge adjacency of a mesh, built on demand and cached
// (SPEC-GEOMETRY §2). It backs edge picking, edge moves, boundary walks,
// crease detection and validation.
type Topology struct {
	// Edges lists every undirected edge exactly once, with A < B.
	Edges []Edge
	// VertEdges[v] are the edge indices incident to vertex v.
	VertEdges [][]int
	// VertFaces[v] are the face indices incident to vertex v.
	VertFaces [][]int

	index map[edgeKey]int
}

type edgeKey struct{ a, b int }

// Edge is one undirected mesh edge and the face uses of it.
type Edge struct {
	A, B int
	// Uses are the directed uses of this edge. A closed 2-manifold has exactly
	// two, in opposite directions.
	Uses []EdgeUse
}

// EdgeUse records one face's traversal of an edge.
type EdgeUse struct {
	Face    int  // index into Mesh.Faces
	Loop    int  // index into Face.Loops
	Pos     int  // position of A-to-B within that loop
	Forward bool // true when the face walks A to B, false for B to A
}

// Manifold reports whether the edge is used exactly twice in opposite
// directions.
func (e *Edge) Manifold() bool {
	if len(e.Uses) != 2 {
		return false
	}
	return e.Uses[0].Forward != e.Uses[1].Forward
}

// Topo returns the cached topology, building it if needed.
func (m *Mesh) Topo() *Topology {
	if m.topo == nil {
		m.topo = buildTopology(m)
	}
	return m.topo
}

func buildTopology(m *Mesh) *Topology {
	t := &Topology{
		index:     make(map[edgeKey]int, len(m.Faces)*2),
		VertEdges: make([][]int, len(m.Verts)),
		VertFaces: make([][]int, len(m.Verts)),
	}
	for fi := range m.Faces {
		for li, loop := range m.Faces[fi].Loops {
			n := len(loop)
			for i := 0; i < n; i++ {
				a, b := loop[i], loop[(i+1)%n]
				if a == b {
					continue // zero-length edge: validation reports it
				}
				t.addUse(a, b, EdgeUse{Face: fi, Loop: li, Pos: i, Forward: a < b})
			}
			for _, v := range loop {
				if v >= 0 && v < len(t.VertFaces) && !containsInt(t.VertFaces[v], fi) {
					t.VertFaces[v] = append(t.VertFaces[v], fi)
				}
			}
		}
	}
	return t
}

func (t *Topology) addUse(a, b int, use EdgeUse) {
	k := edgeKey{a, b}
	if a > b {
		k = edgeKey{b, a}
	}
	ei, ok := t.index[k]
	if !ok {
		ei = len(t.Edges)
		t.Edges = append(t.Edges, Edge{A: k.a, B: k.b})
		t.index[k] = ei
		if k.a < len(t.VertEdges) {
			t.VertEdges[k.a] = append(t.VertEdges[k.a], ei)
		}
		if k.b < len(t.VertEdges) {
			t.VertEdges[k.b] = append(t.VertEdges[k.b], ei)
		}
	}
	t.Edges[ei].Uses = append(t.Edges[ei].Uses, use)
}

// EdgeIndex looks up an undirected edge, returning -1 when absent.
func (t *Topology) EdgeIndex(a, b int) int {
	if a > b {
		a, b = b, a
	}
	if i, ok := t.index[edgeKey{a, b}]; ok {
		return i
	}
	return -1
}

// EdgeKind classifies an edge for the overlay pass (SPEC-RENDER §5).
type EdgeKind uint8

const (
	// EdgeSmooth joins two nearly coplanar faces and is not drawn.
	EdgeSmooth EdgeKind = iota
	// EdgeCrease joins faces whose dihedral angle exceeds CreaseAngleDeg.
	EdgeCrease
	// EdgeBoundary is used by anything other than exactly two faces. On a valid
	// body this should not occur; it is drawn defensively when it does.
	EdgeBoundary
	// EdgeNonPlanar bounds a face flagged non-planar by direct edits.
	EdgeNonPlanar
)

// CreaseAngleDeg is the dihedral threshold above which an edge is drawn
// (SPEC-RENDER §5). Blocky models put nearly every visible edge above it.
const CreaseAngleDeg = 25.0

// FoldCreaseAngleDeg is the far lower threshold for an edge whose two faces
// are pieces of the same source face (V-134). A shallow angle between two
// authored faces is a 16-gon pretending to be a cylinder and stays smooth;
// the same angle between two pieces of what used to be one face is a fold
// the user just made, and an edge you made is an edge you can see and grab —
// however gently you bent it. One degree keeps flush boolean fragments and
// folds later flattened back out from growing seams.
const FoldCreaseAngleDeg = 1.0

// ClassifyEdge returns the drawing class of edge ei.
func (m *Mesh) ClassifyEdge(ei int) EdgeKind {
	t := m.Topo()
	e := &t.Edges[ei]
	if !e.Manifold() {
		return EdgeBoundary
	}
	f0, f1 := e.Uses[0].Face, e.Uses[1].Face
	if m.Faces[f0].NonPlanar || m.Faces[f1].NonPlanar {
		return EdgeNonPlanar
	}
	n0 := m.FaceNormal(f0)
	n1 := m.FaceNormal(f1)
	cos := math.Max(-1, math.Min(1, n0.Dot(n1)))
	angle := math.Acos(cos)
	if angle > CreaseAngleDeg*math.Pi/180 {
		return EdgeCrease
	}
	// Pieces of one former face meeting at any real angle are a fold the user
	// made, not authored smoothness — the NoFace guard matters, because two
	// primitive faces with no lineage both answer NoFace and are not thereby
	// the same face.
	s0, s1 := m.Faces[f0].SrcFace, m.Faces[f1].SrcFace
	if s0 != NoFace && s0 == s1 && angle > FoldCreaseAngleDeg*math.Pi/180 {
		return EdgeCrease
	}
	return EdgeSmooth
}

// DrawnEdges returns the edge indices the overlay pass should draw, sorted for
// determinism (golden shots depend on stable ordering).
func (m *Mesh) DrawnEdges() []int {
	t := m.Topo()
	out := make([]int, 0, len(t.Edges))
	for ei := range t.Edges {
		if m.ClassifyEdge(ei) != EdgeSmooth {
			out = append(out, ei)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := &t.Edges[out[i]], &t.Edges[out[j]]
		if a.A != b.A {
			return a.A < b.A
		}
		return a.B < b.B
	})
	return out
}

// EdgeSegment returns the endpoints of edge ei in world space.
func (m *Mesh) EdgeSegment(ei int) (geom.Vec3, geom.Vec3) {
	e := &m.Topo().Edges[ei]
	return m.Verts[e.A], m.Verts[e.B]
}

// Shells partitions faces into connected components joined across shared
// edges. A body may legally have several (SPEC-GEOMETRY §6.5).
func (m *Mesh) Shells() [][]int {
	t := m.Topo()
	seen := make([]bool, len(m.Faces))
	var out [][]int
	for start := range m.Faces {
		if seen[start] {
			continue
		}
		comp := []int{start}
		seen[start] = true
		for qi := 0; qi < len(comp); qi++ {
			f := comp[qi]
			for _, loop := range m.Faces[f].Loops {
				n := len(loop)
				for i := 0; i < n; i++ {
					ei := t.EdgeIndex(loop[i], loop[(i+1)%n])
					if ei < 0 {
						continue
					}
					for _, u := range t.Edges[ei].Uses {
						if !seen[u.Face] {
							seen[u.Face] = true
							comp = append(comp, u.Face)
						}
					}
				}
			}
		}
		sort.Ints(comp)
		out = append(out, comp)
	}
	return out
}
