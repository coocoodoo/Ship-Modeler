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
	// so the cursor is also a preview of the colour.
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
		fill := ui.WithAlpha(v.Color, 0x99)
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
