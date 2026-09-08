package app

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/paint"
)

func TestInsertedLibraryPaintFollowsLiveMoveAndSavedProject(t *testing.T) {
	a := bareApp()
	a.library.dir = t.TempDir()
	source := &model.Body{ID: 1, Name: "Panel", Visible: true, FaceSeq: 7,
		Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 4, Z: 2}, 1)}
	for fi := range source.Mesh.Faces {
		p, err := paint.Allocate(source.Mesh, fi, 4)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < p.Img.Bounds().Dy(); y++ {
			for x := 0; x < p.Img.Bounds().Dx(); x++ {
				p.Img.SetRGBA(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 12), B: 120, A: 255})
			}
		}
		source.Mesh.Faces[fi].Paint = p
	}
	// A split face shares one painted image and mapping, as boolean results do.
	f := source.Mesh.Faces[0]
	q := f.Outer()
	source.Mesh.Faces[0].Loops = [][]int{{q[0], q[1], q[2]}}
	f.ID, f.Loops = mesh.MakeFaceUID(1, 99), [][]int{{q[0], q[2], q[3]}}
	source.Mesh.Faces = append(source.Mesh.Faces, f)
	source.Mesh.InvalidateCaches()
	part, err := io.SaveLibraryPart(a.library.dir, "Panel", "Modular", []*model.Body{source})
	if err != nil {
		t.Fatal(err)
	}
	a.refreshPartLibrary()
	if !a.insertLibraryPart(part.ID) {
		t.Fatal(a.library.err)
	}
	body := a.Doc().Bodies[0]
	check := func(got *model.Body, delta geom.Vec3) {
		t.Helper()
		for fi, face := range source.Mesh.Faces {
			p := got.Mesh.Faces[fi].Paint
			if !bytes.Equal(p.Img.Pix, face.Paint.Img.Pix) {
				t.Fatal("move modified the painted pixels")
			}
			for _, vi := range face.Outer() {
				want := face.Paint.UV(source.Mesh.Verts[vi])
				actual := p.UV(got.Mesh.Verts[vi])
				if math.Hypot(actual.X-want.X, actual.Y-want.Y) > 1e-8 {
					t.Fatalf("library face %d texture slipped: %v -> %v", fi, want, actual)
				}
			}
			texel := image.Pt(2, 3)
			if !paint.World(p, texel).NearEq(paint.World(face.Paint, texel).Add(delta), 1e-9) {
				t.Fatal("paint anchor did not travel with the body")
			}
		}
	}
	check(body, geom.Vec3{})
	var final geom.Vec3
	for _, delta := range []geom.Vec3{{X: 1}, {Y: 2}, {X: 4, Y: -3, Z: 7}} {
		a.transform.tool.SetDelta(delta)
		a.applyTransformLive()
		check(body, delta)
		final = delta
	}
	a.commitTransform()
	a.Undo()
	check(body, geom.Vec3{})
	a.Redo()
	check(body, final)
	path := filepath.Join(t.TempDir(), "moved.pxm")
	if err := io.SaveShip(path, a.Doc(), nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := io.LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	check(loaded.Doc.Bodies[0], final)
	if !a.insertLibraryPart(part.ID) {
		t.Fatal(a.library.err)
	}
	check(a.Doc().Bodies[1], geom.Vec3{})
	check(body, final)
}
