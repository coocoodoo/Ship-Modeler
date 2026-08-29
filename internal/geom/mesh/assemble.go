package mesh

import (
	"fmt"
	"math"
	"sort"

	"modeler/internal/geom"
)

// Building a Mesh out of triangle soup (the user's request, 2026-08-28).
//
// STL stores every triangle with its own three vertices and shares nothing;
// OBJ shares vertices but usually arrives triangulated all the same. Handing
// either straight to this program would produce a body whose every face is a
// triangle — and a triangle is the one shape none of the tools here want.
// Paint allocates a picture per face, push/pull drags a face, sketch-on-face
// needs somewhere flat and sizeable to draw. A thousand-triangle import would
// technically load and be useless.
//
// So the assembler does the four things that turn soup back into a solid:
// weld the duplicated corners, drop what is degenerate, agree on which way is
// out, and merge coplanar triangles into the polygons they were cut from.

// Tri3 is one input triangle in world space.
type Tri3 struct{ A, B, C geom.Vec3 }

// itri is a welded triangle: corners as vertex indices, plus the normal it had
// once its corners landed on the grid.
type itri struct {
	v      [3]int
	normal geom.Vec3
}

// AssembleOpts tunes the import.
type AssembleOpts struct {
	// Scale multiplies every coordinate before snapping. Mesh files carry no
	// units — an STL out of CAD is usually millimetres — so the caller decides
	// what one file unit is worth here. Zero means 1.
	Scale float64
	// Center moves the result's bounding-box centre to the origin.
	Center bool
}

// AssembleStats is what the import can tell the user afterwards. An importer
// that silently changed the model would be worse than one that refused.
type AssembleStats struct {
	InputTris int
	Dropped   int
	Verts     int
	Faces     int
	// Merged counts faces built from more than one triangle, which is the
	// number that says whether the merge did anything worth doing.
	Merged int
	// OpenEdges and NonManifold are why a result may not be a valid solid.
	OpenEdges   int
	NonManifold int
}

// Assemble builds a mesh from triangle soup.
//
// It returns the mesh even when the result is not a closed solid: a model with
// a hole in it is still worth showing, and the caller has the stats to say so.
// Only input it cannot make a mesh from at all is an error.
func Assemble(tris []Tri3, opts AssembleOpts) (*Mesh, AssembleStats, error) {
	st := AssembleStats{InputTris: len(tris)}
	if len(tris) == 0 {
		return nil, st, fmt.Errorf("that file has no triangles in it")
	}
	scale := opts.Scale
	if scale == 0 {
		scale = 1
	}

	// --- weld -------------------------------------------------------------
	//
	// Snapping to the subunit grid before welding is what makes coincidence
	// exact rather than approximate: two corners that were the same point in
	// the file land on the same integer, and every later step can compare
	// vertex indices instead of distances. It is also what the rest of this
	// program expects — every body built here already lives on that grid.
	var (
		verts []geom.Vec3
		index = map[[3]int64]int{}
	)
	vertOf := func(p geom.Vec3) int {
		p = geom.SnapVec3ToSubunits(p.Mul(scale))
		key := [3]int64{
			int64(math.Round(p.X * geom.Unit)),
			int64(math.Round(p.Y * geom.Unit)),
			int64(math.Round(p.Z * geom.Unit)),
		}
		if i, ok := index[key]; ok {
			return i
		}
		index[key] = len(verts)
		verts = append(verts, p)
		return len(verts) - 1
	}

	var faces []itri
	for _, t := range tris {
		a, b, c := vertOf(t.A), vertOf(t.B), vertOf(t.C)
		if a == b || b == c || c == a {
			st.Dropped++ // two corners welded together: no area left
			continue
		}
		cross := verts[b].Sub(verts[a]).Cross(verts[c].Sub(verts[a]))
		if cross.Len()/2 < DegenerateArea {
			st.Dropped++ // three distinct points on one line
			continue
		}
		faces = append(faces, itri{v: [3]int{a, b, c}, normal: cross.Normalize()})
	}
	if len(faces) == 0 {
		return nil, st, fmt.Errorf("every triangle in that file is degenerate")
	}

	orientShells(faces, verts)

	m := &Mesh{Verts: verts}
	regions, merged := mergeCoplanar(faces, verts)
	for _, r := range regions {
		loops := regionLoops(r, faces, verts)
		if len(loops) == 0 {
			continue
		}
		m.Faces = append(m.Faces, Face{
			ID:    MakeFaceUID(0, uint32(len(m.Faces)+1)),
			Loops: loops,
		})
	}
	if len(m.Faces) == 0 {
		return nil, st, fmt.Errorf("no face boundaries could be traced")
	}

	// Dropped triangles and merged-away detail can leave welded corners that no
	// loop names any more, and an unreferenced vertex is a validation failure.
	compactVerts(m)

	if opts.Center {
		b := m.AABB()
		mid := b.Min.Add(b.Max).Mul(0.5)
		Transform(m, geom.Translate(geom.Vec3{X: -mid.X, Y: -mid.Y, Z: -mid.Z}))
		for i := range m.Verts {
			m.Verts[i] = geom.SnapVec3ToSubunits(m.Verts[i])
		}
		m.InvalidateCaches()
	}

	// A solid assembled inside-out has exactly the negative of the right
	// volume, and nothing downstream would notice until a boolean went wrong.
	if Volume(m) < 0 {
		for fi := range m.Faces {
			for li := range m.Faces[fi].Loops {
				reverseInts(m.Faces[fi].Loops[li])
			}
		}
		m.InvalidateCaches()
	}

	st.Verts, st.Faces, st.Merged = len(m.Verts), len(m.Faces), merged
	st.OpenEdges, st.NonManifold = edgeTrouble(m)
	m.RecheckPlanarity()
	return m, st, nil
}

