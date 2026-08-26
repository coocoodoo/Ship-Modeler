# Progress Journal

> Executor: append an entry per working session. Newest entry at the TOP. Keep entries honest — failed attempts and open bugs belong here, not just wins.

**Current state:** **M6 COMPLETE** — bodies, faces, edges and vertices are all selectable and movable; box select, move and rotate gizmos, Ctrl+D. Next up: **M7** (paint mode).

---

## 2026-08-26 — Fix: closed sketches could not be clicked

**Reported by the user:** closed sketches can't be selected to extrude them.

**Confirmed, and it had been true since M2.** A finished sketch draws as an
overlay, not as geometry, so it was never in the ID buffer the pick pass reads.
Clicking one selected whatever was behind it — which, on the plane it was drawn
on, is that plane. Clicking the sketch's outline hit nothing at all. Nothing
failed loudly; a closed profile simply looked unpickable.

The tree path always worked — select the sketch row, press E — which is why
`TestExtrudeStartsFromASketchPickedInTheTree` passed all through M3 and why this
went unnoticed for four milestones. Every automated route into extrude went
through the tree or through the `extrude` op directly. Nothing clicked a sketch
in the viewport, because until now nothing needed to.

**Fixed:** a click that lands on nothing, or on a plane, now tests the visible
sketches first, in the sketch's own plane rather than by rendering ids — the
same point-in-region code the sketch-mode hover already uses, and exact. A body
in front still wins; the nearest of two overlapping sketches wins. Open profiles
are pickable by their strokes, closed ones anywhere inside.

**The hint bar disagreed with the click, and that took longer to find than the
bug.** With the click fixed, hovering a sketch still read "Top plane". The hint
was reaching `TreeHover` first — which is fed from the viewport hover — so the
fix belonged in `viewportHoverRef`, not in a special case inside `HintText`.
Putting it there means the tree row highlights too, which is what hovering
anything else already does.

**Verified:** `TestClickingASketchSelectsIt` covers the whole path — hover names
the sketch, click selects it, E opens the extrude with its region picked, and
the committed solid is 108 (a 6x6 square pulled 3). Full suite green, no golden
drift.

---

## 2026-08-26 — M6: selection, direct edit, transform

**Done:**
- `model.Selection.VertIndices` — every direct edit is the same operation on a
  different set of vertices, so selection resolves to a deduplicated set first.
  A vertex shared by two selected faces moves once, or the shape tears.
- `model.MoveVerts` / `model.RotateVerts` — translations and rotations with the
  planarity policy of SPEC-GEOMETRY §7.2 and exact undo from saved positions
  rather than arithmetic run backwards.
- `model.DuplicateBody` — Ctrl+D, offset one unit, fresh identity and fresh
  face ids so paint and selection do not follow the copy home.
- `tools.TransformTool` + `scene/transformgizmo.go` — three axis arrows, three
  planar handles, a screen-plane centre, and three rotation rings, all sized in
  screen pixels and hit-tested against the geometry that is drawn.
- `app/transform.go` — the drag runs live through the bus, so the model really
  is moving as you move it, and lands as exactly one undo step. Escape puts it
  back and records nothing.
- `app/boxselect.go` + `render/boxpick.go` — drag on empty space, filter chips
  for Verts / Edges / Faces.
- Clicking an edge selects the edge, a vertex the vertex, and a second click on
  a selected face takes the whole body.

**Verified:**
- **M6's acceptance, as arithmetic.** Box-select the hull's four nose vertices
  from a front-on view, drag them 4 units: the hull grows by **exactly 96**,
  which is its 4×6 end face times four, and nothing bends. Rotate a wing 90°
  about Y: its volume is **unchanged to the bit**.
- Four quarter turns return every vertex to its original bits — not to a
  tolerance. A rotation matrix could not do that: `cos(pi/2)` is 6.1e-17.
- Moving one corner of a box bends exactly the three faces that meet there, and
  undo clears the flags rather than merely leaving them set.
- One drag is one history entry; undo and redo both land on exact volumes.
- Box filters: the same rectangle over the hull collects 16 vertices, 24 edges
  or 6 faces depending on the chip.
- `gofmt -l`, `go vet ./...` clean; `go test ./...` green across 13 packages.
- Shots read: `m6_boxed`, `m6_stretched`.

