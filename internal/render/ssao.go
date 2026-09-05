package render

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Screen-space AO samples the depth of every opaque body together. It updates
// during edits and navigation, without baking or modifying document geometry.
// Depth is packed into RGB24 so the pass works on ordinary GL 3.3 RGBA8 targets.
// A depth-aware filter removes sampling noise without bleeding over silhouettes.
type ambientPass struct {
	depth, sample, blur rl.Shader
	material            rl.Material
	depthRT, rawRT, rt  rl.RenderTexture2D
	w, h                int
}

func newAmbientPass() *ambientPass {
	a := &ambientPass{
		depth:  rl.LoadShaderFromMemory(shadedVS, aoDepthFS),
		sample: rl.LoadShaderFromMemory("", aoSampleFS),
		blur:   rl.LoadShaderFromMemory("", aoBlurFS),
	}
	a.material = rl.LoadMaterialDefault()
	a.material.Shader = a.depth
	return a
}

func (a *ambientPass) resize(w, h int) {
	if a.w == w && a.h == h {
		return
	}
	a.unloadTargets()
	a.w, a.h = w, h
	a.depthRT = rl.LoadRenderTexture(int32(w), int32(h))
	a.rawRT = rl.LoadRenderTexture(int32(w), int32(h))
	a.rt = rl.LoadRenderTexture(int32(w), int32(h))
	for _, t := range []rl.Texture2D{a.depthRT.Texture, a.rawRT.Texture, a.rt.Texture} {
		rl.SetTextureFilter(t, rl.FilterPoint)
		rl.SetTextureWrap(t, rl.WrapClamp)
	}
}

func (a *ambientPass) unloadTargets() {
	if a.w == 0 {
		return
	}
	rl.UnloadRenderTexture(a.depthRT)
	rl.UnloadRenderTexture(a.rawRT)
	rl.UnloadRenderTexture(a.rt)
	a.w, a.h = 0, 0
}

func (a *ambientPass) close() {
	a.unloadTargets()
	// UnloadMaterial owns its shader/maps; detach shared/default textures first.
	a.material.Shader = rl.Shader{}
	a.material.GetMap(rl.MapDiffuse).Texture = rl.Texture2D{}
	rl.UnloadMaterial(a.material)
	rl.UnloadShader(a.depth)
	rl.UnloadShader(a.sample)
	rl.UnloadShader(a.blur)
}

func aoSceneRadius(s *Scene) float64 {
	b := s.SceneBounds()
	if !b.Valid() {
		return 1
	}
	return math.Max(0.2, math.Min(5, b.Max.Sub(b.Min).Len()*0.1))
}

// prepareAO preserves the current framebuffer, including nested headless shots
// and PNG exports. AO never sees planes, grids, wires, or transparent previews.
func (r *Renderer) prepareAO(s *Scene, vp Viewport) bool {
	if s.AO <= 0 || s.Flat || len(s.Bodies) == 0 {
		return false
	}
	if r.ambient == nil {
		r.ambient = newAmbientPass()
	}
	a := r.ambient
	rl.DrawRenderBatchActive()
	target := rl.GetActiveFramebuffer()
	proj, view := rl.GetMatrixProjection(), rl.GetMatrixModelview()
	fw, fh := r.fbW, r.fbH
	a.resize(vp.W, vp.H)
	r.SetFramebuffer(vp.W, vp.H)
	rl.EnableFramebuffer(a.depthRT.ID)
	rl.Viewport(0, 0, int32(vp.W), int32(vp.H))
	rl.DisableScissorTest()
	rl.ClearBackground(color.RGBA{R: 255, G: 255, B: 255})
	r.begin3D(s.Camera, Viewport{W: vp.W, H: vp.H})
	rl.DisableColorBlend()
	rl.EnableBackfaceCulling()
	for _, b := range s.Bodies {
		if b.GPU != nil && b.GPU.uploaded && b.Alpha >= 0.999 {
			rl.DrawMesh(*b.GPU.rlMesh, a.material, toRLMatrix(b.Transform))
		}
	}
	r.end3D()

	p := s.Camera.Proj(vp.Aspect())
	inv, _ := p.Invert()
	setFloat := func(shader rl.Shader, name string, v float64) {
		rl.SetShaderValue(shader, rl.GetShaderLocation(shader, name), []float32{float32(v)}, rl.ShaderUniformFloat)
	}
	for _, shader := range []rl.Shader{a.sample, a.blur} {
		rl.SetShaderValue(shader, rl.GetShaderLocation(shader, "resolution"),
			[]float32{float32(vp.W), float32(vp.H)}, rl.ShaderUniformVec2)
		rl.SetShaderValueMatrix(shader, rl.GetShaderLocation(shader, "inverseProjection"), toRLMatrix(inv))
		setFloat(shader, "radius", aoSceneRadius(s))
	}
	rl.SetShaderValueMatrix(a.sample, rl.GetShaderLocation(a.sample, "projection"), toRLMatrix(p))
	// Fullscreen passes use pixel coordinates; fragment coordinates are also
	// texture coordinates, so no render-texture Y flip can desynchronize AO.
	rl.SetMatrixProjection(rl.MatrixOrtho(0, float32(vp.W), float32(vp.H), 0, -1, 1))
	rl.SetMatrixModelview(rl.MatrixIdentity())
	draw := func(rt rl.RenderTexture2D, shader rl.Shader, source rl.Texture2D) {
		rl.EnableFramebuffer(rt.ID)
		rl.BeginShaderMode(shader)
		rl.DrawTexture(source, 0, 0, rl.White)
		rl.EndShaderMode()
		rl.DrawRenderBatchActive()
	}
	draw(a.rawRT, a.sample, a.depthRT.Texture)
	rl.SetShaderValueTexture(a.blur, rl.GetShaderLocation(a.blur, "depthTexture"), a.depthRT.Texture)
	draw(a.rt, a.blur, a.rawRT.Texture)
	rl.EnableColorBlend()
	rl.EnableFramebuffer(target)
	r.SetFramebuffer(fw, fh)
	rl.Viewport(0, 0, int32(fw), int32(fh))
	rl.SetMatrixProjection(proj)
	rl.SetMatrixModelview(view)
	return true
}

