package mesh

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// Assembling a mesh from triangle soup (the user's request, 2026-08-28).
//
// STL has no shared vertices at all and OBJ rarely has usable faces, so an
// import that stopped at "load the triangles" would hand this program a body
// whose every face is a triangle. Paint would have nothing to paint on,
// push/pull nothing to pull, and sketch-on-face nothing flat to sketch on.
// So the assembler welds, orients and — the part that matters — merges
// coplanar triangles back into the polygons they came from.

// boxSoup is a unit-ish box as twelve loose triangles, in the order and with
// the duplicated corners an STL would have.
func boxSoup(lo, hi geom.Vec3) []Tri3 {
	c := [8]geom.Vec3{
		{X: lo.X, Y: lo.Y, Z: lo.Z}, {X: hi.X, Y: lo.Y, Z: lo.Z},
		{X: hi.X, Y: hi.Y, Z: lo.Z}, {X: lo.X, Y: hi.Y, Z: lo.Z},
		{X: lo.X, Y: lo.Y, Z: hi.Z}, {X: hi.X, Y: lo.Y, Z: hi.Z},
		{X: hi.X, Y: hi.Y, Z: hi.Z}, {X: lo.X, Y: hi.Y, Z: hi.Z},
	}
	quads := [6][4]int{
		{0, 3, 2, 1}, {4, 5, 6, 7}, // -z, +z
		{0, 1, 5, 4}, {2, 3, 7, 6}, // -y, +y
		{1, 2, 6, 5}, {0, 4, 7, 3}, // +x, -x
	}
	var out []Tri3
	for _, q := range quads {
		out = append(out,
			Tri3{c[q[0]], c[q[1]], c[q[2]]},
			Tri3{c[q[0]], c[q[2]], c[q[3]]})
	}
	return out
}

