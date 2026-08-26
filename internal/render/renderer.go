package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/ui"
)

// Renderer owns every GPU resource: shaders, materials, the pick render texture
// and the shared 1x1 textures. One instance lives for the whole process.
type Renderer struct {
	shaded rl.Shader
	pick   rl.Shader

	locTint       int32
	locAlphaScale int32
	locUseTexture int32
	locPickIDBase int32

	shadedMat rl.Material
	pickMat   rl.Material

	whiteTex rl.Texture2D
	blankTex rl.Texture2D // 1x1 transparent: "this face has no paint"

	pickRT rl.RenderTexture2D

	// boxRT is the bigger, rarer target box select renders into.

	boxRT rl.RenderTexture2D

	boxW, boxH int

	boxReady bool

	// Table is rebuilt by every pick pass and read by the resolver.
	Table PickTable

	// saved2D holds the framebuffer's own 2D matrices while a 3D pass borrows
	// the pipeline. rlgl's push/pop share one stack across both matrix modes
	// and treat a modelview push as a push of its transform matrix, so saving
	// and restoring explicitly is the only reliable way to nest a 3D pass
	// inside 2D drawing.
	saved2DProj rl.Matrix
	saved2DView rl.Matrix

	// fbW and fbH are the current framebuffer size. They differ from the window
	// size while rendering offscreen (headless shots, thumbnails), and every
	// viewport calculation goes through them.
	fbW, fbH int
}

// SetFramebuffer tells the renderer the size of the target it is drawing into.
// Call it once per frame, and again around any offscreen render.
func (r *Renderer) SetFramebuffer(w, h int) { r.fbW, r.fbH = w, h }

// FramebufferSize returns the size the renderer is currently drawing into.
func (r *Renderer) FramebufferSize() (w, h int) { return r.fbW, r.fbH }

