// Package geom holds the numeric foundation of the modeler: the two coordinate
// worlds (exact int64 sketch space, float64 model space), linear algebra, and
// the exact 2D predicates the sketch region engine is built on.
//
// This package is deliberately free of any rendering or platform dependency
// (PLAN §4): its tests run without a window.
package geom

// Tolerance table — SPEC-GEOMETRY §1.3. Nothing outside this file may define a
// geometric epsilon; every comparison that needs slack references one of these.
const (
	// SubunitsPerUnit is the sketch-space quantum: 256 subunits = 1 world unit.
	SubunitsPerUnit = 256

	// SubunitsFine is the "Ctrl held" fine snap step (¼ u) — SPEC-UX §9.2.
	SubunitsFine = 64

	// SketchClamp bounds sketch coordinates to ±16,384 u so cross products and
	// shoelace sums stay far inside int64 (SPEC-GEOMETRY §1.1).
	SketchClamp = 4194304

	// Unit is one world unit expressed in subunits, as a float64 for conversions.
	Unit = float64(SubunitsPerUnit)

	// WeldDist (½ subunit) — vertex welding after booleans and rounding.
	WeldDist = 1.0 / 512.0

	// PlanarDist (1 subunit) — face planarity check, SPEC-GEOMETRY §7.2.
	PlanarDist = 1.0 / 256.0

	// NormalEps — unit-vector and parallelism comparisons.
	NormalEps = 1e-9

	// VolumeRelTol — relative tolerance for volume assertions in tests.
	VolumeRelTol = 1e-6
)
