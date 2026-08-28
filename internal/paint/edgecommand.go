package paint

import (
	"bytes"
	"fmt"
	"image"
	"image/color"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// StrokeEdges bakes a line along a set of edges into the faces that meet them
// (the user's request, 2026-08-27).
//
// One command rather than one per face, because one press is one thing the
// user did: an edge line that took four undos to remove — two faces per edge,
// and more for a run of them — would be a worse tool than no tool.
//
// It writes into exactly the same per-face pictures the brush does, so the
// result is ordinary paint from the moment it lands: it saves, exports, cuts
// with the body and can be painted over.
type StrokeEdges struct {
	Body uint32
	// Edges index into the body mesh's topology.
	Edges []int
	Color color.RGBA
	// Size is the band's width in texels, on each face that meets the edge.
	Size int
	// Res is the resolution a face is given if it has no picture yet.
	Res int

	// Filled in by Do, one entry per face actually touched.
	touched []edgeFacePaint
	// allocated is true when any face was given its first picture, which means
	// the body's UVs changed and its render form has to be rebuilt.
	allocated bool
	// texels records how wide one texel was on each face it wrote, so the app
	// can say what the chosen number of pixels came to in real size.
	texels []float64
}

// DefaultEdgeWidth is how many texels a fresh session paints: one, the
// thinnest a face can draw, because a panel seam is what this is usually for.
const DefaultEdgeWidth = 1

// WorldWidth is the thinnest and thickest the band actually came out, in world
// units, across the faces it touched.
//
// A texel is a face's longest side over its resolution, so the same number of
// pixels is a different real thickness on a big face than on a small one. The
// app shows this so "why is one pixel still fat" has an answer on screen: the
// face's resolution, not the tool.
func (c *StrokeEdges) WorldWidth() (lo, hi float64) {
	for _, t := range c.texels {
		w := t * float64(maxInt(c.Size, 1))
		if lo == 0 || w < lo {
			lo = w
		}
		if w > hi {
			hi = w
		}
	}
	return lo, hi
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// edgeFacePaint is one face's share of the command, and what undo needs to put
// it back.
type edgeFacePaint struct {
	// uid rather than an index: a face's index moves when the body is edited,
	// and its identity does not.
	uid       mesh.FaceUID
	paint     *mesh.FacePaint
	allocated bool
	rect      image.Rectangle
	before    *image.RGBA
	after     *image.RGBA
}

func (c *StrokeEdges) Name() string {
	if len(c.Edges) == 1 {
		return "Paint 1 edge"
	}
	return fmt.Sprintf("Paint %d edges", len(c.Edges))
}

// Painted lists the pictures the command wrote into, for the renderer.
func (c *StrokeEdges) Painted() []*mesh.FacePaint {
	out := make([]*mesh.FacePaint, 0, len(c.touched))
	for i := range c.touched {
		out = append(out, c.touched[i].paint)
	}
	return out
}

// LayoutChanged reports that a face was given its first picture, so the body's
// atlas and UVs are new and its render form cannot simply be patched.
func (c *StrokeEdges) LayoutChanged() bool { return c.allocated }

// UndoBytes is the command's cost against the history's memory budget
// (SPEC-DATA §3.3).
func (c *StrokeEdges) UndoBytes() int {
	n := 0
	for i := range c.touched {
		if c.touched[i].before != nil {
			n += len(c.touched[i].before.Pix)
		}
		if c.touched[i].after != nil {
			n += len(c.touched[i].after.Pix)
		}
	}
	return n
}

func (c *StrokeEdges) Do(doc *model.Document) error {
	b := doc.BodyByID(c.Body)
	if b == nil || b.Mesh == nil {
		return fmt.Errorf("that body is no longer there")
	}
	if len(c.Edges) == 0 {
		return fmt.Errorf("no edges were chosen")
	}
	m := b.Mesh

	// A redo replays from the snapshots: the pictures are already known, and
	// putting them back is exact and cheap.
	if len(c.touched) > 0 && c.touched[0].after != nil {
		for i := range c.touched {
			t := &c.touched[i]
			if t.allocated {
				if fi, ok := faceByUID(m, t.uid); ok {
					m.Faces[fi].Paint = t.paint
				}
			}
			Blit(t.paint, t.rect, t.after)
		}
		return nil
	}

	// Which faces take which segments. Grouping first means a face shared by
	// several chosen edges is snapshotted once and written once.
	type seg struct{ a, b geom.Vec3 }
	work := map[int][]seg{}
	order := []int{}
	for _, e := range c.Edges {
		wa, wb, ok := EdgeEndsOf(m, e)
		if !ok {
			return fmt.Errorf("edge %d is no longer there", e)
		}
		for _, fi := range FacesOfEdge(m, e) {
			if _, seen := work[fi]; !seen {
				order = append(order, fi)
			}
			work[fi] = append(work[fi], seg{wa, wb})
		}
	}
	if len(work) == 0 {
		return fmt.Errorf("those edges touch no faces")
	}

	size := c.Size
	if size < 1 {
		size = DefaultEdgeWidth
	}
	brush := Brush{Color: c.Color, Size: size, Under: b.Color}
	c.texels = c.texels[:0]

	var touched []edgeFacePaint
	anyAllocated := false
	for _, fi := range order {
		p := m.Faces[fi].Paint
		fresh := false
		if p == nil {
			res := c.Res
			if res == 0 {
				res = DefaultRes
			}
			var err error
			if p, err = Allocate(m, fi, res); err != nil {
				return err
			}
			fresh = true
		}

		// Every texel the bands could reach, before a single one moves: this
		// is the only moment the old picture still exists.
		planned := image.Rectangle{}
		for _, s := range work[fi] {
			planned = union(planned, EdgeBandBounds(m, fi, p, size, s.a, s.b))
		}
		if planned.Empty() {
			continue
		}
		prior := SubImage(p, planned)

		wrote := image.Rectangle{}
		for _, s := range work[fi] {
			wrote = union(wrote, EdgeBand(m, fi, p, brush, s.a, s.b))
		}
		if wrote.Empty() {
			continue
		}
		before := crop(prior, planned, wrote)
		after := SubImage(p, wrote)
		if !fresh && bytes.Equal(before.Pix, after.Pix) {
			continue // that face already looked like this
		}

		if fresh {
			m.Faces[fi].Paint = p
			anyAllocated = true
		}
		touched = append(touched, edgeFacePaint{
			uid: m.Faces[fi].ID, paint: p, allocated: fresh,
			rect: wrote, before: before, after: after,
		})
		c.texels = append(c.texels, p.Texel)
	}
	if len(touched) == 0 {
		return fmt.Errorf("that changed nothing — the edges are already this colour")
	}

	c.touched, c.allocated = touched, anyAllocated
	return nil
}

func (c *StrokeEdges) Undo(doc *model.Document) {
	b := doc.BodyByID(c.Body)
	if b == nil || b.Mesh == nil {
		return
	}
	for i := range c.touched {
		t := &c.touched[i]
		if t.allocated {
			// The face was bare before, and bare is no picture at all rather
			// than a picture full of nothing.
			if fi, ok := faceByUID(b.Mesh, t.uid); ok {
				b.Mesh.Faces[fi].Paint = nil
			}
			continue
		}
		Blit(t.paint, t.rect, t.before)
	}
}

func (c *StrokeEdges) Events() []model.Event {
	// A face that has just been given its first picture has new UVs and a new
	// atlas slot, so the body's render form has to be rebuilt rather than
	// patched. Otherwise each touched face re-uploads only the band it grew.
	if c.allocated {
		return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
	}
	out := make([]model.Event, 0, len(c.touched))
	for i := range c.touched {
		out = append(out, model.Event{
			Kind: model.EvBodyPainted, BodyID: c.Body,
			Paint: c.touched[i].paint, Rect: c.touched[i].rect,
		})
	}
	return out
}

// faceByUID finds a face by identity, which is what survives an edit.
func faceByUID(m *mesh.Mesh, uid mesh.FaceUID) (int, bool) {
	for i := range m.Faces {
		if m.Faces[i].ID == uid {
			return i, true
		}
	}
	return 0, false
}

// EdgeBandBounds is every texel an edge band could write, without writing any
// of them. The command needs it to snapshot before it paints.
func EdgeBandBounds(m *mesh.Mesh, fi int, p *mesh.FacePaint, size int, worldA, worldB geom.Vec3) image.Rectangle {
	if m == nil || p == nil {
		return image.Rectangle{}
	}
	if size < 1 {
		size = 1
	}
	ta, tb := edgeBandLine(m, fi, p, size, worldA, worldB)
	box := image.Rectangle{
		Min: image.Point{X: min(ta.X, tb.X), Y: min(ta.Y, tb.Y)},
		Max: image.Point{X: max(ta.X, tb.X) + size, Y: max(ta.Y, tb.Y) + size},
	}
	return box.Intersect(FaceRect(m, fi, p))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
