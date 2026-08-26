package geom

import "math"

// Grid snapping. SPEC-GEOMETRY §1.2 states the snap-on-author policy: every
// value a tool produces is snapped before it enters the document, so user
// geometry always lives on the lattice. Only boolean intersections, free
// rotation and draft miters may land off-grid, and that is correct rather than
// drift.

// SnapStep names the snap granularity a tool is currently using.
type SnapStep int

const (
	// SnapGrid is the default 1 u sketch/model grid.
	SnapGrid SnapStep = iota
	// SnapFine is the Ctrl-held quarter-unit step.
	SnapFine
	// SnapNone is the Alt-held free mode: no snapping at all.
	SnapNone
)

// Subunits returns the step size in subunits, or 0 for SnapNone.
func (s SnapStep) Subunits() int64 {
	switch s {
	case SnapGrid:
		return SubunitsPerUnit
	case SnapFine:
		return SubunitsFine
	default:
		return 0
	}
}

// SnapToSubunits rounds a world-unit value to the nearest subunit — the
// finest lattice any authored coordinate may occupy.
func SnapToSubunits(u float64) float64 {
	return math.RoundToEven(u*Unit) / Unit
}

// SnapVec3ToSubunits rounds every component to the subunit lattice.
func SnapVec3ToSubunits(v Vec3) Vec3 {
	return Vec3{SnapToSubunits(v.X), SnapToSubunits(v.Y), SnapToSubunits(v.Z)}
}

// SnapUnits rounds a world-unit value to the given step (in world units).
// A step of zero passes the value through untouched.
func SnapUnits(u, step float64) float64 {
	if step <= 0 {
		return u
	}
	return math.Round(u/step) * step
}

// SnapWithStep rounds a world-unit value according to a SnapStep. SnapNone
// still lands on the subunit lattice, because the document never stores
// coordinates finer than that from a tool.
func SnapWithStep(u float64, s SnapStep) float64 {
	sub := s.Subunits()
	if sub == 0 {
		return SnapToSubunits(u)
	}
	return float64(SnapSubunits(int64(math.Round(u*Unit)), sub)) / Unit
}

// SnapVec3 snaps every component per SnapWithStep.
func SnapVec3(v Vec3, s SnapStep) Vec3 {
	return Vec3{SnapWithStep(v.X, s), SnapWithStep(v.Y, s), SnapWithStep(v.Z, s)}
}

// SnapSubunits rounds a subunit coordinate to the nearest multiple of step
// (also in subunits), half away from zero. step <= 0 passes through.
func SnapSubunits(v, step int64) int64 {
	if step <= 0 {
		return v
	}
	if v >= 0 {
		return ((v + step/2) / step) * step
	}
	return -(((-v + step/2) / step) * step)
}

// SnapVec2i snaps a sketch point to the grid step given in subunits.
func SnapVec2i(p Vec2i, step int64) Vec2i {
	return Vec2i{SnapSubunits(p.X, step), SnapSubunits(p.Y, step)}
}

// SnapVec2iStep snaps a sketch point per a SnapStep. SnapNone leaves the point
// where it is, since sketch coordinates are already integral subunits.
func SnapVec2iStep(p Vec2i, s SnapStep) Vec2i {
	return SnapVec2i(p, s.Subunits())
}

// ToSubunits converts world units to sketch subunits, rounding to nearest and
// clamping into the legal sketch range (SPEC-GEOMETRY §1.1).
func ToSubunits(u float64) int64 {
	return ClampSketch(int64(math.Round(u * Unit)))
}

// ToUnits converts subunits back to world units. Exact for all legal inputs.
func ToUnits(s int64) float64 { return float64(s) / Unit }

// Vec2iFromUnits converts a world-unit 2D point into clamped sketch space.
func Vec2iFromUnits(v Vec2) Vec2i {
	return Vec2i{ToSubunits(v.X), ToSubunits(v.Y)}
}

// ClampSketch bounds a sketch coordinate to ±SketchClamp so that exact
// predicates and shoelace sums stay well inside int64.
func ClampSketch(v int64) int64 {
	if v > SketchClamp {
		return SketchClamp
	}
	if v < -SketchClamp {
		return -SketchClamp
	}
	return v
}

// InSketchRange reports whether a point is inside the legal sketch range; the
// UI uses this to warn instead of silently clamping.
func InSketchRange(p Vec2i) bool {
	return p.X >= -SketchClamp && p.X <= SketchClamp &&
		p.Y >= -SketchClamp && p.Y <= SketchClamp
}

// ClampSketchVec clamps both components.
func ClampSketchVec(p Vec2i) Vec2i {
	return Vec2i{ClampSketch(p.X), ClampSketch(p.Y)}
}
