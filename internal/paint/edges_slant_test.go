package paint

import (
	"image"
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Bands along slanted edges (the user's report, 2026-08-28: "Edge Painter is
// not working properly. Is leaving gaps, used 3px on edges").
//
// Every earlier edge test runs along a lattice-aligned boundary, where a
// Bresenham line of square dabs happens to land flush. A slanted edge is a
// diagonal in texel space, and there the dab walk floors each anchor to the
// grid — up to a whole texel away from the edge, always in the same
// direction — leaving a stair-stepped sliver of bare body colour exactly
// where the line was asked for. The screenshots show it three ways: along the
// slope silhouette, at the corner where two bands meet, and as a thin bare
// line down a painted crease.

// wedgeProfile is the user's shape as one face: the stepped-wedge pentagon,
// tall at the right, a slope falling left. At 4 px/u the face is 24 texels
// across and the slope edge runs a true diagonal in texel space.
func wedgeProfile(t *testing.T) (*mesh.Mesh, *mesh.FacePaint) {
	t.Helper()
	m := &mesh.Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0, Z: 0}, {X: 6, Y: 0, Z: 0}, {X: 6, Y: 6, Z: 0},
			{X: 3, Y: 6, Z: 0}, {X: 0, Y: 3, Z: 0},
		},
		Faces: []mesh.Face{{ID: mesh.MakeFaceUID(1, 1), Loops: [][]int{{0, 1, 2, 3, 4}}}},
	}
	p, err := Allocate(m, 0, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[0].Paint = p
	return m, p
}

// inwardOf is the unit direction from an edge's midpoint toward the face's
// centre, square to the edge — the side of the edge the band lives on.
func inwardOf(m *mesh.Mesh, fi int, a, b geom.Vec3) geom.Vec3 {
	mid := a.Add(b).Mul(0.5)
	in := m.FaceCentroid(fi).Sub(mid)
	along := b.Sub(a).Normalize()
	in = in.Sub(along.Mul(in.Dot(along)))
	return in.Normalize()
}

func TestABandAlongASlantedEdgeLeavesNoGaps(t *testing.T) {
	m, p := wedgeProfile(t)
	a, b := m.Verts[3], m.Verts[4] // the slope: (3,6) down to (0,3)
	const width = 3

	wrote := EdgeBand(m, 0, p, Brush{Color: red, Size: width}, a, b)
	if wrote.Empty() {
		t.Fatal("the band painted nothing")
	}

	// March the whole edge. A point a sliver inside the face, right at the
	// edge, must land on a painted texel at every step — a single bare one is
	// the stair-step gap in the screenshots.
	in := inwardOf(m, 0, a, b)
	bare := 0
	for i := 1; i < 200; i++ {
		f := float64(i) / 200
		q := a.Lerp(b, f).Add(in.Mul(0.1 * p.Texel))
		if At(p, Texel(p, q)).A == 0 {
			bare++
		}
	}
	if bare > 0 {
		t.Errorf("%d of 199 probes along the slanted edge sit on bare texels — the band has gaps", bare)
	}
}

func TestASlantedBandIsNoFatterThanAsked(t *testing.T) {
	m, p := wedgeProfile(t)
	a, b := m.Verts[3], m.Verts[4]
	const width = 3
	EdgeBand(m, 0, p, Brush{Color: red, Size: width}, a, b)

	// Perpendicular distance in texels from the slope's line, for a texel
	// centre. Painted texels may straddle the edge — the renderer clips them
	// to the face — but none may sit meaningfully past the width inward, or
	// the band is thicker than the pixel count the user chose.
	ua, ub := UV(p, a), UV(p, b)
	dir := ub.Sub(ua).Normalize()
	nrm := geom.Vec2{X: -dir.Y, Y: dir.X}
	if UV(p, m.FaceCentroid(0)).Sub(ua).Dot(nrm) < 0 {
		nrm = nrm.Mul(-1)
	}
	r := FaceRect(m, 0, p)
	for y := r.Min.Y - 2; y < r.Max.Y+2; y++ {
		for x := r.Min.X - 2; x < r.Max.X+2; x++ {
			tx := image.Point{X: x, Y: y}
			if At(p, tx).A == 0 {
				continue
			}
			c := geom.Vec2{X: float64(x) + 0.5, Y: float64(y) + 0.5}
			u := c.Sub(ua).Dot(nrm)
			if u > float64(width)+0.71 {
				t.Errorf("texel %v sits %.2f texels inside the edge — past the %d asked for", tx, u, width)
			}
			if u < -0.71 {
				t.Errorf("texel %v sits %.2f texels outside the face across the edge", tx, u)
			}
		}
	}
}

