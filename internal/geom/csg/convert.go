package csg

import (
	"fmt"
	"math"
	"sort"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The converters (SPEC-GEOMETRY §6.3), which are the real work of the boolean
// kernel. Manifold speaks triangles; this document speaks polygons with holes.
// Going down is easy. Coming back up — turning a heap of triangles into the
// handful of flat faces a person drew — is what makes the result editable
// rather than a mesh dump.

// origin is what an output face inherits from the input face it came from.
type origin struct {
	// src is the lineage: the earliest ancestor of this face, so paint applied
	// once keeps working after any number of edits.
	src mesh.FaceUID
	// paint is shared, not copied. Fragments of one painted face are still
	// showing the same picture (TESTING §3.4).
	paint     *mesh.FacePaint
	keepEdges bool
}

// upload turns a body mesh into the triangle soup Manifold wants.
//
// One run per source face, tagged with an id reserved from Manifold, which is
// the thread that survives the boolean: whatever splitting and merging happens
// inside, every output triangle still reports the run it descends from.
func upload(m *mesh.Mesh) (glMesh, map[uint32]origin, error) {
	if m == nil {
		return glMesh{}, nil, fmt.Errorf("no solid given")
	}
	if len(m.Faces) == 0 || len(m.Verts) < 4 {
		return glMesh{}, nil, fmt.Errorf("a solid needs at least four vertices and one face")
	}

	base := reserveIDs(len(m.Faces))
	lineage := make(map[uint32]origin, len(m.Faces))

	g := glMesh{
		verts: make([]float64, 0, len(m.Verts)*3),
		tris:  make([]uint64, 0, len(m.Faces)*6),
		runs:  make([]uint64, 0, len(m.Faces)+1),
		ids:   make([]uint32, 0, len(m.Faces)),
	}
	for _, p := range m.Verts {
		g.verts = append(g.verts, p.X, p.Y, p.Z)
	}

	for fi := range m.Faces {
		tris := m.FaceTris(fi)
		if len(tris) == 0 {
			// A face that will not triangulate cannot be handed over, and
			// silently dropping it would hand Manifold an open surface.
			return glMesh{}, nil, fmt.Errorf("face %d could not be triangulated", fi)
		}
		id := base + uint32(fi)
		g.runs = append(g.runs, uint64(len(g.tris)/3))
		g.ids = append(g.ids, id)

		f := &m.Faces[fi]
		src := f.SrcFace
		if src == mesh.NoFace {
			src = f.ID
		}
		lineage[id] = origin{src: src, paint: f.Paint, keepEdges: f.HasAuthoredEdges()}

		for _, t := range tris {
			g.tris = append(g.tris, uint64(t.A), uint64(t.B), uint64(t.C))
		}
	}
	g.runs = append(g.runs, uint64(len(g.tris)/3))
	return g, lineage, nil
}

// download turns Manifold's triangles back into polygon faces.
//
// The shape of the job: weld the vertices so triangles that touch really share
// an index, group the triangles by the source face they descend from, grow each
// group into connected coplanar patches, trace each patch's boundary, and drop
// the vertices that a split left sitting in the middle of a straight edge. What
// comes out is the face a person would have drawn.
func download(g glMesh, lineage map[uint32]origin, bodyID uint32, seq *uint32) (*mesh.Mesh, int, error) {
	verts, tris := weldSoup(g)
	if len(tris) == 0 {
		return nil, 0, fmt.Errorf("the result has no triangles")
	}

	out := &mesh.Mesh{Verts: verts}
	loose := 0

	// Every triangle knows the source face it descends from.
	triOrigin := make([]origin, len(tris))
	owners := runOwners(g, len(tris))
	for i := range tris {
		triOrigin[i] = lineage[owners[i]]
	}
	tris, triOrigin = dropHollowShells(verts, tris, triOrigin)
	if len(tris) == 0 {
		return nil, 0, fmt.Errorf("the result has no triangles")
	}

	for _, patch := range patches(verts, tris, triOrigin) {
		from := patch.from
		faces, ok := traceLoops(verts, tris, patch.tris)
		if !ok {
			// A patch whose boundary will not close is emitted as plain
			// triangles: still valid geometry, just not tidy.
			for _, ti := range patch.tris {
				t := tris[ti]
				out.Faces = append(out.Faces, mesh.Face{
					ID:        nextUID(bodyID, seq),
					Loops:     [][]int{{t[0], t[1], t[2]}},
					SrcFace:   from.src,
					Paint:     from.paint,
					KeepEdges: from.keepEdges,
				})
				loose++
			}
			continue
		}
		for _, loops := range faces {
			out.Faces = append(out.Faces, mesh.Face{
				ID:        nextUID(bodyID, seq),
				Loops:     loops,
				SrcFace:   from.src,
				Paint:     from.paint,
				KeepEdges: from.keepEdges,
			})
		}
	}

	// No distance weld here: Manifold's merge table already said exactly which
	// vertices are one, and a second opinion based on proximity would only
	// undo it.
	straightenLoops(out)
	splitPinchedVertices(out)
	compact(out)
	return out, loose, nil
}

func nextUID(bodyID uint32, seq *uint32) mesh.FaceUID {
	id := mesh.MakeFaceUID(bodyID, *seq)
	*seq++
	return id
}

// runOwners expands the run table into one source id per triangle.
func runOwners(g glMesh, nTris int) []uint32 {
	out := make([]uint32, nTris)
	for r := 0; r < len(g.ids); r++ {
		start := int(g.runs[r])
		end := nTris
		if r+1 < len(g.runs) {
			end = int(g.runs[r+1])
		}
		if start < 0 || start > nTris {
			continue
		}
		if end > nTris {
			end = nTris
		}
		for i := start; i < end; i++ {
			out[i] = g.ids[r]
		}
	}
	return out
}

// weldSoup applies Manifold's merge table and rewrites the triangles to match.
//
// Manifold hands back a vertex per property-run corner, so the same corner can
// arrive several times over. Every step after this asks "do these two triangles
// share an edge", which is a question about indices, so the indices have to be
// made honest first.
//
// The merge table is the authority, not position. Two solids that touch along
// an edge have coincident vertices Manifold deliberately kept apart, and
// merging those by distance would fuse two legal shells into an edge belonging
// to four faces. What looks like the same point is not always the same point.
//
// Degenerate triangles go here too: a boolean can leave slivers with two
// corners welded together, and they carry no area and no information.
func weldSoup(g glMesh) ([]geom.Vec3, [][3]int) {
	n := len(g.verts) / 3
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range g.mergeFrom {
		from, to := int(g.mergeFrom[i]), int(g.mergeTo[i])
		if from < 0 || from >= n || to < 0 || to >= n {
			continue
		}
		if a, b := find(from), find(to); a != b {
			parent[a] = b
		}
	}

	remap := make([]int, n)
	out := make([]geom.Vec3, 0, n)
	canon := make(map[int]int, n)
	for i := 0; i < n; i++ {
		root := find(i)
		j, seen := canon[root]
		if !seen {
			j = len(out)
			canon[root] = j
			out = append(out, geom.Vec3{
				X: g.verts[root*3], Y: g.verts[root*3+1], Z: g.verts[root*3+2],
			})
		}
		remap[i] = j
	}

	faces := make([][3]int, 0, len(g.tris)/3)
	for i := 0; i+2 < len(g.tris); i += 3 {
		a, b, c := remap[g.tris[i]], remap[g.tris[i+1]], remap[g.tris[i+2]]
		if a == b || b == c || c == a {
			continue
		}
		faces = append(faces, [3]int{a, b, c})
	}
	return out, faces
}

// compact drops vertices no face refers to any more.
//
// Straightening a split edge leaves its middle vertices behind, referenced by
// nothing. They are harmless to draw and fatal to count: the validator's Euler
// check is V-E+F, and a vertex that is not on the surface makes that arithmetic
// describe a different solid than the one that is there.
func compact(m *mesh.Mesh) {
	used := make([]int, len(m.Verts))
	for i := range used {
		used[i] = -1
	}
	verts := make([]geom.Vec3, 0, len(m.Verts))
	for fi := range m.Faces {
		for _, loop := range m.Faces[fi].Loops {
			for k, vi := range loop {
				if used[vi] < 0 {
					used[vi] = len(verts)
					verts = append(verts, m.Verts[vi])
				}
				loop[k] = used[vi]
			}
		}
	}
	m.Verts = verts
	m.InvalidateCaches()
}

// dropHollowShells removes connected pieces that enclose no volume.
//
// Coincident input surfaces can leave Manifold with a pair of back-to-back
// faces: a closed, perfectly legal, perfectly empty shell. It costs nothing in
// volume — the numbers agree with Manifold's either way — but it is two faces
// on top of each other, which is exactly what "duplicate faces" means to the
// validator, and it would be two things to click on in the same place.
//
// The test is enclosed volume, not orientation. A real cavity inside a solid
// also has negative volume and has to stay.
func dropHollowShells(verts []geom.Vec3, tris [][3]int, origins []origin) ([][3]int, []origin) {
	// Anything thinner than a weld in every direction is below the resolution
	// this document can represent at all.
	const hollow = geom.WeldDist * geom.WeldDist * geom.WeldDist

	parent := make([]int, len(verts))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		if ra, rb := find(a), find(b); ra != rb {
			parent[ra] = rb
		}
	}
	for _, t := range tris {
		union(t[0], t[1])
		union(t[1], t[2])
	}

	volumes := map[int]float64{}
	for _, t := range tris {
		a, b, c := verts[t[0]], verts[t[1]], verts[t[2]]
		volumes[find(t[0])] += a.Dot(b.Cross(c)) / 6
	}

	keptTris := make([][3]int, 0, len(tris))
	keptOrigins := make([]origin, 0, len(origins))
	for i, t := range tris {
		if math.Abs(volumes[find(t[0])]) < hollow {
			continue
		}
		keptTris = append(keptTris, t)
		keptOrigins = append(keptOrigins, origins[i])
	}
	return keptTris, keptOrigins
}

