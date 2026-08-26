package mesh

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

func unitBox() *Mesh {
	return Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1)
}

func relClose(t *testing.T, got, want float64, what string) {
	t.Helper()
	scale := math.Max(1, math.Abs(want))
	if math.Abs(got-want)/scale > geom.VolumeRelTol {
		t.Fatalf("%s = %.12g, want %.12g", what, got, want)
	}
}

func TestBoxIsValid(t *testing.T) {
	m := unitBox()
	r := Check(m)
	if !r.OK() {
		t.Fatalf("unit box rejected: %v", r.Problems)
	}
	if r.Shells != 1 {
		t.Errorf("shells = %d, want 1", r.Shells)
	}
	if r.Verts != 8 || r.Edges != 12 || r.Faces != 6 {
		t.Errorf("V/E/F = %d/%d/%d, want 8/12/6", r.Verts, r.Edges, r.Faces)
	}
	if len(r.GenusPer) != 1 || r.GenusPer[0] != 0 {
		t.Errorf("genus = %v, want [0]", r.GenusPer)
	}
	relClose(t, r.Volume, 1, "unit box volume")
}

func TestBoxFaceNormalsPointOutward(t *testing.T) {
	m := Box(geom.Vec3{X: -2, Y: -3, Z: -4}, geom.Vec3{X: 5, Y: 6, Z: 7}, 1)
	center := m.AABB().Center()
	for fi := range m.Faces {
		n := m.FaceNormal(fi)
		outward := m.FaceCentroid(fi).Sub(center)
		if n.Dot(outward) <= 0 {
			t.Errorf("face %d normal %v points inward (centroid offset %v)", fi, n, outward)
		}
		// Every box face is axis aligned, so exactly one component is +-1.
		if math.Abs(math.Abs(n.Axis(n.MaxAbsAxis()))-1) > 1e-12 {
			t.Errorf("face %d normal %v is not axis aligned", fi, n)
		}
	}
	// The six normals must be the six axis directions, each once.
	seen := map[geom.Vec3]bool{}
	for fi := range m.Faces {
		seen[geom.SnapVec3ToSubunits(m.FaceNormal(fi))] = true
	}
	if len(seen) != 6 {
		t.Errorf("distinct normals = %d, want 6", len(seen))
	}
}

func TestBoxVolumeAndArea(t *testing.T) {
	m := Box(geom.Vec3{X: 1, Y: 2, Z: 3}, geom.Vec3{X: 4, Y: 8, Z: 5}, 1)
	relClose(t, Volume(m), 3*6*2, "box volume")
	relClose(t, SurfaceArea(m), 2*(3*6+3*2+6*2), "box surface area")
	c := Centroid(m)
	if !c.NearEq(geom.Vec3{X: 2.5, Y: 5, Z: 4}, 1e-9) {
		t.Errorf("centroid = %v, want {2.5 5 4}", c)
	}
	if got := m.TriangleCount(); got != 12 {
		t.Errorf("triangle count = %d, want 12", got)
	}
	if got := len(m.Triangulate()); got != 12 {
		t.Errorf("triangulated = %d, want 12", got)
	}
}

func TestVolumeIsTranslationInvariant(t *testing.T) {
	m := unitBox()
	before := Volume(m)
	Translate(m, geom.Vec3{X: 1000, Y: -2000, Z: 3000})
	relClose(t, Volume(m), before, "volume after a large translation")
}

func TestNGonPrismVolume(t *testing.T) {
	// A regular n-gon inscribed in radius r has area (n/2) r^2 sin(2pi/n).
	for _, segs := range []int{3, 4, 8, 16, 64} {
		const r, depth = 2.0, 3.0
		m := NGonPrism(geom.PlaneFrame(geom.PlaneFront), r, segs, depth, 1)
		if err := Validate(m); err != nil {
			t.Fatalf("segs=%d: %v", segs, err)
		}
		area := float64(segs) / 2 * r * r * math.Sin(2*math.Pi/float64(segs))
		relClose(t, Volume(m), area*depth, "prism volume")
		if got := len(m.Faces); got != segs+2 {
			t.Errorf("segs=%d: faces = %d, want %d", segs, got, segs+2)
		}
	}
}

func TestNGonPrismNegativeDepthStaysOutward(t *testing.T) {
	m := NGonPrism(geom.PlaneFrame(geom.PlaneTop), 1.5, 12, -4, 1)
	r := Check(m)
	if !r.OK() {
		t.Fatalf("negative-depth prism rejected: %v", r.Problems)
	}
	if r.Volume <= 0 {
		t.Fatalf("volume = %v, want positive", r.Volume)
	}
}

