package app

import (
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/csg"
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
	// The hull is a slab with a raised block, and the two are properly unioned
	// rather than merely both present.
	//
	// Merging them into one mesh looks identical and is not the same thing: they
	// meet face to face, so the shared plane carries two coincident surfaces and
	// the shells only touch rather than join. Every check passes — each shell is
	// closed and manifold on its own — and Manifold then declines to union
	// anything onto it, silently. Half the tests in this repository start from
	// this scene, so it had better be a solid.
	a.addTestBody(unionOrPanic(
		mesh.Box(geom.Vec3{X: -6, Y: -2, Z: -3}, geom.Vec3{X: 6, Y: 2, Z: 3}, 1),
		mesh.Box(geom.Vec3{X: -2, Y: 2, Z: -2}, geom.Vec3{X: 3, Y: 5, Z: 2}, 1),
	), "Hull")

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

// unionOrPanic joins two primitives at load time. The scene is fixed and known
// good, so a failure here is a broken build rather than something a user did.
func unionOrPanic(a, b *mesh.Mesh) *mesh.Mesh {
	res, err := csg.Boolean(csg.Union, a, b)
	if err != nil || res.Mesh == nil {
		panic("test scene: the hull would not union: " + errText(err))
	}
	return res.Mesh
}

func errText(err error) string {
	if err == nil {
		return "no mesh came back"
	}
	return err.Error()
}

// addTestBody runs an AddBody through the bus so ids, names and colours are
// assigned by exactly the same code the real tools will use.
func (a *App) addTestBody(m *mesh.Mesh, name string) {
	cmd := &model.AddBody{Mesh: m, Label: name}
	if err := a.Bus.Run(cmd); err != nil {
		panic("test scene: " + err.Error())
	}
}
