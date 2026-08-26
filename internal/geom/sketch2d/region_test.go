package sketch2d

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// The canonical region cases of TESTING §2. Each asserts region count, hole
// count, open-end count and exact areas, because "is this profile closed?" is
// the question R3 hangs on and it is decided here in exact integer arithmetic.

// u converts world units to sketch subunits for readable test coordinates.
func u(v float64) int64 { return geom.ToSubunits(v) }

// pt builds a sketch point from world units.
func pt(x, y float64) geom.Vec2i { return geom.Vec2i{X: u(x), Y: u(y)} }

// poly turns a list of world-unit corners into a closed run of segments, all
// attributed to one source entity.
func poly(entity int, corners ...geom.Vec2i) []Seg {
	segs := make([]Seg, 0, len(corners))
	for i := range corners {
		segs = append(segs, Seg{
			A:   corners[i],
			B:   corners[(i+1)%len(corners)],
			Src: Source{Entity: entity, Seg: i},
		})
	}
	return segs
}

// rect builds an axis-aligned rectangle from opposite corners.
func rect(entity int, x0, y0, x1, y1 float64) []Seg {
	return poly(entity, pt(x0, y0), pt(x1, y0), pt(x1, y1), pt(x0, y1))
}

// ngon builds a regular polygon the way the Circle entity does.
func ngon(entity int, cx, cy, r float64, segs int) []Seg {
	pts := make([]geom.Vec2i, segs)
	for i := 0; i < segs; i++ {
		a := 2 * math.Pi * float64(i) / float64(segs)
		pts[i] = geom.Vec2i{
			X: u(cx) + int64(math.Round(float64(u(r))*math.Cos(a))),
			Y: u(cy) + int64(math.Round(float64(u(r))*math.Sin(a))),
		}
	}
	return poly(entity, pts...)
}

// area2 of an axis-aligned rectangle in subunits, doubled.
func rectArea2(w, h float64) int64 { return 2 * u(w) * u(h) }

