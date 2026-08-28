package mesh

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// The crease that would not draw (the user's report, 2026-08-28).
//
// A gentle bend folds a face into flat pieces meeting at a shallow angle —
// well under CreaseAngleDeg. The classifier called that edge smooth, so it was
// neither drawn nor pickable: the fold showed as a lighting seam with no line,
// and clicking it selected the face behind it. But a shallow angle between two
// pieces of what used to be ONE face is not the same thing as a shallow angle
// between two authored faces: the first is a crease the user just made, the
// second is a 16-gon pretending to be a cylinder (D-06). Lineage tells them
// apart.

// steppedWedge is the user's shape: a pentagon profile — tall at the back, a
// slope falling to a short front — extruded into a prism. Verts 0..4 are the
// bottom loop, 5..9 the top.
//
//	profile: (0,0) (6,0) (6,6) (3,6) (0,3), counter-clockwise
func steppedWedge() *Mesh {
	profile := []geom.Vec2{
		{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 6}, {X: 3, Y: 6}, {X: 0, Y: 3},
	}
	m := &Mesh{}
	for _, p := range profile {
		m.Verts = append(m.Verts, geom.Vec3{X: p.X, Y: p.Y, Z: 0})
	}
	for _, p := range profile {
		m.Verts = append(m.Verts, geom.Vec3{X: p.X, Y: p.Y, Z: 5})
	}
	n := len(profile)
	top := make([]int, n)
	bottom := make([]int, n)
	for i := 0; i < n; i++ {
		top[i] = n + i
		bottom[i] = n - 1 - i // reversed: outward is -Z
	}
	m.Faces = append(m.Faces,
		Face{ID: MakeFaceUID(1, 1), Loops: [][]int{bottom}},
		Face{ID: MakeFaceUID(1, 2), Loops: [][]int{top}},
	)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		m.Faces = append(m.Faces, Face{
			ID:    MakeFaceUID(1, uint32(3+i)),
			Loops: [][]int{{i, j, n + j, n + i}},
		})
	}
	return m
}

func TestSteppedWedgeIsAValidSolid(t *testing.T) {
	if err := Validate(steppedWedge()); err != nil {
		t.Fatalf("the fixture itself is broken: %v", err)
	}
}

// dihedralDeg is the angle between an edge's two faces, in degrees.
func dihedralDeg(m *Mesh, ei int) float64 {
	e := &m.Topo().Edges[ei]
	n0 := m.FaceNormal(e.Uses[0].Face)
	n1 := m.FaceNormal(e.Uses[1].Face)
	cos := math.Max(-1, math.Min(1, n0.Dot(n1)))
	return math.Acos(cos) * 180 / math.Pi
}

func TestAShallowFoldCreaseStillCountsAsAnEdge(t *testing.T) {
	m := steppedWedge()
	// Nudge the bottom of the slope gently inward — the user's bend. The slope
	// face, the left wall and the bottom all fold, and every crease is shallow.
	moved := map[int]bool{4: true}
	m.Verts[4] = m.Verts[4].Add(geom.Vec3{X: 0.4, Y: -0.4})
	m.InvalidateCaches()

	if folded := FoldBent(m, moved, uidCounter(100)); folded == 0 {
		t.Fatal("nothing folded — the fixture does not bend the way the report did")
	}
	if err := Validate(m); err != nil {
		t.Fatalf("folded wedge invalid: %v", err)
	}

	topo := m.Topo()
	creases := 0
	for ei := range topo.Edges {
		e := &topo.Edges[ei]
		f0, f1 := &m.Faces[e.Uses[0].Face], &m.Faces[e.Uses[1].Face]
		if f0.SrcFace == NoFace || f0.SrcFace != f1.SrcFace {
			continue
		}
		// Two pieces of one former face, meeting at a real angle: the crease
		// the user just made. It has to classify as drawn, however shallow.
		deg := dihedralDeg(m, ei)
		if deg < 1 {
			continue
		}
		creases++
		if deg >= CreaseAngleDeg {
			t.Fatalf("edge %d bends %v degrees — too steep to prove anything, soften the fixture", ei, deg)
		}
		if got := m.ClassifyEdge(ei); got != EdgeCrease {
			t.Errorf("fold crease %d (%.1f degrees, shared source) classifies as %v, want a drawn crease",
				ei, deg, got)
		}
	}
	if creases == 0 {
		t.Fatal("the bend produced no shallow same-source creases — the fixture proves nothing")
	}
}

// The other side of the contract: a 16-gon prism is a circle (D-06). Its side
// faces meet at 22.5 degrees — under the threshold, different faces — and must
// stay smooth, or every engine pod grows sixteen stripes.
func TestACirclePrismKeepsItsSmoothSides(t *testing.T) {
	m := NGonPrism(geom.PlaneFrame(geom.PlaneTop), 2.5, 16, 6, 1)
	topo := m.Topo()
	smooth := 0
	for ei := range topo.Edges {
		if deg := dihedralDeg(m, ei); deg > 1 && deg < CreaseAngleDeg {
			smooth++
			if got := m.ClassifyEdge(ei); got != EdgeSmooth {
				t.Errorf("prism side edge %d (%.1f degrees) classifies as %v, want smooth", ei, deg, got)
			}
		}
	}
	if smooth != 16 {
		t.Fatalf("found %d shallow side edges, want the prism's 16", smooth)
	}
}

// And coplanar pieces of one face — boolean fragments that merged flush, or a
// fold undone by a later move — still draw nothing: there is no crease there.
func TestCoplanarSameSourcePiecesStaySmooth(t *testing.T) {
	m := seamBox()
	src := MakeFaceUID(1, 50)
	// The two top quads lie in one plane; brand them as pieces of one face.
	m.Faces[2].SrcFace = src
	m.Faces[3].SrcFace = src
	m.InvalidateCaches()

	topo := m.Topo()
	checked := false
	for ei := range topo.Edges {
		e := &topo.Edges[ei]
		f0, f1 := &m.Faces[e.Uses[0].Face], &m.Faces[e.Uses[1].Face]
		if f0.SrcFace != src || f1.SrcFace != src {
			continue
		}
		checked = true
		if got := m.ClassifyEdge(ei); got != EdgeSmooth {
			t.Errorf("the flush seam between coplanar pieces classifies as %v, want smooth", got)
		}
	}
	if !checked {
		t.Fatal("the branded pieces share no edge — fixture broken")
	}
}
