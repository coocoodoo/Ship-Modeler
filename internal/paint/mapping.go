// Package paint owns face textures: where a texel is in the world, what the
// brush does to it, and the palette the colours come from (R13,
// SPEC-GEOMETRY §8, SPEC-UX §13).
//
// The RGBA images are the source of truth. Package render mirrors them onto the
// GPU with nearest filtering and never edits them; package model wraps strokes
// in commands so they undo. This package is raylib-free and cgo-free, so all of
// it is testable without a window — which matters, because a mapping that is
// one texel out is invisible in a screenshot and obvious in a unit test.
package paint

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Resolutions are the chips of SPEC-UX §13.1. A chip means "this face is N
// texels across at its widest", which is why the density it produces depends on
// the face and is then fixed for good.
var Resolutions = []int{16, 32, 128, 256, 512}

// DefaultRes is the chip a fresh session starts on.
const DefaultRes = 32

// MaxTextureSize caps a face's image on either axis (SPEC-GEOMETRY §8.3).
// Painting past it is refused rather than allowed to eat memory unbounded.
const MaxTextureSize = 1024

// Margin is how many texels of slack the first allocation leaves around the
// face's bounding box, so a stroke along the very edge has somewhere to land.
const Margin = 1

// ValidRes reports whether res is one of the chips.
func ValidRes(res int) bool {
	for _, r := range Resolutions {
		if r == res {
			return true
		}
	}
	return false
}

// Allocate builds the FacePaint for a face at a chosen resolution
// (SPEC-GEOMETRY §8.2).
//
// The frame is the face's canonical frame with its origin moved to the
// bounding-box corner, so texel (0,0) is a corner of the face rather than
// somewhere in the middle of it. Texel size comes from the longest bbox side
// and never changes afterwards: constant pixel density is the whole pixel-art
// contract, and a texture that silently rescales when the face grows breaks it.
func Allocate(m *mesh.Mesh, fi int, res int) (*mesh.FacePaint, error) {
	if !ValidRes(res) {
		return nil, fmt.Errorf("%d is not a paint resolution", res)
	}
	if m == nil || fi < 0 || fi >= len(m.Faces) {
		return nil, fmt.Errorf("no such face")
	}
	frame := m.FaceFrame(fi)
	lo, hi, ok := faceExtent(m, fi, frame)
	if !ok {
		return nil, fmt.Errorf("that face has no outline to paint on")
	}
	longest := math.Max(hi.X-lo.X, hi.Y-lo.Y)
	if longest <= 0 {
		return nil, fmt.Errorf("that face has no area to paint on")
	}
	texel := longest / float64(res)

	// Origin on the bbox corner: uv(lo) is now exactly (0,0).
	frame.O = frame.ToWorld(lo)

	// Texels 0..ceil(extent/texel)-1 cover the face; the margin adds one ring.
	w := int(math.Ceil((hi.X-lo.X)/texel-1e-9)) + 2*Margin
	h := int(math.Ceil((hi.Y-lo.Y)/texel-1e-9)) + 2*Margin
	w, h = clampSize(w), clampSize(h)

	return &mesh.FacePaint{
		Res:   res,
		Texel: texel,
		Frame: frame,
		Img:   image.NewRGBA(image.Rect(0, 0, w, h)),
		Off:   image.Point{X: -Margin, Y: -Margin},
	}, nil
}

// Resample rebuilds a face's paint at a new resolution, carrying the pixels
// over by nearest sampling in world space (SPEC-UX §13.2).
//
// Sampling through the world rather than through texel arithmetic is what makes
// this correct when the face has moved, grown or been cut since the paint was
// made: the only thing the two images agree on is where they are in space.
func Resample(m *mesh.Mesh, fi int, src *mesh.FacePaint, res int) (*mesh.FacePaint, error) {
	out, err := Allocate(m, fi, res)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return out, nil
	}
	b := out.Img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			tx := image.Point{X: x + out.Off.X, Y: y + out.Off.Y}
			if c := At(src, Texel(src, World(out, tx))); c.A != 0 {
				out.Img.SetRGBA(x, y, c)
			}
		}
	}
	return out, nil
}

// faceExtent is the face's bounding box in frame coordinates.
func faceExtent(m *mesh.Mesh, fi int, frame geom.Frame) (lo, hi geom.Vec2, ok bool) {
	outer := m.Faces[fi].Outer()
	if len(outer) < 3 {
		return geom.Vec2{}, geom.Vec2{}, false
	}
	lo = geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}
	hi = geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, loop := range m.Faces[fi].Loops {
		for _, vi := range loop {
			p := frame.ToLocal(m.Verts[vi])
			lo.X, lo.Y = math.Min(lo.X, p.X), math.Min(lo.Y, p.Y)
			hi.X, hi.Y = math.Max(hi.X, p.X), math.Max(hi.Y, p.Y)
		}
	}
	return lo, hi, true
}

func clampSize(v int) int {
	if v < 1 {
		return 1
	}
	if v > MaxTextureSize {
		return MaxTextureSize
	}
	return v
}

// UV maps a world point to continuous texel coordinates
// (SPEC-GEOMETRY §8.3). The normal component is discarded, so a point anywhere
// along the face's normal maps to the same texel — which is exactly what a
// cursor ray hitting the surface needs.
func UV(p *mesh.FacePaint, world geom.Vec3) geom.Vec2 {
	if p == nil || p.Texel == 0 {
		return geom.Vec2{}
	}
	local := p.Frame.ToLocal(world)
	return geom.Vec2{X: local.X / p.Texel, Y: local.Y / p.Texel}
}

