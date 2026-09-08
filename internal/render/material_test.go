package render

import (
	"image"
	"image/color"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"testing"
)

func TestMaterialAtlasLeavesUnpaintedFacesEmpty(t *testing.T) {
	m := mesh.Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, 1)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	p := &mesh.FacePaint{Img: img, Texel: .25, Frame: m.FaceFrame(0), Material: &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}}
	m.Faces[0].Paint = p
	a := buildAtlas(m)
	if !a.hasMaterial || a.img.RGBAAt(0, 0).A != 0 || a.properties.RGBAAt(0, 0).A != 0 {
		t.Fatal("unpainted fallback inherits material")
	}
	slot := a.slots[p]
	if c := a.properties.RGBAAt(slot.Origin.X, slot.Origin.Y); c.R != 255 || c.G != 230 || c.A != 255 {
		t.Fatalf("bad default material: %v", c)
	}
}