func TestBandsTurnASlantedCorner(t *testing.T) {
	m, p := wedgeProfile(t)
	const width = 3
	// The two edges meeting at the top of the slope, as in the first
	// screenshot's leftmost arrow: the flat top and the slope.
	EdgeBand(m, 0, p, Brush{Color: red, Size: width}, m.Verts[2], m.Verts[3])
	EdgeBand(m, 0, p, Brush{Color: red, Size: width}, m.Verts[3], m.Verts[4])

	// Hug each edge into the shared corner: bare texels there are the notch
	// where the line fails to turn.
	for _, pair := range [][2]geom.Vec3{{m.Verts[2], m.Verts[3]}, {m.Verts[4], m.Verts[3]}} {
		in := inwardOf(m, 0, pair[0], pair[1])
		for i := 0; i <= 40; i++ {
			// The last quarter of the edge, up to a third of a texel shy of
			// the corner itself.
			f := 0.75 + 0.25*float64(i)/40
			q := pair[0].Lerp(pair[1], f)
			d := q.Dist(pair[1])
			if d < 0.34*p.Texel {
				continue
			}
			q = q.Add(in.Mul(0.1 * p.Texel))
			if At(p, Texel(p, q)).A == 0 {
				t.Errorf("bare texel %v hugging the corner (%.2f texels short of it)",
					Texel(p, q), d/p.Texel)
			}
		}
	}
}

// The crease from the third screenshot: two faces meet along an edge, each
// takes its band, and a thin bare line still shows between them. Each face's
// band must reach the shared edge from its own side.
func TestBandsMeetAcrossACrease(t *testing.T) {
	m := paintedCube(t, 4)
	topo := m.Topo()
	e := topo.Edges[0]
	wa, wb := m.Verts[e.A], m.Verts[e.B]
	const width = 3

	for _, use := range e.Uses {
		fi := use.Face
		p := m.Faces[fi].Paint
		EdgeBand(m, fi, p, Brush{Color: red, Size: width}, wa, wb)
		in := inwardOf(m, fi, wa, wb)
		for i := 1; i < 100; i++ {
			f := float64(i) / 100
			q := wa.Lerp(wb, f).Add(in.Mul(0.1 * p.Texel))
			if At(p, Texel(p, q)).A == 0 {
				t.Errorf("face %d: bare texel at %.2f along the shared edge", fi, f)
			}
		}
	}
}

// mathAbs guards against accidental shadowing in table edits above.
var _ = math.Abs

// stepProfile is the user's later shape: the wedge grown a step — a tall
// block at the right, a ledge, then the slope. Its crest edge (6,6)-(3,6)
// ends at two polygon corners that sit deep inside the face's bounding box,
// which is exactly where the border-proximity heuristic looked away.
func stepProfile(t *testing.T) (*mesh.Mesh, *mesh.FacePaint) {
	t.Helper()
	m := &mesh.Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0, Z: 0}, {X: 10, Y: 0, Z: 0}, {X: 10, Y: 9, Z: 0},
			{X: 6, Y: 9, Z: 0}, {X: 6, Y: 6, Z: 0}, {X: 3, Y: 6, Z: 0},
			{X: 0, Y: 3, Z: 0},
		},
		Faces: []mesh.Face{{ID: mesh.MakeFaceUID(1, 1), Loops: [][]int{{0, 1, 2, 3, 4, 5, 6}}}},
	}
	p, err := Allocate(m, 0, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[0].Paint = p
	return m, p
}

