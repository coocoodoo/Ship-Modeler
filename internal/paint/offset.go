package paint

import (
	"fmt"
	"image"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// OffsetTexture wraps the face's texel rectangle, keeping every channel on
// the same persistent grid. It never edits an image shared with another face.
func OffsetTexture(m *mesh.Mesh, fi int, p *mesh.FacePaint, dx, dy int) (*mesh.FacePaint, error) {
	if p == nil || p.Img == nil {
		return nil, fmt.Errorf("this face has no texture to move")
	}
	bounds := p.MaterialBounds(m, fi)
	if bounds.Empty() {
		return nil, fmt.Errorf("this face has no texture area")
	}
	wrap := func(x, lo, n int) int { return lo + ((x-lo)%n+n)%n }
	q := *p
	q.Img = image.NewRGBA(p.Img.Bounds())
	q.Material = nil
	if p.Material != nil {
		q.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}
		for k, src := range p.Material.Maps {
			if src != nil && src.Image != nil {
				q.Material.Maps[k] = &mesh.MaterialMap{Bounds: p.TexelBounds(), Image: image.NewRGBA(p.Img.Bounds())}
			}
		}
	}
	dx, dy = dx%bounds.Dx(), dy%bounds.Dy()
	for y := q.Img.Rect.Min.Y; y < q.Img.Rect.Max.Y; y++ {
		for x := q.Img.Rect.Min.X; x < q.Img.Rect.Max.X; x++ {
			sx := wrap(x+p.Off.X-dx, bounds.Min.X, bounds.Dx())
			sy := wrap(y+p.Off.Y-dy, bounds.Min.Y, bounds.Dy())
			q.Img.SetRGBA(x, y, p.Img.RGBAAt(sx-p.Off.X, sy-p.Off.Y))
			if q.Material != nil {
				for k, dst := range q.Material.Maps {
					dst.Image.SetRGBA(x, y, p.Material.Sample(k, float64(sx)+.5, float64(sy)+.5))
				}
			}
		}
	}

	q.Layers = append([]mesh.PaintLayer(nil), p.Layers...)
	for i, l := range p.Layers {
		for _, mask := range []bool{false, true} {
			src := l.Pixels
			if mask {
				src = l.Mask
			}
			if src == nil {
				continue
			}
			dst := image.NewRGBA(src.Bounds())
			for y := dst.Rect.Min.Y; y < dst.Rect.Max.Y; y++ {
				for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x++ {
					sx := wrap(x+p.Off.X-dx, bounds.Min.X, bounds.Dx())
					sy := wrap(y+p.Off.Y-dy, bounds.Min.Y, bounds.Dy())
					dst.SetRGBA(x, y, src.RGBAAt(sx-p.Off.X, sy-p.Off.Y))
				}
			}
			if mask {
				q.Layers[i].Mask = dst
			} else {
				q.Layers[i].Pixels = dst
			}
		}
	}
	q.CompositeLayers()
	return &q, nil
}

type MoveTexture struct {
	Body          uint32
	Face          mesh.FaceUID
	DX, DY        int
	before, after *mesh.FacePaint
}

func (c *MoveTexture) Name() string { return "Move texture" }
func (c *MoveTexture) Events() []model.Event {
	return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
}
func (c *MoveTexture) Do(d *model.Document) error {
	m, fi, e := resolveFace(d, c.Body, c.Face)
	if e != nil {
		return e
	}
	if c.after == nil {
		c.before = m.Faces[fi].Paint
		c.after, e = OffsetTexture(m, fi, c.before, c.DX, c.DY)
		if e != nil {
			return e
		}
	}
	m.Faces[fi].Paint = c.after
	return nil
}
func (c *MoveTexture) Undo(d *model.Document) {
	if m, fi, e := resolveFace(d, c.Body, c.Face); e == nil {
		m.Faces[fi].Paint = c.before
	}
}
func (c *MoveTexture) UndoBytes() int {
	n := 0
	for _, p := range []*mesh.FacePaint{c.before, c.after} {
		if p != nil && p.Img != nil {
			n += len(p.Img.Pix)
			if p.Material != nil {
				for _, v := range p.Material.Maps {
					if v != nil && v.Image != nil {
						n += len(v.Image.Pix)
					}
				}
			}
		}
	}
	return n
}

func (c *MoveTexture) TargetFace() (uint32, mesh.FaceUID) { return c.Body, c.Face }
