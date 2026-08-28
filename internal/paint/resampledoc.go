package paint

import (
	"fmt"
	"math"

	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// One model, one pixel size (V-140).
//
// The user's words, verbatim: "I want them to be just 1 size, 1px... Not
// SCALE." A pixel that changes physical size from face to face is not a
// pixel, it is a texture — so once anything is painted, the Res chips stop
// meaning "what new faces get" and start meaning "the model's pixel size",
// and changing one resamples everything together, as one undoable step.

// ResampleModel rebuilds every painted picture in the document at one
// density, nearest-sampled, preserving which faces share which picture.
type ResampleModel struct {
	Res int

	// groups is filled by the first Do and replayed by redo.
	groups []resampleGroup
}

// resampleGroup is one distinct picture and every face that reads it.
type resampleGroup struct {
	prev, next *mesh.FacePaint
	body       uint32
	faces      []mesh.FaceUID
}

func (c *ResampleModel) Name() string {
	return fmt.Sprintf("Resample the model to %d px", c.Res)
}

func (c *ResampleModel) Do(doc *model.Document) error {
	if !ValidRes(c.Res) {
		return fmt.Errorf("%d is not a paint resolution", c.Res)
	}
	// A redo replays the pictures already built.
	if len(c.groups) > 0 {
		c.assign(doc, false)
		return nil
	}

	want := 1 / float64(c.Res)
	for _, b := range doc.Bodies {
		if b.Mesh == nil {
			continue
		}
		m := b.Mesh
		seen := map[*mesh.FacePaint][]int{}
		order := []*mesh.FacePaint{}
		for fi := range m.Faces {
			if p := m.Faces[fi].Paint; p != nil {
				if _, had := seen[p]; !had {
					order = append(order, p)
				}
				seen[p] = append(seen[p], fi)
			}
		}
		for _, p := range order {
			if math.Abs(p.Texel-want) < 1e-12 {
				continue
			}
			faces := seen[p]
			next, err := Resample(m, faces[0], p, c.Res)
			if err != nil {
				return err
			}
			g := resampleGroup{prev: p, next: next, body: b.ID}
			for _, fi := range faces {
				g.faces = append(g.faces, m.Faces[fi].ID)
			}
			c.groups = append(c.groups, g)
		}
	}
	if len(c.groups) == 0 {
		return fmt.Errorf("the model already paints at %d px", c.Res)
	}
	c.assign(doc, false)
	return nil
}

func (c *ResampleModel) Undo(doc *model.Document) { c.assign(doc, true) }

// assign points every group's faces at its next (or prev) picture. Faces are
// found by identity, so an edit between do and undo cannot misdirect it.
func (c *ResampleModel) assign(doc *model.Document, back bool) {
	for _, g := range c.groups {
		b := doc.BodyByID(g.body)
		if b == nil || b.Mesh == nil {
			continue
		}
		p := g.next
		if back {
			p = g.prev
		}
		for _, uid := range g.faces {
			for fi := range b.Mesh.Faces {
				if b.Mesh.Faces[fi].ID == uid {
					b.Mesh.Faces[fi].Paint = p
				}
			}
		}
	}
}

func (c *ResampleModel) Events() []model.Event {
	seen := map[uint32]bool{}
	var out []model.Event
	for _, g := range c.groups {
		if !seen[g.body] {
			seen[g.body] = true
			out = append(out, model.Event{Kind: model.EvBodyChanged, BodyID: g.body})
		}
	}
	return out
}