**The spec's box-select design does not work, and why:**

SPEC-RENDER §6.2 specifies one full-viewport ID render scanned by rectangle.
For faces that is right. For vertices it cannot work, and the case where it
fails is the exact case box select exists for. Two vertices at the same screen
position — the near and far corners of a hull seen straight on, which is the
view you stretch a nose in — share one pixel, and one pixel holds one id.
Whichever drew last wins; the other is invisible to the scan. The first attempt
selected two of the four nose corners, and dragging would have torn the hull in
half.

Turning the depth test off did not help: they are not occluded, they are
coincident. Vertices and edges are now collected by projecting them and testing
against the rectangle, which is exact, has nothing to read back, and is faster
than the render it replaced. Faces keep the ID render, because "is this region
inside the rectangle" is a question about area.

**Decisions/deviations:** V-36 (box select projects points and lines), V-37
(quarter turns are permutations), V-38 (the off-grid warning is about the
change), V-39 (second click takes the body), V-40 (bent faces warn by toast,
hover chip deferred to M9), V-41 (a single flat face shows the push/pull arrow
alone), V-42 (bent faces triangulate on demand), V-43 (the new ops). All
goldens regenerated: a selection now means a gizmo and a card, so every shot
that ended with something selected changed.

**A latent bug this milestone surfaced:** the gizmos were armed inside the
branch that only runs when the pointer is over the viewport, so their pivot went
stale whenever the cursor sat over the tree — which in a headless run it always
does. They follow the selection now, not the pointer.

**Open issues:**
- The rotate gizmo's ring drag is implemented and reachable from the card and
  the `rotate` op, but there is no golden of a ring being dragged mid-turn.
- Rotation has no numeric angle field, only a readout. The move gizmo has its
  three; the rotate one should get one in M9's polish pass.
- `Del` on a face, edge or vertex refuses with guidance rather than doing
  anything. Deleting part of a solid leaves something that is not a solid.

**Next:** M7 — paint mode: per-face pixel-art textures, the texel cursor, the
palette, and strokes as undo steps.

**Try it (user):**
1. Run `modeler.exe` and click the hull. A move gizmo appears at its centre.
2. Drag the red arrow — the hull slides along X in whole units; hold Ctrl for
   quarter units.
3. Press `R`: the arrows become three rings. Drag one — it snaps to 90°, and
   holding Shift gives 15°.
4. Press `V`-style view keys or drag the view cube to a front view, then drag a
   box on empty space around one end of the hull.
5. The four corners there light up. Drag the red arrow and the hull stretches.
6. `Ctrl+Z` puts it back in one step, however long the drag took.

---

## 2026-08-26 — M5: sketch on faces & push/pull

**Done:**
- `model.Sketch` grew a face anchor: body, face identity, and a **frame
  snapshot**. The snapshot is what the sketch actually uses, so it survives the
  face being cut away — which happens almost immediately, since extruding a face
  sketch replaces the face it was drawn on.
- `internal/app/facesketch.go` — starting a sketch on a face, the flatness
  refusal, the face's outline as dim reference geometry, and **Project outline**.
- Reference snapping: the anchored face's corners and edge midpoints compete
  with the sketch's own on distance, not on which list they came from.
- `model.PushPull` + `tools.PushPullTool` + `internal/app/pushpull.go` — the
  hero tool. Select a flat face, drag its arrow: out adds, in cuts, decided by
  the direction rather than by a mode. Live preview of the material, CSG on
  release only.
- Clicking a face now selects the face (V-32), which is what makes both of the
  above reachable.
- Extruding from a face sketch defaults to **Add** on the body it was drawn on.

**Verified:**
- **M5's acceptance, as arithmetic.** A stepped hull with two windows, built
  without touching a default plane after the first: slab 12×8×3 = **288**; a
  face pulled out 3 → **360**; a block grown from a face sketch → **504**; two
  2×2 windows cut 2 deep → **488**. Every figure exact.
- Push/pull both ways on the test hull: pulling the 5×4 top out 2.5 adds
  **50**, pushing the 4×6 side in 2 removes **48**, and the previews before each
  change nothing — CSG runs on release, as SPEC-UX §12.5 requires.
- A face with a hole pushes as a ring: 10×10 minus 2×2, three units tall, hole
  still open.
