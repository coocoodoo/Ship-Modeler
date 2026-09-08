package model

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"modeler/internal/geom/mesh"
	"reflect"
)

type FaceScope struct {
	Body uint32       `json:"body"`
	Face mesh.FaceUID `json:"face"`
}

// Only commands declaring a single face can enter a pin request. They run on
// an isolated face image, so shared paint pointers cannot broaden the scope.
type FaceCommand interface{ TargetFace() (uint32, mesh.FaceUID) }
type LayerBrush interface{ PaintsLayer() bool }
type Review struct {
	Pin                     uint32      `json:"pin"`
	Scope                   []FaceScope `json:"scope"`
	Changed                 []FaceScope `json:"changed"`
	Status                  string      `json:"status"`
	Description             string      `json:"description"`
	BeforeImage, AfterImage []byte
	before                  *Document
	commands                []Command
}

func PinStatus(p NotePin) string {
	if p.Done {
		return "Done"
	}
	if p.Status == "In progress" || p.Status == "Needs review" {
		return p.Status
	}
	return "Open"
}
func (b *Bus) Review() *Review { return b.review }
func (b *Bus) BeginReview(id uint32) error {
	if b.review != nil || b.pending != nil {
		return fmt.Errorf("finish the current request or drag first")
	}
	p := b.doc.NotePinByID(id)
	if p == nil {
		return fmt.Errorf("pin not found")
	}
	if _, ok := p.Position(b.doc); !ok {
		return fmt.Errorf("pin is not attached to a current face")
	}
	d := *b.doc
	d.Bodies = append([]*Body(nil), b.doc.Bodies...)
	d.NotePins = append([]NotePin(nil), b.doc.NotePins...)
	d.Features = append([]FeatureRec(nil), b.doc.Features...)
	b.review = &Review{Pin: id, Scope: []FaceScope{{p.Body, p.Face}}, Status: "In progress", before: b.doc, BeforeImage: faceThumbnail(b.doc, FaceScope{p.Body, p.Face})}
	b.doc = &d
	p = d.NotePinByID(id)
	p.Done = false
	p.Status = "In progress"
	b.Events.Emit(Event{Kind: EvDocReplaced})
	return nil
}

// ExpandReview is invoked only by the user's explicit selection control.
func (b *Bus) ExpandReview(s FaceScope) error {
	if b.review == nil {
		return fmt.Errorf("start a pin request first")
	}
	if body, _ := findFace(b.doc, s); body == nil {
		return fmt.Errorf("choose a current face")
	}
	for _, v := range b.review.Scope {
		if v == s {
			return nil
		}
	}
	b.review.Scope = append(b.review.Scope, s)
	return nil
}
func (b *Bus) ProposeReview(text string) error {
	r := b.review
	if r == nil {
		return fmt.Errorf("start work on a pin first")
	}
	if b.pending != nil {
		return fmt.Errorf("finish the drag first")
	}
	if len(r.Changed) == 0 {
		return fmt.Errorf("no face changes to review")
	}
	if text == "" {
		return fmt.Errorf("describe what changed")
	}
	r.Status = "Needs review"
	r.Description = text
	r.AfterImage = faceThumbnail(b.doc, r.Scope[0])
	p := b.doc.NotePinByID(r.Pin)
	p.Status = r.Status
	p.Changes = text
	p.BeforeImage = r.BeforeImage
	p.AfterImage = r.AfterImage
	return nil
}
func (b *Bus) RejectReview() {
	if b.review == nil {
		return
	}
	b.CancelDrag()
	b.doc = b.review.before
	b.review = nil
	b.Events.Emit(Event{Kind: EvDocReplaced})
}
func (b *Bus) AcceptReview() error {
	r := b.review
	if r == nil || r.Status != "Needs review" {
		return fmt.Errorf("submit the changes for review first")
	}
	p := b.doc.NotePinByID(r.Pin)
	p.Done = true
	p.Status = "Done"
	after := *b.doc
	before := *r.before
	b.doc = r.before
	b.review = nil
	return b.Run(&requestCommand{before: &before, after: &after, label: fmt.Sprintf("AI request · Pin %d", r.Pin)})
}

type requestCommand struct {
	before, after *Document
	label         string
}

func (c *requestCommand) Name() string         { return c.label }
func (c *requestCommand) Do(d *Document) error { *d = *c.after; return nil }
func (c *requestCommand) Undo(d *Document)     { *d = *c.before }
func (c *requestCommand) UndoBytes() int {
	n := 0
	for _, b := range c.after.Bodies {
		if b.Mesh != nil {
			for _, p := range b.Mesh.PaintTable() {
				if p.Paint.Img != nil {
					n += len(p.Paint.Img.Pix)
				}
			}
		}
	}
	return n
}

