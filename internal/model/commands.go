package model

import (
	"fmt"
	"image/color"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The commands M1 needs: everything the tree panel can do. Visibility toggles,
// renames, colour changes and deletions all go through the bus, so undo covers
// them exactly like geometry edits will (SPEC-DATA §3.4).
//
// Each command resolves a human label during Do so its toast can name the thing
// it acted on ("Hide Body 2") rather than an id.

// AddBody inserts a new body, assigning it the next id and auto colour.
type AddBody struct {
	Mesh  *mesh.Mesh
	Color color.RGBA
	// Label overrides the auto name "Body N" when non-empty.
	Label string

	body *Body
}

func (c *AddBody) Name() string {
	if c.body != nil {
		return "Add " + c.body.Name
	}
	return "Add body"
}

// AddedBody returns the body the command created, valid after a successful Do.
func (c *AddBody) AddedBody() *Body { return c.body }

func (c *AddBody) Do(doc *Document) error {
	if c.Mesh == nil {
		return fmt.Errorf("a body needs a mesh")
	}
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
		c.body = &Body{ID: id, Name: name, Color: col, Visible: true, Mesh: c.Mesh}
	}
	doc.Bodies = append(doc.Bodies, c.body)
	return nil
}

func (c *AddBody) Undo(doc *Document) {
	if i := doc.bodyIndex(c.body.ID); i >= 0 {
		doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
	}
}

func (c *AddBody) Events() []Event {
	return []Event{{Kind: EvBodyAdded, BodyID: c.body.ID}}
}

// AddSketch inserts a new sketch, on a default plane or anchored to a face.
type AddSketch struct {
	Plane geom.PlaneKind
	Label string

	// OnFace, Body, Face and Frame anchor the sketch to a body's face
	// (R9, SPEC-UX §10). Frame is the snapshot the sketch keeps for good.
	OnFace bool
	Body   uint32
	Face   mesh.FaceUID
	Frame  geom.Frame

	sketch *Sketch
}

func (c *AddSketch) Name() string {
	if c.sketch != nil {
		return "Add " + c.sketch.Name
	}
	return "Add sketch"
}

// AddedSketch returns the sketch the command created.
func (c *AddSketch) AddedSketch() *Sketch { return c.sketch }

func (c *AddSketch) Do(doc *Document) error {
	if c.sketch == nil {
		id := doc.Seq.NextSketch()
		name := c.Label
		if name == "" {
			name = fmt.Sprintf("Sketch %d", id)
		}
		c.sketch = &Sketch{
			ID: id, Name: name, Visible: true, Plane: c.Plane,
			OnFace: c.OnFace, Body: c.Body, FaceID: c.Face, FrameSnap: c.Frame,
		}
	}
	doc.Sketches = append(doc.Sketches, c.sketch)
	return nil
}

func (c *AddSketch) Undo(doc *Document) {
	if i := doc.sketchIndex(c.sketch.ID); i >= 0 {
		doc.Sketches = append(doc.Sketches[:i], doc.Sketches[i+1:]...)
	}
}

func (c *AddSketch) Events() []Event {
	return []Event{{Kind: EvSketchAdded, Sketch: c.sketch.ID}}
}

// visibilityLabel is the shared phrasing for show/hide toasts.
func visibilityLabel(visible bool, what string) string {
	if visible {
		return "Show " + what
	}
	return "Hide " + what
}

// SetBodyVisible shows or hides a body.
type SetBodyVisible struct {
	ID      uint32
	Visible bool

	prev  bool
	label string
}

func (c *SetBodyVisible) Name() string {
	if c.label == "" {
		return visibilityLabel(c.Visible, "body")
	}
	return c.label
}

func (c *SetBodyVisible) Do(doc *Document) error {
	b := doc.BodyByID(c.ID)
	if b == nil {
		return fmt.Errorf("no body %d", c.ID)
	}
	c.prev = b.Visible
	c.label = visibilityLabel(c.Visible, b.Name)
	b.Visible = c.Visible
	return nil
}

func (c *SetBodyVisible) Undo(doc *Document) {
	if b := doc.BodyByID(c.ID); b != nil {
		b.Visible = c.prev
	}
}

