package mesh

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"modeler/internal/geom"
)

// The validation gate (SPEC-GEOMETRY §6.5). Every mutating operation runs this
// before its result is allowed into the document; a failure aborts the command
// atomically and leaves the document untouched (SPEC-UX §11.3).

// DegenerateArea is the smallest face area that counts as real: below this a
// face is a sliver artefact rather than geometry.
const DegenerateArea = geom.WeldDist * geom.WeldDist

// Report is the outcome of a validation run.
type Report struct {
	Problems     []string
	Shells       int
	Verts        int
	Edges        int
	Faces        int
	Volume       float64
	GenusPer     []int
	Unreferenced int
}

// OK reports whether the mesh passed.
func (r Report) OK() bool { return len(r.Problems) == 0 }

// Err returns an error describing every problem found, or nil.
func (r Report) Err() error {
	if r.OK() {
		return nil
	}
	return fmt.Errorf("invalid mesh: %s", strings.Join(r.Problems, "; "))
}

// Validate runs the full gate and returns an error listing everything wrong.
func Validate(m *Mesh) error { return Check(m).Err() }

// Check runs the gate and returns the full report, which callers use for
// diagnostics and tests.
func Check(m *Mesh) Report {
	r := Report{Verts: len(m.Verts), Faces: len(m.Faces)}
	add := func(format string, args ...any) {
		r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
	}

	if len(m.Faces) == 0 {
		add("mesh has no faces")
		return r
	}

	// Vertex indices must be in range before anything else touches them.
	for fi := range m.Faces {
		for li, loop := range m.Faces[fi].Loops {
			if len(loop) < 3 {
				add("face %d loop %d has %d vertices, need 3", fi, li, len(loop))
				continue
			}
			for _, vi := range loop {
				if vi < 0 || vi >= len(m.Verts) {
					add("face %d loop %d references vertex %d of %d", fi, li, vi, len(m.Verts))
					return r // indices are unusable; stop before dereferencing
				}
			}
			if dup := duplicateInLoop(loop); dup >= 0 {
				add("face %d loop %d repeats vertex %d", fi, li, dup)
			}
		}
	}
	if !r.OK() {
		return r
	}

	t := m.Topo()
	r.Edges = len(t.Edges)

	// Closed and 2-manifold: every edge used exactly twice, opposite directions.
	nonManifold, wrongOrient := 0, 0
	for ei := range t.Edges {
		e := &t.Edges[ei]
		if len(e.Uses) != 2 {
			nonManifold++
			continue
		}
		if e.Uses[0].Forward == e.Uses[1].Forward {
			wrongOrient++
		}
	}
	if nonManifold > 0 {
		add("%d edges are not shared by exactly 2 faces (mesh is open or non-manifold)", nonManifold)
	}
	if wrongOrient > 0 {
		add("%d edges are traversed the same way by both faces (inconsistent orientation)", wrongOrient)
	}

	// Degenerate faces.
	degenerate := 0
	for fi := range m.Faces {
		if math.Abs(m.FaceArea(fi)) < DegenerateArea {
			degenerate++
		}
	}
	if degenerate > 0 {
		add("%d faces are degenerate (area below %g)", degenerate, DegenerateArea)
	}

	// Duplicate faces (same vertex set) mean the mesh was assembled twice.
	if dupes := duplicateFaces(m); dupes > 0 {
		add("%d duplicate faces", dupes)
	}

	// Hole loops must wind opposite to their outer loop.
	for fi := range m.Faces {
		f := &m.Faces[fi]
		if len(f.Loops) < 2 {
			continue
		}
		n := m.FaceNormal(fi)
		outerSign := sign(m.loopSignedArea(f.Outer(), n))
		for li, hole := range f.Holes() {
			if sign(m.loopSignedArea(hole, n)) == outerSign {
				add("face %d hole %d winds the same way as its outer loop", fi, li+1)
			}
		}
	}

	shells := m.Shells()
	r.Shells = len(shells)
	r.Volume = Volume(m)
	if r.Volume <= 0 {
		add("signed volume is %g, want positive (faces may be inside-out)", r.Volume)
	}

	// Per-shell Euler sanity: V - E + F - H = 2 - 2g with g >= 0.
	//
	// H counts hole loops: a face with a hole is an annulus, not a disk, and
	// contributes one less to the Euler characteristic. Without this term a
	// tube would report genus 0.
	for si, shell := range shells {
		v, e, f, h := shellCounts(m, shell)
		chi := v - e + f - h
		if chi%2 != 0 {
			add("shell %d has odd Euler characteristic %d", si, chi)
			continue
		}
		g := (2 - chi) / 2
		r.GenusPer = append(r.GenusPer, g)
		if g < 0 {
			add("shell %d has negative genus %d (V=%d E=%d F=%d H=%d)", si, g, v, e, f, h)
		}
	}

	// Unreferenced vertices are not fatal but are always a bug upstream.
	used := make([]bool, len(m.Verts))
	for fi := range m.Faces {
		for _, loop := range m.Faces[fi].Loops {
			for _, vi := range loop {
				used[vi] = true
			}
		}
	}
	for _, u := range used {
		if !u {
			r.Unreferenced++
		}
	}
	if r.Unreferenced > 0 {
		add("%d unreferenced vertices", r.Unreferenced)
	}

	return r
}

func duplicateInLoop(loop []int) int {
	seen := make(map[int]bool, len(loop))
	for _, v := range loop {
		if seen[v] {
			return v
		}
		seen[v] = true
	}
	return -1
}

func duplicateFaces(m *Mesh) int {
	seen := make(map[string]bool, len(m.Faces))
	dupes := 0
	for fi := range m.Faces {
		key := faceKey(m.Faces[fi].Outer())
		if seen[key] {
			dupes++
			continue
		}
		seen[key] = true
	}
	return dupes
}

func faceKey(loop []int) string {
	s := append([]int(nil), loop...)
	sort.Ints(s)
	var b strings.Builder
	for _, v := range s {
		fmt.Fprintf(&b, "%d,", v)
	}
	return b.String()
}

// shellCounts returns the vertex, edge, face and hole-loop counts of one shell.
func shellCounts(m *Mesh, shell []int) (v, e, f, h int) {
	t := m.Topo()
	verts := make(map[int]bool)
	edges := make(map[int]bool)
	for _, fi := range shell {
		h += len(m.Faces[fi].Holes())
		for _, loop := range m.Faces[fi].Loops {
			n := len(loop)
			for i := 0; i < n; i++ {
				verts[loop[i]] = true
				if ei := t.EdgeIndex(loop[i], loop[(i+1)%n]); ei >= 0 {
					edges[ei] = true
				}
			}
		}
	}
	return len(verts), len(edges), len(shell), h
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}