func TestRegionCases(t *testing.T) {
	square := rect(0, 0, 0, 4, 4)

	cases := []struct {
		name string
		segs []Seg
		// regions, holes and openEnds are exact counts.
		regions  int
		holes    int
		openEnds int
		// area2, when non-zero, is the exact total filled area doubled.
		area2 int64
		// check runs extra assertions specific to the case.
		check func(t *testing.T, a Arrangement)
	}{
		{
			name: "square", segs: square,
			regions: 1, holes: 0, openEnds: 0, area2: rectArea2(4, 4),
		},
		{
			// A rectangle with a circle inside it is the acceptance shape of
			// M2: the ring and the disc are two separate filled regions, and
			// together they cover exactly the rectangle.
			name:    "rect with circle hole",
			segs:    append(rect(0, 0, 0, 8, 6), ngon(1, 4, 3, 1.5, 16)...),
			regions: 2, holes: 1, openEnds: 0, area2: rectArea2(8, 6),
			check: func(t *testing.T, a Arrangement) {
				var ring, disc *Region
				for i := range a.Regions {
					if len(a.Regions[i].Holes) == 1 {
						ring = &a.Regions[i]
					} else {
						disc = &a.Regions[i]
					}
				}
				if ring == nil || disc == nil {
					t.Fatal("expected one region with a hole and one without")
				}
				// The hole and the island are the same circle, so they have
				// equal magnitude and opposite winding.
				if ring.Holes[0].Area2() != -disc.Outer.Area2() {
					t.Errorf("hole area %d does not mirror the island's %d",
						ring.Holes[0].Area2(), disc.Outer.Area2())
				}
			},
		},
		{
			// Three concentric squares: two rings and a disc, which is the
			// one-level island nesting GEOM §4.6 calls for.
			name:    "nested islands",
			segs:    concat(rect(0, 0, 0, 9, 9), rect(1, 2, 2, 7, 7), rect(2, 3.5, 3.5, 5.5, 5.5)),
			regions: 3, holes: 2, openEnds: 0, area2: rectArea2(9, 9),
			check: func(t *testing.T, a Arrangement) {
				withHoles := 0
				for _, r := range a.Regions {
					if len(r.Holes) == 1 {
						withHoles++
					}
				}
				if withHoles != 2 {
					t.Errorf("%d regions carry a hole, want 2", withHoles)
				}
			},
		},
		{
			// Two squares meeting at a single corner: one connected component
			// with two bounded faces, and no open ends despite the pinch.
			name:    "figure eight",
			segs:    concat(rect(0, 0, 0, 4, 4), rect(1, 4, 4, 8, 8)),
			regions: 2, holes: 0, openEnds: 0, area2: 2 * rectArea2(4, 4),
		},
		{
			// Two rectangles sharing a whole edge. The shared edge must survive
			// exactly once, or the two faces merge into one.
			name:    "butt joint on a shared edge",
			segs:    concat(rect(0, 0, 0, 4, 4), rect(1, 4, 0, 8, 4)),
			regions: 2, holes: 0, openEnds: 0, area2: 2 * rectArea2(4, 4),
		},
		{
			// Overlapping rectangles split into three faces: two L shapes and
			// the intersection.
			name:    "overlapping rects",
			segs:    concat(rect(0, 0, 0, 4, 4), rect(1, 2, 2, 6, 6)),
			regions: 3, holes: 0, openEnds: 0,
			area2: 2*rectArea2(4, 4) - rectArea2(2, 2),
		},
		{
			name: "open chain",
			segs: []Seg{
				{A: pt(0, 0), B: pt(4, 0), Src: Source{Entity: 0}},
				{A: pt(4, 0), B: pt(4, 3), Src: Source{Entity: 0, Seg: 1}},
			},
			regions: 0, holes: 0, openEnds: 2,
		},
		{
			// A T joint is legal: degree three is fine, only degree one is an
			// open end (GEOM §4.7).
			name: "T joint",
			segs: []Seg{
				{A: pt(0, 0), B: pt(4, 0), Src: Source{Entity: 0}},
				{A: pt(2, 0), B: pt(2, 3), Src: Source{Entity: 1}},
			},
			regions: 0, holes: 0, openEnds: 3,
		},
		{
			// Users retrace edges constantly; drawing the same square twice
			// must not change anything.
			name:    "duplicate segments",
			segs:    concat(square, rect(1, 0, 0, 4, 4)),
			regions: 1, holes: 0, openEnds: 0, area2: rectArea2(4, 4),
		},
		{
			// Two collinear segments that partly overlap re-emit as three
			// maximal pieces with two loose ends.
			name: "collinear partial overlap",
			segs: []Seg{
				{A: pt(0, 0), B: pt(4, 0), Src: Source{Entity: 0}},
				{A: pt(2, 0), B: pt(6, 0), Src: Source{Entity: 1}},
			},
			regions: 0, holes: 0, openEnds: 2,
		},
		{
			// A square crossed by both diagonals: four triangles that together
			// tile the square exactly.
			name: "crossing diagonals in a frame",
			segs: concat(square,
				[]Seg{
					{A: pt(0, 0), B: pt(4, 4), Src: Source{Entity: 1}},
					{A: pt(4, 0), B: pt(0, 4), Src: Source{Entity: 2}},
				}),
			regions: 4, holes: 0, openEnds: 0, area2: rectArea2(4, 4),
		},
		{
			name: "zero length segments are dropped",
			segs: concat(square, []Seg{
				{A: pt(1, 1), B: pt(1, 1), Src: Source{Entity: 1}},
				{A: pt(2, 2), B: pt(2, 2), Src: Source{Entity: 2}},
			}),
			regions: 1, holes: 0, openEnds: 0, area2: rectArea2(4, 4),
		},
		{
			// A triangle one subunit tall is still a real region: the engine is
			// exact, so it neither collapses it nor invents an epsilon.
			name: "one subunit sliver",
			segs: poly(0,
				geom.Vec2i{X: 0, Y: 0},
				geom.Vec2i{X: u(4), Y: 0},
				geom.Vec2i{X: u(2), Y: 1}),
			regions: 1, holes: 0, openEnds: 0, area2: u(4) * 1,
		},
		{
			name:    "circle alone",
			segs:    ngon(0, 0, 0, 2, 16),
			regions: 1, holes: 0, openEnds: 0,
		},
		{
			name:    "two disjoint squares",
			segs:    concat(rect(0, 0, 0, 2, 2), rect(1, 5, 5, 7, 7)),
			regions: 2, holes: 0, openEnds: 0, area2: 2 * rectArea2(2, 2),
		},
		{
			name:    "nothing at all",
			segs:    nil,
			regions: 0, holes: 0, openEnds: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Build(tc.segs)

			if got := len(a.Regions); got != tc.regions {
				t.Errorf("regions = %d, want %d", got, tc.regions)
			}
			holes := 0
			for _, r := range a.Regions {
				holes += len(r.Holes)
			}
			if holes != tc.holes {
				t.Errorf("holes = %d, want %d", holes, tc.holes)
			}
			if got := len(a.OpenEnds); got != tc.openEnds {
				t.Errorf("open ends = %d, want %d (%v)", got, tc.openEnds, a.OpenEnds)
			}
			if tc.area2 != 0 {
				if got := a.Area2(); got != tc.area2 {
					t.Errorf("total area2 = %d, want %d", got, tc.area2)
				}
			}
			assertWellFormed(t, a)
			if tc.check != nil {
				tc.check(t, a)
			}
		})
	}
}

