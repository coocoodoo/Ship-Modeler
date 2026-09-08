package model

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// NotePin anchors a project note to a face triangle. Barycentric coordinates
// follow moves, rotations, scales and vertex edits without accumulating drift.
type NotePin struct {
	Status      string       `json:"status,omitempty"`
	Changes     string       `json:"changes,omitempty"`
	BeforeImage []byte       `json:"beforeImage,omitempty"`
	AfterImage  []byte       `json:"afterImage,omitempty"`
	ID          uint32       `json:"id"`
	Body        uint32       `json:"body"`
	Face        mesh.FaceUID `json:"face"`
	Vertices    [3]int       `json:"vertices"`
	Weights     [3]float64   `json:"weights"`
	At          geom.Vec3    `json:"at"`
	Text        string       `json:"text"`
	Done        bool         `json:"done"`
}

func CleanNoteText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("write a note first")
	}
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 {
		return "", fmt.Errorf("use up to 2,000 characters")
	}
	return text, nil
}

func (d *Document) NotePinByID(id uint32) *NotePin {
	for i := range d.NotePins {
		if d.NotePins[i].ID == id {
			return &d.NotePins[i]
		}
	}
	return nil
}

// Position refuses stale face references instead of attaching a request to an
// unrelated replacement face. The original location remains available.
func (p NotePin) Position(d *Document) (geom.Vec3, bool) {
	b := d.BodyByID(p.Body)
	if b == nil || b.Mesh == nil {
		return p.At, false
	}
	fi := faceIndex(b.Mesh, p.Face)
	if fi < 0 {
		return p.At, false
	}
	var at geom.Vec3
	for k, vi := range p.Vertices {
		if vi < 0 || vi >= len(b.Mesh.Verts) {
			return p.At, false
		}
		found := false
		for _, loop := range b.Mesh.Faces[fi].Loops {
			for _, v := range loop {
				if v == vi {
					found = true
				}
			}
		}
		if !found {
			return p.At, false
		}
		at = at.Add(b.Mesh.Verts[vi].Mul(p.Weights[k]))
	}
	return at, true
}

func AnchorNotePin(b *Body, fi int, at geom.Vec3) (NotePin, error) {
	if b == nil || b.Mesh == nil || fi < 0 || fi >= len(b.Mesh.Faces) {
		return NotePin{}, fmt.Errorf("choose a model face")
	}
	for _, tr := range b.Mesh.FaceTris(fi) {
		a, c, e := b.Mesh.Verts[tr.A], b.Mesh.Verts[tr.B], b.Mesh.Verts[tr.C]
		u, v, w := c.Sub(a), e.Sub(a), at.Sub(a)
		uu, uv, vv := u.Dot(u), u.Dot(v), v.Dot(v)
		den := uu*vv - uv*uv
		if math.Abs(den) < 1e-16 {
			continue
		}
		s, t := (vv*w.Dot(u)-uv*w.Dot(v))/den, (uu*w.Dot(v)-uv*w.Dot(u))/den
		if s < -1e-6 || t < -1e-6 || s+t > 1+1e-6 || math.Abs(w.Dot(u.Cross(v).Normalize())) > 1e-5 {
			continue
		}
		return NotePin{Body: b.ID, Face: b.Mesh.Faces[fi].ID, Vertices: [3]int{tr.A, tr.B, tr.C}, Weights: [3]float64{1 - s - t, s, t}, At: at}, nil
	}
	return NotePin{}, fmt.Errorf("place the pin inside the face")
}

// SetNotePin adds with ID zero or edits by stable ID. Redo keeps the same ID.
type SetNotePin struct {
	Pin    NotePin
	before NotePin
	index  int
	added  bool
}

func (c *SetNotePin) Name() string { return "Save note pin" }
func (c *SetNotePin) Do(d *Document) error {
	text, err := CleanNoteText(c.Pin.Text)
	if err != nil {
		return err
	}
	c.Pin.Text = text
	if c.Pin.ID == 0 || c.added {
		if _, ok := c.Pin.Position(d); !ok {
			return fmt.Errorf("that pin's face is no longer there")
		}
		if c.Pin.ID == 0 {
			for _, p := range d.NotePins {
				if p.ID > d.Seq.NotePin {
					d.Seq.NotePin = p.ID
				}
			}
			d.Seq.NotePin++
			c.Pin.ID = d.Seq.NotePin
		}
		c.added = true
		// Redo after Clear all pins must restore the new numbering sequence too.
		d.Seq.NotePin = max(d.Seq.NotePin, c.Pin.ID)
		c.index = len(d.NotePins)
		d.NotePins = append(d.NotePins, c.Pin)
		return nil
	}
	for i, p := range d.NotePins {
		if p.ID == c.Pin.ID {
			c.index = i
			c.before = p
			d.NotePins[i] = c.Pin
			return nil
		}
	}
	return fmt.Errorf("that note pin is no longer there")
}
func (c *SetNotePin) Undo(d *Document) {
	if c.added {
		d.NotePins = append(d.NotePins[:c.index], d.NotePins[c.index+1:]...)
	} else {
		d.NotePins[c.index] = c.before
	}
}
func (c *SetNotePin) Events() []Event { return []Event{{Kind: EvMarkersChanged}} }

type DeleteNotePin struct {
	ID     uint32
	before NotePin
	index  int
}

func (c *DeleteNotePin) Name() string { return "Delete note pin" }
func (c *DeleteNotePin) Do(d *Document) error {
	for i, p := range d.NotePins {
		if p.ID == c.ID {
			c.index = i
			c.before = p
			d.NotePins = append(d.NotePins[:i], d.NotePins[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("that note pin is no longer there")
}
func (c *DeleteNotePin) Undo(d *Document) {
	d.NotePins = append(d.NotePins, NotePin{})
	copy(d.NotePins[c.index+1:], d.NotePins[c.index:])
	d.NotePins[c.index] = c.before
}
func (c *DeleteNotePin) Events() []Event { return []Event{{Kind: EvMarkersChanged}} }

// ClearNotePins starts a fresh numbering sequence, as one undoable operation.
type ClearNotePins struct {
	before        []NotePin
	beforeCounter uint32
}

func (c *ClearNotePins) Name() string { return "Clear all pins" }
func (c *ClearNotePins) Do(d *Document) error {
	c.before = append([]NotePin(nil), d.NotePins...)
	c.beforeCounter = d.Seq.NotePin
	d.NotePins = nil
	d.Seq.NotePin = 0
	return nil
}
func (c *ClearNotePins) Undo(d *Document) {
	d.NotePins = append([]NotePin(nil), c.before...)
	d.Seq.NotePin = c.beforeCounter
}
func (c *ClearNotePins) Events() []Event { return []Event{{Kind: EvMarkersChanged}} }
