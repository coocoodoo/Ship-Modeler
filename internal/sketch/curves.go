package sketch

import (
	"math"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// The construction arithmetic behind SK2's curve gestures (Sketch_func.md §5).
//
// It lives apart from the session because it is the part that can be wrong in
// ways a screenshot will not show: a circumcentre a few subunits out looks
// perfectly plausible and puts every later join in the wrong place. Pure
// functions, tested directly.

// collinearEps is how close to a straight line three points may be before
// their circle is refused. It is in subunit² of cross product — a hair, but
// far enough from zero that the division below cannot explode.
const collinearEps = 1.0

// Circumcentre is the centre of the circle through three points, or false when
// they are collinear (or two coincide), which has no circle at all.
//
// Computed in floats and rounded once, which is the rule for everything that
// leaves the integer lattice (SPEC-GEOMETRY §3).
func Circumcentre(a, b, c geom.Vec2i) (geom.Vec2i, bool) {
	ax, ay := float64(a.X), float64(a.Y)
	bx, by := float64(b.X), float64(b.Y)
	cx, cy := float64(c.X), float64(c.Y)

	d := 2 * (ax*(by-cy) + bx*(cy-ay) + cx*(ay-by))
	if math.Abs(d) < collinearEps {
		return geom.Vec2i{}, false
	}
	a2 := ax*ax + ay*ay
	b2 := bx*bx + by*by
	c2 := cx*cx + cy*cy
	ux := (a2*(by-cy) + b2*(cy-ay) + c2*(ay-by)) / d
	uy := (a2*(cx-bx) + b2*(ax-cx) + c2*(bx-ax)) / d
	return geom.Vec2i{X: int64(math.Round(ux)), Y: int64(math.Round(uy))}, true
}

// arcThrough builds the arc from start to end that passes through mid.
//
// The centre is the circumcentre; the direction is whichever way round
// actually reaches mid, which is the whole information the third click
// carries.
func arcThrough(start, mid, end geom.Vec2i, segs int) (model.Entity, bool) {
	c, ok := Circumcentre(start, mid, end)
	if !ok {
		return model.Entity{}, false
	}
	return model.NewArc(c, start, end, ccwReaches(c, start, mid, end), segs), true
}

// ccwReaches reports whether sweeping counter-clockwise from start arrives at
// the through-point before it arrives at the end.
//
// That is exactly the question the third click answers: both directions join
// the same two ends, and only one of them passes through the point the user
// asked for.
func ccwReaches(c, start, through, end geom.Vec2i) bool {
	a0 := angleOf(c, start)
	toThrough := norm2Pi(angleOf(c, through) - a0)
	toEnd := norm2Pi(angleOf(c, end) - a0)
	return toThrough < toEnd
}

// arcToward builds the arc centred at c, starting at start, sweeping the short
// way toward the direction of toward.
func arcToward(c, start, toward geom.Vec2i, segs int) (model.Entity, bool) {
	if start == c || toward == c {
		return model.Entity{}, false
	}
	sweep := norm2Pi(angleOf(c, toward) - angleOf(c, start))
	// Past half a turn the counter-clockwise sweep is the long way round, so
	// the user meant the other one.
	ccw := sweep <= math.Pi
	// The end point is the click projected onto the arc's own radius, so the
	// arc really passes through the direction that was clicked.
	radius := start.Sub(c).Len()
	end := pointAt(c, angleOf(c, toward), radius)
	return model.NewArc(c, start, end, ccw, segs), true
}

// arcTangent builds the arc that leaves start along dir and ends at end.
//
// The centre must lie on the line through start perpendicular to dir — that is
// what tangency means — and be equidistant from both ends, which pins it
// exactly. When end lies straight ahead there is no such circle: the answer
// would be a straight line, and this tool does not draw those.
func arcTangent(start, dir, end geom.Vec2i, segs int) (model.Entity, bool) {
	d := geom.Vec2{X: float64(dir.X), Y: float64(dir.Y)}
	l := math.Hypot(d.X, d.Y)
	if l == 0 || start == end {
		return model.Entity{}, false
	}
	d.X, d.Y = d.X/l, d.Y/l
	n := geom.Vec2{X: -d.Y, Y: d.X} // the perpendicular the centre sits on

	v := geom.Vec2{X: float64(end.X - start.X), Y: float64(end.Y - start.Y)}
	den := 2 * (v.X*n.X + v.Y*n.Y)
	if math.Abs(den) < collinearEps {
		return model.Entity{}, false // dead ahead: that is a line, not an arc
	}
	s := (v.X*v.X + v.Y*v.Y) / den
	c := geom.Vec2i{
		X: start.X + int64(math.Round(n.X*s)),
		Y: start.Y + int64(math.Round(n.Y*s)),
	}
	// Which way round: the side the end point falls on, read off the cross
	// product of the tangent with the chord.
	ccw := d.X*v.Y-d.Y*v.X > 0
	return model.NewArc(c, start, end, ccw, segs), true
}

// ellipseThrough builds the oval centred at c with its major axis running to
// major, and its minor axis reaching as far across as p lies from that axis.
func ellipseThrough(c, major, p geom.Vec2i, segs int) (model.Entity, bool) {
	ax := float64(major.X - c.X)
	ay := float64(major.Y - c.Y)
	l := math.Hypot(ax, ay)
	if l == 0 {
		return model.Entity{}, false
	}
	// Distance from p to the major axis: the perpendicular component.
	nx, ny := -ay/l, ax/l
	minor := math.Abs(float64(p.X-c.X)*nx + float64(p.Y-c.Y)*ny)
	w := int64(math.Round(minor))
	if w <= 0 {
		return model.Entity{}, false
	}
	return model.NewEllipse(c, major, w, segs), true
}

// angleOf is the direction from c to p, in radians.
func angleOf(c, p geom.Vec2i) float64 {
	return math.Atan2(float64(p.Y-c.Y), float64(p.X-c.X))
}

// pointAt is the position at an angle and radius from a centre.
func pointAt(c geom.Vec2i, angle, radius float64) geom.Vec2i {
	return geom.Vec2i{
		X: c.X + int64(math.Round(radius*math.Cos(angle))),
		Y: c.Y + int64(math.Round(radius*math.Sin(angle))),
	}
}

// norm2Pi folds an angle into [0, 2π).
func norm2Pi(a float64) float64 {
	for a < 0 {
		a += 2 * math.Pi
	}
	for a >= 2*math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

// endpointNear finds an open entity's endpoint within radius of p, and the
// direction the entity arrives at it in. It is what the tangent arc hangs off.
func endpointNear(p geom.Vec2i, ents []model.Entity, radius int64) (at geom.Vec2i, dir geom.Vec2i, ok bool) {
	best := float64(radius) + 1
	for i := range ents {
		e := ents[i]
		if e.Closed() {
			continue // a loop has no loose end to continue from
		}
		pts := e.Points()
		if len(pts) < 2 {
			continue
		}
		// Both ends, each with the direction the curve leaves in.
		ends := [2]struct{ at, dir geom.Vec2i }{
			{pts[0], pts[0].Sub(pts[1])},
			{pts[len(pts)-1], pts[len(pts)-1].Sub(pts[len(pts)-2])},
		}
		for _, end := range ends {
			if d := p.Sub(end.at).Len(); d < best {
				best, at, dir, ok = d, end.at, end.dir, true
			}
		}
	}
	return at, dir, ok
}

// The construction helpers, exported for the script ops so a scripted arc is
// built by exactly the code an interactive one is.
func ArcThrough(start, mid, end geom.Vec2i, segs int) (model.Entity, bool) {
	return arcThrough(start, mid, end, segs)
}

func ArcToward(c, start, toward geom.Vec2i, segs int) (model.Entity, bool) {
	return arcToward(c, start, toward, segs)
}

func ArcTangent(start, dir, end geom.Vec2i, segs int) (model.Entity, bool) {
	return arcTangent(start, dir, end, segs)
}

func EllipseThrough(c, major, p geom.Vec2i, segs int) (model.Entity, bool) {
	return ellipseThrough(c, major, p, segs)
}

func EndpointNear(p geom.Vec2i, ents []model.Entity, radius int64) (geom.Vec2i, geom.Vec2i, bool) {
	return endpointNear(p, ents, radius)
}

// slotCapSegments is how many segments each rounded end of a slot gets. Eight
// is enough that a cap reads as round at working zoom and few enough that a
// row of portholes does not bury the region engine (Sketch_func.md §3).
const slotCapSegments = 8

// slotThrough builds a capsule between two centres, as wide across as p lies
// from the line joining them.
//
// The width is measured perpendicular, so sliding the third click along the
// track does not change the slot — only moving it across does, which is what
// the gesture looks like it should do.
func slotThrough(a, b, p geom.Vec2i) (model.Entity, bool) {
	d := b.Sub(a)
	l := d.Len()
	if l == 0 {
		return model.Entity{}, false
	}
	nx, ny := -float64(d.Y)/l, float64(d.X)/l
	half := math.Abs(float64(p.X-a.X)*nx + float64(p.Y-a.Y)*ny)
	w := int64(math.Round(half))
	if w <= 0 {
		return model.Entity{}, false
	}
	e := model.NewSlot(a, b, w, slotCapSegments)
	return e, !e.Degenerate()
}

// SlotThrough is slotThrough for the script op, so a scripted slot is the same
// shape a clicked one is.
func SlotThrough(a, b, p geom.Vec2i) (model.Entity, bool) { return slotThrough(a, b, p) }
