// Package sketch2d is the integer-exact 2D engine behind sketches: the planar
// arrangement that answers "is this profile closed?" (R3), region extraction
// with holes, and triangulation.
//
// Everything here runs on int64 subunits (SPEC-GEOMETRY §1.1). There is no
// epsilon anywhere in this package, which is why the answer is deterministic
// forever rather than tuned.
package sketch2d

import (
	"sort"

	"modeler/internal/geom"
)

// Source names the sketch entity and the segment within it that produced an
// edge. It rides through the whole pipeline so extrude can give each side face
// a stable identity (SPEC-GEOMETRY §5.5).
type Source struct {
	Entity int
	Seg    int
}

// Seg is one input segment with its provenance.
type Seg struct {
	A, B geom.Vec2i
	Src  Source
}

// Loop is a closed boundary. Src is parallel to Pts: Src[i] is the source of
// the edge leaving Pts[i].
type Loop struct {
	Pts []geom.Vec2i
	Src []Source
}

// Area2 is twice the signed area: positive counter-clockwise, negative
// clockwise. Exact.
func (l Loop) Area2() int64 { return geom.ShoelaceArea2(l.Pts) }

// Region is one filled area: a counter-clockwise outer loop and any clockwise
// holes directly inside it.
type Region struct {
	Outer Loop
	Holes []Loop
}

// Area2 is twice the region's filled area, holes removed.
func (r Region) Area2() int64 {
	a := r.Outer.Area2()
	for _, h := range r.Holes {
		a += h.Area2() // holes wind the other way, so this subtracts
	}
	return a
}

// Entities returns the distinct sketch entities on the region's boundary.
func (r Region) Entities() []int {
	seen := map[int]bool{}
	var out []int
	add := func(l Loop) {
		for _, s := range l.Src {
			if s.Entity >= 0 && !seen[s.Entity] {
				seen[s.Entity] = true
				out = append(out, s.Entity)
			}
		}
	}
	add(r.Outer)
	for _, h := range r.Holes {
		add(h)
	}
	sort.Ints(out)
	return out
}

// Arrangement is the whole result of a sketch's planar subdivision.
type Arrangement struct {
	// Regions are the closed areas, ordered deterministically.
	Regions []Region
	// OpenEnds are the loose endpoints — nodes of degree one — which the UI
	// draws as red rings and which gate extrude (R3, SPEC-UX §8.6).
	OpenEnds []geom.Vec2i
}

// Area2 is twice the total filled area.
func (a Arrangement) Area2() int64 {
	var sum int64
	for _, r := range a.Regions {
		sum += r.Area2()
	}
	return sum
}

// Closed reports whether the sketch has no loose ends.
func (a Arrangement) Closed() bool { return len(a.OpenEnds) == 0 }

// Build runs the whole pipeline of SPEC-GEOMETRY §4: split, weld, graph, face
// walk, nesting. It is exact and deterministic; the same segments in any order
// and any direction give the same arrangement.
func Build(segs []Seg) Arrangement {
	pieces := split(segs)
	g := buildGraph(pieces)
	cycles := g.walkFaces()
	return assemble(g, cycles)
}

// ---------------------------------------------------------------------------
// Step 1 and 2: quantise and split.

// split cuts every segment at every point another segment meets it, and drops
// zero-length input.
//
// Collinear overlaps need no special interval arithmetic: both segments get
// split at the shared interval's endpoints, so the overlapping pieces come out
// as the identical node pair and the graph's edge deduplication merges them.
// That is the same "maximal non-overlapping pieces" the spec describes, reached
// by construction rather than by bookkeeping.
func split(segs []Seg) []Seg {
	// Drop degenerate input up front so nothing downstream has to think about it.
	live := make([]Seg, 0, len(segs))
	for _, s := range segs {
		if s.A != s.B {
			live = append(live, s)
		}
	}

	cuts := make([][]geom.Vec2i, len(live))
	for i := range live {
		cuts[i] = []geom.Vec2i{live[i].A, live[i].B}
	}

	// The split is O(n^2), so the reject has to be cheap. Hoisting each
	// segment's bounding box out of the inner loop keeps the overwhelming
	// majority of pairs from ever reaching the intersection routine.
	type box struct{ minX, maxX, minY, maxY int64 }
	boxes := make([]box, len(live))
	for i, s := range live {
		b := box{minX: s.A.X, maxX: s.A.X, minY: s.A.Y, maxY: s.A.Y}
		if s.B.X < b.minX {
			b.minX = s.B.X
		} else {
			b.maxX = s.B.X
		}
		if s.B.Y < b.minY {
			b.minY = s.B.Y
		} else {
			b.maxY = s.B.Y
		}
		boxes[i] = b
	}

	for i := 0; i < len(live); i++ {
		bi := boxes[i]
		for j := i + 1; j < len(live); j++ {
			bj := boxes[j]
			if bi.maxX < bj.minX || bj.maxX < bi.minX ||
				bi.maxY < bj.minY || bj.maxY < bi.minY {
				continue
			}
			hit := geom.SegSegIntersect(live[i].A, live[i].B, live[j].A, live[j].B)
			switch hit.Rel {
			case geom.SegNone:
				continue
			case geom.SegCollinear:
				cuts[i] = append(cuts[i], hit.Q0, hit.Q1)
				cuts[j] = append(cuts[j], hit.Q0, hit.Q1)
			default:
				cuts[i] = append(cuts[i], hit.P)
				cuts[j] = append(cuts[j], hit.P)
			}
		}
	}

	out := make([]Seg, 0, len(live)*2)
	for i, s := range live {
		out = appendPieces(out, s, cuts[i])
	}
	return out
}