func (c *SetBodyVisible) Events() []Event {
	return []Event{{Kind: EvBodyChanged, BodyID: c.ID}}
}

// SetSketchVisible shows or hides a sketch.
type SetSketchVisible struct {
	ID      uint32
	Visible bool

	prev  bool
	label string
}

func (c *SetSketchVisible) Name() string {
	if c.label == "" {
		return visibilityLabel(c.Visible, "sketch")
	}
	return c.label
}

func (c *SetSketchVisible) Do(doc *Document) error {
	s := doc.SketchByID(c.ID)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.ID)
	}
	c.prev = s.Visible
	c.label = visibilityLabel(c.Visible, s.Name)
	s.Visible = c.Visible
	return nil
}

func (c *SetSketchVisible) Undo(doc *Document) {
	if s := doc.SketchByID(c.ID); s != nil {
		s.Visible = c.prev
	}
}

func (c *SetSketchVisible) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.ID}}
}

// SetPlaneVisible shows or hides a default plane. Planes can never be deleted,
// only hidden (R1).
type SetPlaneVisible struct {
	Plane   geom.PlaneKind
	Visible bool

	prev bool
}

func (c *SetPlaneVisible) Name() string {
	return visibilityLabel(c.Visible, c.Plane.String()+" plane")
}

func (c *SetPlaneVisible) Do(doc *Document) error {
	if c.Plane < 0 || int(c.Plane) >= geom.PlaneCount {
		return fmt.Errorf("no plane %v", c.Plane)
	}
	c.prev = doc.Planes[c.Plane].Visible
	doc.Planes[c.Plane].Visible = c.Visible
	return nil
}

func (c *SetPlaneVisible) Undo(doc *Document) {
	doc.Planes[c.Plane].Visible = c.prev
}

func (c *SetPlaneVisible) Events() []Event {
	return []Event{{Kind: EvPlanesChanged, Plane: c.Plane}}
}

// RenameBody changes a body's display name.
type RenameBody struct {
	ID uint32
	To string

	prev string
}

func (c *RenameBody) Name() string {
	if c.prev == "" {
		return "Rename body"
	}
	return "Rename " + c.prev
}

func (c *RenameBody) Do(doc *Document) error {
	name, err := CleanName(c.To)
	if err != nil {
		return err
	}
	b := doc.BodyByID(c.ID)
	if b == nil {
		return fmt.Errorf("no body %d", c.ID)
	}
	c.To = name
	c.prev = b.Name
	b.Name = name
	return nil
}

func (c *RenameBody) Undo(doc *Document) {
	if b := doc.BodyByID(c.ID); b != nil {
		b.Name = c.prev
	}
}

func (c *RenameBody) Events() []Event {
	return []Event{{Kind: EvBodyChanged, BodyID: c.ID}}
}

// RenameSketch changes a sketch's display name.
type RenameSketch struct {
	ID uint32
	To string

	prev string
}

func (c *RenameSketch) Name() string {
	if c.prev == "" {
		return "Rename sketch"
	}
	return "Rename " + c.prev
}

func (c *RenameSketch) Do(doc *Document) error {
	name, err := CleanName(c.To)
	if err != nil {
		return err
	}
	s := doc.SketchByID(c.ID)
	if s == nil {
		return fmt.Errorf("no sketch %d", c.ID)
	}
	c.To = name
	c.prev = s.Name
	s.Name = name
	return nil
}

func (c *RenameSketch) Undo(doc *Document) {
	if s := doc.SketchByID(c.ID); s != nil {
		s.Name = c.prev
	}
}

func (c *RenameSketch) Events() []Event {
	return []Event{{Kind: EvSketchChanged, Sketch: c.ID}}
}

// SetBodyColor recolours a body. Painted faces keep their pixels; the colour
// shows through wherever paint is absent (SPEC-UX §7).
type SetBodyColor struct {
	ID uint32
	To color.RGBA

	prev  color.RGBA
	label string
}

func (c *SetBodyColor) Name() string {
	if c.label == "" {
		return "Recolor body"
	}
	return "Recolor " + c.label
}

