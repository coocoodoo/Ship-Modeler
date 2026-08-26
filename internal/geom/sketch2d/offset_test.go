package sketch2d

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

func cos(a float64) float64 { return math.Cos(a) }
func sin(a float64) float64 { return math.Sin(a) }

// The miter offset of SPEC-GEOMETRY §5.3, which is what turns an extrude's
// draft angle into a tapered far cap. TESTING §2 asks for square inset and
// outset, an L shape, a star with spiky miters, holes offset outward, and a
// validity checker that catches self-intersection.

func TestOffsetSquare(t *testing.T) {
	sq := rectLoop(0, 0, 4, 4)

	// Positive delta moves every edge along its left normal, which for a
	// counter-clockwise loop is inward.
	in, ok := OffsetLoop(sq, u(1))
	if !ok {
		t.Fatal("insetting a square by 1 u was rejected")
	}
	want := []geom.Vec2i{pt(1, 1), pt(3, 1), pt(3, 3), pt(1, 3)}
	for i, w := range want {
		if in.Pts[i] != w {
			t.Errorf("inset corner %d = %v, want %v", i, in.Pts[i], w)
		}
	}
	if got, want := in.Area2(), rectArea2(2, 2); got != want {
		t.Errorf("inset area2 = %d, want %d", got, want)
	}

	// A negative delta grows it.
	out, ok := OffsetLoop(sq, -u(1))
	if !ok {
		t.Fatal("outsetting a square by 1 u was rejected")
	}
	if got, want := out.Area2(), rectArea2(6, 6); got != want {
		t.Errorf("outset area2 = %d, want %d", got, want)
	}
}

func TestOffsetKeepsProvenance(t *testing.T) {
	sq := Loop{
		Pts: rectLoop(0, 0, 4, 4).Pts,
		Src: []Source{{Entity: 5, Seg: 0}, {Entity: 5, Seg: 1}, {Entity: 5, Seg: 2}, {Entity: 5, Seg: 3}},
	}
	got, ok := OffsetLoop(sq, u(1))
	if !ok {
		t.Fatal("offset rejected")
	}
	// Each offset edge is the same profile edge moved, so it must keep naming
	// the entity it came from — that is what gives the side faces stable
	// identities (SPEC-GEOMETRY §5.5).
	for i := range got.Src {
		if got.Src[i] != sq.Src[i] {
			t.Errorf("edge %d lost its source: %+v, want %+v", i, got.Src[i], sq.Src[i])
		}
	}
}

func TestOffsetConcaveL(t *testing.T) {
	// An L, counter-clockwise, 4 u across each arm and 1 u thick.
	l := loopOf(
		pt(0, 0), pt(4, 0), pt(4, 1), pt(1, 1), pt(1, 4), pt(0, 4))

	got, ok := OffsetLoop(l, geom.SubunitsPerUnit/4)
	if !ok {
		t.Fatal("insetting an L by a quarter unit was rejected")
	}
	if got.Area2() <= 0 {
		t.Errorf("the inset L flipped winding: area2 = %d", got.Area2())
	}
	if got.Area2() >= l.Area2() {
		t.Errorf("insetting did not shrink the L: %d then %d", l.Area2(), got.Area2())
	}
	if len(got.Pts) != len(l.Pts) {
		t.Errorf("the offset changed the vertex count from %d to %d", len(l.Pts), len(got.Pts))
	}
	// The reflex corner moves outward from the material, the convex ones inward.
	if !ValidOffset(l, got) {
		t.Error("the validity check rejected a legal inset")
	}
}

// TestOffsetCollapsesAreRejected is the check that stops a draft angle from
// turning a profile inside out.
func TestOffsetCollapsesAreRejected(t *testing.T) {
	sq := rectLoop(0, 0, 4, 4)

	// Exactly half the width collapses the square to a point.
	if got, ok := OffsetLoop(sq, u(2)); ok && ValidOffset(sq, got) {
		t.Errorf("insetting a 4 u square by 2 u was accepted: %v", got.Pts)
	}
	// Past half, it turns inside out.
	if got, ok := OffsetLoop(sq, u(3)); ok && ValidOffset(sq, got) {
		t.Errorf("insetting a 4 u square by 3 u was accepted: %v", got.Pts)
	}
}

// TestOffsetStarSpikes covers the spiky-miter case: a star's points shoot a
// long way out under a modest offset, and the validity check is what bounds
// them rather than a separate miter limit (SPEC-GEOMETRY §5.3).
func TestOffsetStarSpikes(t *testing.T) {
	star := starLoop(5, 4, 1.4)

	small, ok := OffsetLoop(star, geom.SubunitsPerUnit/8)
	if !ok || !ValidOffset(star, small) {
		t.Error("a small inset of a star was rejected")
	}
	if small.Area2() >= star.Area2() {
		t.Error("insetting the star did not shrink it")
	}

	// A large inset eats the points and self-intersects.
	if got, ok := OffsetLoop(star, u(2)); ok && ValidOffset(star, got) {
		t.Error("insetting a star past its arms was accepted")
	}
}

