package render

import (
	"image"
	"image/color"
	"sort"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// A body's painted faces packed into one texture (SPEC-GEOMETRY §8.1).
//
// One atlas per body rather than one texture per face, because a body is one
// rl.Mesh drawn in one DrawMesh call: per-face textures would mean splitting
// every painted body into as many meshes as it has painted faces, and rebuilding
// that split after every boolean. The atlas is keyed by the FacePaint pointer,
// not by the face, which is what makes the shared-paint contract of
// SPEC-GEOMETRY §8.4 fall out for free: fragments of a cut face reference one
// picture, so they map into one region of the atlas and stay in step.

const (
	// AtlasPadding is the gap left between packed textures so nearest sampling
	// at a texture's edge can never reach into its neighbour.
	AtlasPadding = 1
	// MaxAtlasSize is the largest atlas we will ask a GPU for.
	MaxAtlasSize = 8192
)

// paintSlot is where one face texture sits in its body's atlas.
type paintSlot struct {
	// Origin is the atlas pixel holding the texture's own pixel (0,0).
	Origin image.Point
	// Texels is the texel rectangle the texture covered when it was packed.
	// A stroke that grows the image past this invalidates the layout, and the
	// body's render form has to be rebuilt rather than patched.
	Texels image.Rectangle
}

// atlas is a body's packed paint.
type atlas struct {
	slots map[*mesh.FacePaint]paintSlot
	W, H  int

	// img is the packed picture, held only until it reaches the GPU.
	img *image.RGBA

	tex   rl.Texture2D
	ready bool

	// Overflow records that some textures did not fit even at the largest atlas
	// we will allocate. Those faces render unpainted, and the app says so
	// rather than leaving the user wondering where their paint went.
	Overflow bool
}

// buildAtlas packs every distinct texture on a mesh, or returns nil when
// nothing is painted.
//
// The order is deterministic — tallest first, ties broken by the face that
// introduced the texture — because golden shots compare pixels, and an atlas
// that packed differently from one run to the next would move every UV.
func buildAtlas(m *mesh.Mesh) *atlas {
	type entry struct {
		p     *mesh.FacePaint
		first int
		r     image.Rectangle
	}
	var list []entry
	seen := map[*mesh.FacePaint]bool{}
	for fi := range m.Faces {
		p := m.Faces[fi].Paint
		if p == nil || p.Img == nil || seen[p] {
			continue
		}
		seen[p] = true
		list = append(list, entry{p: p, first: fi, r: p.TexelBounds()})
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		hi, hj := list[i].r.Dy(), list[j].r.Dy()
		if hi != hj {
			return hi > hj
		}
		return list[i].first < list[j].first
	})

	a := &atlas{slots: map[*mesh.FacePaint]paintSlot{}}
	for size := 64; ; size *= 2 {
		if size > MaxAtlasSize {
			size = MaxAtlasSize
		}
		pack := shelf{w: size, h: size}
		placed := make([]image.Point, len(list))
		fits := true
		for i, e := range list {
			at, ok := pack.place(e.r.Dx(), e.r.Dy())
			if !ok {
				fits = false
				if size == MaxAtlasSize {
					// Take what fits and mark the rest; a face without a slot
					// simply renders in the body colour.
					placed = placed[:i]
					break
				}
				break
			}
			placed[i] = at
		}
		if !fits && size < MaxAtlasSize {
			continue
		}
		a.W, a.H = size, size
		a.Overflow = len(placed) < len(list)
		for i, at := range placed {
			a.slots[list[i].p] = paintSlot{Origin: at, Texels: list[i].r}
		}
		break
	}

	a.img = image.NewRGBA(image.Rect(0, 0, a.W, a.H))
	for p, slot := range a.slots {
		blitInto(a.img, p.Img, slot.Origin)
	}
	return a
}

// shelf is a row-by-row packer: simple, deterministic, and good enough for the
// handful of textures a body carries.
type shelf struct {
	w, h       int
	x, y, rowH int
}