func (c *SetBodyColor) Do(doc *Document) error {
	b := doc.BodyByID(c.ID)
	if b == nil {
		return fmt.Errorf("no body %d", c.ID)
	}
	c.prev = b.Color
	c.label = b.Name
	b.Color = c.To
	return nil
}

func (c *SetBodyColor) Undo(doc *Document) {
	if b := doc.BodyByID(c.ID); b != nil {
		b.Color = c.prev
	}
}

func (c *SetBodyColor) Events() []Event {
	return []Event{{Kind: EvBodyChanged, BodyID: c.ID}}
}

// DeleteBody removes a body, remembering where it sat so undo puts it back in
// the same place rather than at the end of the list.
type DeleteBody struct {
	ID uint32

	body  *Body
	index int
}

func (c *DeleteBody) Name() string {
	if c.body == nil {
		return "Delete body"
	}
	return "Delete " + c.body.Name
}

func (c *DeleteBody) Do(doc *Document) error {
	i := doc.bodyIndex(c.ID)
	if i < 0 {
		return fmt.Errorf("no body %d", c.ID)
	}
	c.body, c.index = doc.Bodies[i], i
	doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
	return nil
}

func (c *DeleteBody) Undo(doc *Document) {
	if c.body == nil {
		return
	}
	i := c.index
	if i > len(doc.Bodies) {
		i = len(doc.Bodies)
	}
	doc.Bodies = append(doc.Bodies, nil)
	copy(doc.Bodies[i+1:], doc.Bodies[i:])
	doc.Bodies[i] = c.body
}

func (c *DeleteBody) Events() []Event {
	return []Event{{Kind: EvBodyRemoved, BodyID: c.ID}}
}

// DeleteSketch removes a sketch, restoring its position on undo.
type DeleteSketch struct {
	ID uint32

	sketch *Sketch
	index  int
}

func (c *DeleteSketch) Name() string {
	if c.sketch == nil {
		return "Delete sketch"
	}
	return "Delete " + c.sketch.Name
}

func (c *DeleteSketch) Do(doc *Document) error {
	i := doc.sketchIndex(c.ID)
	if i < 0 {
		return fmt.Errorf("no sketch %d", c.ID)
	}
	c.sketch, c.index = doc.Sketches[i], i
	doc.Sketches = append(doc.Sketches[:i], doc.Sketches[i+1:]...)
	return nil
}

func (c *DeleteSketch) Undo(doc *Document) {
	if c.sketch == nil {
		return
	}
	i := c.index
	if i > len(doc.Sketches) {
		i = len(doc.Sketches)
	}
	doc.Sketches = append(doc.Sketches, nil)
	copy(doc.Sketches[i+1:], doc.Sketches[i:])
	doc.Sketches[i] = c.sketch
}

func (c *DeleteSketch) Events() []Event {
	return []Event{{Kind: EvSketchRemoved, Sketch: c.ID}}
}

// autoBodyColors is the desaturated eight-colour cycle new bodies take
// (SPEC-UX §3). It lives here rather than in ui so the document can assign a
// colour without depending on the widget kit; ui exposes the same list for
// swatch pickers.
var autoBodyColors = []color.RGBA{
	{R: 0x8E, G: 0xA3, B: 0xB0, A: 255},
	{R: 0xB0, G: 0x8E, B: 0x8E, A: 255},
	{R: 0x8E, G: 0xB0, B: 0x9B, A: 255},
	{R: 0xA3, G: 0x8E, B: 0xB0, A: 255},
	{R: 0xB0, G: 0xA9, B: 0x8E, A: 255},
	{R: 0x8E, G: 0x9B, B: 0xB0, A: 255},
	{R: 0xB0, G: 0x8E, B: 0xA6, A: 255},
	{R: 0x96, G: 0xB0, B: 0x8E, A: 255},
}

// AutoBodyColor returns the auto colour for the n-th body created.
func AutoBodyColor(n int) color.RGBA {
	i := n % len(autoBodyColors)
	if i < 0 {
		i += len(autoBodyColors)
	}
	return autoBodyColors[i]
}

// PaletteForBodies is the preset row the body colour picker offers: the auto
// cycle itself, so a recoloured body can always be put back.
func PaletteForBodies() []color.RGBA {
	return append([]color.RGBA(nil), autoBodyColors...)
}
