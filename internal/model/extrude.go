package model

import (
	"fmt"
	"image/color"

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

	// Built during Do, so the caller can read what happened and Undo can put
	// things back.
	body          *Body
	prevConsumed  bool
	prevVisible   bool
	achievedDraft float64
	clamped       bool
}

func (c *Extrude) Name() string {
	if c.body != nil {
		return "Extrude " + c.body.Name
	}
	return "Extrude"
}

// Body returns the body the extrude created, valid after a successful Do.
func (c *Extrude) Body() *Body { return c.body }

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

func (c *Extrude) Undo(doc *Document) {
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
