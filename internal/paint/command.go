package paint

import (
	"bytes"
	"fmt"
	"image"
	"image/color"

	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// The document commands that edit pixels (SPEC-DATA §3.3).
//
// They live here rather than in model because of the layering: imports run
// geom <- model <- paint (PLAN §4), so the brush and the mapping sit below the
// document and the command that drives them sits above it. They satisfy
// model.Command and go through the same bus as every other edit, which is what
// gives paint undo without a special case anywhere in the history.

// StrokeFace is one mouse-down's worth of painting on one face.
//
// The whole stroke is a single command rather than one per dab: a drag replaces
// the pending command with a longer version of itself each frame and commits
// once on release, so the history holds "a stroke" and not four hundred texels.
type StrokeFace struct {
	Body uint32
	Face mesh.FaceUID

	// Tool is Pencil, Eraser or Fill. Pick never reaches here — sampling a
	// colour changes the palette, which is settings, not document state.
	Tool  Tool
	Color color.RGBA
	Size  int

	// Res is the chip a first stroke allocates at. It is ignored once the face
	// has a texture, because texel density never changes implicitly
	// (SPEC-GEOMETRY §8.2).
	Res int

	// Points is the texel path in the order the pointer visited it. Fill uses
	// the first point as its seed.
	Points []image.Point

	// Filled in by Do.
	paint     *mesh.FacePaint
	allocated bool
	rect      image.Rectangle
	before    *image.RGBA
	after     *image.RGBA
}

func (c *StrokeFace) Name() string {
	switch c.Tool {
	case ToolEraser:
		return "Erase"
	case ToolFill:
		return "Fill face"
	default:
		return "Paint"
	}
}

// Painted is the texture the stroke wrote into, valid after a successful Do.
func (c *StrokeFace) Painted() *mesh.FacePaint { return c.paint }

// LayoutChanged reports that the stroke created the face's texture. The body's
// UVs and its atlas slot did not exist when its render form was built, so that
// form has to be rebuilt rather than patched.
func (c *StrokeFace) LayoutChanged() bool { return c.allocated }

// DirtyRect is the texel rectangle the stroke actually wrote: what the renderer
// re-uploads, and what undo puts back.
func (c *StrokeFace) DirtyRect() image.Rectangle { return c.rect }

// UndoBytes is the stroke's cost against the history's memory budget
// (SPEC-DATA §3.3). Both snapshots count, because both are held.
func (c *StrokeFace) UndoBytes() int {
	n := 0
	if c.before != nil {
		n += len(c.before.Pix)
	}
	if c.after != nil {
		n += len(c.after.Pix)
	}
	return n
}

func (c *StrokeFace) Do(doc *model.Document) error {
	m, fi, err := resolveFace(doc, c.Body, c.Face)
	if err != nil {
		return err
	}

	// A redo replays from the snapshot rather than from the brush: the picture
	// is already known, and putting it back is both exact and cheap.
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
		if c.Tool == ToolEraser {
			return fmt.Errorf("there is no paint on that face to rub out")
		}
		res := c.Res
		if res == 0 {
			res = DefaultRes
		}
		if p, err = Allocate(m, fi, res); err != nil {
			return err
		}
		allocated = true
	}

	// Fragments of a cut face share one texture on purpose (SPEC-GEOMETRY
	// §8.4), so the stroke writes into the shared image and every sibling reads
	// the result. That is the contract rather than an oversight: the mapping is
	// anchored in the world, so a texel belongs to exactly one place on exactly
	// one fragment however many faces are reading from the picture.

	planned := c.plannedRect(m, fi, p)
	if planned.Empty() {
		return fmt.Errorf("that stroke did not land on the face")
	}
	// Snapshot before a single texel moves: this is the only moment the old
	// picture still exists.
	prior := SubImage(p, planned)

	wrote := c.apply(m, fi, p)
	if wrote.Empty() {
		return fmt.Errorf("that stroke did not change anything")
	}
	before := crop(prior, planned, wrote)
	after := SubImage(p, wrote)
	if bytes.Equal(before.Pix, after.Pix) {
		// A fill onto the colour already there, or an eraser over texels that
		// were already bare. Refusing keeps a step that undoes nothing out of
		// the history — and since nothing was written, there is nothing to put
		// back either.
		return fmt.Errorf("that stroke did not change anything")
	}

	// Nothing below can fail.
	if allocated {
		m.Faces[fi].Paint = p
	}
	c.paint, c.allocated = p, allocated
	c.rect, c.before, c.after = wrote, before, after
	return nil
}

func (c *StrokeFace) Undo(doc *model.Document) {
	if c.paint == nil {
		return
	}
	if c.allocated {
		// The face was bare before, and bare is not "an image full of
		// transparent texels" — it is no image at all.
		if m, fi, err := resolveFace(doc, c.Body, c.Face); err == nil {
			m.Faces[fi].Paint = nil
		}
		return
	}
	Blit(c.paint, c.rect, c.before)
}