- Pushing a face further than the body is deep empties it legally and undoes.
- `gofmt -l`, `go vet ./...` clean; `go test ./...` green across 13 packages.
- Shots read: `m5_stepped`, `m5_pulling`, `m5_pulled`, `m5_pushing`,
  `m5_projected`.

**The thing M5 found, which had been there since M0:**

The test scene's hull was two boxes joined with `mesh.Merge`, which concatenates
meshes rather than unioning them. It rendered correctly, picked correctly, and
passed every check in the validation gate — each shell is closed, manifold,
positively oriented, with the right Euler count — and its volume agreed with
Manifold's, because the two shells enclose no shared volume. It was still not a
solid: the plane where they meet carries two coincident surfaces, so the shells
touch rather than join.

Manifold's answer to a union onto such a mesh is to return the target unchanged.
Not an error, not a warning: nothing. Every M4 boolean test passed because none
of them unioned *onto* the hull — they subtracted from it, which happens to work.
The first thing that did was M5's "extrude a face sketch and add it to its body",
which reported success and changed nothing.

The scene is fixed and `mesh.Merge` now says what it is and is not for. A
validator check was written for the general case and then removed: telling "two
solids pressed together" apart from a legal edge-to-edge touch means asking
whether two coplanar opposed faces overlap over an *area*, and the version
precise enough to pass M4's acceptance matrix still rejected two of its cases. A
check that rejects correct geometry is worse than no check, so the gap is
documented (DECISIONS V-33) rather than half-covered.

**Decisions/deviations:** V-31 (a sketch stores its plane, not a reference),
V-32 (clicking a face selects the face), V-33 (the validation gap above), V-34
(push/pull is its own command), V-35 (the new ops). All goldens regenerated: the
hull's silhouette changed when it became a real solid.

**Open issues:**
- The validator does not detect shells that touch face to face (V-33). Only
  `mesh.Merge` can produce one, and it is now the only caller's job to know that.
- Push/pull arms on the selected face every frame from Idle. Multi-face push
  (drag several coplanar faces at once) is not built; nothing in the spec asks
  for it yet.
- Reference geometry is drawn and snapped to but not pickable, which is right —
  though it means there is no way to select a face edge *as* an edge until M6.

**Next:** M6 — unified selection (bodies / faces / edges / verts with the pick
priority of SPEC-RENDER §6), direct vertex and edge editing, and the transform
gizmo.

**Try it (user):**
1. Run `modeler.exe` and click the top of the hull's raised block.
2. An arrow appears on the face. Drag it up — the block grows; drag it down
   past where it started and it cuts in instead.
3. Press `S` with that face selected: you are now sketching on it, looking
   straight at it, with its outline drawn dim underneath.
4. Click **Project outline** in the card to turn that outline into real lines.
5. Draw a rectangle inside it and press `E` — the Result chips open on **Add**,
   because a sketch on a face belongs to that body.
6. Pick **Subtract** instead and it cuts a window rather than adding a block.

---

## 2026-08-26 — M4: booleans via Manifold

**Done:**
- `third_party/manifold/` — v3.5.2 built static with gcc 15.2 (MinGW-UCRT),
  `MANIFOLD_PAR=OFF`, tests off, cross sections **on** because `MANIFOLD_CBIND`
  depends on them. Exact tag and flags in DECISIONS D-12.
- `internal/geom/csg/manifold.go` — the binding. Uses the **float64** MeshGL,
  which the spec did not know existed and which retires its float32 caveat.
- `internal/geom/csg/convert.go` — the converters: triangles in, polygons with
  holes out, provenance carried on Manifold's reserved run ids.
- `internal/model/boolean.go` — the Boolean command; `internal/model/extrude.go`
  grew Result modes. Both build and validate everything before writing anything.
- `internal/tools/boolean.go` + `internal/app/booleanmode.go` — the Boolean tool:
  pick what survives, pick what to combine, Enter. Picked bodies tint.
- `internal/app/csgdebug.go` — the §11.3 failure path: standard toast, document
  untouched, OBJ repro under `debug/csg/` when `MODELER_CSG_DUMP` is set.

**Verified:**
- **The 25-case acceptance matrix was written first**, against an API that did
  not exist, and all 25 pass — including the flush butt-join family, edge and
  vertex touches, the one-subunit sliver, and the ten-op chain.
