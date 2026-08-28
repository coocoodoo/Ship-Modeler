package mesh

import (
	"math"

	"modeler/internal/geom"
)

// Folding bent faces into flat ones (the user's request, 2026-08-28).
//
// A polygon face that a direct edit has left non-planar does not really exist:
// the renderer and the boolean kernel only ever see its triangulation, so the
// surface actually shown folds along whichever diagonals the ear-clipper
// happened to pick — a crease the user never drew, in a place nobody chose.
// Folding makes the crease real instead: the face splits into planar pieces
// joined by proper edges, and the mesh says what the screen shows.
//
// The crease people expect is the line between what moved and what stayed.
// That is knowledge only the edit has — geometry alone cannot tell a face
// that bent on its left from one that bent on its right — which is why the
// moved set is an argument rather than something inferred here.

// FoldBent splits every bent single-loop face of m into planar pieces and
// returns how many faces it split. New pieces get identities from nextID and
// inherit the source face's paint and lineage exactly as boolean fragments do
// (SPEC-GEOMETRY §8.4). Faces with holes are left alone: a chord there would
// have to dodge the hole, and a wrong guess would be worse than the bend.
//
// moved says which vertex indices the edit displaced. It steers the choice of
// crease; correctness does not depend on it.
func FoldBent(m *Mesh, moved map[int]bool, nextID func() FaceUID) int {
	folded := 0
	// Iterating by index over a slice that grows is deliberate: pieces are
	// appended planar, so revisiting them is a cheap planarity check and the
	// loop terminates. Replaced faces are rewritten in place (first piece)
	// plus appended (the rest), keeping face order stable for determinism.
	for fi := 0; fi < len(m.Faces); fi++ {
		f := &m.Faces[fi]
		if len(f.Loops) != 1 {
			continue
		}
		if m.Planarity(fi) <= geom.PlanarDist {
			continue
		}
		pieces := foldLoop(m, f.Outer(), moved)
		if len(pieces) < 2 {
			continue
		}
		src := f.SrcFace
		if src == NoFace {
			src = f.ID
		}
		paint := f.Paint
		// The first piece replaces the original in place; the rest append.
		// Every piece is a new face with a new identity: the original was one
		// surface and none of the pieces is it, which is the same reasoning
		// the boolean kernel applies to its fragments.
		for i, loop := range pieces {
			nf := Face{ID: nextID(), Loops: [][]int{loop}, SrcFace: src, Paint: paint}
			if i == 0 {
				m.Faces[fi] = nf
			} else {
				m.Faces = append(m.Faces, nf)
			}
		}
		folded++
	}
	if folded > 0 {
		m.InvalidateCaches()
		m.RecheckPlanarity()
	}
	return folded
}

// foldLoop splits one loop into planar pieces, recursively. The result is the
// loop itself when it is already flat enough, cannot be split, or is a
// triangle — a triangle is always planar.
func foldLoop(m *Mesh, loop []int, moved map[int]bool) [][]int {
	if len(loop) <= 3 || loopPlanarity(m, loop) <= geom.PlanarDist {
		return [][]int{loop}
	}
	i, j, ok := creaseChord(m, loop, moved)
	if !ok {
		return [][]int{loop}
	}
	a, b := splitLoop(loop, i, j)
	return append(foldLoop(m, a, moved), foldLoop(m, b, moved)...)
}

// creaseChord picks the pair of loop positions to split at.
//
// First choice: the chord separating the moved run from the still one — the
// two unmoved vertices flanking a single contiguous run of moved ones. That is
// the "reference line" a person would draw. When there is no such chord (the
// whole loop moved, several separate runs, or the flanks are neighbours
// already), fall back to the diagonal that leaves the flattest pair of pieces.
func creaseChord(m *Mesh, loop []int, moved map[int]bool) (int, int, bool) {
	n := len(loop)
	if i, j, ok := runFlanks(loop, moved); ok && !adjacent(i, j, n) {
		if chordUsable(m, loop, i, j) {
			return i, j, true
		}
	}
	// Best diagonal: minimise the worse piece's bend. Ties resolve to the
	// lowest (i, j), so the same loop always folds the same way.
	best, bestScore := [2]int{-1, -1}, math.Inf(1)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if adjacent(i, j, n) || !chordUsable(m, loop, i, j) {
				continue
			}
			a, b := splitLoop(loop, i, j)
			score := math.Max(loopPlanarity(m, a), loopPlanarity(m, b))
			if score < bestScore-1e-12 {
				best, bestScore = [2]int{i, j}, score
			}
		}
	}
	if best[0] < 0 {
		return 0, 0, false
	}
	return best[0], best[1], true
}