func (s *shelf) place(w, h int) (image.Point, bool) {
	if w <= 0 || h <= 0 || w > s.w || h > s.h {
		return image.Point{}, false
	}
	if s.x+w > s.w {
		s.x = 0
		s.y += s.rowH + AtlasPadding
		s.rowH = 0
	}
	if s.y+h > s.h {
		return image.Point{}, false
	}
	at := image.Point{X: s.x, Y: s.y}
	s.x += w + AtlasPadding
	if h > s.rowH {
		s.rowH = h
	}
	return at, true
}

// blitInto copies a texture into the atlas image, one row at a time.
func blitInto(dst, src *image.RGBA, at image.Point) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < h; y++ {
		di := dst.PixOffset(at.X, at.Y+y)
		si := src.PixOffset(0, y)
		copy(dst.Pix[di:di+w*4], src.Pix[si:si+w*4])
	}
}

// upload puts the packed picture on the GPU with nearest filtering, which is
// the Crisp pillar in one line: a pixel-art texture that gets interpolated is
// no longer pixel art.
func (a *atlas) upload() {
	if a == nil || a.ready || a.img == nil {
		return
	}
	blank := rl.GenImageColor(a.W, a.H, color.RGBA{})
	a.tex = rl.LoadTextureFromImage(blank)
	rl.UnloadImage(blank)
	rl.UpdateTexture(a.tex, a.img)
	rl.SetTextureFilter(a.tex, rl.FilterPoint)
	a.ready = true
	// The CPU copy has done its job. The FacePaint images remain the source of
	// truth, and every later upload reads from them.
	a.img = nil
}

func (a *atlas) unload() {
	if a == nil || !a.ready {
		return
	}
	rl.UnloadTexture(a.tex)
	a.ready = false
}

// uv maps a world point on a painted face into atlas texture coordinates.
func (a *atlas) uv(p *mesh.FacePaint, world geom.Vec3) (u, v float32, ok bool) {
	slot, ok := a.slots[p]
	if !ok || a.W == 0 || a.H == 0 {
		return 0, 0, false
	}
	t := p.UV(world)
	x := float64(slot.Origin.X) + t.X - float64(slot.Texels.Min.X)
	y := float64(slot.Origin.Y) + t.Y - float64(slot.Texels.Min.Y)
	return float32(x / float64(a.W)), float32(y / float64(a.H)), true
}

// updateRect re-uploads one texture's changed texels, which is what a stroke
// costs: a few hundred bytes rather than a rebuilt body.
//
// It reports false when the layout it was packed against no longer holds — the
// image grew past its slot, or the texture is new — and the caller rebuilds.
func (a *atlas) updateRect(p *mesh.FacePaint, texels image.Rectangle) bool {
	if a == nil || !a.ready || p == nil || p.Img == nil {
		return false
	}
	slot, ok := a.slots[p]
	if !ok || p.TexelBounds() != slot.Texels {
		return false
	}
	r := texels.Intersect(slot.Texels)
	if r.Empty() {
		return true // nothing visible changed; the layout is still good
	}

	// UpdateTextureRec wants the rectangle tightly packed, and an image row is
	// wider than the rectangle, so the rows are gathered here.
	buf := make([]byte, r.Dx()*r.Dy()*4)
	for y := 0; y < r.Dy(); y++ {
		si := p.Img.PixOffset(r.Min.X-p.Off.X, r.Min.Y-p.Off.Y+y)
		copy(buf[y*r.Dx()*4:(y+1)*r.Dx()*4], p.Img.Pix[si:si+r.Dx()*4])
	}
	rl.UpdateTextureRec(a.tex, rl.Rectangle{
		X:      float32(slot.Origin.X + r.Min.X - slot.Texels.Min.X),
		Y:      float32(slot.Origin.Y + r.Min.Y - slot.Texels.Min.Y),
		Width:  float32(r.Dx()),
		Height: float32(r.Dy()),
	}, buf)
	return true
}

// uvOf is uv on a possibly nil atlas, so the mesh builder can ask without
// checking first.
func (a *atlas) uvOf(p *mesh.FacePaint, world geom.Vec3) (u, v float32, ok bool) {
	if a == nil || p == nil {
		return 0, 0, false
	}
	return a.uv(p, world)
}