// orientShells makes every triangle agree with its neighbours about which side
// is out, by walking the surface and flipping whatever disagrees.
//
// STL in particular is notorious for this: the format stores a normal per
// facet that nothing is obliged to honour, and plenty of writers wind the
// corners however they please.
func orientShells(faces []itri, verts []geom.Vec3) {
	// Which triangles use each undirected edge.
	type edgeKey [2]int
	uses := map[edgeKey][]int{}
	key := func(a, b int) edgeKey {
		if a > b {
			a, b = b, a
		}
		return edgeKey{a, b}
	}
	for i := range faces {
		v := faces[i].v
		uses[key(v[0], v[1])] = append(uses[key(v[0], v[1])], i)
		uses[key(v[1], v[2])] = append(uses[key(v[1], v[2])], i)
		uses[key(v[2], v[0])] = append(uses[key(v[2], v[0])], i)
	}

	// Does this triangle traverse a->b in that direction?
	forward := func(i, a, b int) bool {
		v := faces[i].v
		return (v[0] == a && v[1] == b) || (v[1] == a && v[2] == b) || (v[2] == a && v[0] == b)
	}
	flip := func(i int) {
		faces[i].v[1], faces[i].v[2] = faces[i].v[2], faces[i].v[1]
		faces[i].normal = faces[i].normal.Mul(-1)
	}

	seen := make([]bool, len(faces))
	for start := range faces {
		if seen[start] {
			continue
		}
		seen[start] = true
		queue := []int{start}
		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			v := faces[i].v
			for e := 0; e < 3; e++ {
				a, b := v[e], v[(e+1)%3]
				for _, j := range uses[key(a, b)] {
					if j == i || seen[j] {
						continue
					}
					// Neighbours of a consistent surface traverse their shared
					// edge in opposite directions. Same direction means one of
					// them is wound the wrong way.
					if forward(j, a, b) {
						flip(j)
					}
					seen[j] = true
					queue = append(queue, j)
				}
			}
			v = faces[i].v // flips above may have changed it
			_ = v
		}
	}
}

// mergeCoplanar groups triangles into regions that share one plane, and
// reports how many regions came from more than one triangle.
//
// The test is deliberately tight. Adjacent facets of a tessellated cylinder
// are nearly coplanar and must not merge, or a round hull comes back as a
// polygon and the shape is gone. Two triangles join only when their normals
// agree to within a hair and every corner of the growing region stays inside
// the planarity tolerance the rest of the program already enforces.
func mergeCoplanar(faces []itri, verts []geom.Vec3) ([][]int, int) {
	type edgeKey [2]int
	key := func(a, b int) edgeKey {
		if a > b {
			a, b = b, a
		}
		return edgeKey{a, b}
	}
	uses := map[edgeKey][]int{}
	for i := range faces {
		v := faces[i].v
		uses[key(v[0], v[1])] = append(uses[key(v[0], v[1])], i)
		uses[key(v[1], v[2])] = append(uses[key(v[1], v[2])], i)
		uses[key(v[2], v[0])] = append(uses[key(v[2], v[0])], i)
	}

	const normalDot = 1 - 1e-6 // about a twentieth of a degree
	seen := make([]bool, len(faces))
	var regions [][]int
	merged := 0

	for start := range faces {
		if seen[start] {
			continue
		}
		n := faces[start].normal
		d := verts[faces[start].v[0]].Dot(n)
		seen[start] = true
		region := []int{start}
		queue := []int{start}

		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			v := faces[i].v
			for e := 0; e < 3; e++ {
				for _, j := range uses[key(v[e], v[(e+1)%3])] {
					if seen[j] || faces[j].normal.Dot(n) < normalDot {
						continue
					}
					// Every corner must sit on the region's plane. Snapping
					// moves vertices, so a face that was exactly planar in the
					// file may only be planar to within a subunit here — which
					// is the tolerance the mesh itself is held to.
					flat := true
					for _, vi := range faces[j].v {
						if math.Abs(verts[vi].Dot(n)-d) > geom.PlanarDist {
							flat = false
							break
						}
					}
					if !flat {
						continue
					}
					seen[j] = true
					region = append(region, j)
					queue = append(queue, j)
				}
			}
		}
		if len(region) > 1 {
			merged++
		}
		regions = append(regions, region)
	}
	return regions, merged
}

