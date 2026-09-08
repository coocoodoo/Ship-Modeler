package paint

import (
	"image"
	"image/color"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"testing"
)

func TestMaterialMapHistoryAndResample(t *testing.T) {
	bus, b, uid, fi := painted(t)
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{20, 20, 20, 255})
	img.SetRGBA(1, 0, color.RGBA{240, 240, 240, 255})
	c := &SetMaterialMap{Body: b.ID, Face: uid, Kind: "roughness", Image: img, Res: 8}
	if err := bus.Run(c); err != nil {
		t.Fatal(err)
	}
	p := b.Mesh.Faces[fi].Paint
	if p.Material.Sample("roughness", 1, 1).R != 20 || p.Material.Sample("roughness", 63, 1).R != 240 {
		t.Fatal("image did not fit the face")
	}
	img.SetRGBA(0, 0, color.RGBA{})
	if p.Material.Sample("roughness", 1, 1).R != 20 {
		t.Fatal("import retained mutable source")
	}
	copy := model.SnapshotBody(b)
	x := geom.Translate(geom.Vec3{X: 5, Y: -3, Z: 2}).Mul(geom.RotateY(.7))
	beforeWorld := p.Frame.ToWorld(geom.Vec2{X: p.Texel, Y: p.Texel})
	mesh.Transform(copy.Mesh, x)
	qcopy := copy.Mesh.Faces[fi].Paint
	uv := qcopy.UV(x.TransformPoint(beforeWorld))
	if qcopy.Material.Sample("roughness", uv.X, uv.Y).R != 20 || p.Frame == qcopy.Frame {
		t.Fatal("material failed to follow transformed copy")
	}
	bus.Undo()
	if b.Mesh.Faces[fi].Paint != nil {
		t.Fatal("undo retained material")
	}
	bus.Redo()
	if b.Mesh.Faces[fi].Paint != p {
		t.Fatal("redo changed material")
	}
	q, err := Resample(b.Mesh, fi, p, 16)
	if err != nil {
		t.Fatal(err)
	}
	if q.Material.Sample("roughness", 1, 1).R != 20 || q.Material.Sample("roughness", 127, 1).R != 240 {
		t.Fatal("resampling moved material")
	}
	b.Mesh.Faces[(fi+1)%len(b.Mesh.Faces)].Paint = p
	clear := &SetMaterialMap{Body: b.ID, Face: uid, Kind: "roughness"}
	if err := bus.Run(clear); err != nil {
		t.Fatal(err)
	}
	if b.Mesh.Faces[(fi+1)%len(b.Mesh.Faces)].Paint != b.Mesh.Faces[fi].Paint {
		t.Fatal("shared fragments split")
	}
	if p.Material.Maps["roughness"] == nil {
		t.Fatal("replacement mutated old material")
	}
	bus.Undo()
	if b.Mesh.Faces[fi].Paint != p {
		t.Fatal("remove undo lost material")
	}
	if err := bus.Run(&SetMaterialMap{Body: b.ID, Face: uid, Kind: "invalid"}); err == nil {
		t.Fatal("invalid channel accepted")
	}
}
func TestMaterialBaseRemainsPaintable(t *testing.T) {
	bus, b, uid, fi := painted(t)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, red)
	if err := bus.Run(&SetMaterialMap{Body: b.ID, Face: uid, Kind: "base_color", Image: img, Res: 8}); err != nil {
		t.Fatal(err)
	}
	p := b.Mesh.Faces[fi].Paint
	if At(p, image.Pt(4, 4)) != red {
		t.Fatal("base import missing")
	}
	if err := bus.Run(stroke(b.ID, uid, 8, ToolPencil, blue, image.Pt(4, 4))); err != nil {
		t.Fatal(err)
	}
	if At(p, image.Pt(4, 4)) != blue {
		t.Fatal("base import blocks painting")
	}
	bus.Undo()
	if At(p, image.Pt(4, 4)) != red {
		t.Fatal("paint undo lost imported base")
	}
}