func TestValidateRejectsOpenBox(t *testing.T) {
	m := unitBox()
	m.Faces = m.Faces[:5] // drop a face
	m.InvalidateCaches()
	r := Check(m)
	if r.OK() {
		t.Fatal("open box accepted")
	}
	if !hasProblem(r, "non-manifold") && !hasProblem(r, "open") {
		t.Errorf("problems do not mention openness: %v", r.Problems)
	}
}

func TestValidateRejectsFlippedFace(t *testing.T) {
	m := unitBox()
	reverse(m.Faces[0].Loops[0])
	m.InvalidateCaches()
	r := Check(m)
	if r.OK() {
		t.Fatal("box with one flipped face accepted")
	}
	if !hasProblem(r, "inconsistent orientation") {
		t.Errorf("problems do not mention orientation: %v", r.Problems)
	}
}

func TestValidateRejectsFullyInvertedBox(t *testing.T) {
	m := unitBox()
	FlipAll(m)
	r := Check(m)
	if r.OK() {
		t.Fatal("inside-out box accepted")
	}
	if !hasProblem(r, "signed volume") {
		t.Errorf("problems do not mention volume sign: %v", r.Problems)
	}
}

func TestValidateRejectsDuplicateFace(t *testing.T) {
	m := unitBox()
	m.Faces = append(m.Faces, m.Faces[0])
	m.InvalidateCaches()
	r := Check(m)
	if r.OK() {
		t.Fatal("duplicated face accepted")
	}
	if !hasProblem(r, "duplicate") {
		t.Errorf("problems do not mention duplication: %v", r.Problems)
	}
}

func TestValidateRejectsOutOfRangeIndex(t *testing.T) {
	m := unitBox()
	m.Faces[0].Loops[0][0] = 99
	m.InvalidateCaches()
	if Validate(m) == nil {
		t.Fatal("out-of-range vertex index accepted")
	}
}

func TestValidateAcceptsTwoShells(t *testing.T) {
	m := unitBox()
	Merge(m, Box(geom.Vec3{X: 10, Y: 10, Z: 10}, geom.Vec3{X: 12, Y: 12, Z: 12}, 2))
	r := Check(m)
	if !r.OK() {
		t.Fatalf("two-shell mesh rejected: %v", r.Problems)
	}
	if r.Shells != 2 {
		t.Errorf("shells = %d, want 2", r.Shells)
	}
	relClose(t, r.Volume, 1+8, "two-shell volume")
}

// TestValidateAcceptsTube covers a genus-1 body: a square tube built as an
// outer box and an inner box, joined by two faces with holes.
func TestValidateAcceptsTube(t *testing.T) {
	m := squareTube()
	r := Check(m)
	if !r.OK() {
		t.Fatalf("tube rejected: %v", r.Problems)
	}
	if r.Shells != 1 {
		t.Errorf("shells = %d, want 1", r.Shells)
	}
	if len(r.GenusPer) != 1 || r.GenusPer[0] != 1 {
		t.Errorf("genus = %v, want [1]", r.GenusPer)
	}
	// Outer 4x4x2 minus inner 2x2x2 through-hole.
	relClose(t, r.Volume, 4*4*2-2*2*2, "tube volume")
}

// squareTube builds a 4x4x2 block with a 2x2 hole through it along Z, using
// faces with holes on the two capped ends.
func squareTube() *Mesh {
	outerMin := geom.Vec3{X: -2, Y: -2, Z: 0}
	outerMax := geom.Vec3{X: 2, Y: 2, Z: 2}
	innerMin := geom.Vec3{X: -1, Y: -1, Z: 0}
	innerMax := geom.Vec3{X: 1, Y: 1, Z: 2}

	m := &Mesh{}
	addQuadVerts := func(min, max geom.Vec3) (lo, hi [4]int) {
		// Corners in CCW order seen from +Z.
		pts := [4]geom.Vec2{{X: min.X, Y: min.Y}, {X: max.X, Y: min.Y}, {X: max.X, Y: max.Y}, {X: min.X, Y: max.Y}}
		for i, p := range pts {
			lo[i] = len(m.Verts)
			m.Verts = append(m.Verts, geom.Vec3{X: p.X, Y: p.Y, Z: min.Z})
		}
		for i, p := range pts {
			hi[i] = len(m.Verts)
			m.Verts = append(m.Verts, geom.Vec3{X: p.X, Y: p.Y, Z: max.Z})
		}
		return lo, hi
	}
	oLo, oHi := addQuadVerts(outerMin, outerMax)
	iLo, iHi := addQuadVerts(innerMin, innerMax)

	seq := uint32(1)
	next := func() FaceUID { id := MakeFaceUID(1, seq); seq++; return id }
	face := func(loops ...[]int) {
		m.Faces = append(m.Faces, Face{ID: next(), Loops: loops})
	}

	// Top cap (+Z): outer CCW, hole CW.
	face([]int{oHi[0], oHi[1], oHi[2], oHi[3]}, []int{iHi[0], iHi[3], iHi[2], iHi[1]})
	// Bottom cap (-Z): outer CW seen from +Z (so CCW from -Z), hole reversed.
	face([]int{oLo[0], oLo[3], oLo[2], oLo[1]}, []int{iLo[0], iLo[1], iLo[2], iLo[3]})
	// Outer walls, normals pointing away from the axis.
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		face([]int{oLo[i], oLo[j], oHi[j], oHi[i]})
	}
	// Inner walls, normals pointing toward the axis (into the hole).
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		face([]int{iLo[j], iLo[i], iHi[i], iHi[j]})
	}
	return m
}