// regionLoops traces the boundary of a merged region: the edges used once
// inside it, chained into loops, with the outer one first.
func regionLoops(region []int, faces []itri, verts []geom.Vec3) [][]int {
	// Directed edges of the region. Anything used in both directions is
	// interior to the region and disappears with the merge.
	type dir [2]int
	count := map[dir]int{}
	for _, i := range region {
		v := faces[i].v
		count[dir{v[0], v[1]}]++
		count[dir{v[1], v[2]}]++
		count[dir{v[2], v[0]}]++
	}
	next := map[int][]int{}
	for e, n := range count {
		if n == 0 {
			continue
		}
		if count[dir{e[1], e[0]}] > 0 {
			continue // interior: walked from both sides
		}
		next[e[0]] = append(next[e[0]], e[1])
	}
	// A stable order so the same soup always assembles the same way.
	for k := range next {
		sort.Ints(next[k])
	}

	n := faces[region[0]].normal
	var loops [][]int
	used := map[dir]bool{}
	for start := range next {
		for _, first := range next[start] {
			if used[dir{start, first}] {
				continue
			}
			loop := []int{start}
			a, b := start, first
			for {
				used[dir{a, b}] = true
				if b == start {
					break
				}
				loop = append(loop, b)
				var step = -1
				for _, c := range next[b] {
					if !used[dir{b, c}] {
						step = c
						break
					}
				}
				if step < 0 {
					break // an open chain: not a loop, drop it
				}
				a, b = b, step
			}
			if len(loop) >= 3 && b == start {
				loops = append(loops, dropCollinear(loop, verts))
			}
		}
	}
	if len(loops) == 0 {
		return nil
	}

	// The outer loop is the one with the largest area; the rest are holes and
	// must wind against it, which is what Validate insists on.
	area := func(loop []int) float64 { return regionLoopArea(loop, verts, n) }
	sort.SliceStable(loops, func(i, j int) bool {
		return math.Abs(area(loops[i])) > math.Abs(area(loops[j]))
	})
	outer := area(loops[0])
	for i := 1; i < len(loops); i++ {
		if sign(area(loops[i])) == sign(outer) {
			reverseInts(loops[i])
		}
	}
	// Guard against a trace that produced nothing usable.
	for _, l := range loops {
		if len(l) < 3 {
			return nil
		}
	}
	return loops
}

// dropCollinear removes the points that sit in the middle of a straight run.
//
// A merged region's boundary comes out of a triangulation, so it is littered
// with them: the shared corner of two triangles that met along one straight
// edge of the original polygon. Left in, a rectangle has twelve corners where
// it has four, and everything that walks a loop pays for them.
func dropCollinear(loop []int, verts []geom.Vec3) []int {
	if len(loop) < 3 {
		return loop
	}
	out := make([]int, 0, len(loop))
	n := len(loop)
	for i := 0; i < n; i++ {
		prev := verts[loop[(i-1+n)%n]]
		cur := verts[loop[i]]
		next := verts[loop[(i+1)%n]]
		a := cur.Sub(prev)
		b := next.Sub(cur)
		if a.Cross(b).Len() > geom.WeldDist*a.Len() {
			out = append(out, loop[i])
		}
	}
	if len(out) < 3 {
		return loop
	}
	return out
}

// loopArea is twice the signed area of a loop projected onto a plane normal.
func regionLoopArea(loop []int, verts []geom.Vec3, n geom.Vec3) float64 {
	var sum geom.Vec3
	for i := range loop {
		a := verts[loop[i]]
		b := verts[loop[(i+1)%len(loop)]]
		sum = sum.Add(a.Cross(b))
	}
	return sum.Dot(n) / 2
}

func reverseInts(a []int) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

// edgeTrouble counts the two ways an imported mesh fails to be a solid, so the
// import can say which one it hit rather than only that something is wrong.
func edgeTrouble(m *Mesh) (open, nonManifold int) {
	t := m.Topo()
	for ei := range t.Edges {
		switch n := len(t.Edges[ei].Uses); {
		case n == 1:
			open++
		case n > 2:
			nonManifold++
		}
	}
	return open, nonManifold
}

// compactVerts removes vertices no loop refers to and renumbers the rest.
func compactVerts(m *Mesh) {
	keep := make([]int, len(m.Verts))
	for i := range keep {
		keep[i] = -1
	}
	var verts []geom.Vec3
	for fi := range m.Faces {
		for li := range m.Faces[fi].Loops {
			for k, vi := range m.Faces[fi].Loops[li] {
				if keep[vi] < 0 {
					keep[vi] = len(verts)
					verts = append(verts, m.Verts[vi])
				}
				m.Faces[fi].Loops[li][k] = keep[vi]
			}
		}
	}
	m.Verts = verts
	m.InvalidateCaches()
}
