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
out vec3 fragPositionView;
flat out vec3 fragNormalView;
flat out vec4 fragFaceKey;

void main()
{
    fragTexCoord = vertexTexCoord;
    fragPositionView = vec3(matView * matModel * vec4(vertexPosition,1.0));
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
in vec3 fragPositionView;
flat in vec3 fragNormalView;
flat in vec4 fragFaceKey;

uniform sampler2D texture0;
uniform sampler2D propertiesMap;
uniform sampler2D normalAOMap;
uniform float useMaterial;
uniform float materialView;
uniform float materialFace;
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

vec3 pbrLight(vec3 base,vec3 n,vec3 v,vec3 l,float rough,float spec) {
    vec3 h=normalize(v+l);
    float nl=max(dot(n,l),0.0),nv=max(dot(n,v),0.001),nh=max(dot(n,h),0.0);
    float a=rough*rough, a2=a*a;
    float d=a2/(3.14159265*pow(nh*nh*(a2-1.0)+1.0,2.0));
    float k=pow(rough+1.0,2.0)/8.0;
    float g=(nv/(nv*(1.0-k)+k))*(nl/(nl*(1.0-k)+k));
    vec3 f=(vec3(.04)+vec3(.96)*pow(1.0-max(dot(h,v),0.0),5.0))*spec;
    return ((1.0-f)*base/3.14159265+d*g*f/max(4.0*nv*nl,.001))*nl;
}

vec3 materialNormal(vec3 n,vec3 sampleNormal) {
    vec3 dp1=dFdx(fragPositionView),dp2=dFdy(fragPositionView);
    vec2 duv1=dFdx(fragTexCoord),duv2=dFdy(fragTexCoord);
    float det=duv1.x*duv2.y-duv1.y*duv2.x;
    if(abs(det)<1e-12)return n;
    vec3 t=normalize((dp1*duv2.y-dp2*duv1.y)/det);
    t=normalize(t-n*dot(n,t));
    vec3 b=normalize((dp2*duv1.x-dp1*duv2.x)/det);
    b=normalize(b-n*dot(n,b)-t*dot(t,b));
    return normalize(mat3(t,b,n)*sampleNormal);
}

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
    vec4 props=texture(propertiesMap,fragTexCoord);
    vec4 normalAO=texture(normalAOMap,fragTexCoord);
    // Evaluate derivatives outside divergent material branches.
    vec3 mappedNormal=materialNormal(n,normalize(normalAO.rgb*2.0-1.0));
    if(useMaterial>.5 && props.a>.5) {
        vec3 linearBase=pow(base,vec3(2.2));
        vec3 v=normalize(-fragPositionView);
        float rough=clamp(props.g,.045,1.0);
        vec3 energy=linearBase*.32*normalAO.a*openness;
        energy+=pbrLight(linearBase,mappedNormal,v,L1,rough,props.r)*2.5;
        energy+=pbrLight(linearBase,mappedNormal,v,normalize(vec3(-.6,.25,.7)),rough,props.r)*.8;
        rgb=mix(pow(max(energy,vec3(0.0)),vec3(1.0/2.2)),base,flatShade);
    }
    // Inspection works on unpainted faces too, using the channel defaults.
    if(useMaterial<.5 || props.a<.5) {
        props=vec4(1.0,230.0/255.0,128.0/255.0,0.0);
        normalAO=vec4(128.0/255.0,128.0/255.0,1.0,1.0);
    }
    float faceKey=floor(fragFaceKey.r*255.0+.5)+256.0*floor(fragFaceKey.g*255.0+.5);
    if(materialFace<0.0 || abs(faceKey-materialFace)<.5) {
        if(materialView>0.5 && materialView<1.5)rgb=base;
        else if(materialView<2.5 && materialView>1.5)rgb=vec3(props.r);
        else if(materialView<3.5 && materialView>2.5)rgb=vec3(normalAO.a);
        else if(materialView<4.5 && materialView>3.5)rgb=vec3(props.b);
        else if(materialView<5.5 && materialView>4.5)rgb=vec3(props.g);
        else if(materialView>5.5)rgb=normalAO.rgb;
    }
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
