package sketch

import (
	"fmt"
	"math"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// The modify tools of Sketch_func.md §5 SK5: operations on what is already
// drawn rather than gestures that draw something new.
//
// They live here, apart from the app and free of any UI, for the same reason
// the curve construction does: these are the places a small error is
// invisible. A fillet arc a subunit off its lines leaves two hairline gaps
// that turn a closed profile into four open ends, and nothing says so until an
// extrude refuses. Pure functions, tested directly.
//
// Every one of them produces entities for a model.ReplaceEntities, so a whole
// gesture lands as one undo step (V-98).

// FilletResult is what a corner treatment produces: the trimmed legs, and for
// a fillet the arc that joins them.
type FilletResult struct {
	// Lines are the entities that replace the two originals. A fillet trims
	// both legs; a chamfer trims both and adds the cut across.
	Lines []model.Entity
	// Arc is the rounded corner, valid for a fillet.
	Arc model.Entity
}

// Fillet rounds the corner where two lines meet, with the given radius.
func Fillet(a, b model.Entity, radius int64) (FilletResult, error) {
	c, da, db, err := corner(a, b)
	if err != nil {
		return FilletResult{}, err
	}
	if radius <= 0 {
		return FilletResult{}, fmt.Errorf("a fillet needs a radius")
	}

	// Half the angle between the legs decides how far back the arc starts:
	// tan of it relates the trim distance to the radius.
	half := math.Acos(clamp(da.dot(db), -1, 1)) / 2
	if half <= 1e-6 || math.Abs(half-math.Pi/2) <= 1e-6 {
		return FilletResult{}, fmt.Errorf("those two lines do not turn a corner")
	}
	trim := float64(radius) / math.Tan(half)

	if reach := math.Min(lengthOf(a), lengthOf(b)); trim > reach {
		most := reach * math.Tan(half)
		return FilletResult{}, fmt.Errorf(
			"that radius does not fit — %.2f u is the most this corner takes",
			most/geom.Unit)
	}

	// Where the arc meets each leg, and the centre a radius in from both.
	pa := da.scaledFrom(c, trim)
	pb := db.scaledFrom(c, trim)
	bisect := da.add(db).unit()
	if bisect.zero() {
		return FilletResult{}, fmt.Errorf("those two lines do not turn a corner")
	}
	centre := bisect.scaledFrom(c, float64(radius)/math.Sin(half))

	// A fillet is always the minor arc: the short way from one tangent point
	// to the other. Deriving the winding from the legs' cross product instead
	// picks the long way round half the time, and a 270° arc across the corner
	// both looks absurd and closes a region that was never drawn.
	sweep := norm2Pi(angleOf(centre, pb) - angleOf(centre, pa))
	arc := model.NewArc(centre, pa, pb, sweep <= math.Pi, model.DefaultCircleSegs)

	return FilletResult{
		Lines: []model.Entity{trimTo(a, c, pa), trimTo(b, c, pb)},
		Arc:   arc,
	}, nil
}

// Chamfer cuts the corner off square, trimming each leg by the same distance
// and joining what is left.
func Chamfer(a, b model.Entity, distance int64) (FilletResult, error) {
	c, da, db, err := corner(a, b)
	if err != nil {
		return FilletResult{}, err
	}
	if distance <= 0 {
		return FilletResult{}, fmt.Errorf("a chamfer needs a distance")
	}
	d := float64(distance)
	if reach := math.Min(lengthOf(a), lengthOf(b)); d > reach {
		return FilletResult{}, fmt.Errorf(
			"that chamfer does not fit — %.2f u is the most this corner takes",
			reach/geom.Unit)
	}
	pa := da.scaledFrom(c, d)
	pb := db.scaledFrom(c, d)
	if pa == pb {
		return FilletResult{}, fmt.Errorf("those two lines do not turn a corner")
	}
	return FilletResult{
		Lines: []model.Entity{
			trimTo(a, c, pa),
			trimTo(b, c, pb),
			model.NewLine(pa, pb),
		},
	}, nil
}

// corner finds the point two lines share and the unit directions away from it
// along each. Both are needed by fillet and chamfer alike.
func corner(a, b model.Entity) (c geom.Vec2i, da, db vec2, err error) {
	if a.Kind != model.EntLine || b.Kind != model.EntLine {
		return c, da, db, fmt.Errorf("only two straight lines can be filleted or chamfered")
	}
	// The shared endpoint, whichever ends touch.
	var otherA, otherB geom.Vec2i
	switch {
	case a.B == b.A:
		c, otherA, otherB = a.B, a.A, b.B
	case a.B == b.B:
		c, otherA, otherB = a.B, a.A, b.A
	case a.A == b.A:
		c, otherA, otherB = a.A, a.B, b.B
	case a.A == b.B:
		c, otherA, otherB = a.A, a.B, b.A
	default:
		return c, da, db, fmt.Errorf("those two lines do not share a corner")
	}
	da = toward(c, otherA)
	db = toward(c, otherB)
	if da.zero() || db.zero() {
		return c, da, db, fmt.Errorf("one of those lines has no length")
	}
	if math.Abs(da.cross(db)) < 1e-9 {
		return c, da, db, fmt.Errorf("those two lines do not turn a corner")
	}
	return c, da, db, nil
}

// trimTo shortens a line so the end that was at the corner is now at p.
func trimTo(e model.Entity, cornerAt, p geom.Vec2i) model.Entity {
	if e.B == cornerAt {
		return model.NewLine(e.A, p)
	}
	return model.NewLine(p, e.B)
}

func lengthOf(e model.Entity) float64 { return e.B.Sub(e.A).Len() }

// MirrorEntity reflects an entity about the line through axisA and axisB.
//
// Reflection is exact in integers when the axis is exact, which is what lets a
// mirrored hull half meet its original without a seam. An arc also flips its
// winding: a reflected sweep runs the other way round, and one that did not
// would bulge the wrong side.
func MirrorEntity(e model.Entity, axisA, axisB geom.Vec2i) model.Entity {
	m := e
	m.A = reflectAcross(e.A, axisA, axisB)
	m.B = reflectAcross(e.B, axisA, axisB)
	m.C = reflectAcross(e.C, axisA, axisB)
	m.D = reflectAcross(e.D, axisA, axisB)
	if len(e.Pts) > 0 {
		pts := make([]geom.Vec2i, len(e.Pts))
		for i, p := range e.Pts {
			pts[i] = reflectAcross(p, axisA, axisB)
		}
		m.Pts = pts
	}
	if e.Kind == model.EntArc {
		m.R = -e.R
	}
	return m
}

// reflectAcross mirrors a point about a line.
func reflectAcross(p, a, b geom.Vec2i) geom.Vec2i {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	den := dx*dx + dy*dy
	if den == 0 {
		return p
	}
	// The reflection formula for a point about the line through a with
	// direction (dx,dy), rounded once at the end (SPEC-GEOMETRY §3).
	px, py := float64(p.X-a.X), float64(p.Y-a.Y)
	t := (px*dx + py*dy) / den
	// The foot of the perpendicular, doubled.
	fx, fy := 2*t*dx-px, 2*t*dy-py
	return geom.Vec2i{
		X: a.X + int64(math.Round(fx)),
		Y: a.Y + int64(math.Round(fy)),
	}
}

// LinearPattern repeats entities along a delta. count includes the original,
// so a count of four returns three copies.
func LinearPattern(ents []model.Entity, delta geom.Vec2i, count int) []model.Entity {
	if count < 2 || delta == (geom.Vec2i{}) {
		return nil
	}
	out := make([]model.Entity, 0, len(ents)*(count-1))
	for i := 1; i < count; i++ {
		step := geom.Vec2i{X: delta.X * int64(i), Y: delta.Y * int64(i)}
		for _, e := range ents {
			out = append(out, e.Translate(step))
		}
	}
	return out
}

// CircularPattern repeats entities about a pivot, spread across a sweep in
// degrees. count includes the original.
//
// A full turn spaces them by sweep/count, so the last copy does not land on
// the first; anything less spreads them across the sweep inclusive, which is
// what "three across ninety degrees" plainly means.
func CircularPattern(ents []model.Entity, pivot geom.Vec2i, count int, sweepDeg float64) []model.Entity {
	if count < 2 {
		return nil
	}
	full := math.Abs(math.Abs(sweepDeg)-360) < 1e-9
	steps := float64(count)
	if !full {
		steps = float64(count - 1)
	}
	if steps == 0 {
		return nil
	}
	out := make([]model.Entity, 0, len(ents)*(count-1))
	for i := 1; i < count; i++ {
		a := sweepDeg * math.Pi / 180 * float64(i) / steps
		for _, e := range ents {
			out = append(out, rotateEntity(e, pivot, a))
		}
	}
	return out
}

// rotateEntity turns an entity about a pivot by an angle in radians.
func rotateEntity(e model.Entity, pivot geom.Vec2i, angle float64) model.Entity {
	sin, cos := math.Sin(angle), math.Cos(angle)
	turn := func(p geom.Vec2i) geom.Vec2i {
		if p == (geom.Vec2i{}) && e.Kind != model.EntPoint {
			// An unset field stays unset: rotating it would invent a position
			// the entity does not use.
			return p
		}
		x, y := float64(p.X-pivot.X), float64(p.Y-pivot.Y)
		return geom.Vec2i{
			X: pivot.X + int64(math.Round(x*cos-y*sin)),
			Y: pivot.Y + int64(math.Round(x*sin+y*cos)),
		}
	}
	r := e
	r.A, r.B, r.C, r.D = turn(e.A), turn(e.B), turn(e.C), turn(e.D)
	if len(e.Pts) > 0 {
		pts := make([]geom.Vec2i, len(e.Pts))
		for i, p := range e.Pts {
			pts[i] = turn(p)
		}
		r.Pts = pts
	}
	return r
}

// OffsetLoop moves every edge of a closed loop out by distance, positive
// outward. The corners are mitred: each new corner is where the two moved
// edges cross.
//
// It refuses rather than emitting a self-intersecting result, because a bowtie
// is not a shape the region engine can make sense of and a silently wrong
// profile is worse than a refusal (Sketch_func.md §6).
func OffsetLoop(loop []geom.Vec2i, distance int64) ([]geom.Vec2i, error) {
	n := len(loop)
	if n < 3 {
		return nil, fmt.Errorf("an offset needs a closed loop")
	}
	if distance == 0 {
		return nil, fmt.Errorf("an offset needs a distance")
	}
	// Which way round the loop runs decides which side "outward" is.
	sign := 1.0
	if signedArea(loop) < 0 {
		sign = -1
	}
	d := float64(distance) * sign

	out := make([]geom.Vec2i, n)
	for i := 0; i < n; i++ {
		prev := loop[(i-1+n)%n]
		cur := loop[i]
		next := loop[(i+1)%n]

		// The two edges meeting here, moved out along their own normals.
		n1, ok1 := outwardNormal(prev, cur)
		n2, ok2 := outwardNormal(cur, next)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("that loop has a zero-length edge")
		}
		// The mitre direction bisects them, and the mitre length grows as the
		// corner sharpens.
		bx, by := n1.X+n2.X, n1.Y+n2.Y
		l := math.Hypot(bx, by)
		if l < 1e-9 {
			return nil, fmt.Errorf("that loop doubles back on itself")
		}
		bx, by = bx/l, by/l
		cosHalf := bx*n1.X + by*n1.Y
		if math.Abs(cosHalf) < 1e-6 {
			return nil, fmt.Errorf("that corner is too sharp to offset")
		}
		reach := d / cosHalf
		out[i] = geom.Vec2i{
			X: cur.X + int64(math.Round(bx*reach)),
			Y: cur.Y + int64(math.Round(by*reach)),
		}
	}

	// An offset that turned the shape inside out shows as a flipped or
	// collapsed area. Catching it here is cheaper and clearer than letting the
	// region engine puzzle over a bowtie.
	before, after := signedArea(loop), signedArea(out)
	if after == 0 || (before > 0) != (after > 0) {
		return nil, fmt.Errorf("that offset collapses the shape — try a smaller distance")
	}
	if distance < 0 && math.Abs(after) >= math.Abs(before) {
		return nil, fmt.Errorf("that offset collapses the shape — try a smaller distance")
	}
	return out, nil
}