- **200-op fuzz with an independent oracle** passes: every step validates and
  agrees with Manifold's own volume, and every 20th step a 96³ voxel comparison
  replays the op history as arithmetic and ray-casts the mesh. Deterministic
  across three consecutive runs.
- Paint survives a cut: fragments share the source `FacePaint` by identity and
  sampled colours away from the hole are unchanged.
- End to end: Boolean tool subtract 348 → 312 with the tool consumed; extrude
  Result=Subtract through-all cuts 348 → 285 (a 3×3 column through 7 units);
  Result=Add merges 348 → 364 without creating a body; undo restores all of it.
- `gofmt -l`, `go vet ./...` clean; `go test ./...` green across 13 packages.
- Shots read: `m4_picking`, `m4_applied`, `m4_cut`, `m4_union`, all in
  `docs/shots/`.

**Decisions/deviations:** D-12 filled in; V-23 (float64 MeshGL), V-24 (merge
table, not distance, decides which vertices are one), V-25 (coplanar faces merge
across sources except when both are painted), V-26 (straightening is a
whole-mesh decision, and never collapses a spur), V-27 (pinched vertices split
per fan), V-28 (zero-volume shells dropped), V-29 (glyph check over every
message), V-30 (new ops). All goldens regenerated — the toolbar's Boolean button
is live now, and the stale-baseline check from M3 caught it.

**What the fuzz found that nothing else would have:**
Four defects, all in the converter, none reachable by a single well-formed op.
A boundary walk that started at whichever vertex Go's map iteration offered
first, so the same boolean produced different face numbering run to run. A
straightening pass that collapsed the tip of a zero-width spur and invented an
edge running past vertices the neighbouring faces still held. A face pinched to a
point, which Manifold permits and our document cannot represent. And a
near-degenerate sliver from a free-rotated box, which has a normal in the
arithmetic sense and noise in every other sense, seeding a patch of its own.
Each was found by the chain reaching a state no isolated case would reach.

**Two cgo lessons, both paid for in crashes:**
A Go struct holding Go pointers cannot cross into C — the MeshGL options struct
is exactly that, so everything is copied into C memory first. And Manifold
constructs with placement new into a buffer you supply but destroys with
`delete`, so that buffer must come from C++'s `operator new`; malloc memory dies
inside the deleter on this toolchain. Manifold's own C tests sidestep this by
never destroying anything, which a session running a boolean per click cannot do.

**Open issues:**
- Target picking for Add/Subtract/Intersect uses bounding-box overlap, not real
  intersection. A box that overlaps but does not touch offers a chip that turns
  out to do nothing; the alternative costs a full boolean on every slider move.
- The async spinner for ops over 120 ms (SPEC-UX §9.5, §6.6) is not built. Every
  operation measured so far is well under it, so there has been nothing to hang.
- Paint is shared by pointer between fragments, which is what §3.4 asks for.
  Repainting one fragment will need copy-on-write; that belongs with M7.

**Next:** M5 — sketch on faces and push/pull. Face plane becomes the sketch
plane with a persistent frame snapshot, and push/pull is an extrude of the face
outline through the same command bus.

**Try it (user):**
1. Run `modeler.exe`. The scene starts with a hull, an engine pod and a wing pod.
2. Click **Hull** in the tree, then press `B`.
3. Click the hull in the viewport (it tints blue — this one survives), then the
   wing pod (it tints red — this one gets used up).
4. Pick **Subtract** in the card and press `Enter`.
5. Press `Ctrl+Z` — the wing pod comes back exactly where it was.
6. Or: press `S` on the Top plane, draw a rectangle over the hull, press `E`,
   and pick **Subtract** in the Result chips to cut it out instead.

---

## 2026-08-26 — M3: extrude to a new body

**Done:**
- `internal/geom/extrude` — `Build(regions, Params, bodyID)`: caps triangulated
  from polygon-with-holes, side quads under draft via the M3 miter offset, per-face
  stable IDs, weld, `mesh.Validate` gate. Normal / Reverse / Symmetric; symmetric
  with draft builds three rings (two frusta widest at the sketch plane).
- `internal/tools/extrude.go` — the tool's pure state: signed drag depth with the
  sign folded into the direction on read, grid/fine/free snap, flip, draft clamp
  reporting, result availability, Through-All, and the arrow's screen hit test.