func TestFaceWithHoleTriangulates(t *testing.T) {
	m := squareTube()
	// Face 0 is the top cap: outer 4 verts plus a 4-vert hole.
	tris := m.FaceTris(0)
	if len(tris) != 8 {
		t.Fatalf("cap with hole gave %d triangles, want 8", len(tris))
	}
	// Triangle areas must sum to the ring area: 4*4 - 2*2 = 12.
	var area float64
	n := m.FaceNormal(0)
	for _, tr := range tris {
		a, b, c := m.Verts[tr.A], m.Verts[tr.B], m.Verts[tr.C]
		area += b.Sub(a).Cross(c.Sub(a)).Dot(n) / 2
	}
	if math.Abs(area-12) > 1e-9 {
		t.Fatalf("triangulated area = %v, want 12", area)
	}
	relClose(t, m.FaceArea(0), 12, "FaceArea with hole")
}

func TestConcaveFaceTriangulationPreservesArea(t *testing.T) {
	// An L-shaped face in the Z=0 plane, counter-clockwise seen from +Z.
	m := &Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 1},
			{X: 1, Y: 1}, {X: 1, Y: 4}, {X: 0, Y: 4},
		},
		Faces: []Face{{ID: 1, Loops: [][]int{{0, 1, 2, 3, 4, 5}}}},
	}
	tris := m.FaceTris(0)
	if len(tris) != 4 {
		t.Fatalf("L face gave %d triangles, want 4", len(tris))
	}
	var area float64
	n := m.FaceNormal(0)
	for _, tr := range tris {
		a, b, c := m.Verts[tr.A], m.Verts[tr.B], m.Verts[tr.C]
		tri := b.Sub(a).Cross(c.Sub(a)).Dot(n) / 2
		if tri <= 0 {
			t.Errorf("flipped triangle %+v (signed area %v)", tr, tri)
		}
		area += tri
	}
	if math.Abs(area-7) > 1e-9 {
		t.Fatalf("L area = %v, want 7", area)
	}
}

func TestTopologyAdjacency(t *testing.T) {
	m := unitBox()
	tp := m.Topo()
	if len(tp.Edges) != 12 {
		t.Fatalf("edges = %d, want 12", len(tp.Edges))
	}
	for ei := range tp.Edges {
		if !tp.Edges[ei].Manifold() {
			t.Fatalf("edge %d is not manifold: %+v", ei, tp.Edges[ei])
		}
	}
	for v := range m.Verts {
		if len(tp.VertEdges[v]) != 3 {
			t.Errorf("vertex %d has %d edges, want 3", v, len(tp.VertEdges[v]))
		}
		if len(tp.VertFaces[v]) != 3 {
			t.Errorf("vertex %d has %d faces, want 3", v, len(tp.VertFaces[v]))
		}
	}
	if tp.EdgeIndex(0, 7) != -1 {
		t.Error("diagonal reported as an edge")
	}
	// The cache is dropped on edit.
	m.InvalidateCaches()
	if m.topo != nil {
		t.Error("topology cache survived invalidation")
	}
}

func TestCreaseClassification(t *testing.T) {
	m := unitBox()
	// Every box edge is a 90 degree crease, so all 12 are drawn.
	if got := len(m.DrawnEdges()); got != 12 {
		t.Fatalf("drawn edges = %d, want 12", got)
	}
	// A 64-gon prism has 4.5 degree side-to-side angles: only the cap rings and
	// the two caps' seams should be creases.
	p := NGonPrism(geom.PlaneFrame(geom.PlaneFront), 3, 64, 2, 1)
	drawn := len(p.DrawnEdges())
	if drawn != 128 {
		t.Errorf("prism drawn edges = %d, want 128 (the two cap rings only)", drawn)
	}
	// Determinism: two calls give the same order.
	a := p.DrawnEdges()
	b := p.DrawnEdges()
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("DrawnEdges is not deterministic")
		}
	}
}

