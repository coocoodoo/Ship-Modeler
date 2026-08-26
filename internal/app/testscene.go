package app

import (
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// LoadTestScene fills the document with a small blocky shape, a faceted
// cylinder and a rotated box: enough geometry to exercise flat shading, crease
// edges, face and vertex picking, the tree panel and the golden shots.
//
// M2 and M3 replace it with real sketching and extrude; M9 replaces it with the
// embedded sample ship, built by an op script.
func (a *App) LoadTestScene() {
	hull := mesh.Box(
		geom.Vec3{X: -6, Y: -2, Z: -3},
		geom.Vec3{X: 6, Y: 2, Z: 3}, 1)
	mesh.Merge(hull, mesh.Box(
		geom.Vec3{X: -2, Y: 2, Z: -2},
		geom.Vec3{X: 3, Y: 5, Z: 2}, 1))
	a.addTestBody(hull, "Hull")

	// A 16-segment prism standing on the Top plane, offset along X so it reads
	// separately from the hull.
	frame := geom.PlaneFrame(geom.PlaneTop)
	frame.O = geom.Vec3{X: -10, Y: -2, Z: 0}
	a.addTestBody(mesh.NGonPrism(frame, 2.5, 16, 6, 2), "Engine pod")

	// A rotated box, so the crease classifier and the pick pass see something
	// that is not axis aligned.
	wedge := mesh.Box(
		geom.Vec3{X: -1.5, Y: -1.5, Z: -1.5},
		geom.Vec3{X: 1.5, Y: 1.5, Z: 1.5}, 3)
	mesh.Transform(wedge, geom.Translate(geom.Vec3{X: 10, Y: 0, Z: 0}).
		Mul(geom.RotateY(math.Pi/4)).
		Mul(geom.RotateZ(math.Pi/6)))
	a.addTestBody(wedge, "Wing pod")

	// The scene is the starting state, not something the user did, so it is not
	// undoable and does not mark the document dirty.
	a.Bus.Replace(a.Doc())
	a.Doc().DirtySinceSave = false
}

// addTestBody runs an AddBody through the bus so ids, names and colours are
// assigned by exactly the same code the real tools will use.
func (a *App) addTestBody(m *mesh.Mesh, name string) {
	cmd := &model.AddBody{Mesh: m, Label: name}
	if err := a.Bus.Run(cmd); err != nil {
		panic("test scene: " + err.Error())
	}
}