// patch is one flat face's worth of triangles, with the lineage it inherits.
type patch struct {
	tris []int
	from origin
}

// patches groups the result's triangles into the flat faces a person would
// recognise: connected, coplanar, and safe to treat as one surface.
//
// Two things pull in opposite directions here. A source face can come out of a
// boolean in pieces — a bar cut in two leaves two halves that both remember the
// same face — and those pieces have to become separate faces. But two *different*
// source faces that end up coplanar and adjacent, which is exactly what a flush
// butt-join produces, have to become one: stacking two boxes should leave a wall
// you can drag, not two half-walls with a seam down the middle.
//
// So the grouping is by geometry, not by provenance, with one exception. Paint
// is anchored to a face, so merging two painted faces would have to throw one
// picture away. Those stay apart; everything else merges.
func patches(verts []geom.Vec3, tris [][3]int, triOrigin []origin) []patch {
	type edge struct{ a, b int }
	key := func(a, b int) edge {
		if a > b {
			a, b = b, a
		}
		return edge{a, b}
	}
	byEdge := map[edge][]int{}
	for ti := range tris {
		t := tris[ti]
		for e := 0; e < 3; e++ {
			k := key(t[e], t[(e+1)%3])
			byEdge[k] = append(byEdge[k], ti)
		}
	}

	seen := make([]bool, len(tris))
	var out []patch
	// Slivers never seed a patch: a patch needs a plane, and a sliver has no
	// opinion about which one. They are picked up as neighbours instead, and
	// anything still homeless at the end is dealt with after.
	for seed := range tris {
		if seen[seed] {
			continue
		}
		n, d, ok := triPlane(verts, tris[seed])
		if !ok {
			continue
		}
		group := []int{seed}
		seen[seed] = true
		from := triOrigin[seed]
		for qi := 0; qi < len(group); qi++ {
			t := tris[group[qi]]
			for e := 0; e < 3; e++ {
				for _, nb := range byEdge[key(t[e], t[(e+1)%3])] {
					if seen[nb] || !coplanar(verts, tris[nb], n, d) {
						continue
					}
					other := triOrigin[nb]
					if !mergeable(from, other) {
						continue
					}
					seen[nb] = true
					group = append(group, nb)
					from = inherit(from, other)
				}
			}
		}
		out = append(out, patch{tris: group, from: from})
	}
	// Slivers with no solid neighbour at all. There should be none; emitting
	// them keeps the surface closed if there ever are.
	for ti := range tris {
		if !seen[ti] {
			out = append(out, patch{tris: []int{ti}, from: triOrigin[ti]})
		}
	}
	return out
}

