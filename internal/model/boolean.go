package model

import (
	"fmt"

	"modeler/internal/geom/csg"
	"modeler/internal/geom/mesh"
)

// The boolean command (R12, SPEC-UX §11, SPEC-GEOMETRY §6.4).
//
// Everything is computed before anything is written. A boolean that fails
// leaves the document byte for byte as it was, which is the whole of the
// promise in SPEC-UX §11.3: never partial results, never a corrupt mesh.

// Boolean combines existing bodies.
type Boolean struct {
	Op     csg.Op
	Target uint32
	Tools  []uint32
	// KeepTools leaves the tool bodies in the document instead of consuming
	// them (SPEC-UX §11).
	KeepTools bool

	// Everything below is filled in by Do so Undo can put it all back.
	done       bool
	prevMesh   *mesh.Mesh
	prevSeq    uint32
	emptied    bool
	targetAt   int
	target     *Body
	consumed   []*Body
	consumedAt []int
	summary    string
}

func (c *Boolean) Name() string {
	if c.summary != "" {
		return c.summary
	}
	return c.Op.String()
}

// Summary is the sentence the success toast shows.
func (c *Boolean) Summary() string { return c.summary }

// Emptied reports that the operation legally produced nothing and the target
// body was removed (SPEC-GEOMETRY §6.4).
func (c *Boolean) Emptied() bool { return c.emptied }

func (c *Boolean) Do(doc *Document) error {
	target := doc.BodyByID(c.Target)
	if target == nil || target.Mesh == nil {
		return fmt.Errorf("the body to keep is no longer there")
	}
	tools := make([]*mesh.Mesh, 0, len(c.Tools))
	names := make([]string, 0, len(c.Tools))
	bodies := make([]*Body, 0, len(c.Tools))
	for _, id := range c.Tools {
		b := doc.BodyByID(id)
		if b == nil || b.Mesh == nil {
			return fmt.Errorf("one of the bodies is no longer there")
		}
		if b.ID == target.ID {
			return fmt.Errorf("a body cannot be combined with itself")
		}
		tools = append(tools, b.Mesh)
		names = append(names, b.Name)
		bodies = append(bodies, b)
	}
	if len(tools) == 0 {
		return fmt.Errorf("pick at least one more body")
	}

	res, err := csg.Boolean(c.Op, target.Mesh, tools...)
	if err != nil {
		return err
	}

	// Past this line nothing can fail.
	c.prevMesh, c.prevSeq = target.Mesh, target.FaceSeq
	c.target, c.targetAt = target, doc.bodyIndex(target.ID)
	c.summary = summarise(c.Op, target.Name, names, res.Empty)

	if res.Empty {
		c.emptied = true
		doc.Bodies = append(doc.Bodies[:c.targetAt], doc.Bodies[c.targetAt+1:]...)
	} else {
		target.Mesh = res.Mesh
		target.FaceSeq = res.NextFaceSeq
	}

	if !c.KeepTools {
		for _, b := range bodies {
			if i := doc.bodyIndex(b.ID); i >= 0 {
				c.consumed = append(c.consumed, b)
				c.consumedAt = append(c.consumedAt, i)
				doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
			}
		}
	}
	c.done = true
	return nil
}

func (c *Boolean) Undo(doc *Document) {
	if !c.done {
		return
	}
	// Tools go back first, at the positions they came from, so the tree reads
	// exactly as it did before.
	for i := len(c.consumed) - 1; i >= 0; i-- {
		doc.Bodies = insertBodyAt(doc.Bodies, c.consumedAt[i], c.consumed[i])
	}
	if c.emptied {
		doc.Bodies = insertBodyAt(doc.Bodies, c.targetAt, c.target)
	}
	c.target.Mesh, c.target.FaceSeq = c.prevMesh, c.prevSeq
}

func (c *Boolean) Events() []Event {
	out := []Event{{Kind: EvBodyChanged, BodyID: c.Target}}
	for _, id := range c.Tools {
		out = append(out, Event{Kind: EvBodyRemoved, BodyID: id})
	}
	return out
}

func insertBodyAt(bodies []*Body, at int, b *Body) []*Body {
	if at < 0 || at > len(bodies) {
		at = len(bodies)
	}
	bodies = append(bodies, nil)
	copy(bodies[at+1:], bodies[at:])
	bodies[at] = b
	return bodies
}

// summarise is the sentence SPEC-UX §11 wants after a successful boolean.
func summarise(op csg.Op, target string, tools []string, empty bool) string {
	// Only glyphs the UI's font atlas actually carries (D-11). A true minus
	// sign and an intersection symbol would both come out as missing-glyph
	// boxes, which is a worse way to say "subtract" than the word already
	// leading the sentence.
	sep := " + "
	switch op {
	case csg.Subtract:
		sep = " – "
	case csg.Intersect:
		sep = " with "
	}
	list := target
	for _, t := range tools {
		list += sep + t
	}
	if empty {
		return fmt.Sprintf("%s: %s → nothing left", op, list)
	}
	return fmt.Sprintf("%s: %s → %s", op, list, target)
}