const aoDepthFS = `#version 330
out vec4 finalColor;
void main() {
    // Integer RGB packing retains 24 bits with normalized 8-bit attachments.
    float d = floor(clamp(gl_FragCoord.z, 0.0, 1.0) * 16777214.0);
    finalColor = vec4(mod(d, 256.0), mod(floor(d / 256.0), 256.0), floor(d / 65536.0), 255.0) / 255.0;
}
`

const aoDepthHelpers = `
uniform vec2 resolution;
uniform mat4 inverseProjection;
uniform float radius;
float depthAt(sampler2D tex, vec2 uv) {
    vec3 b = floor(texture(tex, uv).rgb * 255.0 + 0.5);
    return dot(b, vec3(1.0, 256.0, 65536.0)) / 16777214.0;
}
vec3 positionAt(vec2 uv, float depth) {
    vec4 p = inverseProjection * vec4(uv * 2.0 - 1.0, depth * 2.0 - 1.0, 1.0);
    return p.xyz / p.w;
}
`

const aoSampleFS = `#version 330
uniform sampler2D texture0;
uniform mat4 projection;
out vec4 finalColor;
` + aoDepthHelpers + `
void main() {
    vec2 uv = gl_FragCoord.xy / resolution;
    float depth = depthAt(texture0, uv);
    if (depth >= 1.0) { finalColor = vec4(1.0); return; }
    vec3 p = positionAt(uv, depth);
    vec2 texel = 1.0 / resolution;
    vec2 ux = vec2(texel.x, 0), uy = vec2(0, texel.y);
    vec3 left = p - positionAt(uv-ux, depthAt(texture0, uv-ux));
    vec3 right = positionAt(uv+ux, depthAt(texture0, uv+ux)) - p;
    vec3 down = p - positionAt(uv-uy, depthAt(texture0, uv-uy));
    vec3 up = positionAt(uv+uy, depthAt(texture0, uv+uy)) - p;
    vec3 dx = abs(left.z) < abs(right.z) ? left : right;
    vec3 dy = abs(down.z) < abs(up.z) ? down : up;
    vec3 n = normalize(cross(dx, dy));
    vec3 ref = abs(n.z) < 0.9 ? vec3(0,0,1) : vec3(0,1,0);
    vec3 tangent = normalize(cross(ref, n));
    vec3 bitangent = cross(n, tangent);
    // Stable interleaved sampling; no frame-to-frame random flicker.
    float rotation = 6.2831853 * fract(dot(mod(floor(gl_FragCoord.xy), 4.0), vec2(0.0625, 0.25)));
    float occlusion = 0.0;
    for (int i = 0; i < 32; i++) {
        float fi = float(i) + 0.5;
        float z = sqrt(1.0 - fi/32.0);
        float r = sqrt(fi/32.0);
        float angle = fi * 2.39996323 + rotation;
        vec3 dir = tangent * (r*cos(angle)) + bitangent * (r*sin(angle)) + n*z;
        float reach = radius * (0.15 + 0.85 * fract(fi * 0.61803399));
        vec3 samplePos = p + n * (radius * 0.015) + dir * reach;
        vec4 projected = projection * vec4(samplePos, 1.0);
        vec2 sampleUV = projected.xy / projected.w * 0.5 + 0.5;
        if (projected.w <= 0.0 || any(lessThan(sampleUV, vec2(0))) || any(greaterThan(sampleUV, vec2(1)))) continue;
        float sd = depthAt(texture0, sampleUV);
        if (sd >= 1.0) continue;
        vec3 surface = positionAt(sampleUV, sd);
        float rangeWeight = 1.0 - smoothstep(radius*0.35, radius, length(surface-p));
        occlusion += (surface.z > samplePos.z + radius*0.012 ? 1.0 : 0.0) * rangeWeight;
    }
    float openness = pow(clamp(1.0 - occlusion / 32.0, 0.0, 1.0), 2.2);
    finalColor = vec4(vec3(openness), 1.0);
}
`

const aoBlurFS = `#version 330
uniform sampler2D texture0;
uniform sampler2D depthTexture;
out vec4 finalColor;
` + aoDepthHelpers + `
void main() {
    vec2 uv = gl_FragCoord.xy / resolution;
    float depth = depthAt(depthTexture, uv);
    if (depth >= 1.0) { finalColor = vec4(1.0); return; }
    vec3 p = positionAt(uv, depth);
    float sum = 0.0, weights = 0.0;
    for (int y = -2; y <= 2; y++) for (int x = -2; x <= 2; x++) {
        vec2 q = uv + vec2(x, y) / resolution;
        float d = depthAt(depthTexture, q);
        if (d >= 1.0) continue;
        vec3 pos = positionAt(q, d);
        float weight = exp(-float(x*x+y*y)/5.0) * exp(-abs(pos.z-p.z)/max(radius*0.04, 0.001));
        sum += texture(texture0, q).r * weight;
        weights += weight;
    }
    finalColor = vec4(vec3(sum/max(weights, 0.0001)), 1.0);
}
`