// mergeable reports whether two lineages can share one face. Faces carrying
// different paint cannot: there is no honest way to keep both pictures.
func mergeable(a, b origin) bool {
	if a.src == b.src || a.paint == b.paint {
		return true
	}
	return a.paint == nil || b.paint == nil
}

// inherit picks the lineage a merged face keeps: the painted side, if there is
// one, so a picture is never dropped by a merge.
func inherit(a, b origin) origin {
	keepEdges := a.keepEdges || b.keepEdges
	if a.paint == nil && b.paint != nil {
		b.keepEdges = keepEdges
		return b
	}
	a.keepEdges = keepEdges
	return a
}

// triPlane is a triangle's unit normal and plane offset, and whether it has one
// worth believing.
//
// A triangle whose corners are nearly collinear does have a normal in the
// arithmetic sense, and it is noise: the cross product of two nearly parallel
// edges points wherever the last bit of rounding sent it. The test is relative
// to the edge lengths, because "is this triangle flat" is a question about
// shape, not about size, and a long thin sliver has a much larger cross product
// than a small honest triangle.
func triPlane(verts []geom.Vec3, t [3]int) (geom.Vec3, float64, bool) {
	a, b, c := verts[t[0]], verts[t[1]], verts[t[2]]
	ab, ac := b.Sub(a), c.Sub(a)
	cross := ab.Cross(ac)
	if cross.Len() <= 1e-9*ab.Len()*ac.Len() {
		return geom.Vec3{}, 0, false
	}
	n, ok := cross.NormalizeOK()
	if !ok {
		return geom.Vec3{}, 0, false
	}
	return n, n.Dot(a), true
}

