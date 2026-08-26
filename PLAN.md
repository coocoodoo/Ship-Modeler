# Modeler — Master Build Plan

**Product:** "Modeler" — a polished, OnShape-inspired 3D modeling tool purpose-built for **pixel-art spaceships**, written in **Go (libraries welcome, cgo allowed)**, running on Windows.

**Executor:** Claude Opus 5 (or any Claude Code agent) working in `C:\Modeler`. **Author of this plan:** Claude Fable 5, 2026-08-26, with the user (chris.colon.1992@gmail.com).

**Status:** PLANNING COMPLETE — no code written yet. Execution starts at Milestone M0.

> **User directive (2026-08-26):** do NOT constrain to pure Go — use libraries as needed to accelerate development. The stack below reflects that: raylib for the engine layer, Manifold for booleans.

---

## 0. Instructions to the executing agent (read first, re-read when resuming)

1. **Source of truth.** This file + the `docs/` specs define the product. Read `PLAN.md` fully before any code. Before starting a milestone, read the spec sections it references. If you deviate from a spec, append the deviation + reason to `docs/DECISIONS.md` (§ Deviations log).
2. **Machine facts** (verified 2026-08-26): Go **1.26.3** at `c:\go\bin\go.exe` (windows/amd64). Git 2.54 on PATH. **gcc (WinLibs mingw64, POSIX/UCRT) on PATH** — cgo works; verify with `gcc --version` and a hello-world cgo build in M0 before anything else. 16 logical CPUs. Active desktop session (GPU rendering + hidden-window screenshots work). `C:\Modeler` is the project root (this repo). Network available for `go get`, `git clone`, `winget`.
3. **Build/test commands** (PowerShell):
   ```powershell
   $env:CGO_ENABLED = "1"                       # cgo is REQUIRED (raylib, manifold)
   c:\go\bin\go.exe build ./...
   c:\go\bin\go.exe test ./...                  # geometry/model tests are raylib-free and fast
   c:\go\bin\go.exe vet ./...
   c:\go\bin\go.exe run ./cmd/modeler           # run the app
   c:\go\bin\go.exe build -ldflags "-s -w -H windowsgui" -o modeler.exe ./cmd/modeler   # release; dev builds keep the console
   ```
   First raylib build compiles vendored C sources — it's slow once (minutes), then cached. Never clear the Go build cache casually.
4. **Milestone protocol.** Work milestones strictly in order (M0→M9; M10 is backlog). Within a milestone: write the listed geometry/unit tests **before** implementing the geometry they test. A milestone is done only when every acceptance checkbox is checked, `build`/`test`/`vet` are green, and headless screenshots for it exist under `docs/shots/`.
5. **Journal.** After every working session update `PROGRESS.md`: what was done, verification evidence (test counts, shot filenames), open issues, next step, and a short "Try it" checklist for the user. Check off acceptance boxes in this file as you go.
6. **Git.** `git init` in M0. Commit at least once per milestone (more is better), message prefix `M<N>:`. Local repo only — never push anywhere. Vendored third-party code (`third_party/`) gets committed.
7. **Keep it runnable.** The app must build and run at the end of every session. Hide unfinished tools (feature flag / not registered in toolbar) rather than shipping broken UI.
8. **Self-verification is mandatory.** You cannot watch the window, so give yourself eyes: `modeler.exe -headless -script <file> -out <png>` opens a **hidden** window, runs an op script through the command bus, renders, saves PNG(s), exits (SPEC-RENDER §9, TESTING §4). Build this in M0 and use it after every visual change. Read the PNGs you produce. Pure-logic packages (`geom`, `model`, `io` core) stay raylib-free so `go test` on them needs no window at all.
9. **Dependencies.** Chosen stack (add more only with a DECISIONS entry; ask the user only if something big or license-awkward):
   - `github.com/gen2brain/raylib-go/raylib` — window, input, GPU 3D/2D, render textures, text (cgo; vendors raylib C sources, zlib license)
   - **Manifold** (`github.com/elalish/manifold`, Apache-2.0) — robust mesh booleans via its C API (`manifoldc`), built once with CMake+mingw and vendored as a static lib under `third_party/manifold/` (DECISIONS D-04/D-12; build recipe in SPEC-GEOMETRY §6.1)
   - `github.com/ncruces/zenity` — native file dialogs (MIT)
   - `golang.org/x/image` — `font/gofont/goregular` TTF bytes for the UI font (BSD)
   - Go standard library. Licenses listed in README credits (M9).
