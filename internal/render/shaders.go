package render

// GLSL 330 shaders (SPEC-RENDER §3, §6.1).
//
// Attribute and uniform names follow raylib's defaults so rl.DrawMesh fills in
// the matrix uniforms and the diffuse map for us.

// shadedVS is shared by the shaded and pick passes.
//
// vertexColor carries the face's index within its body, packed into the red and
// green bytes. The shaded pass ignores it; the pick pass turns it into an ID.
const shadedVS = `#version 330

in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

uniform mat4 mvp;
uniform mat4 matModel;
uniform mat4 matNormal;
uniform mat4 matView;

out vec2 fragTexCoord;
flat out vec3 fragNormalView;
flat out vec4 fragFaceKey;

void main()
{
    fragTexCoord = vertexTexCoord;
    // Light in view space so the model reads the same while orbiting.
    vec3 worldNormal = normalize(vec3(matNormal * vec4(vertexNormal, 0.0)));
    fragNormalView = normalize(vec3(matView * vec4(worldNormal, 0.0)));
    fragFaceKey = vertexColor;
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
`

// shadedFS implements the hemispheric two-light model of SPEC-RENDER §3.
//
// The paint texture is composited over the body color (alpha 0 texels mean
// unpainted), then multiplied by the lighting term and the hover/selection
// tint. alphaScale drives translucent previews.
const shadedFS = `#version 330

in vec2 fragTexCoord;
flat in vec3 fragNormalView;

uniform sampler2D texture0;
uniform vec4 colDiffuse;
uniform vec4 tint;
uniform float alphaScale;
uniform float useTexture;
uniform float aoStrength;
uniform sampler2D aoTexture;
uniform vec4 aoViewport;
uniform float flatShade;

out vec4 finalColor;

const vec3 L1 = normalize(vec3(0.4, 0.8, 0.45));
const vec3 L2 = -L1;

void main()
{
    vec3 n = normalize(fragNormalView);
    float openness = 1.0;
    if (aoStrength > 0.0) {
        vec2 uv = (gl_FragCoord.xy - aoViewport.xy) / aoViewport.zw;
        openness = mix(1.0, texture(aoTexture, uv).r, clamp(aoStrength, 0.0, 1.0));
    }
    // Occlusion attenuates ambient light, leaving direct light and paint intact.
    float lit = 0.55 * openness + 0.45 * max(0.0, dot(n, L1)) + 0.15 * max(0.0, dot(n, L2));
    // flatShade 1 is the unlit view: every face at full brightness, AO off.
    lit = mix(lit, 1.0, flatShade);

    vec3 base = colDiffuse.rgb;
    if (useTexture > 0.5) {
        vec4 texel = texture(texture0, fragTexCoord);
        base = mix(base, texel.rgb, texel.a);
    }

    vec3 rgb = base * lit;
    rgb = mix(rgb, tint.rgb, tint.a);
    finalColor = vec4(rgb, colDiffuse.a * alphaScale);
}
`

// pickFS writes a flat ID color with no lighting, no texture and no blending.
//
// idBase is the pick table index of this draw call's first element; the packed
// face index in vertexColor selects within it. That indirection means the GPU
// mesh never has to be rebuilt when the table is renumbered between passes.
const pickFS = `#version 330

flat in vec4 fragFaceKey;

uniform float idBase;

out vec4 finalColor;

void main()
{
    float lo = floor(fragFaceKey.r * 255.0 + 0.5);
    float hi = floor(fragFaceKey.g * 255.0 + 0.5);
    float id = idBase + lo + hi * 256.0;

    float r = mod(id, 256.0);
    float g = mod(floor(id / 256.0), 256.0);
    float b = mod(floor(id / 65536.0), 256.0);
    finalColor = vec4(r / 255.0, g / 255.0, b / 255.0, 1.0);
}
`
