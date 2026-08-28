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
