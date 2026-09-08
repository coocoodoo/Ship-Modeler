package paint

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"sort"
)

// EditLayer is face-local and undoable. Visible layers are flattened for all
// existing renderers/exporters; original pixels and masks stay editable.
type EditLayer struct {
	Body          uint32
	Face          mesh.FaceUID
	Action, Label string
	Index         int
	Value         float64
	On            bool
	before, after *mesh.FacePaint
}

func (c *EditLayer) TargetFace() (uint32, mesh.FaceUID) { return c.Body, c.Face }
func (c *EditLayer) NameOfAction() string               { return "Paint layer · " + c.Action }
func (c *EditLayer) Name() string                       { return c.NameOfAction() }
func (c *EditLayer) Do(d *model.Document) error {
	m, fi, e := resolveFace(d, c.Body, c.Face)
	if e != nil {
		return e
	}
	c.before = m.Faces[fi].Paint
	p := mesh.ClonePaint(c.before)
	if p == nil {
		p, e = Allocate(m, fi, 32)
		if e != nil {
			return e
		}
	}
	if len(p.Layers) == 0 {
		p.Layers = []mesh.PaintLayer{{Name: "Base", Visible: true, Opacity: 1, Pixels: mesh.CopyImage(p.Img)}}
		p.ActiveLayer = 0
	}
	i := c.Index
	if i < 0 || i >= len(p.Layers) {
		return fmt.Errorf("layer index out of range")
	}
	switch c.Action {
	case "add":
		name := c.Label
		if name == "" {
			name = "Detail"
		}
		p.Layers = append(p.Layers, mesh.PaintLayer{Name: name, Visible: true, Opacity: 1, Pixels: image.NewRGBA(p.Img.Bounds())})
		p.ActiveLayer = len(p.Layers) - 1
	case "select":
		p.ActiveLayer = i
		p.PaintMask = false
	case "rename":
		if c.Label == "" {
			return fmt.Errorf("enter a layer name")
		}
		p.Layers[i].Name = c.Label
	case "visible":
		p.Layers[i].Visible = c.On
	case "opacity":
		if c.Value < 0 || c.Value > 1 {
			return fmt.Errorf("opacity must be 0 to 1")
		}
		p.Layers[i].Opacity = c.Value
	case "mask":
		p.ActiveLayer = i
		p.PaintMask = c.On
		if c.On && p.Layers[i].Mask == nil {
			p.Layers[i].Mask = image.NewRGBA(p.Img.Bounds())
			for y := p.Img.Rect.Min.Y; y < p.Img.Rect.Max.Y; y++ {
				for x := p.Img.Rect.Min.X; x < p.Img.Rect.Max.X; x++ {
					p.Layers[i].Mask.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
				}
			}
		}
	case "mask.clear":
		p.Layers[i].Mask = nil
		p.PaintMask = false
	case "delete":
		if len(p.Layers) == 1 {
			return fmt.Errorf("keep at least one layer")
		}
		p.Layers = append(p.Layers[:i], p.Layers[i+1:]...)
		p.ActiveLayer = min(i, len(p.Layers)-1)
	case "up", "down":
		j := i + 1
		if c.Action == "down" {
			j = i - 1
		}
		if j < 0 || j >= len(p.Layers) {
			return fmt.Errorf("layer is already at the end")
		}
		p.Layers[i], p.Layers[j] = p.Layers[j], p.Layers[i]
		p.ActiveLayer = j
	default:
		return fmt.Errorf("unknown layer action")
	}
	p.CompositeLayers()
	if c.Action != "select" && c.Action != "mask" && c.Action != "rename" {
		p.PBRStale = true
	}
	c.after = p
	m.Faces[fi].Paint = p
	return nil
}
func (c *EditLayer) Undo(d *model.Document) {
	if m, i, e := resolveFace(d, c.Body, c.Face); e == nil {
		m.Faces[i].Paint = c.before
	}
}
func (c *EditLayer) Events() []model.Event {
	return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
}

