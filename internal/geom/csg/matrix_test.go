package csg

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The CSG acceptance matrix of TESTING §3, written before the kernel it tests.
//
// Every case builds its inputs in code, runs the op, and asserts three things:
// the result passes `mesh.Validate`, its volume matches an analytic figure, and
// our volume agrees with Manifold's own. The third is the one that catches a
// converter that quietly drops or duplicates geometry — a mesh can be perfectly
// valid and still be the wrong solid.
//
// The named hard cases are the point of the exercise. Flush butt-joins, walls
// that land exactly on other walls, and touches at a single edge or vertex are
// where a boolean kernel either works or doesn't, and they are ordinary things
// to draw: two hull sections meeting, a window cut flush with a panel.

// --- helpers -------------------------------------------------------------

func v(x, y, z float64) geom.Vec3 { return geom.Vec3{X: x, Y: y, Z: z} }

// box builds an axis-aligned box body.
func box(minX, minY, minZ, maxX, maxY, maxZ float64, bodyID uint32) *mesh.Mesh {
	return mesh.Box(v(minX, minY, minZ), v(maxX, maxY, maxZ), bodyID)
}

// cube is a box of the given side with one corner at (x,y,z).
func cube(x, y, z, side float64, bodyID uint32) *mesh.Mesh {
	return box(x, y, z, x+side, y+side, z+side, bodyID)
}

// frustum builds a six-faced solid whose top face is inset on all four sides,
// which is what a drafted extrude produces.
func frustum(minX, minY, minZ, maxX, maxY, maxZ, inset float64, bodyID uint32) *mesh.Mesh {
	lo := []geom.Vec3{
		v(minX, minY, minZ), v(maxX, minY, minZ), v(maxX, maxY, minZ), v(minX, maxY, minZ),
	}
	hi := []geom.Vec3{
		v(minX+inset, minY+inset, maxZ), v(maxX-inset, minY+inset, maxZ),
		v(maxX-inset, maxY-inset, maxZ), v(minX+inset, maxY-inset, maxZ),
	}
	m := &mesh.Mesh{Verts: append(append([]geom.Vec3{}, lo...), hi...)}
	face := func(loop ...int) {
		id := mesh.MakeFaceUID(bodyID, uint32(len(m.Faces)))
		m.Faces = append(m.Faces, mesh.Face{ID: id, Loops: [][]int{loop}})
	}
	face(3, 2, 1, 0) // bottom, wound to look down
	face(4, 5, 6, 7) // top
	for i := 0; i < 4; i++ {
		face(i, (i+1)%4, 4+(i+1)%4, 4+i)
	}
	return m
}

// run executes one boolean and applies the checks every case shares.
func run(t *testing.T, op Op, target *mesh.Mesh, tools ...*mesh.Mesh) Result {
	t.Helper()
	res, err := Boolean(op, target, tools...)
	if err != nil {
		t.Fatalf("%v: %v", op, err)
	}
	if res.Empty {
		if res.Mesh != nil {
			t.Error("an empty result still carried a mesh")
		}
		return res
	}
	if res.Mesh == nil {
		t.Fatal("a non-empty result carried no mesh")
	}
	if err := mesh.Validate(res.Mesh); err != nil {
		t.Fatalf("%v produced an invalid solid: %v", op, err)
	}
	// SPEC-GEOMETRY §6.5: our volume and Manifold's must agree, or one of the
	// two converters is losing something.
	ours := mesh.Volume(res.Mesh)
	if rel(ours, res.Volume) > geom.VolumeRelTol {
		t.Errorf("our volume %.9f disagrees with Manifold's %.9f", ours, res.Volume)
	}
	return res
}

// wantVolume asserts the result's volume against an analytic figure.
func wantVolume(t *testing.T, res Result, want float64) {
	t.Helper()
	if res.Empty {
		t.Fatalf("result was empty, want volume %v", want)
	}
	if got := mesh.Volume(res.Mesh); rel(got, want) > geom.VolumeRelTol {
		t.Errorf("volume = %.9f, want %.9f", got, want)
	}
}

