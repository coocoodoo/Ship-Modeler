package paint

import (
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
	"testing"
)

func TestMoveTextureWrapChannelsAndFaceIsolation(t *testing.T) {
	bus, b, uid, fi := painted(t)
	p, e := Allocate(b.Mesh, fi, 4)
	if e != nil {
		t.Fatal(e)
	}
	b.Mesh.Faces[fi].Paint = p
	other := (fi + 1) % len(b.Mesh.Faces)
	b.Mesh.Faces[other].Paint = p
	bounds := p.MaterialBounds(b.Mesh, fi)
	p.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}
	for _, k := range []string{"roughness", "specular", "ao", "height", "normal"} {
		p.Material.Maps[k] = &mesh.MaterialMap{Bounds: p.TexelBounds(), Image: image.NewRGBA(p.Img.Bounds())}
	}
	for y := 0; y < p.Img.Rect.Dy(); y++ {
		for x := 0; x < p.Img.Rect.Dx(); x++ {
			c := color.RGBA{uint8(x), uint8(y), 71, 255}
			p.Img.SetRGBA(x, y, c)
			for _, m := range p.Material.Maps {
				m.Image.SetRGBA(x, y, c)
			}
		}
	}
	dx, dy := -1, 2
	c := &MoveTexture{Body: b.ID, Face: uid, DX: dx, DY: dy}
	if e = bus.Run(c); e != nil {
		t.Fatal(e)
	}
	q := b.Mesh.Faces[fi].Paint
	if q == p || b.Mesh.Faces[other].Paint != p {
		t.Fatal("shared face edited")
	}
	if q.Frame != p.Frame || q.Off != p.Off || q.Res != p.Res || q.Texel != p.Texel {
		t.Fatal("paint grid changed")
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sx := bounds.Min.X + ((x-dx-bounds.Min.X)%bounds.Dx()+bounds.Dx())%bounds.Dx()
			sy := bounds.Min.Y + ((y-dy-bounds.Min.Y)%bounds.Dy()+bounds.Dy())%bounds.Dy()
			want := p.Img.RGBAAt(sx-p.Off.X, sy-p.Off.Y)
			if q.Img.RGBAAt(x-q.Off.X, y-q.Off.Y) != want {
				t.Fatal("base wrap")
			}
			for k := range q.Material.Maps {
				if q.Material.Sample(k, float64(x)+.5, float64(y)+.5) != want {
					t.Fatal("PBR misaligned", k)
				}
			}
		}
	}
	bus.Undo()
	if b.Mesh.Faces[fi].Paint != p {
		t.Fatal("undo")
	}
	bus.Redo()
	if b.Mesh.Faces[fi].Paint != q {
		t.Fatal("redo")
	}
}
