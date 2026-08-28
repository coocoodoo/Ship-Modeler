package paint

import (
	"image"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Painting a line along an edge (the user's request, 2026-08-27).
//
// The point of the feature is the corner: a panel line that runs along an edge
// has to land on *both* faces that meet there, in register, or it reads as two
// separate lines that nearly line up. So the band is measured from the edge
// inward on each face, and these tests are about where the paint lands rather
// than about it landing at all.

// paintedCube is a 8x8x8 box with every face given a texture.
func paintedCube(t *testing.T, res int) *mesh.Mesh {
	t.Helper()
	m := mesh.Box(geom.Vec3{X: -4, Y: -4, Z: -4}, geom.Vec3{X: 4, Y: 4, Z: 4}, 1)
	for i := range m.Faces {
		p, err := Allocate(m, i, res)
		if err != nil {
			t.Fatalf("allocating face %d: %v", i, err)
		}
		m.Faces[i].Paint = p
	}
	return m
}

// countPainted reports how many texels of a face carry paint.
func countPainted(m *mesh.Mesh, fi int) int {
	p := m.Faces[fi].Paint
	r := FaceRect(m, fi, p)
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if At(p, image.Point{X: x, Y: y}).A != 0 {
				n++
			}
		}
	}
	return n
}

// The band lands on the face, not in the margin beside it. A line drawn
// straight down the middle of the edge would put half its width on texels that
// are not on the face at all — invisible, and half the thickness that was
// asked for.
func TestAnEdgeBandLandsOnItsFace(t *testing.T) {
	m := paintedCube(t, 4)
	topo := m.Topo()
	if len(topo.Edges) == 0 {
		t.Fatal("a box has no edges")
	}

	e := topo.Edges[0]
	if len(e.Uses) != 2 {
		t.Fatalf("edge 0 has %d uses, want the 2 of a closed solid", len(e.Uses))
	}
	fi := e.Uses[0].Face
	p := m.Faces[fi].Paint
	const width = 3

	wrote := EdgeBand(m, fi, p, Brush{Color: red, Size: width}, m.Verts[e.A], m.Verts[e.B])
	if wrote.Empty() {
		t.Fatal("the band painted nothing")
	}
	face := FaceRect(m, fi, p)
	if !wrote.In(face) {
		t.Errorf("the band wrote %v, which is not inside the face's %v — "+
			"paint outside the face is paint nobody can see", wrote, face)
	}

	// A band of width w along one edge of a 32-texel face covers about w x 32.
	got := countPainted(m, fi)
	if got < width*20 || got > width*40 {
		t.Errorf("the band covers %d texels; a %d-wide line along a 32-texel "+
			"edge should be near %d", got, width, width*32)
	}
}

// Both faces of the edge get the line, and each hugs the shared edge. That is
// the whole reason the feature exists: one press, one line, round the corner.
func TestAnEdgeBandReachesBothItsFaces(t *testing.T) {
	m := paintedCube(t, 4)
	topo := m.Topo()
	e := topo.Edges[0]

	for _, use := range e.Uses {
		fi := use.Face
		p := m.Faces[fi].Paint
		if wrote := EdgeBand(m, fi, p, Brush{Color: red, Size: 2}, m.Verts[e.A], m.Verts[e.B]); wrote.Empty() {
			t.Fatalf("face %d took no paint from the shared edge", fi)
		}
		if countPainted(m, fi) == 0 {
			t.Errorf("face %d reports no painted texels", fi)
		}
	}
}

// The band sits against the edge rather than somewhere in the middle of the
// face: every painted texel is within the band's width of the edge.
func TestAnEdgeBandHugsItsEdge(t *testing.T) {
	m := paintedCube(t, 4)
	topo := m.Topo()
	e := topo.Edges[0]
	fi := e.Uses[0].Face
	p := m.Faces[fi].Paint
	const width = 2

	EdgeBand(m, fi, p, Brush{Color: red, Size: width}, m.Verts[e.A], m.Verts[e.B])

	// The edge in this face's texel space.
	ea := Texel(p, m.Verts[e.A])
	eb := Texel(p, m.Verts[e.B])
	face := FaceRect(m, fi, p)
	for y := face.Min.Y; y < face.Max.Y; y++ {
		for x := face.Min.X; x < face.Max.X; x++ {
			at := image.Point{X: x, Y: y}
			if At(p, at).A == 0 {
				continue
			}
			if d := distanceToSegment(at, ea, eb); d > float64(width)+1.5 {
				t.Fatalf("a texel at %v is %.1f from the edge %v..%v, "+
					"further than the %d-wide band should reach", at, d, ea, eb, width)
			}
		}
	}
}