// NewRenderer creates the GPU resources. A GL context must already exist.
func NewRenderer() *Renderer {
	r := &Renderer{}
	r.shaded = rl.LoadShaderFromMemory(shadedVS, shadedFS)
	r.pick = rl.LoadShaderFromMemory(shadedVS, pickFS)

	r.locTint = rl.GetShaderLocation(r.shaded, "tint")
	r.locAlphaScale = rl.GetShaderLocation(r.shaded, "alphaScale")
	r.locUseTexture = rl.GetShaderLocation(r.shaded, "useTexture")
	r.locPickIDBase = rl.GetShaderLocation(r.pick, "idBase")

	white := rl.GenImageColor(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	r.whiteTex = rl.LoadTextureFromImage(white)
	rl.UnloadImage(white)
	blank := rl.GenImageColor(1, 1, color.RGBA{})
	r.blankTex = rl.LoadTextureFromImage(blank)
	rl.UnloadImage(blank)

	r.shadedMat = rl.LoadMaterialDefault()
	r.shadedMat.Shader = r.shaded
	r.shadedMat.GetMap(rl.MapDiffuse).Texture = r.blankTex

	r.pickMat = rl.LoadMaterialDefault()
	r.pickMat.Shader = r.pick
	r.pickMat.GetMap(rl.MapDiffuse).Texture = r.whiteTex

	r.pickRT = rl.LoadRenderTexture(PickRTSize, PickRTSize)
	return r
}

// Close releases the GPU resources.
//
// The two materials' map arrays are C allocations that raylib would free
// through UnloadMaterial, but that call would also unload the shader and
// textures we free here, so those few dozen bytes are left to process exit
// rather than risking a double free.
func (r *Renderer) Close() {
	rl.UnloadRenderTexture(r.pickRT)
	if r.boxReady {
		rl.UnloadRenderTexture(r.boxRT)
		r.boxReady = false
	}
	rl.UnloadTexture(r.whiteTex)
	rl.UnloadTexture(r.blankTex)
	rl.UnloadShader(r.shaded)
	rl.UnloadShader(r.pick)
}

// ClearColor paints the whole framebuffer with the window background.
func (r *Renderer) ClearColor() { rl.ClearBackground(ui.ColorBG) }

// DrawViewport renders one frame of the 3D scene into a rectangle of the
// current framebuffer, in the pass order of SPEC-RENDER §1.
func (r *Renderer) DrawViewport(s *Scene, vp Viewport) {
	if vp.W <= 0 || vp.H <= 0 {
		return
	}
	r.drawBackground(vp)

	r.begin3D(s.Camera, vp)
	r.drawShadedPass(s)
	r.drawEdgePass(s, vp)
	r.drawTranslucentPass(s, vp)
	r.drawOverlayPass(s, vp)
	r.end3D()
}

// begin3D sets the GL viewport and matrices for a sub-rectangle of the window.
// raylib's BeginMode3D always spans the whole framebuffer, so this configures
// the viewport and projection directly instead.
func (r *Renderer) begin3D(cam Camera, vp Viewport) {
	rl.DrawRenderBatchActive() // flush any 2D work queued before this
	glY := int32(r.fbH - (vp.Y + vp.H))
	rl.Viewport(int32(vp.X), glY, int32(vp.W), int32(vp.H))
	rl.EnableScissorTest()
	rl.Scissor(int32(vp.X), glY, int32(vp.W), int32(vp.H))

	r.saved2DProj = rl.GetMatrixProjection()
	r.saved2DView = rl.GetMatrixModelview()
	rl.SetMatrixProjection(toRLMatrix(cam.Proj(vp.Aspect())))
	rl.SetMatrixModelview(toRLMatrix(cam.View()))

	rl.EnableDepthTest()
	rl.SetTexture(r.whiteTex.ID)
}

func (r *Renderer) end3D() {
	rl.SetTexture(0)
	rl.DrawRenderBatchActive()
	rl.SetMatrixProjection(r.saved2DProj)
	rl.SetMatrixModelview(r.saved2DView)
	rl.DisableDepthTest()
	rl.DisableScissorTest()
	rl.Viewport(0, 0, int32(r.fbW), int32(r.fbH))
}

// drawBackground fills the viewport with the vertical gradient of SPEC-UX §3.
func (r *Renderer) drawBackground(vp Viewport) {
	rl.DrawRectangleGradientV(
		int32(vp.X), int32(vp.Y), int32(vp.W), int32(vp.H),
		ui.ColorViewportTop, ui.ColorViewportBottom)
	rl.DrawRenderBatchActive()
}

// drawShadedPass draws opaque bodies with the flat-shading shader.
func (r *Renderer) drawShadedPass(s *Scene) {
	rl.EnableBackfaceCulling()
	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || !b.GPU.uploaded || b.Alpha < 0.999 {
			continue
		}
		r.drawBody(b, s.dimFor(b))
	}
}

// drawTranslucentPass draws planes, the grid and previews after the opaque set.
func (r *Renderer) drawTranslucentPass(s *Scene, vp Viewport) {
	rl.DisableBackfaceCulling()
	r.drawPlanes(s, vp)
	if s.Grid != nil {
		r.drawGrid(r.ribbonCtx(s.Camera, vp), s.Grid)
		rl.DrawRenderBatchActive()
	}
	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || !b.GPU.uploaded || b.Alpha >= 0.999 {
			continue
		}
		r.drawBody(b, s.dimFor(b))
	}
	rl.EnableBackfaceCulling()
}

