package render

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/ui"
)

// Immediate-mode world-space drawing helpers built on rlgl's batch. They feed
// raylib's default shader, whose output is vertexColor * colDiffuse * texel —
// with the white 1x1 texture bound that is exactly "draw this color", which is
// what overlays, gizmos, grid lines and pick ribbons all want.

// EdgeScreenWidth is the drawn thickness of a mesh edge (SPEC-RENDER §5).
const (
	EdgeScreenWidth     = 1.5
	EdgeSelectedWidth   = 3.0
	EdgePickWidth       = 5.0
	VertPickScreenSize  = 9.0
	EyeShrinkFraction   = 0.0005 // 0.05% of eye distance, the z-fight guard
	EyeShrinkEdgeFactor = 2.0
	EyeShrinkVertFactor = 3.0
)

// ribbonContext carries what screen-constant sizing needs.
type ribbonContext struct {
	cam    Camera
	viewH  float64
	aspect float64
}

func (r *Renderer) ribbonCtx(cam Camera, vp Viewport) ribbonContext {
	return ribbonContext{cam: cam, viewH: float64(vp.H), aspect: vp.Aspect()}
}

// worldPerPixel returns how many world units one screen pixel spans at p.
func (c ribbonContext) worldPerPixel(p geom.Vec3) float64 {
	if c.viewH <= 0 {
		return 0
	}
	if !c.cam.Perspective {
		return c.cam.OrthoScale / c.viewH
	}
	// In perspective the scale depends on depth along the view direction.
	depth := math.Abs(p.Sub(c.cam.Eye()).Dot(c.cam.Forward()))
	if depth < 1e-6 {
		depth = 1e-6
	}
	return 2 * depth * math.Tan(PerspectiveFOV*math.Pi/360) / c.viewH
}

// shrinkToEye nudges a point toward the camera to keep overlay geometry from
// z-fighting the surface it decorates (SPEC-RENDER §5). The offset scales with
// eye distance, so it is backend agnostic and resolution independent.
func (c ribbonContext) shrinkToEye(p geom.Vec3, factor float64) geom.Vec3 {
	eye := c.cam.Eye()
	if !c.cam.Perspective {
		// Orthographic has no eye point that means anything for scaling, so
		// shift along the view direction by a fraction of the view depth.
		return p.Sub(c.cam.Forward().Mul(c.cam.OrthoScale * EyeShrinkFraction * factor))
	}
	d := p.Sub(eye)
	return eye.Add(d.Mul(1 - EyeShrinkFraction*factor))
}

// drawRibbon emits a camera-facing quad along a to b with a constant screen
// width in pixels.
func (r *Renderer) drawRibbon(c ribbonContext, a, b geom.Vec3, widthPx float64, col color.RGBA, shrink float64) {
	a = c.shrinkToEye(a, shrink)
	b = c.shrinkToEye(b, shrink)
	dir, ok := b.Sub(a).NormalizeOK()
	if !ok {
		return
	}
	toEye := c.cam.Forward().Neg()
	if c.cam.Perspective {
		mid := a.Add(b).Mul(0.5)
		if d, ok := c.cam.Eye().Sub(mid).NormalizeOK(); ok {
			toEye = d
		}
	}
	side, ok := dir.Cross(toEye).NormalizeOK()
	if !ok {
		return
	}
	ha := side.Mul(widthPx * c.worldPerPixel(a) / 2)
	hb := side.Mul(widthPx * c.worldPerPixel(b) / 2)
	quad3(a.Sub(ha), a.Add(ha), b.Add(hb), b.Sub(hb), col)
}

// drawBillboardQuad emits a screen-facing square of a constant pixel size.
func (r *Renderer) drawBillboardQuad(c ribbonContext, p geom.Vec3, sizePx float64, col color.RGBA, shrink float64) {
	p = c.shrinkToEye(p, shrink)
	h := sizePx * c.worldPerPixel(p) / 2
	right := c.cam.Right().Mul(h)
	up := c.cam.Up().Mul(h)
	quad3(
		p.Sub(right).Sub(up),
		p.Add(right).Sub(up),
		p.Add(right).Add(up),
		p.Sub(right).Add(up),
		col,
	)
}

