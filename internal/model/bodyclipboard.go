package model

import (
	"fmt"
	"image"
	"image/draw"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// SnapshotBody owns its pixels as well as its geometry. Ordinary mesh clones
// deliberately share paint for undo; a clipboard must outlive later strokes.
func SnapshotBody(src *Body) *Body {
	b := *src
	b.Mesh = src.Mesh.Clone()
	paints := map[*mesh.FacePaint]*mesh.FacePaint{}
	for i := range b.Mesh.Faces {
		f := &b.Mesh.Faces[i]
		if f.Paint == nil {
			continue
		}
		p, ok := paints[f.Paint]
		if !ok {
			cp := *f.Paint
			if cp.Img != nil {
				cp.Img = image.NewRGBA(cp.Img.Bounds())
				draw.Draw(cp.Img, cp.Img.Bounds(), f.Paint.Img, cp.Img.Bounds().Min, draw.Src)
			}
			p = &cp
			paints[f.Paint] = p
		}
		f.Paint = p
	}
	return &b
}

// PasteBodies inserts an independent group in one undo step.
type PasteBodies struct {
	Sources []*Body
	Offset  geom.Vec3
	copies  []*Body
}

func (c *PasteBodies) Name() string    { return "Paste bodies" }
func (c *PasteBodies) Copies() []*Body { return c.copies }
func (c *PasteBodies) Do(doc *Document) error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("copy a body first")
	}
	if c.copies == nil {
		for _, b := range c.Sources {
			if b == nil || b.Mesh == nil {
				return fmt.Errorf("clipboard body has no geometry")
			}
		}
		ids := map[mesh.FaceUID]mesh.FaceUID{}
		for _, src := range c.Sources {
			b := SnapshotBody(src)
			b.ID = doc.Seq.NextBody()
			b.Name = fmt.Sprintf("%s copy %d", src.Name, b.ID)
			b.Visible = true
			for i := range b.Mesh.Faces {
				f := &b.Mesh.Faces[i]
				id := mesh.MakeFaceUID(b.ID, uint32(i+1))
				ids[f.ID] = id
				f.ID = id
			}
			b.FaceSeq = uint32(len(b.Mesh.Faces))
			mesh.Translate(b.Mesh, c.Offset)
			c.copies = append(c.copies, b)
		}
		for _, b := range c.copies {
			for i := range b.Mesh.Faces {
				f := &b.Mesh.Faces[i]
				if id, ok := ids[f.SrcFace]; ok {
					f.SrcFace = id
				}
			}
		}
	}
	doc.Bodies = append(doc.Bodies, c.copies...)
	return nil
}
func (c *PasteBodies) Undo(doc *Document) {
	for _, b := range c.copies {
		if i := doc.bodyIndex(b.ID); i >= 0 {
			doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
		}
	}
}
func (c *PasteBodies) Events() []Event {
	var out []Event
	for _, b := range c.copies {
		out = append(out, Event{Kind: EvBodyAdded, BodyID: b.ID})
	}
	return out
}