func wantShells(t *testing.T, res Result, want int) {
	t.Helper()
	if got := mesh.Check(res.Mesh).Shells; got != want {
		t.Errorf("shells = %d, want %d", got, want)
	}
}

func wantGenus(t *testing.T, res Result, want ...int) {
	t.Helper()
	got := mesh.Check(res.Mesh).GenusPer
	if len(got) != len(want) {
		t.Fatalf("genus = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("genus = %v, want %v", got, want)
			return
		}
	}
}

// wantTidy asserts that every result triangle found a polygon face to belong
// to. A stray triangle is valid but ugly, and on this matrix there is no excuse
// for one (SPEC-GEOMETRY §6.3).
func wantTidy(t *testing.T, res Result) {
	t.Helper()
	if res.Loose != 0 {
		t.Errorf("%d result triangles could not be merged into a face", res.Loose)
	}
}

func rel(got, want float64) float64 {
	scale := math.Max(1, math.Abs(want))
	return math.Abs(got-want) / scale
}

// --- Union ---------------------------------------------------------------

// 1
func TestUnionDisjoint(t *testing.T) {
	res := run(t, Union, cube(0, 0, 0, 2, 1), cube(10, 0, 0, 2, 2))
	wantVolume(t, res, 8+8)
	wantShells(t, res, 2)
	wantTidy(t, res)
}

// 2
func TestUnionOverlap(t *testing.T) {
	res := run(t, Union, cube(0, 0, 0, 4, 1), cube(2, 2, 2, 4, 2))
	wantVolume(t, res, 64+64-8)
	wantShells(t, res, 1)
}

// 3
func TestUnionContained(t *testing.T) {
	res := run(t, Union, cube(0, 0, 0, 8, 1), cube(2, 2, 2, 2, 2))
	wantVolume(t, res, 512)
	wantShells(t, res, 1)
	wantTidy(t, res)
}

// 4
func TestUnionIdentical(t *testing.T) {
	res := run(t, Union, cube(0, 0, 0, 4, 1), cube(0, 0, 0, 4, 2))
	wantVolume(t, res, 64)
	wantShells(t, res, 1)
	wantTidy(t, res)
}

// 5 — the flush butt-join, the case this whole exercise exists for. Two boxes
// stacked with a shared face: the shared face has to vanish entirely and leave
// one box, not two shells glued along a wall that is still there.
func TestUnionFlushButtJoin(t *testing.T) {
	res := run(t, Union, box(0, 0, 0, 4, 4, 4, 1), box(0, 0, 4, 4, 4, 8, 2))
	wantVolume(t, res, 128)
	wantShells(t, res, 1)
	wantGenus(t, res, 0)
	wantTidy(t, res)
	// The two stacked boxes are one box now, so it has a box's six faces.
	if n := len(res.Mesh.Faces); n != 6 {
		t.Errorf("the joined solid has %d faces, want 6 — the shared wall survived", n)
	}
}

// 6 — a small box landing flush on a big one: the shared area vanishes but the
// big box's top face stays, with the join cut into it.
func TestUnionPartialFlushButt(t *testing.T) {
	res := run(t, Union, box(0, 0, 0, 8, 8, 4, 1), box(2, 2, 4, 6, 6, 6, 2))
	wantVolume(t, res, 8*8*4+4*4*2)
	wantShells(t, res, 1)
	wantGenus(t, res, 0)
}

// 7
func TestUnionEdgeTouch(t *testing.T) {
	res := run(t, Union, box(0, 0, 0, 4, 4, 4, 1), box(4, 0, 4, 8, 4, 8, 2))
	wantVolume(t, res, 128)
}

// 8
func TestUnionVertexTouch(t *testing.T) {
	res := run(t, Union, cube(0, 0, 0, 4, 1), cube(4, 4, 4, 4, 2))
	wantVolume(t, res, 128)
}

// 9 — two drafted prisms, which is what the extrude tool actually makes.
func TestUnionDraftedPrisms(t *testing.T) {
	a := frustum(0, 0, 0, 6, 6, 4, 1, 1)
	b := frustum(3, 3, 0, 9, 9, 4, 1, 2)
	res := run(t, Union, a, b)

	va, vb := mesh.Volume(a), mesh.Volume(b)
	if got := mesh.Volume(res.Mesh); got >= va+vb || got <= math.Max(va, vb) {
		t.Errorf("union volume %v is not between %v and %v", got, math.Max(va, vb), va+vb)
	}
	wantShells(t, res, 1)
}

