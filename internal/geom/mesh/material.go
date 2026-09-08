package mesh

import (
	"image"
	"image/color"
	"math"
)

// Material maps use the paint's persistent texel coordinates. Images are
// immutable; assigning a map replaces the material, preserving undo and copies.
type Material struct {
	Maps map[string]*MaterialMap `json:"maps"`
}
type MaterialMap struct {
	Bounds image.Rectangle `json:"bounds"`
	Image  *image.RGBA     `json:"-"`
}

var MaterialChannels = []string{"base_color", "specular", "ao", "height", "roughness", "normal"}

// MaterialBounds is shared by image import and authoring export. Excluding
// allocation margins makes an exported image fit the same face on reimport.
func (p *FacePaint) MaterialBounds(m *Mesh, fi int) image.Rectangle {
	if p == nil || m == nil || fi < 0 || fi >= len(m.Faces) || len(m.Faces[fi].Outer()) < 3 {
		return image.Rectangle{}
	}
	loX, loY, hiX, hiY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, vi := range m.Faces[fi].Outer() {
		uv := p.UV(m.Verts[vi])
		loX = math.Min(loX, uv.X)
		loY = math.Min(loY, uv.Y)
		hiX = math.Max(hiX, uv.X)
		hiY = math.Max(hiY, uv.Y)
	}
	return image.Rect(int(math.Floor(loX+1e-8)), int(math.Floor(loY+1e-8)), int(math.Ceil(hiX-1e-8)), int(math.Ceil(hiY-1e-8)))
}

func ValidMaterialChannel(kind string) bool {
	for _, k := range MaterialChannels {
		if kind == k {
			return true
		}
	}
	return false
}
func MaterialDefault(kind string) color.RGBA {
	switch kind {
	case "normal":
		return color.RGBA{128, 128, 255, 255}
	case "roughness":
		return color.RGBA{230, 230, 230, 255}
	case "height":
		return color.RGBA{128, 128, 128, 255}
	default:
		return color.RGBA{255, 255, 255, 255}
	}
}
func (m *Material) Sample(kind string, x, y float64) color.RGBA {
	if m != nil {
		if p := m.Maps[kind]; p != nil && p.Image != nil && !p.Bounds.Empty() {
			b := p.Image.Bounds()
			u := (x - float64(p.Bounds.Min.X)) / float64(p.Bounds.Dx())
			v := (y - float64(p.Bounds.Min.Y)) / float64(p.Bounds.Dy())
			ix := max(b.Min.X, min(b.Max.X-1, b.Min.X+int(u*float64(b.Dx()))))
			iy := max(b.Min.Y, min(b.Max.Y-1, b.Min.Y+int(v*float64(b.Dy()))))
			return p.Image.RGBAAt(ix, iy)
		}
	}
	return MaterialDefault(kind)
}

// SurfaceNormal combines the normal image and height-derived surface relief.
func (m *Material) SurfaceNormal(u, v float64) color.RGBA {
	n := m.Sample("normal", u, v)
	if m != nil && m.Maps["height"] != nil {
		dx := float64(m.Sample("height", u+1, v).R) - float64(m.Sample("height", u-1, v).R)
		dy := float64(m.Sample("height", u, v+1).R) - float64(m.Sample("height", u, v-1).R)
		n.R = uint8(max(0, min(255, int(n.R)-int(dx*.5))))
		n.G = uint8(max(0, min(255, int(n.G)-int(dy*.5))))
	}
	n.A = 255
	return n
}

// Raster aligns a material channel with the base image, including its margin.
func (p *FacePaint) MaterialRaster(kind string) *image.RGBA {
	out := image.NewRGBA(p.Img.Bounds())
	for y := out.Rect.Min.Y; y < out.Rect.Max.Y; y++ {
		for x := out.Rect.Min.X; x < out.Rect.Max.X; x++ {
			out.SetRGBA(x, y, p.Material.Sample(kind, float64(x+p.Off.X)+.5, float64(y+p.Off.Y)+.5))
		}
	}
	return out
}
