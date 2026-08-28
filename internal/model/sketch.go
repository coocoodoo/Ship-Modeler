package model

import (
	"fmt"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/sketch2d"
)

// Sketch entities (SPEC-GEOMETRY §3). A sketch stores what the user drew in its
// logical form — a rectangle stays a rectangle — and expands to segments only
// when the region engine needs them. That is what lets a whole rectangle be
// selected, moved and deleted as one thing.

// EntityKind is which of the three drawing tools produced an entity.
type EntityKind uint8

// The kinds are serialized by value, so they are append-only and never
// reordered (Sketch_func.md §3).
const (
	// EntLine is a single straight segment.
	EntLine EntityKind = iota
	// EntRect is an axis-aligned rectangle, stored by opposite corners and
	// expanded to four segments.
	EntRect
	// EntCircle is a regular polygon inscribed in a radius: a circle in this
	// app is an n-gon, which is what keeps the result crisp and low-poly (D-06).
	EntCircle
	// EntPoint is a bare position. It makes no segments and so never joins a
	// region; it exists to be snapped to and measured from.
	EntPoint
	// EntArc is part of a circle: centre C, from A round to B, the direction
	// in R (+1 counter-clockwise, -1 clockwise).
	EntArc
	// EntEllipse is a closed oval: centre C, major-axis endpoint A, semi-minor
	// axis W. The angle of A about C is the rotation.
	EntEllipse
	// EntPolygon is a regular n-gon: centre C, first vertex A, sides in Segs.
	// A circumscribed polygon is stored as its inscribed equivalent, so there
	// is one kind and no variant flag (Sketch_func.md §2).
	EntPolygon
	// EntSlot is a capsule: a track from centre A to centre B, W across.
	EntSlot
	// EntSpline is a smooth curve through the points in Pts, subdivided Segs
	// times per span. R is 1 when the curve closes back on its start.
	EntSpline
	// EntBezier is one cubic: Pts holds its four control points.
	EntBezier
)

// Spline subdivision limits (Sketch_func.md §3). The ceiling comes from
// BenchmarkBuildLoop: a closed spline through a dozen points at 16 is under
// 200 short segments, which the region engine builds in a fifth of a
// millisecond.
const (
	MinSplineSegs     = 2
	MaxSplineSegs     = 16
	DefaultSplineSegs = 8
)

func clampSplineSegs(n int) int {
	if n < MinSplineSegs {
		return MinSplineSegs
	}
	if n > MaxSplineSegs {
		return MaxSplineSegs
	}
	return n
}

// Polygon side-count limits (Sketch_func.md §3). The ceiling is lower than a
// circle's because past it a polygon is a circle, and the circle tool is the
// one with the segment control.
const (
	MinPolygonSides     = 3
	MaxPolygonSides     = 24
	DefaultPolygonSides = 6
)

func clampSides(n int) int {
	if n < MinPolygonSides {
		return MinPolygonSides
	}
	if n > MaxPolygonSides {
		return MaxPolygonSides
	}
	return n
}

func (k EntityKind) String() string {
	switch k {
	case EntRect:
		return "rectangle"
	case EntCircle:
		return "circle"
	case EntPoint:
		return "point"
	case EntArc:
		return "arc"
	case EntEllipse:
		return "ellipse"
	case EntPolygon:
		return "polygon"
	case EntSlot:
		return "slot"
	case EntSpline:
		return "spline"
	case EntBezier:
		return "bezier"
	default:
		return "line"
	}
}

// Circle segment-count limits (SPEC-UX §8.3).
const (
	MinCircleSegs     = 3
	MaxCircleSegs     = 64
	DefaultCircleSegs = 16
)

