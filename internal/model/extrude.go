package model

import (
	"fmt"
	"image/color"

	"modeler/internal/geom/csg"
	"modeler/internal/geom/extrude"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
)

// The extrude command (R4–R8, SPEC-UX §9.5). It builds and validates the whole
// solid before touching the document, so a failure leaves the document exactly
// as it was — the atomicity contract of SPEC-DATA §3.1.

// Extrude turns selected regions of a sketch into a new body.
type Extrude struct {
	Sketch  uint32
	Regions []int
	Params  extrude.Params
	// Color is the new body's colour; a zero alpha means the next auto colour.
	Color color.RGBA
	// Label overrides the auto name.
	Label string

	// Combine folds the extruded solid into existing bodies instead of making
	// a new one (R8, SPEC-UX §9.4). Nil is a new body. Targets are the bodies
	// it acts on: one for Add and Intersect, every intersected body for
	// Subtract.
	Combine *csg.Op
	Targets []uint32

	// Built during Do, so the caller can read what happened and Undo can put
	// things back.
	body          *Body
	prevConsumed  bool
	prevVisible   bool
	achievedDraft float64
	clamped       bool
	// combined records what a Result other than New did, so Undo can reverse it.
	combined []combinedBody
	emptied  []*Body
	emptyAt  []int
}

// combinedBody is one body a boolean touched, and what it looked like before.
type combinedBody struct {
	body     *Body
	prevMesh *mesh.Mesh
	prevSeq  uint32
}

func (c *Extrude) Name() string {
	if c.Combine != nil {
		return "Extrude " + c.Combine.String()
	}
	if c.body != nil {
		return "Extrude " + c.body.Name
	}
	return "Extrude"
}

// Body returns the body the extrude created, valid after a successful Do.
// Nil when the result folded into existing bodies instead of making a new one.
func (c *Extrude) Body() *Body {
	if c.Combine != nil {
		return nil
	}
	return c.body
}

// AchievedDraft and Clamped report what the geometry could actually do with the
// requested draft angle (SPEC-UX §9.3).
func (c *Extrude) AchievedDraft() float64 { return c.achievedDraft }
func (c *Extrude) Clamped() bool          { return c.clamped }

func (c *Extrude) Do(doc *Document) error {
	s := doc.SketchByID(c.Sketch)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.Sketch)
	}
	arr := s.Arrangement()
	if len(arr.Regions) == 0 {
		return fmt.Errorf("that sketch has no closed region to extrude")
	}

	picked := make([]sketch2d.Region, 0, len(c.Regions))
	for _, i := range c.Regions {
		if i < 0 || i >= len(arr.Regions) {
			return fmt.Errorf("region %d is not in that sketch", i)
		}
		picked = append(picked, arr.Regions[i])
	}
	if len(picked) == 0 {
		return fmt.Errorf("select a closed region to extrude")
	}

	// The body id has to be known before the mesh is built, because every face
	// identity is derived from it (SPEC-DATA §1). Reusing the id across a redo
	// keeps those identities stable.
	if c.body == nil {
		id := doc.Seq.NextBody()
		name := c.Label
		if name == "" {
			name = fmt.Sprintf("Body %d", id)
		}
		col := c.Color
		if col.A == 0 {
			col = AutoBodyColor(int(id) - 1)
		}
		c.body = &Body{ID: id, Name: name, Color: col, Visible: true}
	}

	p := c.Params
	p.Frame = s.Frame()
	built, err := extrude.Build(picked, p, c.body.ID)
	if err != nil {
		return err
	}

	if c.Combine != nil {
		return c.combine(doc, built.Mesh, built.AchievedDraft, built.Clamped, s)
	}

	// Everything below this line is a plain assignment: nothing can fail now.
	c.body.Mesh = built.Mesh
	c.body.FaceSeq = uint32(len(built.Mesh.Faces))
	c.achievedDraft, c.clamped = built.AchievedDraft, built.Clamped

	doc.Bodies = append(doc.Bodies, c.body)

	// The sketch is consumed and auto-hides, so the new body is what you see
	// (SPEC-UX §9.5). It is kept, not deleted: it can be re-entered and
	// extruded again.
	c.prevConsumed, c.prevVisible = s.Consumed, s.Visible
	s.Consumed, s.Visible = true, false
	return nil
}