// The user's report (2026-08-28, second round): "it skipped this end". A band
// along the crest of the stepped face must reach both of its corners — the
// convex 135-degree one at the top of the slope AND the one against the step
// wall — even though neither corner is anywhere near the face's bounding box.
func TestABandReachesCornersInsideTheBoundingBox(t *testing.T) {
	m, p := stepProfile(t)
	a, b := m.Verts[4], m.Verts[5] // the crest: (6,6) to (3,6)
	const width = 3

	if EdgeBand(m, 0, p, Brush{Color: red, Size: width}, a, b).Empty() {
		t.Fatal("the band painted nothing")
	}
	in := inwardOf(m, 0, a, b)
	for i := 0; i <= 200; i++ {
		f := float64(i) / 200
		q := a.Lerp(b, f)
		// Right up to a third of a texel from either corner.
		if q.Dist(a) < 0.34*p.Texel || q.Dist(b) < 0.34*p.Texel {
			continue
		}
		q = q.Add(in.Mul(0.1 * p.Texel))
		if At(p, Texel(p, q)).A == 0 {
			t.Errorf("bare texel %v at %.2f along the crest — the band skipped an end",
				Texel(p, q), f)
		}
	}
}

// The rule that replaces the heuristic has to keep the other screenshot fix:
// an edge that ends where the face keeps going — the base of a box unioned
// onto a plate — must NOT grow a painted tail past its corner.
func TestABandDoesNotGrowATailOntoItsOwnFace(t *testing.T) {
	// A plate face with a notch bitten out of its edge: the notch's bottom
	// edge ends at corners where the face continues on, exactly like a union
	// seam. Loop: a 12x6 plate with a 4x2 notch in the top edge.
	m := &mesh.Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0, Z: 0}, {X: 12, Y: 0, Z: 0}, {X: 12, Y: 6, Z: 0},
			{X: 8, Y: 6, Z: 0}, {X: 8, Y: 4, Z: 0}, {X: 4, Y: 4, Z: 0},
			{X: 4, Y: 6, Z: 0}, {X: 0, Y: 6, Z: 0},
		},
		Faces: []mesh.Face{{ID: mesh.MakeFaceUID(1, 1), Loops: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}}},
	}
	p, err := Allocate(m, 0, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[0].Paint = p

	// Band along the notch floor, (8,4) to (4,4). Its continuations run into
	// the plate on both sides.
	a, b := m.Verts[4], m.Verts[5]
	const width = 3
	EdgeBand(m, 0, p, Brush{Color: red, Size: width}, a, b)

	// No paint beyond the notch floor's ends: probe the strip the band would
	// have bled into, just past each corner, at the band's own depth.
	in := inwardOf(m, 0, a, b)
	along := b.Sub(a).Normalize()
	for _, probe := range []struct {
		from geom.Vec3
		dir  geom.Vec3
	}{
		{a, along.Mul(-1)}, // past (8,4), toward the plate's right half
		{b, along},         // past (4,4), toward the plate's left half
	} {
		for d := 0.3; d <= float64(width); d += 0.25 {
			q := probe.from.Add(probe.dir.Mul(d * p.Texel)).Add(in.Mul(0.5 * p.Texel))
			if At(p, Texel(p, q)).A != 0 {
				t.Errorf("painted tail at texel %v, %.2f texels past the edge's end",
					Texel(p, q), d)
			}
		}
	}
}

// A picked edge is a line, not a segment (the user's report, 2026-08-28,
// third round: "it skipped this end"). A crest that history has split at a
// vertex — a union seam, a fold chord landing on the boundary — reads as one
// straight line, and clicking it must take the whole line. The band then runs
// to the end the user pointed at, instead of stopping at an invisible vertex.

