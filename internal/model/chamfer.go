package model

import (
	"fmt"
	"modeler/internal/geom/csg"
	"modeler/internal/geom/mesh"
	"sort"
)

// ChamferEdges prepares every body before applying any of them. Its cached
// result is also the live preview, so Apply uses exactly the displayed shape.
type ChamferEdges struct {
	Edges    map[uint32][]int
	Distance float64
	prepared []chamferBody
}

type chamferBody struct {
	id                  uint32
	before, after       *mesh.Mesh
	beforeSeq, afterSeq uint32
}

func (c *ChamferEdges) Name() string { return "Chamfer edges" }

func (c *ChamferEdges) Prepare(doc *Document) error {
	c.prepared = nil
	ids := make([]uint32, 0, len(c.Edges))
	for id := range c.Edges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		return fmt.Errorf("select at least one solid edge")
	}
	var pending []chamferBody
	for _, id := range ids {
		b := doc.BodyByID(id)
		if b == nil || b.Mesh == nil {
			return fmt.Errorf("the selected body is no longer there")
		}
		r, err := csg.Chamfer(b.Mesh, c.Edges[id], c.Distance)
		if err != nil {
			return fmt.Errorf("%s: %w", b.Name, err)
		}
		pending = append(pending, chamferBody{id, b.Mesh, r.Mesh, b.FaceSeq, r.NextFaceSeq})
	}
	c.prepared = pending
	return nil
}

func (c *ChamferEdges) Preview(id uint32) *mesh.Mesh {
	for _, b := range c.prepared {
		if b.id == id {
			return b.after
		}
	}
	return nil
}

func (c *ChamferEdges) Do(doc *Document) error {
	if len(c.prepared) == 0 {
		if err := c.Prepare(doc); err != nil {
			return err
		}
	}
	for _, p := range c.prepared {
		b := doc.BodyByID(p.id)
		if b == nil || b.Mesh != p.before || b.FaceSeq != p.beforeSeq {
			return fmt.Errorf("the body changed — select its edges again")
		}
	}
	for _, p := range c.prepared {
		b := doc.BodyByID(p.id)
		b.Mesh, b.FaceSeq = p.after, p.afterSeq
	}
	return nil
}

func (c *ChamferEdges) Undo(doc *Document) {
	for _, p := range c.prepared {
		if b := doc.BodyByID(p.id); b != nil {
			b.Mesh, b.FaceSeq = p.before, p.beforeSeq
		}
	}
}

func (c *ChamferEdges) Events() []Event {
	out := make([]Event, 0, len(c.prepared))
	for _, p := range c.prepared {
		out = append(out, Event{Kind: EvBodyChanged, BodyID: p.id})
	}
	return out
}
