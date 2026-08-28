# Architecture Decision Records

These decisions were made deliberately at planning time (2026-08-26). The executing agent should **not relitigate** them mid-build. If reality contradicts one, record the evidence under § Deviations log, implement the named reversal path, and continue.

> **Superseding user directive (2026-08-26):** an earlier draft of this plan was pure-Go-only. The user explicitly lifted that: *"Don't use pure Go — feel free to use libraries as needed to accelerate development."* The records below reflect the library-friendly stack. cgo is fully allowed (WinLibs mingw64 gcc is installed and on PATH).

---

## D-01 — raylib-go as the engine layer (window, input, GPU 3D/2D)

**Decision:** Use `github.com/gen2brain/raylib-go/raylib` for the OS window, input polling, GPU rendering (depth buffer, MSAA, render textures, shaders via rlgl), 2D drawing, and text. The viewport is our own set of render passes over raylib (SPEC-RENDER); the UI is our own widget kit drawn with raylib 2D.

**Why:**
- One dependency covers the entire platform/engine surface. raylib-go vendors the raylib C sources and compiles them through cgo with the gcc already on this machine — no SDKs, no DLL hunting, no CMake for the app itself.
- Real GPU: z-buffer, MSAA 4×, render-to-texture, custom GLSL — the "very polished" pillar (smooth 60 fps at any resolution, clean AA) comes essentially free compared to a software rasterizer.
- Battle-tested, huge example corpus (agent-friendly), simple C-style API.
- Render textures give us the ID pick pass (exact per-pixel picking) with a few lines.

**Alternatives rejected:**
- *Ebiten + in-house CPU rasterizer* (the old pure-Go plan): more code to write (a whole rasterizer), no MSAA, CPU-bound at 4K. Its one real advantage — byte-deterministic goldens — is recovered well enough via tolerance-based GPU goldens (TESTING §5).
- *go-gl + GLFW raw:* all the plumbing raylib already does, with none of the batteries.
- *G3N / engines:* scene-graph frameworks fight a CAD tool's custom passes; smaller communities.

**Reversal path:** none anticipated; `internal/render` isolates raylib so a different backend remains a bounded rewrite.

## D-02 — In-house immediate-mode widget kit (not Dear ImGui, not raygui, not ebitenui)

**Decision:** Small IMGUI in `internal/ui` drawn with raylib primitives, implementing exactly the frozen widget list of SPEC-UX §4, themed per SPEC-UX §3.

**Why:** "Very polished" is a pillar and the UI is bespoke (floating cards near gizmos, hint bar, tree with eye/swatch rows, chip groups). Dear ImGui (cimgui-go/giu) would accelerate raw widget count but fights custom look/feel and adds a heavyweight dependency for ~15 widget kinds; raygui is too crude for the polish bar. The costly widgets (text input with caret, drag-number) are bounded and specced.

**Risk control:** widget list frozen at M1 — creep requires a DECISIONS entry. If widget work overruns badly (>2 sessions past M1 estimate), reconsider cimgui-go here with a Deviations entry.

## D-03 — Numeric model: exact integer sketch space, float64 model space with snap-on-author

**Decision:**
- **Sketch space (2D):** `int64` subunits, 256 per world unit. The arrangement/region engine (the "is it closed?" question, R3) is integer-exact — no epsilon tuning, deterministic forever.
- **Model space (3D):** `float64`. Every *authored* coordinate is grid-snapped at input time (extrude depths, moves, 90° rotations are exact permutations), so user geometry lives on the 1/256-u grid; only boolean intersection vertices may land off-grid, which is geometrically correct. Tolerances centralized in GEOM §1.3.

**Why:** Integer exactness is cheap and valuable in 2D where closedness is decided. In 3D, Manifold (D-04) owns robustness internally, so exact int predicates + int128 machinery are no longer needed — float64 + snap-on-author keeps the pixel-native contract with far less code.

## D-04 — Booleans via Manifold (vendored static lib + ~15-function cgo binding)

**Decision:** Use **Manifold** (github.com/elalish/manifold, Apache-2.0) through its C API (`manifoldc`) for union/difference/intersect. Built once with CMake + the installed mingw64, vendored as static libs in `third_party/manifold/` (see D-12). Face provenance via `manifold_reserve_ids` + per-run `runOriginalID` maps result triangles back to source faces (paint/selection continuity). Our code keeps: prism construction, polygon-face reassembly from result triangles, and a validation gate.

**Why:** Robust mesh booleans are *the* graveyard of projects like this one — and blocky modeling makes coplanar contact (the hardest case) the common case. Manifold is the modern, actively-maintained, guaranteed-manifold-output kernel (adopted by OpenSCAD and others). Binding ~15 C functions beats months of BSP debugging; its provenance IDs map exactly onto our paint-persistence design.

**Fallback (keep honest):** if Manifold can't be built/bound after two honest timeboxed attempts, implement the in-house BSP CSG fully specced in **SPEC-GEOMETRY Appendix A** against the same test matrix. The matrix, not the library's reputation, is the acceptance bar either way.

## D-05 — Pseudo-parametric document (direct meshes + feature log), not full history regen

**Decision:** Bodies are polygon meshes edited in place. Every operation is recorded in an append-only feature log (debug/undo/forward-compat), but editing an old sketch does not regenerate downstream bodies in v1.