// combine folds the extruded solid into existing bodies (SPEC-UX §9.4).
//
// Every boolean is run and validated before a single body is written to, so a
// subtract that fails on the third of three targets leaves the first two as
// they were rather than half-cut.
func (c *Extrude) combine(doc *Document, tool *mesh.Mesh, draft float64, clamped bool, s *Sketch) error {
	type pending struct {
		body  *Body
		mesh  *mesh.Mesh
		seq   uint32
		empty bool
	}
	out := make([]pending, 0, len(c.Targets))
	for _, id := range c.Targets {
		b := doc.BodyByID(id)
		if b == nil || b.Mesh == nil {
			return fmt.Errorf("the body to combine with is no longer there")
		}
		res, err := csg.Boolean(*c.Combine, b.Mesh, tool)
		if err != nil {
			return err
		}
		out = append(out, pending{body: b, mesh: res.Mesh, seq: res.NextFaceSeq, empty: res.Empty})
	}
	if len(out) == 0 {
		return fmt.Errorf("nothing to combine with")
	}

	// Nothing below can fail.
	c.achievedDraft, c.clamped = draft, clamped
	for _, p := range out {
		c.combined = append(c.combined, combinedBody{
			body: p.body, prevMesh: p.body.Mesh, prevSeq: p.body.FaceSeq,
		})
		if p.empty {
			if i := doc.bodyIndex(p.body.ID); i >= 0 {
				c.emptied = append(c.emptied, p.body)
				c.emptyAt = append(c.emptyAt, i)
				doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
			}
			continue
		}
		p.body.Mesh, p.body.FaceSeq = p.mesh, p.seq
	}

	c.prevConsumed, c.prevVisible = s.Consumed, s.Visible
	s.Consumed, s.Visible = true, false
	return nil
}

// CombinedInto lists the bodies a non-New result changed, for toasts and
// selection.
func (c *Extrude) CombinedInto() []*Body {
	out := make([]*Body, 0, len(c.combined))
	for _, cb := range c.combined {
		out = append(out, cb.body)
	}
	return out
}

// Emptied reports bodies a subtract or intersect removed entirely.
func (c *Extrude) Emptied() []*Body { return c.emptied }

func (c *Extrude) Undo(doc *Document) {
	for i := len(c.emptied) - 1; i >= 0; i-- {
		doc.Bodies = insertBodyAt(doc.Bodies, c.emptyAt[i], c.emptied[i])
	}
	c.emptied, c.emptyAt = nil, nil
	for _, cb := range c.combined {
		cb.body.Mesh, cb.body.FaceSeq = cb.prevMesh, cb.prevSeq
	}
	c.combined = nil

	if c.body != nil {
		if i := doc.bodyIndex(c.body.ID); i >= 0 {
			doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
		}
	}
	if s := doc.SketchByID(c.Sketch); s != nil {
		s.Consumed, s.Visible = c.prevConsumed, c.prevVisible
	}
}

func (c *Extrude) Events() []Event {
	if c.Combine != nil {
		out := []Event{{Kind: EvSketchChanged, Sketch: c.Sketch}}
		for _, id := range c.Targets {
			out = append(out, Event{Kind: EvBodyChanged, BodyID: id})
		}
		return out
	}
	return []Event{
		{Kind: EvBodyAdded, BodyID: c.body.ID},
		{Kind: EvSketchChanged, Sketch: c.Sketch},
	}
}

// FaceCount reports how many faces the built solid has, for toasts and tests.
func (c *Extrude) FaceCount() int {
	if c.body == nil || c.body.Mesh == nil {
		return 0
	}
	return len(c.body.Mesh.Faces)
}

// Volume reports the built solid's volume.
func (c *Extrude) Volume() float64 {
	if c.body == nil || c.body.Mesh == nil {
		return 0
	}
	return mesh.Volume(c.body.Mesh)
}
