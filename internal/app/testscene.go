package app

import (
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// LoadTestScene fills the document with a small blocky shape plus a faceted
// cylinder: enough geometry to exercise flat shading, crease edges, face and
// vertex picking, and the golden shots.
//
// M0 has no sketch or extrude tools yet, so this stands in for a document.
// M9 replaces it with the embedded sample ship, built by an op script.
func (a *App) LoadTestScene() {
	hull := mesh.Box(
		geom.Vec3{X: -6, Y: -2, Z: -3},
		geom.Vec3{X: 6, Y: 2, Z: 3}, 1)
	mesh.Merge(hull, mesh.Box(
		geom.Vec3{X: -2, Y: 2, Z: -2},
		geom.Vec3{X: 3, Y: 5, Z: 2}, 1))
	a.AddBody(hull)

	// A 16-segment prism standing on the Top plane, offset along X so it reads
	// separately from the hull.
	frame := geom.PlaneFrame(geom.PlaneTop)
	frame.O = geom.Vec3{X: -10, Y: -2, Z: 0}
	a.AddBody(mesh.NGonPrism(frame, 2.5, 16, 6, 2))

	// A rotated box, so the crease classifier and the pick pass see something
	// that is not axis aligned.
	wedge := mesh.Box(
		geom.Vec3{X: -1.5, Y: -1.5, Z: -1.5},
		geom.Vec3{X: 1.5, Y: 1.5, Z: 1.5}, 3)
	mesh.Transform(wedge, geom.Translate(geom.Vec3{X: 10, Y: 0, Z: 0}).
		Mul(geom.RotateY(math.Pi/4)).
		Mul(geom.RotateZ(math.Pi/6)))
	a.AddBody(wedge)
}