func (r *Renderer) drawBody(b *BodyDraw, dim float64) {
	if b.Alpha <= 0 {
		return
	}
	col := b.Color
	if dim > 0 && dim < 1 {
		col = ui.Shade(col, dim)
	}
	r.shadedMat.GetMap(rl.MapDiffuse).Color = col
	rl.SetShaderValue(r.shaded, r.locTint, colorToVec4(b.Tint), rl.ShaderUniformVec4)
	rl.SetShaderValue(r.shaded, r.locAlphaScale, []float32{float32(b.Alpha)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shaded, r.locUseTexture, []float32{0}, rl.ShaderUniformFloat)
	rl.DrawMesh(*b.GPU.rlMesh, r.shadedMat, toRLMatrix(b.Transform))
}

// drawPlanes renders the default planes as two-sided translucent quads with a
// screen-constant border (SPEC-UX §5).
func (r *Renderer) drawPlanes(s *Scene, vp Viewport) {
	// The planes are part of "everything else": while a mode is dimming the
	// scene to make one thing stand out, three full-strength quads across the
	// viewport are exactly what it is trying to get out of the way.
	dim := s.DimFactor
	if len(s.Planes) == 0 {
		return
	}
	c := r.ribbonCtx(s.Camera, vp)
	for i := range s.Planes {
		p := &s.Planes[i]
		quad := planeQuad(p.Frame, p.HalfSize)

		// A selected plane is tinted in the accent, not merely outlined in it.
		// These quads are enormous, and a one-and-a-half pixel border on
		// something that fills the viewport is not a highlight anybody sees.
		fill := p.Color
		border := ui.WithAlpha(p.Color, 0x4D)
		width := 1.5
		switch {
		case p.Selected:
			fill = ui.WithAlpha(ui.ColorAccent, PlaneSelectedAlpha)
			border, width = ui.ColorAccent, 3
		case p.Hovered:
			fill = ui.WithAlpha(fill, uint8(min255(int(fill.A)*2)))
			border = ui.WithAlpha(p.Color, 0xAA)
		}
		if dim > 0 && dim < 1 {
			fill = fadeAlpha(fill, dim)
			border = fadeAlpha(border, dim)
		}
		drawPolyFan(quad[:], fill)
		for k := 0; k < 4; k++ {
			r.drawRibbon(c, quad[k], quad[(k+1)%4], width, border, 1)
		}
	}
	rl.DrawRenderBatchActive()
}

// PlaneSelectedAlpha is how strongly a selected plane is tinted. Stronger than
// its resting tint so the selection reads across the whole quad, weak enough
// that the model behind it still does.
const PlaneSelectedAlpha = 0x3A

// drawEdgePass draws the crease and boundary overlay for every opaque body
// (SPEC-RENDER §5).
func (r *Renderer) drawEdgePass(s *Scene, vp Viewport) {
	c := r.ribbonCtx(s.Camera, vp)
	rl.DisableBackfaceCulling()
	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || b.Alpha < 0.999 {
			continue
		}
		col := b.EdgeColor
		if col.A == 0 {
			col = ui.EdgeColor(b.Color)
		}
		width := float64(EdgeScreenWidth)
		if b.Selected {
			col, width = ui.ColorAccent, EdgeSelectedWidth
		}
		if d := s.dimFor(b); d > 0 && d < 1 {
			col = fadeAlpha(col, d)
		}
		for _, e := range b.GPU.Edges {
			a := b.Transform.TransformPoint(e.A)
			z := b.Transform.TransformPoint(e.B)
			ec := col
			if e.Kind == mesh.EdgeNonPlanar {
				ec = ui.ColorWarn
			}
			r.drawRibbon(c, a, z, width, ec, EyeShrinkEdgeFactor)
		}
	}
	rl.DrawRenderBatchActive()
	rl.EnableBackfaceCulling()
}

// ScreenRay returns the world ray under a window pixel, for tools that need
// geometric picking rather than the ID buffer.
func (r *Renderer) ScreenRay(cam Camera, vp Viewport, windowX, windowY float64) (origin, dir geom.Vec3) {
	return cam.Ray(vp.Local(windowX, windowY), float64(vp.W), float64(vp.H))
}

func min255(v int) int {
	if v > 255 {
		return 255
	}
	return v
}
