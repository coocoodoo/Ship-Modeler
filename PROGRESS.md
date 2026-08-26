# Progress Journal

> Executor: append an entry per working session. Newest entry at the TOP. Keep entries honest — failed attempts and open bugs belong here, not just wins.

**Current state:** **M1 COMPLETE.** The app has its full window chrome — toolbar, tree panel, hint bar, toasts — backed by a real document and command bus, so every tree action is undoable. Next up: **M2** (sketch mode: tools, snapping, the region engine).

---

## 2026-08-26 — M1: UI shell, document model and command bus

**Done:**
- **`internal/model`** — the document (bodies, sketches, the three permanent planes, sequences, feature log, camera state) and the command bus of SPEC-DATA §3: targeted-snapshot undo capped at 200, redo, drag coalescing (`BeginDrag`/`UpdateDrag`/`CommitDrag`/`CancelDrag`), change events that derived caches listen to, and the selection model. Ten commands cover everything the tree can do.
- **`internal/ui`** — the frozen SPEC-UX §4 widget set, all fifteen: button, icon button, toggle, eye, slider, drag-number field (scrub with snapping, click to type, Enter commits, Escape reverts), text field with caret and selection, tree row, colour swatch with an HSV popover, chip group, tooltip on a 600 ms delay, toast, floating card, hint bar, modal and the `?` shortcut sheet. Plus fifteen stroke icons.
- **`internal/app`** — the window chrome: a toolbar whose tools are present but disabled with tooltips naming the milestone that turns each on, undo/redo buttons wired to the bus, the Planes/Sketches/Bodies tree with counts and a stats footer, a collapsible panel, and the always-populated hint bar.
- **Bidirectional highlighting** — hovering a tree row outlines its body in the viewport, and hovering geometry names it in the hint bar. Selection shows as an accent silhouette in 3D and an accent row in the tree.
- **Plane name tags** — each visible plane labels itself in its axis colour at the corner nearest the camera.
- **`internal/io/settings.go`** — `%APPDATA%\Modeler\settings.json` with an atomic writer and a reader that returns defaults rather than failing, so preferences can never stop the app from starting.

**Verified:**
- `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean. `go test ./...` — **166 tests pass** (98 at the end of M0).
- Coverage by package: geom 25, geom/mesh 23, model 26, ui 25, render 23, io 18, scene 12, apptest 14.
- **Goldens** (read, not just generated): `docs/shots/m1_shell.png`, `m1_tree_collapsed.png`, `m1_planes_hidden.png`, `m1_planes_restored.png`, `m1_tree_click.png`, `m1_plane_delete_toast.png`.
- **Flows are asserted on behaviour, not pixels.** A new `dump` op prints the document, selection, hint and toasts as machine-readable lines, and a new `click` op synthesizes a real press-and-release through the widget code. So `TestTreeClicksToggleAndSelect` genuinely clicks the Top plane's eye toggle at (24, 80) and the Hull row at (120, 224), then checks the plane is hidden, exactly one undo step exists, and the selection reads "Hull".
- **R1 is covered from both sides**: `TestPlaneVisibilityIsUndoable` hides two planes and undoes them; `TestDefaultPlanesCannotBeDeleted` tries to delete one and requires a toast that says what to do instead.
- **Frame cost** with the full chrome: mean **3.43 ms** at 1280x720 (p95 4.12, max 5.55) against a 16.6 ms budget — up from 2.32 ms for the 3D view alone, still comfortable.
- The windowed app launches and runs clean.

**Decisions/deviations:** five entries appended to `docs/DECISIONS.md` — V-07 the widget kit runs once per frame inside the drawing block, with pointer ownership decided geometrically; V-08 the ops added for M1 flows, including `click` and `dump`; V-09 the glyph atlas limited to what Go Regular empirically contains; V-10 `internal/model` built here rather than deferred, since every tree action must be undoable; V-11 headless runs ignore the user's saved settings so goldens stay deterministic.

**Bugs found and fixed** (all had visible symptoms, all now covered):
1. **No click ever fired.** The widget kit cleared the active widget in `Begin`, before the widgets could see the release — but the release frame *is* the frame a click happens on. Moved to `End`. The scripted `click` op is what caught it: the first attempt hovered beautifully and did nothing.
2. **The window position was never saved.** `WindowRect` gave both `X` and `Y` the JSON tag `"x"`, and both `Width` and `Height` the tag `"w"`; encoding/json silently drops conflicting fields, so the rect round-tripped as zeros.
3. **The two-pass UI design was unsound.** Running widgets once in `Update` for input and again in `Draw` for pixels issued raylib draw calls outside `BeginDrawing` and would have reset drag state mid-drag. Replaced with a single pass.
4. **An em dash rendered as `?`.** The glyph atlas had been trimmed too far in M0; the characters Go Regular actually carries are now determined empirically.
5. **Goldens depended on the developer's machine.** Headless runs were loading the real `%APPDATA%` settings, so a collapsed tree panel left over from a manual session would have changed every baseline.
6. **Stale pick probes.** The M0 picking test used window coordinates that moved when the tree panel claimed 240 px; re-derived them with a sweep of `pick` ops rather than nudging by hand, and the test now says so.

**Open issues:**
- The toolbar tools are all disabled by design; each says which milestone turns it on. The `Move` entry currently maps to Idle because its mode arrives with M6.
- Face, edge and vertex selection resolve to the whole body for now. The sub-element selection model and its overlay highlighting are M6.
- The colour picker commits through the drag machinery, so scrubbing hue is one undo step — but it has no golden yet, because the popover needs a two-stage scripted interaction (click the swatch, then drag inside the popover) that would be clearer to write once M2 adds drag ops.
- DPI scaling above 1.0 is still untested in practice; this machine reports 1.0.

**Next:** M2 — start with `internal/geom/sketch2d`: write the twelve-plus region cases of TESTING §2 as failing tests first, then the integer arrangement pipeline (quantize, split, weld, face walk, nesting) behind them.

**Try it (user):**
```bash
c:\goin\go.exe run ./cmd/modeler
```
1. **Click an eye** next to Top, Front or Right — the plane vanishes, the row dims, and **Ctrl+Z** brings it back.
2. **Click a body row** — it gets an accent silhouette in the viewport; **hover** a row instead and watch that body outline in accent too.
3. **Hover a body row** and use the **pencil** to rename it, or the **trash** to delete it — the toast that appears carries an **Undo** button.
4. **Click a body's colour swatch** to open the HSV picker and drag around the square.
5. **Select a plane and press Del** — it refuses, and says you can hide it instead.
6. Press **?** for the shortcut sheet, and click the **‹** handle on the panel edge to collapse the tree.

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