- `internal/model/extrude.go` — the command. Builds and validates the whole solid
  before touching the document, so a refusal leaves it byte-identical. Reuses the
  body id across redo so face identities stay stable.
- `internal/app/extrudemode.go` — arrow drag, live preview at 55% alpha, camera
  tilt off the axis, commit/cancel, and the scene measurement behind Through-All.
- `internal/scene/gizmo.go` — the arrow: shaft, cone as a triangle fan, base dot.
- `internal/app/shell.go` — the options card (depth + flip, direction chips, draft
  slider + field with the clamp warning, result chips, Through-All), an Extrude
  button on the sketch toolbar, and the main toolbar's Sketch and Extrude buttons
  finally wired to the actions their shortcuts already ran.
- Script ops `extrude`, `extrude.begin`, `extrude.commit`, `extrude.cancel`, plus
  `shift`/`ctrl`/`alt` modifiers on `click` and `hover`.

**Verified:**
- `go build ./...`, `go vet ./...` and `gofmt -l` all clean.
- `go test ./...` green across 12 packages; `internal/apptest` 28.6s.
- Volumes checked against the closed-form frustum `V = h/3(A1 + A2 + sqrt(A1 A2))`,
  not against recorded numbers: cube **216.0000 exactly**; 12° draft over 6 u
  **137.2856** vs 137.2856 analytic; symmetric 10° **180.2560** vs 180.2560;
  two disjoint squares **72.0000 exactly**; through-all volume equals 4x its own
  reported depth.
- Draft clamp: 40° on a 2 u square clamps to **9.43°**, volume **8.0314**, which is
  the frustum formula for a taper stopping one subunit short of collapse.
- Shots produced and read: `m3_straight`, `m3_draft`, `m3_symmetric`, `m3_gizmo`,
  `m3_through`, `m3_touching`. All six are in `docs/shots/`.
- Through-all verified from the Right view: the bar spans the hull and pokes one
  unit past each end, which is the margin.

**Decisions/deviations:** DECISIONS.md V-17 (symmetric draft = two frusta),
V-18 (through-all reach measured both ways, +1 u margin), V-19 (camera tilt tested
against the axis, not against named views), V-20 (goldens fail as stale when they
drift inside tolerance), V-21 (a dump counts as an observation), V-22 (the new ops).
**Every golden in the repo was regenerated** — see the stale-baseline note below.

**Two things the session turned up that were not on the checklist:**

1. *Every golden in the repo was silently out of date.* Enabling the toolbar's
   Sketch and Extrude buttons changed about 0.04% of each frame — comfortably
   inside the 99.7% tolerance TESTING §5 sets — so all sixteen goldens kept
   passing while none of them matched the app any more. SPEC-RENDER §10 calls
   these "same-machine baselines", so the honest expectation is an exact match and
   the tolerance is a cushion for driver updates. `checkGolden` now fails a shot
   that passes tolerance but has more than 200 changed pixels, with the diff image
   and the instruction to regenerate. It caught this the first time it ran.

2. *Extruding two regions that touch produced mesh jargon.* A circle inside a
   rectangle makes a ring and a disc; picking both and extruding gave
   "face 0 loop 0 repeats vertex 3". Each region becomes its own shell, and two
   shells sharing a wall — or even a single corner — weld into edges belonging to
   four faces, which is not a manifold and not something extruding harder can fix.
   `extrude.Build` now detects any shared boundary point up front and refuses with
   "Regions 0 and 1 touch — extrude them one at a time, or combine them once
   booleans arrive". The card's Extrude button greys out with that as its reason.
   Written test-first; the corner case corrected a wrong assumption of mine that a
   single shared point would be fine.

**Open issues:**
- Adjacent regions can only be extruded one at a time. Merging them properly is a
  2D union, which is M4's business.
- `Result` = Add / Subtract / Intersect are visible and disabled, as specified,
  until the boolean kernel lands.
- Ctrl-fine and Alt-free snapping on the arrow drag are covered by unit tests but
  not by an end-to-end script: the runner has no press-move-release drag op yet.
  Worth adding when M6 needs it for transform gizmos.

**Next:** M4 — write the boolean acceptance matrix first (TESTING §3, ~25 named
cases including the flush butt-join family), then build and vendor Manifold.