10. **Fallback discipline.** If a spec item resists two honest implementation attempts, implement the documented fallback (each risky area lists one), note it in DECISIONS.md, and move on. Do not thrash. Biggest one: if Manifold won't build/bind sanely, fall back to the fully-specced in-house CSG (SPEC-GEOMETRY Appendix A).
11. **Stuck / scope questions.** Ask the user only at the OPEN QUESTIONS gates (§7), for new heavyweight dependencies, or when a spec conflict is discovered. Otherwise proceed.
12. **Code style.** `gofmt` + `go vet` clean. Package dependency direction per §4 — enforce it (no upward imports; raylib types must not leak into `geom`/`model`). No panics in library packages — return errors, wrap with `%w`. Exported geometry functions get doc comments + tests. Keep files under ~500 lines where reasonable.
13. **Performance.** Budgets in `docs/SPEC-RENDER.md` §8. Don't optimize past them without profiling evidence.

---

## 1. Product vision

A tool where someone who has never used CAD can build a crisp, low-poly, hand-painted spaceship in their first session — and where the modeling loop (sketch on a face → pull it out → paint it) feels as immediate as a pixel-art editor.

**Pillars** (tiebreakers for every design decision, in order):
1. **Utterly easy** — every state tells you what to do next (hint bar), everything pickable pre-highlights on hover, defaults are always sensible, snapping does what you meant.
2. **Very polished** — 60 fps, animated camera transitions, MSAA-clean viewport, consistent theme, zero jank, no dead ends, undo works everywhere.
3. **Crisp / pixel-native** — grid-snapped geometry, nearest-neighbor paint, hard edges, integer world units.
4. **Robust** — booleans either succeed and validate, or fail cleanly leaving the document untouched. Never corrupt user work.

**Core user loop:** pick plane → sketch closed profile → extrude (drag arrow) → click a flat face → sketch again → extrude add/cut → move verts/edges to shape it → paint faces → export.

## 2. Scope

### v1.0 features (user requirements → where specified)
| # | Requirement | Spec |
|---|---|---|
| R1 | Top/Front/Right planes; hideable, never deletable | UX §5, §7 |
| R2 | Sketch on those planes (lines, rects, circles; snapping) | UX §8, GEOM §3 |
| R3 | Closed-profile detection (no open ends) gates extrude; open ends flagged | UX §8.6, GEOM §4 |
| R4 | Extrude straight along sketch-plane / face normal | UX §9, GEOM §5 |
| R5 | LMB-drag arrow gizmo to set extrude depth | UX §9.2 |
| R6 | Draft angle in degrees, adjustable live | UX §9.3, GEOM §5.3 |
| R7 | Symmetric extrude (both directions) | UX §9.3, GEOM §5.4 |
| R8 | Extrude result choice: **New body / Add / Subtract / Intersect** | UX §9.4 |
| R9 | Sketch on any flat face of a body | UX §10 |
| R10 | Move edges & vertices (direct editing) | UX §12, GEOM §7 |
| R11 | Transform tool: move whole body or selected verts/edges/faces | UX §12 |
| R12 | Boolean tool between bodies: union / subtract / intersect | UX §11, GEOM §6 |
| R13 | Paint faces at 16 / 32 / 128 / 256 / 512 px resolutions, nearest-neighbor crisp | UX §13, GEOM §8 |
| R14 | XYZ axis triad, bottom-right of viewport | UX §6.2 |
| R15 | View cube, top-right: click faces/edges/corners to reorient, drag to orbit | UX §6.1 |
| R16 | Left side panel listing all bodies (plus planes & sketches): show/hide, rename, color, delete | UX §7 |
| R17 | Undo/redo everywhere | DATA §3 |
| R18 | Save/load project, export OBJ(+textures)/STL/PNG render | DATA §4–5 |