// Entity is one drawn thing in a sketch. The fields it uses depend on Kind:
// a line uses A and B, a rectangle uses A and B as opposite corners, a circle
// uses C, R and Segs, and a point uses A alone. The later kinds of
// Sketch_func.md §3 draw on D, W and Pts.
//
// One flat struct rather than an interface per kind: an entity is data that
// gets saved, compared, translated and JSON round-tripped, and the only
// behaviour that varies is Points(). A sum type would buy nothing and cost a
// serialization scheme.
type Entity struct {
	Kind EntityKind `json:"kind"`
	A    geom.Vec2i `json:"a,omitempty"`
	B    geom.Vec2i `json:"b,omitempty"`
	C    geom.Vec2i `json:"c,omitempty"`
	// D is a fourth anchor, for the kinds that need one.
	D geom.Vec2i `json:"d,omitempty"`
	R int64      `json:"r,omitempty"`
	// W is a second scalar: a slot's half-width, an ellipse's semi-minor axis.
	W    int64 `json:"w,omitempty"`
	Segs int   `json:"segs,omitempty"`
	// Pts carries a variable-length point list for splines and beziers.
	Pts []geom.Vec2i `json:"pts,omitempty"`

	// Construction marks a guide: something to snap to and measure against
	// that is deliberately not part of the shape. Construction entities are
	// skipped when the sketch expands to segments, so they never close a
	// region and never show as an open end (Sketch_func.md §3).
	Construction bool `json:"cx,omitempty"`
}

// NewLine builds a line entity.
func NewLine(a, b geom.Vec2i) Entity { return Entity{Kind: EntLine, A: a, B: b} }

// NewRect builds a rectangle from two opposite corners.
func NewRect(a, b geom.Vec2i) Entity { return Entity{Kind: EntRect, A: a, B: b} }

// NewPoint builds a bare position.
func NewPoint(a geom.Vec2i) Entity { return Entity{Kind: EntPoint, A: a} }

// NewArc builds an arc of the circle centred at c, running from start round to
// end. ccw picks which way round: the two directions are different shapes, and
// which one the user meant is decided by the gesture, not by the endpoints.
//
// The radius is taken from the start point, so the arc passes through it
// exactly. An end point off that circle is projected onto it by angle, which
// is what every construction gesture wants — three-point and centre arcs alike
// hand over an end that is only approximately on the radius.
func NewArc(c, start, end geom.Vec2i, ccw bool, segs int) Entity {
	dir := int64(1)
	if !ccw {
		dir = -1
	}
	return Entity{Kind: EntArc, C: c, A: start, B: end, R: dir, Segs: clampSegs(segs)}
}

// NewEllipse builds an oval centred at c whose major axis runs to major and
// whose semi-minor axis is minor.
func NewEllipse(c, major geom.Vec2i, minor int64, segs int) Entity {
	return Entity{Kind: EntEllipse, C: c, A: major, W: minor, Segs: clampSegs(segs)}
}

// NewPolygon builds a regular n-gon with a vertex at the given point: the
// clicked corner is a corner of the result.
func NewPolygon(c, vertex geom.Vec2i, sides int) Entity {
	return Entity{Kind: EntPolygon, C: c, A: vertex, Segs: clampSides(sides)}
}

// NewCircumscribedPolygon builds the n-gon whose flat sides — rather than its
// corners — touch the given radius, which is how a nut or a bolt head is
// measured.
//
// It is stored as an ordinary polygon with the corner radius that produces
// that: r / cos(pi/n). One kind, no variant flag to carry through
// serialization, selection and every later tool (Sketch_func.md §2).
func NewCircumscribedPolygon(c, midSide geom.Vec2i, sides int) Entity {
	n := clampSides(sides)
	d := midSide.Sub(c)
	r := d.Len() / math.Cos(math.Pi/float64(n))
	// The first vertex sits at the same angle the clicked point did, so the
	// polygon is oriented the way the drag was.
	a := math.Atan2(float64(d.Y), float64(d.X))
	vertex := geom.Vec2i{
		X: c.X + int64(math.Round(r*math.Cos(a))),
		Y: c.Y + int64(math.Round(r*math.Sin(a))),
	}
	return NewPolygon(c, vertex, n)
}

// NewSlot builds a capsule from centre to centre, half is its half-width.
// caps is how many segments each rounded end is drawn with.
func NewSlot(a, b geom.Vec2i, half int64, caps int) Entity {
	if caps < 2 {
		caps = 2
	}
	return Entity{Kind: EntSlot, A: a, B: b, W: half, Segs: caps}
}

// NewSpline builds a smooth curve through the given points. closed joins the
// last back to the first.
func NewSpline(through []geom.Vec2i, closed bool, segs int) Entity {
	e := Entity{
		Kind: EntSpline,
		Pts:  append([]geom.Vec2i(nil), through...),
		Segs: clampSplineSegs(segs),
	}
	if closed {
		e.R = 1
	}
	return e
}