func findFace(d *Document, s FaceScope) (*Body, int) {
	b := d.BodyByID(s.Body)
	if b != nil && b.Mesh != nil {
		for i, f := range b.Mesh.Faces {
			if f.ID == s.Face {
				return b, i
			}
		}
	}
	return nil, -1
}
func findPaint(d *Document, s FaceScope) *mesh.FacePaint {
	b, i := findFace(d, s)
	if b == nil {
		return nil
	}
	return b.Mesh.Faces[i].Paint
}
func faceThumbnail(d *Document, s FaceScope) []byte {
	p := findPaint(d, s)
	if p == nil || p.Img == nil {
		return nil
	}
	b, fi := findFace(d, s)
	rect := p.MaterialBounds(b.Mesh, fi).Sub(p.Off)
	out := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			out.SetRGBA(x, y, p.Img.RGBAAt(rect.Min.X+x*rect.Dx()/128, rect.Min.Y+y*rect.Dy()/128))
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, out)
	return buf.Bytes()
}
func (b *Bus) prepareCommand(cmd Command) (Command, error) {
	f, ok := cmd.(FaceCommand)
	if b.review != nil {
		if b.review.Status == "Needs review" {
			return nil, fmt.Errorf("accept or reject the pending request before editing")
		}
		if !ok {
			return nil, fmt.Errorf("pin scope is locked: this operation is not an isolated face edit")
		}
		body, face := f.TargetFace()
		allowed := false
		for _, s := range b.review.Scope {
			allowed = allowed || (s.Body == body && s.Face == face)
		}
		if !allowed {
			return nil, fmt.Errorf("pin scope rejected an edit outside its attached face; expand scope in the review panel first")
		}
	}
	if ok {
		body, face := f.TargetFace()
		p := findPaint(b.doc, FaceScope{body, face})
		if b.review != nil || (p != nil && len(p.Layers) > 0) {
			return &isolatedFaceCommand{inner: cmd, scope: FaceScope{body, face}}, nil
		}
	}
	return cmd, nil
}

type isolatedFaceCommand struct {
	inner         Command
	scope         FaceScope
	before, after *mesh.FacePaint
}

func (c *isolatedFaceCommand) Name() string { return c.inner.Name() }
func (c *isolatedFaceCommand) Events() []Event {
	return []Event{{Kind: EvBodyChanged, BodyID: c.scope.Body}}
}
func (c *isolatedFaceCommand) Do(d *Document) error {
	b, fi := findFace(d, c.scope)
	if b == nil {
		return fmt.Errorf("face is no longer available")
	}
	if c.after != nil {
		copyBody := *b
		copyBody.Mesh = b.Mesh.Clone()
		copyBody.Mesh.Faces[fi].Paint = c.after
		d.Bodies[d.bodyIndex(b.ID)] = &copyBody
		return nil
	}
	local := *d
	local.Bodies = append([]*Body(nil), d.Bodies...)
	work := *b
	work.Mesh = b.Mesh.Clone()
	p := mesh.ClonePaint(work.Mesh.Faces[fi].Paint)
	work.Mesh.Faces[fi].Paint = p
	local.Bodies[d.bodyIndex(b.ID)] = &work
	brush, isBrush := c.inner.(LayerBrush)
	layered := isBrush && brush.PaintsLayer() && p != nil && len(p.Layers) > 0
	var oldComposite *image.RGBA
	if layered {
		if p.ActiveLayer < 0 || p.ActiveLayer >= len(p.Layers) {
			return fmt.Errorf("select a paint layer")
		}
		l := &p.Layers[p.ActiveLayer]
		if !l.Visible {
			return fmt.Errorf("show this layer before painting")
		}
		oldComposite = p.Img
		if p.PaintMask {
			if l.Mask == nil {
				l.Mask = image.NewRGBA(p.Img.Bounds())
				for y := l.Mask.Rect.Min.Y; y < l.Mask.Rect.Max.Y; y++ {
					for x := l.Mask.Rect.Min.X; x < l.Mask.Rect.Max.X; x++ {
						l.Mask.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
					}
				}
			}
			p.Img = l.Mask
		} else {
			p.Img = l.Pixels
		}
	}
	if err := c.inner.Do(&local); err != nil {
		return err
	}
	// Commands may replace the face paint, but may not mutate geometry or siblings.
	if !reflect.DeepEqual(work.Mesh.Verts, b.Mesh.Verts) || len(work.Mesh.Faces) != len(b.Mesh.Faces) {
		return fmt.Errorf("face edit changed geometry")
	}
	for i := range work.Mesh.Faces {
		if i != fi && !reflect.DeepEqual(work.Mesh.Faces[i].Paint, b.Mesh.Faces[i].Paint) {
			return fmt.Errorf("face edit attempted to change a sibling")
		}
	}
	p = work.Mesh.Faces[fi].Paint
	if layered {
		l := &p.Layers[p.ActiveLayer]
		if p.PaintMask {
			l.Mask = p.Img
		} else {
			l.Pixels = p.Img
		}
		p.Img = oldComposite
		p.CompositeLayers()
		p.PBRStale = true
	}
	c.before = b.Mesh.Faces[fi].Paint
	c.after = p
	d.Bodies[d.bodyIndex(b.ID)] = &work
	return nil
}
func (c *isolatedFaceCommand) Undo(d *Document) {
	b, fi := findFace(d, c.scope)
	if b != nil {
		v := *b
		v.Mesh = b.Mesh.Clone()
		v.Mesh.Faces[fi].Paint = c.before
		d.Bodies[d.bodyIndex(b.ID)] = &v
	}
}
func (c *isolatedFaceCommand) UndoBytes() int {
	n := 0
	for _, p := range []*mesh.FacePaint{c.before, c.after} {
		if p != nil && p.Img != nil {
			n += len(p.Img.Pix)
		}
	}
	return n
}

func (b *Bus) OriginalDocument() *Document {
	if b.review != nil {
		return b.review.before
	}
	return b.doc
}
