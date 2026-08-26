package model

import (
	"fmt"

	"modeler/internal/geom"
	"modeler/internal/geom/csg"
	"modeler/internal/geom/extrude"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
)

// Push/pull (R11, SPEC-UX §12.5), the tool that does most of the work in a
// blocky ship.
//
// It is an extrude of a face's own outline, folded straight back into the body
// it came from: out adds material, in takes it away, and which one you get is
// decided by the direction you dragged rather than by a mode you had to pick
// first. Everything is built and validated before the body is touched.

// PushPull moves one face of a body along its normal.
type PushPull struct {
	Body uint32
	Face mesh.FaceUID
	// Distance is signed subunits along the face normal: positive adds
	// material, negative cuts into the body.
	Distance int64

	// Filled in by Do.
	body     *Body
	prevMesh *mesh.Mesh
	prevSeq  uint32
	emptied  bool
	at       int
	op       csg.Op
}

func (c *PushPull) Name() string {
	if c.Distance < 0 {
		return "Push face in"
	}
	return "Pull face out"
}

// Emptied reports that the push removed the last of the body.
func (c *PushPull) Emptied() bool { return c.emptied }

func (c *PushPull) Do(doc *Document) error {
	b := doc.BodyByID(c.Body)
	if b == nil || b.Mesh == nil {
		return fmt.Errorf("that body is no longer there")
	}
	fi := faceIndex(b.Mesh, c.Face)
	if fi < 0 {
		return fmt.Errorf("that face is no longer there")
	}
	if c.Distance == 0 {
		return fmt.Errorf("drag the arrow to move the face")
	}
	if b.Mesh.Faces[fi].NonPlanar || b.Mesh.Planarity(fi) > geom.PlanarDist {
		return fmt.Errorf("that face is not flat — push and pull need a flat face")
	}

	// The tool prism gets a body id of its own so its face identities can never
	// be mistaken for the target's: they travel into the result as lineage, and
	// lineage pointing at the wrong face is how paint ends up on the wrong
	// surface.
	toolID := doc.Seq.NextBody()
	frame := b.Mesh.FaceFrame(fi)
	region, err := FaceRegion(b.Mesh, fi, frame)
	if err != nil {
		return err
	}

	dir := extrude.Normal
	c.op = csg.Union
	if c.Distance < 0 {
		dir = extrude.Reverse
		c.op = csg.Subtract
	}
	depth := c.Distance
	if depth < 0 {
		depth = -depth
	}
	built, err := extrude.Build([]sketch2d.Region{region},
		extrude.Params{Frame: frame, Depth: depth, Dir: dir}, toolID)
	if err != nil {
		return err
	}

	res, err := csg.Boolean(c.op, b.Mesh, built.Mesh)
	if err != nil {
		return err
	}

	// Nothing below can fail.
	c.body, c.prevMesh, c.prevSeq = b, b.Mesh, b.FaceSeq
	c.at = doc.bodyIndex(b.ID)
	if res.Empty {
		c.emptied = true
		doc.Bodies = append(doc.Bodies[:c.at], doc.Bodies[c.at+1:]...)
		return nil
	}
	b.Mesh, b.FaceSeq = res.Mesh, res.NextFaceSeq
	return nil
}

func (c *PushPull) Undo(doc *Document) {
	if c.body == nil {
		return
	}
	if c.emptied {
		doc.Bodies = insertBodyAt(doc.Bodies, c.at, c.body)
		c.emptied = false
	}
	c.body.Mesh, c.body.FaceSeq = c.prevMesh, c.prevSeq
}

func (c *PushPull) Events() []Event {
	return []Event{{Kind: EvBodyChanged, BodyID: c.Body}}
}

// faceIndex finds a face by identity. Indices move whenever a body is edited;
// identities do not, which is why every reference the app keeps is an identity.
func faceIndex(m *mesh.Mesh, uid mesh.FaceUID) int {
	for i := range m.Faces {
		if m.Faces[i].ID == uid {
			return i
		}
	}
	return -1
}

// FaceRegion projects a face's loops into a frame and returns them as a sketch
// region, ready to extrude.
//
// A face's outer loop already runs counter-clockwise seen from outside and its
// holes run the other way, and the frame's normal is the face's own, so the
// windings arrive exactly as the region engine wants them. Nothing needs
// reversing; that it works out is the point of both conventions agreeing.
func FaceRegion(m *mesh.Mesh, fi int, frame geom.Frame) (sketch2d.Region, error) {
	loops := m.Faces[fi].Loops
	if len(loops) == 0 || len(loops[0]) < 3 {
		return sketch2d.Region{}, fmt.Errorf("that face has no outline to move")
	}
	project := func(loop []int) sketch2d.Loop {
		pts := make([]geom.Vec2i, 0, len(loop))
		for _, vi := range loop {
			uv := frame.ToLocal(m.Verts[vi])
			pts = append(pts, geom.Vec2i{
				X: geom.ToSubunits(uv.X),
				Y: geom.ToSubunits(uv.Y),
			})
		}
		return sketch2d.Loop{Pts: pts, Src: make([]sketch2d.Source, len(pts))}
	}
	r := sketch2d.Region{Outer: project(loops[0])}
	for _, h := range loops[1:] {
		r.Holes = append(r.Holes, project(h))
	}
	if r.Outer.Area2() <= 0 {
		return sketch2d.Region{}, fmt.Errorf("that face's outline has no area")
	}
	return r, nil
}