// coplanar reports whether a triangle lies in the given plane, facing the same
// way. The distance tolerance is half a weld: closer than that and the vertices
// would have been merged already.
func coplanar(verts []geom.Vec3, t [3]int, n geom.Vec3, d float64) bool {
	for _, i := range t {
		if math.Abs(verts[i].Dot(n)-d) > geom.WeldDist/2 {
			return false
		}
	}
	tn, _, ok := triPlane(verts, t)
	if !ok {
		// A flat sliver lies in every plane through its corners, and having no
		// direction of its own it cannot disagree with one. Letting it join the
		// patch it touches is the whole point: left on its own it becomes a
		// face with no area, which is neither useful nor valid.
		return true
	}
	return tn.Dot(n) >= 1-1e-9
}

// traceLoops turns a connected coplanar patch into an outer loop and its holes.
//
// Every directed edge that appears exactly once is on the boundary: an interior
// edge is walked once each way by the two triangles sharing it, and cancels.
// Following what is left, always turning into an unused edge, walks the
// boundary; the loop enclosing the most area is the outer one and the rest are
// holes, which is exactly the convention `mesh.Face` wants.
func traceLoops(verts []geom.Vec3, tris [][3]int, patch []int) ([][][]int, bool) {
	type dir struct{ a, b int }
	count := map[dir]int{}
	for _, ti := range patch {
		t := tris[ti]
		count[dir{t[0], t[1]}]++
		count[dir{t[1], t[2]}]++
		count[dir{t[2], t[0]}]++
	}
	next := map[int][]int{}
	total := 0
	for e, n := range count {
		if n != 1 || count[dir{e.b, e.a}] > 0 {
			continue
		}
		next[e.a] = append(next[e.a], e.b)
		total++
	}
	if total < 3 {
		return nil, false
	}
	// A deterministic walk needs a deterministic choice at every fork, and a
	// deterministic place to start. Map iteration order in Go is randomised, so
	// leaving either to it would make the same boolean produce different face
	// numbering from one run to the next — which breaks golden shots, undo
	// replay, and any hope of reproducing a bug report.
	starts := make([]int, 0, len(next))
	for k := range next {
		sort.Ints(next[k])
		starts = append(starts, k)
	}
	sort.Ints(starts)

	n, _, ok := triPlane(verts, tris[patch[0]])
	if !ok {
		return nil, false
	}

	used := 0
	var loops [][]int
	for _, start := range starts {
		for len(next[start]) > 0 {
			loop := []int{}
			at := start
			for {
				outs := next[at]
				if len(outs) == 0 {
					return nil, false
				}
				to := outs[0]
				next[at] = outs[1:]
				used++
				if used > total {
					return nil, false
				}
				loop = append(loop, at)
				at = to
				if at == start {
					break
				}
			}
			if len(loop) < 3 {
				return nil, false
			}
			loops = append(loops, splitSelfTouching(loop)...)
		}
	}
	if used != total || len(loops) == 0 {
		return nil, false
	}
	for _, l := range loops {
		if len(l) < 3 {
			return nil, false
		}
	}

	// Sort the loops into outer boundaries and holes by which way they wind.
	//
	// A patch usually has exactly one outer loop, but not always: two regions of
	// the same face can meet at a single vertex, which leaves them connected by
	// the region grow and yet bounded separately. That is two faces, not one
	// face with a strange hole, and calling it one was worth twelve backwards
	// edges before this said so.
	f := geom.FrameFromNormal(verts[loops[0][0]], n)
	type bounded struct {
		loop  []int
		area  float64
		holes [][]int
	}
	var outers []*bounded
	var holes [][]int
	for _, l := range loops {
		if a := signedArea(verts, l, n); a > 0 {
			outers = append(outers, &bounded{loop: l, area: a})
		} else {
			holes = append(holes, l)
		}
	}
	if len(outers) == 0 {
		return nil, false
	}
	for _, h := range holes {
		// A hole belongs to the tightest outer loop that encloses it.
		best := -1
		for i, o := range outers {
			if !loopContains(verts, f, o.loop, verts[h[0]]) {
				continue
			}
			if best < 0 || o.area < outers[best].area {
				best = i
			}
		}
		if best < 0 {
			best = 0
		}
		outers[best].holes = append(outers[best].holes, h)
	}

	faces := make([][][]int, 0, len(outers))
	for _, o := range outers {
		faces = append(faces, append([][]int{o.loop}, o.holes...))
	}
	return faces, true
}

