# SPEC-GEOMETRY — Numbers, Kernel Algorithms & Data

Two numeric worlds, one rule each: **2D sketch space is integer-exact** (closedness is decided here — no epsilons), **3D model space is float64 with authored values grid-snapped** (robust booleans are Manifold's job — D-03/D-04).

## 1. Numeric model

### 1.1 Sketch space (2D, exact)
- Coordinates are `int64` **subunits**; **256 subunits = 1 world unit (u)**. 1 u = "1 pixel-unit" of a ship.
- Grid snapping = rounding to multiples of 256 (or 64 for ¼-u fine snap). Default sketch grid 1 u.
- Exact predicates (`geom/pred2d.go`): `Cross(a,b,c) int64` sign (orientation), on-segment test, `SegSegIntersect` classification (proper cross / T-touch / collinear-overlap / none) with rational intersection point **rounded to subunits** (a documented rounding site, always followed by weld).
- Range bound: sketch coords clamped to ±4,194,304 subunits (±16,384 u) → cross terms ≤ ~7·10¹³, shoelace sums over ≤10⁴ segments stay far inside int64. Enforce the clamp at input with a friendly toast, assert in debug.

### 1.2 Model space (3D, float64)
- `Vec3{X,Y,Z float64}` in world units. **Snap-on-author policy:** every value produced by a tool (extrude depth, gizmo move, plane lift of sketch points, 90° rotations) is snapped to the 1/256-u grid before entering the document. Off-grid verts appear only where geometry truly demands it (boolean intersections, free rotation, draft miters) — that's correct, not drift.
- 90° rotations are exact coordinate permutations/negations (never trig).

### 1.3 Tolerance table (centralize in `geom/tol.go`; nothing else defines epsilons)
| Name | Value | Used for |
|---|---|---|
| `WeldDist` | 1/512 u (half subunit) | vertex welding after booleans/rounding |
| `PlanarDist` | 1/256 u | face planarity check (§7.2) |
| `NormalEps` | 1e-9 | unit-vector / parallelism comparisons |
| `VolumeRelTol` | 1e-6 | volume assertions in tests |

## 2. Core types (geom, geom/mesh)

```go
type Vec2i struct{ X, Y int64 }   // sketch space, subunits
type Vec3 struct{ X, Y, Z float64 } // model space, world units
type Mat4, AABB                     // float64
type Frame struct{ O, U, V, N Vec3 } // plane frame for sketches/paint; U,V,N orthonormal

type Mesh struct {
    Verts []Vec3
    Faces []Face
}
type Face struct {
    ID        FaceUID     // global unique, never reused: (bodyID<<32 | faceSeq)
    Loops     [][]int     // Loops[0] outer CCW viewed from outside; rest holes CW
    SrcFace   FaceUID     // lineage through booleans (paint/selection continuity)
    NonPlanar bool        // set by direct edits (§7.2)
    Paint     *FacePaint  // §8, nil = body color
}
```
- Face plane (point+normal) derived from loop verts (Newell's method for robustness) and cached; invariant: all loop verts within `PlanarDist` of it unless `NonPlanar`.
- Half-edge adjacency (`mesh.Topology`) built on demand & cached, invalidated on edit: edge picking, edge moves, boundary walks, crease detection, validation.

## 3. Sketch model

```go
type Sketch struct {
    ID, Name; Visible bool
    Plane    SketchPlane  // builtin plane enum OR face ref {bodyID, faceUID} + Frame snapshot
    Entities []Entity     // Line{A,B Vec2i} | Rect{A,B} | Circle{C Vec2i, R int64, Segs int} | Ref (projected outline, UX §10)
}
```
- The Frame snapshot makes face-sketches robust: if the face later vanishes, the sketch keeps working (dangling ref only loses snap-to-face-edges).
- `Segments(s) []Seg2i` expands entities (Rect→4, Circle→N chords; circle verts = `C + R·(cos,sin)` rounded to subunits — exactness resumes after rounding).
- Frame basis convention (shared with faces, §8.3): `ref = world axis with min |dot(N, axis)|` (ties X>Y>Z); `U = normalize(ref − N(ref·N))`, `V = N × U`. Deterministic. Builtin planes use natural axes chosen so sketches read upright from the normal-on camera (fix once in a table + test).

## 4. Region engine (closed-profile detection — R3)

Input: `[]Seg2i`. Output: `[]Region{Outer []Vec2i CCW, Holes [][]Vec2i CW}` + `[]OpenEnd Vec2i` + per-region source-entity sets (for stable side-face IDs, §5.5).

Pipeline (all integer-exact; rerun on every sketch edit — target <2 ms at 500 segments):
1. **Quantize** endpoints to subunits (already ints); drop zero-length segs.
2. **Split**: pairwise `SegSegIntersect` (O(n²) with bbox prefilter is fine); proper crossings and T-touches split segs at the (rounded) point. **Collinear overlaps**: project onto the shared line, merge into interval union, re-emit maximal non-overlapping pieces (users retrace edges constantly — this must be rock solid).
3. **Weld**: hash nodes by exact coord; merge; drop duplicate segments (same node pair).
4. **Graph**: nodes + undirected edges; per node sort incident directed edges by exact pseudo-angle comparator (octant + cross sign — no atan2).
5. **Face walk** (DCEL traversal): for directed edge e, `next(e)` = the CW-most edge around `head(e)` after `reverse(e)`; walk all faces; signed area by int64 shoelace. Exactly one unbounded face (largest |negative area|); bounded CCW cells are region candidates, bounded CW cells are hole boundaries.
6. **Nesting**: containment tree via exact point-in-polygon (ray parity with `Cross`); a CCW cell with its directly-nested CW cycles ⇒ Region with holes; CCW cells inside holes are separate island regions (support one level; test).
7. **Open ends** = nodes of degree 1 (degree ≥3 is legal — T-joints fine).

Property: any segment borders ≤2 bounded faces; entity→region mapping recorded for §5.5.

## 5. Extrude construction (R4–R8)

Input: regions (sketch frame), direction (Normal/Reverse/Symmetric), depth `d` (snapped, ≥1 subunit), draft `θ` ∈ [−45°,+45°], result mode. Output: closed prism Mesh(es) → then §6 per result mode. Multiple selected regions extrude as one command (their prisms union via Manifold if they touch; else multi-body for New / sequential ops otherwise).

### 5.1 Frames & lift
`dir = +N` (Normal) / `−N` (Reverse). Lift `P3(p2, t) = O + U·(p2.x/256) + V·(p2.y/256) + N·t`, then **snap to grid** for axis-aligned frames (builtin planes, faces with axis-aligned N); arbitrary face frames keep float results (correct, off-grid allowed).

### 5.2 Caps
Near cap = region (outer CCW + holes), facing −dir; far cap = offset region (§5.3) reversed, facing +dir. Caps are polygon-with-holes `Face`s. Triangulation (render + MeshGL handoff) in `geom/sketch2d/triangulate.go`: ear clipping with hole bridging (max-x-vertex visible-bridge), exact `Cross` predicates, O(n²) fine at sketch scale.

### 5.3 Draft = miter offset of the profile (integer, with validity clamp)
δ = d·tanθ rounded to subunits. Positive θ shrinks the far cap ("taper"): offset outer loop **inward** by δ, holes **outward** (material thins consistently). Per loop vertex: new position = intersection of the two adjacent edges translated by δ along their left normals (rational → rounded to subunits). Then **validate topology**:
- every offset edge keeps direction (`dot(new_edge, old_edge) > 0`),
- no self-intersection (reuse §4 split pass in check-only mode),
- no loop-loop crossing, no hole escaping its outer.
If invalid → **binary-search the max valid δ** (12 iterations), report the achieved angle for the UX clamp display (UX §9.3). Spiky-vertex miters are bounded by the validity check — no separate miter limit in v1.

### 5.4 Sides & symmetric
- One side quad per profile edge: `[a0, b0, b1, a1]` — miter offset keeps near/far edges parallel ⇒ quads planar (assert ≤ `PlanarDist` in tests).
- **Symmetric**: base cap at `−(d/2)·dir`, far at `+(d/2)·dir`, both caps offset by `(d/2)·tanθ` (widest at the sketch plane); still one planar quad per edge (equal offsets both ends of each side edge pair — verify in tests).

### 5.5 Face identity
Cap FaceUIDs: 2 fresh. Side faces: one per source profile edge, keyed (sketch entity, segment index) → stable within the body for paint/selection; re-extrudes make independent bodies with fresh UIDs.

### 5.6 Result modes
New → add body (auto color). Add/Subtract/Intersect → §6 against targets per UX §9.4; on any boolean error the command aborts atomically (UX §11.3).

## 6. Booleans via Manifold (R8, R12)

Manifold (elalish/manifold, Apache-2.0) guarantees manifold outputs and tracks per-triangle provenance — exactly what we need. We bind its C API (`manifoldc`) in `internal/geom/csg` (the only cgo-to-manifold package).

### 6.1 One-time build & vendoring (M4; full policy in DECISIONS D-12)
1. Ensure CMake (`cmake --version`; else `winget install Kitware.CMake`).
2. `git clone --depth 1 --branch <pinned v3.x tag> https://github.com/elalish/manifold` into a temp dir.
3. `cmake -G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release -DMANIFOLD_PAR=OFF -DMANIFOLD_TEST=OFF -DBUILD_SHARED_LIBS=OFF` (+`-DMANIFOLD_CROSS_SECTION=OFF` if supported — we don't use cross sections), then `cmake --build`.
4. Vendor `libmanifold*.a`, `include/manifold/**` headers, `LICENSE` → `third_party/manifold/`; commit. Record tag + flags in D-12.
5. Smoke test: cube ∪ offset cube → volume check, before writing any converter code.
Timebox: two honest attempts; then Appendix A fallback.

### 6.2 Binding surface (~15 functions; keep it minimal)
`manifold_meshgl` construct from verts/tris + `runIndex`/`runOriginalID`; `manifold_of_meshgl`; status check (`manifold_status`); `manifold_boolean` (ADD/SUBTRACT/INTERSECT); `manifold_get_meshgl`; properties (volume/surface area) for cross-checks; `manifold_reserve_ids`; destructors. Wrap in Go types with finalizer-free explicit `Close()` (deterministic frees), errors never panics.

### 6.3 Converters (the real work of M4)
**Body → MeshGL:** triangulate each polygon face (§5.2 triangulator on its frame projection); emit one *triangle run per source face* with `runOriginalID = reserved ID mapped 1:1 to FaceUID` (keep the map alive per op). float32 verts (MeshGL is float32 — acceptable: inputs are grid-snapped, well within float32 exactness for ±16k u… 4,194,304 subunit grid steps at 1/256: representable exactly for |coord| ≤ 32,768 u ✓).

**MeshGL → Body:** for each output triangle, provenance = `runOriginalID` (Manifold propagates it through splits/merges). Group triangles by `(originalID, plane-key)` (plane key: quantized Newell normal + offset, `NormalEps`); region-grow across shared edges within a group; trace boundary loops (outer + holes by signed area in the source face's frame) → emit polygon `Face` inheriting `SrcFace` + `Paint` from the FaceUID map; fresh FaceUIDs for the fragments. Leftover ungroupable triangles become triangle faces (valid, cosmetic only — count them in tests; drive to ~zero on the matrix). Weld (`WeldDist`), then **validation gate** (§6.5).

### 6.4 Op semantics
- Union fold for multi-select; Subtract = target minus each tool sequentially (validating between); Intersect = pairwise fold.
- Empty results (e.g., subtract swallowed the body, intersect of disjoint) are **legal**: body is deleted with an explanatory toast, undo restores.
- Any Manifold error status / validation failure ⇒ command aborts atomically, document untouched, UX §11.3 toast; dev flag dumps inputs to `debug/csg/` as OBJ pairs for repro.

### 6.5 Validation gate (`mesh.Validate`, used after every mutating op, cheap)
- Combinatorially closed & 2-manifold: every edge in exactly 2 faces, opposite orientation; orientable; connected shells counted
- No degenerate faces (area < ~(1/512 u)²); all outer loops CCW-from-outside (signed volume of face fans consistent)
- Signed volume > 0; per-shell Euler sanity V−E+F = 2−2g, g≥0
- Cross-check volume vs Manifold's reported property (|Δ| ≤ `VolumeRelTol`) right after boolean ops

### 6.6 Performance
Bodies ≤ ~50k tris at the extreme; Manifold handles this in milliseconds-to-tens. Budget: typical op <100 ms; >120 ms runs async with cancel (UX §9.5). Previews never run CSG — they're cheap transformed meshes; CSG fires on commit only.

## 7. Direct edits (R10, R11)

### 7.1 Translations
Vertex/edge/face/body moves add a (snapped) delta to underlying verts (shared verts move once — edits are topological, via vert indices from selection). Command captures before/after for undo. Face plane caches invalidated.

### 7.2 Planarity policy
After a sub-body edit, each touched face re-checks planarity (max point-plane distance ≤ `PlanarDist`). Violators get `NonPlanar=true`: rendering + MeshGL handoff use their stored triangulation (fan/ear in the original frame projection); sketch-on-face and push/pull refuse with guidance (UX §12.3); paint keeps projecting via its stored frame (§8.3). Moving verts back within tolerance clears the flag.

### 7.3 Rotation
90° about world axes: exact permutation/negation (grid-preserving — the UX-default detents). 15°/free: float rotate about pivot (no rounding after — off-grid is the honest result; toast once per session: *"Free rotation leaves the pixel grid"*). Paint frames transform by the same rigid motion, so paint stays glued.

## 8. Face paint (R13)

### 8.1 Storage
```go
type FacePaint struct {
    Res   int         // 1|2|4|8|16|32 texels per unit (chip at creation)
    Texel float64     // world units per texel = 1/Res, FIXED at creation
    Frame Frame       // persistent paint anchor
    Img   *image.RGBA // alpha 0 = unpainted (body color shows through)
    Off   image.Point // texel index of Img origin (allows growth)
}
```
`paint` owns the RGBA source of truth; `render` mirrors it into GPU textures (nearest filter) with dirty-rect uploads.

### 8.2 Creation
On first stroke: Frame = face frame (§3 convention) with O = bbox-min corner of the face in that frame; `Texel = 1 / Res`. A chip is a **density**, not a count: at 8, a texel is an eighth of a unit on every face of every body, so a pixel is the same physical size wherever it is painted (V-128). Img covers the face bbox +1 texel margin, so its size now grows with the face — an allocation that would exceed the §8.3 cap is **refused**, naming a chip that fits, rather than clamped to a picture too small to cover the face. **Texel never rescales implicitly** — constant pixel density is the pixel-art contract, from both directions. Explicit resample (UX §13.2) rebuilds at a new Res via nearest.

A ship saved before V-128 stores a `Texel` worked out from its face's longest side, and keeps it: the file loads and renders exactly as it did. Only its `Res` field is then stale, so the program reports a picture's density as `1/Texel` rather than reading that field.

### 8.3 Mapping & painting
`uv(p) = ((p−O)·U / Texel, (p−O)·V / Texel)` → floor to texel ints. Cursor→texel: pick pass gives FaceUID; ray ∩ face plane → p → uv. Strokes rasterize brush squares in texel space with UV-space line interpolation between input samples. Painting outside Img grows it (adjusting Off) up to a 1024² cap (toast if capped). Renderer samples the same mapping — one source of truth in `paint/mapping.go`, unit-tested round-trip (uv→world→uv identity).

### 8.4 Persistence across modeling (the "paint survives" contract)
- Boolean fragments inherit `SrcFace` via Manifold provenance (§6.3) ⇒ fragments **share** the source `FacePaint` object (multiple faces referencing one paint; strokes write into the shared image — visually correct and simple; v1 keeps it shared).
- Body translations/rotations transform `Frame` by the same rigid motion (exact for translations & 90° rotations).
- Vertex edits don't touch Frame — paint stays put in the plane (correct feel).
- Dedicated test: paint checkerboard → subtract a through-hole → surviving texels identical in world position (TESTING §3.4).

## 9. Volume & measures
`mesh.Volume` (divergence-theorem tet sum, Kahan-compensated) and `mesh.AABB` — used by tests (analytic prisms; §6.5 cross-check vs Manifold's property), Through-All depth, frame-all camera, and the stats line.

---

## Appendix A — Fallback: in-house BSP CSG (only if Manifold is abandoned per D-04)

Budget XL. Acceptance = the same TESTING §3 matrix. Summary of the fully-considered design (expand into a working module if triggered):

1. **Representation:** tagged triangles `{V0,V1,V2, Src FaceUID}`; all coordinates snapped to 1/256-u grid on input; predicates float64 with `WeldDist` epsilon discipline (or reintroduce exact int predicates on subunit ints if epsilon-tuning thrashes — coordinates ≤±16k u fit int128 accumulation via `math/bits`).
2. **BSP (csg.js structure):** node plane from first inserted tri; classify verts Front/Back/Coplanar/Spanning; spanning tris split, intersection points rounded to subunits, exact-degenerate slivers dropped immediately. `ClipTo`/`Invert`/`AllTris`; Union `a.clip(b); b.clip(a) + coplanar rules`; Subtract via invert-union-invert; Intersect via De Morgan.
3. **Coplanar policy (the butt-join make-or-break):** coplanar tris bucket to the node; same-orientation keeps the target mesh's copy per csg.js coplanarFront rules; opposite-orientation pairs cancel for union / cap cuts for subtract. The win condition is deciding "coplanar" consistently (snap-rounded grid makes true coplanarity common and detectable).
4. **Reassembly:** weld (`WeldDist`) → drop degenerates → **t-junction repair** (spatial-hash verts onto edges, split, re-fan) → merge coplanar same-`Src` triangles into polygon faces (region grow, boundary trace, holes by signed area) → `mesh.Validate` gate; fail ⇒ op error, document untouched.
5. **Order of implementation:** matrix cases as failing tests → predicates → split/clip → union only → subtract → intersect → reassembly → fuzz with the voxel/volume oracle (TESTING §3.5).