// 10
func TestUnionRotatedNinetyBoxes(t *testing.T) {
	a := box(-4, -1, -1, 4, 1, 1, 1)
	b := box(-1, -1, -4, 1, 1, 4, 2)
	res := run(t, Union, a, b)
	wantVolume(t, res, 32+32-8)
	wantShells(t, res, 1)
}

// 11 — free rotation puts every vertex off the grid, so there is no tidy
// analytic answer. The invariant that still holds is that our reading of the
// solid and Manifold's agree, which `run` already checked, plus the bounds any
// union has to respect.
func TestUnionFreeRotatedBoxes(t *testing.T) {
	a := box(-3, -3, -3, 3, 3, 3, 1)
	b := box(-3, -3, -3, 3, 3, 3, 2)
	mesh.Transform(b, geom.RotateY(37*math.Pi/180).Mul(geom.RotateX(23*math.Pi/180)))

	res := run(t, Union, a, b)
	va, vb := 216.0, 216.0
	if got := mesh.Volume(res.Mesh); got <= math.Max(va, vb) || got >= va+vb {
		t.Errorf("union volume %v is not between %v and %v", got, math.Max(va, vb), va+vb)
	}
	wantShells(t, res, 1)
}

// 12 — an overlap one subunit deep. Below this the two solids are flush, so
// this is the narrowest case that is still genuinely an overlap.
func TestUnionOneSubunitSliver(t *testing.T) {
	const sub = 1.0 / 256
	a := box(0, 0, 0, 4, 4, 4, 1)
	b := box(4-sub, 0, 0, 8, 4, 4, 2)
	res := run(t, Union, a, b)
	wantVolume(t, res, 64+(4+sub)*16-sub*16)
	wantShells(t, res, 1)
}

// --- Subtract ------------------------------------------------------------

// 13
func TestSubtractThroughHole(t *testing.T) {
	plate := box(0, 0, 0, 10, 10, 2, 1)
	tool := box(4, 4, -1, 6, 6, 3, 2)
	res := run(t, Subtract, plate, tool)
	wantVolume(t, res, 200-2*2*2)
	wantShells(t, res, 1)
	wantGenus(t, res, 1)
	wantTidy(t, res)
}

// 14
func TestSubtractBlindPocket(t *testing.T) {
	plate := box(0, 0, 0, 10, 10, 4, 1)
	tool := box(4, 4, 2, 6, 6, 5, 2)
	res := run(t, Subtract, plate, tool)
	wantVolume(t, res, 400-2*2*2)
	wantShells(t, res, 1)
	wantGenus(t, res, 0)
	wantTidy(t, res)
}

// 15 — the tool's side walls land exactly on the target's. Nothing about the
// answer is ambiguous; everything about computing it is.
func TestSubtractFlushWallCut(t *testing.T) {
	target := box(0, 0, 0, 10, 10, 4, 1)
	tool := box(5, 0, 1, 15, 10, 3, 2)
	res := run(t, Subtract, target, tool)
	wantVolume(t, res, 400-5*10*2)
	wantShells(t, res, 1)
	wantTidy(t, res)
}

// 16
func TestSubtractExactFitIsLegallyEmpty(t *testing.T) {
	res := run(t, Subtract, cube(0, 0, 0, 4, 1), cube(0, 0, 0, 4, 2))
	if !res.Empty {
		t.Errorf("subtracting a body from itself left volume %v", mesh.Volume(res.Mesh))
	}
}

// 17
func TestSubtractSwallowIsLegallyEmpty(t *testing.T) {
	res := run(t, Subtract, cube(2, 2, 2, 2, 1), cube(0, 0, 0, 8, 2))
	if !res.Empty {
		t.Error("a tool that contains the target did not empty it")
	}
}

