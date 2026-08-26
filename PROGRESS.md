# Progress Journal

> Executor: append an entry per working session. Newest entry at the TOP. Keep entries honest — failed attempts and open bugs belong here, not just wins.

**Current state:** **M0 COMPLETE.** The app builds, runs, renders a real 3D scene with a working view cube, axis triad and hint bar, and can screenshot itself headlessly. Next up: **M1** (UI shell: widget kit, toolbar, tree panel, plane toggles).

---

## 2026-08-26 — M0: foundation (cgo+raylib, geom, mesh, passes, picking, headless)

**Done:**
- **Toolchain proven first, as instructed.** Go 1.26.3, WinLibs mingw64 gcc 15.2 (UCRT), Git 2.54 all present; a hello-world cgo binary built and ran before any project code. `git init`, `go.mod` (module `modeler`, go 1.26), `.gitignore`, and the full package skeleton of PLAN §4.
- **`internal/geom`** — `Vec2i` (exact int64 subunits), `Vec2`, `Vec3`, row-major `Mat4` (compose/invert/LookAt/Ortho/Perspective), `AABB`, the SPEC-GEOMETRY §1.3 tolerance table as the only home for epsilons, exact 2D predicates (orientation, on-segment, full segment-vs-segment classification with subunit-rounded intersection points via a 128-bit mul-div), grid snapping (`SnapGrid`/`SnapFine`/`SnapNone`), and the plane/face `Frame` convention including the fixed table for the three default planes.
- **`internal/geom/mesh`** — polygon faces with holes and stable `FaceUID`s, Newell face planes with caching, half-edge topology with crease classification, the SPEC-GEOMETRY §6.5 validation gate, Kahan-compensated volume, box and n-gon-prism primitives, weld, and rigid transforms that carry paint frames along.
- **`internal/render`** — flat-shading GLSL 330 shader with the §4 hemispheric light in view space, the ID pick shader, per-body GPU meshes with vertices duplicated per face, camera-facing edge ribbons with the eye-shrink z-fight guard, translucent plane quads, the vertical-gradient background, the turntable camera (orbit/pan/zoom-to-cursor, ortho + perspective, framing), the 220 ms cubic camera animator, and the 64×64 ID pick pass with center-out priority resolve.
- **`internal/scene`** — the 26-zone view cube (drawn as projected 3×3 grids per visible face, click to snap, drag to orbit 1:1, home button) and the depth-sorted axis triad.
- **`internal/ui`** — the full SPEC-UX §3 theme token set, the embedded Go Regular font rasterised at exact per-DPI pixel sizes, and the first stroke icons.
- **`internal/app` / `internal/io` / `cmd/modeler`** — the raylib-free `InputFrame` abstraction, the frame loop, the op-script format with per-op validation, and headless mode: `modeler.exe -headless -script x.json -out dir`.
- **`internal/apptest`** — the golden harness: builds the real exe once, runs each case as a real headless process, compares PNGs under the SPEC-RENDER §10 tolerance policy, writes diff images on failure, `GOLDEN_UPDATE=1` regenerates.