**Try it (user):**
1. Run `modeler.exe`, click the **Top** plane in the tree, press `S`.
2. Press `R` and drag out a rectangle.
3. Press `E` — the camera swings to a three-quarter view and an arrow appears.
4. Drag the arrow to set the depth; hold `Ctrl` for quarter units.
5. Push the **Draft** slider to about 12° and watch the crate taper live.
6. Press `Enter`. The body lands in the tree and the sketch hides itself.

---

## 2026-08-26 — Fix: finished sketches were invisible

**Reported by the user:** sketches disappear once you finish them.

**Confirmed, and it was mine.** The viewport only ever built a draw list inside
an `if a.InSketch()` guard, so the moment the session ended the sketch stopped
being drawn. It was still in the document — entities, regions, tree row all
intact — just never rendered. The same gap from the other end: `Sketch.Visible`
was written by the tree's eye toggle and reported in dumps, but nothing in the
viewport read it, so that toggle controlled nothing at all. I had marked M2's
"tree section, rename/hide/delete" checkbox complete; the hide half was not
honestly done.

**Why the tests missed it:** every M2 golden was shot *while in sketch mode*.
The script called `sketch.finish` but only dumped afterwards, never took a
picture. The dump assertions all passed because the document was correct — the
defect was purely in what got drawn.

**Fixed:** `render.Scene` now carries a list of sketch overlays rather than one,
and `BuildScene` builds one for every visible sketch, with the active sketch
last so it lands on top. An idle sketch draws as quiet dimmed strokes: no region
fills, no red rings, no snap glyphs, because those all answer "what am I about
to do", which only the active sketch is being asked.

**A second bug, in the test I wrote to catch the first.** My regression test
asserted `!res.WithinTolerance()` to mean "the picture changed" — but
`WithinTolerance` asks the opposite question, "does this match its baseline",
and allows a 0.3% outlier budget for driver variation. The sketch's thin strokes
cover 0.158% of the frame, so the test reported no change while the feature was
working. There is now an explicit `Differs()` that counts changed pixels, used
by both this test and the tree-collapse one.

**Verified:** new `sketch.visible` op; `TestFinishedSketchStaysVisible` renders
the sketch after finishing and again with it hidden and requires the two to
differ, so the bug cannot come back from either side. New goldens
`m2_after_finish.png` and `m2_sketch_hidden.png`. 217 tests pass.

---

## 2026-08-26 — M2: sketch mode, the region engine and R3

**Done:**
- **`internal/geom/sketch2d`** — the integer-exact planar arrangement of GEOM §4, written test-first as the protocol requires. Split at every meeting point, weld by exact coordinate, sort each node's darts with an exact half-plane-plus-cross comparator, walk faces by taking the dart clockwise from each twin, then nest each component's outer boundary into the smallest face that strictly contains it. That last step gives holes and one-level islands in the same pass. Plus ear-clipping triangulation with hole bridging, so a ring renders as a ring.
- **`internal/model`** — sketch entities in the logical form the user drew: a Line keeps its two points, a Rect keeps two corners and draws four sides, a Circle is a regular n-gon (D-06). Five commands cover drawing, deleting, moving and re-segmenting, and the arrangement is cached behind an edit stamp so the several times a frame the UI asks for it cost one build.
- **`internal/sketch`** — snapping and the tool state machine. Endpoint beats midpoint beats grid, radii scale with zoom so they feel identical however far in you are, Alt suppresses everything, and horizontal/vertical inference locks the direction then lets the grid quantise along it.
- **`internal/render` + `internal/scene`** — the sketch overlay: translucent region fills, entity strokes, the rubber band, dashed inference guides, red rings on loose ends, and the snap glyph under the cursor (circle, diamond or cross, matching what it latched onto).
- **`internal/app`** — sketch mode itself: entering on a plane with the camera animating normal-on and the model dimming to 30%, the toolbar swapping to Select/Line/Rectangle/Circle, and a contextual card reporting entities, regions and open ends.