// 18
func TestSubtractDisjointChangesNothing(t *testing.T) {
	res := run(t, Subtract, cube(0, 0, 0, 4, 1), cube(20, 0, 0, 4, 2))
	wantVolume(t, res, 64)
	wantShells(t, res, 1)
	wantTidy(t, res)
}

// 19 — the shape a round window actually is: a 16-gon, not a circle. The tool's
// own volume is the exact expected loss, so no approximation of pi is involved.
func TestSubtractSixteenGonHole(t *testing.T) {
	plate := box(-5, -5, 0, 5, 5, 2, 1)
	f := geom.Frame{O: v(0, 0, -1), U: geom.AxisX, V: geom.AxisY, N: geom.AxisZ}
	tool := mesh.NGonPrism(f, 3, 16, 4, 2)
	// Only the part inside the plate is removed, and the tool runs right
	// through, so that is its cross-section times the plate's thickness.
	toolVolume := mesh.Volume(tool)
	res := run(t, Subtract, plate, tool)
	wantVolume(t, res, 200-toolVolume/4*2)
	wantGenus(t, res, 1)
	wantTidy(t, res)
}

// 20
func TestSubtractSymmetricPrismCutter(t *testing.T) {
	target := box(-6, -6, -6, 6, 6, 6, 1)
	// A tool widest in the middle and tapering both ways, fully enclosed.
	lower := frustum(-3, -3, 0, 3, 3, 3, 1, 2)
	upper := frustum(-3, -3, 0, 3, 3, 3, 1, 3)
	mesh.Transform(upper, geom.Scale(v(1, 1, -1)))
	mesh.FlipAll(upper)

	res := run(t, Subtract, target, lower, upper)
	wantVolume(t, res, 12*12*12-mesh.Volume(lower)-mesh.Volume(upper))
	// The cutter is entirely enclosed, so what it leaves behind is a sealed
	// cavity: an inward-facing shell inside the outer one.
	wantShells(t, res, 2)
}

// 21
func TestSubtractToolSpanningTwoShells(t *testing.T) {
	target := box(0, 0, 0, 4, 4, 4, 1)
	mesh.Merge(target, box(10, 0, 0, 14, 4, 4, 1))
	tool := box(-1, 1, 1, 15, 3, 3, 2)

	res := run(t, Subtract, target, tool)
	wantVolume(t, res, 2*(64-4*2*2))
	wantShells(t, res, 2)
}

// --- Intersect -----------------------------------------------------------

// 22
func TestIntersectOverlap(t *testing.T) {
	res := run(t, Intersect, cube(0, 0, 0, 4, 1), cube(2, 2, 2, 4, 2))
	wantVolume(t, res, 8)
	wantShells(t, res, 1)
	wantTidy(t, res)
}

// 23
func TestIntersectDisjointIsLegallyEmpty(t *testing.T) {
	res := run(t, Intersect, cube(0, 0, 0, 4, 1), cube(20, 0, 0, 4, 2))
	if !res.Empty {
		t.Error("solids that do not meet produced an intersection")
	}
}

// 24
func TestIntersectContained(t *testing.T) {
	res := run(t, Intersect, cube(0, 0, 0, 8, 1), cube(2, 2, 2, 2, 2))
	wantVolume(t, res, 8)
	wantShells(t, res, 1)
	wantTidy(t, res)
}

// --- Chain ---------------------------------------------------------------