**Verified:**
- `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean. `go test ./...` — **98 tests pass**, no failures.
- Coverage by package: geom 25, geom/mesh 23, render 23, scene 12, io 9, apptest 6.
- **Goldens** (read, not just generated): `docs/shots/m0_iso.png`, `m0_front.png`, `m0_perspective.png`, `m0_pick.png`. Baselines committed under `testdata/golden/`.
- **Determinism**: `TestRenderIsDeterministic` runs the same script twice and requires *byte-identical* output — it passes, so the tolerance policy has slack it does not currently need.
- **Picking**: `TestPickPassResolvesGeometry` probes seven known points in a front-framed scene and checks each resolves to the right element and body — face centre, top edge, a prism side face, a crease on the rotated box, empty space, a bottom edge, and a corner. The corner probe resolves to a *vertex* even though an edge and a face also cover that pixel, which is the specced forgiveness order (verts > edges > faces).
- **Frame cost** (`-bench 300`, each frame including a forced GPU readback so the number is honest): 1280×720 mean **2.32 ms**, p95 3.31 ms, max 6.02 ms. 1920×1080 mean **4.25 ms**, p95 6.59 ms, max 8.98 ms. Budget is 16.6 ms — comfortable headroom for 60 fps.

**Decisions/deviations:** six entries appended to `docs/DECISIONS.md` — V-01 camera animation interpolates azimuth/elevation instead of quaternion slerp (the camera is roll-free by construction and slerp introduces roll mid-path); V-02 view cube zones are hit-tested analytically against the drawn sub-quads instead of a second ID render (cheaper and exactly consistent with what is drawn); V-03 `FacePaint` is declared in `geom/mesh` so `mesh.Face` can name it without inverting the dependency direction; V-04 a float ear-clipper lives in `geom/mesh` for off-lattice boolean output, with the integer sketch triangulator still due in M2; V-05 `render` imports `ui` for theme tokens rather than duplicating the palette; V-06 the `pick` op and `-bench` flag were added to the headless toolset as executor verification tools.

**Bugs found and fixed during the milestone** (all had visible symptoms, all are now covered by tests):
1. **Euler characteristic ignored hole loops** — a square tube reported genus 0. A face with a hole is an annulus, not a disk, so the validator now subtracts hole loops: `V − E + F − H = 2 − 2g`.
2. **cgo pointer panic on mesh upload** — `UploadMesh` panicked with "Go pointer to unpinned Go pointer" because Go's cgo check scans the *entire* heap object a C pointer lands in, and the `rl.Mesh` was a field of `BodyGPU` alongside unpinned slices. `rl.Mesh` is now its own allocation.
3. **rlgl's matrix stack is shared across matrix modes** — pushing projection then modelview and popping in the same order swapped the two matrices, so every 2D element drawn after the 3D pass (cube, triad, hint bar) vanished. The renderer now saves and restores the 2D matrices explicitly.
4. **Camera ops fought each other mid-animation** — `camera.view front` followed by `camera.frame` framed the half-finished transition instead of the front view, because framing read the live camera rather than the animation's destination. Camera moves now compose from the animation target. This was a genuine user-facing bug: pressing F during a view transition did the same thing.
5. **Translucent planes double-drawn** — the two-sided plane quad was drawn twice, compounding its alpha. Backface culling is disabled for it instead.
6. **Font asked for glyphs Go Regular does not have** — 7 of 109 codepoints were missing. The atlas is now trimmed to what the typeface carries and the missing symbols (home, chevrons, tick, cross) are stroke icons per D-11.

**Open issues:**
- DPI scaling is implemented (scale rounded to 1.0/1.25/1.5/2.0, fonts rasterised at the exact scaled pixel size) but this machine reports 1.0, so the other three steps are **untested in practice**. Worth a manual check when a scaled display is available.
- Face hover currently produces no viewport highlight — only the hint bar naming what is under the cursor. A body-wide tint was removed because it misrepresented the selection granularity; the per-face highlight belongs to the overlay pass and lands with the selection model in M6.
- The gizmo pass (depth cleared, drawn on top) has its drawing helpers but no pass of its own yet — nothing needs it until the extrude arrow in M3, which matches the M0 checklist's "gizmos later".
- `app.LoadTestScene` is a stand-in document (two boxes, a 16-gon prism, a rotated box). M1 replaces it with the real document and the welcome state; M9 replaces it with the embedded sample ship.

**Next:** M1 — start with `internal/ui`: the frozen SPEC-UX §4 widget set and the layout (toolbar 40 px, tree panel 240 px, viewport, hint bar 26 px), then wire the plane rows with their eye toggles and confirm the tree ↔ viewport hover highlighting works both ways.

**Try it (user):**
```bash
c:\go\bin\go.exe run ./cmd/modeler
```
1. **Right-drag** in the viewport to orbit — it should feel level and precise, never rolling.
2. **Scroll** with the pointer over a corner of the hull: the point under the cursor should stay put as you zoom.
3. **Click a face of the view cube** (top-right) — the camera should glide, not snap; then **drag the cube** to orbit directly.
4. Click the small **home button** under the cube to return to the framed isometric view.
5. Press **O** to toggle perspective, then **F** to re-frame.
6. Hover over the model and watch the **hint bar** (bottom-left) name the exact face, edge or vertex under the cursor.

---

## Entry template (copy for each session)

```
## YYYY-MM-DD — M<N>: <short title>
**Done:** bullets of what actually landed (files/packages touched)
**Verified:** build/test/vet status, test counts, bench numbers, shot filenames produced & READ
**Decisions/deviations:** anything logged to DECISIONS.md, baseline regens, fallbacks taken
**Open issues:** known bugs, flaky tests, TODOs with owners (usually "next session")
**Next:** the first concrete task of the next session
**Try it (user):** up to 6 steps to feel what changed, e.g. "Run modeler.exe → press S → click Front plane → draw a rectangle → E → drag the arrow"
```
