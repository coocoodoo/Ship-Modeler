package paint

import (
	"fmt"
	"image"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// SetMaterialMap assigns an image to a channel. Nil removes that channel.
// A base-color image becomes ordinary editable paint at the face's resolution.
type SetMaterialMap struct {
	Body          uint32
	Face          mesh.FaceUID
	Kind          string
	Image         *image.RGBA
	Res           int
	before, after *mesh.FacePaint
	faces         []int
}

func (c *SetMaterialMap) Name() string { return "Set " + c.Kind + " texture" }
func (c *SetMaterialMap) Events() []model.Event {
	return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
}
func (c *SetMaterialMap) Do(d *model.Document) error {
	if !mesh.ValidMaterialChannel(c.Kind) {
		return fmt.Errorf("unknown texture type %q", c.Kind)
	}
	m, fi, err := resolveFace(d, c.Body, c.Face)
	if err != nil {
		return err
	}
	if c.after == nil {
		c.before = m.Faces[fi].Paint
		p := c.before
		if p == nil {
			r := c.Res
			if r == 0 {
				r = DefaultRes
			}
			p, err = Allocate(m, fi, r)
			if err != nil {
				return err
			}
		}
		q := *p
		q.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}
		if p.Material != nil {
			for k, v := range p.Material.Maps {
				q.Material.Maps[k] = v
			}
		}
		if c.Kind == "base_color" {
			q.PBRStale = true
			q.Img = image.NewRGBA(p.Img.Bounds())
		}
		if c.Image == nil {
			delete(q.Material.Maps, c.Kind)
		} else {
			if c.Image.Bounds().Empty() || c.Image.Bounds().Dx() > 4096 || c.Image.Bounds().Dy() > 4096 {
				return fmt.Errorf("texture must be between 1 and 4096 pixels per side")
			}
			// Fit to the actual face, excluding the paint allocation's margin.
			bounds := p.MaterialBounds(m, fi)
			if bounds.Empty() {
				return fmt.Errorf("face has no texture area")
			}
			src := image.NewRGBA(image.Rect(0, 0, c.Image.Bounds().Dx(), c.Image.Bounds().Dy()))
			for y := 0; y < src.Rect.Dy(); y++ {
				for x := 0; x < src.Rect.Dx(); x++ {
					src.SetRGBA(x, y, c.Image.RGBAAt(x+c.Image.Rect.Min.X, y+c.Image.Rect.Min.Y))
				}
			}
			q.Material.Maps[c.Kind] = &mesh.MaterialMap{Bounds: bounds, Image: src}
			if c.Kind == "base_color" {
				q.Img = q.MaterialRaster(c.Kind)
				delete(q.Material.Maps, c.Kind)
			}
		}
		c.after = &q
		c.Image = nil // Redo uses the owned snapshot, not the import buffer.
		for i := range m.Faces {
			if i == fi || (c.before != nil && m.Faces[i].Paint == c.before) {
				c.faces = append(c.faces, i)
			}
		}
	}
	for _, i := range c.faces {
		m.Faces[i].Paint = c.after
	}
	return nil
}
func (c *SetMaterialMap) Undo(d *model.Document) {
	if b := d.BodyByID(c.Body); b != nil {
		for _, i := range c.faces {
			b.Mesh.Faces[i].Paint = c.before
		}
	}
}
func (c *SetMaterialMap) UndoBytes() int {
	n := 0
	seen := map[*image.RGBA]bool{}
	count := func(img *image.RGBA) {
		if img != nil && !seen[img] {
			n += len(img.Pix)
			seen[img] = true
		}
	}
	for _, p := range []*mesh.FacePaint{c.before, c.after} {
		if p == nil {
			continue
		}
		count(p.Img)
		if p.Material != nil {
			for _, m := range p.Material.Maps {
				if m != nil {
					count(m.Image)
				}
			}
		}
	}
	return n
}

func (c *SetMaterialMap) TargetFace() (uint32, mesh.FaceUID) { return c.Body, c.Face }

func (c *SetMaterialMap) PaintsLayer() bool { return c.Kind == "base_color" }
