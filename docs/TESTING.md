# TESTING — Verification Strategy

The executor cannot watch the app run; these tests are how the product is *known* to work. Geometry correctness is enforced by exhaustive unit tests, visuals by golden screenshots, flows by op-script e2e, and the whole thing by per-milestone user "Try it" checklists in PROGRESS.md.

## 1. Layers

| Layer | Packages | Style | Needs window? |
|---|---|---|---|
| Pure logic | `geom`, `geom/*`, `model`, `paint` (mapping), `io` (formats) | table-driven unit tests, fast | No |
| CSG integration | `geom/csg` (cgo→Manifold) | acceptance matrix §3 + fuzz | No |
| Visual | `render`, `scene`, `ui`, tools | golden shots via headless exe §4 | Hidden window (desktop session ok) |
| Flows | whole app | op-script e2e §4 | Hidden window |
| Feel/perf | whole app | benchmarks §6 + human checklist §7 | Yes (user) |

Race detector: `go test -race` on `model` and the async-boolean path (goroutine → main-thread apply).

## 2. Unit tests (selected canon; grow freely)

- **geom/pred2d:** orientation/on-segment/intersection classification incl. collinear-overlap; property: rounding an intersection point never moves it > 1 subunit.
- **Region engine (GEOM §4)** — canonical cases, each asserting region count, hole count, open-end count, exact areas: square; rect+circle hole; nested islands (region-in-hole); figure-8 (shared vertex); two rects sharing an edge (butt); overlapping rects (3 regions); open chain (0 regions, 2 open ends); T-joint; retraced/duplicate segments; collinear partial overlap; crossing X lines (4 regions with outer frame); zero-length degenerate; 1-subunit sliver region.
- **Triangulation:** area preservation (Σtri = polygon area exact), holes bridged, no flipped triangles (all CCW).
- **Extrude (GEOM §5):** analytic volumes — box, 16-gon prism, drafted box (frustum formula), symmetric drafted, region-with-hole (tube); side-quad planarity ≤ `PlanarDist`; draft clamp: concave L-profile at excessive draft → clamped angle reported, output valid.
- **Offset (§5.3):** square inset/outset, L-shape, star (spiky miters), hole loops offset outward; validity checker catches self-intersection.
- **mesh.Validate:** accepts cube/tube/two-shell; rejects open box, flipped-face box, duplicated face.
- **Paint mapping (GEOM §8):** uv→world→uv identity; texel size invariance under body translate + 90° rotate; growth (Off adjustment) preserves world position of existing pixels.
- **model.Bus:** do/undo/redo sequences restore deep-equal documents; drag coalescing = one entry; failed command leaves document identical (pointer-swap atomicity probed with an injected failing command).
- **io:** .ship round-trip byte-identical `document.json`; tolerant reader on truncated/garbage zip (error, no panic); OBJ export structural checks (v/vt/f counts, material per painted face); settings round-trip.

## 3. CSG acceptance matrix (write FIRST in M4; each case: build inputs by code, run op, assert validator + expected volume ± `VolumeRelTol`, expected shell/genus where noted)

Union: 1 disjoint (two shells, V=A+B) · 2 overlap (V=A+B−A∩B analytic) · 3 contained (V=big) · 4 identical (V=A) · **5 flush butt-join** (box on box, shared face vanishes, one shell) · **6 partial flush butt** (small on big) · 7 edge-touch · 8 vertex-touch · 9 drafted prisms overlapping · 10 rotated-90 boxes · 11 free-rotated (off-grid) boxes · 12 1-subunit sliver overlap
Subtract: 13 through-hole (genus 1) · 14 blind pocket · **15 flush-wall cut** (tool face coplanar with target wall) · 16 exact-fit (tool≡target → legal empty) · 17 swallow (tool⊃target → legal empty) · 18 disjoint (V unchanged) · 19 16-gon cylinder hole in plate · 20 symmetric-prism cutter · 21 multi-shell target, tool spanning both shells
Intersect: 22 overlap (V=A∩B) · 23 disjoint (legal empty) · 24 contained (V=small)
Chain: 25 ten mixed sequential ops on one body, validator + Manifold-vs-ours volume cross-check after each step.

### 3.4 Paint persistence
Checkerboard-paint a face at 32 px → subtract a through-hole crossing it → assert: surviving fragments share the source FacePaint, and sampled world-space colors at probe points away from the hole are unchanged.

### 3.5 Fuzz with an independent oracle
Seeded PRNG, 200 ops: random grid-aligned boxes/prisms (occasionally free-rotated), random union/subtract/intersect onto an evolving body. After each op: `mesh.Validate` passes; our `mesh.Volume` vs Manifold's property agree ≤ `VolumeRelTol`; every 20th op, **voxel oracle**: sample a 96³ grid with an independent point-in-mesh parity test (ray-cast, no shared code with csg) over the analytic op history's expected occupancy — volume agreement ≤ 3%. Failures dump inputs to `debug/csg/` and print the seed.

## 4. Golden screenshots & op-script e2e

- Harness: `internal/apptest` with `TestMain` building `modeler.exe` once (temp dir, `CGO_ENABLED=1`), then each case = run `-headless -script testdata/scripts/<case>.json -out <tmp>` → compare PNGs to `testdata/golden/<case>/`.
- Compare per RENDER §10 tolerance: per-channel |Δ|≤3 for ≥99.7% px, mean |Δ|≤0.5. Env `GOLDEN_UPDATE=1` regenerates baselines (review the diff images it writes before committing!). Curated copies of milestone shots also go to `docs/shots/` for the visual diary.
- e2e scripts double as flow tests: every milestone adds at least one (M2 sketch-with-fills, M3 drafted crate, M4 boolean trio, M5 face-cut hull, M6 vert-stretch, M7 painted ship, M9 full sample ship). Script errors exit non-zero = test failure with the op index in the message.

## 5. What "green" means before any commit
`c:\go\bin\go.exe build ./... && test ./... && vet ./...` — zero failures, zero vet complaints. Golden failures require either a fix or a reviewed, PROGRESS-noted baseline regeneration (never blind `-update`).

## 6. Benchmarks (run per milestone, numbers into PROGRESS)
`BenchmarkRegionEngine500segs` (<2 ms) · `BenchmarkTriangulate1k` · `BenchmarkBoolean5kTris` (<100 ms typical) · `BenchmarkPickResolve` (<0.5 ms) · frame-time probe in headless (log render ms for the sample ship scene; alert >8 ms).

## 7. Human checklists
Each milestone's PROGRESS entry ends with a ≤6-item "Try it" list for the user (exact clicks, expected feel). M9 uses the full UX §15 polish checklist as the sign-off gate for v1.0.