// appendPieces orders a segment's cut points along it and emits the runs
// between them.
func appendPieces(dst []Seg, s Seg, cuts []geom.Vec2i) []Seg {
	dir := s.B.Sub(s.A)
	// Order by projection onto the segment. A cut point rounded off the line by
	// half a subunit still sorts correctly, which is what lets the rounding in
	// SegSegIntersect be harmless here.
	//
	// An insertion sort, not sort.Slice: a segment usually carries two or three
	// cut points, and at that size the reflection-free version is several times
	// faster and allocates nothing.
	for i := 1; i < len(cuts); i++ {
		c := cuts[i]
		pc := c.Sub(s.A).Dot(dir)
		j := i - 1
		for j >= 0 {
			p := cuts[j].Sub(s.A).Dot(dir)
			if p < pc || (p == pc && !lexLess(c, cuts[j])) {
				break
			}
			cuts[j+1] = cuts[j]
			j--
		}
		cuts[j+1] = c
	}

	prev := cuts[0]
	for _, c := range cuts[1:] {
		if c == prev {
			continue
		}
		dst = append(dst, Seg{A: prev, B: c, Src: s.Src})
		prev = c
	}
	return dst
}

// ---------------------------------------------------------------------------
// Step 3 and 4: weld into nodes, build the undirected graph.

type graph struct {
	pts   []geom.Vec2i
	index map[geom.Vec2i]int

	// edges are undirected, deduplicated, each stored once.
	edges []edge
	// darts holds the two directed uses of every edge: 2*e and 2*e+1.
	darts []dart
	// around[n] lists the darts leaving node n, sorted counter-clockwise.
	around [][]int
}

type edge struct {
	A, B int
	Src  Source
}

type dart struct {
	from, to int
	edge     int
}

