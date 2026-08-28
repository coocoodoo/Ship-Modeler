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

// Resolutions are the chips of SPEC-UX §13.1, in texels per unit.
//
// A chip is a density, not a count. At 8, one texel is an eighth of a unit on
// every face of every body in the document — so a pixel is the same physical
// size wherever it is painted, an edge line is the same thickness on both faces
// it touches, and pushing a face bigger does not make its pixels bigger. The
// chip used to mean "this face is N texels across at its widest", which made a
// pixel a different real size on every face (V-128).
var Resolutions = []int{1, 2, 4, 8, 16, 32}

// DefaultRes is the chip a fresh session starts on: eight texels to the unit,
// which puts a pixel on the eighth-unit grid.
const DefaultRes = 8

// MaxTextureSize caps a face's image on either axis (SPEC-GEOMETRY §8.3).
// Painting past it is refused rather than allowed to eat memory unbounded.
const MaxTextureSize = 1024

// Margin is how many texels of slack the first allocation leaves around the
// face's bounding box, so a stroke along the very edge has somewhere to land.
const Margin = 1

// Density is how many texels to the unit a picture actually has.
//
// For anything allocated since V-128 that is simply its chip, but a ship saved
// before then stored a texel size worked out from its face's longest side, so
// the honest answer comes from the texel rather than from the field. The panel
// reports this, not Res, so an old file describes itself truthfully.
func Density(p *mesh.FacePaint) float64 {
	if p == nil || p.Texel <= 0 {
		return 0
	}
	return 1 / p.Texel
}

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
// somewhere in the middle of it. Texel size is one over the chip and nothing
// else — not the face's size, not its shape — which is what makes a pixel the
// same thing everywhere in the document (V-128). It never changes afterwards
// either: a texture that silently rescaled when the face grew would break the
// pixel-art contract from the other direction.
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
	texel := 1 / float64(res)

	// Origin on the bbox corner: uv(lo) is now exactly (0,0).
	frame.O = frame.ToWorld(lo)

	// Texels 0..ceil(extent/texel)-1 cover the face; the margin adds one ring.
	w := int(math.Ceil((hi.X-lo.X)/texel-1e-9)) + 2*Margin
	h := int(math.Ceil((hi.Y-lo.Y)/texel-1e-9)) + 2*Margin

	// A density makes the picture grow with the face, so unlike a fixed texel
	// count this can ask for more than the cap allows. Refuse, and name the chip
	// that would fit: clamping would hand back a picture too small to cover the
	// face, and a face quietly painted at a density other than the chosen one is
	// exactly the thing the density was introduced to stop.
	if w > MaxTextureSize || h > MaxTextureSize {
		return nil, fmt.Errorf("that face is %.4g u across, which needs %d px at %d px/u — past the %d limit; try %d px/u",
			longest, maxInt(w, h)-2*Margin, res, MaxTextureSize, largestResFor(longest))
	}
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

// largestResFor is the finest chip whose picture still fits the cap for a face
// that long, so a refusal can say what to do rather than only what went wrong.
func largestResFor(longest float64) int {
	best := Resolutions[0]
	for _, r := range Resolutions {
		if int(math.Ceil(longest*float64(r)-1e-9))+2*Margin <= MaxTextureSize {
			best = r
		}
	}
	return best
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

// FaceRect is the texel rectangle the face itself occupies, which is not the
// same as the rectangle the image covers: the image carries a margin outside
// the face, and grows further whenever a stroke runs off the edge.
//
// The distinction matters wherever "on the face" is the question rather than
// "in the image" — a flood fill has to stop at the face's edge instead of
// flooding the margin, and the atlas packer wants the texels that can actually
// be seen.
func FaceRect(m *mesh.Mesh, fi int, p *mesh.FacePaint) image.Rectangle {
	if p == nil || m == nil || fi < 0 || fi >= len(m.Faces) || p.Texel <= 0 {
		return image.Rectangle{}
	}
	lo, hi, ok := faceExtent(m, fi, p.Frame)
	if !ok {
		return image.Rectangle{}
	}
	return image.Rect(
		floor(lo.X/p.Texel), floor(lo.Y/p.Texel),
		ceil(hi.X/p.Texel), ceil(hi.Y/p.Texel),
	)
}

// UV maps a world point to continuous texel coordinates
// (SPEC-GEOMETRY §8.3). The arithmetic lives on mesh.FacePaint so the renderer
// shares it rather than reimplementing it a package away.
func UV(p *mesh.FacePaint, world geom.Vec3) geom.Vec2 { return p.UV(world) }

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

// Bounds is the texel rectangle the image currently covers.
func Bounds(p *mesh.FacePaint) image.Rectangle { return p.TexelBounds() }

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

// ceil is math.Ceil for ints, with the same slack Allocate uses so a face whose
// extent lands a rounding error past a texel boundary does not gain a row.
func ceil(v float64) int {
	return int(math.Ceil(v - 1e-9))
}