**Why:** Full parametric regen (OnShape's actual core) is a multi-month kernel problem (topological naming, reference healing). The user's workflow — sketch, extrude, tweak verts, paint — is served by direct modeling plus persistent, re-usable sketches. The UI is explicit about this (SPEC-UX §8.8) so it never *feels* broken. M10 revisits parametric-lite.

## D-06 — Sketch regions via integer planar arrangement; no 2D boolean library

**Decision:** Closed regions come from a planar-graph face walk over quantized, split, welded segments (SPEC-GEOMETRY §4). Multi-region extrudes union in 3D via Manifold (which we have anyway). Draft uses an in-house miter offset with validity clamp (GEOM §5.3) — small, fully specced, no dependency roulette on Clipper ports.

## D-07 — Polygon faces with holes as the mesh face primitive (triangulate only internally)

**Decision:** `Face = planar loops (outer + holes) + stable FaceUID + optional paint + srcFace lineage`. Triangulation happens for rendering and for MeshGL handoff; result triangles are merged back into polygon faces (grouped by originalID + plane) after booleans.

**Why:** Faces are the user-facing unit: you sketch on one, push/pull one, paint one. Stable identity is what keeps paint glued and picking meaningful. Triangle soup would leak into every tool's UX.

## D-08 — Orthographic default projection

**Decision:** Ortho by default; `O` toggles perspective; view-cube face clicks read like blueprints. (User may override — PLAN Q1.)

## D-09 — `.ship` project container = zip(JSON + PNGs)

**Decision:** `manifest.json` (format version), `document.json` (sketch ints in subunits, model floats, human-diffable), `paint/<faceUID>.png`, `thumbnail.png`. Tolerant reader (unknown fields ignored, never crash on old/new versions).

## D-10 — File dialogs via ncruces/zenity

**Decision:** Open/Save/Export dialogs through `github.com/ncruces/zenity`. Color picking uses our own in-app picker. **Fallback:** minimal in-app file browser panel if zenity misbehaves; note in Deviations.

## D-11 — Fonts embedded, icons procedural

**Decision:** UI text = Go Regular TTF bytes (`golang.org/x/image/font/gofont/goregular`) loaded via raylib `LoadFontFromMemory` at the needed pixel sizes per DPI scale; icons are stroke-vector drawings in code (`ui/icons.go`). No binary art assets the executor can't author or regenerate.

## D-12 — Vendoring policy for Manifold

**Decision:** Build once in M4: pin a v3.x release tag (record the exact tag here when done), CMake flags `-G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release -DMANIFOLD_PAR=OFF -DMANIFOLD_TEST=OFF -DBUILD_SHARED_LIBS=OFF` (add `-DMANIFOLD_CROSS_SECTION=OFF` if the tag supports skipping it — we don't use cross sections). Copy `libmanifold*.a` + `include/manifold/*.h` + `LICENSE` into `third_party/manifold/`; commit them. cgo flags live only in `internal/geom/csg` (`CFLAGS: -I…`, `LDFLAGS: -L… -lmanifoldc -lmanifold -lstdc++ -static-libgcc -static-libstdc++`). The clone/build tree itself is NOT committed — only the vendored outputs, so later sessions never need CMake again.

**Done, 2026-08-26.** Tag **v3.5.2**. CMake 4.3.1, gcc 15.2.0 (MinGW-W64 x86_64-ucrt-posix-seh), Ninja generator:

```
cmake -S <clone> -B <build> -G Ninja -DCMAKE_BUILD_TYPE=Release   -DCMAKE_C_COMPILER=gcc -DCMAKE_CXX_COMPILER=g++   -DMANIFOLD_PAR=OFF -DMANIFOLD_TEST=OFF -DMANIFOLD_PYBIND=OFF   -DMANIFOLD_CBIND=ON -DMANIFOLD_CROSS_SECTION=ON   -DMANIFOLD_USE_BUILTIN_CLIPPER2=ON -DBUILD_SHARED_LIBS=OFF
```

Two departures from the plan above, both forced by the tag:

- **Cross sections stay ON.** `MANIFOLD_CBIND` is declared with `cmake_dependent_option` on `MANIFOLD_CROSS_SECTION`, so turning cross sections off takes the C bindings with it. The dependency it pulls in, Clipper2, is built from the in-tree copy (`MANIFOLD_USE_BUILTIN_CLIPPER2=ON`) and vendored alongside as `libClipper2.a`, with its licence.
- **Ninja rather than MinGW Makefiles.** The generator has no bearing on the artifact and Ninja is several times faster; the compiler and flags are as specified.

Vendored (2.8 MB total): `lib/libmanifoldc.a`, `lib/libmanifold.a`, `lib/libClipper2.a`, `include/manifold/{manifoldc.h,types.h}`, `LICENSE`, `LICENSE.clipper2`. The clone and build trees were left in scratch and are not committed.

---

## Deviations log

*(Executor appends here: date, spec section, what changed, why, evidence.)*

### 2026-08-26 — M0

**V-01 · Camera transitions interpolate azimuth/elevation, not quaternions.**
SPEC-RENDER §7 specifies "quaternion slerp of orientation". The camera model is
an up-locked turntable with no roll by construction (§7 also says "no roll
ever"), and slerping between two up-locked orientations passes through
orientations that carry roll. `CameraAnim` therefore interpolates azimuth along
the shorter arc and elevation linearly, which is the roll-free equivalent and
keeps the view cube's mirrored orientation exact throughout the animation.
Evidence: `TestCameraAnimTakesTheShorterArc`, `TestCameraBasisIsRollFree`.

**V-02 · View cube zones are hit-tested analytically, not with an ID render.**
SPEC-RENDER §6.3 specifies a dedicated pick render for the cube's 26 zones. The
cube is drawn as projected 2D sub-quads (one 3x3 grid per visible face), so
point-in-polygon over exactly those quads is both cheaper — no GPU readback on
every hover frame — and guaranteed consistent with what is on screen. The zone
set, the snap orientations and the 1:1 drag orbit are unchanged.
Evidence: `TestCubeHitTestMatchesWhatIsDrawn` walks every drawn cell.

**V-03 · `FacePaint` is declared in `geom/mesh`, not in `internal/paint`.**
SPEC-GEOMETRY §2 puts a `Paint *FacePaint` field on `mesh.Face` while §8.1
introduces the struct under the paint heading. The dependency direction of
PLAN §4 runs `geom` <- `paint`, so the type has to live below `paint` for
`mesh.Face` to name it. `internal/paint` keeps ownership of everything that
operates on it: mapping, brushes, palette, growth.

**V-04 · A float ear-clipping triangulator lives in `geom/mesh`.**
SPEC-GEOMETRY §5.2 places the triangulator in `geom/sketch2d` and §6.3 has the
boolean handoff reuse it. Mesh faces arriving from booleans may legitimately sit
off the subunit lattice, so they cannot be fed to an integer-exact triangulator
without quantising them first. `mesh` therefore carries a float ear-clipper with
hole bridging for rendering and MeshGL handoff; the integer sketch triangulator
still lands in M2 for sketch regions. Revisit at M4 whether the boolean handoff
should share one of them.
Evidence: `TestFaceWithHoleTriangulates`, `TestConcaveFaceTriangulationPreservesArea`.

**V-05 · `internal/render` imports `internal/ui` for theme tokens.**
PLAN §4 lists `ui` as standalone, meaning it depends on nothing above it; it
does not forbid others from reading its tokens. SPEC-UX §3 requires the theme to
be centralised in `ui/theme.go`, and duplicating the colours in `render` would
break that. The import is acyclic: `ui` pulls in only raylib and the stdlib.

**V-06 · Two additions to the headless toolset.**
The `pick` op (`{"op":"pick","at":[x,y]}`) runs the ID pass at a window pixel and
prints a machine-readable line; it is how the flow tests prove picking resolves
the right element, since the executor cannot watch the cursor. The `-bench N`
flag renders N frames after a script and reports the frame-cost distribution,
which is how the SPEC-RENDER §8 budget is checked. Both are executor tools and
neither changes the documented op semantics.

**Implementation note (not a deviation) · cgo pointer pinning.**
`rl.Mesh` must be its own heap allocation rather than a field of `BodyGPU`. Go's
cgo pointer check scans the *entire* heap object a C pointer lands in, so any
unpinned Go pointer sharing that object — our vertex slices, edge list, face
table — makes `UploadMesh` panic with "Go pointer to unpinned Go pointer".
raylib-go pins the mesh's own array fields, so an `rl.Mesh` allocated alone
passes. See `render.BodyGPU.Upload`.

### 2026-08-26 — M1

**V-07 · The widget kit runs exactly once per frame, inside the drawing block.**
An immediate-mode kit holds real state — which widget is being dragged, where
the caret is, how long a tooltip has waited — so running the widget code twice
per frame (once to read input, once to paint) corrupts it, and running it
outside `BeginDrawing` queues geometry into the wrong framebuffer. The app
therefore has one `Frame` entry point: logic, then the 3D view, then the chrome.
Whether the pointer belongs to the chrome or the viewport is decided
*geometrically* (`Layout.Viewport`, the view cube's rect, an open popover or
modal) before the widgets run, so the viewport never has to ask the kit what it
did.

**V-08 · Ops added for M1 flows.**
`plane.visible`, `select`, `deselect`, `delete`, `undo`, `redo`, `ui.tree`,
`hover`, `click` and `dump`. The last three are what make the chrome testable:
`click` synthesizes a full press-and-release through the real widget code, and
`dump` prints the document, selection, hint and toasts as machine-readable
lines, so a flow test asserts on behaviour rather than on a picture that
happened to change. SPEC-DATA §7 anticipates the op set growing as tools land.

**V-09 · The glyph atlas carries only what Go Regular actually has.**
Determined empirically rather than assumed: `·  ×  °  —  –  ‹  ›  “  ”  ‘  ’  …
±  →  ↔` are present; `✓ ✕ ▾ ▸ ⇄ ↶ ↷ ` are **not**. Those are stroke icons
(D-11), so no UI string can fall back to a missing-glyph box. An em dash in a
toast rendered as `?` until this was fixed.

**V-10 · `internal/model` was built in M1, not deferred.**
The milestone list does not name it until later, but the tree panel's own
actions — show/hide, rename, recolour, delete — must all be undoable
(SPEC-DATA §3.4), and retrofitting a command bus under a UI that already mutates
state directly is worse than building it first. The bus, the targeted-snapshot
undo, drag coalescing and the change events all landed here; the geometry
commands join them from M3.

**V-11 · Headless runs ignore the user's saved settings.**
`app.New` loads `%APPDATA%\Modeler\settings.json` interactively but uses
defaults when headless. Otherwise a golden shot would depend on whatever tree
width or collapse state the developer last left behind, which is not a property
of the document.

### 2026-08-26 — M2

**V-12 · Sketch overlays draw on top of the model, not depth-tested against it.**
SPEC-RENDER §1 lists the in-sketch overlays inside the scene pass, which would
leave them fighting the depth buffer. In practice a default plane passes
*through* the model, so a profile drawn on it disappears behind whatever
geometry is in front — the very first sketch on the Front plane was completely
hidden by the test scene's hull. The rest of the scene is already dimmed to 30%
(SPEC-UX §8.1); the sketch pass now also disables depth testing, so the thing
being edited is always visible. This is what every CAD tool does and what makes
sketching on an interior plane possible at all.

**V-13 · Collinear overlaps are resolved by construction, not by interval
arithmetic.** SPEC-GEOMETRY §4.2 describes projecting collinear segments onto
their shared line, merging into an interval union and re-emitting maximal
pieces. The implementation reaches the identical result more simply: both
segments are cut at the shared interval's endpoints like any other meeting
point, and the overlapping pieces then arrive at the graph as the same node
pair, where edge deduplication merges them. Covered by the "collinear partial
overlap" and "duplicate segments" cases.

**V-14 · A line chain commits one Line entity per placed segment.**
SPEC-GEOMETRY §3 lists Line, Rect, Circle and Ref as the entity kinds, without
saying whether a click-chain is one entity or many. Each segment is its own
entity, so every click is an independent undo step and deleting one segment of
a chain leaves the rest — which is what the Select tool's per-entity delete
needs. A rectangle stays a single Rect entity, as specified, because dragging
one later has to move all four sides together.

**V-15 · A sketch row's double-click re-enters editing; renaming is the pencil.**
SPEC-UX §7 splits the gesture by where it lands — double-click the *name* to
rename, double-click the *row* to re-enter editing. Distinguishing the two by
hit position inside a row is fiddly and easy to get wrong by a few pixels, so
the row's double-click always re-enters editing and the pencil hover action is
the only way to rename. Both actions stay reachable and neither can be
triggered by accident.

**V-16 · Ops added for M2 flows.** `sketch.tool` selects the active drawing
tool, and the `dump` op grew a `sketch active=...` line reporting the entity,
region and open-end counts. The specified `sketch.begin`, `sketch.line`,
`sketch.rect`, `sketch.circle` and `sketch.finish` all landed as written.


### 2026-08-26 — M3

**V-17 · Symmetric with draft is two frusta meeting at the sketch plane.**
SPEC-UX §9.3 says the draft "applies outward from the plane both ways (widest at
the sketch plane)" and SPEC-GEOMETRY §5 describes an extrusion as two rings. The
two only reconcile with three rings: the profile at the sketch plane, and a
tapered ring at each end. A symmetric drafted solid therefore has a mid-belt of
vertices and twice the side faces of a one-sided one, which is exactly what
"widest at the sketch plane" has to mean for a shape that runs both ways.
Evidence: `TestExtrudeSymmetricIsWidestAtTheSketchPlane`,
`TestSymmetricStraddlesThePlane` (volume against 2x the frustum formula).

**V-18 · Through-All measures the scene's reach in both directions.**
SPEC-UX §9.3 defines the toggle as "past everything (scene bbox + margin)"
without saying past everything in which direction. The depth is the furthest any
existing geometry reaches from the sketch origin along the axis, either way,
plus a 1 u margin, snapped to the grid — so flipping a through-all extrude
cannot silently stop short. Symmetric doubles it, because it splits the run
either side of the plane. The margin exists so the M4 subtract never has to
resolve a cut that lands exactly flush with a face. Evidence:
`TestThroughAllTakesOverTheDepth`, `TestThroughAllClearsTheWholeScene`.

**V-19 · The extrude camera tilt is decided by the axis, not by the view.**
SPEC-UX §9.1 asks the camera to pull back "if it was normal-on". Testing that
against named views only covers the planes someone thought to list; the tool
tests `|dot(forward, axis)| > 0.98` instead, which is the actual condition — an
arrow pointing at the camera is invisible whichever plane produced it — and
swings to a three-quarter view built in that axis's own frame.

**V-20 · A golden that drifts inside tolerance fails as stale.**
SPEC-RENDER §10 calls goldens "same-machine baselines" with a tolerance for
driver updates, and TESTING §5 fixes that tolerance at |Δ|≤3 for ≥99.7% of
pixels. Passing it is not the same as being unchanged: a control added to a
toolbar covers about 0.04% of the frame, so every golden in the repo can go out
of date while every golden test still passes — which is what had happened by the
time this check was added. The gate itself is unchanged; a shot that passes it
but has more than 200 changed pixels now fails separately, with the diff image
and the instruction to regenerate. This strengthens §10's stated intent rather
than departing from it.

**V-21 · A script must observe something, and a dump counts.**
The headless runner rejected any script that produced no shots. State probes
that only `dump` are a legitimate and common kind of flow test, so the rule is
now that a script must either capture or dump. Silence is still an error.
Evidence: `TestAScriptThatObservesNothingIsRejected`.

**V-22 · Ops added for M3 flows.** `extrude` runs and commits in one step;
`extrude.begin`, `extrude.commit` and `extrude.cancel` split it so a shot can
catch the tool mid-interaction. `extrude` and `extrude.begin` take `depth`,
`draft`, `dir`, `through`, `regions` and `result`. With no sketch open and none
named, they start from the sketch selected in the tree, which is the second
entry point of SPEC-UX §9.1. The `dump` op grew an `extrude ...` line and `vol=`
on every body line.

### 2026-08-26 — M4

**V-23 · The float64 MeshGL, not the float32 one.** SPEC-GEOMETRY §6.3 specifies
float32 vertices and argues carefully that the grid-snapped coordinates this
document uses are exactly representable up to ±32,768 u. The argument is sound
and no longer needed: the C API also carries `manifold_meshgl64_*`, which takes
doubles, so the model's own float64 coordinates cross unchanged in both
directions and the reasoning about representable ranges is moot.

**V-24 · Manifold's merge table decides which vertices are one, not position.**
SPEC-GEOMETRY §6.3 says to weld (`WeldDist`) after converting a result back.
Welding by distance is wrong here: two solids touching along an edge have
coincident vertices that Manifold has deliberately kept apart, and fusing them
turns two legal shells into an edge belonging to four faces. The converter uses
`mergeFromVert`/`mergeToVert` instead, which is Manifold's own verdict on the
question, and does not weld again afterwards. The acceptance matrix's edge-touch
and vertex-touch cases are what this is for.

**V-25 · Output faces merge across source faces when they are coplanar.**
SPEC-GEOMETRY §6.3 groups result triangles by `(originalID, plane-key)`, which
keeps every source face separate. That leaves a flush butt-join as two half-walls
with a seam down the middle rather than the one wall it visibly is, and a wall
you can only drag half of is not the shape anybody drew. Grouping is by geometry
— connected and coplanar — with one exception: two faces that both carry paint
stay apart, because merging them would have to discard one picture. A merged
face inherits the painted side's lineage.

**V-26 · Straightening a split edge is a decision about the whole mesh.**
A boolean leaves vertices in the middle of edges it split, and dropping them is
what turns two stacked boxes back into one clean box. But a vertex can be
mid-edge on one face and a genuine corner of the face next door — a T-junction,
which booleans produce constantly — so a vertex goes only if every loop
containing it agrees it is not a corner. And never when the loop doubles back on
itself: the two sides of a zero-width spur are collinear too, and collapsing its
tip invents an edge that runs straight past the vertices the neighbouring faces
are still holding. The dot product tells a straight run from a reversal.

**V-27 · Pinched vertices are split so each fan gets its own copy.**
Manifold guarantees every *edge* has two faces, which permits a solid touching
itself at a single point. Our mesh model assumes the neighbourhood of a vertex is
a disc, and the validator sees the difference as an odd Euler characteristic. The
converter now walks the fans of faces around each vertex and gives every fan
after the first its own copy, in the same place. Nothing moves; the surface stops
claiming to be joined where it is not.

**V-28 · Zero-volume shells are dropped.** Coincident input surfaces can leave a
pair of back-to-back faces: a closed, legal, empty shell. It costs nothing in
volume and is two faces in the same place, which the validator reads as duplicate
faces and a user would read as two things to click on. Connected pieces enclosing
less than `WeldDist³` are removed. The test is enclosed volume rather than
orientation, because a real cavity inside a solid also has negative volume and
has to stay.

**V-29 · A golden's text is checked for renderable glyphs.** The font atlas is a
curated codepoint list (D-11) and anything outside it draws as an empty box. A
boolean summary reached for a true minus sign (U+2212) and shipped a question
mark to the user, invisible to every existing test. `TestEveryMessageIsRenderable`
now runs every op script in the repository and checks every toast and hint line
against the atlas.

**V-30 · Ops added for M4 flows.** `boolean` runs and commits in one step;
`boolean.begin`, `boolean.commit` and `boolean.cancel` split it so a shot can
catch the tool mid-pick. They take `kind` (union/subtract/intersect), `target`,
`tools` and `visible` (the keep-tools toggle). The extrude ops grew `result`
(new/add/subtract/intersect), and the `dump` op grew a `boolean ...` line plus
`result=` and `targets=` on the extrude line.

### 2026-08-26 — M5

**V-31 · A sketch stores its plane, not a reference to one.** SPEC-GEOMETRY §3
allows a sketch's plane to be "builtin plane enum OR face ref + Frame
snapshot". Face sketches store all three: the body, the face identity, and the
frame. The frame is what the sketch actually uses, so it keeps working when the
face is gone; the identity is only for snap references and Project outline. This
is not a hypothetical — extruding a face sketch replaces the very face it was
drawn on, so by the second edit most face sketches are already orphaned, and
they all still open, draw and extrude correctly.

**V-32 · Clicking a face selects the face.** M1 and M2 selected the whole body
on a viewport click, with a note that sub-element selection was M6's. SPEC-UX
§10 needs a face to be selectable to sketch on it or push it, so faces are
selectable now; edges and vertices stay with the unified selection model in M6.

**V-33 · Two shells touching face to face pass validation, and should not.**
The test scene's hull was two boxes joined with `mesh.Merge`, which concatenates
rather than unions. Every check in SPEC-GEOMETRY §6.5 passes — each shell is
closed, manifold, positively oriented, and the Euler count is right — and the
volume agrees with Manifold's, because the shells enclose no shared volume. It
is still not a solid: the shared plane carries two coincident surfaces. Manifold
answers a union onto such a mesh by returning the target unchanged, silently,
which is how this survived five milestones.

The scene is fixed — the hull is unioned properly now, and `mesh.Merge` says in
its doc comment what it is and is not for. A validator check for it was written
and then removed: distinguishing "two solids pressed together" from a legal
edge-to-edge or corner-to-corner touch needs to know whether two coplanar
opposed faces overlap over an *area*, and the version that was precise enough to
pass the M4 acceptance matrix still rejected two of its cases. A check that
rejects correct geometry is worse than no check. The gap is recorded here
instead, and `TestEveryBodyIsAlwaysAValidSolid` says out loud what it does not
cover.

**V-34 · Push/pull is its own command, not a synthesised extrude.** PLAN
describes it as "extrude-of-face-outline through the command bus". It is exactly
that geometrically — the face's loops become a region, the region becomes a
prism, the prism unions or subtracts — but it is `model.PushPull` rather than a
`model.Extrude` with a fabricated sketch. Push/pull has no sketch, and inventing
one would put an entry in the tree that the user never made and cannot use.

**V-35 · Ops added for M5 flows.** `sketch.face` starts a sketch on a face,
`sketch.project` copies its outline in, and `pushpull` moves a face
(`kind: "preview"` stops before the commit so a shot can catch the drag). All
three, and `select` with `kind: "face"`, name a face either by index or by
`axis` — `"+y"` picks the outermost face pointing that way, which keeps meaning
the same thing after an edit has renumbered everything. The `dump` op grew
`pushpull` and `facesketch` lines and a `valid=` field on every body.

### 2026-08-26 — M6

**V-36 · Box select projects vertices and edges; only faces are rendered.**
SPEC-RENDER §6.2 specifies one full-viewport ID render for box select, scanned
by rectangle. That works for faces and cannot work for vertices. Two vertices at
the same screen position — the near and far corners of a hull seen straight on,
which is precisely the view somebody stretches a nose in — occupy one pixel, and
one pixel holds one id. Whichever drew last wins, the other is invisible to the
scan, and the workflow §12.1 names as box select's reason for existing tears the
hull in half. Vertices and edges are therefore collected by projecting them,
which is exact, needs no readback and is faster than the render it replaces.
Faces keep the ID render, because "is this region inside the rectangle" is a
question about area with no comparably cheap answer. Evidence:
`TestStretchAHullAndRotateAWing` selects all four nose vertices from a
front-on view.

**V-37 · A quarter turn is a coordinate permutation, not a matrix.**
SPEC-GEOMETRY §7.3 requires 90° rotations to be exact and grid-preserving.
`sin(pi/2)` is 1 but `cos(pi/2)` is 6.1e-17, which is enough to take every vertex
off the lattice and keep it off. Multiples of 90° about a world axis are applied
as a permutation of coordinates and a sign flip instead. Evidence:
`TestFourQuarterTurnsReturnExactly` — four turns return every vertex to its
original bits, not to a tolerance.

**V-38 · "Leaves the grid" is about the change, not the result.**
The free-rotation warning fires when something that was on the lattice no longer
is. A body already off the grid from an earlier free rotation stays off it
through a subsequent quarter turn, and warning again there would be telling the
user about something they did not just do.

**V-39 · A second click on a selected face takes the body.**
SPEC-UX §12.1 asks for double-click. Clicking an already-selected face is the
same gesture without a timer to tune, and it cannot misfire on a slow
double-click or a fast pair of deliberate single clicks.

**V-40 · Bent faces warn by toast, not by a hover chip.**
SPEC-UX §12.3 describes a tiny warn chip shown when hovering a bent face. The
warning is given once when an edit bends something, which is when it is news.
The hover chip needs per-face hover copy in the overlay pass and belongs with
the polish pass in M9; what matters now — that sketching and push/pull refuse a
bent face with guidance — is already true.

**V-41 · A single flat face shows the push/pull arrow alone.**
SPEC-UX §12.2 says a single-face selection "leads with its normal arrow". It
shows that arrow and not the move gizmo, because two gizmos on one face would be
two overlapping sets of handles arguing about the same drag. Moving a face
rather than push/pulling it is a box-select away.

**V-42 · Bent faces are triangulated on demand, not stored.**
SPEC-GEOMETRY §7.2 says a flagged face's rendering and MeshGL handoff "use their
stored triangulation". Ours re-runs the ear clipper in the face's own frame each
time, which produces the same triangles from the same vertices and avoids a
cache that would have to be invalidated on every vertex move. If profiling ever
says otherwise this is the place to add one.

**V-43 · Ops added for M6 flows.** `move` (delta), `rotate` (axis, degrees),
`duplicate`, and `box.select` (rect, kind) drive the real gizmo and the real
coalesced commands. `select` gained `kind: "vert"` and `kind: "edge"`. The
`dump` op grew a `gizmo` line carrying the mode, the pivot and the box filter.

### 2026-08-26 — M7

**V-44 · One texture atlas per body, not one texture per painted face.**
PLAN's M7 checklist says "GPU texture per painted face". A body is one `rl.Mesh`
drawn in one `DrawMesh` call, so per-face textures would mean splitting every
painted body into as many meshes as it has painted faces and rebuilding that
split after every boolean. The atlas is keyed by the `*mesh.FacePaint` pointer
rather than by the face, which makes the shared-paint contract of
SPEC-GEOMETRY §8.4 fall out for free: fragments of a cut face reference one
picture, so they map into one region of the atlas and stay in step with no
special case. Pixels changing re-uploads the stroke's dirty rectangle; only a
*layout* change — a first stroke, a resample, a picture that grew past its slot
— rebuilds the body's render form, and geometry edits already do exactly that.
Evidence: `TestPaintSurvivesACutInTheApp`, `TestGoldenPaintedShip`.

**V-45 · Fragments of a cut face keep sharing one picture; no copy-on-write.**
SPEC-GEOMETRY §8.4 is explicit that v1 keeps the picture shared, and the kernel
test (`TestPaintSurvivesACut`) asserts pointer identity across the boolean. So a
stroke on one fragment shows on its siblings, which is right rather than
surprising: the mapping is anchored in the world, so a texel belongs to exactly
one place however many faces read the picture it lives in. `paint.Copy` exists
for the day that changes. Evidence:
`TestAStrokeOnOneFragmentShowsOnItsSiblings`.

**V-46 · Paint mode picks faces only.**
The ID pass draws edge ribbons five pixels wide and vertex quads nine, biased
toward the eye so they beat the surfaces they belong to (SPEC-RENDER §6.1). A
brush cannot paint an edge, so in paint mode they are kept out of the pass
entirely (`Scene.PickFacesOnly`). This was not theoretical: after a boolean cut
the fragment boundary sat under the cursor and swallowed a stroke aimed at the
face behind it, and the eyedropper read nothing where there was plainly paint.

**V-47 · The texel cursor shows the grid a face has not got yet.**
Hovering an unpainted face allocates nothing, but the cursor still needs a
mapping to outline. The resolution chip's provisional `paint.Allocate` is cached
per (body, face, chip) and rebuilt only when one of the three changes — at 512 px
that allocation is a megabyte, so doing it per frame was never an option. It also
makes the chips mean something before you commit: you can see 16 px against
128 px on the face itself. Evidence:
`TestTheCursorShowsTheGridBeforeYouCommitToIt`.

**V-48 · `.hex` import lands by file drop until M8 brings the dialogs.**
SPEC-UX §13.3 wants Import .hex behind a file dialog, and dialogs
(`ncruces/zenity`, D-10) belong to M8's file work. Rather than ship a parser
nothing can reach, dropping a `.hex` file on the window imports it, and the
button is disabled with a tooltip that says so — SPEC-UX §15's rule that a
disabled control explains how to enable it. M8 wires the same `ImportPalette`
entry point to the dialog.

**V-49 · A stroke is not announced by a toast.**
SPEC-UX §15 asks for a toast per completed op. A stroke per mouse-up is not that
kind of op — a minute of painting would be a minute of toasts — so strokes are
silent and the things that are genuinely news keep their toasts: a picked
colour, a resample, an eraser on a face with no paint. The undo entry is still
there and still named.

**V-50 · Paint-mode keys are D/E/G/I, and they are mode-local.**
SPEC-UX §16's global map gives E to Extrude. Inside paint mode E is the eraser,
the way sketch mode already takes V/L/R/C for its own tools. The letters are the
ones `paint.Tool.Shortcut` had already pinned. P leaves the mode, which is what
pressing the tool's own key a second time should do.

**V-51 · Ops added for M7 flows.** `paint.begin` / `paint.exit`, `paint.res`,
`paint.color`, `paint.tool`, `paint.size`, `paint.pixel`, `paint.stroke` (a
texel path, so a script exercises the same interpolation a drag does),
`paint.resample`, `paint.textures` and `paint.faceview`. The `dump` op grew a
`paint` line (brush state), a `painthover` line (face, texel, obliqueness) and a
`facepaint` line per distinct picture — including a hash of its pixels, because
a stroke over already-painted texels leaves the painted *count* exactly where it
was, and that is most of what an undo has to put back.

**V-52 · Every earlier golden was regenerated, for one button.**
Shipping the Paint tool turned its toolbar button from disabled-grey to live,
which changed 243 pixels of every shot in the suite (x 291–336, y 12–27 —
measured, then regenerated per TESTING §5). The `docs/shots` diary was
deliberately *not* rewritten: it records what each milestone looked like when it
landed, and a greyed-out Paint button was accurate then.

### 2026-08-26 — M7 follow-up: shapes, gradients and the face lock

Requested by the user after trying the build. All of it is specced in SPEC-UX
§13.4 and §13.5, written at the same time as the code.

**V-53 · Four two-point tools, decided by their ends and nothing between.**
Line, rectangle, circle and gradient read only the first and last of the
stroke's points; a freehand tool reads all of them. That is the whole
difference, and it is what makes them rubber-band: the drag replaces its pending
command with a longer version each frame, so a shape that read the whole path
would stamp every size it passed through onto the face. Evidence:
`TestAShapeRubberBandsRatherThanAccumulating`.

**V-54 · Softness is spent on coverage, and coverage is spent one of two ways.**
A soft brush and a gradient both produce a value between nothing and everything.
With no dithering that value blends — a real colour between the two. With a
Bayer mode it decides *how many whole texels* are painted instead, so the result
stays inside the palette it was drawn from. One concept, one control, two tools:
the Dither chips are shown for exactly the two tools that have a coverage to
spend. This is the pixel-art answer to a soft edge and the reason the user asked
for the matrices in the same breath as the brush.

**V-55 · Brush sizes 8 and 16 were added.** SPEC-UX §13.1 lists 1/2/4, which is
right for a pencil and useless for a soft brush: at four texels across there is
nowhere for a falloff to happen. The soft brush also has a solid core (40% of
its radius) before the falloff starts, because a brush that is never fully its
own colour anywhere reads as weak rather than soft.

**V-56 · A soft dab blends in the paint layer, against the body's colour.**
The obvious implementation is a partial alpha, and it is wrong. The texture
composites over the *body* colour, not over the paint already on the face, so a
half-alpha texel laid over existing paint would show the hull through it. The
command hands the body's own colour down to the brush, the blend happens against
whatever is under the texel, and what is stored is always opaque — which also
keeps alpha binary for the eraser, the eyedropper and the fill, all of which
already assumed it. Evidence: `TestSoftBrushFadesIntoWhatIsUnderIt`,
`TestTheSoftBrushBlendsIntoTheBodyColour`.

**V-57 · An ellipse is rasterised by scanline, and its outline is a boundary
test.** Solving the ellipse for x at each row gives runs that are symmetric by
construction and trivially fillable; a midpoint walk needs special cases at both
axis crossings and leaves gaps where the curve runs flat. The outline is then
"in the shape, with a neighbour outside it", which is closed and symmetric for
free. Evidence: `TestEllipseIsSymmetricInBothAxes`, `TestFilledEllipseHasNoHoles`.

**V-58 · The face lock is a paint lock, not a camera lock.** The user asked for
the camera to focus on a face and to be stopped from painting the others. Both
happen, but navigation stays free: orbit, pan and zoom work identically in every
mode (SPEC-UX §1), and checking your work from an angle is part of painting.
What is fixed is where the paint can land.

**V-59 · A locked cursor is resolved against the face's plane, not the ID pass.**
The pick pass answers "what is in front here", which is the wrong question once
a face has been chosen — a body drifting in front, or an edge along the border,
would take the stroke. Intersecting the locked face's own plane cannot be
stolen, confines the cursor to the face's texel rectangle, and costs no readback
at all, so a locked session is cheaper than a free one. Evidence:
`TestTheLockKeepsPaintOnOneFace`, where the same window pixel resolves a second
face once unlocked and nothing at all while locked.

**V-60 · `Camera.FrameTightly` frames along the screen axes.** `FrameBox` frames
the sphere around a box so that orbiting afterwards can never lose anything,
which on a flat wide face wastes most of the viewport. Where the orientation is
the point — the camera has just been pointed squarely at a face — the points are
measured along the camera's own right and up. The locked face is then offset
clear of the palette panel, because the one camera move whose entire job is "let
me see this face" should not put a quarter of it under a panel.

**V-61 · Ops and dump fields added.** `paint.color2`, `paint.swap`,
`paint.dither`, `paint.shapefill`, `paint.lock`, `paint.unlock`; `paint.tool`
gained line/rect/circle/gradient/brush (and accepts "square" and "ellipse"). The
`paint` dump line gained `color2`, `dither`, `fill`, `slot`, `locked` and
`lockface`. Extending that line broke the parser written against its first
version, which is why there is now one parser for it and not two.

**V-62 · Goldens regenerated again.** The palette panel grew a second tool row,
the dither chips, a second colour swatch and the lock row, so every M7 shot
changed inside the panel — 53,076 pixels, none of them left of x=1027, measured
before regenerating (TESTING §5).

### 2026-08-26 — Fix: panel controls that disarmed themselves on the way to being clicked

**V-63a · The lock is armed first and the face picked second.**
The user's second report on the same control: "I want to be able to click the
button, then select the face I want to lock to". The first version acted on the
face under the pointer when the button was pressed, which cannot work — moving
the pointer to the button is exactly what takes it off the face — so the button
greyed out as you reached for it. It now arms a pick and the next click on a
face chooses it, the same shape as pressing S with no plane selected
(SPEC-UX §8.1). The button is live whatever the pointer is doing, which is the
point: a control whose availability depends on where the pointer is cannot be
reached by moving the pointer. The click that chooses is spent on the choice and
paints nothing. Evidence: `TestLockIsArmedFirstAndPickedSecond`.

**V-63 · The panel acts on the last face the pointer resolved, not the live one.**
Reported by the user: Lock to this face and Face view were impossible to click.
Both act on "the face you are pointing at", and the pointer stops being on a
face the moment it leaves the viewport for the panel — so the button was live
while you looked at it and disabled by the time you arrived, and Face view,
which only shows at an oblique angle, vanished en route. The live hover still
governs the cursor and the stroke, because there is no texel under a button;
the panel reads a sticky hover that outlives the journey. The resolution
mismatch prompt had the same bug and the same fix: its own buttons were in the
panel it was disappearing from. The lock went further and dropped the
dependency altogether (V-63a); the sticky hover is what still carries Face view
and the mismatch prompt, which genuinely are about the face you were pointing
at.

**V-64 · A floating card owns the pointer over it.**
The other half of the same report. `chromeOwnsPointer` treated the toolbar, the
tree and the hint bar as chrome and everything inside the viewport rectangle as
model — but the cards float *inside* the viewport, so the viewport's own
hit-testing ran behind them. Every press on a chip resolved whatever face was
behind the panel and left a dab on it. `FloatingCard` now registers its
rectangle for the next frame's hit test, the way the colour popover already did.
Asking whether a widget is hovered would not have been enough: the gaps between
a card's controls are still the card.

**V-65 · Cards take the left button and leave navigation alone.**
Making a card chrome outright would also have stopped orbiting from starting on
top of one, and navigation works from wherever the pointer is in every mode
(SPEC-UX §1). `cardOnlyOwnsPointer` separates the two: over a card the camera
still moves and the tools do not. Over real chrome, neither does.

### 2026-08-26 — M8

**V-66 · The mesh serialises itself, because only it knows about shared paint.**
SPEC-DATA §4 describes `document.json` as the whole document, which suggests
package io mirroring the types. It cannot: fragments of a cut face share one
`FacePaint` by pointer (SPEC-GEOMETRY §8.4), and a face-at-a-time marshaller
would write that picture once per fragment and read back a copy each — painting
one fragment would stop showing on its siblings, a contract broken by having
been saved. The pictures go in a per-mesh table and the faces hold an index into
it. Evidence: `TestSharedPaintIsStillSharedAfterALoad`.

**V-67 · Paint PNGs are named after the face that introduced the picture.**
SPEC-DATA §4 says `paint/<faceUID>.png`, which reads as one file per painted
face and would duplicate a shared picture. The name is kept and the meaning
narrowed: the file is named for the *owning* face, the first one that referenced
it, which is stable because identities are never reused (SPEC-DATA §1).

**V-68 · Vectors serialise as arrays.** A saved ship is mostly vertices, and
`[0,1,2]` against `{"X":0,"Y":1,"Z":2}` is a third of the bytes and easier to
read in a diff. The reader still accepts the object form, so a file written
before this does not become unreadable.

**V-69 · An autosave is a whole .ship, not a journal.** Recovery is then the
ordinary load path, already covered by its own tests, rather than a second
reader that only ever runs on somebody's worst day. Recovery files are found and
cleared by process id rather than by name: a save is exactly the moment the
document's name changes, and clearing by the new name would leave the file
written under the old one to be offered back as work that was in fact saved.
Evidence: `TestASavedDocumentIsNotOfferedBack`.

**V-70 · Export options live in a card; the dialog decides only the path.**
SPEC-DATA §5 calls it a "Ctrl+E dialog: format, path via zenity". A native file
dialog has nowhere sensible to put a scale chip, and the format has to be chosen
*before* the dialog opens because it decides what the dialog filters for. So the
card picks the format and its options, and then the dialog picks the place.

**V-71 · glTF export, in both spellings, added at the user's request.**
It was M10 backlog item 4; the user asked for it during M8 and it slots into the
same export work. `.glb` packs everything into one file and `.gltf` writes JSON
with a `.bin` and the PNGs beside it. Every sampler is NEAREST — glTF is the
only format here that can carry that instruction in the file rather than in a
README, and a pixel-art texture filtered smooth is not this program's output.
Vertices are emitted per face rather than shared, because these are flat-shaded
solids and a shared vertex shares its normal.

**V-72 · The release build must be statically linked.** PLAN listed
`-extldflags=-static` as an "if needed". It is needed: without it the exe
imports `libgcc_s_seh-1.dll`, `libstdc++-6.dll` and `libwinpthread-1.dll` —
the last two from Manifold's C++ — and no machine without mingw has them.
Measured with `objdump -p`; with the flag the only imports left are Windows
system DLLs and the Universal CRT.

**V-73 · Dialogs run between frames, never inside one.** A native file dialog is
modal and pumps its own message loop, and running one between BeginDrawing and
EndDrawing means running somebody else's loop with a frame half submitted. Every
dialog-driven action is queued during the frame and performed by
`RunPendingFile` after it.

**V-74 · `MODELER_CONFIG_DIR` overrides where settings and autosaves live.**
Tests must not write into the profile of whoever is running them — an autosave
test that recovered the user's actual work would be worse than no test. A
headless run only looks for recovery files when that variable is set, so a
golden shot can never grow a recovery card because the machine it ran on
crashed last week.

**V-75 · Goldens regenerated for the toolbar.** The disabled settings gear became
three live file buttons (open, save, export), changing 245 pixels of every shot
in the suite, all inside the toolbar. Measured before regenerating (TESTING §5).

### 2026-08-26 — M9, first fix

**V-76 · Through all reaches both ways when the sketch plane is inside the model.**
Reported by the user as "the subtract function when extruding a sketch is
buggy, not working", and it was: a Through-all Subtract on one of the three
default planes cut exactly half a hole. Those planes all pass through the
origin, and so through the middle of most ships; "past everything" was measured
as the distance to the furthest corner of the scene and then spent in one
direction, which starts the cut *inside* the material and leaves a blind
pocket. Turning Through all on now selects Symmetric when the scene straddles
the plane along the extrude axis. Direction stays the user's to change — Normal
with Through all then means "past everything ahead of the plane", which is a
real thing to want — and the toggle says which it resolved to, because a user
who asked for a hole and got a pocket has no other way to tell why.

The bug survived M3 because the only Through-all test used `dir: "symmetric"`
explicitly, which is the one case that already worked. `m9_throughcut` uses the
default direction and measures the hole: 24 units of hull, not 12.

**V-77 · The async boolean is deliberately not built.**
SPEC-RENDER §8 says an operation over 120 ms should run on a goroutine behind a
spinner, with a cancel path. `BenchmarkBooleanOnAShip` measures the commonest
expensive case — cutting a window through a 1282-triangle hull, which is the
sample ship's scale — at **4.7 ms**. That is 25 times under the threshold, so
the machinery would be complexity with no cause, and PLAN §13 says not to
optimize past a budget without profiling evidence. The benchmark stays, so the
day somebody builds a ship an order of magnitude heavier the number says so.

**V-78 · No about card.** UX §15 asks for the version "in title bar & about
card". The gear an about card would have lived behind became the export button
in M8, and inventing a menu for one line of text is worse than not having it.
The version is in the window title and the hint bar's right corner — two places
the user already looks.

**V-79 · The sample ship is a script, and that is the point.**
`assets/sample_ship.json` is embedded and run through the same ScriptRunner the
headless tests use. So the thing a first-time user is shown is built by the path
a test drives: it cannot rot without `TestTheSampleShipBuildsEndToEnd` going
red, and it doubles as the full-app end-to-end and the README's hero image. A
saved `.ship` would only have proved the loader works.

**V-80 · Closing with unsaved work asks.** The autosave is a safety net, not an
answer: it lives in a folder the user has never seen, under a name they did not
choose. The window's close is intercepted and the prompt offers Save or Close
without saving; cancelling the save dialog cancels the close too, rather than
quietly discarding.

**V-81 · Escape on a modal is "dismissed", not "cancelled".** The close prompt's
cancel button reads "Close without saving" — words, which can be read. Escape is
a reflex, and DrawModal used to report it as that same cancel, so the reflex key
on "Save before closing?" threw the work away. ModalResult now carries a third
outcome: Confirm and the labelled button each mean what they say, and Escape
dismisses the question leaving everything as it was.

**V-82 · A modal owns the keyboard.** The mode key handlers ran under an open
modal: Escape on the close prompt also reached paint mode and exited it, Enter
also committed a pending extrude, and S started a sketch behind the dialog. The
update loop now skips every mode handler while a modal is up — the dialog's own
Esc/Enter handling is the whole keyboard. The shortcut sheet owns it the same
way, so the sheet explaining the S key is no longer a thing the S key acts
through.

**V-83 · New, Open, a recent, the sample and a dropped .ship all stop at
"Discard unsaved changes?" when there is unsaved work.** The window's close has
asked since M8 (V-80); Ctrl+N reached the same cliff with no fence at all — one
reflexive "new ship" and the old one was gone. The guard parks the action, asks
with a danger-styled Discard / Keep working pair, and only an explicit Discard
releases it. Headless runs are exempt: scripts drive NewDocument and OpenPath
directly, below the dialog layer, as they always have.

**V-84 · The interactive app starts empty; the test scene is headless-only.**
`Run()` still loaded the M1 debug scene — a hull, an engine pod and a wing pod —
on every launch, which buried the entire M8/M9 entry experience: with a
non-empty document the welcome card, and with it New, Open, the recents and the
Sample ship button, could never appear at all. The golden scripts are written
against those three bodies, so `LoadTestScene` stays for `RunHeadless`; the
interactive path starts on an empty document and the welcome card, which is
what SPEC-UX §14 said all along.

**V-85 · Move is wired, and it is not a mode.** The toolbar's Move button
shipped disabled behind "Move arrives with milestone M6" — three milestones
after M6 shipped — and the M key, listed in the shortcut sheet, was bound to
nothing. Both now point the armed gizmo at moving, and with nothing selected
they say what to select. The button lights whenever a gizmo is up, because the
gizmo is a property of the selection (V-38) and there is no mode to enter.

**V-86 · A camera drag survives the chrome.** An orbit is decided when the
button goes down, not re-litigated every pixel: the drag used to freeze the
moment the pointer crossed the toolbar or the tree and resume on the way back,
which read as the camera stuttering. An active orbit, pan or cube drag now keeps
receiving input wherever the pointer is; starting one over chrome is still
impossible, because the press is only honoured inside the viewport.

**V-87 · Small keeps from the same audit.** Escape cancels the armed "click a
plane" state the hint bar was already promising it would. Cancelling an extrude
restores the camera the tool tilted, as its own comment claimed. A recovered
autosave no longer plants its hidden-folder path in the recents or the
last-used directory. Closing the colour picker mid-scrub commits the drag it
was holding, instead of wedging the bus. The autosave writes after EndDrawing
rather than mid-frame, so the two-minute tick cannot hitch a stroke. The saved
window rectangle — written since M8, read by nothing — is applied at launch,
position only if it still lands on a monitor. And a dropped .ship opens the way
a dropped .hex imports, through the same guard as Ctrl+O.

**V-88 · The look: elevation, and a ladder with wider rungs.** "Make the UI
cooler, looks crappy and low quality" — and the diagnosis is that nothing in
the chrome had any depth: background, panel and card sat within a few points of
value of each other and nothing cast a shadow, so every card read as pasted
on. Three moves, all in the kit so every widget gets them for free: the value
ladder deepened and widened (theme.go), every floating surface — card, popover,
modal, tooltip, toast — now sits on a layered soft shadow with falloff (equal
rings read as a sticker outline; the falloff is what makes it a shadow), and
raised surfaces carry a one-pixel top bevel. The viewport gradient also
flipped: it ran dark-on-top, and light falls from above. The active tool
button gained an accent wash beside its underline. A toast stack draws all its
shadows before any body, because a shadow painted over the neighbouring toast
is a halo, not depth. Every golden regenerated — the gradient touches every
pixel — after eyeballing the sample, sketch, paint and transform shots.

**V-89 · The sketch grid has a step, and it is one setting.** The user asked
for a way to change the grid size while sketching. `Settings.GridStep` had
existed since M2 — the snap read it, nothing set it, and the drawn grid
ignored it at a hardcoded unit. Now one row of chips on the sketch card
(0.25 / 0.5 / 1 / 2 u) writes the setting, and both the snap and the drawn
grid read it, so the lines the eye lands on are the lines the point lands on.
The major line follows every eighth minor rather than every eight units,
keeping the visual rhythm identical at every step — and making the default
grid pixel-identical to what the program always drew. Ctrl remains the ¼ u
override and Alt still suppresses snapping (SPEC-UX §16). The step persists in
settings, drives the `sketch.grid` script op, and `m9_grid` pins two spacings
against each other: a step that changed nothing on screen would fail the test.

**V-90 · The view cube owns its clicks.** Clicking a cube zone while sketching
turned the camera *and* put a point down through it — updateSketch consumed
presses by plane geometry alone, and the cube lives inside the viewport. Idle
mode has known this since M1 (handleViewportClick always checked the cube);
every later tool had to learn it separately, so it now has a name —
cubeOwnsPointer — and sketch, boolean, both arrow gizmos and the transform
gizmo all ask it before taking a press.

**V-91 · Answering the welcome card puts it away.** "I click new ship and
message wont go away" — and it could not: the card shows for an empty, clean,
unnamed document, and New ship replaces the empty document with another empty
document, so the show condition was true again the same frame. The card was in
a loop with its own primary button. It now carries a session-scoped dismissed
flag, set by any answer: a button (New ship via NewDocument, so Ctrl+N counts
too), Escape, or a click that starts work in the viewport. The hole never
surfaced before because until V-84 the interactive app never started empty, so
the card had never actually been clicked. The state machine is pinned in
welcome_test.go, GPU-free; there was no golden of the card and the layout was
never the problem.

**V-92 · A drag update announces what its undo changed, not only what its do
did.** "Dragging one end of a line up and down leaves behind artifacts" — the
rubber-band replace (UpdateDrag) undoes the previous frame's shape and draws
the new one, and the document came out perfect every frame. The bus then
emitted only the new command's events, so the renderer's texture cache — a
mirror of the document driven by dirty rects — never heard about the pixels
the undo had restored. The old shape stayed on screen wherever the new one's
rect did not cover it. Freehand strokes only ever append, which is why M7
never met this; shapes shrink and swing, and the gradient escaped because its
dirty rect is the whole face. UpdateDrag now emits the previous command's
events after the new one's Do succeeds, so both regions re-upload. Pinned by
TestRubberBandReplaceKeepsAMirrorTrue, which rebuilds a texel mirror from the
event stream and demands it match the document exactly — the honest statement
of what a dirty rect is for.

**V-93 · Tool groups and a flyout, added to the frozen widget set.** SPEC-UX §4
freezes the widget list; SK1 adds two — `ChevronButton` and `Menu` — because
the sketch toolbar grew from four tools to a dozen with more coming
(Sketch_func.md). Without grouping either the toolbar overflows the minimum
window or every variant competes for its own letter. The group button shows
the variant last used and its key cycles the group, so the flyout is discovery
and the key is speed; neither is the only way in. The `Menu` is deliberately
not a general menu system — no submenus, no separators, no traversal. It lists
a handful of siblings and closes.

**V-94 · Construction geometry is absent from `Segments()` and nowhere else.**
A guide must not close a region or ring as an open end, and it must still be
drawn, snapped to and selectable. One skip, at the single seam where entities
become segments, buys all of that: the region engine simply never sees them,
and every other consumer — drawing, snapping, picking, saving — is unchanged.
`Q` converts a selection or, with none, arms the mode for what is drawn next.

**V-95 · The aligned rectangle commits four lines, not a new entity kind.**
`EntRect` is axis-aligned by definition — two corners cannot express a
rotation — and a rotated rectangle is exactly its four edges. The tradeoff is
real and accepted: it cannot later be selected or dragged as one unit the way
a rectangle can. Adding a kind to avoid that would mean a fifth set of
Points/Closed/Degenerate cases, a serialization change and a snapping case, to
buy grouping that the Select tool's box-select already approximates.

**V-96 · A point is an entity that makes no segments.** `EntPoint` returns a
single position from `Points()`, so `AppendSegments` skips it for having
nothing to join — the region engine needs no special case at all. It needed
three: `Closed()` must exclude it, `Degenerate()` must not judge it by the
line rule (a point at the origin has A == the unset B, and is a real place to
put one), and the pick distance measures to the position rather than to a
segment that is not there.

**V-97 · `Escape` steps back one point only for staged gestures.** The aligned
rectangle takes three clicks, and losing all of them to one reflex would be
the same mistake the close prompt made (V-81). The line chain deliberately
keeps its old behaviour: its hint promises "Esc to cancel the chain", and its
placed points are already committed entities, so there is nothing but the
pending point to lose.

**V-98 · `ReplaceEntities` is the shape of every modify tool.** Fillet trims
two lines and adds an arc; mirror adds copies; offset swaps a chain for a
parallel one. As remove-then-add pairs each would be two history entries for
one gesture and could strand a half-edited sketch if the second failed. One
atomic command, validated before anything moves, undone by removing the
additions from the end and reinserting the removals at their recorded indices.
SK1 ships it with the aligned rectangle as its first user.

**V-99 · A curve's tessellation ends exactly where it was built.** An arc's
first and last points, and an ellipse's major-axis endpoint, are written back
verbatim after the trigonometry rather than left as whatever the cosine
rounded to. Those positions are snap targets and the places lines join: one
subunit of drift and a profile of three lines and an arc has four open ends
and no region instead of a closed outline — silently, and only visible when an
extrude refuses. Everything between the ends is ordinary rounding
(SPEC-GEOMETRY §3). Pinned per kind, and end to end by
TestAnArcClosesAProfileWithItsLines.

**V-100 · Three gestures, one arc.** Centre, 3-point and tangent are three ways
to say the same thing, and they all build the same `EntArc` — centre, start,
end, direction. Only the construction differs, and it lives in
`internal/sketch/curves.go` as pure functions tested on their own, because a
circumcentre a few subunits out draws a shape that looks perfectly plausible
and puts every later join in the wrong place.

**V-101 · The tangent arc demands an endpoint, and says so.** Tangency needs a
direction, and the only honest source of one is an entity that already ends
there. Rather than guess from the nearest geometry or default to horizontal,
the first click must land on a loose endpoint; if it does not, the tool
refuses with a reason (Sketch_func.md §6). The session learns what is drawn
through `SetContext`, fed each frame rather than held, so an undo cannot leave
it pointing at entities that are gone.

**V-102 · Arc segments are spent at circle density.** `Segs` means "sides for a
whole circle", so an arc takes the fraction its sweep covers. A quarter arc at
32 is eight segments, not thirty-two crammed into ninety degrees. That keeps
one number meaningful across circles, arcs and ellipses alike — the card row
is now "Curve segments" — and keeps a hull outline's arcs the same smoothness
as its circles.

**V-103 · A refused gesture is over.** A click that cannot make a shape ends
the attempt and says why, for every tool. Keeping the good points so the user
can retry the last one is kinder for three-click tools and inconsistent with
the circle and rectangle, which have reset since M2. One rule, explained once,
beats a better rule that applies to half the toolbar.

**V-104 · Elliptical arcs and conics are not built.** Sketch_func.md §2 listed
them as stretch goals inside SK2. They are the two shapes from the Onshape
toolbar with no obvious use in a low-poly hull that the arc and ellipse
already shipped do not cover, and each needs its own entity kind, gesture and
tessellation. Deferred, not refused: the kinds are reserved in the enum order
of Sketch_func.md §3 and can be appended without disturbing anything.