func buildGraph(pieces []Seg) *graph {
	g := &graph{index: make(map[geom.Vec2i]int, len(pieces)*2)}

	node := func(p geom.Vec2i) int {
		if i, ok := g.index[p]; ok {
			return i
		}
		i := len(g.pts)
		g.pts = append(g.pts, p)
		g.index[p] = i
		return i
	}

	// Weld by exact coordinate, then drop repeated node pairs. Drawing the same
	// edge twice, or retracing one, collapses here.
	type key struct{ a, b int }
	seen := make(map[key]bool, len(pieces))
	for _, s := range pieces {
		a, b := node(s.A), node(s.B)
		if a == b {
			continue
		}
		k := key{a, b}
		if a > b {
			k = key{b, a}
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		g.edges = append(g.edges, edge{A: k.a, B: k.b, Src: s.Src})
	}

	g.around = make([][]int, len(g.pts))
	for ei, e := range g.edges {
		g.darts = append(g.darts,
			dart{from: e.A, to: e.B, edge: ei},
			dart{from: e.B, to: e.A, edge: ei})
		g.around[e.A] = append(g.around[e.A], 2*ei)
		g.around[e.B] = append(g.around[e.B], 2*ei+1)
	}

	// Sort each node's outgoing darts by angle. The comparator is exact: a
	// half-plane test plus a cross-product sign, never atan2.
	// Insertion sort again: a node has two or three incident edges in almost
	// every sketch, and this runs once per node on every rebuild.
	for n := range g.around {
		list := g.around[n]
		origin := g.pts[n]
		dir := func(d int) geom.Vec2i { return g.pts[g.darts[d].to].Sub(origin) }
		for i := 1; i < len(list); i++ {
			d := list[i]
			dd := dir(d)
			j := i - 1
			for j >= 0 && angleLess(dd, dir(list[j])) {
				list[j+1] = list[j]
				j--
			}
			list[j+1] = d
		}
	}
	return g
}

// upperHalf splits directions into the two halves the angular sort needs:
// angles in [0,180) come first.
func upperHalf(d geom.Vec2i) bool {
	return d.Y > 0 || (d.Y == 0 && d.X > 0)
}

// angleLess orders directions counter-clockwise starting from +X, exactly.
func angleLess(a, b geom.Vec2i) bool {
	ua, ub := upperHalf(a), upperHalf(b)
	if ua != ub {
		return ua
	}
	// Within a half turn the cross product alone decides.
	if c := a.CrossZ(b); c != 0 {
		return c > 0
	}
	// Exactly collinear and same direction: order by length so the sort is
	// total. Two darts from one node can only be collinear if they are
	// distinct nodes along the same ray, which the split step allows.
	return a.LenSq() < b.LenSq()
}

// twin returns the dart running the other way along the same edge.
func twin(d int) int { return d ^ 1 }

// next returns the following dart of the face to the left of d: the dart
// immediately clockwise from d's twin around its head node (SPEC-GEOMETRY §4.5).
//
// With this rule, bounded faces come out counter-clockwise and each connected
// component's outer boundary comes out clockwise.
func (g *graph) next(d int) int {
	t := twin(d)
	head := g.darts[t].from
	list := g.around[head]
	for i, cand := range list {
		if cand == t {
			return list[(i-1+len(list))%len(list)]
		}
	}
	return t // unreachable for a well-formed graph
}

// ---------------------------------------------------------------------------
// Step 5: face walk.

type cycle struct {
	darts []int
	pts   []geom.Vec2i
	src   []Source
	area2 int64
}

func (g *graph) walkFaces() []cycle {
	visited := make([]bool, len(g.darts))
	var out []cycle

	for start := range g.darts {
		if visited[start] {
			continue
		}
		var c cycle
		d := start
		for !visited[d] {
			visited[d] = true
			c.darts = append(c.darts, d)
			c.pts = append(c.pts, g.pts[g.darts[d].from])
			c.src = append(c.src, g.edges[g.darts[d].edge].Src)
			d = g.next(d)
		}
		c.area2 = geom.ShoelaceArea2(c.pts)
		out = append(out, c)
	}
	return out
}

// ---------------------------------------------------------------------------
// Steps 6 and 7: nesting and open ends.

func assemble(g *graph, cycles []cycle) Arrangement {
	var a Arrangement

	// A cycle with positive area bounds a face and is a candidate region. A
	// cycle with negative area is a connected component's outer boundary and
	// may be a hole. Zero area means the component is all dangles — a pure open
	// chain — which bounds nothing.
	var faces []int
	var boundaries []int
	for i := range cycles {
		switch {
		case cycles[i].area2 > 0:
			faces = append(faces, i)
		case cycles[i].area2 < 0:
			boundaries = append(boundaries, i)
		}
	}

	// Each outer boundary sinks into the smallest face that strictly contains
	// it. Distinct components never share a point after splitting and welding,
	// so testing one of its vertices is unambiguous: a vertex of a component's
	// own boundary lies *on* its own faces, never strictly inside them.
	holesOf := make(map[int][]int, len(boundaries))
	for _, b := range boundaries {
		probe := cycles[b].pts[0]
		best, bestArea := -1, int64(0)
		for _, f := range faces {
			inside, onEdge := geom.PointInPolygon(probe, cycles[f].pts)
			if !inside || onEdge {
				continue
			}
			if best < 0 || cycles[f].area2 < bestArea {
				best, bestArea = f, cycles[f].area2
			}
		}
		if best >= 0 {
			holesOf[best] = append(holesOf[best], b)
		}
	}

	for _, f := range faces {
		r := Region{Outer: canonical(cycles[f])}
		holes := holesOf[f]
		sort.Slice(holes, func(i, j int) bool {
			return lexLess(cycles[holes[i]].pts[0], cycles[holes[j]].pts[0])
		})
		for _, h := range holes {
			r.Holes = append(r.Holes, canonical(cycles[h]))
		}
		a.Regions = append(a.Regions, r)
	}

	// A stable order keeps region indices meaningful between frames, which the
	// selection and the golden shots both rely on.
	sort.Slice(a.Regions, func(i, j int) bool {
		pi, pj := a.Regions[i].Outer.Pts[0], a.Regions[j].Outer.Pts[0]
		if pi != pj {
			return lexLess(pi, pj)
		}
		return a.Regions[i].Area2() > a.Regions[j].Area2()
	})

	// Open ends are the degree-one nodes (SPEC-GEOMETRY §4.7).
	for n, darts := range g.around {
		if len(darts) == 1 {
			a.OpenEnds = append(a.OpenEnds, g.pts[n])
		}
	}
	sort.Slice(a.OpenEnds, func(i, j int) bool { return lexLess(a.OpenEnds[i], a.OpenEnds[j]) })

	return a
}

// canonical rotates a cycle to start at its lexicographically smallest vertex,
// so one loop always has one representation.
func canonical(c cycle) Loop {
	if len(c.pts) == 0 {
		return Loop{}
	}
	start := 0
	for i, p := range c.pts {
		if lexLess(p, c.pts[start]) {
			start = i
		}
	}
	n := len(c.pts)
	l := Loop{Pts: make([]geom.Vec2i, n), Src: make([]Source, n)}
	for i := 0; i < n; i++ {
		l.Pts[i] = c.pts[(start+i)%n]
		l.Src[i] = c.src[(start+i)%n]
	}
	return l
}

func lexLess(a, b geom.Vec2i) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}