func (c *StrokeFace) Events() []model.Event {
	if c.allocated {
		// A new texture means a new atlas layout and new UVs, so the body's
		// render form has to be rebuilt rather than patched.
		return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
	}
	return []model.Event{{
		Kind: model.EvBodyPainted, BodyID: c.Body, Paint: c.paint, Rect: c.rect,
	}}
}

// plannedRect is every texel the stroke could possibly reach: the path's bounds
// grown by the brush, or the whole face for a fill.
//
// Unioning the dabs at the samples is enough even though the brush also dabs
// between them, because every interpolated point lies inside the box its two
// samples span.
func (c *StrokeFace) plannedRect(m *mesh.Mesh, fi int, p *mesh.FacePaint) image.Rectangle {
	if c.Tool == ToolFill {
		return FaceRect(m, fi, p)
	}
	size := c.Size
	if size < 1 {
		size = 1
	}
	r := image.Rectangle{}
	for _, t := range c.Points {
		dab := image.Rectangle{Min: t, Max: t.Add(image.Point{X: size, Y: size})}
		if r.Empty() {
			r = dab
			continue
		}
		r = r.Union(dab)
	}
	return r
}

// apply runs the brush and returns the rectangle it wrote.
func (c *StrokeFace) apply(m *mesh.Mesh, fi int, p *mesh.FacePaint) image.Rectangle {
	if len(c.Points) == 0 {
		return image.Rectangle{}
	}
	if c.Tool == ToolFill {
		// The fill is bounded by the face rather than by the image: the image
		// carries a margin that is not on the face at all, and paint there
		// would be paint on nothing.
		region := FaceRect(m, fi, p)
		if Fill(p, region, c.Points[0], c.Color) == 0 {
			return image.Rectangle{}
		}
		return region
	}
	b := Brush{Color: c.Color, Size: c.Size, Erase: c.Tool == ToolEraser}
	if b.Size < 1 {
		b.Size = 1
	}
	dirty := image.Rectangle{}
	prev := c.Points[0]
	for _, t := range c.Points {
		if r := Stroke(p, b, prev, t); !r.Empty() {
			if dirty.Empty() {
				dirty = r
			} else {
				dirty = dirty.Union(r)
			}
		}
		prev = t
	}
	return dirty
}

// ResampleFace rebuilds a face's texture at a different resolution, carrying
// the picture over by nearest sampling through the world (SPEC-UX §13.2).
type ResampleFace struct {
	Body uint32
	Face mesh.FaceUID
	Res  int

	prev *mesh.FacePaint
	next *mesh.FacePaint
}

func (c *ResampleFace) Name() string { return fmt.Sprintf("Resample face to %d px", c.Res) }

func (c *ResampleFace) Do(doc *model.Document) error {
	m, fi, err := resolveFace(doc, c.Body, c.Face)
	if err != nil {
		return err
	}
	prev := m.Faces[fi].Paint
	if prev == nil {
		return fmt.Errorf("there is no paint on that face to resample")
	}
	if prev.Res == c.Res {
		return fmt.Errorf("that face is already %d px", c.Res)
	}
	next := c.next
	if next == nil {
		if next, err = Resample(m, fi, prev, c.Res); err != nil {
			return err
		}
	}
	// Nothing below can fail.
	c.prev, c.next = prev, next
	m.Faces[fi].Paint = next
	return nil
}

func (c *ResampleFace) Undo(doc *model.Document) {
	if m, fi, err := resolveFace(doc, c.Body, c.Face); err == nil {
		m.Faces[fi].Paint = c.prev
	}
}

func (c *ResampleFace) Events() []model.Event {
	return []model.Event{{Kind: model.EvBodyChanged, BodyID: c.Body}}
}

// resolveFace finds a face by body and identity, the way every command that
// refers to one must: indices move when a body is edited, identities do not.
func resolveFace(doc *model.Document, bodyID uint32, uid mesh.FaceUID) (*mesh.Mesh, int, error) {
	b := doc.BodyByID(bodyID)
	if b == nil || b.Mesh == nil {
		return nil, -1, fmt.Errorf("that body is no longer there")
	}
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].ID == uid {
			return b.Mesh, i, nil
		}
	}
	return nil, -1, fmt.Errorf("that face is no longer there")
}

// crop cuts the want rectangle out of a snapshot covering have, which is how a
// snapshot of the region a stroke might touch becomes a snapshot of the region
// it did. want is always inside have.
func crop(src *image.RGBA, have, want image.Rectangle) *image.RGBA {
	if src == nil {
		return nil
	}
	if have == want {
		return src
	}
	out := image.NewRGBA(image.Rect(0, 0, want.Dx(), want.Dy()))
	for y := want.Min.Y; y < want.Max.Y; y++ {
		for x := want.Min.X; x < want.Max.X; x++ {
			if !(image.Point{X: x, Y: y}).In(have) {
				continue
			}
			out.SetRGBA(x-want.Min.X, y-want.Min.Y,
				src.RGBAAt(x-have.Min.X, y-have.Min.Y))
		}
	}
	return out
}