// 25 — ten mixed ops on one evolving body, with the validator and the
// Manifold-versus-ours volume cross-check after every step (TESTING §3).
//
// A kernel that drifts does it slowly. A single op can be wrong by a rounding
// error and still look fine; ten in a row on the same body do not. The early
// steps carry an analytic volume as well, where the arithmetic is unarguable.
func TestChainOfTenOps(t *testing.T) {
	steps := []struct {
		name string
		op   Op
		tool *mesh.Mesh
		// want is the analytic volume, or 0 to check only the invariants.
		want float64
	}{
		{"union an identical plate", Union, box(0, 0, 0, 20, 20, 4, 2), 1600},
		{"stack a block flush on top", Union, box(4, 4, 4, 12, 12, 8, 3), 1600 + 256},
		{"bore through plate and block", Subtract, box(7, 7, -1, 9, 9, 9, 4), 1856 - 16 - 16},
		{"cut a flush-walled slot", Subtract, box(16, -1, 1, 21, 21, 3, 5), 1824 - 160},
		{"add a disjoint pod", Union, box(30, 0, 0, 34, 4, 4, 6), 1664 + 64},
		{"trim the pod flush", Subtract, box(32, -1, -1, 35, 5, 5, 7), 1728 - 32},
		{"fill the bore back in", Union, box(7, 7, 0, 9, 9, 8, 8), 1696 + 32},
		{"shave the plate's end", Subtract, box(-1, -1, -1, 1, 21, 5, 9), 1728 - 80},
		{"weld a rib across the top", Union, box(2, 9, 4, 18, 11, 5, 10), 0},
		{"keep only the middle", Intersect, box(2, 2, -1, 26, 18, 9, 11), 0},
	}

	body := box(0, 0, 0, 20, 20, 4, 1)
	for i, s := range steps {
		res, err := Boolean(s.op, body, s.tool)
		if err != nil {
			t.Fatalf("step %d (%s): %v", i+1, s.name, err)
		}
		if res.Empty {
			t.Fatalf("step %d (%s) emptied the body", i+1, s.name)
		}
		if err := mesh.Validate(res.Mesh); err != nil {
			t.Fatalf("step %d (%s) produced an invalid solid: %v", i+1, s.name, err)
		}
		ours := mesh.Volume(res.Mesh)
		if rel(ours, res.Volume) > geom.VolumeRelTol {
			t.Fatalf("step %d (%s): our volume %.9f, Manifold's %.9f",
				i+1, s.name, ours, res.Volume)
		}
		if s.want != 0 && rel(ours, s.want) > geom.VolumeRelTol {
			t.Errorf("step %d (%s): volume %.6f, want %.6f", i+1, s.name, ours, s.want)
		}
		body = res.Mesh
	}
	if mesh.Volume(body) <= 0 {
		t.Error("the chain ended with nothing")
	}
}

// TestFaceProvenanceSurvivesABoolean covers the provenance half of §6.3: an
// output face has to remember which input face it came from, or paint and
// selection cannot survive an edit.
func TestFaceProvenanceSurvivesABoolean(t *testing.T) {
	plate := box(0, 0, 0, 10, 10, 2, 1)
	// Remember the top face's identity before the cut.
	top := -1
	for i := range plate.Faces {
		if plate.FaceNormal(i).Dot(geom.AxisZ) > 0.99 {
			top = i
		}
	}
	if top < 0 {
		t.Fatal("the plate has no top face")
	}
	topUID := plate.Faces[top].ID

	res := run(t, Subtract, plate, box(4, 4, -1, 6, 6, 3, 2))

	var found int
	for i := range res.Mesh.Faces {
		if res.Mesh.Faces[i].SrcFace == topUID {
			found++
		}
	}
	if found == 0 {
		t.Error("no output face traces back to the plate's top")
	}
	// Every face still needs its own identity.
	seen := map[mesh.FaceUID]bool{}
	for i := range res.Mesh.Faces {
		id := res.Mesh.Faces[i].ID
		if seen[id] {
			t.Errorf("face id %v was reused", id)
		}
		seen[id] = true
	}
}

// TestBooleanRefusesRubbish keeps the errors-not-panics rule of §6.2.
func TestBooleanRefusesRubbish(t *testing.T) {
	good := cube(0, 0, 0, 4, 1)
	if _, err := Boolean(Union, nil, good); err == nil {
		t.Error("a nil target was accepted")
	}
	if _, err := Boolean(Union, good); err == nil {
		t.Error("a boolean with no tools was accepted")
	}
	if _, err := Boolean(Union, good, nil); err == nil {
		t.Error("a nil tool was accepted")
	}
	open := &mesh.Mesh{
		Verts: []geom.Vec3{v(0, 0, 0), v(1, 0, 0), v(0, 1, 0)},
		Faces: []mesh.Face{{ID: mesh.MakeFaceUID(1, 0), Loops: [][]int{{0, 1, 2}}}},
	}
	if _, err := Boolean(Union, open, good); err == nil {
		t.Error("an open surface was accepted as a solid")
	}
}
