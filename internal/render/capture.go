package render

import (
	"image"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Rendering the scene into an image rather than onto the window: the thumbnail
// that goes into a .ship, and the PNG export (SPEC-DATA §4, §5).
//
// Both want the same thing — the current view, at a size of their own choosing,
// read back as pixels — and both are rare, so this allocates its target, uses
// it and frees it rather than keeping one around.

// CaptureOpts configures one off-screen render.
type CaptureOpts struct {
	// W and H are the image size in pixels.
	W, H int
	// Transparent leaves the background out, so the ship can be dropped onto
	// something else. Without it the viewport's own gradient is drawn.
	Transparent bool
}

// Capture renders a scene off-screen and reads it back.
//
// It must be called with a GL context current and outside any other texture
// mode. The returned image is top-down, the way every image in this program is;
// render targets are stored the other way up and are flipped here.
func (r *Renderer) Capture(s *Scene, opts CaptureOpts) *image.RGBA {
	if opts.W <= 0 || opts.H <= 0 || s == nil {
		return nil
	}
	// Debug map views must not leak into project thumbnails or model exports.
	materialView := r.MaterialView
	r.MaterialView = 0
	defer func() { r.MaterialView = materialView }()
	rt := rl.LoadRenderTexture(int32(opts.W), int32(opts.H))
	defer rl.UnloadRenderTexture(rt)

	savedW, savedH := r.fbW, r.fbH
	r.SetFramebuffer(opts.W, opts.H)
	defer r.SetFramebuffer(savedW, savedH)

	vp := Viewport{X: 0, Y: 0, W: opts.W, H: opts.H}
	rl.BeginTextureMode(rt)
	if opts.Transparent {
		rl.ClearBackground(color.RGBA{})
	} else {
		r.ClearColor()
	}
	// The gradient is part of the window's look, not part of the ship. A
	// transparent capture skips it; an opaque one keeps it so the image matches
	// what was on screen.
	if opts.Transparent {
		r.drawSceneNoBackground(s, vp)
	} else {
		r.DrawViewport(s, vp)
	}
	rl.EndTextureMode()

	img := rl.LoadImageFromTexture(rt.Texture)
	if img == nil {
		return nil
	}
	defer rl.UnloadImage(img)
	rl.ImageFlipVertical(img)

	pixels := rl.LoadImageColors(img)
	if len(pixels) < opts.W*opts.H {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, opts.W, opts.H))
	for y := 0; y < opts.H; y++ {
		for x := 0; x < opts.W; x++ {
			out.SetRGBA(x, y, pixels[y*opts.W+x])
		}
	}
	return out
}

// drawSceneNoBackground is DrawViewport without the gradient, for a capture
// that has to keep its alpha.
func (r *Renderer) drawSceneNoBackground(s *Scene, vp Viewport) {
	if vp.W <= 0 || vp.H <= 0 {
		return
	}
	ao := float32(0)
	r.shadedMat.GetMap(rl.MapOcclusion).Texture = r.whiteTex
	if r.prepareAO(s, vp) {
		ao = float32(s.AO)
		r.shadedMat.GetMap(rl.MapOcclusion).Texture = r.ambient.rt.Texture
	}
	rl.SetShaderValue(r.shaded, r.locAOStrength, []float32{ao}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shaded, r.locAOViewport,
		[]float32{float32(vp.X), float32(r.fbH - vp.Y - vp.H), float32(vp.W), float32(vp.H)}, rl.ShaderUniformVec4)
	r.begin3D(s.Camera, vp)
	r.drawShadedPass(s)
	// Overlays and translucent previews do not receive screen-space shadows.
	rl.SetShaderValue(r.shaded, r.locAOStrength, []float32{0}, rl.ShaderUniformFloat)
	r.drawEdgePass(s, vp)
	r.drawTranslucentPass(s, vp)
	r.drawOverlayPass(s, vp)
	r.end3D()
}