**Verified:**
- `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean. `go test ./...` — **216 tests pass** (166 at the end of M1).
- New coverage: sketch2d 8 top-level tests spanning **16 region cases** and 8 triangulation cases; sketch 21; model grew to 39; apptest to 22.
- **The 16 region cases are the TESTING §2 canon and then some**: square, rect with a circle hole, three-deep nested islands, figure-eight, butt joint on a shared edge, overlapping rects, open chain, T joint, duplicate segments, collinear partial overlap, crossing diagonals in a frame, zero-length segments, a one-subunit sliver, a bare circle, two disjoint squares, and nothing at all. Each asserts region count, hole count, open-end count and **exact** area.
- **Triangulation is checked by exact area preservation**, not a tolerance: the triangles must sum to the region's integer area or the test fails.
- **Determinism**: `TestBuildIsDeterministic` reorders the input segments and reverses each one's direction, and requires the identical arrangement — which is what makes region indices stable enough to select and to golden.
- **Goldens** (read, not just generated): `docs/shots/m2_rect_circle.png`, `m2_open_ends.png`, `m2_closed_region.png`, `m2_tools.png`.
- **R3 end to end**: `TestOpenProfileGatesExtrude` clicks three points, checks the chain reports 2 open ends and 0 regions, clicks the chain's own start, and checks it becomes 1 region with none loose — then undoes one stroke and watches it reopen.
- **Performance**: the 500-segment budget case of GEOM §4 runs in **0.38 ms** against a 2 ms budget. Frame cost in sketch mode is 3.34 ms at 720p against 16.6 ms.

**Decisions/deviations:** five entries appended — V-12 sketch overlays draw on top of the model rather than depth-tested (a profile on the Front plane was completely hidden by the hull until this changed); V-13 collinear overlaps resolve by construction rather than interval arithmetic; V-14 a line chain commits one entity per segment, so every click is its own undo step; V-15 a sketch row's double-click re-enters editing and the pencil renames; V-16 the `sketch.tool` op and the dump's sketch line.

**Bugs found and fixed:**
1. **Nothing in a holed polygon was ever clippable.** The ear test rejected any vertex touching the candidate triangle, but a hole bridge deliberately doubles two vertices — and a duplicate sitting exactly on a corner *is* that corner. Every ring triangulated to nothing until the test compared by position rather than index.
2. **The region engine missed its budget by 6x.** The first measurement was 13 ms, but on the wrong workload: 200 inputs that cross into ~10k edges, not the 500-segment sketch the spec means. Measuring the right case gave 2.48 ms, still over; hoisting the bounding-box reject out of the O(n²) split and replacing two `sort.Slice` calls with insertion sorts brought it to 0.38 ms.
3. **Sketch-mode shortcuts died over the tree panel.** Tool keys were handled inside the pointer branch, so resting the cursor on the panel made L and R stop working. Keyboard handling now runs regardless of where the pointer is.
4. **The first sketch was invisible.** Drawn on the Front plane, depth-tested, and therefore behind the hull the plane passes through.

**Open issues:**
- The Select tool picks and deletes entities and selects regions, but dragging an entity or endpoint to move it is not wired up yet; `MoveEntities` exists and is tested, so this is UI work rather than model work.
- Region selection is recorded but does nothing visible beyond the brighter fill — M3's extrude is what consumes it.
- Box-select in sketch mode (SPEC-UX §8.5) is not implemented; it shares machinery with M6's box select and is better done once.
- The "click the message to zoom to the nearest open end" affordance of SPEC-UX §8.6 is not built; the count and the red rings are.
- Sketches are still plane-only. Face sketches, with their frame snapshot, are M5.

**Next:** M3 — extrude. Start with the analytic volume tests of TESTING §2 (box, n-gon prism, drafted box against the frustum formula, symmetric, region-with-hole tube), then build the prism from a region, then the arrow gizmo.

**Try it (user):**
```bash
c:\go\bin\go.exe run ./cmd/modeler
```
1. **Click the Front plane** in the tree, then **click it again in the viewport** — the camera swings normal-on and the grid appears.
2. With the **Line** tool, click three corners and then **click your first point again** — the ring turns green as you approach it, and the profile fills.
3. Watch the **red rings** on loose ends while the chain is open, and the card counting them.
4. Press **R** for Rectangle and **C** for Circle; the card's chips change how many sides a circle gets.
5. Hold **Alt** while drawing to switch snapping off, or **Ctrl** for the quarter-unit grid; move nearly-horizontally and watch the **dashed guide** appear.
6. Press **Esc** to drop a half-drawn chain, and again to leave — the sketch is kept either way.

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
c:\go\bin\go.exe run ./cmd/modeler
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