func concat(groups ...[]Seg) []Seg {
	var out []Seg
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// assertWellFormed checks the invariants every arrangement must hold, whatever
// the input was.
func assertWellFormed(t *testing.T, a Arrangement) {
	t.Helper()
	for i, r := range a.Regions {
		if len(r.Outer.Pts) < 3 {
			t.Errorf("region %d has a %d-point outer loop", i, len(r.Outer.Pts))
		}
		if r.Outer.Area2() <= 0 {
			t.Errorf("region %d outer loop is not counter-clockwise (area2 %d)", i, r.Outer.Area2())
		}
		if len(r.Outer.Pts) != len(r.Outer.Src) {
			t.Errorf("region %d outer loop has %d points but %d sources",
				i, len(r.Outer.Pts), len(r.Outer.Src))
		}
		for j, h := range r.Holes {
			if h.Area2() >= 0 {
				t.Errorf("region %d hole %d is not clockwise (area2 %d)", i, j, h.Area2())
			}
			if len(h.Pts) != len(h.Src) {
				t.Errorf("region %d hole %d has %d points but %d sources",
					i, j, len(h.Pts), len(h.Src))
			}
		}
		if r.Area2() <= 0 {
			t.Errorf("region %d has non-positive area %d", i, r.Area2())
		}
		// A loop must never repeat a vertex, which would mean the walk closed
		// early or doubled back.
		assertNoRepeats(t, r.Outer, i, "outer")
		for j := range r.Holes {
			assertNoRepeats(t, r.Holes[j], i, "hole")
		}
	}
}

func assertNoRepeats(t *testing.T, l Loop, region int, what string) {
	t.Helper()
	seen := map[geom.Vec2i]bool{}
	for _, p := range l.Pts {
		if seen[p] {
			t.Errorf("region %d %s loop repeats vertex %v", region, what, p)
			return
		}
		seen[p] = true
	}
}

// TestBuildIsDeterministic is what the golden shots and stable region indices
// depend on: the same segments in any order must produce the same output.
func TestBuildIsDeterministic(t *testing.T) {
	segs := concat(rect(0, 0, 0, 8, 6), ngon(1, 4, 3, 1.5, 16))

	want := Build(segs)
	for trial := 0; trial < 5; trial++ {
		if got := Build(segs); !sameArrangement(got, want) {
			t.Fatalf("trial %d differed from the first run", trial)
		}
	}

	// Reversing the input order must not reorder the output.
	reversed := make([]Seg, len(segs))
	for i, s := range segs {
		reversed[len(segs)-1-i] = s
	}
	if got := Build(reversed); !sameArrangement(got, want) {
		t.Error("reversing the input segments changed the arrangement")
	}

	// Reversing individual segments must not either.
	flipped := make([]Seg, len(segs))
	for i, s := range segs {
		flipped[i] = Seg{A: s.B, B: s.A, Src: s.Src}
	}
	if got := Build(flipped); !sameArrangement(got, want) {
		t.Error("flipping each segment's direction changed the arrangement")
	}
}

func sameArrangement(a, b Arrangement) bool {
	if len(a.Regions) != len(b.Regions) || len(a.OpenEnds) != len(b.OpenEnds) {
		return false
	}
	for i := range a.Regions {
		if !sameLoop(a.Regions[i].Outer, b.Regions[i].Outer) {
			return false
		}
		if len(a.Regions[i].Holes) != len(b.Regions[i].Holes) {
			return false
		}
		for j := range a.Regions[i].Holes {
			if !sameLoop(a.Regions[i].Holes[j], b.Regions[i].Holes[j]) {
				return false
			}
		}
	}
	for i := range a.OpenEnds {
		if a.OpenEnds[i] != b.OpenEnds[i] {
			return false
		}
	}
	return true
}

func sameLoop(a, b Loop) bool {
	if len(a.Pts) != len(b.Pts) {
		return false
	}
	for i := range a.Pts {
		if a.Pts[i] != b.Pts[i] {
			return false
		}
	}
	return true
}

// TestSourceProvenanceSurvives covers the mapping M3 needs for stable side-face
// identities (GEOM §5.5): every boundary edge must name the entity it came from.
func TestSourceProvenanceSurvives(t *testing.T) {
	a := Build(rect(7, 0, 0, 4, 4))
	if len(a.Regions) != 1 {
		t.Fatalf("expected one region, got %d", len(a.Regions))
	}
	seen := map[int]bool{}
	for _, s := range a.Regions[0].Outer.Src {
		if s.Entity != 7 {
			t.Errorf("edge attributed to entity %d, want 7", s.Entity)
		}
		seen[s.Seg] = true
	}
	if len(seen) != 4 {
		t.Errorf("the four rectangle edges map to %d distinct source segments", len(seen))
	}
}

// TestOpenEndsAreSorted keeps the "2 open ends" readout and the red rings
// stable frame to frame.
func TestOpenEndsAreSorted(t *testing.T) {
	segs := []Seg{
		{A: pt(5, 5), B: pt(6, 5)},
		{A: pt(0, 0), B: pt(1, 0)},
		{A: pt(2, 9), B: pt(3, 9)},
	}
	a := Build(segs)
	if len(a.OpenEnds) != 6 {
		t.Fatalf("open ends = %d, want 6", len(a.OpenEnds))
	}
	for i := 1; i < len(a.OpenEnds); i++ {
		prev, cur := a.OpenEnds[i-1], a.OpenEnds[i]
		if cur.X < prev.X || (cur.X == prev.X && cur.Y < prev.Y) {
			t.Fatalf("open ends are not sorted: %v then %v", prev, cur)
		}
	}
}

// TestClosedProfileGate is R3 stated directly: a profile with any loose end
// must not offer a region to extrude, and closing it must.
func TestClosedProfileGate(t *testing.T) {
	openSquare := []Seg{
		{A: pt(0, 0), B: pt(4, 0)},
		{A: pt(4, 0), B: pt(4, 4)},
		{A: pt(4, 4), B: pt(0, 4)},
	}
	a := Build(openSquare)
	if len(a.Regions) != 0 {
		t.Errorf("an open profile produced %d regions", len(a.Regions))
	}
	if len(a.OpenEnds) != 2 {
		t.Errorf("open ends = %d, want 2", len(a.OpenEnds))
	}

	closed := append(openSquare, Seg{A: pt(0, 4), B: pt(0, 0)})
	b := Build(closed)
	if len(b.Regions) != 1 || len(b.OpenEnds) != 0 {
		t.Errorf("closing the profile gave %d regions and %d open ends",
			len(b.Regions), len(b.OpenEnds))
	}
}

// BenchmarkBuild500Segments is the budget case of SPEC-GEOMETRY §4: a
// 500-segment sketch, rebuilt on every edit, in under 2 ms. A real sketch of
// that size is mostly separate shapes, so the bounding-box reject in the split
// pass throws out nearly every pair.
func BenchmarkBuild500Segments(b *testing.B) {
	var segs []Seg
	for i := 0; i < 31; i++ {
		cx := float64(i%8) * 5
		cy := float64(i/8) * 5
		segs = append(segs, ngon(i, cx, cy, 2, 16)...)
	}
	if len(segs) < 480 || len(segs) > 520 {
		b.Fatalf("benchmark builds %d segments, want about 500", len(segs))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Build(segs)
	}
}

// BenchmarkBuildDenseGrid is the adversarial case: 200 input segments that
// cross into thousands of pieces. It is far past anything a person draws, and
// exists to show where the O(n^2) split starts to hurt.
func BenchmarkBuildDenseGrid(b *testing.B) {
	var segs []Seg
	for i := 0; i < 25; i++ {
		x := float64(i) * 1.5
		segs = append(segs, rect(i, x, 0, x+1, 20)...)
	}
	for i := 0; i < 100; i++ {
		y := float64(i) * 0.2
		segs = append(segs, Seg{A: pt(0, y), B: pt(37, y), Src: Source{Entity: 100 + i}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Build(segs)
	}
}