type MaterialReference struct {
	Body       uint32             `json:"body"`
	Face       mesh.FaceUID       `json:"face"`
	Resolution int                `json:"resolution"`
	Palette    []color.RGBA       `json:"palette"`
	PBRMeans   map[string]float64 `json:"pbrMeans"`
	Weathering float64            `json:"weathering"`
}

func AnalyzeReference(d *model.Document, body uint32, face mesh.FaceUID) (MaterialReference, error) {
	m, i, e := resolveFace(d, body, face)
	if e != nil {
		return MaterialReference{}, e
	}
	p := m.Faces[i].Paint
	if p == nil || p.Img == nil {
		return MaterialReference{}, fmt.Errorf("reference face needs a texture")
	}
	r := MaterialReference{Body: body, Face: face, Resolution: p.Res, PBRMeans: map[string]float64{}}
	counts := map[color.RGBA]int{}
	rect := p.MaterialBounds(m, i).Sub(p.Off)
	var sum, sum2, n float64
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			c := p.Img.RGBAAt(x, y)
			if c.A < 128 {
				continue
			}
			counts[c]++
			v := (float64(c.R) + float64(c.G) + float64(c.B)) / 3
			sum += v
			sum2 += v * v
			n++
		}
	}
	for c := range counts {
		r.Palette = append(r.Palette, c)
	}
	sort.Slice(r.Palette, func(i, j int) bool {
		a, b := r.Palette[i], r.Palette[j]
		if counts[a] != counts[b] {
			return counts[a] > counts[b]
		}
		return uint32(a.R)<<16|uint32(a.G)<<8|uint32(a.B) < uint32(b.R)<<16|uint32(b.G)<<8|uint32(b.B)
	})
	if len(r.Palette) > 24 {
		r.Palette = r.Palette[:24]
	}
	if n > 0 {
		r.Weathering = math.Sqrt(math.Max(0, sum2/n-sum/n*sum/n)) / 255
	}
	if p.Material != nil {
		for k, v := range p.Material.Maps {
			if v == nil || v.Image == nil {
				continue
			}
			var sum, n float64
			for y := v.Image.Rect.Min.Y; y < v.Image.Rect.Max.Y; y++ {
				for x := v.Image.Rect.Min.X; x < v.Image.Rect.Max.X; x++ {
					sum += float64(v.Image.RGBAAt(x, y).R)
					n++
				}
			}
			if n > 0 {
				r.PBRMeans[k] = sum / n / 255
			}
		}
	}
	return r, nil
}

type MatchReference struct {
	Body          uint32
	Face          mesh.FaceUID
	Reference     MaterialReference
	before, after *mesh.FacePaint
}

func (c *MatchReference) Name() string                       { return "Match reference face material" }
func (c *MatchReference) TargetFace() (uint32, mesh.FaceUID) { return c.Body, c.Face }
func (c *MatchReference) Do(d *model.Document) error {
	m, i, e := resolveFace(d, c.Body, c.Face)
	if e != nil {
		return e
	}
	c.before = m.Faces[i].Paint
	if c.before == nil {
		return fmt.Errorf("paint the target face first")
	}
	r := c.Reference
	if len(r.Palette) == 0 {
		return fmt.Errorf("choose a painted material reference")
	}
	p := mesh.ClonePaint(c.before)
	if p.Res != r.Resolution {
		p, e = Resample(m, i, p, r.Resolution)
		if e != nil {
			return e
		}
	}
	recolor := func(img *image.RGBA) {
		if img == nil {
			return
		}
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				src := img.RGBAAt(x, y)
				if src.A == 0 {
					continue
				}
				best := r.Palette[0]
				dist := math.Inf(1)
				for _, v := range r.Palette {
					dd := math.Pow(float64(src.R)-float64(v.R), 2) + math.Pow(float64(src.G)-float64(v.G), 2) + math.Pow(float64(src.B)-float64(v.B), 2)
					if dd < dist {
						dist = dd
						best = v
					}
				}
				best.A = src.A
				img.SetRGBA(x, y, best)
			}
		}
	}
	if len(p.Layers) > 0 {
		for j := range p.Layers {
			recolor(p.Layers[j].Pixels)
		}
		p.CompositeLayers()
	} else {
		recolor(p.Img)
	}
	GeneratePBR(p, &r)
	c.after = p
	m.Faces[i].Paint = p
	return nil
}
func (c *MatchReference) Undo(d *model.Document) {
	if m, i, e := resolveFace(d, c.Body, c.Face); e == nil {
		m.Faces[i].Paint = c.before
	}
}
func (c *MatchReference) Events() []model.Event {
	return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
}