func TestPlanarityAndNonPlanarFlag(t *testing.T) {
	m := unitBox()
	if got := m.Planarity(0); got > 1e-12 {
		t.Fatalf("flat face planarity = %v, want ~0", got)
	}
	if n := m.RecheckPlanarity(); n != 0 {
		t.Fatalf("%d faces flagged non-planar on a box", n)
	}
	// Bend one vertex well past the tolerance.
	m.Verts[7] = m.Verts[7].Add(geom.Vec3{X: 0.05, Y: 0.05, Z: 0.05})
	m.InvalidateCaches()
	if n := m.RecheckPlanarity(); n == 0 {
		t.Fatal("bent box reported all faces planar")
	}
	// Non-planar faces change how their edges are drawn.
	found := false
	for _, ei := range m.DrawnEdges() {
		if m.ClassifyEdge(ei) == EdgeNonPlanar {
			found = true
			break
		}
	}
	if !found {
		t.Error("no edge classified as bounding a non-planar face")
	}
}

func TestWeldMergesCoincidentVerts(t *testing.T) {
	m := unitBox()
	// Duplicate every vertex a hair away, then point one face at the copies.
	base := len(m.Verts)
	for i := 0; i < base; i++ {
		m.Verts = append(m.Verts, m.Verts[i].Add(geom.Vec3{X: geom.WeldDist / 4}))
	}
	for i, vi := range m.Faces[0].Loops[0] {
		m.Faces[0].Loops[0][i] = vi + base
	}
	m.InvalidateCaches()
	if err := Validate(m); err == nil {
		t.Fatal("split mesh should fail validation before welding")
	}
	Weld(m)
	if len(m.Verts) != 8 {
		t.Fatalf("after weld verts = %d, want 8", len(m.Verts))
	}
	if err := Validate(m); err != nil {
		t.Fatalf("welded mesh invalid: %v", err)
	}
	relClose(t, Volume(m), 1, "welded volume")
}

func TestWeldDropsCollapsedFaces(t *testing.T) {
	// A box squashed flat in Z collapses its four side faces.
	m := Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: geom.WeldDist / 4}, 1)
	Weld(m)
	if len(m.Verts) != 4 {
		t.Fatalf("verts = %d, want 4", len(m.Verts))
	}
	if len(m.Faces) != 2 {
		t.Fatalf("faces = %d, want 2", len(m.Faces))
	}
}

func TestCloneIsDeep(t *testing.T) {
	m := unitBox()
	c := m.Clone()
	c.Verts[0] = geom.Vec3{X: 99}
	c.Faces[0].Loops[0][0] = 3
	if m.Verts[0].X == 99 {
		t.Error("clone shares the vertex array")
	}
	if m.Faces[0].Loops[0][0] == 3 {
		t.Error("clone shares loop slices")
	}
}

func TestFaceUIDPacking(t *testing.T) {
	u := MakeFaceUID(7, 42)
	if u.BodyID() != 7 || u.Seq() != 42 {
		t.Fatalf("FaceUID round trip broke: body=%d seq=%d", u.BodyID(), u.Seq())
	}
	if MakeFaceUID(0, 0) != NoFace {
		t.Error("zero FaceUID is not NoFace")
	}
	// Different bodies never collide.
	if MakeFaceUID(1, 2) == MakeFaceUID(2, 1) {
		t.Error("FaceUID collision across bodies")
	}
}

func TestTransformKeepsPaintFrameGlued(t *testing.T) {
	m := unitBox()
	fp := &FacePaint{Res: 32, Texel: 1.0 / 32, Frame: m.FaceFrame(0)}
	m.Faces[0].Paint = fp
	probe := fp.Frame.ToWorld(geom.Vec2{X: 0.25, Y: -0.25})

	x := geom.Translate(geom.Vec3{X: 3, Y: 0, Z: 0}).Mul(geom.RotateY(math.Pi / 2))
	Transform(m, x)

	want := x.TransformPoint(probe)
	got := m.Faces[0].Paint.Frame.ToWorld(geom.Vec2{X: 0.25, Y: -0.25})
	if !got.NearEq(want, 1e-9) {
		t.Fatalf("paint anchor drifted: got %v, want %v", got, want)
	}
	relClose(t, Volume(m), 1, "volume after rigid motion")
}

func hasProblem(r Report, substr string) bool {
	for _, p := range r.Problems {
		if containsFold(p, substr) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