// seamPrism is a 2x1x1 box with a seam of vertices at x=1, the same fixture
// mesh.fold uses: its front-top crest is two collinear edges meeting at the
// seam vertex.
func seamPrism() *mesh.Mesh {
	m := &mesh.Mesh{
		Verts: []geom.Vec3{
			{X: 0, Y: 0, Z: 0}, {X: 2, Y: 0, Z: 0}, {X: 2, Y: 1, Z: 0}, {X: 0, Y: 1, Z: 0},
			{X: 0, Y: 0, Z: 1}, {X: 2, Y: 0, Z: 1}, {X: 2, Y: 1, Z: 1}, {X: 0, Y: 1, Z: 1},
			{X: 1, Y: 0, Z: 1}, {X: 1, Y: 1, Z: 1},
			{X: 1, Y: 0, Z: 0}, {X: 1, Y: 1, Z: 0},
		},
	}
	loops := [][]int{
		{4, 8, 5, 6, 9, 7},
		{0, 3, 11, 2, 1, 10},
		{7, 9, 11, 3}, {9, 6, 2, 11},
		{4, 0, 10, 8}, {8, 10, 1, 5},
		{0, 4, 7, 3}, {1, 2, 6, 5},
	}
	for i, l := range loops {
		m.Faces = append(m.Faces, mesh.Face{ID: mesh.MakeFaceUID(1, uint32(i+1)), Loops: [][]int{l}})
	}
	return m
}

// edgeBetween finds the topology edge joining two vertices.
func edgeBetween(t *testing.T, m *mesh.Mesh, a, b int) int {
	t.Helper()
	for i, e := range m.Topo().Edges {
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			return i
		}
	}
	t.Fatalf("no edge between verts %d and %d", a, b)
	return -1
}

func TestEdgeChainFollowsTheStraightLineThroughASeam(t *testing.T) {
	m := seamPrism()
	// The front-top crest: (0,1,1)-(1,1,1)-(2,1,1), split at seam vert 9.
	half := edgeBetween(t, m, 7, 9)
	other := edgeBetween(t, m, 9, 6)

	chain := EdgeChain(m, half, 25)
	if len(chain) != 2 {
		t.Fatalf("chain from one crest half has %d edges, want both halves", len(chain))
	}
	found := map[int]bool{}
	for _, e := range chain {
		found[e] = true
	}
	if !found[half] || !found[other] {
		t.Errorf("chain %v does not contain both crest halves %d and %d", chain, half, other)
	}
}

func TestEdgeChainStopsAtRealCorners(t *testing.T) {
	m := seamPrism()
	// A short top edge at the box's end: its continuations turn 90 degrees
	// everywhere, so the chain is just itself.
	lone := edgeBetween(t, m, 7, 3)
	if chain := EdgeChain(m, lone, 25); len(chain) != 1 {
		t.Errorf("chain from a corner-bounded edge has %d edges, want 1: %v", len(chain), chain)
	}
}

// The continuation joins even when its own crease has gone shallow — a fold
// piece leaning until the far half of the line dips under the pick threshold
// is still, to the eye, the same line.
func TestEdgeChainDoesNotAskTheContinuationToBeSharp(t *testing.T) {
	m := seamPrism()
	// Lean the right half of the top: the crest stays collinear (the shared
	// verts do not move), but the right-top face tilts.
	for _, vi := range []int{2, 6} {
		m.Verts[vi] = m.Verts[vi].Add(geom.Vec3{Y: -0.2})
	}
	m.InvalidateCaches()

	half := edgeBetween(t, m, 7, 9)
	other := edgeBetween(t, m, 9, 6)
	chain := EdgeChain(m, half, 25)
	found := map[int]bool{}
	for _, e := range chain {
		found[e] = true
	}
	if !found[other] {
		t.Errorf("chain %v dropped the shallow half of the line", chain)
	}
}

// A bend in the line beyond the tolerance is a corner, and the chain respects
// it: a line that turns is two lines.
func TestEdgeChainStopsWhereTheLineBends(t *testing.T) {
	m := seamPrism()
	// Kink the crest itself: the seam vertex drops, so the two halves meet at
	// a real angle.
	m.Verts[9] = m.Verts[9].Add(geom.Vec3{Y: -0.6})
	m.InvalidateCaches()

	half := edgeBetween(t, m, 7, 9)
	if chain := EdgeChain(m, half, 25); len(chain) != 1 {
		t.Errorf("chain crossed a %d-edge kink the tolerance should refuse: %v", len(chain), chain)
	}
}