// A wider setting paints more.
func TestAWiderEdgeBandPaintsMore(t *testing.T) {
	thin := paintedCube(t, 4)
	thick := paintedCube(t, 4)
	for _, tc := range []struct {
		m    *mesh.Mesh
		size int
	}{{thin, 1}, {thick, 4}} {
		e := tc.m.Topo().Edges[0]
		fi := e.Uses[0].Face
		EdgeBand(tc.m, fi, tc.m.Faces[fi].Paint,
			Brush{Color: red, Size: tc.size}, tc.m.Verts[e.A], tc.m.Verts[e.B])
	}
	e := thin.Topo().Edges[0]
	fi := e.Uses[0].Face
	a, b := countPainted(thin, fi), countPainted(thick, fi)
	if b <= a {
		t.Errorf("a 4-wide band painted %d texels and a 1-wide one %d", b, a)
	}
}

// distanceToSegment is the distance from a texel to a texel-space segment.
func distanceToSegment(p, a, b image.Point) float64 {
	ax, ay := float64(a.X), float64(a.Y)
	bx, by := float64(b.X), float64(b.Y)
	px, py := float64(p.X), float64(p.Y)
	dx, dy := bx-ax, by-ay
	den := dx*dx + dy*dy
	t := 0.0
	if den > 0 {
		t = ((px-ax)*dx + (py-ay)*dy) / den
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
	}
	cx, cy := ax+t*dx, ay+t*dy
	return hypot(px-cx, py-cy)
}

func hypot(a, b float64) float64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	return sqrt(a*a + b*b)
}

func sqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 30; i++ {
		x = 0.5 * (x + v/x)
	}
	return x
}

// --- The command ----------------------------------------------------------

// One press is one undo. An edge line touches two faces per edge, and a tool
// that needed four undos to take back one click would be worse than no tool.
func TestPaintingEdgesIsOneUndoStep(t *testing.T) {
	bus, b, _, _ := painted(t)
	m := b.Mesh
	edges := []int{0, 1, 2}

	cmd := &StrokeEdges{Body: b.ID, Edges: edges, Color: red, Size: 2, Res: 4}
	if err := bus.Run(cmd); err != nil {
		t.Fatalf("StrokeEdges: %v", err)
	}

	// Every face along those edges took paint.
	want := map[int]bool{}
	for _, e := range edges {
		for _, fi := range FacesOfEdge(m, e) {
			want[fi] = true
		}
	}
	if len(want) < 2 {
		t.Fatalf("three edges reached %d faces", len(want))
	}
	for fi := range want {
		if m.Faces[fi].Paint == nil {
			t.Errorf("face %d has no picture after being painted", fi)
		} else if countPainted(m, fi) == 0 {
			t.Errorf("face %d has a picture but nothing in it", fi)
		}
	}

	// One step back takes all of it.
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	for fi := range want {
		if m.Faces[fi].Paint != nil {
			t.Errorf("face %d still has a picture after undo; those faces were bare", fi)
		}
	}
}

// Undo has to put back what was there, not merely remove what was added.
func TestPaintingEdgesOverPaintRestoresIt(t *testing.T) {
	bus, b, uid, fi := painted(t)
	// Fill the face first, so the edge line lands on top of something.
	if err := bus.Run(stroke(b.ID, uid, 32, ToolFill, blue, image.Point{X: 8, Y: 8})); err != nil {
		t.Fatal(err)
	}
	before := countPainted(b.Mesh, fi)
	if before == 0 {
		t.Fatal("the fill painted nothing")
	}

	// An edge of that face.
	edge := -1
	for i := range b.Mesh.Topo().Edges {
		for _, f := range FacesOfEdge(b.Mesh, i) {
			if f == fi {
				edge = i
			}
		}
		if edge >= 0 {
			break
		}
	}
	if edge < 0 {
		t.Fatal("that face has no edges")
	}

	if err := bus.Run(&StrokeEdges{
		Body: b.ID, Edges: []int{edge}, Color: red, Size: 2, Res: 4,
	}); err != nil {
		t.Fatalf("StrokeEdges: %v", err)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if got := countPainted(b.Mesh, fi); got != before {
		t.Errorf("after undo the face has %d painted texels, want the %d it had", got, before)
	}
	if got := At(b.Mesh.Faces[fi].Paint, image.Point{X: 8, Y: 8}); got != blue {
		t.Errorf("the fill came back as %v, want %v", got, blue)
	}
}

func TestPaintingNoEdgesIsRefused(t *testing.T) {
	bus, b, _, _ := painted(t)
	if err := bus.Run(&StrokeEdges{Body: b.ID, Color: red, Size: 1, Res: 4}); err == nil {
		t.Error("painting an empty edge selection was accepted")
	}
}