// quad3 pushes two triangles with a single color into the rlgl batch.
func quad3(a, b, c, d geom.Vec3, col color.RGBA) {
	tri3(a, b, c, col)
	tri3(a, c, d, col)
}

func tri3(a, b, c geom.Vec3, col color.RGBA) {
	rl.Begin(rl.Triangles)
	rl.Color4ub(col.R, col.G, col.B, col.A)
	rl.Vertex3f(float32(a.X), float32(a.Y), float32(a.Z))
	rl.Vertex3f(float32(b.X), float32(b.Y), float32(b.Z))
	rl.Vertex3f(float32(c.X), float32(c.Y), float32(c.Z))
	rl.End()
}

// drawPolyFan fills a convex loop with one color, used for plane quads and
// region fills. Callers that want it visible from behind disable backface
// culling; drawing the loop twice would compound its translucency.
func drawPolyFan(pts []geom.Vec3, col color.RGBA) {
	for i := 1; i+1 < len(pts); i++ {
		tri3(pts[0], pts[i], pts[i+1], col)
	}
}

// planeQuad returns the four corners of a bounded plane quad.
func planeQuad(f geom.Frame, half float64) [4]geom.Vec3 {
	return [4]geom.Vec3{
		f.ToWorld(geom.Vec2{X: -half, Y: -half}),
		f.ToWorld(geom.Vec2{X: half, Y: -half}),
		f.ToWorld(geom.Vec2{X: half, Y: half}),
		f.ToWorld(geom.Vec2{X: -half, Y: half}),
	}
}

// drawGrid draws minor and major lines on a frame as thin ribbons.
func (r *Renderer) drawGrid(c ribbonContext, g *GridDraw) {
	if g == nil || g.Alpha <= 0.01 {
		return
	}
	minor := fadeAlpha(ui.ColorGridMinor, g.Alpha)
	major := fadeAlpha(ui.ColorGridMajor, g.Alpha)

	// Lines whose on-screen spacing drops below this many pixels are dropped
	// rather than drawn into moire (SPEC-UX §8.1).
	const minSpacingPx = 8
	pxPerUnit := c.cam.PixelsPerWorldUnit(c.viewH)

	drawSet := func(step float64, col color.RGBA, skipMultiplesOf float64) {
		if step <= 0 || step*pxPerUnit < minSpacingPx {
			return
		}
		n := int(g.HalfSize/step) + 1
		for i := -n; i <= n; i++ {
			t := float64(i) * step
			if t < -g.HalfSize || t > g.HalfSize {
				continue
			}
			if skipMultiplesOf > 0 && math.Abs(math.Mod(t, skipMultiplesOf)) < 1e-9 {
				continue // the major pass draws this one
			}
			r.drawRibbon(c,
				g.Frame.ToWorld(geom.Vec2{X: t, Y: -g.HalfSize}),
				g.Frame.ToWorld(geom.Vec2{X: t, Y: g.HalfSize}),
				1, col, 1)
			r.drawRibbon(c,
				g.Frame.ToWorld(geom.Vec2{X: -g.HalfSize, Y: t}),
				g.Frame.ToWorld(geom.Vec2{X: g.HalfSize, Y: t}),
				1, col, 1)
		}
	}
	drawSet(g.MinorStep, minor, g.MajorStep)
	drawSet(g.MajorStep, major, 0)

	if g.ShowAxes {
		r.drawRibbon(c,
			g.Frame.ToWorld(geom.Vec2{X: -g.HalfSize}),
			g.Frame.ToWorld(geom.Vec2{X: g.HalfSize}),
			1.5, fadeAlpha(g.AxisUColor, g.Alpha), 1)
		r.drawRibbon(c,
			g.Frame.ToWorld(geom.Vec2{Y: -g.HalfSize}),
			g.Frame.ToWorld(geom.Vec2{Y: g.HalfSize}),
			1.5, fadeAlpha(g.AxisVColor, g.Alpha), 1)
	}
}

func fadeAlpha(c color.RGBA, a float64) color.RGBA {
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: uint8(float64(c.A)*a + 0.5)}
}