// NewBezier builds one cubic from its four control points: the curve runs from
// a to d, pulled toward b and c without reaching them.
func NewBezier(a, b, c, d geom.Vec2i, segs int) Entity {
	return Entity{
		Kind: EntBezier,
		Pts:  []geom.Vec2i{a, b, c, d},
		Segs: clampSplineSegs(segs),
	}
}

// CCW reports an arc's sweep direction.
func (e Entity) CCW() bool { return e.R >= 0 }

// NewCircle builds an n-gon. The segment count is clamped to the legal range.
func NewCircle(c geom.Vec2i, r int64, segs int) Entity {
	return Entity{Kind: EntCircle, C: c, R: r, Segs: clampSegs(segs)}
}

func clampSegs(n int) int {
	if n < MinCircleSegs {
		return MinCircleSegs
	}
	if n > MaxCircleSegs {
		return MaxCircleSegs
	}
	return n
}

// Degenerate reports whether an entity has collapsed to nothing and should not
// be kept.
func (e Entity) Degenerate() bool {
	switch e.Kind {
	case EntRect:
		return e.A.X == e.B.X || e.A.Y == e.B.Y
	case EntCircle:
		return e.R <= 0
	case EntPoint:
		// A point is its own reason for existing, and one at the origin — where
		// A happens to equal the unset B — is a real place to put one.
		return false
	case EntArc:
		// Only a radius of nothing kills an arc. A start and end in the same
		// place is a full turn, which is a ring and a perfectly good shape.
		return e.A == e.C
	case EntEllipse:
		return e.A == e.C || e.W <= 0
	case EntPolygon:
		return e.A == e.C
	case EntSlot:
		// Both a width and two distinct centres. A slot whose centres coincide
		// is a circle, and the circle tool draws those.
		return e.W <= 0 || e.A == e.B
	case EntSpline, EntBezier:
		// Two distinct points somewhere in the list, or there is no curve.
		if len(e.Pts) < 2 {
			return true
		}
		for _, p := range e.Pts[1:] {
			if p != e.Pts[0] {
				return false
			}
		}
		return true
	default:
		return e.A == e.B
	}
}

// Points returns the entity's corner positions in draw order. For a line that
// is its two endpoints, for a rectangle its four corners, for a circle its
// n-gon vertices.
func (e Entity) Points() []geom.Vec2i {
	switch e.Kind {
	case EntPoint:
		return []geom.Vec2i{e.A}
	case EntRect:
		return []geom.Vec2i{
			{X: e.A.X, Y: e.A.Y},
			{X: e.B.X, Y: e.A.Y},
			{X: e.B.X, Y: e.B.Y},
			{X: e.A.X, Y: e.B.Y},
		}
	case EntCircle:
		n := clampSegs(e.Segs)
		pts := make([]geom.Vec2i, n)
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			// Rounding to subunits here is the only inexact step; exactness
			// resumes immediately afterwards (SPEC-GEOMETRY §3).
			pts[i] = geom.Vec2i{
				X: e.C.X + int64(math.Round(float64(e.R)*math.Cos(a))),
				Y: e.C.Y + int64(math.Round(float64(e.R)*math.Sin(a))),
			}
		}
		return pts
	case EntArc:
		return e.arcPoints()
	case EntEllipse:
		return e.ellipsePoints()
	case EntPolygon:
		return e.polygonPoints()
	case EntSlot:
		return e.slotPoints()
	case EntSpline:
		return e.splinePoints()
	case EntBezier:
		return e.bezierPoints()
	default:
		return []geom.Vec2i{e.A, e.B}
	}
}

