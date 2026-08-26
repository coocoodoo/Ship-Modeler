package geom

import (
	"math/big"
	"math/bits"
)

// Exact 2D predicates on sketch space (SPEC-GEOMETRY §1.1).
//
// Every function here is exact for coordinates within ±SketchClamp: the
// question "is this profile closed?" (R3) is decided by integer arithmetic
// alone, so it is deterministic forever and needs no epsilon tuning.

// Cross returns the sign-carrying cross product (b−a)×(c−a).
//
//	>0  c is left of a→b (counter-clockwise turn)
//	=0  a, b, c are collinear
//	<0  c is right of a→b (clockwise turn)
//
// Exact for clamped coordinates: the intermediate terms stay below 2^47.
func Cross(a, b, c Vec2i) int64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// Orient returns the sign of Cross as -1, 0 or +1.
func Orient(a, b, c Vec2i) int {
	switch v := Cross(a, b, c); {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// Collinear reports whether the three points lie on one line.
func Collinear(a, b, c Vec2i) bool { return Cross(a, b, c) == 0 }

// OnSegment reports whether p lies on the closed segment a→b, endpoints
// included. Exact.
func OnSegment(a, b, p Vec2i) bool {
	if Cross(a, b, p) != 0 {
		return false
	}
	return inRange(p.X, a.X, b.X) && inRange(p.Y, a.Y, b.Y)
}

func inRange(v, lo, hi int64) bool {
	if lo > hi {
		lo, hi = hi, lo
	}
	return v >= lo && v <= hi
}

// SegRel classifies how two closed segments meet (SPEC-GEOMETRY §1.1).
type SegRel int

const (
	// SegNone: the segments do not touch.
	SegNone SegRel = iota
	// SegProper: interiors cross at a single point, no endpoint involved.
	SegProper
	// SegTouch: they meet at exactly one point which is an endpoint of at least
	// one segment (T-touch, endpoint-to-endpoint, or collinear point contact).
	SegTouch
	// SegCollinear: collinear with a positive-length shared interval.
	SegCollinear
)

func (r SegRel) String() string {
	switch r {
	case SegProper:
		return "proper"
	case SegTouch:
		return "touch"
	case SegCollinear:
		return "collinear"
	default:
		return "none"
	}
}

// SegHit is the result of SegSegIntersect.
//
// For SegProper and SegTouch, P is the meeting point: exact for SegTouch, and
// rounded to the nearest subunit for SegProper. That rounding is the single
// documented rounding site of the sketch engine; it moves the point by at most
// half a subunit per axis and is always followed by a weld pass.
//
// For SegCollinear, Q0 to Q1 is the shared overlap interval and P is unset.
type SegHit struct {
	Rel    SegRel
	P      Vec2i
	Q0, Q1 Vec2i
	// Exact is false only when P was rounded.
	Exact bool
}

// SegSegIntersect classifies the closed segments a1..a2 and b1..b2.
// Degenerate (zero-length) inputs are handled as points.
func SegSegIntersect(a1, a2, b1, b2 Vec2i) SegHit {
	// Bounding-box reject first: the region engine calls this O(n^2) times.
	if !bboxOverlap(a1, a2, b1, b2) {
		return SegHit{Rel: SegNone}
	}

	d1 := a2.Sub(a1)
	d2 := b2.Sub(b1)
	aDeg := d1 == Vec2i{}
	bDeg := d2 == Vec2i{}

	switch {
	case aDeg && bDeg:
		if a1 == b1 {
			return SegHit{Rel: SegTouch, P: a1, Exact: true}
		}
		return SegHit{Rel: SegNone}
	case aDeg:
		if OnSegment(b1, b2, a1) {
			return SegHit{Rel: SegTouch, P: a1, Exact: true}
		}
		return SegHit{Rel: SegNone}
	case bDeg:
		if OnSegment(a1, a2, b1) {
			return SegHit{Rel: SegTouch, P: b1, Exact: true}
		}
		return SegHit{Rel: SegNone}
	}

	denom := d1.CrossZ(d2)
	if denom == 0 {
		// Parallel: collinear only if b1 lies on the line through a1,a2.
		if Cross(a1, a2, b1) != 0 {
			return SegHit{Rel: SegNone}
		}
		return collinearOverlap(a1, a2, b1, b2, d1)
	}

	// Non-parallel: solve the parameters exactly as rationals.
	//   a1 + t*d1 = b1 + u*d2
	//   t = cross(b1-a1, d2) / denom,  u = cross(b1-a1, d1) / denom
	w := b1.Sub(a1)
	tNum := w.CrossZ(d2)
	uNum := w.CrossZ(d1)
	if !inUnitRange(tNum, denom) || !inUnitRange(uNum, denom) {
		return SegHit{Rel: SegNone}
	}

	// Endpoint-exact cases first, so shared endpoints are never rounded.
	if p, ok := sharedEndpoint(a1, a2, b1, b2, tNum, uNum, denom); ok {
		return SegHit{Rel: SegTouch, P: p, Exact: true}
	}

	p, exact := RoundedIntersection(a1, d1, tNum, denom)
	return SegHit{Rel: SegProper, P: p, Exact: exact}
}

// sharedEndpoint returns the exact meeting point when the intersection
// parameter lands on an endpoint of either segment.
func sharedEndpoint(a1, a2, b1, b2 Vec2i, tNum, uNum, denom int64) (Vec2i, bool) {
	switch {
	case tNum == 0:
		return a1, true
	case tNum == denom:
		return a2, true
	case uNum == 0:
		return b1, true
	case uNum == denom:
		return b2, true
	}
	return Vec2i{}, false
}

// inUnitRange reports whether num/den lies in [0,1] without dividing.
func inUnitRange(num, den int64) bool {
	if den < 0 {
		num, den = -num, -den
	}
	return num >= 0 && num <= den
}

// RoundedIntersection evaluates origin + (num/den)*d and rounds each component
// to the nearest subunit. The bool reports whether the point was already exact.
func RoundedIntersection(origin, d Vec2i, num, den int64) (Vec2i, bool) {
	x, xe := addMulDivRound(origin.X, d.X, num, den)
	y, ye := addMulDivRound(origin.Y, d.Y, num, den)
	return Vec2i{x, y}, xe && ye
}

func addMulDivRound(base, a, num, den int64) (int64, bool) {
	q, exact := mulDivRound(a, num, den)
	return base + q, exact
}

// mulDivRound computes round(a*b/c), half away from zero, using a 128-bit
// intermediate. It falls back to math/big if the quotient would not fit in 64
// bits, which clamped sketch input can never cause.
func mulDivRound(a, b, c int64) (int64, bool) {
	if c == 0 {
		return 0, true
	}
	neg := false
	if a < 0 {
		a, neg = -a, !neg
	}
	if b < 0 {
		b, neg = -b, !neg
	}
	if c < 0 {
		c, neg = -c, !neg
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi >= uint64(c) {
		return bigMulDivRound(a, b, c, neg)
	}
	q, rem := bits.Div64(hi, lo, uint64(c))
	exact := rem == 0
	if rem*2 >= uint64(c) {
		q++
	}
	if neg {
		return -int64(q), exact
	}
	return int64(q), exact
}

func bigMulDivRound(a, b, c int64, neg bool) (int64, bool) {
	num := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	den := big.NewInt(c)
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	exact := r.Sign() == 0
	if new(big.Int).Lsh(r, 1).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	v := q.Int64()
	if neg {
		v = -v
	}
	return v, exact
}

// collinearOverlap classifies two collinear segments by projecting onto the
// direction of the first.
func collinearOverlap(a1, a2, b1, b2, d1 Vec2i) SegHit {
	proj := func(p Vec2i) int64 { return p.Sub(a1).Dot(d1) }
	aLo, aHi := int64(0), d1.Dot(d1)
	bLo, bHi := proj(b1), proj(b2)
	bLoP, bHiP := b1, b2
	if bLo > bHi {
		bLo, bHi = bHi, bLo
		bLoP, bHiP = bHiP, bLoP
	}
	lo, loP := aLo, a1
	if bLo > lo {
		lo, loP = bLo, bLoP
	}
	hi, hiP := aHi, a2
	if bHi < hi {
		hi, hiP = bHi, bHiP
	}
	switch {
	case lo > hi:
		return SegHit{Rel: SegNone}
	case lo == hi:
		return SegHit{Rel: SegTouch, P: loP, Exact: true}
	default:
		return SegHit{Rel: SegCollinear, Q0: loP, Q1: hiP, Exact: true}
	}
}

func bboxOverlap(a1, a2, b1, b2 Vec2i) bool {
	return maxi(a1.X, a2.X) >= mini(b1.X, b2.X) &&
		maxi(b1.X, b2.X) >= mini(a1.X, a2.X) &&
		maxi(a1.Y, a2.Y) >= mini(b1.Y, b2.Y) &&
		maxi(b1.Y, b2.Y) >= mini(a1.Y, a2.Y)
}

func mini(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxi(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ShoelaceArea2 returns twice the signed area of a closed polygon in subunits.
// Exact; positive for counter-clockwise loops.
func ShoelaceArea2(pts []Vec2i) int64 {
	n := len(pts)
	if n < 3 {
		return 0
	}
	var s int64
	for i, p := range pts {
		q := pts[(i+1)%n]
		s += p.X*q.Y - q.X*p.Y
	}
	return s
}

// PointInPolygon runs an exact ray-parity test. Points exactly on the boundary
// report onEdge, and callers decide what that means for them.
func PointInPolygon(p Vec2i, poly []Vec2i) (inside, onEdge bool) {
	n := len(poly)
	if n < 3 {
		return false, false
	}
	in := false
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		if OnSegment(a, b, p) {
			return false, true
		}
		// Half-open rule on Y so shared vertices are not counted twice.
		if (a.Y > p.Y) != (b.Y > p.Y) {
			side := Cross(a, b, p)
			if b.Y < a.Y {
				side = -side
			}
			if side > 0 {
				in = !in
			}
		}
	}
	return in, false
}
