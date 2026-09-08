package mesh

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// Layer pixels and masks travel inside document.json in the compressed PXM.
// Img remains the flattened rendering/export surface for older consumers.
type PaintLayer struct {
	Name    string
	Visible bool
	Opacity float64
	Pixels  *image.RGBA
	Mask    *image.RGBA // white reveals, black protects; nil reveals everything
}
type layerJSON struct {
	Name         string
	Visible      bool
	Opacity      float64
	Pixels, Mask []byte
}

func pngBytes(p *image.RGBA) []byte {
	if p == nil {
		return nil
	}
	var b bytes.Buffer
	_ = png.Encode(&b, p)
	return b.Bytes()
}
func readLayerPNG(b []byte) (*image.RGBA, error) {
	if len(b) == 0 {
		return nil, nil
	}
	p, e := png.Decode(bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	r := image.NewRGBA(p.Bounds())
	draw.Draw(r, r.Bounds(), p, p.Bounds().Min, draw.Src)
	return r, nil
}
func (l PaintLayer) MarshalJSON() ([]byte, error) {
	return json.Marshal(layerJSON{l.Name, l.Visible, l.Opacity, pngBytes(l.Pixels), pngBytes(l.Mask)})
}
func (l *PaintLayer) UnmarshalJSON(b []byte) error {
	var v layerJSON
	if e := json.Unmarshal(b, &v); e != nil {
		return e
	}
	p, e := readLayerPNG(v.Pixels)
	if e != nil {
		return e
	}
	m, e := readLayerPNG(v.Mask)
	if e != nil {
		return e
	}
	*l = PaintLayer{v.Name, v.Visible, v.Opacity, p, m}
	return nil
}
func CopyImage(p *image.RGBA) *image.RGBA {
	if p == nil {
		return nil
	}
	r := image.NewRGBA(p.Bounds())
	draw.Draw(r, r.Bounds(), p, p.Bounds().Min, draw.Src)
	return r
}
func ClonePaint(p *FacePaint) *FacePaint {
	if p == nil {
		return nil
	}
	q := *p
	q.Img = CopyImage(p.Img)
	if p.Material != nil {
		q.Material = &Material{Maps: map[string]*MaterialMap{}}
		for k, m := range p.Material.Maps {
			if m != nil {
				n := *m
				n.Image = CopyImage(m.Image)
				q.Material.Maps[k] = &n
			}
		}
	}
	q.Layers = append([]PaintLayer(nil), p.Layers...)
	for i := range q.Layers {
		q.Layers[i].Pixels = CopyImage(q.Layers[i].Pixels)
		q.Layers[i].Mask = CopyImage(q.Layers[i].Mask)
	}
	return &q
}
func (p *FacePaint) CompositeLayers() {
	if len(p.Layers) == 0 {
		return
	}
	var bounds image.Rectangle
	for _, l := range p.Layers {
		if l.Pixels != nil {
			bounds = bounds.Union(l.Pixels.Bounds())
		}
	}
	out := image.NewRGBA(bounds)
	for _, l := range p.Layers {
		if !l.Visible || l.Pixels == nil {
			continue
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := l.Pixels.RGBAAt(x, y)
				alpha := max(0.0, min(1.0, l.Opacity))
				if l.Mask != nil {
					alpha *= float64(l.Mask.RGBAAt(x, y).R) / 255
				}
				d := out.RGBAAt(x, y)
				a := float64(c.A) / 255 * alpha
				out.SetRGBA(x, y, color.RGBA{uint8(float64(c.R)*alpha + float64(d.R)*(1-a)), uint8(float64(c.G)*alpha + float64(d.G)*(1-a)), uint8(float64(c.B)*alpha + float64(d.B)*(1-a)), uint8(float64(c.A)*alpha + float64(d.A)*(1-a))})
			}
		}
	}
	p.Img = out
}