// arcPoints tessellates an arc, starting and ending exactly on the points it
// was built from.
//
// The exactness at the ends is the whole contract: those two positions are
// snap targets and the places lines join, so they are written back verbatim
// rather than left as whatever the cosine rounded to. Everything between them
// is ordinary rounding (SPEC-GEOMETRY §3).
func (e Entity) arcPoints() []geom.Vec2i {
	radius := e.A.Sub(e.C).Len()
	if radius <= 0 {
		return []geom.Vec2i{e.A}
	}
	start := math.Atan2(float64(e.A.Y-e.C.Y), float64(e.A.X-e.C.X))
	end := math.Atan2(float64(e.B.Y-e.C.Y), float64(e.B.X-e.C.X))

	sweep := end - start
	if e.CCW() {
		for sweep <= 0 {
			sweep += 2 * math.Pi
		}
	} else {
		for sweep >= 0 {
			sweep -= 2 * math.Pi
		}
	}

	// Segments are spent at the same density a whole circle would use, so a
	// quarter arc is as smooth as a quarter of a circle and no smoother.
	full := clampSegs(e.Segs)
	n := int(math.Round(float64(full) * math.Abs(sweep) / (2 * math.Pi)))
	if n < 1 {
		n = 1
	}

	pts := make([]geom.Vec2i, n+1)
	for i := 0; i <= n; i++ {
		a := start + sweep*float64(i)/float64(n)
		pts[i] = geom.Vec2i{
			X: e.C.X + int64(math.Round(radius*math.Cos(a))),
			Y: e.C.Y + int64(math.Round(radius*math.Sin(a))),
		}
	}
	// The ends are exactly where they were placed, whatever the trigonometry
	// made of them.
	pts[0] = e.A
	pts[n] = e.B
	return pts
}

// splinePoints tessellates a Catmull-Rom curve through its control points.
//
// Catmull-Rom rather than a B-spline because it interpolates: the curve goes
// through the points that were clicked, which makes them snap targets and
// makes the tool behave the way its preview looked. The ends of an open curve
// duplicate their neighbours, which is the standard way to give the first and
// last spans a phantom control point.
func (e Entity) splinePoints() []geom.Vec2i {
	src := e.Pts
	if len(src) < 2 {
		return append([]geom.Vec2i(nil), src...)
	}
	closed := e.R == 1
	n := clampSplineSegs(e.Segs)

	// at wraps for a closed curve and clamps for an open one, which is what
	// makes the phantom endpoints work.
	at := func(i int) geom.Vec2i {
		if closed {
			m := len(src)
			return src[((i%m)+m)%m]
		}
		if i < 0 {
			return src[0]
		}
		if i >= len(src) {
			return src[len(src)-1]
		}
		return src[i]
	}

	spans := len(src) - 1
	if closed {
		spans = len(src)
	}
	out := make([]geom.Vec2i, 0, spans*n+1)
	for i := 0; i < spans; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		// The span starts exactly on its control point, every time (V-99).
		out = append(out, p1)
		for k := 1; k < n; k++ {
			t := float64(k) / float64(n)
			out = append(out, catmullRom(p0, p1, p2, p3, t))
		}
	}
	if !closed {
		out = append(out, src[len(src)-1])
	}
	return out
}

// catmullRom is the uniform curve through p1 and p2, shaped by its neighbours.
func catmullRom(p0, p1, p2, p3 geom.Vec2i, t float64) geom.Vec2i {
	t2 := t * t
	t3 := t2 * t
	axis := func(a, b, c, d int64) int64 {
		v := 0.5 * (2*float64(b) +
			(-float64(a)+float64(c))*t +
			(2*float64(a)-5*float64(b)+4*float64(c)-float64(d))*t2 +
			(-float64(a)+3*float64(b)-3*float64(c)+float64(d))*t3)
		return int64(math.Round(v))
	}
	return geom.Vec2i{
		X: axis(p0.X, p1.X, p2.X, p3.X),
		Y: axis(p0.Y, p1.Y, p2.Y, p3.Y),
	}
}

// bezierPoints tessellates one cubic.
func (e Entity) bezierPoints() []geom.Vec2i {
	if len(e.Pts) < 4 {
		return append([]geom.Vec2i(nil), e.Pts...)
	}
	p0, p1, p2, p3 := e.Pts[0], e.Pts[1], e.Pts[2], e.Pts[3]
	// A cubic is spent at the whole subdivision count rather than per span:
	// there is only one span.
	n := clampSplineSegs(e.Segs) * 2
	out := make([]geom.Vec2i, n+1)
	axis := func(a, b, c, d int64, t float64) int64 {
		u := 1 - t
		v := u*u*u*float64(a) + 3*u*u*t*float64(b) +
			3*u*t*t*float64(c) + t*t*t*float64(d)
		return int64(math.Round(v))
	}
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		out[i] = geom.Vec2i{
			X: axis(p0.X, p1.X, p2.X, p3.X, t),
			Y: axis(p0.Y, p1.Y, p2.Y, p3.Y, t),
		}
	}
	// The ends are the controls that were placed (V-99).
	out[0], out[n] = p0, p3
	return out
}