// runFlanks finds the single contiguous run of moved vertices in the loop and
// returns the positions of the unmoved vertices flanking it. It reports false
// when there is no moved vertex, no still vertex, or more than one run.
func runFlanks(loop []int, moved map[int]bool) (int, int, bool) {
	n := len(loop)
	transitions := 0
	start := -1 // position where a still->moved transition happens
	for i := 0; i < n; i++ {
		prev := moved[loop[(i+n-1)%n]]
		cur := moved[loop[i]]
		if !prev && cur {
			transitions++
			start = i
		}
	}
	if transitions != 1 {
		return 0, 0, false
	}
	end := start
	for moved[loop[(end+1)%n]] {
		end = (end + 1) % n
	}
	// The flanking still vertices sit just outside the run.
	return (start + n - 1) % n, (end + 1) % n, true
}

// adjacent reports whether two loop positions are neighbours, in which case
// the "chord" is an existing boundary edge and splits nothing.
func adjacent(i, j, n int) bool {
	d := j - i
	if d < 0 {
		d = -d
	}
	return d <= 1 || d == n-1
}

// chordUsable rejects chords that would produce a degenerate piece: fewer
// than three vertices (impossible for a non-adjacent chord, kept as a guard)
// or a piece with no area to its name.
func chordUsable(m *Mesh, loop []int, i, j int) bool {
	a, b := splitLoop(loop, i, j)
	return len(a) >= 3 && len(b) >= 3 && loopArea(m, a) > 0 && loopArea(m, b) > 0
}

// splitLoop cuts a cyclic loop along the chord between positions i and j,
// keeping the original winding in both pieces. Both pieces contain both chord
// vertices; that shared pair is the new crease edge.
func splitLoop(loop []int, i, j int) (a, b []int) {
	n := len(loop)
	for k := i; ; k = (k + 1) % n {
		a = append(a, loop[k])
		if k == j {
			break
		}
	}
	for k := j; ; k = (k + 1) % n {
		b = append(b, loop[k])
		if k == i {
			break
		}
	}
	return a, b
}

// loopPlanarity is the largest distance from any loop vertex to the loop's own
// Newell plane — Planarity for a loop that is not (yet) a face.
func loopPlanarity(m *Mesh, loop []int) float64 {
	nrm, centroid, ok := loopPlane(m, loop)
	if !ok {
		return math.Inf(1)
	}
	d := nrm.Dot(centroid)
	worst := 0.0
	for _, vi := range loop {
		if dist := math.Abs(nrm.Dot(m.Verts[vi]) - d); dist > worst {
			worst = dist
		}
	}
	return worst
}

// loopArea is the magnitude of the loop's Newell vector: twice its area. Zero
// means the loop is a sliver or a line.
func loopArea(m *Mesh, loop []int) float64 {
	acc := newellVector(m, loop)
	return acc.Len()
}

// loopPlane is the loop's Newell plane.
func loopPlane(m *Mesh, loop []int) (n geom.Vec3, centroid geom.Vec3, ok bool) {
	if len(loop) < 3 {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	acc := newellVector(m, loop)
	unit, ok := acc.NormalizeOK()
	if !ok {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	for _, vi := range loop {
		centroid = centroid.Add(m.Verts[vi])
	}
	return unit, centroid.Mul(1 / float64(len(loop))), true
}

// newellVector accumulates Newell's method over a loop, the same formula the
// face plane cache uses.
func newellVector(m *Mesh, loop []int) geom.Vec3 {
	var acc geom.Vec3
	n := len(loop)
	for i := 0; i < n; i++ {
		a := m.Verts[loop[i]]
		b := m.Verts[loop[(i+1)%n]]
		acc.X += (a.Y - b.Y) * (a.Z + b.Z)
		acc.Y += (a.Z - b.Z) * (a.X + b.X)
		acc.Z += (a.X - b.X) * (a.Y + b.Y)
	}
	return acc
}
