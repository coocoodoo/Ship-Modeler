// Package sketch is sketch mode: the drawing tools, the snapping and inference
// that make them feel like they read your mind, and the session state machine
// that ties them together (SPEC-UX §8).
//
// It is raylib-free: everything here is decided in exact sketch coordinates and
// tested without a window.
package sketch

import (
	"math"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// Snap radii in screen pixels (SPEC-UX §8.4). An endpoint pulls hardest, then
// a midpoint; the grid is always available underneath both.
const (
	EndpointRadiusPx = 7.0
	MidpointRadiusPx = 5.0
	// InferenceDegrees is how close to horizontal or vertical a direction has
	// to be before the guide takes over.
	InferenceDegrees = 4.0
)

// SnapKind is what the cursor latched onto, which decides the glyph drawn at
// the snap point.
type SnapKind uint8

const (
	// SnapFree means nothing was snapped: Alt is held.
	SnapFree SnapKind = iota
	// SnapGrid is the lattice intersection, the always-available fallback.
	SnapGrid
	// SnapEndpoint latched onto an existing entity endpoint.
	SnapEndpoint
	// SnapMidpoint latched onto the middle of an existing segment.
	SnapMidpoint
)

func (k SnapKind) String() string {
	switch k {
	case SnapGrid:
		return "grid"
	case SnapEndpoint:
		return "endpoint"
	case SnapMidpoint:
		return "midpoint"
	default:
		return "free"
	}
}

// Inference is the axis a guide locked the direction to.
type Inference uint8

const (
	InferNone Inference = iota
	InferHorizontal
	InferVertical
)

// Snap is the resolved cursor position plus what it latched onto.
type Snap struct {
	Point geom.Vec2i
	Kind  SnapKind
	// Infer names the guide axis, and From is the point the guide runs from.
	// The UI draws a dashed line between them (SPEC-UX §8.4).
	Infer Inference
	From  geom.Vec2i
}

// HasGuide reports whether a dashed inference guide should be drawn.
func (s Snap) HasGuide() bool { return s.Infer != InferNone }

// Config carries the tolerances for one snap query.
type Config struct {
	// GridStep is the snap lattice in subunits; zero disables the grid.
	GridStep int64
	// SubunitsPerPixel converts the pixel radii above into sketch distance, so
	// snapping feels the same at every zoom level.
	SubunitsPerPixel float64
	// Suppressed is Alt held: no snapping at all, not even the grid.
	Suppressed bool
	// Reference is geometry to snap to that is not part of the sketch: the
	// boundary of the face a face-sketch sits on (SPEC-UX §10). Its corners and
	// edge midpoints are targets like any other, which is the whole reason for
	// projecting a face's outline onto the plane you are drawing on.
	Reference [][]geom.Vec2i
}

// DefaultConfig is the 1 u grid of SPEC-UX §8.4 at a given zoom.
func DefaultConfig(subunitsPerPixel float64) Config {
	return Config{GridStep: geom.SubunitsPerUnit, SubunitsPerPixel: subunitsPerPixel}
}

// Resolve snaps a raw cursor position.
//
// from, when non-nil, is the point a rubber band is being drawn from — the
// previous point of a line chain, or a rectangle's first corner — and is what
// the horizontal and vertical inference works against.
//
// The priority is the one SPEC-UX §8.4 fixes: an existing endpoint wins over a
// midpoint, which wins over the grid. Inference only applies once nothing
// stronger has claimed the cursor, because latching onto a real point is
// always more useful than staying on an axis.
func Resolve(cursor geom.Vec2i, ents []model.Entity, from *geom.Vec2i, cfg Config) Snap {
	if cfg.Suppressed {
		return Snap{Point: cursor, Kind: SnapFree}
	}

	endpointR := int64(EndpointRadiusPx * cfg.SubunitsPerPixel)
	midpointR := int64(MidpointRadiusPx * cfg.SubunitsPerPixel)

	// Drawn geometry and reference geometry compete on distance within a tier,
	// not on which list they came from: the nearest corner is the one you meant
	// whether you drew it or the face brought it.
	ep, epOK := nearestEndpoint(cursor, ents, endpointR)
	rp, rpOK := nearestPoint(cursor, referenceCorners(cfg.Reference), endpointR)
	if p, ok := nearer(ep, epOK, rp, rpOK, cursor); ok {
		return Snap{Point: p, Kind: SnapEndpoint}
	}
	mp, mpOK := nearestMidpoint(cursor, ents, midpointR)
	rm, rmOK := nearestPoint(cursor, referenceMidpoints(cfg.Reference), midpointR)
	if p, ok := nearer(mp, mpOK, rm, rmOK, cursor); ok {
		return Snap{Point: p, Kind: SnapMidpoint}
	}

	// Inference constrains the direction first, then the grid quantises along
	// it, so an inferred line still lands on the lattice.
	if from != nil {
		if inf := inferAxis(*from, cursor); inf != InferNone {
			p := *from
			switch inf {
			case InferHorizontal:
				p.X = snapTo(cursor.X, cfg.GridStep)
			case InferVertical:
				p.Y = snapTo(cursor.Y, cfg.GridStep)
			}
			return Snap{Point: p, Kind: SnapGrid, Infer: inf, From: *from}
		}
	}

	return Snap{
		Point: geom.Vec2i{X: snapTo(cursor.X, cfg.GridStep), Y: snapTo(cursor.Y, cfg.GridStep)},
		Kind:  SnapGrid,
	}
}

// nearer picks whichever of two candidates is closer to the cursor, with a
// deterministic tie-break.
func nearer(a geom.Vec2i, aOK bool, b geom.Vec2i, bOK bool, cursor geom.Vec2i) (geom.Vec2i, bool) {
	switch {
	case aOK && bOK:
		da, db := a.Sub(cursor).LenSq(), b.Sub(cursor).LenSq()
		if db < da || (db == da && lexLess(b, a)) {
			return b, true
		}
		return a, true
	case aOK:
		return a, true
	case bOK:
		return b, true
	}
	return geom.Vec2i{}, false
}

// nearestPoint finds the closest of a plain list of candidates.
func nearestPoint(cursor geom.Vec2i, pts []geom.Vec2i, radius int64) (geom.Vec2i, bool) {
	if radius <= 0 {
		return geom.Vec2i{}, false
	}
	r2 := radius * radius
	best, bestD := geom.Vec2i{}, int64(0)
	found := false
	for _, p := range pts {
		d := p.Sub(cursor).LenSq()
		if d > r2 {
			continue
		}
		if !found || d < bestD || (d == bestD && lexLess(p, best)) {
			best, bestD, found = p, d, true
		}
	}
	return best, found
}

// referenceCorners and referenceMidpoints expand the reference loops into the
// two kinds of target a snap can land on.
func referenceCorners(loops [][]geom.Vec2i) []geom.Vec2i {
	var out []geom.Vec2i
	for _, l := range loops {
		out = append(out, l...)
	}
	return out
}

func referenceMidpoints(loops [][]geom.Vec2i) []geom.Vec2i {
	var out []geom.Vec2i
	for _, l := range loops {
		for i := range l {
			a, b := l[i], l[(i+1)%len(l)]
			if a == b {
				continue
			}
			out = append(out, geom.Vec2i{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2})
		}
	}
	return out
}

func snapTo(v, step int64) int64 {
	if step <= 0 {
		return v
	}
	return geom.SnapSubunits(v, step)
}

// nearestEndpoint finds the closest entity endpoint within the radius.
func nearestEndpoint(cursor geom.Vec2i, ents []model.Entity, radius int64) (geom.Vec2i, bool) {
	if radius <= 0 {
		return geom.Vec2i{}, false
	}
	r2 := radius * radius
	best, bestD := geom.Vec2i{}, int64(0)
	found := false
	for i := range ents {
		for _, p := range ents[i].Points() {
			d := p.Sub(cursor).LenSq()
			if d > r2 {
				continue
			}
			if !found || d < bestD || (d == bestD && lexLess(p, best)) {
				best, bestD, found = p, d, true
			}
		}
	}
	return best, found
}

// nearestMidpoint finds the closest segment midpoint within the radius.
func nearestMidpoint(cursor geom.Vec2i, ents []model.Entity, radius int64) (geom.Vec2i, bool) {
	if radius <= 0 {
		return geom.Vec2i{}, false
	}
	r2 := radius * radius
	best, bestD := geom.Vec2i{}, int64(0)
	found := false
	for i := range ents {
		pts := ents[i].Points()
		n := len(pts)
		if !ents[i].Closed() {
			n--
		}
		for j := 0; j < n; j++ {
			a, b := pts[j], pts[(j+1)%len(pts)]
			// Midpoints of lattice points can land off the lattice; that is
			// correct, and the point is used as drawn.
			m := geom.Vec2i{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
			d := m.Sub(cursor).LenSq()
			if d > r2 {
				continue
			}
			if !found || d < bestD || (d == bestD && lexLess(m, best)) {
				best, bestD, found = m, d, true
			}
		}
	}
	return best, found
}

// inferAxis reports whether the direction from a to b is close enough to an
// axis for the guide to take over.
func inferAxis(a, b geom.Vec2i) Inference {
	d := b.Sub(a)
	if d.X == 0 && d.Y == 0 {
		return InferNone
	}
	ax, ay := absI(d.X), absI(d.Y)
	// tan(4°) as an exact rational avoids a trig call in the hot path.
	const tanNum, tanDen = 6993, 100000 // tan(4 degrees) to five places
	switch {
	case ay*tanDen <= ax*tanNum:
		return InferHorizontal
	case ax*tanDen <= ay*tanNum:
		return InferVertical
	}
	return InferNone
}

func absI(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func lexLess(a, b geom.Vec2i) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

// AngleDegrees is the direction from a to b, for the readout that floats near
// the cursor while drawing (SPEC-UX §8.3).
func AngleDegrees(a, b geom.Vec2i) float64 {
	d := b.Sub(a)
	if d.X == 0 && d.Y == 0 {
		return 0
	}
	deg := math.Atan2(float64(d.Y), float64(d.X)) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return deg
}

// LengthUnits is the distance between two sketch points in world units, for
// the same readout.
func LengthUnits(a, b geom.Vec2i) float64 {
	return b.Sub(a).Len() / geom.Unit
}