// starLoop builds a counter-clockwise star with alternating radii.
func starLoop(points int, outer, inner float64) Loop {
	pts := make([]geom.Vec2i, 0, points*2)
	for i := 0; i < points*2; i++ {
		r := outer
		if i%2 == 1 {
			r = inner
		}
		a := float64(i) / float64(points*2) * 2 * 3.141592653589793
		pts = append(pts, geom.Vec2i{
			X: int64(float64(u(r)) * cos(a)),
			Y: int64(float64(u(r)) * sin(a)),
		})
	}
	return loopOf(pts...)
}

// TestOffsetRegionMovesHolesOutward covers the material-thins-consistently rule:
// one delta shrinks the outer loop and grows every hole.
func TestOffsetRegionMovesHolesOutward(t *testing.T) {
	r := Region{
		Outer: rectLoop(0, 0, 10, 10),
		Holes: []Loop{reversedLoop(rectLoop(4, 4, 6, 6))},
	}
	got, ok := OffsetRegion(r, geom.SubunitsPerUnit)
	if !ok {
		t.Fatal("offsetting a ring by 1 u was rejected")
	}

	// Outer shrank from 10x10 to 8x8.
	if want := rectArea2(8, 8); got.Outer.Area2() != want {
		t.Errorf("outer area2 = %d, want %d", got.Outer.Area2(), want)
	}
	// The hole grew from 2x2 to 4x4, and is still wound the other way.
	if want := -rectArea2(4, 4); got.Holes[0].Area2() != want {
		t.Errorf("hole area2 = %d, want %d", got.Holes[0].Area2(), want)
	}
	// The ring is thinner than it was: material thinned on both sides.
	if got.Area2() >= r.Area2() {
		t.Errorf("the ring did not thin: %d then %d", r.Area2(), got.Area2())
	}
}

func TestOffsetRegionRejectsHoleEscapingOuter(t *testing.T) {
	// A hole close to the outer edge escapes once the offset is large enough.
	r := Region{
		Outer: rectLoop(0, 0, 10, 10),
		Holes: []Loop{reversedLoop(rectLoop(1, 1, 9, 9))},
	}
	if _, ok := OffsetRegion(r, u(2)); ok {
		t.Error("an offset that pushes a hole through its outer loop was accepted")
	}
}

func TestOffsetZeroIsIdentity(t *testing.T) {
	sq := rectLoop(0, 0, 4, 4)
	got, ok := OffsetLoop(sq, 0)
	if !ok {
		t.Fatal("a zero offset was rejected")
	}
	for i := range sq.Pts {
		if got.Pts[i] != sq.Pts[i] {
			t.Errorf("a zero offset moved corner %d from %v to %v", i, sq.Pts[i], got.Pts[i])
		}
	}
}

// TestClampOffsetFindsTheLargestValidDelta is what the UX clamp reads: when the
// requested draft is too steep, the offset backs off to the most it can do and
// says how far it got (SPEC-UX §9.3).
func TestClampOffsetFindsTheLargestValidDelta(t *testing.T) {
	r := Region{Outer: rectLoop(0, 0, 4, 4)}

	// Well within range: nothing is clamped.
	got, achieved := ClampOffsetRegion(r, u(1))
	if achieved != u(1) {
		t.Errorf("a legal offset was clamped from %d to %d", u(1), achieved)
	}
	if got.Outer.Area2() != rectArea2(2, 2) {
		t.Errorf("the unclamped result is wrong: area2 %d", got.Outer.Area2())
	}

	// Past the collapse point: clamped to just under half the width.
	got, achieved = ClampOffsetRegion(r, u(5))
	if achieved >= u(2) {
		t.Errorf("clamped to %d, which still collapses a 4 u square", achieved)
	}
	if achieved <= 0 {
		t.Errorf("clamped all the way to %d, which gives up too early", achieved)
	}
	if got.Outer.Area2() <= 0 {
		t.Errorf("the clamped result is degenerate: area2 %d", got.Outer.Area2())
	}
	if !ValidOffset(r.Outer, got.Outer) {
		t.Error("the clamped result does not pass its own validity check")
	}
	// It should get close to the limit rather than settling for a token amount:
	// half of 2 u is a generous floor.
	if achieved < u(1) {
		t.Errorf("clamped to %d, want close to the 2 u limit", achieved)
	}
}

func TestClampOffsetOnAnUnclampableProfile(t *testing.T) {
	// A negative offset grows a convex loop and can never self-intersect, so it
	// is never clamped.
	r := Region{Outer: rectLoop(0, 0, 4, 4)}
	_, achieved := ClampOffsetRegion(r, -u(10))
	if achieved != -u(10) {
		t.Errorf("an outset was clamped from %d to %d", -u(10), achieved)
	}
}