// polygonPoints walks a regular n-gon from its placed vertex.
func (e Entity) polygonPoints() []geom.Vec2i {
	n := clampSides(e.Segs)
	d := e.A.Sub(e.C)
	r := d.Len()
	if r <= 0 {
		return []geom.Vec2i{e.A}
	}
	start := math.Atan2(float64(d.Y), float64(d.X))
	pts := make([]geom.Vec2i, n)
	for i := 0; i < n; i++ {
		a := start + 2*math.Pi*float64(i)/float64(n)
		pts[i] = geom.Vec2i{
			X: e.C.X + int64(math.Round(r*math.Cos(a))),
			Y: e.C.Y + int64(math.Round(r*math.Sin(a))),
		}
	}
	// The clicked corner is exactly the clicked corner (V-99).
	pts[0] = e.A
	return pts
}

// slotPoints walks a capsule: down one side, round the far cap, back up the
// other side, round the near cap.
//
// The caps are half-circles centred on A and B, so the outline is what a
// router bit of that width would actually leave — which is the shape a slot
// is for.
func (e Entity) slotPoints() []geom.Vec2i {
	d := e.B.Sub(e.A)
	l := d.Len()
	if l <= 0 || e.W <= 0 {
		return []geom.Vec2i{e.A}
	}
	// The axis and its perpendicular, as unit vectors.
	ux, uy := float64(d.X)/l, float64(d.Y)/l
	w := float64(e.W)
	axis := math.Atan2(uy, ux)

	caps := e.Segs
	if caps < 2 {
		caps = 2
	}
	pts := make([]geom.Vec2i, 0, 2*caps+2)
	// The far cap sweeps from one side of B round to the other, and the near
	// cap does the same about A half a turn later.
	arcAbout := func(c geom.Vec2i, from float64) {
		for i := 0; i <= caps; i++ {
			a := from + math.Pi*float64(i)/float64(caps)
			pts = append(pts, geom.Vec2i{
				X: c.X + int64(math.Round(w*math.Cos(a))),
				Y: c.Y + int64(math.Round(w*math.Sin(a))),
			})
		}
	}
	arcAbout(e.B, axis-math.Pi/2)
	arcAbout(e.A, axis+math.Pi/2)
	return pts
}

// ellipsePoints tessellates a closed oval, rotated so its major axis runs from
// the centre to A.
func (e Entity) ellipsePoints() []geom.Vec2i {
	major := e.A.Sub(e.C).Len()
	if major <= 0 {
		return []geom.Vec2i{e.A}
	}
	minor := float64(e.W)
	rot := math.Atan2(float64(e.A.Y-e.C.Y), float64(e.A.X-e.C.X))
	cosR, sinR := math.Cos(rot), math.Sin(rot)

	n := clampSegs(e.Segs)
	pts := make([]geom.Vec2i, n)
	for i := 0; i < n; i++ {
		t := 2 * math.Pi * float64(i) / float64(n)
		// A point on the axis-aligned ellipse, then turned into place.
		x, y := major*math.Cos(t), minor*math.Sin(t)
		pts[i] = geom.Vec2i{
			X: e.C.X + int64(math.Round(x*cosR-y*sinR)),
			Y: e.C.Y + int64(math.Round(x*sinR+y*cosR)),
		}
	}
	// t=0 is the major-axis endpoint, which the user placed: exact.
	pts[0] = e.A
	return pts
}

// Closed reports whether the entity's points form a loop.
func (e Entity) Closed() bool {
	switch e.Kind {
	case EntLine, EntPoint, EntArc, EntBezier:
		return false
	case EntSpline:
		return e.R == 1
	default:
		return true
	}
}

