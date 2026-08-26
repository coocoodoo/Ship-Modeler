package sketch2d

import (
	"testing"

	"modeler/internal/geom"
)

// TESTING §2 for triangulation: area preservation must be exact, holes must be
// bridged, and no triangle may come out flipped.

// loopOf builds a Loop from world-unit corners, with placeholder provenance.
func loopOf(corners ...geom.Vec2i) Loop {
	l := Loop{Pts: append([]geom.Vec2i(nil), corners...)}
	l.Src = make([]Source, len(corners))
	return l
}

// reversedLoop flips a loop's winding, which is how a hole is written.
func reversedLoop(l Loop) Loop {
	out := Loop{Pts: make([]geom.Vec2i, len(l.Pts)), Src: make([]Source, len(l.Src))}
	for i := range l.Pts {
		out.Pts[i] = l.Pts[len(l.Pts)-1-i]
		out.Src[i] = l.Src[len(l.Src)-1-i]
	}
	return out
}

func rectLoop(x0, y0, x1, y1 float64) Loop {
	return loopOf(pt(x0, y0), pt(x1, y0), pt(x1, y1), pt(x0, y1))
}

func TestTriangulateCases(t *testing.T) {
	cases := []struct {
		name   string
		region Region
		// tris, when non-zero, is the exact expected triangle count.
		tris int
	}{
		{
			name:   "triangle",
			region: Region{Outer: loopOf(pt(0, 0), pt(4, 0), pt(2, 3))},
			tris:   1,
		},
		{
			name:   "square",
			region: Region{Outer: rectLoop(0, 0, 4, 4)},
			tris:   2,
		},
		{
			name: "concave L",
			region: Region{Outer: loopOf(
				pt(0, 0), pt(4, 0), pt(4, 1), pt(1, 1), pt(1, 4), pt(0, 4))},
			tris: 4,
		},
		{
			name: "square ring",
			region: Region{
				Outer: rectLoop(0, 0, 8, 8),
				Holes: []Loop{reversedLoop(rectLoop(2, 2, 6, 6))},
			},
		},
		{
			name: "rect with a sixteen sided hole",
			region: Region{
				Outer: rectLoop(0, 0, 10, 8),
				Holes: []Loop{reversedLoop(ngonLoop(5, 4, 2, 16))},
			},
		},
		{
			name: "rect with two holes",
			region: Region{
				Outer: rectLoop(0, 0, 12, 6),
				Holes: []Loop{
					reversedLoop(rectLoop(1, 1, 4, 5)),
					reversedLoop(rectLoop(7, 2, 10, 4)),
				},
			},
		},
		{
			name: "one subunit sliver",
			region: Region{Outer: loopOf(
				geom.Vec2i{X: 0, Y: 0},
				geom.Vec2i{X: u(4), Y: 0},
				geom.Vec2i{X: u(2), Y: 1})},
			tris: 1,
		},
		{
			name: "hole touching nothing but off centre",
			region: Region{
				Outer: rectLoop(0, 0, 20, 4),
				Holes: []Loop{reversedLoop(rectLoop(17, 1, 19, 3))},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tris := Triangulate(tc.region)
			if len(tris) == 0 {
				t.Fatal("no triangles produced")
			}
			if tc.tris != 0 && len(tris) != tc.tris {
				t.Errorf("triangles = %d, want %d", len(tris), tc.tris)
			}

			var sum int64
			for i, tr := range tris {
				a := tr.Area2()
				if a <= 0 {
					t.Errorf("triangle %d is flipped or degenerate (area2 %d): %v", i, a, tr)
				}
				sum += a
			}
			// The whole point: an exact partition of the region.
			if want := tc.region.Area2(); sum != want {
				t.Errorf("triangle area2 sums to %d, want the region's %d (off by %d)",
					sum, want, sum-want)
			}
		})
	}
}

func ngonLoop(cx, cy, r float64, segs int) Loop {
	sgs := ngon(0, cx, cy, r, segs)
	pts := make([]geom.Vec2i, len(sgs))
	for i, s := range sgs {
		pts[i] = s.A
	}
	return loopOf(pts...)
}

// TestTriangulateArrangement runs the acceptance shape of M2 end to end: the
// region engine's output must triangulate exactly.
func TestTriangulateArrangement(t *testing.T) {
	a := Build(append(rect(0, 0, 0, 8, 6), ngon(1, 4, 3, 1.5, 16)...))
	if len(a.Regions) != 2 {
		t.Fatalf("expected two regions, got %d", len(a.Regions))
	}
	var total int64
	for i, r := range a.Regions {
		tris := Triangulate(r)
		var sum int64
		for _, tr := range tris {
			if tr.Area2() <= 0 {
				t.Errorf("region %d produced a flipped triangle %v", i, tr)
			}
			sum += tr.Area2()
		}
		if sum != r.Area2() {
			t.Errorf("region %d triangulates to %d, want %d", i, sum, r.Area2())
		}
		total += sum
	}
	if want := rectArea2(8, 6); total != want {
		t.Errorf("the two regions cover %d, want the whole rectangle's %d", total, want)
	}
}

func TestTriangulateRejectsDegenerateInput(t *testing.T) {
	if got := Triangulate(Region{}); got != nil {
		t.Errorf("an empty region produced %d triangles", len(got))
	}
	if got := Triangulate(Region{Outer: loopOf(pt(0, 0), pt(1, 0))}); got != nil {
		t.Errorf("a two-point loop produced %d triangles", len(got))
	}
	// A loop with zero area has nothing to cover.
	flat := loopOf(pt(0, 0), pt(4, 0), pt(2, 0))
	if got := Triangulate(Region{Outer: flat}); len(got) != 0 {
		t.Errorf("a collinear loop produced %d triangles", len(got))
	}
}

func BenchmarkTriangulateRingWith64Sides(b *testing.B) {
	r := Region{
		Outer: ngonLoop(0, 0, 10, 64),
		Holes: []Loop{reversedLoop(ngonLoop(0, 0, 5, 64))},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Triangulate(r)
	}
}
