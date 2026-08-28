package paint

import (
	"image"
	"image/color"
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// One model, one pixel size (V-140, the user's demand 2026-08-28: "I want
// them to be just 1 size, 1px... Not SCALE"). ResampleModel rebuilds every
// picture in the document at one density, as a single undoable step — the
// command behind the Res chips once anything is painted.

// mixedDoc is one body with three painted faces arranged to catch the traps:
// faces 0 and 1 SHARE one picture (boolean fragments do this), face 2 carries
// its own at a different density (a legacy mixed document).
func mixedDoc(t *testing.T) (*model.Bus, *model.Body) {
	t.Helper()
	bus := model.NewBus(model.NewDocument())
	cmd := &model.AddBody{Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 4, Y: 2, Z: 3}, 1), Label: "Crate"}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	b := cmd.AddedBody()
	m := b.Mesh

	p0, err := Allocate(m, 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	Set(p0, image.Point{X: 1, Y: 1}, color.RGBA{R: 200, A: 255})
	m.Faces[0].Paint = p0
	m.Faces[1].Paint = p0 // shared, like fragments of a cut face

	p2, err := Allocate(m, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	Set(p2, image.Point{X: 0, Y: 0}, color.RGBA{G: 200, A: 255})
	m.Faces[2].Paint = p2
	return bus, b
}

func TestResampleModelMakesEveryPictureOneDensity(t *testing.T) {
	bus, b := mixedDoc(t)
	m := b.Mesh
	sharedBefore := m.Faces[0].Paint

	if err := bus.Run(&ResampleModel{Res: 4}); err != nil {
		t.Fatal(err)
	}
	for _, fi := range []int{0, 1, 2} {
		p := m.Faces[fi].Paint
		if p == nil || math.Abs(p.Texel-0.25) > 1e-12 {
			t.Errorf("face %d is %v u/texel after the resample, want 0.25", fi, p.Texel)
		}
	}
	// Sharing survives: the fragments still read one picture, a new one.
	if m.Faces[0].Paint != m.Faces[1].Paint {
		t.Error("the resample split a shared picture into two")
	}
	if m.Faces[0].Paint == sharedBefore {
		t.Error("the shared picture was rescaled in place rather than rebuilt")
	}
	// The pixels stayed where they were in the world: face 0's red texel at
	// 8 px/u sat at world offset (1.5/8, 1.5/8); at 4 px/u that is inside
	// texel (0,0)... probe via world round trip instead of arithmetic.
	world := World(sharedBefore, image.Point{X: 1, Y: 1})
	if got := At(m.Faces[0].Paint, Texel(m.Faces[0].Paint, world)); got.R != 200 {
		t.Errorf("the red texel moved: %v at its old world position", got)
	}
}

func TestResampleModelUndoesToTheExactPointers(t *testing.T) {
	bus, b := mixedDoc(t)
	m := b.Mesh
	old0, old2 := m.Faces[0].Paint, m.Faces[2].Paint

	if err := bus.Run(&ResampleModel{Res: 4}); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if m.Faces[0].Paint != old0 || m.Faces[1].Paint != old0 || m.Faces[2].Paint != old2 {
		t.Error("undo did not restore the exact pictures, sharing included")
	}
	if _, ok := bus.Redo(); !ok {
		t.Fatal("nothing to redo")
	}
	if math.Abs(m.Faces[0].Paint.Texel-0.25) > 1e-12 || m.Faces[0].Paint != m.Faces[1].Paint {
		t.Error("redo did not reapply the resample with sharing intact")
	}
}

func TestResampleModelRefusesANoOp(t *testing.T) {
	bus, b := mixedDoc(t)
	if err := bus.Run(&ResampleModel{Res: 4}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Run(&ResampleModel{Res: 4}); err == nil {
		t.Error("resampling to the density everything already has went into the history")
	}
	_ = b
}

func TestResampleModelRefusesAnEmptyDocument(t *testing.T) {
	bus := model.NewBus(model.NewDocument())
	if err := bus.Run(&ResampleModel{Res: 8}); err == nil {
		t.Error("resampling a document with no paint went into the history")
	}
}