// Equal compares two entities field for field.
//
// It exists because Pts made Entity uncomparable with ==, and the modify tools
// of Sketch_func.md §5 need to ask "did this change" about whole entities. A
// method the compiler cannot silently accept a wrong answer from beats an ==
// that stops compiling one day and gets replaced by something laxer.
func (e Entity) Equal(o Entity) bool {
	if e.Kind != o.Kind || e.A != o.A || e.B != o.B || e.C != o.C || e.D != o.D ||
		e.R != o.R || e.W != o.W || e.Segs != o.Segs ||
		e.Construction != o.Construction || len(e.Pts) != len(o.Pts) {
		return false
	}
	for i := range e.Pts {
		if e.Pts[i] != o.Pts[i] {
			return false
		}
	}
	return true
}

// AppendSegments expands the entity into segments, attributing each to the
// given entity index so provenance survives into the region engine.
func (e Entity) AppendSegments(dst []sketch2d.Seg, entity int) []sketch2d.Seg {
	pts := e.Points()
	if len(pts) < 2 {
		return dst
	}
	n := len(pts)
	if !e.Closed() {
		n--
	}
	for i := 0; i < n; i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		if a == b {
			continue
		}
		dst = append(dst, sketch2d.Seg{A: a, B: b, Src: sketch2d.Source{Entity: entity, Seg: i}})
	}
	return dst
}

// Bounds returns the entity's bounding box in subunits.
func (e Entity) Bounds() (min, max geom.Vec2i) {
	pts := e.Points()
	if len(pts) == 0 {
		return
	}
	min, max = pts[0], pts[0]
	for _, p := range pts[1:] {
		if p.X < min.X {
			min.X = p.X
		}
		if p.Y < min.Y {
			min.Y = p.Y
		}
		if p.X > max.X {
			max.X = p.X
		}
		if p.Y > max.Y {
			max.Y = p.Y
		}
	}
	return
}

// Translate moves the whole entity.
func (e Entity) Translate(d geom.Vec2i) Entity {
	e.A = e.A.Add(d)
	e.B = e.B.Add(d)
	e.C = e.C.Add(d)
	e.D = e.D.Add(d)
	if len(e.Pts) > 0 {
		// A copy: entities are values, and translating one must not move the
		// original's points out from under it.
		pts := make([]geom.Vec2i, len(e.Pts))
		for i, p := range e.Pts {
			pts[i] = p.Add(d)
		}
		e.Pts = pts
	}
	return e
}

// Segments expands every entity of a sketch, in order.
//
// Construction entities are skipped: they are guides, and a guide that closed
// a region or rang as an open end would be doing the one thing it exists not
// to do. They are still drawn, still snapped to and still selectable — this is
// the only place they are absent (Sketch_func.md §3).
func (s *Sketch) Segments() []sketch2d.Seg {
	var out []sketch2d.Seg
	for i := range s.Entities {
		if s.Entities[i].Construction {
			continue
		}
		out = s.Entities[i].AppendSegments(out, i)
	}
	return out
}

// Arrangement returns the sketch's regions and open ends, rebuilding the cache
// only when the entities have changed.
//
// The engine is fast enough to run on every edit (SPEC-GEOMETRY §4), but the
// UI asks for the arrangement several times a frame — to fill regions, to draw
// open ends, to label the card — so caching it keeps that to one build.
func (s *Sketch) Arrangement() sketch2d.Arrangement {
	if s.arrCached && s.arrStamp == s.stamp {
		return s.arrangement
	}
	s.arrangement = sketch2d.Build(s.Segments())
	s.arrCached = true
	s.arrStamp = s.stamp
	return s.arrangement
}

// Touch marks the sketch's derived data stale. Every mutation goes through a
// command, and every such command calls this.
func (s *Sketch) Touch() { s.stamp++ }

// EntityCount, RegionCount and OpenEndCount are what the sketch card reports
// (SPEC-UX §8.1).
func (s *Sketch) EntityCount() int  { return len(s.Entities) }
func (s *Sketch) RegionCount() int  { return len(s.Arrangement().Regions) }
func (s *Sketch) OpenEndCount() int { return len(s.Arrangement().OpenEnds) }

// Frame returns the sketch's plane frame in world space.
func (s *Sketch) Frame() geom.Frame {
	if s.OnFace {
		return s.FrameSnap
	}
	return geom.PlaneFrame(s.Plane)
}

// Where names the sketch's plane for toasts and the tree.
func (s *Sketch) Where() string {
	if s.OnFace {
		return "a face"
	}
	return "the " + s.Plane.String() + " plane"
}

