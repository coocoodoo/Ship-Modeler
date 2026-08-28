package paint

import (
	"bytes"
	"fmt"
	"image"

	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// Stamping tiles onto faces (Tile_paint.md TP1).
//
// A stamp is a batch of texel writes at an offset — all the hard mapping work
// already exists in mapping.go. What this file owns is the contract around
// the batch: the alpha threshold, the clip, and the command that makes a
// trail of stamps one undo step.

// StampAlphaThreshold is where a tile pixel counts as paint. At or above it
// the pixel lands opaque, like every brush; below it the pixel leaves the
// surface exactly as it was. A stamp never erases — the eraser erases.
const StampAlphaThreshold = 128

// StampRect writes a tile's pixels with its min corner at a texel, clipped to
// a rectangle, and returns the exact rect of texels it changed.
func StampRect(p *mesh.FacePaint, tile *image.RGBA, at image.Point, clip image.Rectangle) image.Rectangle {
	if p == nil || tile == nil {
		return image.Rectangle{}
	}
	b := tile.Bounds()
	dirty := image.Rectangle{}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := tile.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A < StampAlphaThreshold {
				continue
			}
			t := image.Point{X: at.X + x, Y: at.Y + y}
			if !t.In(clip) {
				continue
			}
			c.A = 255
			if Set(p, t, c) {
				dirty = union(dirty, oneTexel(t))
			}
		}
	}
	return dirty
}

// StampFace is one mouse-down's worth of stamping on one face — the tile
// (already oriented) placed at each cell in Cells. Like a stroke, a drag
// replaces the pending command with a longer version of itself each frame and
// commits once, so the history holds "a trail" and not one entry per cell
// (SPEC-DATA §3.2, §3.3).
type StampFace struct {
	Body uint32
	Face mesh.FaceUID

	// Res is the chip a first stamp allocates at, ignored once the face has a
	// texture (SPEC-GEOMETRY §8.2).
	Res int

	// Tile is the oriented pixels, 1 tile pixel = 1 texel, never scaled.
	Tile *image.RGBA
	// Cells are the min corners the tile lands at, in the order stamped.
	Cells []image.Point

	// Filled in by Do.
	paint     *mesh.FacePaint
	allocated bool
	rect      image.Rectangle
	before    *image.RGBA
	after     *image.RGBA
}

func (c *StampFace) Name() string {
	if len(c.Cells) > 1 {
		return "Stamp tiles"
	}
	return "Stamp tile"
}

// Painted is the texture the stamp wrote into, valid after a successful Do.
func (c *StampFace) Painted() *mesh.FacePaint { return c.paint }

// LayoutChanged reports that the stamp created the face's texture, which
// means the body's render form rebuilds rather than patches.
func (c *StampFace) LayoutChanged() bool { return c.allocated }

// DirtyRect is the texel rectangle the stamp actually wrote.
func (c *StampFace) DirtyRect() image.Rectangle { return c.rect }

// UndoBytes is the stamp's cost against the paint-undo budget.
func (c *StampFace) UndoBytes() int {
	n := 0
	if c.before != nil {
		n += len(c.before.Pix)
	}
	if c.after != nil {
		n += len(c.after.Pix)
	}
	return n
}

func (c *StampFace) Do(doc *model.Document) error {
	m, fi, err := resolveFace(doc, c.Body, c.Face)
	if err != nil {
		return err
	}
	if c.Tile == nil || c.Tile.Bounds().Empty() || len(c.Cells) == 0 {
		return fmt.Errorf("there is no tile to stamp")
	}

	// A redo replays from the snapshot: the picture is already known.
	if c.after != nil && c.paint != nil {
		if c.allocated {
			m.Faces[fi].Paint = c.paint
		}
		Blit(c.paint, c.rect, c.after)
		return nil
	}

	p := m.Faces[fi].Paint
	allocated := false
	if p == nil {
		res := c.Res
		if res == 0 {
			res = DefaultRes
		}
		if p, err = Allocate(m, fi, res); err != nil {
			return err
		}
		allocated = true
	}

	// Everything the trail could reach: the cells' bounds grown by the tile,
	// clipped to the face — stamping clips to FaceRect and lets the renderer
	// clip the polygon (V-135's rule).
	face := FaceRect(m, fi, p)
	tb := c.Tile.Bounds()
	planned := image.Rectangle{}
	for _, cell := range c.Cells {
		r := image.Rectangle{Min: cell, Max: cell.Add(image.Point{X: tb.Dx(), Y: tb.Dy()})}
		planned = union(planned, r)
	}
	planned = planned.Intersect(face)
	if planned.Empty() {
		return fmt.Errorf("that stamp did not land on the face")
	}
	prior := SubImage(p, planned)

	wrote := image.Rectangle{}
	for _, cell := range c.Cells {
		wrote = union(wrote, StampRect(p, c.Tile, cell, face))
	}
	if wrote.Empty() {
		return fmt.Errorf("that stamp did not change anything")
	}
	before := crop(prior, planned, wrote)
	after := SubImage(p, wrote)
	if bytes.Equal(before.Pix, after.Pix) {
		return fmt.Errorf("that stamp did not change anything")
	}

	// Nothing below can fail.
	if allocated {
		m.Faces[fi].Paint = p
	}
	c.paint, c.allocated = p, allocated
	c.rect, c.before, c.after = wrote, before, after
	return nil
}

func (c *StampFace) Undo(doc *model.Document) {
	if c.paint == nil {
		return
	}
	if c.allocated {
		// Bare before means no image at all, not an image of nothing.
		if m, fi, err := resolveFace(doc, c.Body, c.Face); err == nil {
			m.Faces[fi].Paint = nil
		}
		return
	}
	Blit(c.paint, c.rect, c.before)
}

func (c *StampFace) Events() []model.Event {
	if c.allocated {
		return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
	}
	return []model.Event{{
		Kind: model.EvBodyPainted, BodyID: c.Body, Paint: c.paint, Rect: c.rect,
	}}
}
