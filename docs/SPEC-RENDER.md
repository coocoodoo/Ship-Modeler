# SPEC-RENDER — Viewport, Picking, Camera, Headless

All GPU work goes through raylib/rlgl inside `internal/render` + `internal/scene`. Everything here serves the pillars: MSAA-clean polish, exact picking, and screenshots the executing agent can read.

## 1. Frame composition (per frame, in order)

1. **Scene pass** (MSAA 4× when on): opaque bodies (shaded + textured faces) → edge overlay → translucent set (planes, region fills, extrude previews) sorted back-to-front → in-sketch overlays (grid, entities, snap glyphs)
2. **Gizmo pass**: depth cleared first — gizmos/handles always on top, still z-ordered among themselves
3. **View cube + triad** (own small viewports/render textures, top-right / bottom-right)
4. **UI pass**: raylib 2D — panels, cards, hint bar, toasts, cursors
The main scene renders into the default framebuffer at window resolution (no intermediate RT needed except picking/cube/headless).

## 2. Mesh data on GPU

- Per body: one raylib `Mesh` for faces (triangulated polygon faces; **vertices duplicated per face** so each carries its face's normal and paint UV — identical normals per face ⇒ flat shading with the standard pipeline), plus a CPU-built edge list (§5).
- Rebuild-on-change with a dirty flag (`UploadMesh` new / free old). Bodies are ≤~50k tris — full rebuild is fine; `UpdateMeshBuffer` optimization only if profiling demands (M9).
- Gizmo/preview transforms are model matrices — no rebuilds during drags. Vertex-drag edits rebuild the dragged body per frame (small meshes; budget-checked).
- Painted faces reference a per-face GPU texture (nearest filter, from `paint`'s RGBA via dirty-rect `UpdateTextureRec`); unpainted faces sample a 1×1 white texture tinted by body color.

## 3. Shading (one custom GLSL 330 shader for all bodies)

- Inputs: vertex pos/normal/uv, uniforms: mvp, normal matrix, view-space light dirs, base color, texture, tint (hover/selection), alpha (previews).
- Model: `lit = 0.55 + 0.45·max(0, n·L1) + 0.15·max(0, n·L2)`, L1 = normalize(0.4, 0.8, 0.45) in **view space** (headlight feel — model reads the same while orbiting), L2 = opposite fill. `rgb = texel.rgb⊕bodyColor (alpha-over) × lit × tint`.
- No shadows and no screen-space pass. **Baked ambient occlusion** instead (V-141, V-142): sixteen fixed hemisphere rays per rendered corner against the body's own triangles, the weighted hit fraction stored as an openness byte in the vertex colour's spare blue channel and multiplied into `lit`, scaled by the settings-owned `ao` strength (0.7 default, 0 disables, no rebuild to change it). Deterministic — a fixed ray pattern and no noise — which is what lets the goldens pin it.
- Reach is `0.35 × the body's bounding-box diagonal`, clamped to [1.5, 12] units. A fixed radius made occlusion vanish on anything bigger than itself; "local" only means something relative to the size of the thing being shaded.
- Because openness is a *corner* value the shader interpolates, a face needs interior corners for a falloff to exist on: `BuildBodyGPU` cuts each face on a barycentric grid at roughly a fifth of the AO radius, decided **per face** so two triangles sharing an edge inside a face split it identically and leave no seam. A face with nothing in front of it is not cut at all — on a convex body that is every face, so a plain box costs exactly what it always did. The tessellation is the render mesh only: the document mesh, picking, and the paint mapping never see it.

## 4. Theme in 3D
Viewport clear = vertical gradient (two-triangle background quad, UX §3 colors). Planes = two-sided translucent tinted quads + label billboards. Region fills = accentSoft translucent triangulations lifted 0.05 u above the sketch plane (no z-fighting).

## 5. Edge overlay

- CPU edge extraction per body (cached with topology): **crease** (dihedral > 25°), **boundary** (any non-2-manifold edge — shouldn't exist post-validate, but draw defensively), **non-planar face** perimeter. Blocky models: creases cover effectively every visible edge; view-dependent silhouettes are an optional M9 polish item if cheap.
- Drawing: edges as camera-facing ribbons (two tris per segment, constant screen width ~1.5 px; 3 px + accent color for selected). Depth-tested; anti-z-fight by shrinking edge vertices toward the eye by 0.05% of their eye distance (backend-agnostic, no polygon-offset dependency).
- Color: body color × 0.35, α 0.85 (UX §3 edgeLine). Selected body: accent edges + slight face tint lift (uniform).

## 6. Picking (exact, ID-based)

### 6.1 Pick pass
- On demand (mouse moved, throttled to ≤30 Hz; always right before processing a click): render the scene with a **pick shader** (flat unlit color = ID, blending OFF, MSAA OFF, textures OFF) into a **64×64 RenderTexture**, using a *pick projection*: the current projection pre-multiplied by an NDC translate+scale that maps the cursor's ±12 px neighborhood onto the full RT (classic gluPickMatrix; set via `rlSetMatrixProjection`).
- **Pick table, not bit-packing:** each drawn pickable appends `{kind, bodyID, faceUID | edgeRef | vertRef | gizmoPart | cubeZone | entityID | regionIdx | planeID}` to a per-pass slice; its ID = index+1 encoded RGBA8 (A=255). Decode by table lookup. Kind set: face, edge, vert, region, sketchEntity, plane, gizmo, cubeZone.
- Draw order & bias: faces/planes/regions first (normal depth), then edges as 5 px pick-ribbons (eye-shrink bias ×2), then verts as 9 px billboards (bias ×3), then gizmos after a depth clear.
- Readback (`rlReadTexturePixels`-equivalent via `LoadImageFromTexture`) → **priority resolve**: search center-out rings (r ≤ 12 px): first gizmo found wins; else nearest vert within 4.5 px; else nearest edge within 2.5 px; else the center pixel's face/region/plane. This reproduces "verts > edges > faces" forgiveness (UX §12.1).

### 6.2 Box select
On drag-release: one full-viewport ID render (same shader, MSAA off, full or half res) → scan the rect, collect unique IDs of the filtered kind (UX §12.1 chips), apply "fully inside" test per vert/edge/face by checking all its pixels' membership… simplified rule: an element is selected if **any** of its pixels are in the rect for verts/edges, and for faces only if **no** pixel of it lies outside the rect (approximate "fully inside"; document in code). One readback per box select — fine.

### 6.3 View cube picking
The cube renders to its own 84 px RT with a synced-orientation camera; a matching pick render colors its 26 zones (6 faces / 12 edges / 8 corners as sub-quads/strips). Hover highlights the zone; click animates to the zone's canonical orientation; LMB-drag on the cube feeds the orbit controller 1:1.

## 7. Camera

- State: `{target Vec3, azimuthDeg, elevationDeg, dist, orthoScale, projection}` — turntable, up locked +Y, elevation clamped ±89.5°, no roll ever.
- Ortho default (D-08): raylib `CAMERA_ORTHOGRAPHIC`, `fovy = orthoScale` (world units of viewport height). Perspective: fovy 45°.
- **Zoom-to-cursor**: solve target shift so the world point under the cursor stays under it after scale/dolly (both projections; smoothed ~120 ms exponential).
- Pan: screen-space delta → world delta at target depth. Orbit: azimuth/elevation delta per pixel (0.3°/px), inertia-free (precise CAD feel).
- **Animated transitions** (view cube, F-frame, normal-on): quaternion slerp of orientation + lerp of target/dist/orthoScale, 220 ms cubic ease-in-out, interruptible by any camera input (input takes over from the animation's current state, no snap).
- F-frame: fit selection (or all visible) AABB with 15% margin in both projections.

## 8. Performance budgets (60 fps machine budget ≈ 16.6 ms)

| Item | Budget |
|---|---|
| Scene + edges + translucents | ≤ 6 ms |
| UI pass | ≤ 2 ms |
| Pick pass (when it runs) | ≤ 0.5 ms |
| App logic / tools | ≤ 2 ms |
| Headroom (GC, driver) | remainder |

Steady-state render loop allocates ~zero per frame (reused buffers; check with pprof in M9). Boolean commits >120 ms run async with a spinner overlay (UX §9.5) — never on the render thread… (raylib is single-threaded GL: run Manifold in a goroutine, apply result on the main thread next frame).

## 9. Headless mode (the executor's eyes)

`modeler.exe -headless -script <ops.json> [-out <png|dir>] [-size 1280x720]`
- `SetConfigFlags(FLAG_WINDOW_HIDDEN)` before init; normal GL context (requires an active desktop session — true on this machine); VSync off; fixed virtual clock (each script step advances a synthetic frame time — animations resolve deterministically, `"settle"` op steps until animations finish).
- Script ops (format in SPEC-DATA §7) drive the same command bus + camera as the UI. `{"op":"shot","name":"m3_draft"}` renders one frame to an RT and writes `<name>.png` (into `-out` dir).
- Exit non-zero with a clear stderr message on any script error — golden tests depend on it.

## 10. Determinism policy for golden shots
Test shots run with MSAA **off**, gradient background on, fixed 1280×720 RT, fixed clock. GPU output can still vary per driver update — goldens are same-machine baselines compared with tolerance (TESTING §5): per-channel |Δ| ≤ 3 for ≥ 99.7% of pixels, mean |Δ| ≤ 0.5. `-update` regenerates. If a driver update shifts baselines, regenerate once and note it in PROGRESS.