// GeneratePBR derives restrained relief and wear from the actual pixel grid.
// Reference means control the material response without copying its artwork.
func GeneratePBR(p *mesh.FacePaint, ref *MaterialReference) {
	if p == nil || p.Img == nil {
		return
	}
	p.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}
	rough, spec, wear := .72, .35, .18
	if ref != nil {
		wear = ref.Weathering
		if v, ok := ref.PBRMeans["roughness"]; ok {
			rough = v
		}
		if v, ok := ref.PBRMeans["specular"]; ok {
			spec = v
		}
	}
	gray := func(v float64) color.RGBA {
		n := uint8(math.Max(0, math.Min(255, v*255)))
		return color.RGBA{n, n, n, 255}
	}
	lum := func(x, y int) float64 {
		c := p.Img.RGBAAt(max(p.Img.Rect.Min.X, min(p.Img.Rect.Max.X-1, x)), max(p.Img.Rect.Min.Y, min(p.Img.Rect.Max.Y-1, y)))
		return (.2126*float64(c.R) + .7152*float64(c.G) + .0722*float64(c.B)) / 255
	}
	for _, k := range mesh.MaterialChannels[1:] {
		img := image.NewRGBA(p.Img.Bounds())
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				l := lum(x, y)
				c := gray(1)
				switch k {
				case "specular":
					c = gray(spec + (l-.5)*.12)
				case "roughness":
					c = gray(rough + (1-l)*wear*.3)
				case "height":
					c = gray(.46 + l*.08)
				case "ao":
					c = gray(.85 + .15*l)
				case "normal":
					dx := (lum(x+1, y) - lum(x-1, y)) * .12
					dy := (lum(x, y+1) - lum(x, y-1)) * .12
					c = color.RGBA{uint8(128 - dx*127), uint8(128 - dy*127), 255, 255}
				}
				img.SetRGBA(x, y, c)
			}
		}
		p.Material.Maps[k] = &mesh.MaterialMap{Bounds: p.TexelBounds(), Image: img}
	}
	p.PBRStale = false
}

type RefreshPBR struct {
	Body   uint32
	Face   mesh.FaceUID
	before *mesh.FacePaint
}

func (c *RefreshPBR) Name() string                       { return "Generate PBR from base color" }
func (c *RefreshPBR) TargetFace() (uint32, mesh.FaceUID) { return c.Body, c.Face }
func (c *RefreshPBR) Do(d *model.Document) error {
	m, i, e := resolveFace(d, c.Body, c.Face)
	if e != nil {
		return e
	}
	c.before = m.Faces[i].Paint
	if c.before == nil {
		return fmt.Errorf("paint the face first")
	}
	p := mesh.ClonePaint(c.before)
	GeneratePBR(p, nil)
	m.Faces[i].Paint = p
	return nil
}
func (c *RefreshPBR) Undo(d *model.Document) {
	if m, i, e := resolveFace(d, c.Body, c.Face); e == nil {
		m.Faces[i].Paint = c.before
	}
}
func (c *RefreshPBR) Events() []model.Event {
	return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
}