func TestABoxOfTrianglesComesBackAsSixFaces(t *testing.T) {
	m, st, err := Assemble(boxSoup(geom.Vec3{}, geom.Vec3{X: 4, Y: 3, Z: 2}), AssembleOpts{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(m.Faces) != 6 {
		t.Fatalf("a box came back as %d faces, want 6 — coplanar triangles were "+
			"not merged, so every face of the import is a triangle", len(m.Faces))
	}
	if len(m.Verts) != 8 {
		t.Errorf("a box came back with %d vertices, want 8 welded corners", len(m.Verts))
	}
	for fi := range m.Faces {
		if got := len(m.Faces[fi].Outer()); got != 4 {
			t.Errorf("face %d has %d corners, want the 4 of a rectangle", fi, got)
		}
	}
	if err := Validate(m); err != nil {
		t.Errorf("the assembled box is not a valid solid: %v", err)
	}
	if st.InputTris != 12 || st.Faces != 6 {
		t.Errorf("stats say %d tris in, %d faces out", st.InputTris, st.Faces)
	}
}

// Volume is the honest check that winding survived: a solid assembled
// inside-out has exactly the negative of the right answer.
func TestAssemblyOrientsTheSolidOutward(t *testing.T) {
	// Every triangle wound backwards on the way in.
	soup := boxSoup(geom.Vec3{}, geom.Vec3{X: 4, Y: 3, Z: 2})
	for i := range soup {
		soup[i].B, soup[i].C = soup[i].C, soup[i].B
	}
	m, _, err := Assemble(soup, AssembleOpts{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got := Volume(m); math.Abs(got-24) > 1e-9 {
		t.Errorf("volume %v, want 24 — an inside-out import was not turned back", got)
	}
}

// The merge must not flatten curvature. A tessellated cylinder's side facets
// are adjacent and nearly coplanar; merging them would turn a round thing into
// a polygon with one enormous face and destroy the shape.
func TestNearlyCoplanarFacetsAreNotMerged(t *testing.T) {
	const seg = 24
	var soup []Tri3
	top, bot := 2.0, 0.0
	at := func(i int, y float64) geom.Vec3 {
		a := 2 * math.Pi * float64(i%seg) / seg
		return geom.Vec3{X: 3 * math.Cos(a), Y: y, Z: 3 * math.Sin(a)}
	}
	for i := 0; i < seg; i++ {
		a0, a1 := at(i, bot), at(i+1, bot)
		b0, b1 := at(i, top), at(i+1, top)
		soup = append(soup, Tri3{a0, a1, b1}, Tri3{a0, b1, b0})
		// Caps, fanned from the first corner.
		soup = append(soup,
			Tri3{at(0, bot), at(i+1, bot), at(i, bot)},
			Tri3{at(0, top), at(i, top), at(i+1, top)})
	}
	m, _, err := Assemble(soup, AssembleOpts{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	// Two caps plus one face per side facet: the round wall must survive.
	if len(m.Faces) < seg {
		t.Errorf("a %d-sided cylinder came back as %d faces — the merge ate the "+
			"curvature", seg, len(m.Faces))
	}
	if err := Validate(m); err != nil {
		t.Errorf("the assembled cylinder is not a valid solid: %v", err)
	}
}

// A merged face's boundary comes out of a triangulation, so it is full of
// vertices sitting in the middle of straight runs. They have to go, or the
// face has twelve corners where it has four and every downstream thing that
// walks a loop does twelve times the work for the same shape.
func TestMergedLoopsDropCollinearCorners(t *testing.T) {
	// A 2x2 grid of quads on one plane, each split into two triangles: the
	// merged square's boundary passes through eight points, four of which are
	// mid-edge.
	soup := boxSoup(geom.Vec3{}, geom.Vec3{X: 4, Y: 4, Z: 4})
	// Split the +z face into four quads by hand.
	out := soup[:0]
	for _, tr := range soup {
		if tr.A.Z == 4 && tr.B.Z == 4 && tr.C.Z == 4 {
			continue
		}
		out = append(out, tr)
	}
	q := func(x0, y0, x1, y1 float64) {
		a := geom.Vec3{X: x0, Y: y0, Z: 4}
		b := geom.Vec3{X: x1, Y: y0, Z: 4}
		c := geom.Vec3{X: x1, Y: y1, Z: 4}
		d := geom.Vec3{X: x0, Y: y1, Z: 4}
		out = append(out, Tri3{a, b, c}, Tri3{a, c, d})
	}
	q(0, 0, 2, 2)
	q(2, 0, 4, 2)
	q(0, 2, 2, 4)
	q(2, 2, 4, 4)

	m, _, err := Assemble(out, AssembleOpts{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for fi := range m.Faces {
		if n := m.FaceNormal(fi); n.Z < 0.999 {
			continue
		}
		if got := len(m.Faces[fi].Outer()); got != 4 {
			t.Errorf("the merged top face has %d corners, want 4 — collinear "+
				"points from the triangulation were left in", got)
		}
	}
}

// Scale is applied before the snap, because snapping first would quantise the
// model at the wrong size and a millimetre part would collapse to nothing.
func TestScaleIsAppliedBeforeSnapping(t *testing.T) {
	// A box 2 mm across. At 1/256-unit snapping it would round to a sliver;
	// scaled up by 10 first it is a clean 20-unit box.
	m, _, err := Assemble(boxSoup(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}),
		AssembleOpts{Scale: 10})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b := m.AABB()
	if got := b.Max.X - b.Min.X; math.Abs(got-20) > 1e-9 {
		t.Errorf("the scaled box is %v units across, want 20", got)
	}
}

func TestDegenerateTrianglesAreDroppedNotAssembled(t *testing.T) {
	soup := boxSoup(geom.Vec3{}, geom.Vec3{X: 4, Y: 3, Z: 2})
	p := geom.Vec3{X: 1, Y: 1, Z: 1}
	soup = append(soup, Tri3{p, p, p}, Tri3{p, p, geom.Vec3{X: 2, Y: 1, Z: 1}})
	m, st, err := Assemble(soup, AssembleOpts{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if st.Dropped != 2 {
		t.Errorf("dropped %d degenerate triangles, want 2", st.Dropped)
	}
	if err := Validate(m); err != nil {
		t.Errorf("degenerate input broke the result: %v", err)
	}
}