// Summary is the one-line description the sketch card shows.
func (s *Sketch) Summary() string {
	a := s.Arrangement()
	parts := fmt.Sprintf("%s · %s",
		plural(len(s.Entities), "entity", "entities"),
		plural(len(a.Regions), "region", "regions"))
	if n := len(a.OpenEnds); n > 0 {
		parts += fmt.Sprintf(" · %s", plural(n, "open end", "open ends"))
	}
	return parts
}

// ---------------------------------------------------------------------------
// Commands.

// AddEntity appends a drawn entity to a sketch.
type AddEntity struct {
	Sketch uint32
	Entity Entity

	index int
}

func (c *AddEntity) Name() string { return "Draw " + c.Entity.Kind.String() }

func (c *AddEntity) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if c.Entity.Degenerate() {
		return fmt.Errorf("that %s has no size", c.Entity.Kind)
	}
	c.index = len(s.Entities)
	s.Entities = append(s.Entities, c.Entity)
	s.Touch()
	return nil
}

func (c *AddEntity) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil || c.index >= len(s.Entities) {
		return
	}
	s.Entities = append(s.Entities[:c.index], s.Entities[c.index+1:]...)
	s.Touch()
}

func (c *AddEntity) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// DeleteEntities removes entities by index, restoring them in place on undo.
type DeleteEntities struct {
	Sketch  uint32
	Indices []int

	removed []Entity
	at      []int
}

func (c *DeleteEntities) Name() string {
	return "Delete " + plural(len(c.Indices), "entity", "entities")
}

func (c *DeleteEntities) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if len(c.Indices) == 0 {
		return fmt.Errorf("nothing selected to delete")
	}
	drop := map[int]bool{}
	for _, i := range c.Indices {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
		drop[i] = true
	}

	c.removed = c.removed[:0]
	c.at = c.at[:0]
	kept := make([]Entity, 0, len(s.Entities)-len(drop))
	for i, e := range s.Entities {
		if drop[i] {
			c.removed = append(c.removed, e)
			c.at = append(c.at, i)
			continue
		}
		kept = append(kept, e)
	}
	s.Entities = kept
	s.Touch()
	return nil
}

func (c *DeleteEntities) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	// Reinsert lowest index first so each lands back where it was.
	for k := range c.removed {
		i := c.at[k]
		if i > len(s.Entities) {
			i = len(s.Entities)
		}
		s.Entities = append(s.Entities, Entity{})
		copy(s.Entities[i+1:], s.Entities[i:])
		s.Entities[i] = c.removed[k]
	}
	s.Touch()
}

func (c *DeleteEntities) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// SetCircleSegs changes an n-gon's segment count, which the sketch card offers
// while a circle is selected (SPEC-UX §8.3).
type SetCircleSegs struct {
	Sketch uint32
	Index  int
	Segs   int

	prev int
}

func (c *SetCircleSegs) Name() string { return "Change circle segments" }

func (c *SetCircleSegs) Do(doc *Document) error {
	e, err := entityAt(doc, c.Sketch, c.Index)
	if err != nil {
		return err
	}
	if e.Kind != EntCircle {
		return fmt.Errorf("that entity is not a circle")
	}
	c.prev = e.Segs
	e.Segs = clampSegs(c.Segs)
	doc.SketchByID(c.Sketch).Touch()
	return nil
}

func (c *SetCircleSegs) Undo(doc *Document) {
	if e, err := entityAt(doc, c.Sketch, c.Index); err == nil {
		e.Segs = c.prev
		doc.SketchByID(c.Sketch).Touch()
	}
}

func (c *SetCircleSegs) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// MoveEntities shifts entities by a snapped delta, which is what dragging in
// the Select tool commits.
type MoveEntities struct {
	Sketch  uint32
	Indices []int
	Delta   geom.Vec2i
}

func (c *MoveEntities) Name() string {
	return "Move " + plural(len(c.Indices), "entity", "entities")
}

func (c *MoveEntities) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	for _, i := range c.Indices {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
	}
	for _, i := range c.Indices {
		s.Entities[i] = s.Entities[i].Translate(c.Delta)
	}
	s.Touch()
	return nil
}