// outwardNormal is the left-hand normal of an edge, as a unit vector.
func outwardNormal(a, b geom.Vec2i) (geom.Vec2, bool) {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	l := math.Hypot(dx, dy)
	if l == 0 {
		return geom.Vec2{}, false
	}
	return geom.Vec2{X: dy / l, Y: -dx / l}, true
}

// signedArea is twice the area of a polygon, negative when it winds clockwise.
func signedArea(p []geom.Vec2i) float64 {
	var a float64
	for i := range p {
		j := (i + 1) % len(p)
		a += float64(p[i].X)*float64(p[j].Y) - float64(p[j].X)*float64(p[i].Y)
	}
	return a / 2
}

// vec2 is a unit direction in sketch space, used by the corner maths.
type vec2 struct{ X, Y float64 }

func toward(from, to geom.Vec2i) vec2 {
	dx, dy := float64(to.X-from.X), float64(to.Y-from.Y)
	l := math.Hypot(dx, dy)
	if l == 0 {
		return vec2{}
	}
	return vec2{X: dx / l, Y: dy / l}
}

func (v vec2) zero() bool           { return v.X == 0 && v.Y == 0 }
func (v vec2) dot(o vec2) float64   { return v.X*o.X + v.Y*o.Y }
func (v vec2) cross(o vec2) float64 { return v.X*o.Y - v.Y*o.X }
func (v vec2) add(o vec2) vec2      { return vec2{X: v.X + o.X, Y: v.Y + o.Y} }

func (v vec2) unit() vec2 {
	l := math.Hypot(v.X, v.Y)
	if l == 0 {
		return vec2{}
	}
	return vec2{X: v.X / l, Y: v.Y / l}
}

// scaledFrom is the point d away from base along v.
func (v vec2) scaledFrom(base geom.Vec2i, d float64) geom.Vec2i {
	return geom.Vec2i{
		X: base.X + int64(math.Round(v.X*d)),
		Y: base.Y + int64(math.Round(v.Y*d)),
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