### Explicit non-goals for v1 (do not build; some are M10 backlog)
Full parametric regeneration (edit sketch → downstream features rebuild), fillets/chamfers, splines/arcs-as-true-curves, assemblies/joints, STEP/Parasolid import-export, multi-document windows, plugins/scripting, collaborative anything, macOS/Linux packaging (keep code portable, but don't test there), sounds.

### Product behaviors worth naming (so they don't get lost)
- **Pseudo-parametric:** features (sketches, extrudes, booleans, edits) are recorded in an append-only feature log for undo/debugging/forward-compat, but the document is direct-modeled meshes. Sketches persist as objects after being consumed (auto-hidden), and can be re-edited and re-extruded manually — but downstream bodies do **not** regenerate. The UI must never pretend otherwise (see UX §8.8).
- **Push/pull:** selecting a flat face and dragging its normal arrow re-extrudes that face (add when dragging out, subtract when dragging in). This is the single most important "easy mode" tool for blocky ships. (UX §12.5)
- **Paint survives modeling:** face paint is anchored to a persistent per-face frame; boolean fragments inherit the source face's frame (via Manifold's provenance IDs) so pixels stay glued. (GEOM §8)

## 3. Technology decisions (summary — full rationale in docs/DECISIONS.md)

| Area | Decision |
|---|---|
| Language/toolchain | Go 1.26, `CGO_ENABLED=1` with the installed WinLibs mingw64 gcc, module name `modeler` |
| Engine layer | **raylib-go**: window, input, GPU rendering (z-buffer, MSAA 4×, render textures), text — one dependency, vendored C sources compile via cgo, no SDKs |
| 3D viewport | GPU passes over raylib/rlgl: shaded pass (flat-shading shader), edge overlay pass, overlay/gizmo pass, plus an on-demand 64×64 **ID pick pass** around the cursor for exact face/edge/vertex picking (SPEC-RENDER) |
| UI toolkit | In-house immediate-mode widget kit drawn with raylib 2D (widget list frozen in UX §4; Dear ImGui consciously rejected — D-02) |
| Geometry numbers | Sketch space: exact `int64` subunits (1/256 u) — arrangement/regions are integer-exact. Model space: `float64` with all *authored* values grid-snapped; tolerances table in GEOM §1 |
| 2D sketch engine | In-house: integer planar arrangement → region extraction; in-house miter offset for draft (small, fully specced) |
| Booleans | **Manifold** (the kernel OpenSCAD uses) via a ~15-function cgo binding to `manifoldc`; guarantees manifold output + per-face provenance. In-house BSP CSG spec retained as Appendix A fallback |
| File dialogs | `ncruces/zenity` (in-app fallback browser if it misbehaves) |
| Project file | `.ship` = zip(manifest.json, document.json, paint/*.png, thumbnail.png) |
| Fonts/icons | Embedded Go Regular TTF via raylib `LoadFontFromMemory`; icons drawn procedurally as vector strokes in `ui/icons.go` |

## 4. Architecture

```
cmd/modeler/main.go        flags: -headless -script <json> -out <png>; panic-recovery + crash-save
internal/geom              math: vectors (Vec2i sketch / Vec3 float64 model), Mat4, AABB, 2D exact predicates
internal/geom/sketch2d     arrangement, region extraction, miter offset (draft), triangulation w/ holes
internal/geom/mesh         Body mesh model: verts, polygon faces w/ holes, half-edge adjacency, weld, validate, volume
internal/geom/csg          Manifold binding (cgo): MeshGL⇄mesh converters, boolean ops, provenance mapping
internal/model             Document: bodies, sketches, planes, selection, command bus, undo, feature log, IDs
internal/render            raylib-backed viewport renderer: passes, pick pass, camera math, headless shots
internal/scene             viewport composition: grid, planes, triad, view cube, overlays, camera animator
internal/ui                immediate-mode widgets, theme tokens, layout, toasts, hint bar
internal/sketch            sketch mode: tools, snapping, inference, live regions
internal/tools             extrude, boolean, transform/gizmos, push-pull, paint controller
internal/paint             face textures, palette, brush ops (image-side; GPU upload lives in render)
internal/io                .ship save/load, autosave/recovery, OBJ/MTL/STL/PNG export, settings, op-script runner
internal/app               the App: mode state machine, input routing, frame loop
third_party/manifold/      vendored static libs + headers + LICENSE (built once in M4)
assets/                    embedded font bytes, palettes (go:embed)
testdata/                  golden PNGs, CSG cases, op scripts
docs/                      these specs + shots/
```

**Dependency rules** (imports may only point left→right): `geom` ← `model` ← {`sketch`,`tools`,`paint`,`io`} ← `app`; `render`/`scene` see `geom`+`model`; `ui` is standalone. **raylib may be imported only by `render`, `scene`, `ui`, `tools` (gizmo drawing), `app`** — `geom`, `model`, `io`, `paint`-core stay raylib-free so their tests run fast with no window. `geom/csg` is the only cgo-to-manifold package.

**Input abstraction:** `app` consumes an `InputFrame` struct (mouse pos/buttons/wheel/keys/chars) produced from raylib polling in real runs and from script/e2e feeders in tests. No package below `app` polls raylib input directly.

**Command bus:** every document mutation is a `Command` executed through `model.Bus` (Do/Undo via targeted snapshots, drag coalescing). Tools never mutate `Document` directly. This gives R17 for free and is what the op-script runner drives.

## 5. Milestones

Sizes: S ≈ a focused session, M ≈ 1–2 sessions, L ≈ 2–4, XL ≈ budget generously. Order is dependency-driven; do not reorder without a DECISIONS entry.

---
### M0 — Foundation: cgo+raylib up, camera, passes, picking, headless (L)
Specs: GEOM §1–2, RENDER all, UX §6.
- [x] Verify toolchain: hello-world cgo build; then `git init`, `go.mod` (module `modeler`, go 1.26), package skeleton per §4, `.gitignore` (modeler.exe; keep docs/shots)
- [x] raylib window 1600×900 resizable "Modeler", dark clear color, MSAA 4× hint, VSync; embedded Go Regular font rendering at 13/15 px (crisp at DPI scales 1.0/1.25/1.5/2.0)
- [x] `internal/geom`: vectors/Mat4/AABB, 2D exact predicates (int64 cross), grid snap helpers — unit tests
- [x] `internal/render`: shaded pass (flat-shading GLSL 330 shader: per-face normal via `flat` varying, hemispheric fill light per RENDER §4), edge overlay pass (camera-offset trick per RENDER §5), overlay pass (gizmos later); render-to-texture proven
- [x] **Pick pass**: 64×64 ID render around cursor (pick-matrix projection), readback, decode to {type, bodyID, faceID/edge/vert} per RENDER §6; prove on a test mesh (log hovered face)
- [x] Camera: turntable orbit (up-locked)/pan/zoom-to-cursor; ortho (default) + perspective; animated transitions (220 ms cubic) per RENDER §7
- [x] View cube (top-right): 26 hover zones via its own tiny pick render, click → animated snap, drag → orbit; home button. Axis triad (bottom-right): mirrors orientation, display-only
- [x] Headless mode: `-headless -script x.json -out y.png` (hidden window) runs op script (io.ScriptRunner) → PNG; golden harness with tolerance policy (TESTING §5: MSAA off in test shots)
- [x] Golden shots: iso/front/perspective of a test mesh → `docs/shots/m0_*.png`
**Accept:** 60 fps orbit; cube/triad behave; pick pass returns correct IDs at cursor; goldens pass twice in a row (determinism check); PROGRESS updated. — **MET** (frame cost 2.3 ms @720p / 4.3 ms @1080p vs a 16.6 ms budget; picking verified by `TestPickPassResolvesGeometry`; goldens bit-identical across runs via `TestRenderIsDeterministic`). Cube zone picking is analytic rather than a second ID render — see DECISIONS V-02.

### M1 — UI shell: panels, theme, widgets, planes (M)
Specs: UX §3–7.
- [x] `internal/ui`: exactly the widget set of UX §4 (button, icon button, toggle/eye, slider, drag-number field, text field, tree row, color swatch, chip group, tooltip, toast, floating card, hint bar, modal, shortcut overlay). Theme tokens from UX §3. No additions without DECISIONS entry.
- [x] Layout: top toolbar (tools stubbed/disabled), left tree panel (collapsible sections), viewport, bottom hint bar
- [x] Default planes Top/Front/Right as translucent tinted bounded quads with labels; eye toggles in tree; **no delete/rename affordance** (R1); click plane in tree or viewport → highlight (planes join the pick pass)
- [x] Toasts + hint bar working; settings (`%APPDATA%\Modeler\settings.json`): window size/pos, persisted
- [x] Golden shots incl. plane visibility toggling via script
**Accept:** shell matches UX §2 mock; planes toggle; hover states everywhere; goldens. — **MET** (goldens `m1_shell`, `m1_tree_collapsed`, `m1_planes_hidden/restored`, `m1_tree_click`, `m1_plane_delete_toast`; flows driven by synthetic clicks and asserted on document dumps, not pixels alone). `internal/model` — the document, command bus and undo — was built here rather than later, because every tree action must be undoable (DECISIONS V-10).

### M2 — Sketch mode (L)
Specs: UX §8, GEOM §3–4.
- [x] Enter: click a plane (viewport or tree) → camera animates normal-on, model dims, sketch grid + origin appear
- [x] Tools: Select, Line (click-chain), Rectangle, Circle (center-radius, segment count field, default 16); Delete entity; Esc semantics per UX §8.7
- [x] Snapping: grid (default 1 u), endpoint, midpoint, H/V inference with dashed guides + glyphs (Alt disables) per UX §8.4
- [x] Region engine (GEOM §4): quantize → split at intersections → weld → merge collinear overlaps → planar-graph face walk → regions with holes; live translucent fill of closed regions; **open endpoints drawn as red rings** (R3)
- [x] Sketch objects: tree section, rename/hide/delete, re-enter edit; per-sketch entity storage (Line/Rect/Circle logical form)
- [x] Unit tests: ≥12 region cases (square, nested hole, figure-8, shared-edge butt, overlapping rects, open chain, crossing lines, duplicate segments, collinear overlap, circle-in-rect, sliver, degenerate zero-length) — **16 cases**
- [x] Golden shots: sketch with fills + open-end markers
**Accept:** draw "rect + circle hole" and see two regions filled; tests+goldens green. — **MET** (`TestRectAndCircleGiveTwoRegions`; goldens `m2_rect_circle`, `m2_open_ends`, `m2_closed_region`, `m2_tools`). The region engine runs the 500-segment budget case in 0.38 ms against GEOM §4's 2 ms. Sketch overlays draw on top of the model rather than depth-tested against it — see DECISIONS V-12.

### M3 — Extrude to new body (L)
Specs: UX §9, GEOM §5.
- [x] Region selection (click fill, Shift multi); toolbar Extrude / `E`
- [x] Preview solid (translucent) + **arrow gizmo**: LMB-drag along normal, grid-snapped depth, Ctrl = fine (¼ u); floating drag-number field synced both ways; flip by dragging through zero or ⇄ button (R4, R5)
- [x] Options card: Direction (Normal/Reverse/**Symmetric**), **Draft** −45°..45° slider+field with live preview & clamp warning, Result (New only until M4 — others visible, disabled, tooltip "coming with M4"), Through-All toggle
- [x] Solid construction (GEOM §5): caps (polygon+holes, triangulated), planar side quads under draft via miter offset; per-face stable IDs; weld; validator must pass
- [x] Body appears in tree ("Body N", auto color); shaded + crease/silhouette edges; sketch auto-hides with toast (UX §9.5)
- [x] Undo/redo across the whole flow; volume unit tests vs analytic (box, n-gon prism, with draft, symmetric)
**Accept:** draw square → E → drag out a drafted crate; goldens for straight/drafted/symmetric.

### M4 — Booleans via Manifold (L; highest-leverage integration — test-first)
Specs: GEOM §6, TESTING §3.
- [ ] **Write the acceptance test matrix first** (TESTING §3: ~25 named cases incl. flush butt-joins, coincident faces, edge/vertex touches, slivers, chained fuzz with volume oracle)
- [ ] Build Manifold once: install CMake if absent (winget Kitware.CMake), `git clone` at a pinned v3.x release tag, MinGW Makefiles, `MANIFOLD_PAR=OFF`, tests off; produce static `libmanifold.a`+`libmanifoldc.a`; vendor libs+headers+LICENSE into `third_party/manifold/`; record exact tag + flags in DECISIONS D-12
- [ ] `geom/csg` cgo binding (~15 functions): MeshGL in/out, `manifold_boolean` (union/difference/intersect), status check, properties (volume), `manifold_reserve_ids` + `runOriginalID` per input face-run for provenance
- [ ] Converters: Body mesh (polygon faces) → triangulated MeshGL with per-face runs; result MeshGL → Body: group triangles by (originalID, plane) → merge into polygon faces with holes → inherit FaceUID/paint → weld → `mesh.Validate` gate
- [ ] On any error/empty result: op returns error, document untouched, red toast (UX §11.3), repro geometry dumped to `debug/csg/` under a dev flag
- [ ] Wire into Extrude Result: **New / Add / Subtract / Intersect** with target rules of UX §9.4 (R8); Boolean tool for existing bodies with keep-tools option (R12)
- [ ] Fallback ladder if Manifold won't build/bind after two honest attempts: implement in-house BSP CSG per GEOM **Appendix A** against the same matrix (that path is fully specced; budget XL)
**Accept:** full matrix green incl. the flush butt-join family; 200-op random fuzz keeps validity + volume within oracle tolerance; UX flows work.

### M5 — Sketch on faces & push/pull (M)
Specs: UX §10, §12.5.
- [ ] Click flat face → "Sketch" → camera normal-on, face plane becomes sketch plane (persistent frame snapshot), face edges become snap references; "Project outline" button copies face boundary into sketch entities
- [ ] Extrude from face sketch: defaults Result=Add with owning body; Subtract when cutting into the body per UX §9.4; through-all cut works
- [ ] **Push/pull**: select face → normal arrow → drag out = Add, drag in = Subtract (auto), grid-snapped, full preview; implemented as extrude-of-face-outline through the command bus
**Accept:** build a stepped hull + cut windows entirely via face sketches and push/pull; goldens.

### M6 — Selection, direct edit, transform (L)
Specs: UX §12, GEOM §7.
- [ ] Unified selection: bodies / faces / edges / verts via pick pass (priority verts>edges>faces, radii per RENDER §6); Shift add, Ctrl toggle; box-select (one full-viewport ID render on drag-release, rect scan)
- [ ] Move gizmo: 3 axes + 3 planes + screen-plane center; grid snap; numeric offset field; single-face selection leads with the normal arrow (push/pull consistency)
- [ ] Vertex/edge/face moves (R10): mesh edit + non-planar policy GEOM §7.2 (auto-triangulate flagged faces, subtle warn chip); body move; Ctrl+D duplicate; Del delete with undo toast
- [ ] Rotate rings: 90° detents (Shift 15°, Alt free) around selection pivot (R11); 90° paths exact/grid-preserving (GEOM §7.3)
- [ ] Coalesced drag undo (one step per drag)
**Accept:** stretch a hull by box-selecting nose verts and dragging; rotate a wing 90°; goldens + tests.

### M7 — Paint mode (L)
Specs: UX §13, GEOM §8.
- [ ] Paint mode `P`: right palette panel (embedded default 32-color palette, custom colors via HSV picker, recents, `.hex` Lospec import)
- [ ] Resolution chips **16/32/128/256/512** (R13) — texel density fixed at first paint from face bbox (GEOM §8.2); per-face resize prompt with nearest resample
- [ ] Tools: Pencil, Eraser (to body color), Fill, Eyedropper (Alt), brush sizes 1/2/4; live **texel cursor** outline on the 3D face; strokes via pick-pass→face-frame UV; oblique-angle hint + "Face view" button
- [ ] Textures: `image.RGBA` source of truth in `paint`, GPU texture per painted face with nearest filter, dirty-rect `UpdateTextureRec` uploads; per-stroke dirty-rect undo (DATA §3.3); texture on/off view toggle
- [ ] **Paint persistence test:** paint a face, subtract a hole through it, pixels stay glued (GEOM §8.4)
**Accept:** paint hull plating + cockpit glow on a ship; goldens at multiple resolutions.

### M8 — Files: save/load, autosave, export (M)
Specs: DATA §4–6.
- [ ] `.ship` zip save/load/save-as via zenity dialogs; round-trip tests incl. paint + view state; format version 1 with tolerant reader
- [ ] Autosave every 120 s when dirty + crash-save on panic; recovery prompt on next launch; recent files on a welcome/empty state screen (UX §14)
- [ ] Export: OBJ+MTL+PNGs (nearest-note in README), binary STL, PNG screenshot current-view at 1×/2×/4× with transparent-background option
- [ ] Release build documented & tested: `-H windowsgui`, exe runs on a machine without Go (static mingw runtime: `-extldflags=-static` if needed)
**Accept:** kill the process mid-edit → relaunch recovers; OBJ verified structurally in tests (counts, materials, UVs) and opens in an external viewer.

### M9 — Polish pass to v1.0 (L)
Specs: UX §15 checklist is the work list. Highlights:
- [ ] Hover/pressed/disabled audit on every control; cursor set per tool; tooltips with shortcut labels; complete keyboard map (UX §16) + `?` overlay
- [ ] Camera/UI animation audit; empty states; first-run sample ship (built by op script, shipped embedded); error toast copy pass
- [ ] Perf profile: steady 60 fps with sample ship + paint; pick-pass ≤0.5 ms; boolean ops async >120 ms with spinner (RENDER §8)
- [ ] README.md (user-facing quickstart + shortcuts + library credits/licenses); version v1.0.0 in title bar & about card
- [ ] Full-app e2e script: build the sample ship end-to-end headlessly, compare final golden + volume
**Accept:** the user runs through UX §15 "feel checklist" and signs off. Tag `v1.0.0`.

### M10 — Backlog (post-v1, priority order — do not start without user)
1. **Live mirror symmetry** (model+paint across X plane) — the single biggest win for spaceships; strongly recommended next
2. Bevel/chamfer edges at 45° (blocky-friendly; Manifold makes this tractable)
3. Sketch re-edit → parametric-lite regeneration of the owning extrude
4. glTF export; texture atlas export
5. Palette from PNG; Aseprite palette import
6. Turntable GIF export (stdlib gif)
7. Measure tool; body drag-reorder in tree; lasso select; hollow/shell (Manifold offset)

## 6. Risk register

| Risk | Sev | Mitigation |
|---|---|---|
| Manifold build on mingw (CMake, C++17) fails or fights | Med | Pinned release tag; minimal flags (no tests, no parallel backend); half-session timebox per attempt; **fully-specced in-house CSG fallback (GEOM Appendix A)**; binding is ~15 functions |
| cgo/raylib first-build friction (mingw quirks) | Low-Med | gcc verified present; retire in M0 hour one with hello-cgo + raylib window; keep Go build cache |
| GPU golden-test nondeterminism (driver AA/rounding) | Low-Med | MSAA off in test shots; small per-pixel tolerance + outlier allowance (TESTING §5); same-machine baselines |
| Boolean edge cases despite Manifold (empty results, provenance loss on coplanar merges) | Med | Adversarial matrix is acceptance, not Manifold's reputation; validation gate + atomic command abort keeps documents safe |
| Draft offset self-intersects on concave profiles | Med | Topology-validity check + binary-search clamp with visible "clamped" state (GEOM §5.3) |
| Paint alignment lost across booleans | Med | Persistent per-face frames + Manifold `runOriginalID` provenance + dedicated test (GEOM §8.4) |
| UI scope creep | Med | Widget list frozen at M1; additions need DECISIONS entry |
| Pick-pass readback stalls pipeline | Low | 64×64 on-demand @ ≤30 Hz hover; full-viewport ID render only on box-select release |
| Agent context drift across sessions | Med | This protocol: specs are truth, PROGRESS journal, re-read rule, per-milestone commits |

## 7. Open questions for the user (defaults chosen — silence = proceed with defaults)

| # | Question | Default in this plan |
|---|---|---|
| Q1 | Projection default: orthographic (pixel-precise feel) or perspective? | **Ortho** default, `O` toggles perspective |
| Q2 | Add **64×64** to the paint resolutions (you listed 16/32/128/256/512)? | Not added; trivial to add |
| Q3 | Circle tool = regular N-gon (crisp low-poly). Default segment count? | **16**, editable per circle |
| Q4 | Rotation detents 90° (Shift 15°, Alt free) OK? | Yes as specced |
| Q5 | Prioritize **mirror symmetry mode** right after v1.0? | Yes — top of M10 |
| Q6 | App display name stays "Modeler"? (binary `modeler.exe`) | Yes |
| Q7 | Default palette: custom 32-color built-in + Lospec `.hex` import? | Yes |
| Q8 | Minimum window size 1280×720? | Yes |

## 8. Reading order for the executor

1. This file (again, fully)
2. `docs/DECISIONS.md` — why the stack is what it is; do-not-relitigate list
3. `docs/SPEC-UX.md` — the product; every flow, layout, theme token, shortcut
4. `docs/SPEC-GEOMETRY.md` — numbers, sketch engine, extrude, Manifold binding, edits, paint mapping (+ Appendix A fallback CSG)
5. `docs/SPEC-RENDER.md` — passes, picking, camera, budgets, headless
6. `docs/SPEC-DATA.md` — document model, commands/undo, .ship, exports, settings
7. `docs/TESTING.md` — how everything is verified; CSG matrix; golden workflow