func (c *MoveEntities) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	for _, i := range c.Indices {
		if i >= 0 && i < len(s.Entities) {
			s.Entities[i] = s.Entities[i].Translate(c.Delta.Neg())
		}
	}
	s.Touch()
}

func (c *MoveEntities) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// SetConstruction flips entities between real geometry and guides.
type SetConstruction struct {
	Sketch  uint32
	Indices []int
	On      bool

	prev []bool
}

func (c *SetConstruction) Name() string {
	verb := "Convert to construction"
	if !c.On {
		verb = "Convert to geometry"
	}
	return verb
}

func (c *SetConstruction) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if len(c.Indices) == 0 {
		return fmt.Errorf("nothing selected to convert")
	}
	// Validate every index before touching one, so a bad list changes nothing.
	for _, i := range c.Indices {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
	}
	c.prev = c.prev[:0]
	for _, i := range c.Indices {
		c.prev = append(c.prev, s.Entities[i].Construction)
		s.Entities[i].Construction = c.On
	}
	s.Touch()
	return nil
}

func (c *SetConstruction) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	for k, i := range c.Indices {
		if i >= 0 && i < len(s.Entities) && k < len(c.prev) {
			s.Entities[i].Construction = c.prev[k]
		}
	}
	s.Touch()
}

func (c *SetConstruction) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

// ReplaceEntities removes and adds entities in one atomic, undoable step.
//
// It is the shape of every modify tool: fillet trims two lines and adds an
// arc, mirror adds copies, offset replaces a chain with a parallel one. Doing
// each as a remove-then-add pair would put two entries in the history for one
// gesture and leave a half-applied sketch if the second failed
// (Sketch_func.md §3).
type ReplaceEntities struct {
	Sketch uint32
	// Remove indexes the entities to drop, in any order.
	Remove []int
	Add    []Entity
	// Label is the undo name: what the user asked for, not what it did.
	Label string

	removed []Entity
	at      []int
	added   int
}

func (c *ReplaceEntities) Name() string {
	if c.Label != "" {
		return c.Label
	}
	return "Edit sketch"
}

func (c *ReplaceEntities) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	if len(c.Remove) == 0 && len(c.Add) == 0 {
		return fmt.Errorf("that would change nothing")
	}
	// Everything that can fail is checked before anything moves.
	drop := map[int]bool{}
	for _, i := range c.Remove {
		if i < 0 || i >= len(s.Entities) {
			return fmt.Errorf("no entity %d in that sketch", i)
		}
		drop[i] = true
	}
	for _, e := range c.Add {
		if e.Degenerate() {
			return fmt.Errorf("that %s has no size", e.Kind)
		}
	}

	// Nothing below can fail.
	c.removed = c.removed[:0]
	c.at = c.at[:0]
	kept := make([]Entity, 0, len(s.Entities)-len(drop)+len(c.Add))
	for i, e := range s.Entities {
		if drop[i] {
			c.removed = append(c.removed, e)
			c.at = append(c.at, i)
			continue
		}
		kept = append(kept, e)
	}
	c.added = len(c.Add)
	s.Entities = append(kept, c.Add...)
	s.Touch()
	return nil
}

func (c *ReplaceEntities) Undo(doc *Document) {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return
	}
	// The additions went on the end, so they come off the end.
	if c.added > 0 && len(s.Entities) >= c.added {
		s.Entities = s.Entities[:len(s.Entities)-c.added]
	}
	// Then the removals go back where they were, lowest index first, which is
	// the order they were collected in.
	for k := range c.removed {
		i := c.at[k]
		if i > len(s.Entities) {
			i = len(s.Entities)
		}
		s.Entities = append(s.Entities, Entity{})
		copy(s.Entities[i+1:], s.Entities[i:])
		s.Entities[i] = c.removed[k]
	}
	s.Touch()
}

func (c *ReplaceEntities) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
}

func entityAt(doc *Document, sketch uint32, index int) (*Entity, error) {
	s := doc.SketchByID(sketch)
	if s == nil {
		return nil, fmt.Errorf("no sketch %d", sketch)
	}
	if index < 0 || index >= len(s.Entities) {
		return nil, fmt.Errorf("no entity %d in that sketch", index)
	}
	return &s.Entities[index], nil
}