// Texel maps a world point to the integer texel containing it.
func Texel(p *mesh.FacePaint, world geom.Vec3) image.Point {
	uv := UV(p, world)
	return image.Point{X: floor(uv.X), Y: floor(uv.Y)}
}

// World returns the world-space centre of a texel.
func World(p *mesh.FacePaint, t image.Point) geom.Vec3 {
	if p == nil {
		return geom.Vec3{}
	}
	return p.Frame.ToWorld(geom.Vec2{
		X: (float64(t.X) + 0.5) * p.Texel,
		Y: (float64(t.Y) + 0.5) * p.Texel,
	})
}

// Corners returns a texel's four world-space corners, wound the same way as the
// frame's axes so the texel cursor can trace them as a loop.
func Corners(p *mesh.FacePaint, t image.Point) [4]geom.Vec3 {
	if p == nil {
		return [4]geom.Vec3{}
	}
	x0, y0 := float64(t.X)*p.Texel, float64(t.Y)*p.Texel
	x1, y1 := x0+p.Texel, y0+p.Texel
	return [4]geom.Vec3{
		p.Frame.ToWorld(geom.Vec2{X: x0, Y: y0}),
		p.Frame.ToWorld(geom.Vec2{X: x1, Y: y0}),
		p.Frame.ToWorld(geom.Vec2{X: x1, Y: y1}),
		p.Frame.ToWorld(geom.Vec2{X: x0, Y: y1}),
	}
}

// Bounds is the texel rectangle the image currently covers. Texel coordinates
// and image coordinates differ by Off, and every read and write goes through
// here rather than doing that arithmetic by hand.
func Bounds(p *mesh.FacePaint) image.Rectangle {
	if p == nil || p.Img == nil {
		return image.Rectangle{}
	}
	return p.Img.Bounds().Add(p.Off)
}

// At reads a texel. Anything outside the image is unpainted, which is the same
// answer as a transparent texel inside it: the body colour shows through.
func At(p *mesh.FacePaint, t image.Point) color.RGBA {
	if p == nil || p.Img == nil || !t.In(Bounds(p)) {
		return color.RGBA{}
	}
	return p.Img.RGBAAt(t.X-p.Off.X, t.Y-p.Off.Y)
}

// Set writes a texel, growing the image if the texel falls outside it. It
// reports false when the growth would pass MaxTextureSize, in which case
// nothing is written and the caller says so (SPEC-GEOMETRY §8.3).
func Set(p *mesh.FacePaint, t image.Point, c color.RGBA) bool {
	if p == nil || p.Img == nil {
		return false
	}
	if !t.In(Bounds(p)) && !Grow(p, image.Rectangle{Min: t, Max: t.Add(image.Point{X: 1, Y: 1})}) {
		return false
	}
	p.Img.SetRGBA(t.X-p.Off.X, t.Y-p.Off.Y, c)
	return true
}

// Grow enlarges the image so it covers r as well as what it already held,
// keeping every existing pixel at the same texel — and therefore at the same
// place in the world, since the frame and texel size never move.
func Grow(p *mesh.FacePaint, r image.Rectangle) bool {
	if p == nil || p.Img == nil || r.Empty() {
		return false
	}
	want := Bounds(p).Union(r)
	if want == Bounds(p) {
		return true
	}
	if want.Dx() > MaxTextureSize || want.Dy() > MaxTextureSize {
		return false
	}
	dst := image.NewRGBA(image.Rect(0, 0, want.Dx(), want.Dy()))
	old := Bounds(p)
	for y := old.Min.Y; y < old.Max.Y; y++ {
		for x := old.Min.X; x < old.Max.X; x++ {
			dst.SetRGBA(x-want.Min.X, y-want.Min.Y, p.Img.RGBAAt(x-p.Off.X, y-p.Off.Y))
		}
	}
	p.Img, p.Off = dst, want.Min
	return true
}

// Copy returns an independent duplicate, which is what a fragment needs before
// it can be painted without writing into the paint its siblings still share
// (SPEC-GEOMETRY §8.4).
func Copy(p *mesh.FacePaint) *mesh.FacePaint {
	if p == nil {
		return nil
	}
	out := *p
	if p.Img != nil {
		img := image.NewRGBA(p.Img.Bounds())
		copy(img.Pix, p.Img.Pix)
		out.Img = img
	}
	return &out
}

// SubImage copies a texel rectangle out of a face's image, which is how a
// stroke command snapshots what it is about to overwrite (SPEC-DATA §3.3).
func SubImage(p *mesh.FacePaint, r image.Rectangle) *image.RGBA {
	if p == nil || p.Img == nil || r.Empty() {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			out.SetRGBA(x-r.Min.X, y-r.Min.Y, At(p, image.Point{X: x, Y: y}))
		}
	}
	return out
}

// Blit writes a rectangle back, growing the image if it has since shrunk. It is
// the inverse of SubImage and the undo half of a stroke.
func Blit(p *mesh.FacePaint, r image.Rectangle, src *image.RGBA) {
	if p == nil || src == nil || r.Empty() {
		return
	}
	Grow(p, r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			Set(p, image.Point{X: x, Y: y}, src.RGBAAt(x-r.Min.X, y-r.Min.Y))
		}
	}
}

// floor is math.Floor for ints, correct on the negative side where a plain
// int() conversion truncates toward zero and puts texel -0.5 in texel 0.
func floor(v float64) int {
	return int(math.Floor(v))
}
