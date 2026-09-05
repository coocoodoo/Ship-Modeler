package scene

import (
	"image"
	"image/color"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// The texel cursor of SPEC-UX §13.2: the exact texels the brush is about to
// write, outlined on the surface itself.
//
// It is the whole reason paint mode can be trusted at an angle. A brush drawn
// as a screen-space circle tells you where the pointer is; this tells you which
// pixels you are going to hit, which is a different question the moment the
// face is not square-on to the camera.

// PaintCursorLift is how far the outline floats above the face, in texels.
// Overlays draw with the depth test off, so this is not about z-fighting — it
// is about the outline of a texel on a face that is being viewed edge-on not
// collapsing into the face's own silhouette.
const PaintCursorLift = 0.02

// PaintCursorView is what the cursor needs to draw itself.
type PaintCursorView struct {
	// Paint is the face's texture mapping. For an unpainted face this is the
	// provisional allocation the first stroke would create, so the cursor shows
	// the grid you are about to commit to rather than nothing at all.
	Paint *mesh.FacePaint
	// Texel is the texel under the pointer: the brush square's top-left, the
	// same anchor the brush itself uses.
	Texel image.Point
	// Size is the brush's edge in texels.
	Size int
	// Color is what the brush would write, drawn as a fill inside the outline
	// so the cursor is also a preview of the colour. Its alpha is the brush's
	// own, so a glaze looks like a glaze before it lands (V-158).
	Color color.RGBA
	// Erasing draws the cursor as an outline with no fill, because an eraser
	// has no colour to preview.
	Erasing bool
	// Filling widens the outline to the whole face's rectangle, which is the
	// honest preview of what a flood fill could reach.
	Filling bool
	// FaceRect bounds the fill preview.
	FaceRect image.Rectangle
}

// BuildPaintCursor traces the brush's texels as an overlay, or returns nil when
// there is nothing under the pointer.
func BuildPaintCursor(v PaintCursorView) *render.Overlay {
	if v.Paint == nil || v.Paint.Texel <= 0 {
		return nil
	}
	size := v.Size
	if size < 1 {
		size = 1
	}
	o := &render.Overlay{}

	// The colour preview sits inside the outline rather than replacing it: a
	// fill alone would be indistinguishable from paint already on the face.
	if !v.Erasing && !v.Filling {
		fill := ui.WithAlpha(v.Color, cursorFillAlpha(v.Color.A))
		for dy := 0; dy < size; dy++ {
			for dx := 0; dx < size; dx++ {
				t := v.Texel.Add(image.Point{X: dx, Y: dy})
				c := liftedCorners(v.Paint, t)
				o.Fills = append(o.Fills,
					render.OverlayTri{A: c[0], B: c[1], C: c[2], Color: fill},
					render.OverlayTri{A: c[0], B: c[2], C: c[3], Color: fill})
			}
		}
	}

	// The internal grid, so a 2x2 or 4x4 brush still reads as pixels rather
	// than as one bigger square.
	if size > 1 {
		for dy := 0; dy < size; dy++ {
			for dx := 0; dx < size; dx++ {
				traceTexel(o, v.Paint, v.Texel.Add(image.Point{X: dx, Y: dy}),
					ui.WithAlpha(ui.ColorText, 0x55), 1)
			}
		}
	}

	// The brush's own outline, in white so it reads over any paint under it.
	outline := image.Rectangle{Min: v.Texel, Max: v.Texel.Add(image.Point{X: size, Y: size})}
	traceRect(o, v.Paint, outline, ui.ColorText, 2)

	// A fill's reach, dashed, because it is a bound and not a boundary: the
	// flood stops at the colour it started on, somewhere inside this.
	if v.Filling && !v.FaceRect.Empty() {
		traceRectDashed(o, v.Paint, v.FaceRect, ui.Fade(ui.ColorAccent, 0.8), 1.5)
	}
	if o.Empty() {
		return nil
	}
	return o
}

// CursorFillAlpha is how solid the cursor's colour preview is at a full-alpha
// brush. It is well short of opaque on purpose: the cursor is a preview of
// paint, and paint you cannot see through the cursor is a cursor you cannot
// aim.
const CursorFillAlpha = 0x99

// cursorFillMin keeps the preview visible at the bottom of the alpha slider.
// A brush at 1/255 writes almost nothing, but the cursor still has to say
// where the brush is.
const cursorFillMin = 0x20

// cursorFillAlpha scales the preview by the brush's alpha.
func cursorFillAlpha(a uint8) uint8 {
	if a == 0 {
		return CursorFillAlpha // a caller that set no alpha means an opaque brush
	}
	v := int(float64(a)/255*CursorFillAlpha + 0.5)
	if v < cursorFillMin {
		v = cursorFillMin
	}
	return uint8(v)
}

// liftedCorners is a texel's four world corners, floated clear of the face.
func liftedCorners(p *mesh.FacePaint, t image.Point) [4]geom.Vec3 {
	c := paint.Corners(p, t)
	lift := p.Frame.N.Mul(p.Texel * PaintCursorLift)
	for i := range c {
		c[i] = c[i].Add(lift)
	}
	return c
}

// traceTexel outlines one texel.
func traceTexel(o *render.Overlay, p *mesh.FacePaint, t image.Point, col color.RGBA, width float64) {
	traceRect(o, p, image.Rectangle{Min: t, Max: t.Add(image.Point{X: 1, Y: 1})}, col, width)
}

// rectCorners is a texel rectangle's four world corners, wound with the frame.
func rectCorners(p *mesh.FacePaint, r image.Rectangle) [4]geom.Vec3 {
	lift := p.Frame.N.Mul(p.Texel * PaintCursorLift)
	at := func(x, y int) geom.Vec3 {
		return p.Frame.ToWorld(geom.Vec2{
			X: float64(x) * p.Texel, Y: float64(y) * p.Texel,
		}).Add(lift)
	}
	return [4]geom.Vec3{
		at(r.Min.X, r.Min.Y), at(r.Max.X, r.Min.Y),
		at(r.Max.X, r.Max.Y), at(r.Min.X, r.Max.Y),
	}
}

func traceRect(o *render.Overlay, p *mesh.FacePaint, r image.Rectangle, col color.RGBA, width float64) {
	c := rectCorners(p, r)
	for i := 0; i < 4; i++ {
		o.Lines = append(o.Lines, render.OverlayLine{
			A: c[i], B: c[(i+1)%4], Color: col, WidthPx: width,
		})
	}
}

func traceRectDashed(o *render.Overlay, p *mesh.FacePaint, r image.Rectangle, col color.RGBA, width float64) {
	c := rectCorners(p, r)
	for i := 0; i < 4; i++ {
		o.Lines = append(o.Lines, render.OverlayLine{
			A: c[i], B: c[(i+1)%4], Color: col, WidthPx: width, Dashed: true,
		})
	}
}

// TileGhostView is what the tile stamp's preview needs: the face mapping, the
// cell the stamp would land on, and the oriented tile pixels themselves.
type TileGhostView struct {
	Paint         *mesh.FacePaint
	Cell          image.Point
	Tile          *image.RGBA
	PreserveAlpha bool
	Clip          *image.Rectangle
}

func BuildPixelSelection(p *mesh.FacePaint, rect image.Rectangle) *render.Overlay {
	if p == nil || rect.Empty() {
		return nil
	}
	o := &render.Overlay{}
	traceRect(o, p, rect, ui.ColorBG, 3)
	traceRectDashed(o, p, rect, ui.ColorAccent, 2)
	return o
}

// BuildTileGhost draws the armed tile as a translucent preview at its cell —
// the actual pixels, half-strength, with the stamp's outline over them. Seeing
// the real pixels is the point: a rectangle would say where, this says what.
func BuildTileGhost(v TileGhostView) *render.Overlay {
	if v.Paint == nil || v.Paint.Texel <= 0 || v.Tile == nil {
		return nil
	}
	b := v.Tile.Bounds()
	if b.Empty() {
		return nil
	}
	o := &render.Overlay{}
	// Clip before walking the clipboard, and merge equal-colour runs. Large
	// solid selections then need one quad per row rather than one per pixel.
	visible := image.Rect(0, 0, b.Dx(), b.Dy())
	if v.Clip != nil {
		visible = visible.Intersect(v.Clip.Sub(v.Cell))
	}
	for y := visible.Min.Y; y < visible.Max.Y; y++ {
		for x := visible.Min.X; x < visible.Max.X; x++ {
			px := v.Tile.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if px.A == 0 || (!v.PreserveAlpha && px.A < paint.StampAlphaThreshold) {
				continue
			}
			fill := px
			fill.A = 0x8C
			if v.PreserveAlpha {
				fill.A = uint8(int(px.A) * 140 / 255)
			}
			end := x + 1
			if v.PreserveAlpha {
				for end < visible.Max.X && v.Tile.RGBAAt(b.Min.X+end, b.Min.Y+y) == px {
					end++
				}
			}
			c := rectCorners(v.Paint, image.Rect(x, y, end, y+1).Add(v.Cell))
			o.Fills = append(o.Fills,
				render.OverlayTri{A: c[0], B: c[1], C: c[2], Color: fill},
				render.OverlayTri{A: c[0], B: c[2], C: c[3], Color: fill})
			x = end - 1
		}
	}
	outline := image.Rectangle{Min: v.Cell, Max: v.Cell.Add(image.Point{X: b.Dx(), Y: b.Dy()})}
	if v.Clip != nil {
		outline = outline.Intersect(*v.Clip)
	}
	traceRect(o, v.Paint, outline, ui.ColorText, 2)
	if o.Empty() {
		return nil
	}
	return o
}
