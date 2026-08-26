package geom

import (
	"math"
	"math/rand"
	"testing"
)

func TestCrossOrientation(t *testing.T) {
	a := Vec2i{0, 0}
	b := Vec2i{256, 0}
	cases := []struct {
		name string
		c    Vec2i
		want int
	}{
		{"left", Vec2i{128, 64}, 1},
		{"right", Vec2i{128, -64}, -1},
		{"on", Vec2i{128, 0}, 0},
		{"beyond", Vec2i{512, 0}, 0},
		{"before", Vec2i{-512, 0}, 0},
	}
	for _, tc := range cases {
		if got := Orient(a, b, tc.c); got != tc.want {
			t.Errorf("%s: Orient = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestCrossExactAtClampBounds(t *testing.T) {
	// The clamp exists so cross terms cannot overflow int64; verify the extreme
	// case exactly against big-free reasoning: terms are ±(2*SketchClamp)^2.
	a := Vec2i{-SketchClamp, -SketchClamp}
	b := Vec2i{SketchClamp, -SketchClamp}
	c := Vec2i{SketchClamp, SketchClamp}
	want := int64(2*SketchClamp) * int64(2*SketchClamp)
	if got := Cross(a, b, c); got != want {
		t.Fatalf("Cross = %d, want %d", got, want)
	}
	if want <= 0 {
		t.Fatal("overflow: cross term wrapped")
	}
}

func TestOnSegment(t *testing.T) {
	a, b := Vec2i{0, 0}, Vec2i{300, 600}
	in := []Vec2i{{0, 0}, {300, 600}, {150, 300}, {1, 2}}
	out := []Vec2i{{150, 301}, {-1, -2}, {301, 602}, {450, 900}}
	for _, p := range in {
		if !OnSegment(a, b, p) {
			t.Errorf("OnSegment(%v) = false, want true", p)
		}
	}
	for _, p := range out {
		if OnSegment(a, b, p) {
			t.Errorf("OnSegment(%v) = true, want false", p)
		}
	}
}

func TestSegSegIntersectClassification(t *testing.T) {
	cases := []struct {
		name           string
		a1, a2, b1, b2 Vec2i
		want           SegRel
		wantP          *Vec2i
	}{
		{
			name: "proper cross at center",
			a1:   Vec2i{-256, 0}, a2: Vec2i{256, 0},
			b1: Vec2i{0, -256}, b2: Vec2i{0, 256},
			want: SegProper, wantP: &Vec2i{0, 0},
		},
		{
			name: "parallel disjoint",
			a1:   Vec2i{0, 0}, a2: Vec2i{256, 0},
			b1: Vec2i{0, 256}, b2: Vec2i{256, 256},
			want: SegNone,
		},
		{
			name: "far apart",
			a1:   Vec2i{0, 0}, a2: Vec2i{256, 0},
			b1: Vec2i{1000, 1000}, b2: Vec2i{2000, 2000},
			want: SegNone,
		},
		{
			name: "shared endpoint",
			a1:   Vec2i{0, 0}, a2: Vec2i{256, 0},
			b1: Vec2i{256, 0}, b2: Vec2i{256, 256},
			want: SegTouch, wantP: &Vec2i{256, 0},
		},
		{
			name: "T touch: endpoint on interior",
			a1:   Vec2i{0, 0}, a2: Vec2i{512, 0},
			b1: Vec2i{256, 0}, b2: Vec2i{256, 512},
			want: SegTouch, wantP: &Vec2i{256, 0},
		},
		{
			name: "collinear overlap",
			a1:   Vec2i{0, 0}, a2: Vec2i{512, 0},
			b1: Vec2i{256, 0}, b2: Vec2i{768, 0},
			want: SegCollinear,
		},
		{
			name: "collinear contained",
			a1:   Vec2i{0, 0}, a2: Vec2i{1024, 0},
			b1: Vec2i{256, 0}, b2: Vec2i{768, 0},
			want: SegCollinear,
		},
		{
			name: "collinear identical",
			a1:   Vec2i{0, 0}, a2: Vec2i{512, 0},
			b1: Vec2i{0, 0}, b2: Vec2i{512, 0},
			want: SegCollinear,
		},
		{
			name: "collinear touching at a point",
			a1:   Vec2i{0, 0}, a2: Vec2i{256, 0},
			b1: Vec2i{256, 0}, b2: Vec2i{512, 0},
			want: SegTouch, wantP: &Vec2i{256, 0},
		},
		{
			name: "collinear disjoint",
			a1:   Vec2i{0, 0}, a2: Vec2i{256, 0},
			b1: Vec2i{512, 0}, b2: Vec2i{768, 0},
			want: SegNone,
		},
		{
			name: "degenerate point on segment",
			a1:   Vec2i{128, 0}, a2: Vec2i{128, 0},
			b1: Vec2i{0, 0}, b2: Vec2i{256, 0},
			want: SegTouch, wantP: &Vec2i{128, 0},
		},
		{
			name: "degenerate point off segment",
			a1:   Vec2i{128, 8}, a2: Vec2i{128, 8},
			b1: Vec2i{0, 0}, b2: Vec2i{256, 0},
			want: SegNone,
		},
		{
			name: "two identical points",
			a1:   Vec2i{5, 5}, a2: Vec2i{5, 5},
			b1: Vec2i{5, 5}, b2: Vec2i{5, 5},
			want: SegTouch, wantP: &Vec2i{5, 5},
		},
		{
			name: "touching bboxes but no hit",
			a1:   Vec2i{0, 0}, a2: Vec2i{256, 256},
			b1: Vec2i{256, 0}, b2: Vec2i{300, 100},
			want: SegNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SegSegIntersect(tc.a1, tc.a2, tc.b1, tc.b2)
			if got.Rel != tc.want {
				t.Fatalf("Rel = %v, want %v (hit %+v)", got.Rel, tc.want, got)
			}
			if tc.wantP != nil && got.P != *tc.wantP {
				t.Fatalf("P = %v, want %v", got.P, *tc.wantP)
			}
			// Symmetry: swapping the operands must not change the class.
			rev := SegSegIntersect(tc.b1, tc.b2, tc.a1, tc.a2)
			if rev.Rel != got.Rel {
				t.Fatalf("asymmetric: %v vs %v", got.Rel, rev.Rel)
			}
		})
	}
}

// TestIntersectionRoundingBound is the property SPEC-GEOMETRY §1.1 promises:
// rounding an intersection point never moves it more than one subunit.
func TestIntersectionRoundingBound(t *testing.T) {
	rng := rand.New(rand.NewSource(20260826))
	const trials = 20000
	rounded := 0
	for i := 0; i < trials; i++ {
		r := func() int64 { return int64(rng.Intn(4001) - 2000) }
		a1 := Vec2i{r(), r()}
		a2 := Vec2i{r(), r()}
		b1 := Vec2i{r(), r()}
		b2 := Vec2i{r(), r()}
		hit := SegSegIntersect(a1, a2, b1, b2)
		if hit.Rel != SegProper && hit.Rel != SegTouch {
			continue
		}
		if !hit.Exact {
			rounded++
		}
		// Exact reference point in float64: the inputs are small enough that
		// the float solve is far more accurate than one subunit.
		d1x, d1y := float64(a2.X-a1.X), float64(a2.Y-a1.Y)
		d2x, d2y := float64(b2.X-b1.X), float64(b2.Y-b1.Y)
		den := d1x*d2y - d1y*d2x
		if den == 0 {
			continue
		}
		wx, wy := float64(b1.X-a1.X), float64(b1.Y-a1.Y)
		tt := (wx*d2y - wy*d2x) / den
		px := float64(a1.X) + tt*d1x
		py := float64(a1.Y) + tt*d1y
		if dx := math.Abs(px - float64(hit.P.X)); dx > 1 {
			t.Fatalf("x moved %.4f subunits (%v x %v -> %v)", dx, a1, a2, hit.P)
		}
		if dy := math.Abs(py - float64(hit.P.Y)); dy > 1 {
			t.Fatalf("y moved %.4f subunits (%v x %v -> %v)", dy, a1, a2, hit.P)
		}
	}
	if rounded == 0 {
		t.Fatal("no rounded intersections generated; the property went untested")
	}
	t.Logf("checked %d random pairs, %d needed rounding", trials, rounded)
}

func TestMulDivRound(t *testing.T) {
	cases := []struct {
		a, b, c int64
		want    int64
		exact   bool
	}{
		{10, 10, 5, 20, true},
		{10, 10, 3, 33, false},   // 33.33 -> 33
		{10, 10, 7, 14, false},   // 14.28 -> 14
		{-10, 10, 3, -33, false}, // half away from zero, negative side
		{1, 1, 2, 1, false},      // 0.5 rounds away from zero
		{-1, 1, 2, -1, false},
		{0, 12345, 7, 0, true},
		{1 << 40, 1 << 20, 1 << 30, 1 << 30, true}, // needs the 128-bit path
	}
	for _, tc := range cases {
		got, exact := mulDivRound(tc.a, tc.b, tc.c)
		if got != tc.want || exact != tc.exact {
			t.Errorf("mulDivRound(%d,%d,%d) = (%d,%v), want (%d,%v)",
				tc.a, tc.b, tc.c, got, exact, tc.want, tc.exact)
		}
	}
}

func TestShoelaceArea2(t *testing.T) {
	// A 2u square in subunits: area 512*512, twice that is 524288.
	sq := []Vec2i{{0, 0}, {512, 0}, {512, 512}, {0, 512}}
	if got := ShoelaceArea2(sq); got != 512*512*2 {
		t.Errorf("CCW square area2 = %d, want %d", got, 512*512*2)
	}
	rev := []Vec2i{{0, 512}, {512, 512}, {512, 0}, {0, 0}}
	if got := ShoelaceArea2(rev); got != -512*512*2 {
		t.Errorf("CW square area2 = %d, want %d", got, -512*512*2)
	}
	if got := ShoelaceArea2([]Vec2i{{0, 0}, {1, 1}}); got != 0 {
		t.Errorf("degenerate area2 = %d, want 0", got)
	}
}

func TestPointInPolygon(t *testing.T) {
	poly := []Vec2i{{0, 0}, {512, 0}, {512, 512}, {0, 512}}
	inside := []Vec2i{{256, 256}, {1, 1}, {511, 511}}
	outside := []Vec2i{{-1, 256}, {513, 256}, {256, -1}, {256, 513}}
	edge := []Vec2i{{0, 0}, {256, 0}, {512, 256}, {0, 512}}

	for _, p := range inside {
		if in, on := PointInPolygon(p, poly); !in || on {
			t.Errorf("%v: in=%v on=%v, want in", p, in, on)
		}
	}
	for _, p := range outside {
		if in, on := PointInPolygon(p, poly); in || on {
			t.Errorf("%v: in=%v on=%v, want out", p, in, on)
		}
	}
	for _, p := range edge {
		if _, on := PointInPolygon(p, poly); !on {
			t.Errorf("%v: want onEdge", p)
		}
	}
}

// TestPointInPolygonConcave guards the half-open Y rule against the classic
// double-count bug at shared vertices.
func TestPointInPolygonConcave(t *testing.T) {
	// An L shape (subunits), CCW.
	poly := []Vec2i{
		{0, 0}, {1024, 0}, {1024, 256}, {256, 256}, {256, 1024}, {0, 1024},
	}
	in, _ := PointInPolygon(Vec2i{128, 512}, poly)
	if !in {
		t.Error("point in the vertical arm reported outside")
	}
	in, _ = PointInPolygon(Vec2i{512, 128}, poly)
	if !in {
		t.Error("point in the horizontal arm reported outside")
	}
	in, _ = PointInPolygon(Vec2i{512, 512}, poly)
	if in {
		t.Error("point in the notch reported inside")
	}
	// A ray leaving from y exactly at a vertex height must still be correct.
	in, _ = PointInPolygon(Vec2i{128, 256}, poly)
	if !in {
		t.Error("point level with a vertex reported outside")
	}
}

func BenchmarkSegSegIntersect(b *testing.B) {
	a1, a2 := Vec2i{-2000, -37}, Vec2i{2000, 41}
	b1, b2 := Vec2i{13, -2000}, Vec2i{-19, 2000}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SegSegIntersect(a1, a2, b1, b2)
	}
}