// splitSelfTouching cuts a boundary walk that visits a vertex twice into the
// simple loops it is really made of.
//
// A face can be pinched to a point: two lobes of the same flat region meeting
// at a single corner, which a cut through the middle of a face produces without
// trying. The boundary walk goes round one lobe, through the shared corner, and
// round the other, so it comes back as one loop naming that corner twice. A
// polygon with a repeated vertex is not something the rest of the document can
// use, and it is two polygons that happen to touch.
func splitSelfTouching(loop []int) [][]int {
	var out [][]int
	pos := make(map[int]int, len(loop))
	stack := make([]int, 0, len(loop))
	for _, v := range loop {
		if at, seen := pos[v]; seen {
			// Everything since the last visit closes a loop of its own.
			if sub := append([]int(nil), stack[at:]...); len(sub) >= 3 {
				out = append(out, sub)
			}
			for _, u := range stack[at+1:] {
				delete(pos, u)
			}
			stack = stack[:at+1]
			continue
		}
		pos[v] = len(stack)
		stack = append(stack, v)
	}
	if len(stack) >= 3 {
		out = append(out, stack)
	}
	return out
}

// loopContains is a point-in-polygon test in the loop's own plane.
func loopContains(verts []geom.Vec3, f geom.Frame, loop []int, p geom.Vec3) bool {
	q := f.ToLocal(p)
	in := false
	for i := range loop {
		a := f.ToLocal(verts[loop[i]])
		b := f.ToLocal(verts[loop[(i+1)%len(loop)]])
		if (a.Y > q.Y) != (b.Y > q.Y) &&
			q.X < (b.X-a.X)*(q.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// splitPinchedVertices gives each fan of faces around a vertex its own copy of
// that vertex.
//
// A cut can leave a solid touching itself at a single point: two lobes meeting
// at one corner, like an hourglass. Manifold allows that — it guarantees every
// *edge* has two faces, which this still satisfies — but the surface there is
// not a disc, and a document that assumes it is will draw the wrong outline and
// select the wrong thing. It also makes the Euler count come out odd, which is
// the validator noticing the same problem from the other end.
//
// Two lobes sharing a point become two lobes with a point each, in the same
// place. Nothing moves; the surface just stops pretending it is joined there.
func splitPinchedVertices(m *mesh.Mesh) {
	type site struct{ fi, li, k int }
	corners := make([][]site, len(m.Verts))
	for fi := range m.Faces {
		for li, loop := range m.Faces[fi].Loops {
			for k := range loop {
				corners[loop[k]] = append(corners[loop[k]], site{fi, li, k})
			}
		}
	}

	for v := 0; v < len(corners); v++ {
		cs := corners[v]
		if len(cs) < 2 {
			continue
		}
		// Corners that share an edge out of v are the same fan.
		parent := make([]int, len(cs))
		for i := range parent {
			parent[i] = i
		}
		var find func(int) int
		find = func(i int) int {
			for parent[i] != i {
				parent[i] = parent[parent[i]]
				i = parent[i]
			}
			return i
		}
		byNeighbour := map[int][]int{}
		for i, s := range cs {
			loop := m.Faces[s.fi].Loops[s.li]
			n := len(loop)
			for _, nb := range [2]int{loop[(s.k-1+n)%n], loop[(s.k+1)%n]} {
				byNeighbour[nb] = append(byNeighbour[nb], i)
			}
		}
		for _, group := range byNeighbour {
			for _, other := range group[1:] {
				if a, b := find(group[0]), find(other); a != b {
					parent[a] = b
				}
			}
		}

		// The first fan keeps the original vertex; every other fan gets a copy.
		// Corners are visited in index order so which fan is "first" — and
		// therefore the whole numbering — is the same on every run.
		assigned := map[int]int{}
		for i := range cs {
			root := find(i)
			idx, seen := assigned[root]
			if !seen {
				if len(assigned) == 0 {
					idx = v
				} else {
					idx = len(m.Verts)
					m.Verts = append(m.Verts, m.Verts[v])
				}
				assigned[root] = idx
			}
			m.Faces[cs[i].fi].Loops[cs[i].li][cs[i].k] = idx
		}
	}
	m.InvalidateCaches()
}

// straightenLoops drops vertices that sit in the middle of a straight run.
//
// A boolean splits edges wherever another solid crossed them, and those extra
// vertices stay behind even when the cut left nothing on that side. Without
// this, two boxes joined face to face come back as one box with a dozen
// pointless vertices along its sides instead of the six clean quads it is.
//
// The decision has to be made across the whole mesh at once, not loop by loop.
// A vertex can lie in the middle of one face's edge and be a genuine corner of
// the face next door — a T-junction, which a boolean produces constantly. Drop
// it from the first and the two faces stop agreeing on where their shared edge
// runs, and a solid that was closed a moment ago has a crack in it. So a vertex
// goes only if every loop it appears in agrees it is not a corner.
func straightenLoops(m *mesh.Mesh) {
	corner := make([]bool, len(m.Verts))
	for fi := range m.Faces {
		for _, loop := range m.Faces[fi].Loops {
			for i := range loop {
				if !collinearAt(m.Verts, loop, i) {
					corner[loop[i]] = true
				}
			}
		}
	}
	for fi := range m.Faces {
		for li, loop := range m.Faces[fi].Loops {
			kept := make([]int, 0, len(loop))
			for _, vi := range loop {
				if corner[vi] {
					kept = append(kept, vi)
				}
			}
			if len(kept) >= 3 {
				m.Faces[fi].Loops[li] = kept
			}
		}
	}
	m.InvalidateCaches()
}

// collinearAt reports whether the loop runs straight on through its i'th
// vertex — carries on in the same direction, not merely along the same line.
//
// The distinction matters. A boundary can walk out along a line and come
// straight back, which happens wherever a face is pinched to nothing against
// its neighbour, and both turns there are 180 degrees. Those points look
// collinear to a cross product and are anything but removable: delete the tip
// of that spur and its two sides fuse into an edge that never existed, running
// clean past the vertices the faces on the other side are still holding on to.
// The dot product is what tells the two cases apart.
func collinearAt(verts []geom.Vec3, loop []int, i int) bool {
	n := len(loop)
	prev := verts[loop[(i-1+n)%n]]
	cur := verts[loop[i]]
	next := verts[loop[(i+1)%n]]
	a := cur.Sub(prev)
	b := next.Sub(cur)
	if a.Dot(b) <= 0 {
		return false
	}
	// The cross product's length is |a||b|sin(theta): comparing it against a
	// fraction of |a||b| is an angle test that needs no normalising and no
	// division, and it stays honest for very short edges.
	return a.Cross(b).Len() <= 1e-9*a.Len()*b.Len()
}

// signedArea is the loop's area in its own plane, positive when it winds
// counter-clockwise seen from the normal's side.
func signedArea(verts []geom.Vec3, loop []int, n geom.Vec3) float64 {
	var sum geom.Vec3
	for i := range loop {
		a := verts[loop[i]]
		b := verts[loop[(i+1)%len(loop)]]
		sum = sum.Add(a.Cross(b))
	}
	return sum.Dot(n) / 2
}

func reverse(l []int) {
	for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}
}
