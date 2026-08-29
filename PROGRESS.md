# Progress Journal

> Executor: append an entry per working session. Newest entry at the TOP. Keep entries honest — failed attempts and open bugs belong here, not just wins.

## 2026-08-28 — Toolbar icon refresh

**Asked for:** Better icons across the UI.

**Done:** Reworked the high-frequency toolbar symbols so they share a clean,
consistent stroke system: Boolean now reads as an intersection, Move as a
balanced four-way cursor, and Paint as a recognisable brush. Circular artwork
now honors DPI-scaled stroke widths instead of falling back to one-pixel rings.
The file bar also has distinct import and export icons; Export finally shows an
outward arrow instead of reusing the import glyph.

**Verified:** `go test ./internal/ui ./internal/app`; refreshed and reviewed
the complete scripted visual-baseline set (`go test ./internal/apptest` with
baseline update enabled).

**Current state:** Baked ambient occlusion in the viewport (V-141). One
model, one pixel size — allocation follows existing
paint and the Res chips resample the whole model (V-140). Tile stamping shipped end to end — TP1-TP3 of Tile_paint.md
(V-138). Edge bands land on the face side of their edge, decided by
winding (V-137); edge picks follow the whole line through seam vertices
(V-136); edge paint bands reach their edges gap-free (V-135); fold
creases draw and pick at any angle (V-134); bent faces
fold into flat pieces along real creases (V-133), and the whole-corpus validity invariant is back from the dead. Placed
dots are selectable and draggable (V-132). .pxm
shipped end to end (V-131): markers in the modeler,
game payload in the file, loader + basis correction + thruster anchors in
Iron Drift. Both suites green. Repo now pushes to
github.com/coocoodoo/Iron-Drift-Modeler by the owner's instruction.

---

## 2026-08-28 — Simple ambient occlusion (V-141)

**Asked for:** "Can you do a simple ambient occlusion?"

**Done:** a per-corner CPU bake at mesh-build time — sixteen fixed hemisphere
rays per rendered corner against nearby triangles (2.5 u radius), the
weighted hit fraction packed into the vertex colour's spare blue byte,
interpolated across faces, multiplied into the lighting by a settings-owned
strength (`"ao": 0.5`, 0 disables, changing it needs no rebuild). The sample
point insets a third of a unit toward the face interior — an exact corner
sees an abutting wall edge-on and would sample brightest where it should be
darkest. Bakes skip while a drag is live (per-frame rebuilds), landing on
release. New `view.ao` op for scripts.

**Verified:** 5 render unit tests — lone convex plate fully open, the step's
inside corner darker than its open edge (and the wall base darker than the
wall top), geometry beyond the radius contributing nothing, byte-exact
determinism; the `ao_step` golden pair (default strength vs zero) pins the
look. Corpus: 62 body-bearing goldens regenerated after spot-reviewing three
diffs — the raised hull block grounds itself with soft contact darkening,
convex bodies stay clean; five 0-2 px jitter files reverted rather than
committed. Full fresh suite green.

**Next:** nothing outstanding.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Look at any inside corner - a step, a pocket, where a block meets a
   plate: it now sits in soft shadow.
2. Too strong or unwanted? Set `"ao"` in settings.json (0 to 1, 0 = off).


## 2026-08-28 — One model, one pixel size (V-140)

**The user, after the res chip bit them a second time:** "Why is the scale
tile sets a thing? I don't want to scale the pixels, I want them to be just
1px... the tile set division is what I'd like. Not SCALE."

They are right. V-128 made the chip a density so pixels would be uniform —
and left the leak: the chip only governed new faces, so a drifted chip put
two pixel sizes on one model. Closed in both directions:

- **New paint always matches existing paint.** `allocResFor` (body density,
  else document, else chip) feeds every allocation: strokes, shapes, fill,
  edge bands, tile stamps, and the ghost's provisional mapping. A stale chip
  from settings cannot diverge a model.
- **The Res chips are the model's pixel size.** With paint present the
  highlighted chip is the actual density, and choosing another runs the new
  `paint.ResampleModel`: every picture rebuilt at the new density in one
  undoable step — world positions preserved, fragment-face sharing
  preserved — announced with an Undo toast.

**Verified:** 4 new command tests (uniformity, shared pictures surviving as
one, exact-pointer undo, no-op and empty refusals); `tile_resguard`
rewritten to drive the whole contract including the stale-chip case
(desynced by undoing a resample — the bare face still previews and stamps at
the model's density); `m7_resample` rewritten to wholesale semantics;
`m7_painted`'s two-density ship is now a one-density ship and its test
asserts the user's own sentence unconditionally. Golden diffs read: the ship
shot's changes are the resample toast; the dead mismatch baseline deleted.
Full suite green.

**Superseded:** V-139's bare-face prompt (unreachable now, removed) and
SPEC-UX §13.2's "first stroke allocates at the selected chip".

**Next:** nothing outstanding.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Paint or stamp anywhere — every face, painted or bare, uses one pixel
   size. The chips cannot diverge it.
2. Want finer or chunkier pixels? Click a Res chip: the whole model
   resamples together, one Ctrl+Z brings it back.


## 2026-08-28 — The giant stamp, and the warning that was missing (V-139)

**Reported by the user** with their .pxm and screenshots: stamping the slope
worked; on the bare top face the stamp "gets huge".

**Diagnosed from their settings file:** `paint.res` sat at 1 px/u — a legal
chip, one miss-click under the tool icons. Painted faces pin their own
density (the slope stayed 8 px/u), but a bare face allocates at the chip, so
a 32 px tile landed 32 units wide. The ghost previewed it honestly; nothing
warned.

**Fix:** the density-mismatch prompt — which already covered painted faces —
now also fires for a bare face on a painted body: "New paint here lands at
N px/u — the rest of this body is M", one button, Use M. The tile hint names
the trap inline. The chip's first-paint contract is unchanged.

**Verified:** the recreation script pins the prompt (`resprompt offer=8
armed=1`) and the hint on a bare-face hover with the chip at 1;
fail-checked — reverting the bare branch loses the prompt line. Golden read:
the warn text and the Use 8 px/u button in the panel with the chip visibly
on 1. Full suite green.

**For the user's current file:** Ctrl+Z removes the giant stamp; click the
Res chip 8 (or the new Use 8 px/u offer) and stamps land right everywhere.

**Second report, same day:** "No matter what Res I choose, is not changing
size." Verified both paths headlessly: a bare face resizes with every chip
change; a painted face is pinned at its first density by design
(SPEC-GEOMETRY §8.2) — their face had been pinned at 1 by the giant stamp
itself. The gap was that nothing at the cursor said so, which made the Res
row read as a dead control. The tile hint now says "This face is pinned at
N px/u — Resample in the panel changes it", and the test pins the hint, the
prompt and the pinned painthover line together.

**Next:** nothing outstanding.


## 2026-08-28 — Tile stamping: TP1-TP3 in one sweep (V-138)

**Asked for:** "do them in one sweep" — the whole of Tile_paint.md's planned
scope: import a tileset, adjust the tile grid (8/16/32/64/Custom), select a
tile, stamp it onto the model.

**Done, one commit per milestone:**
- **TP1** `internal/paint/tileset.go` + `stamp.go`: Tiled-convention slicing
  (margin, spacing, partial columns dropped), D4 orientation
  (flip-then-rotate, all eight members witnessed by an asymmetric probe
  tile), `StampRect` (alpha >= 128 paints opaque, below leaves the surface
  alone — a stamp never erases), `StampFace` mirroring the stroke command:
  replayed from its cells, coalesced per mouse-down, dirty-rect undo,
  allocate-on-first-touch. `TileSettings` on io.Settings.
- **TP2** ops `tile.import/grid/select/orient/stamp` + a `tileset` dump line;
  the committed 19x19 fixture sheet is generated, never hand-drawn, pinned
  pixel-for-pixel by a test (`MODELER_WRITE_FIXTURE=1` regenerates). Flow
  test with hand-computed counts: two snapped stamps butt to 128 opaque
  texels, the turned stamp 192, the half-transparent tile adds exactly its
  32, undo hashes back bit-identical.
- **TP3** the Tile tool (`T`, sixth column of the tools rows), the panel
  section: Import via zenity (copied into the config dir), grid preset chips
  + Custom fields, the sheet picker (nearest-filtered, faint slicing grid,
  armed tile in the accent), rotate/mirror buttons, the ghost preview
  drawing the armed tile's actual pixels half-strength at the snapped cell,
  click stamps / drag trails / Alt places free, face lock respected, setup
  persisted like the palette.

**Verified:**
- 20 new paint-package tests (slicing tables, D4, snap flooring negatives,
  stamp clip/threshold/undo/coalesce/refusal, fixture pinning, settings
  round-trip), 4 apptest flows, 3 new goldens (stamps, panel+ghost, pointer
  click-stamp) — all read.
- **Fail-checked:** with the snap broken the flow test fails at 206 opaque
  texels where 224 belong — the unsnapped stamp hangs off the face and the
  clip eats it.
- The pointer path is exercised for real: a scripted `click` stamps 64
  texels through beginTileStamp -> bus drag -> commit.
- Full suite green. 26 paint-panel goldens legitimately regenerated (the
  tools rows went 5 -> 6 columns for the new tool; the diff was read — the
  change is exactly the second row gaining the tile icon). Ten more goldens
  the update pass touched showed 0-2 changed pixels of encoder jitter and
  were REVERTED rather than committed.

**Decisions/deviations:** V-138 — the picker fits the whole sheet in a fixed
box instead of fit-width+scroll; the footer stays under the tile section.

**Open issues:**
- TP4 (multi-tile block stamping, grid phase offset) remains parked for the
  user's go-ahead, per the plan.
- The picker has no hover highlight; clicking is the only affordance. Worth
  a polish pass if the tool sees use.

**Next:** nothing outstanding — TP1-TP3 complete.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Paint mode, pick the **Tile** tool (the 2x2-squares icon, or `T`).
2. **Import tileset PNG**, then set the grid chips to your tile size —
   or Custom for sheets with margins and gutters.
3. Click a tile in the sheet; hover the model — the tile previews on the
   surface, snapped to the grid.
4. Click to stamp; drag to lay a run; they butt seamlessly.
5. The rotate and mirror buttons turn the stamp; Alt places off-grid.
6. Ctrl+Z removes a whole trail at once.


## 2026-08-28 — The band lands on the face's side of its edge (V-137)

**Reported by the user, with their .pxm attached:** "still, didnt paint that
face side" — the ledge-end band showing on the ledge but not on the L-shaped
right wall below it.

**Diagnosed from the file by hand:** the wall is a hexagon whose vertex
average lands exactly on the painted edge's line. EdgeBand picked the band's
side by asking which side the centroid was on; the answer was neither, the
normal kept its default, and the band painted into texels the renderer never
shows. The unit recreation (the wall verbatim from the file) failed with 99
of 99 probes bare on the face side and 99 texels painted on the invisible
one — and argument order flipped the result, since the degenerate dot broke
the symmetry.

**Fix:** the winding decides. The outer loop projected into the paint frame
is counter-clockwise, so the interior is to the left of the loop's own
traversal of the edge; hole loops wind the other way and keep the material on
their left too. `loopWalksEdge` reads the traversal direction by exact vertex
match; the centroid survives only as the fallback for an edge not on the
face's boundary.

**Verified:**
- 2 unit tests on the wall from the file: band on the face side and nowhere
  else; identical texels whichever end of the edge is listed first.
- Flow test `edge_lwall` rebuilds the user's model from its own feature
  history (the .pxm feature list is the recipe: rect, extrude, face sketch,
  extrude union, move edge) and asserts both faces take exactly 48 opaque
  texels — 2 u of edge at 8 px/u, 3 wide. A probe sweep confirmed every edge
  of the shape now paints both its faces.
- Golden read: the orange band at the ledge end wraps onto both the ledge and
  the wall — the user's arrow spot, painted.
- Full suite green; no existing golden moved.

**Next:** nothing outstanding on this report.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Open your ship and repaint that ledge-end edge.
2. The band now lands on both faces — the ledge and the wall side.


## 2026-08-28 — A picked edge is a line, not a segment (V-136)

**Reported by the user:** "thats better, but it skipped this end" — the edge
band stopping mid-crest with an angled cut, bare face beyond.

**Diagnosed from the screenshot's signature** after two recreations came up
clean: the band ends exactly where a fold crease meets the crest. The crest
there is two collinear topology edges split at a vertex the model's history
left on the boundary. The click picked one segment; the band covered exactly
that segment; the user meant the line.

**Fix:** `paint.EdgeChain` — from the clicked edge, walk out of each endpoint
while exactly one other edge continues within 25 degrees of straight ahead;
corners and junctions stop the walk, and the continuation is deliberately not
required to be sharp itself (a fold piece that leaned does not cut the line
short). Clicking toggles the whole chain; Pick creases chain-completes its
sweep the same way. New `paint.pickedge` op drives the real click path in
scripts.

**Verified:**
- 4 unit tests on the seam prism: chain through the seam, stop at corners,
  shallow continuation joins, a real kink stops the chain.
- App flow `edge_chain`: flush-butt union, pick one edge by index through the
  click path, bake — toast pins the count; golden pins the pixels.
- Full suite green; no existing golden moved; existing "Picked N edges"
  toasts unchanged (chain-completion adds nothing on unsegmented shapes).

**Open issues:**
- App-level fixture gap, recorded in V-136: no op sequence currently
  manufactures a segmented straight boundary (booleans straighten collinear
  verts; fold chords end at corners), so the chain-through-a-seam case is
  pinned at the unit level only.

**Next:** nothing outstanding on this report.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Edge tool, click the crest that stopped short before — the whole line
   lights up, including the stretch past the fold vertex.
2. Paint: the band now runs to the end you pointed at.
3. Click the line again to unpick all of it at once.


## 2026-08-28 — Edge bands reach their edges (V-135)

**Reported by the user** with three screenshots: 3 px edge paint leaving
stair-stepped gaps along a slanted silhouette, a bare notch at a corner where
two bands meet, and a thin bare line down a painted crease.

**Recreated first, in a unit test:** the stepped-wedge profile face, whose
slope is a true diagonal in texel space. A probe every half-percent along the
edge, a sliver inside the face: **172 of 199 probes sat on bare texels.** The
old rasterizer walked the band as Bresenham dabs anchored at floored texels —
flush on lattice edges (the only kind ever tested), up to a whole texel short
on diagonals, always toward the same side.

**Fix:** the band is now the geometry it claims to be — the edge swept inward
by the width, an oriented rectangle in continuous texel space — and every
texel whose square genuinely overlaps it takes paint (strict separating-axis
test, so lattice bands keep their exact width). Overshoot past the edge is
invisible by construction: the renderer clips the picture to the face. Corner
miters fill by extending border ends by the band width; interior ends still
never extend (the union-seam nub rule). One visit per texel also removes the
old walk's double-blend on overlapping dabs.

**Verified:**
- 4 new paint tests: no-gaps along the slant (fails 172/199 against the old
  code), width honesty on the diagonal, corner turn, crease meeting. All 8
  prior edge contracts still pass unchanged.
- The lattice `edge_paint` golden did not move by a single pixel — the
  rewrite reproduces the already-correct cases exactly.
- New script `edge_slant` + golden: the wedge with 3 px creases; the zoomed
  shot read and checked — outer side flush against the silhouette, stair-step
  on the inner side only, corners filled.
- Full suite green; build/vet/gofmt clean.

**Next:** nothing outstanding on this report.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Make a shape with a slanted edge (your wedge), enter Paint, pick the Edge
   tool, width 3.
2. Paint the slanted edges — the band now hugs the edge the whole way, gap-free.
3. Corners where two bands meet are filled; creases show no bare line.


## 2026-08-28 — A shallow fold crease is still an edge (V-134)

**Reported by the user** with screenshots: after bending an edge on the
stepped-wedge shape, the fold happened but the crease "didn't process as an
edge" — a visible lighting seam with no line, not clickable.

**Recreated headlessly first:** a hand-built stepped wedge (the screenshot
shape), bottom-of-slope vertex nudged 0.4 units. The folds land, and the
creases come out at 9.9 and 10.2 degrees — under CreaseAngleDeg (25), so
ClassifyEdge called them smooth. DrawnEdges feeds both the overlay and the
pick pass, so a smooth edge neither draws nor picks. Diagnosis in one line:
the fold made the faces but the classifier refused the edge.

**Fix:** lineage, not thresholds. Fold pieces share their source face's
SrcFace (V-133, same convention as boolean fragments) — and an angle between
two pieces of one former face is a crease somebody made. ClassifyEdge creases
same-nonzero-SrcFace pairs at any angle past 1 degree, while different faces
keep the 25-degree rule, so the 16-gon engine pod stays a smooth cylinder
(D-06) and flush fragment seams stay invisible.

**Verified:**
- New mesh tests: the wedge recreation (fails against the old classifier with
  the exact symptom), the 16-gon contract, the coplanar-pieces contract.
- New flow test `fold_shallow`: a 0.6-unit corner nudge folds 2 faces and the
  drawn-and-pickable edge count goes 12 → 14 → 12 across fold and undo. The
  body dump line grew an `edges=` field to make that assertable.
- Goldens: `fold_shallow.png` new (crease strokes visible on a gentle bend);
  `fold_bend.png` moved by 16 pixels — read before accepting: the sliver of
  the under-25-degree crease visible past the silhouette, previously hidden.
- Full suite green; build/vet/gofmt clean.

**Next:** nothing outstanding on this report.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Bend an edge gently — barely off flat.
2. The fold now draws its crease line, however shallow the bend.
3. Hover the crease: it names itself as an edge; click it and drag — the
   crease itself is grabbable, so you can keep adjusting the bend you made.


## 2026-08-28 — Bent faces fold along real creases (V-133)

**Asked for:** "when I bend edge on left, is not generating new vertices and
edges to move edges appropriately. I would like the program to best split edges
and vertices based on nearest crease" — with screenshots of a face bent by an
edge drag, folding along an arbitrary triangulation diagonal instead of the
vertical reference line between the two existing crease vertices.

**Done:**
- `geom/mesh/fold.go` — `FoldBent(m, moved, nextID)`: splits every bent
  single-loop face into planar pieces. Crease choice: the chord between the two
  still vertices flanking the single contiguous run of moved ones (the user's
  reference line); otherwise the diagonal leaving the flattest pair, recursing
  to triangles. Pieces get fresh ids, SrcFace lineage and the shared FacePaint
  pointer — the boolean-fragment convention. Holed faces decline and stay on
  the old bent-warning path.
- `model` — `vertEdit.FoldBent` flag on MoveVerts/RotateVerts; folding
  snapshots the face list and the body's FaceSeq so undo is exact and redo
  cannot mint colliding ids. Drag frames never fold; only the commit does.
- `app` — `foldBentOnCommit` swaps the drag's final command for the folding
  version through `UpdateDrag`, keeping the gesture one undo entry. Toast:
  "Folded N bent faces along the crease". Headless `move`/`rotate` ops commit
  through the same path, so scripts get folding for free.

**Also fixed, found while wiring the tests:** the body dump line had lost its
`valid=` field somewhere in the SK sessions — `TestEveryBodyIsAlwaysAValidSolid`
matched nothing and **silently skipped every script**. The whole-corpus
validity net was dead. Restored, plus a new `faces=` field (a quad folded into
two triangles draws the same triangles — only the face count can tell). 53
scripts are genuinely checked again; 5 dump no bodies and skip honestly.

**Verified:**
- build/vet/gofmt clean; full suite green including the resurrected invariant
  over all 58 scripts. No existing golden moved — no script in the corpus bends
  a face on commit today, so there was no baseline churn to review.
- New: 8 mesh fold tests (seam-box scenario from the screenshots, corner pull
  to triangles, planar no-op, paint/lineage inheritance, holed skip,
  determinism, fixture validity), 4 model tests (fold, exact undo incl.
  FaceSeq, one-undo drags, off-by-default), 2 apptest flow tests.
- **Fail-checked:** with `foldBentOnCommit` disabled the flow test fails with
  all three symptoms — 6 faces where 8 belong, no fold toast, and the old
  "faces are now bent" warning firing in its place.
- Golden `fold_bend.png` generated and read: the Wing pod's corner pulled up
  with two crease edges drawn where the folds landed, "Folded 2 bent faces
  along the crease" toasted. (2, not 3: the pod's rotation leaves one face's
  plane containing world Y, so a Y-drag cannot bend it.)

**Decisions/deviations:** V-133 in DECISIONS.md.

**Open issues:**
- Holed faces still just bend, with the old warning. Folding them means chords
  that dodge holes — deferred until it is actually wanted.
- A face-selection that folds loses its selection (pieces are new identities,
  same as boolean fragments). Rare: folding usually starts from an edge or
  vertex selection.

**Next:** nothing outstanding on this request.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Click an edge of a body (or box-select some verts) — the move gizmo appears.
2. Drag the edge sideways so its face cannot stay flat.
3. On release: the face splits along the crease nearest the bend — real edges,
   flat pieces — and the toast says "Folded N bent faces along the crease".
4. **Ctrl+Z** once: the whole thing — move, pieces, creases — comes back off.
5. Bend the edge of a face whose neighbours already have a mid-edge vertex
   (like your screenshots): the fold lands exactly on that reference line.


## 2026-08-28 — Dots you can click and drag (V-132)

**Asked for:** "can you make it so I can click on placed dots and move them as
needed with a gizmo?"

**Done:**
- `model`: `SelMarker` selection kind with a `Marker` index on `Ref`, pruned like
  every other index-based reference; `MoveMarkers` command (a set, because
  shift-clicking two dots must move both or the gizmo is lying); `MarkerLabel` so
  the tree row, the hint bar, the toast and the undo entry all call the same dot
  by the same name; `Selection.Pivot` answers with the dot itself.
- `app`: `markerAt` screen-space hit test; dots outrank everything under them in
  the click order; hover swells a dot and names it in the hint bar; the selected
  dot wears an accent ring; tree rows select as well as delete; `Del` removes a
  selected dot; `F` frames one; `H` says dots cannot be hidden instead of quietly
  doing nothing. The transform gizmo is forced move-only on dots and its Rotate
  chip carries the reason.
- `io`/headless: `select kind:"marker"`, a `marker.move` op, and a `marker` dump
  line carrying position, direction, selected, hovered **and the dot's screen
  pixel** — the same trick the push/pull arrow uses, so a scripted click aims
  where a person would instead of guessing.

**Verified:**
- `go build` / `go vet` / `gofmt -l .` all clean. Full suite green: every package
  ok, `apptest` 100 s.
- New: 8 model tests (single move, coalescing, bad index, prune, describe, pivot,
  multi-dot, all-or-nothing) and 3 apptest tests driving the real pointer path —
  hover, click, gizmo drag, release, undo.
- **The flow test was checked against the broken code.** Disabling the
  marker-first click routing makes it fail with *"clicking the dot did not select
  it — the face behind it took the click"*, which is the bug it exists to catch:
  the front dot sits exactly on the hull's +X face.
- Golden `marker_drag.png` generated and **read**: tree row highlighted, card
  titled "Front dot" with Rotate greyed, Offset reading 0/3/0, hint bar
  "Moving along screen: +0, +3, +0", dot lifted three units clear of the hull.

**Decisions/deviations:** V-132 in DECISIONS.md — screen-space hit test rather
than a fourth pick kind; dots outranking the face they are authored on (the same
rule as V-12 for sketches); move-only gizmo; `Dir` never changed by a move.

**Open issues:**
- **The gizmo does not travel with the selection during a drag.** `armTransform`
  freezes the pivot while `Dragging()`, so a dragged dot separates from its own
  handles until release. Pre-existing and shared by every selection kind, not
  something dots introduced — but it reads worse on a dot than on a body, because
  a dot is small enough to look abandoned. One line to change; it would move
  mid-drag pixels in the M6 baselines, so it is not being changed unasked.
- Dots are still not collected by a box select. The box filter chips are
  verts/edges/faces and dots are none of those.
- Three files upstream were not gofmt-clean (`io/script.go`, `model/sketch.go`,
  `scene/sketchdraw.go`) — struct-field alignment only. Formatted in this pass so
  the pre-commit gate of TESTING §5 holds again.

**Next:** nothing outstanding on this request.

**Try it (user):**
```bash
C:\Modeler\modeler.exe
```
1. Place a couple of dots from the tree (**Set front dot…**, **Add thruster dot…**).
2. Hover one in the viewport — it swells and the hint bar names it.
3. Click it: it takes an accent ring and the move gizmo appears on it.
4. Drag an arrow, or the centre handle, and watch the Offset fields count.
5. Release, then **Ctrl+Z** — one undo puts it back where it started.
6. Click a dot sitting on a hull face: you get the dot, not the face behind it.

---

## 2026-08-28 — .pxm: the format the game eats (V-131)

The big one: rename .ship to .pxm, add orientation markers to the modeler,
and teach Iron Drift (the user's Rust game) to consume the result. Read the
game first — its gpu.rs is a museum of hand-measured orientation facts
(`model_nose_negative_z`, the THR_* thruster tables, SHIP_NOSE), which told
me exactly what the markers must carry.

**Modeler:** front/top/thruster dots placed by arm-then-pick from a new tree
Markers section, drawn as coloured dots with exhaust ticks; singleton
front/top with move-not-multiply commands; every save embeds game/ship.glb
(same builder as the glTF export) and game/markers.json (dots + derived
orthonormal basis + extents + per-thruster position/direction/radius); the
whole zip is STORED now; .ship still opens everywhere. New ops
marker.front/top/thruster/clear; golden `pxm_markers`; pxm_test.go pins
payload-present, stored-only, basis math, round-trip, legacy open.

**Game:** src/pxm.rs — a dependency-free stored-only zip reader (~100 lines,
the reason the zip went STORED), serde markers, glb staged to a temp file for
raylib. gpu.rs scans assets/models/PXM at load; draw_model_basis_ori composes
the authored basis (a .pxm can never be on the backwards-noses list);
pxm_thrusters/pxm_nose expose anchors in the THR_* convention. The sample
ship, with markers, is generated INTO the game repo and cargo's pxm tests
parse it — reader tested against the writer's real bytes. 95 game tests
green, 16 modeler packages green.

Goldens: all regenerated once (the tree grew a Markers section in every shot
with bodies; verified the diff sat in the tree region before updating).

---

## 2026-08-28 — The paint audit: corners, and a wedged bus

"Work on the painting side, needs polish, find bugs and fix." Two instruments,
two harvests (V-130).

**The screenshot found the geometry.** Baked a 2 px edge outline onto the test
scene and zoomed in: bands one texel off their edge on min sides, one texel
thin on max sides, and a bare notch at every corner. Root cause was a dab
pullback of (size-1)/2 where centring needs size/2. The fix took three renders
to land clean — the first attempt extended band ends everywhere and traded the
notches for nubs poking past T-junction corners inside faces, so the extension
now applies only to ends that reach the face's border, where the clip owns the
overshoot. TestEdgeBandsMeetAtTheFaceCorners failed on the old code and pins
the new; the edge golden regenerated (97.6% of pixels unchanged — the bands
are the diff).

**The read found the state machine.** Switching paint tools mid-stroke left
the stroke's drag open on the bus — and the edge tool never reaches the stroke
code, so it stayed open: undo dead, redo dead, the bake erroring, all silent.
setPaintTool closes a live stroke first, from every switch path. Plus: eraser
strokes no longer stamp the foreground colour into the recents (gradients now
record both ends), the hovered edge's glow clears when the pointer leaves for
the panel, the lock pick's promised click beats edge picking, and the
resample/face-view prompts stay down under the edge tool. GPU-free regression
tests in paint_test.go.

---

## 2026-08-27 (fix) — The scale contract: planes, grid, camera and zoom agree now

"Can you fix the scale of pixels, grid, models and planes, they are a
clusterfukk." Rendered four probe shots and looked: every pair disagreed
somewhere. The sketch grid ran 80 u across a 24 u plane — paper sprawling
three times past its sheet, with the plane you clicked indistinguishable from
open space. The other two planes crossed that paper edge-on as coloured bands,
labels stranded mid-grid. Plane edges (±12) missed the major lines (±8).
Zoomed to a part, the planes were translucent walls over everything. New ship
kept the last document's zoom. Pixels themselves were already right (V-128) —
it was the frames of reference around them fighting.

The contract (V-129): planes are 32 u across so their edges are 8 u majors at
every grid step; the sketch grid's extent IS the plane, drawn alone while
sketching and framed whole on entry; planes fade out as the camera zooms past
them (and stop being pickable before they are invisible); New ship goes Home.

The bug found on the way is the recordable one: PlaneDraw.Fade used zero to
mean "unset, draw opaque" — so a *fully faded* plane drew at full strength,
caught only by measuring wall pixels across builds when the eyeball said
"nothing changed". Zero means invisible now and every constructor sets the
field. Thirty goldens regenerated after eyeballing sk2_curves and m3_straight;
the sample-ship golden did not move, because paint mode already hid planes —
the contract holding on its own.

---

## 2026-08-28 (feature) — Mesh import: STL and OBJ, merged back into polygons

"Can you make it so I can import *.step files?"

STEP is two problems. The Part 21 text format is tractable; a B-rep on trimmed
NURBS surfaces is not, and OpenCASCADE — the only real answer to the second —
is a ~200 MB C++/CMake dependency against everything D-04 and D-12 decided.
Put as a choice, the answer was mesh import instead: STL and OBJ, reachable
from any STEP file via one conversion in FreeCAD.

**The assembler is the feature, not the reader.** Loading triangles would have
produced a body with a face per triangle, and a triangle is the one shape
nothing here wants — paint needs a face to allocate a picture on, push/pull
needs one to drag, sketch-on-face needs somewhere flat. `mesh.Assemble` welds
onto the subunit grid, drops degenerates, walks the surface flipping triangles
until neighbours agree which way is out, and merges coplanar triangles back
into polygons. Twelve triangles in, six rectangles out, valid solid (V-144).

The merge tolerance is tight on purpose: adjacent facets of a tessellated
cylinder are nearly coplanar, and a loose test would eat the curvature. Normals
must agree to a twentieth of a degree and every corner must stay within
`PlanarDist` of the region's plane. Merged loops then drop collinear points.

**Scale is asked rather than guessed** — mesh files carry no units. The card
defaults to fitting the longest side to 16 u and shows the result in units
before committing; the toast says how many faces came from how many triangles,
and says plainly when the result is not closed.

Two traps worth recording: a *binary* STL may begin with the word "solid", so
the encoding is decided by whether `84 + 50n` matches the file size, not by
sniffing text. And OBJ allows negative indices counting back from the end.

Reached by Ctrl+I or the toolbar's import button. Goldens regenerated — the new
icon at x=1114 tripped the staleness check this time, where the New button
last commit did not.

---

## 2026-08-28 (feature) — A New button, and a gate that offers to save

"I see open, save and export, but I don't see a new button to start a new
model, can you add that with save before creating new dialog if dirty?"

Both halves. New sits with Open, Save and Export at the right end of the
toolbar, routing to the same `fileNew` the keyboard always sent — a shortcut
with no button is a feature only manual-readers have.

The gate was the more interesting half. It offered Discard or Keep working, so
the answer most often wanted — save it, then go ahead — was something you did
yourself: dismiss, save by hand, ask again. It has three answers now, with Save
as the confirm so Enter keeps the work, Discard carrying the danger styling,
and Escape still meaning "I did not mean to ask" (V-81, V-143). The title names
what triggered it: "Save before starting a new ship?", "Save before opening?".

**Save runs first and the action follows only if it worked.** A dialog waved
away or a disk that refuses is not a save; treating it as one would make "Save"
the fastest way to lose the work. Tested by blocking the write with a plain file
standing where the directory would have to be.

Two things fell out. `SaveAs` had no headless guard — a pathless `Save()` in a
script would have opened a native dialog with nobody there; it refuses now,
which is also what lets the save-then-act step see an honest failure. And the
new icon is ~600 px in a 921,600-px shot, 0.065% against a 0.30% tolerance, so
**no golden failed on a visible UI addition**. Regenerated them anyway; a diary
showing a toolbar the program no longer has is worse than none.

Six new tests on the guard, one on the modal. The dialog itself has no golden:
`guardUnsaved` is bypassed headlessly by design, so a screenshot of it needs a
patched build. Verified that way by hand, not pinned.

---

## 2026-08-28 (fix) — Ambient occlusion, now with somewhere to land

"I really don't see ambient occlusion, do you think is possible to make it
work?" It was possible, and the report was correct in a more interesting way
than "the number is too low".

**Measured first.** On a hollowed box at full strength, 2.9% of pixels changed
and a scanline across the inner floor read `154.3 -> 124.0` at *every* pixel of
the face — a delta of exactly 30.3, flat in both directions, then zero off the
face. Not a gradient. A flat tint.

**The cause was structural.** Openness is a corner value interpolated across
the face, and a CAD face is a big flat polygon whose only corners are its
outline. All four corners of that floor are equally occluded, and interpolating
four equal numbers gives a constant. The body had 10 faces, 32 triangles and no
interior vertices anywhere — there was physically nowhere for a corner falloff
to exist (V-142).

**The fix** is a shading tessellation in the render mesh: each face cut on a
barycentric grid so shading has interior samples. Only faces with something in
front of them are cut, which on a convex body is none of them, so a plain box
costs what it always did. The bake was rebuilt to match — per-face candidate
lists instead of a full triangle scan per corner, and the corner loop spread
across cores, byte-identical either way. Sample ship went 33s naive, then 8.9s
with the convex skip, then 1.6s parallel, against 0.75s before.

**Two tunings, same complaint.** Reach is `0.35 x the body's diagonal` clamped
to [1.5, 12] — a fixed 2.5 u put a three-unit cavity's ceiling out of range of
its own floor. Default strength 0.5 -> 0.7.

The ao_step golden tells the story: the old ledge is one flat tone, the new one
darkens into the step wall with the wall darkening toward its base. 36 baselines
regenerated after review; no behavioural test moved.

---

## 2026-08-27 (fix) — A pixel is now the same size everywhere

"So 1 u = 1 pixel correct?" No — and finding out why was worth the whole day.
`Texel` was the face's longest side over the chip, so on the test box one pixel
was 0.375 u on the 12-unit faces and 0.250 on the 8-unit ones. The same chip
meant a different physical pixel on every face. That is the root of the edge
line being thicker on one side of a corner than the other; the edge tool was
only where it first became visible.

**The chip is a density now** — 1, 2, 4, 8, 16 or 32 texels to the unit, and
`Texel = 1/Res` with the face's size given no say (V-128). One pixel is one
pixel anywhere in the document.

**What it cost.** The picture grows with the face now, so an allocation can
exceed the 1024 cap; it is refused and names a chip that fits, rather than
clamped to a picture too small to cover its face. Old ships are safe — the
format stores each face's `Texel`, so a pre-V-128 file keeps the pixel size it
was painted at, and the program reports density as `1/Texel` rather than
trusting the now-stale `Res` field.

**Goldens.** Every paint fixture addresses texels by index, so each stroke was
scaled by `faceLongestSide x newChip / oldChip` and the baselines regenerated
after review. The sample ship came back within 99.9% of its old render, which
is the evidence the conversion was arithmetic and not guesswork. Two brush
sizes moved to keep a stroke's *world* thickness (a size-3 brush does not
exist; m7_persist went to 4 because a pixel probe rides on that line).

The visible proof is the edge_paint corner: the old baseline has a fat top band
meeting a thin side band in a stepped notch, and the new one has one continuous
trim line.

---

## 2026-08-27 (fix) — The edge line's width control

"Why is my edge line crappy compared to yours? My edge paint is way thick even
at 1."

Measured before changing anything: at size 1 the band is exactly one texel, so
there was no width bug. What there was: a texel is a face's longest side over
its resolution, so on the test box one pixel is **0.375 units on four faces and
0.250 on two** — and on a bigger model every pixel is bigger. One pixel is the
thinnest line a face can draw, and if that is too thick the answer is the
face's resolution.

**Two changes.** The control is a slider now, 1–16 pixels, which is what was
asked for and the right shape for a value whose useful setting depends on the
model. And the program says what the number means: the panel shows the live
conversion beside the slider — "3 px = 0.75–1.12 u", a range when the faces
differ — and the toast repeats it after the bake (V-126).

**A road not taken.** I started converting the width to world units so both
halves of an edge line would match physically, and stopped: this is a pixel-art
tool, every brush stroke already works in the face's own texels, and "how many
pixels" is the question someone drawing panel seams is asking. The mismatch is
inherent to per-face texel grids, and the honest answer is to show it rather
than to hide it behind a unit conversion.

Also: the brush's Size row is hidden while the edge tool is armed — it has its
own width and the brush square does nothing for it (V-127).

---

## 2026-08-27 (feature) — Edge lines

"I want a way to paint on edges that I like to select and bake it to the model
… choose how thick and with a press of a button it becomes the painted edge to
the model itself."

**Shipped.** A new paint tool (`K`): click edges to pick them — they highlight
in the colour they are about to become — set a width in texels, press Paint. A
band is baked along each edge onto *both* faces that meet there, as one undo
step however many edges it covered. **All corners** takes every sharp edge of a
body at once, because clicking forty of them to outline a hull is a chore
rather than a tool (V-124).

**Baked means baked** (V-122). The band goes into the same per-face pictures
the brush writes into, so from the moment it lands it is ordinary paint: it
saves in the .ship, exports to glTF and OBJ, survives a boolean the way the
rest of the paint does, and can be painted over. The alternative — a list of
decorated edges the renderer draws separately — would have been a second kind
of colour on the model, with its own place in the file, its own export path,
and its own answer to what happens when the edge is cut in half.

**The one piece of real geometry** is where the band sits. An edge is the
boundary between two faces, so a brush centred on it spends half its width on
texels belonging to neither — invisible, and half the thickness asked for. Each
face's band is offset inward by half the width and every dab is clipped to the
face's own rectangle, so the two halves meet at the corner and read as one line
(V-123). That is the whole reason the feature exists: a seam painted face by
face never lines up.

**How little was needed.** An edge already knows which faces use it (Topo), a
face's paint already knows where a world position lands in its texels (Texel),
and Stroke already draws a line in texel space. The new work was the inward
offset, the clip, and a command that snapshots several faces at once.

Found while looking at the user's zoomed screenshot of a painted crate — the
pale line they pointed at along the corner turned out to be exactly the thing
they were about to ask for.

---

## 2026-08-27 (fix) — Every pocket has been invisible since M4

Chasing "subtract in extruding on a sketch on a model is not working" a second
time led somewhere much worse than the extrude tool.

The subtract *had* worked. A pocket 12 x 4 x 3 was cut into a face, 144 units
of material were gone, the volume said so — and the face rendered as though
nothing had happened. Cutting a face leaves it pierced: an outer boundary and a
hole. `appendEarTris` bridges the hole in and ear-clips, which is the right
algorithm; but no ear was ever clipped, so it fell through to its own "fan
whatever remains" fallback and drew a lid across the opening (V-121).

The cause is what a bridge is. Splicing a hole into an outer loop leaves two
pairs of vertices at identical positions. The ear test rejected any other
vertex inside the candidate triangle, and pointInTri counts the boundary as
inside — so a duplicate always sat exactly on a corner of any ear using its
twin, every ear was refused, and the fan ran. The fix skips vertices coincident
with the ear's own corners.

**Why it survived seven milestones.** The damage was only in the drawing.
Solver, volumes, exports and saved files were right the whole time, and every
acceptance number in the repository agreed with the geometry. Seven goldens —
m4_cut, m4_applied, m4_union, m5_stepped, m5_pullpush, m9_throughcut,
sk3_shapes — have shown windows as flat outlines on unbroken walls since M4,
and they were the baselines. The only thing that could catch it was looking at
a picture and asking why a hole was not a hole.

**How it was found.** Reproduced the user's shot exactly (a chamfered box, a
face sketch, a subtract), confirmed the volume changed and the render did not,
then walked down: the mesh has the pocket floor and all four walls; the pierced
face triangulates to 240 of area where 96 is right; spliceHole is correct at
96; earClip returns a pure fan from vertex 0 with several backwards triangles.

The tests measure *unsigned* triangle area, because a fan's signed areas cancel
to the correct total while overlapping — and check that every triangle winds
with its face, which is what a fan cannot do.

---

## 2026-08-27 (fix) — Subtract on a face sketch

"Subtract in extruding on a sketch on a model is not working." Two faults, one
symptom.

**Why it failed every time.** A sketch on a face opens with Result=Add and the
arrow pointing outward along the face normal — right for adding. Clicking
Subtract left it pointing the same way, so the solid sat against the *outside*
of the body. The boolean ran, correctly, and removed nothing (V-119). Picking
Subtract now turns the solid round, the way turning Through all on picks
Symmetric when the plane sits inside the model (V-76). An explicit flip
afterwards is still the user's.

**Why it failed silently.** `bodiesReachedBy` offers a target on bounding-box
overlap, and its own comment allowed that this can mean "a chip being offered
that turns out to do nothing". When that happens the extrude succeeds, records
history, and leaves the model identical — which without a word is
indistinguishable from a broken tool or a click that missed. The commit now
measures its targets before and after and says so, with Undo on the toast
(V-120). The loose box test stays: real CSG per frame to grey out a chip would
cost more than the extrude itself.

**Reproduced before fixing.** A face sketch, rect 2×1, Subtract at depth 3:
348.0000 → 348.0000, no toast, 28 triangles becoming 26 — the boolean had run
and re-tessellated the face without cutting anything. After: 348 → 342, exactly
the 6 units the profile and depth describe.

The "removes nothing" test needed a case the first fix does not cure, so it
uses a genuine geometric miss instead: the hull is an L, and a cut placed in
the air beside its raised block is inside the bounding box and outside the
material. Boxes overlap, geometry does not, nothing is removed — legitimately,
and now audibly.

---

## 2026-08-27 (fix) — The flyout was clicking through itself

"Dropdown menu is buggy, is also clicking behind the drop down menu." It was,
and the interesting part is which half was broken.

The **viewport** half was already covered: `CardCapturesPointer` reads
`lastCards`, which has the menu in it one frame after it is drawn, and a user
cannot click something before it appears. The **tree** half was not covered at
all. `buildShell` draws the toolbar first and the tree second, so the tree's
rows hit-tested a pointer the menu had already taken — and the menu's rows sit
exactly over the plane rows. Clicking "Centre rectangle" picked the tool *and*
selected the Top plane behind it (V-117).

`lastCards` cannot fix that: it is read during update, before anything is
drawn, so it knows nothing about draw order within a frame. The fix is a
same-frame claim — `Context.ClaimPointer(rect)`, consulted by `hovering`, taken
by the menu *after* it hit-tests its own rows so it does not block itself, and
cleared every frame so a closed menu stops swallowing anything at once. The app
also derives the open flyout's rectangle during update from the same layout the
toolbar draws from, which covers the one frame a menu first appears and keeps
the hit test and the drawing from ever disagreeing about where it is.

**Proved both ways.** `sk5_flyout` arms the Point tool — which commits on every
click, so a leak into the viewport would show — opens the Rectangle group and
clicks the row sitting over the Top plane. With the fix: the tool changes,
nothing is selected, nothing is drawn. With the claim removed: `sel count=1
desc="Top plane"`.

**Found on the way.** `parseSketchTool` still named only M2's four tools, so no
script could arm a point, an arc or a slot by name. It now derives the names
from `AllTools` (V-118).

---

## 2026-08-27 (SK5) — The modify tools

The milestone the plan called the multiplier: fillet, chamfer, offset, mirror
and both patterns, each a `ReplaceEntities` over a selection so a whole
gesture is one undo step (V-98, written in SK1 for exactly this).

**Shipped.** A Modify section on the sketch card, appearing when the Select
tool has a selection: Fillet and Chamfer sharing a radius, Offset with a
distance, Mirror about the sketch's vertical or horizontal axis, and Linear or
Circular patterns with a count and either a step or a sweep. Buttons that do
not apply are disabled with the reason rather than hidden, so the section's
rows do not move under the pointer as the selection changes. Six script ops,
each calling the same function the button does.

**The acceptance.** One porthole, patterned four across, mirrored to eight,
cut through an 18×10×2 plate: **360 → 332.5**, against 27.55 of hole computed
from the polygon formula. Eight identical holes is a number both tools have to
earn — a pattern that dropped one or a mirror that resized what it reflected
moves it.

**The bug this milestone found in itself.** The script reported *nine* regions
where eight circles had been drawn. The ninth was the fillet: its winding came
from the cross product of the two legs, which picks the 270° sweep for half of
the four corner orientations. A 270° arc crosses its own legs, and the region
engine then correctly finds a closed loop — so a plain L, which has no inside,
had become an extrudable shape (V-113). The fix is to take the sweep between
the tangent points and pick the short way; the test walks all four
orientations plus the one that caught it. It is the same class of bug as M9's
through-all: geometry that looks plausible and is quietly wrong, caught only
by a number.

**Two of the six deferred, and named.** Transform (move/rotate/scale of a
selection) and general Use/Project. Transform's value is in dragging, which
needs V-111's missing §8.5 machinery; Use/Project needs face picking from
inside sketch mode, and the face-sketch case it exists for has had
`ProjectFaceOutline` on the card since M5 (V-116).

**Next:** SK6 is the image underlay, and the plan says it needs the user's
go-ahead before starting.

---

## 2026-08-27 (SK4) — Splines and beziers

**Measured first.** §6 named the region engine as this milestone's risk and
said to benchmark before building. `BenchmarkBuildLoop`: 1024 short segments
in a single closed loop build in **0.57 ms**, scaling about n^1.4 rather than
n². A realistic spline is ~96 segments — 0.045 ms, once per edit. The risk
does not materialize (V-109); the benchmark stays.

**Shipped.** `EntSpline` (Catmull-Rom through the clicked points) and
`EntBezier` (one cubic, handles previewed as a cage), a Spline group on `S`,
and the double-click that ends an open run.

**The acceptance.** A closed spline hull and a bezier canopy, extruded 3 deep.
The test does not record the volume — it samples both curves from their
textbook definitions two hundred times more finely than the program draws
them, and asserts the program's coarser tessellation comes in slightly under
that and never over. A spline that interpolated the wrong control points, or a
bezier that read its handles in the wrong order, would still close into
something plausible; only a number from the definition catches it.

**A seven-milestone-old broken promise.** SPEC-UX §8.3 has said "double-click
to finish" for the line chain since M2. The hint bar says it. `FinishChain`
was written for it. Nothing ever called it (V-110). The spline needed the same
gesture, so it is wired now — for both.

**Deferred, and honestly.** SK4's third item was dragging a spline's control
points, "extending the existing endpoint-drag machinery in §8.5". That
machinery does not exist: `MoveEntities` is a command nothing calls, and
sketch mode has no drag path at all. Building it is a feature in its own
right — and one that would give *every* entity the dragging §8.5 has promised
since M2, not just curves. Left as its own piece of work rather than
half-built here (V-111).

**Next:** SK5 — the modify tools. It is the largest milestone, and every one
of its tools is a `ReplaceEntities` production over a selection.

---

## 2026-08-27 (SK3) — Polygons and slots

The smallest milestone, and the one with the most direct payoff: `EntPolygon`
and `EntSlot`, two toolbar groups (`P` and `O`), a Sides row on the card that
appears only when it applies, and the shape a slot exists for.

**The acceptance.** A slot, a hexagon and a circumscribed octagon cut clean
through a 16×8×2 plate: **256 → 172.6577**, against 172.66 computed from the
polygon-area formulas for each shape. The octagon is the one that matters —
circumscribed normalization could be wrong and still produce an octagon, just
the wrong size, and only a number catches that (V-105).

**Two design calls.** A circumscribed polygon is stored as its inscribed
equivalent, so there is one kind and no variant flag anywhere downstream. A
slot's width is measured perpendicular to its axis, so sliding the third click
along the track does not fatten it (V-106).

**One thing the script taught me.** The first draft put the hexagon's top edge
exactly on the slot's bottom edge, and the extrude refused — correctly:
touching regions have been refused since M3, and M3 has a golden for it. The
script was wrong, not the program. Moved them apart.

The sketch toolbar now carries eight groups and still fits its labels at the
1280 px minimum; below that `sketchToolbarFitsLabels` drops to icons, measured
rather than guessed.

**Next:** SK4 — splines and beziers.

---

## 2026-08-27 (SK2) — Arcs, ellipses and a circle through three points

**Shipped.** `EntArc` and `EntEllipse`, the 3-point circle (no new kind — it
is a fitted `EntCircle`), and five new gestures: 3-point circle, centre arc,
3-point arc, tangent arc, ellipse. The Arc group joins the toolbar on `A`;
the card row became "Curve segments" because one number now decides how round
every round thing is (V-102).

**The contract that matters.** An arc's tessellation begins and ends *exactly*
on the points it was built from (V-99). Everything else in SK2 is downstream
of that: the acceptance script draws a D-shape from three lines closed by an
arc, and it becomes a region with **zero open ends**. One subunit of drift at
either end and it would be four open ends and no region — silently, visible
only when an extrude refuses.

**The acceptance number.** Three curved profiles pulled 2 units deep:
**151.4946**, against 151.4944 computed from the regular-polygon area formula
for each (a 6×3 rectangle plus half a 32-gon of radius 3; a 32-gon of radius
2; a 32-gon ellipse with semi-axes 5 and 2). The test computes those rather
than recording them, so a tessellation that changed density or dropped a
segment fails instead of passing on a stale baseline.

**Design calls.** Three gestures build one entity, with the construction
arithmetic in `curves.go` as pure functions — a circumcentre a few subunits
out draws something plausible and puts every later join wrong (V-100). The
tangent arc demands a loose endpoint and refuses without one rather than
guessing a direction (V-101), and learns what is drawn through `SetContext`
fed per frame, so an undo cannot strand it. A refused gesture resets, matching
the circle and rectangle since M2 rather than inventing a kinder rule for
half the toolbar (V-103).

**Deferred:** elliptical arc and conic, the two stretch goals — the shapes
from the Onshape toolbar with no use a low-poly hull cannot already meet
(V-104). The enum order reserves them.

**Next:** SK3 — polygons and slots.

---

## 2026-08-27 (SK1) — Foundations: groups, guides, points, variants

First milestone of Sketch_func.md. The plan called it "the milestone that
makes the rest mechanical", and it was: almost everything landed at the seams
the plan named, in the order it named them.

**Shipped.** Tool groups in the sketch toolbar — one button per group showing
the variant last used, a chevron opening the rest, and the group's key cycling
its members (V-93; two new widgets, `ChevronButton` and `Menu`, logged against
the frozen widget set). Construction geometry: `Q` converts a selection or
arms the mode, guides draw dashed and dimmed, and they are skipped at exactly
one seam — `Sketch.Segments()` — so the region engine never sees them while
drawing, snapping, picking and saving are untouched (V-94). `EntPoint`, the
entity that makes no segments (V-96). Midpoint line and centre rectangle,
both session-only arithmetic. Aligned rectangle: three clicks, four lines,
committed as one undo step (V-95), with Esc stepping back a point at a time
(V-97). And `ReplaceEntities`, the atomic swap every SK5 modify tool will be
(V-98) — the aligned rectangle is its first user.

**The acceptance number.** `sk1_tools` draws all six things and extrudes every
region 2 units deep: **68.0000** exactly. That is 12 (centre rectangle) + 10
(aligned rectangle — √20 × √5, and the perpendicular offset lands exactly on
the lattice, so it is not 9.98) + 12 (ordinary rectangle), doubled. The
construction rectangle is 80 more units of area; if guides ever start closing
regions the number becomes 228 and the test says which mistake was made.

**Two things the plan did not predict.**

*`Entity` stopped being comparable.* Adding `Pts []geom.Vec2i` for SK4's
splines made `==` a compile error. No production code compared whole entities,
only a new test did, so the fix was an explicit `Equal` method — which SK5's
modify tools want anyway, since "did this entity change" is their whole
question.

*The dump line I extended had two parsers, not one.* §1.2's trap says one
parser per line; I grepped for `sketch id=` and found one. There is a second,
richer line — `sketch active=` — with its own parser in m2, already carrying
entities/regions/openends. That was the right place for the construction
count, and it is now there. The trap holds; my grep was too narrow.

Five sketch goldens changed for the regrouped toolbar (chevrons on Line and
Rectangle, plus Point and the construction toggle), eyeballed as a crop
before regenerating.

**Next:** SK2 — 3-point circle, `EntArc` with three gestures, `EntEllipse`.

---

## 2026-08-27 (planning) — Sketch_func.md: the full sketch toolset, planned

The user brought Onshape's sketch toolbar — line/midpoint, three rectangles,
circles/ellipse, four arcs and a conic, polygons, splines, point, project,
fillet/chamfer, offset/slot, mirror, construction geometry, patterns,
transform, image underlay — and asked for a robust plan another executor can
follow. Written as **Sketch_func.md**: milestones SK1–SK6 on the one seam
everything already funnels through (Entity → Points() → Segments →
Arrangement — every curve is a tessellation, exactly as circles are n-gons),
with a per-kind checklist of the eight places a new entity touches, storage
decided per kind, gestures and Esc-staging specified, ops named, tests named,
the traps recorded (one parser per dump line, knownOps registration, frozen
widget kit), and a risk register. Non-goal stated loudly: no constraint
solver — the pixel grid is this program's solver. PLAN.md §M10 now points at
it. No code in this session; the plan is the deliverable.

---

## 2026-08-27 (later again) — The line that left its ghosts behind

"When I'm about to place a line and I drag one end up and down, it leaves
behind artifacts, same with circle." The two-point tools rubber-band by drag
replace: every frame undoes the previous shape and draws the new one, and the
document was perfect throughout — the bus just announced only the new
command's dirty rect, so the texture cache never re-uploaded the region the
undo had erased. The old line stayed on the GPU wherever the new line's box
missed it (V-92). Freehand strokes append and never shrink, which is why M7
never saw it; the gradient escaped because its rect is the whole face.

UpdateDrag now emits the previous command's events too. The new test builds a
texel mirror out of the event stream — the same thing the renderer is — and
demands it match the document texel for texel; it failed with 10 stale texels
on the old code, which is the vertical line minus the texel the two lines
share.

---

## 2026-08-27 (later still) — The welcome card answered its own button with itself

"I click new ship and message wont go away." It could not go away: the card
shows for an empty, clean, unnamed document, and New ship produces exactly
that, so the show condition was true again the same frame (V-91). The card
now carries a dismissed flag set by any answer — a button, Escape, or a click
that starts work in the viewport — and welcome_test.go pins the state machine.
The hole is older than it looks: until V-84 the app never started empty, so
nobody had ever actually clicked this button.

---

## 2026-08-27 (later) — Grid steps, the cube's clicks, and the look

Three user asks in one sitting.

**"Add a way to change the grid size" (V-89).** `Settings.GridStep` had
existed since M2: the snap read it, nothing ever set it, and the drawn grid
ignored it entirely at a hardcoded unit — a setting wired to one consumer and
no producer. Now a Grid chip row on the sketch card (0.25 / 0.5 / 1 / 2 u)
sets it, and the drawn grid and the snap both read the one value. Major lines
follow every eighth minor rather than every eight units, so the rhythm holds
at every step and the default grid stays pixel-identical. New `sketch.grid`
op; the `m9_grid` golden renders 0.5 u and 2 u and asserts the two shots
differ — chips that changed nothing on screen would fail it. Snap-side unit
tests in `grid_test.go` (including the Ctrl override and the zero-step
fallback).

**"Clicking the cube also triggers sketching" (V-90).** True, and not only
sketching: updateSketch, updateBoolean, both arrow gizmos and the transform
gizmo all consumed presses by geometry alone, and the cube floats inside the
viewport. Idle's handleViewportClick has checked the cube since M1; the tools
that came after each needed to learn it. It has a name now — cubeOwnsPointer —
and all five ask it.

**"Make the UI cooler, looks crappy and low quality" (V-88).** The honest
diagnosis: no depth anywhere. Background, panel and card sat within a few
points of value of each other; nothing cast a shadow; the viewport gradient
ran dark-side-up. The pass, all in the kit so every widget inherits it:
deeper, wider-spaced value ladder; a layered soft shadow with falloff under
every floating surface (three iterations to get here — equal-strength rings
read first as grey halos between stacked toasts, then as sticker outlines;
the fixes were a shadows-before-bodies pass for the toast stack and a
weighted falloff); a one-pixel top bevel on raised surfaces and filled
buttons; the viewport gradient flipped light-side-up and widened; an accent
wash on the active tool button. Reviewed by rendering the sample ship, sketch,
paint and transform shots before regenerating — every golden changed, as a
whole-frame gradient must. README hero updated as a new diary shot
(docs/shots/v1_hero.png); the old m9_sample.png stays, historical as always.

**Also:** `bareApp` in the app tests grew Settings; scene's grid test covers
the step scaling and the zero fallback.

---

## 2026-08-27 — The audit: why it looked broken

The user came back from trying v1.0.0 with "a lot of things aren't working, do
an audit." Every finding below came from reading the interactive layer — the
one place the headless suite cannot see, because scripts drive the app below
the input layer. That is also why all of it was green while being wrong.

**The flagship: every launch loaded the M1 debug scene.** `Run()` still called
`LoadTestScene()`, so the program opened on a hull, an engine pod and a wing
pod — and with a non-empty document `showWelcome()` could never be true. The
welcome card, New, Open, the recents, the Sample ship button: all of M8/M9's
entry experience, unreachable since the day it was written. The comment in
testscene.go even said M9 replaces it; nothing did. The interactive app now
starts empty (V-84); headless keeps the scene — the golden scripts are written
against it.

**Dead controls that claimed otherwise.** The Move button was disabled behind
"Move arrives with milestone M6", three milestones after M6 shipped, and the M
key — listed in the shortcut sheet — was bound to nothing. P entered paint from
nowhere: `handleKeys` had no case for it, so the key only worked for leaving.
Wired (V-85); Move lights whenever a gizmo is armed and says what to select
when none is.

**Two ways to lose work without being asked.** Ctrl+N, Open, a recent, the
sample: all replaced a dirty document silently — only the window's close asked
(V-80). They now stop at "Discard unsaved changes?" (V-83). Worse: Escape on
the close prompt itself was read as the cancel button, and in paint mode the
same keypress also fell through to the mode underneath — the reflex key closed
the program without saving. Escape is now "dismissed", its own outcome that
keeps working (V-81), and a modal owns the whole keyboard while it is up
(V-82). `guard_test.go` pins all of it, GPU-free.

**Feel bugs.** An orbit froze the moment the pointer crossed the toolbar or
tree and resumed on the way back (V-86 — drags are decided at press, not
re-litigated every pixel). Esc did not cancel the armed "click a plane" state
its own hint bar promised to cancel. Cancelling an extrude kept the camera
tilt the tool had added, despite a comment claiming otherwise. The `?` sheet
said "Esc to close" while Esc, in paint mode, exited paint behind it.

**Smaller keeps (V-87).** A recovered autosave planted its hidden-folder path
in the recents and LastDir. Closing the colour picker mid-scrub left a drag on
the bus that refused every later command. The autosave zipped the document
mid-frame (now after EndDrawing). The window rect was saved on every exit and
read by nobody (now restored, position only if still on a monitor). Dropping a
.ship said "opening projects arrives with M8" in a build whose title bar says
v1.0.0 — it opens now, through the same unsaved-work guard as Ctrl+O.

**Goldens.** Twelve stale, one cause, verified by cropping the diff region
before regenerating: the Move button in the toolbar goes from greyed-out to
accent-lit-with-underline in every shot whose script leaves a body or vertices
selected (worst delta 215 at (228,34) — the underline row; 99.96% of pixels
unchanged). m5's push/pull shots went stale under the first draft of `canMove`
(any selection) and came back on their own once it asked "is a gizmo armed"
instead — a face selection arms the push/pull arrow, not the gizmo, and the
button now says so. Regenerated with GOLDEN_UPDATE=1 after the visual check.

**Not fixed, recorded:** the tree panel's width is saved in settings but there
is still no way to drag-resize it; sketch-toolbar "Finish" and "Close" are the
same action twice (changing it moves every sketch golden for cosmetics); hover
picking still waits for pointer motion, so orbiting under a stationary cursor
can leave a stale highlight for a frame or two.

---

## 2026-08-26 — M9: the polish pass, and the bug that came with it

**The bug first, because it was the ask.** "The subtract function when extruding
a sketch is buggy, not working." It was, and the way it was wrong is worth
recording: Through all measured its reach as the distance to the furthest corner
of the scene, then spent all of it in one direction. The three default planes all
pass through the origin — and so through the middle of most ships — so a cut from
one of them *started inside the material* and stopped past the far side. It took
exactly half the hole and left a blind pocket.

It survived M3 because the only Through-all test set `dir: "symmetric"`
explicitly, which is the one case that already worked. The new test uses the
default direction and measures the hole: **24 units of hull, not 12**. It fails
on the old code with exactly the number you would have seen.

Turning Through all on now picks Symmetric when the scene straddles the plane,
and the toggle says which it resolved to — "both ways" or "one way only" —
because a user who asked for a hole and got a pocket has no other way to tell
why.

**Then the polish.**

| | |
|---|---|
| **Sample ship** | `assets/sample_ship.json`, embedded, offered on the welcome card. Hull, two swept wings, two engine cylinders, a cockpit sketched on the hull's own face, plating and a dithered intake glow — 176 triangles, one body |
| **Cursors** | `internal/app/cursor.go` — crosshair to draw and paint, resize on an arrow, pointing hand on the cube and while choosing a face, not-allowed off a paintable face |
| **Keyboard** | the `?` sheet gained the file and paint maps it was missing |
| **Closing** | unsaved work is asked about rather than left to the autosave |
| **README** | quickstart, the whole keyboard map, why the exports look right, and the credits with licences |
| **v1.0.0** | in the window title and the hint bar |

**Two things I decided not to build, with the numbers.**

1. *The async boolean.* SPEC-RENDER §8 wants a goroutine and a spinner past
   120 ms. `BenchmarkBooleanOnAShip` cuts a window through a 1282-triangle hull
   — the sample ship's scale — in **4.7 ms**. Twenty-five times under. Building
   the machinery would be complexity with no cause; the benchmark stays so the
   day someone builds a ship ten times heavier, the number says so.
2. *The about card.* UX §15 asks for the version in a title bar and an about
   card. The gear it would have lived behind became the export button in M8, and
   inventing a menu for one line of text is worse than not having it. The version
   is in two places you already look.

**Verified.**

- `go build ./...`, `go vet ./...`, `go test ./...` — green, goldens twice.
- **Full-app end to end**: `TestTheSampleShipBuildsEndToEnd` drives every tool
  in order and checks the result is *one* body — two would mean a union quietly
  fell back to New — with the volume in range, all six sketches consumed and
  hidden, and three faces painted.
- **Perf**: the sample ship renders at **0.93 ms mean at 720p** and **1.61 ms
  mean / 2.53 ms p99 at 1080p**, against a 16.6 ms budget.
- Every golden regenerated once, for the version string in the hint-bar corner:
  247 pixels at x 1222–1263, y 702–709, measured before regenerating.

**What is left, and it is yours.** PLAN's accept clause for M9 is that *you* run
the UX §15 feel checklist and sign off. The parts I can verify are verified; the
parts that are about how it feels are not something I can do from here:

1. Every control's hover, pressed and disabled states — the kit has had them
   since M1 and every disabled control carries its reason, but nobody has looked
   at all of them in one pass.
2. Camera and UI animation: 220 ms cubic everywhere, interruptible, no snaps.
3. Resize and DPI at 1.25, 1.5 and 2.0 scales.
4. Whether a first-timer can build the sample ship from the hint bar alone.
5. **Whether the OBJ and glTF open properly in Blender or your engine** — the
   one part of M8's accept clause I could not close either.

**Try it** (`go run ./cmd/modeler`):

1. **Sample ship** on the welcome card. Take it apart — every sketch that built
   it is still in the tree.
2. Sketch a rectangle on the Front plane, `E`, **Subtract**, **Through all** —
   the toggle now says "both ways" and you get a hole, not a pocket.
3. **Ctrl+E**, pick **glb**, and open the result in a glTF viewer.
4. Close the window with unsaved work and see what it asks.

**Next:** M10 is post-v1 backlog and PLAN says not to start it without you.
Mirror symmetry is top of that list.

---

## 2026-08-26 — M8: files, autosave, recovery and export

The file layer, built bottom-up: everything in `internal/io` first, with its own
tests, then wired into the app. That order was not tidiness — this is the layer
that loses people's work when it is wrong, and the parts that lose it are the
parts with no UI.

**What shipped.**

| Piece | Where |
|---|---|
| `.ship` container: zip of manifest, document, one PNG per picture, thumbnail | `internal/io/ship.go` |
| Mesh and vector serialisation | `internal/geom/mesh/serial.go`, `internal/geom/json.go` |
| Autosave, crash-save, recovery discovery | `internal/io/recover.go` |
| Exports: **glTF (.glb and .gltf)**, OBJ+MTL+PNGs, binary STL, PNG at 1×/2×/4× | `internal/io/gltf.go`, `internal/io/export.go` |
| Native dialogs (`ncruces/zenity`, D-10) | `internal/io/dialogs.go` |
| App wiring: New/Open/Save/Save As, autosave timer, crash handler, recents | `internal/app/files.go` |
| Welcome and recovery cards (SPEC-UX §14) | `internal/app/welcome.go` |
| Export options card | `internal/app/exportcard.go` |
| Off-screen capture for thumbnails and PNG export | `internal/render/capture.go` |

**glTF was the user's, mid-milestone.** It was M10 backlog item 4; they asked
during M8 and it belongs with the rest of the export work. Both spellings are
written. Every sampler says NEAREST — glTF is the only format here that can put
that instruction *in the file* rather than in a README, which for a pixel-art
tool is most of the point of supporting it at all.

**Three things that had to be got right and were not obvious.**

1. *The mesh has to serialise itself.* Fragments of a cut face share one
   `FacePaint` by pointer. A face-at-a-time marshaller writes that picture once
   per fragment and reads back a copy each — and from then on painting one
   fragment stops showing on its siblings. The contract would have been broken
   by the act of saving. The pictures go in a table and the faces index into it.
2. *Recovery files are cleared by process, not by name.* An autosave is named
   after the document it came from, and **a save is exactly the moment that name
   changes**. Clearing by the new name left the file written under the old one
   sitting there, to be offered back next launch as work that had in fact been
   saved. A test caught it.
3. *The release build needs `-extldflags=-static`.* PLAN called it an "if
   needed". It is needed: without it the exe imports `libgcc_s_seh-1.dll`,
   `libstdc++-6.dll` and `libwinpthread-1.dll` — the last two from Manifold's
   C++ — and no machine without mingw has them. `objdump -p` before and after;
   with the flag the only imports left are Windows system DLLs and the UCRT.

**Verified.**

- `go build ./...`, `go vet ./...`, `go test ./...` — green; goldens twice.
- **The acceptance clause, honestly**: `TestAutosaveIsRecoveredByTheNextRun`
  paints something, never saves it, writes a crash file, and then starts a
  **second process** which finds it and takes it back with the paint intact. Two
  processes because one deliberately ignores its own recovery files — a test
  that recovered from itself would prove nothing about the case the feature
  exists for. A third run confirms the file was consumed.
- **Round-trip**: a save of a document that was just loaded is byte-identical,
  so a save never shows as a change to whatever the user keeps their ships in.
- **The reader survives rubbish**: empty, truncated, garbage, a valid zip with a
  broken document inside — each an error, none a crash. A missing paint PNG
  costs its face the paint and a warning, not the ship.
- **Exports are checked structurally**, because nothing reads them back: every
  glTF index resolves, every accessor fits its buffer, the POSITION bounds are
  the real bounds (viewers frame the camera from those), the OBJ's every
  `usemtl` exists in the MTL, and the STL header does not start with "solid".
- 25 new tests in `internal/io` (39 there now), 7 new app tests, 2 new goldens.
- The statically-linked release exe (9.3 MB) runs the M7 script and produces
  identical output.

**Open.**

- **The user's sign-off on an external viewer.** The OBJ and glTF are checked
  structurally here; whether Blender or an engine likes them is something only
  you can tell me. That is the one part of M8's accept clause I cannot close.
- **The sample ship** on the welcome card is disabled and says it arrives with
  M9, which is where PLAN puts it.
- **No "you have unsaved work" prompt on close.** Closing the window discards
  unsaved work with only the autosave to fall back on. That belongs with M9's
  polish pass, and it is written down here so it is not forgotten.
- The recovery card offers back the **newest** file and discards the rest; a
  list to choose from is more than the case warrants.

**Try it** (`go run ./cmd/modeler`):

1. Start it: the welcome card offers **New ship** and **Open…**.
2. Build something, **Ctrl+S**, name it. The title bar shows the name, and the
   dot beside it appears whenever there is unsaved work.
3. **Ctrl+E**: pick **glb** and export. Drop the file into any glTF viewer — the
   pixels should be crisp, because the file says so.
4. Pick **png** instead: the scale chips and **Transparent background** appear.
5. Paint something, do *not* save, and kill the process from Task Manager. Start
   it again — the work is offered back.

**Next:** M9 — the polish pass: hover/pressed/disabled audit, per-tool cursors,
the `?` overlay, empty states, the embedded sample ship, a perf profile, the
README with credits, and the full-app end-to-end script.

---

## 2026-08-26 — Fix: the lock is now armed first and the face picked second

**The user again, on the same control:** "I want to be able to click the button
'Lock to this face', then select the face I want to lock to, and the camera
moves towards that face. It's not working because it grays out and locks when I
try to click it."

The previous fix made the button *reachable*; it did not make it right. It still
acted on the face under the pointer when it was pressed, and that dependency is
the bug: moving the pointer to a button is exactly what takes it off the face.
So it arms a pick now. Press **Lock to a face…** — always live, never greyed —
and it reads **Click a face…**; the next click chooses the face, turns the
camera to it and locks. That click is spent on the choice and paints nothing.
Pressing the button again cancels, and so does Esc. It is the same shape as
pressing S with no plane selected (SPEC-UX §8.1), which was already in the
program.

The sticky hover from the previous fix stays, because **Face view** and the
resolution mismatch prompt genuinely are about the face you were last pointing
at — there is nothing to arm there.

**Verified.** `TestLockIsArmedFirstAndPickedSecond` drives it with nothing
hovered at all — the case the old version could not handle — and asserts the
arm, the hint copy, the camera turning square-on, and that the click left no
paint. Every M7 golden was regenerated: the button's label and enabled state
changed, 194 max delta at x≈1165, y≈638–669, nowhere else.

---

## 2026-08-26 — Fix: the panel's buttons could not be clicked, and clicking them painted through the panel

**Reported by the user:** "Lock to this face and faceview is buggy, the button
is impossible to click when I am painting… when I am not painting it grays out
and face view disappears, so there's no way for me to use it."

Exactly right, and there were two bugs behind it.

**The button disarmed itself as you reached for it.** Both controls act on "the
face you are pointing at", and the pointer stops being on a face the instant it
leaves the viewport for the panel. So Lock was enabled while you looked at it and
disabled by the time you got there, and Face view — which only appears past 70°
— vanished on the way. The live hover still drives the cursor and the stroke,
because there is no texel under a button; the panel now reads a **sticky** hover
that outlives the journey. The resolution mismatch prompt had the identical bug,
which I had not noticed: its own Use/Resample buttons were in the panel it was
disappearing from.

**And clicking the panel painted the face behind it.** Found while fixing the
first one. `chromeOwnsPointer` counted the toolbar, the tree and the hint bar as
chrome and everything inside the viewport rectangle as model — but the cards
float *inside* the viewport, so the viewport's hit-testing ran behind them. Every
press on a chip resolved whatever face was behind the panel and left a dab on
it. `FloatingCard` now registers its rectangle for the next frame's hit test,
the way the colour popover already did; hovering a widget is not enough of a
test, because the gaps between a card's controls are still the card.

That fix is general — it applies to the extrude, boolean, sketch and transform
cards too, which had the same latent hole.

**One thing I deliberately did not do.** Making a card chrome outright would also
stop an orbit from starting on top of one, and navigation works from wherever
the pointer is in every mode (SPEC-UX §1). A card takes the left button and
leaves the camera alone.

**Verified.**

- `TestThePanelsControlsCanActuallyBeReached` drives the whole journey: hover a
  face, move onto the panel, click the button. It asserts the target survives
  the move and that the click actually locks — the test fails on the old code at
  the first assertion.
- `TestClickingThePanelDoesNotPaintThroughIt` asserts the same click left no
  paint on the model.
- Full suite green; no golden moved, because none of this changes a pixel — it
  changes who the pointer belongs to.

**Try it:** press **P**, hover a face, then move to the panel. Lock stays live
all the way there, and Face view stays put at an oblique angle. Clicking chips
no longer leaves dabs on the hull behind the panel.

---

## 2026-08-26 — Paint tools: shapes, gradients, dithering and a face lock

**Asked for by the user, after running the build.** Two requests, one session:
line/circle/square/soft-brush/gradient tools with 2x2, 4x4 and 8x8 Bayer
gradient modes; and "when painting on a face, focus the camera on it and lock to
it, to keep from painting other faces". Both are now specced — SPEC-UX §13.4 and
§13.5 were written alongside the code, so the comments referring to them are not
pointing at nothing.

**The tools.** Nine now, in two rows: pencil, soft brush, eraser, fill, pick /
line, rect, circle, gradient. The four on the second row are *two-point* tools —
they read where you pressed and where the pointer is now, and nothing in
between. That is what makes them rubber-band, and it was the one thing that
could have gone badly: a drag replaces its pending command with a longer version
of itself every frame, so a shape that read the whole path would have stamped
every size it passed through onto the face. There is a test that drags a
rectangle out and back and checks the ghosts are not there.

**Dithering is one idea used twice.** A soft brush and a gradient both produce a
coverage between nothing and everything. With **None** that coverage blends into
a real colour; with **2x2 / 4x4 / 8x8** it decides *how many whole texels* get
painted instead, so a ramp between two palette colours stays two palette
colours. That is the pixel-art answer to a soft edge, and it is why the user
asked for the matrices in the same breath as the brush. The chips show only for
the two tools that have a coverage to spend.

**Three things I got wrong first and the tests caught.**

1. *The soft brush stored partial alpha.* Obvious, and wrong: the texture
   composites over the **body** colour, not over the paint already on the face,
   so a half-alpha texel over existing paint would show the hull through it. The
   command now hands the body's own colour down and the blend happens in the
   paint layer, storing opaque texels — which also keeps alpha binary for the
   eraser, the dropper and the fill, all of which already assumed it.
2. *The soft brush had no solid middle.* With a pure linear falloff no texel is
   ever fully the brush colour, which reads as a weak brush rather than a soft
   one. It has a 40%-of-radius solid core now.
3. *The locked face was framed by its bounding sphere.* `FrameBox` is
   deliberately orientation-independent so orbiting can never lose anything; on
   a flat wide hull side that wastes most of the viewport. `FrameTightly`
   measures the points along the camera's own axes instead, and the result is
   then slid clear of the palette panel — the one camera move whose entire job
   is "let me see this face" should not put a quarter of it behind a panel.

**The lock is a paint lock, not a camera lock.** Orbit, pan and zoom keep working
exactly as they do everywhere else, because checking your work from an angle is
part of painting. What is fixed is where the paint can land. While locked the
cursor is resolved against the locked face's own **plane** rather than through
the ID pass — a body drifting in front of it cannot steal a stroke, the pointer
running off the face simply shows no cursor, and it costs no readback at all, so
a locked session is *cheaper* than a free one.

**Verified.**

- `go build ./...`, `go vet ./...`, `go test ./...` — all green, goldens twice.
- 24 new unit tests in `internal/paint` (59 total there): the Bayer matrices are
  checked as matrices — a permutation of 0..n²-1, tiling the grid, monotonic in
  coverage, and exactly half the texels at half coverage — and the shapes are
  checked for the things that are invisible in a screenshot: an ellipse
  symmetric in both axes, filled rows with no gaps, a rectangle that is hollow
  in the middle, a ramp that is monotonic along its axis and perpendicular to
  the drag.
- 4 new app flow tests. **The lock's proof**: the same window pixel resolves a
  *different* face once unlocked, and nothing at all while locked.
  **Dithering's proof**: the same drag between the same two colours produces four
  different pictures under the four modes, covering exactly the same texels.
- 8 new goldens across two scripts (`m7_shapes`, `m7_lock`), 23 for M7 in all.

**Open.**

- The dither mode, the two colours, the brush size and the lock are **session
  state, not settings** — none of it survives a restart yet. That goes with M8's
  settings pass, alongside the custom palette noted below.
- A **soft brush with no dithering is off the palette** by construction. That is
  what "blends" means and the panel says so, but a pixel-art purist should stay
  on a Bayer mode.
- Everything still open from the M7 entry below: the Import .hex button waits on
  M8's dialogs (drop a file on the window meanwhile), and custom colours are not
  persisted.

**Try it** (`go run ./cmd/modeler`, then **P**):

1. Hover a face and press **Lock to this face**. The camera turns square-on and
   fills the screen with it. Now run the pointer off the edge — nothing arms.
2. **N** for the gradient. Click the second colour swatch, pick something dark,
   then drag across the face. Try each **Dither** chip and drag again.
3. **C** for a circle, **R** for a rectangle — hold **Shift** for a true circle
   or square, and toggle **Fill the shape**.
4. **B** for the soft brush at size 16, dither **None**: it fades into the hull.
   Switch to **4x4** and it fades in whole texels instead.
5. **X** swaps the two colours. **Esc** unlocks; **Esc** again leaves paint mode.

**Next:** M8 — `.ship` save/load, autosave and crash recovery, OBJ/STL/PNG
export, the file dialogs that finish the palette import, and the settings pass
that makes the brush remember itself.

---

## 2026-08-26 — M7 paint mode: finished

Picking up the hand-off below. The tree it describes as "does not build" builds:
`palette.go` landed in `75c845a`, and the stroke command plus the render atlas
landed in `a10c4d9` before this session started. What was left was the whole app
layer — the cursor, the mode, the panel, the ops, the tests — and it is done.

**What shipped.**

| Piece | Where |
|---|---|
| Texel cursor on the 3D face | `internal/scene/paintcursor.go` — brush square outlined in white, internal grid at 1 px, the armed colour previewed inside it, a dashed face rectangle under the fill tool |
| The mode | `internal/app/paintmode.go` — entry/exit, hover→face→texel, the stroke drag, the eyedropper, Face view, palette import |
| The panel | `internal/app/paintpanel.go` — tools, sizes, chips, the 8x4 page, recents, the mismatch prompt, the Textures eye |
| Eraser / fill / dropper / import icons | `internal/ui/icons.go` (D-11: strokes, not glyphs) |
| Ten paint ops + three dump lines | `internal/io/script.go`, `internal/app/headless.go` |
| Five scripts, fifteen goldens, seven flow tests | `testdata/scripts/m7_*.json`, `internal/apptest/m7_test.go` |

**Two bugs found by building it, both worth naming.**

*The pick pass swallowed strokes after a cut.* Painting a face, cutting a slot
through it and then eyedropping the surviving paint read **nothing** — and
before the cut, at the same pixel, it read the paint. The ID pass draws edge
ribbons five pixels wide and biases them toward the eye so they win over the
faces they belong to (SPEC-RENDER §6.1), which is right when you are selecting
and wrong when you are painting: the boolean had left a fragment boundary under
the cursor. Paint mode now keeps edges and vertices out of the pass entirely
(`Scene.PickFacesOnly`, DECISIONS V-46). A brush cannot paint an edge, so there
was never anything to lose.

*Entering a sketch from paint mode left a stroke pending on the bus.* Exactly one
mode is active (SPEC-UX §1), but `enterSketch`, `BeginExtrude` and `BeginBoolean`
each set `a.Mode` directly and none of them knew paint mode existed. A stroke
mid-drag holds `Bus.pending`; the sketch's first edit would then have arrived on
top of it. All three now call `ExitPaint`, which commits or drops the stroke
first.

**The trap the hand-off named was real, and it is handled.** Paint mode arms on
hover over any face of any visible body, which is the widest hit area in the
program. `update()` routes to `updatePaint` in its own branch, so
`updateTransform`, `updatePushPull`, `handleViewportClick` and box select never
run — disarmed, not merely undrawn. The `Mode == ModeIdle` guard already dropped
both gizmos.

**Two things from the hand-off's notes I deliberately did not do.**

1. *Copy-on-write before painting a shared fragment.* SPEC-GEOMETRY §8.4 is
   explicit that v1 keeps the picture shared, the kernel test asserts pointer
   identity across the boolean, and `command.go` was already written that way.
   Sharing is the contract, not an oversight (V-45). `paint.Copy` stays for the
   day it changes.
2. *Rewriting the `docs/shots` diary.* Turning the Paint button live changed 243
   pixels of every golden in the repository (x 291–336, y 12–27 — measured
   before regenerating, per TESTING §5). The regression baselines were
   regenerated; the diary was not, because it records what each milestone looked
   like when it landed and a greyed-out Paint button was accurate then (V-52).

**Verified.**

- `go build ./...`, `go vet ./...`, `go test ./...` — all green.
- Goldens pass twice in a row from a clean run: determinism holds.
- `go test -race ./internal/model/ ./internal/paint/` — clean.
- 35 paint unit tests; 12 new app tests (7 flow, 5 golden); 15 new goldens
  across five scripts.
- **The persistence contract, end to end** (`TestPaintSurvivesACutInTheApp`):
  paint the hull's front, cut a slot through it, and the same window pixel lands
  on the same texel (6,5) of the same picture (sum `bd4f6efd`, texel 0.375 u) now
  read by **two** fragment faces instead of one — while the mesh went 28 → 54
  triangles and 348 → 320 u³. The eyedropper reads `#21E7E7` on both sides of the
  cut.
- **A real drag** (`TestADraggedStrokeIsLiveAndIsOneStep`): press, twelve frames
  of motion, release. The picture is live mid-drag, the history does not grow
  until release, and then by exactly one. This is the path M6's last bug lived
  on, so it is driven through a real press rather than by calling the command.
- **Density is per face and fixed** (`TestPaintingAShipAtThreeDensities`): the
  same 32 px chip gives 0.375 u/texel on a 12 u face and 0.15625 u/texel on a 5 u
  one, and 128 px gives 0.046875 u/texel on a 6 u one.
- Frame cost with three painted faces: **mean 1.57 ms, p95 3.11 ms, p99 4.10 ms**
  at 1280x720, against the 16.6 ms budget (SPEC-RENDER §8).

**Open, and honest about it.**

- The **Import .hex button is disabled**. The import works — drop a `.hex` file
  on the window — and the button's tooltip says so, but the dialog itself is
  M8's (`ncruces/zenity`, D-10). `ImportPalette` is the entry point M8 wires up.
- **Custom colours are not persisted yet.** The HSV picker and the recents strip
  work in-session; `Settings.CustomPalette` / `RecentColors` already exist and
  are read at startup, but nothing writes them back. That belongs with M8's
  settings pass.
- **No toast per stroke** (V-49), deliberately.
- The **bent-face warn chip** is still M9's (V-40, unchanged).

**Try it** (`go run ./cmd/modeler`):

1. Press **P**. The planes get out of the way and the palette docks on the right.
2. Hover the hull — the texels under the brush are outlined *on the surface*.
   Click a **Res** chip and watch the grid change before you have painted a thing.
3. Drag across a face. Pick another colour, drag again. **Ctrl+Z** takes back one
   stroke, not one texel.
4. Press **G** and click: fill. **E** and drag: erase back to the body's colour.
   Hold **Alt** and click: eyedropper.
5. Orbit until a face is nearly edge-on — a **Face view** button appears; press
   it. Then turn **Textures** off and on to see the bare geometry under the paint.
6. Paint a face, then cut through it (sketch on the Front plane, extrude
   subtract, through-all). The pixels stay exactly where they were.

**Next:** M8 — `.ship` save/load, autosave and crash recovery, OBJ/STL/PNG
export, and the file dialogs that finish the palette import.

---

## 2026-08-26 — M7 paint mode: PAUSED PART-WAY (hand-off to another machine)

> **READ THIS FIRST. `go build ./...` FAILS at this commit and that is expected.**
> `internal/paint` has its tests and two of its three source files. `palette.go`
> does not exist yet, so the package does not compile. Nothing outside
> `internal/paint/` has been touched — every other package is exactly as M6 left
> it, and deleting the `internal/paint/` directory returns the tree to green.

**Where the work stopped.** Mid-sentence, effectively: the last action was adding
the `mesh` import to `internal/paint/brush.go`. The next action was to write
`internal/paint/palette.go`, which is the file that would make the package build.

**Nothing was committed this session, deliberately.** TESTING §5 makes a clean
`build && test && vet` the gate for every commit, and this tree passes none of
them, so `git log` still ends at M6's last fix (`901d38f`) and `HEAD` still
builds. The work lives in the working tree — `git status` shows the untracked
`internal/paint/` and this modified file, and that is the whole delta.

**What exists (all new, all untracked):**

| File | State | Notes |
|---|---|---|
| `internal/paint/mapping_test.go` | **complete** | 7 tests: density from bbox, uv↔world round trip, texel size surviving translate + 90° turn, growth keeping pixels put, the 1024² cap, chip validation, a slanted face |
| `internal/paint/brush_test.go` | **complete** | 10 tests: stroke interpolation (the dotted-line bug), diagonal connectivity, brush-size squares, eraser to alpha 0, flood fill stopping at a wall and staying in its region, eyedropper compositing, the dirty rect covering everything it painted, nearest resample |
| `internal/paint/palette_test.go` | **complete** | 4 tests: 32 opaque distinct colours, Lospec `.hex` parsing, rubbish rejection, recents promoting rather than duplicating |
| `internal/paint/mapping.go` | **complete** | `Allocate`, `Resample`, `UV`, `Texel`, `World`, `Corners`, `Bounds`, `At`, `Set`, `Grow`, `Copy`, `SubImage`, `Blit` |
| `internal/paint/brush.go` | **complete** | `Tool` (Pencil/Eraser/Fill/Pick), `Brush`, `Stroke` (Bresenham walk + square dabs, returns the dirty rect), `Fill`, `Sample` |
| `internal/paint/palette.go` | **MISSING — write this first** | needs `PaletteSize = 32`, `RecentsSize = 8`, `DefaultPalette()`, `ParseHex(io.Reader)`, `Recents` with `Add`/`List`. The tests pin the exact contract |

**None of the tests have ever been run.** They were written before the code they
test, per PLAN §0, and the package has not compiled yet. Expect real failures on
first run — that is the point of writing them first, not a sign something is wrong.

**Design decisions already made** (so the next session does not re-litigate them):

1. **Texture layout: one atlas per body, keyed by `*mesh.FacePaint` — not one GPU
   texture per face.** PLAN's M7 checklist says "GPU texture per painted face";
   this is a deliberate deviation and **still needs a `docs/DECISIONS.md` entry**.
   Two reasons. A body is one `rl.Mesh` and one `rl.DrawMesh` call, so per-face
   textures would mean splitting every painted body into N meshes and rebuilding
   that split after every boolean. And SPEC-GEOMETRY §8.4 has boolean fragments
   *share* one `FacePaint` by pointer — an atlas keyed by the paint object stores
   that image once and lets every fragment sharing it map into the same region,
   which is what the contract actually describes.
2. **Atlas rebuild policy.** Pixels changing (every stroke) → `UpdateTextureRec`
   on the stroke's dirty rect. *Layout* changing (first stroke on a face, or a
   resample) → drop the whole `BodyGPU` via the existing `a.dropGPU(id)` path and
   let it rebuild. Layout changes are rare; geometry edits already do exactly this.
3. **The shader is already done.** `shadedFS` in `internal/render/shaders.go`
   composites `texture0` over the body colour by texel alpha and is driven by the
   `useTexture` uniform, whose location is already fetched in `NewRenderer`.
   `drawBody` currently hard-codes `useTexture = 0` — that line and the diffuse
   map binding are the whole render-side change.
4. **Vertex UVs.** `BuildBodyGPU` already writes a `texcoords` entry per vertex
   (currently `0,0`, with a comment saying M7 fills them). Vertices are duplicated
   per face for flat shading, so each face can carry its own UVs, and because the
   paint mapping is affine within the face's plane, linear interpolation across
   its triangles is exact. No mesh re-upload is needed for a stroke.
5. **Copy-on-write is required and is not written yet.** Fragments share a
   `FacePaint` by pointer identity (`csg` guarantees it, `TestPaintSurvivesACut`
   proves it). Painting one fragment must therefore `paint.Copy` first, or the
   stroke lands on every sibling that came from the same original face. `Copy`
   exists in `mapping.go`; nothing calls it yet.
6. **Glyphs.** SPEC-UX §13.1 draws the panel with `✏ ◻ ▨ 💧 👁`. **None of those
   are in the font atlas** and `TestEveryMessageIsRenderable` sweeps every script
   for unrenderable runes. Use stroke icons (D-11) — `ui.DrawPencilIcon` and
   `ui.DrawEyeIcon` already exist; an eraser, a fill bucket and a dropper need
   drawing in `internal/ui/icons.go`.

**Still to do after `palette.go`, in the order I intended to take them:**

1. `go test ./internal/paint/` — fix whatever the 21 tests turn up.
2. `internal/model/paint.go` — the `PaintStroke` command: dirty-rect before/after
   sub-images (`paint.SubImage` / `paint.Blit`), coalesced per mouse-down through
   the existing `BeginDrag`/`UpdateDrag`/`CommitDrag` bus API, 40 MB cap per
   SPEC-DATA §3.3. Palette and recents are settings, **not** document state.
3. Render: the per-body atlas, UV fill in `BuildBodyGPU`, `useTexture` in
   `drawBody`, nearest filtering, dirty-rect uploads.
4. `internal/scene/paintcursor.go` — the texel cursor as a `render.Overlay`
   (`paint.Corners` gives the four world points; trace them as `OverlayLine`s).
5. `internal/app/paintmode.go` — mode entry, ray→face→texel, stroke drag, the
   res-mismatch prompt, the >70° oblique "Face view" chip, the Textures toggle.
   Wire the toolbar button in `shell.go` (currently `start: nil, milestone: "M7"`)
   and `P` in `handleKeys`.
6. Ops: `paint.res`, `paint.color`, `paint.pixel` are already in `knownOps` in
   `internal/io/script.go` and already have their `io.Op` fields (`Res`, `Hex`,
   `UV`) — they just have no executor. Add `paint.tool` / `paint.size` /
   `paint.textures` alongside them, and add a `paint` line to `dump()`.
7. Scripts + goldens: at least `m7_painted.json` (TESTING §4 wants a painted ship
   flow test), plus goldens at more than one resolution per the M7 accept clause.

**One trap worth naming.** M6's last bug was two tools armed on the same pixel,
where the invisible one ate the press. Paint mode arms on *hover over any face of
any visible body*, which is the widest hit area in the program — check what else
is live in `update()` when `ModePaint` is set, and make sure the transform gizmo
and the push/pull arrow are both disarmed, not merely undrawn.

**Verified:** nothing. No build, no tests, no shots this session — the package is
incomplete by design of where the pause fell.

**Next:** write `internal/paint/palette.go`, then run `go test ./internal/paint/`.

---

## 2026-08-26 — Fix: the face arrow was being eaten by an invisible gizmo

**Reported by the user:** selecting a face and dragging its arrow did nothing —
the arrow would not move and the planes would not hide.

Both symptoms, one cause. Selecting a face armed *two* tools at the same point:
the push/pull arrow and the move gizmo, whose screen-plane handle is a disc
centred exactly where the arrow starts. M6 made the drawing exclusive — only the
arrow appears — but not the input. The gizmo was still hit-tested, and being
hit-tested first it swallowed the press. So the arrow never engaged, its
distance stayed at zero, and the planes stayed because they hide on a drag that
never began. Worse, the drag was not doing nothing: it was silently running the
move gizmo, deforming the face's vertices instead of push/pulling it. Dragging
the arrow up three units produced the toast "Move +0, +3, +0".

A second bug sat directly behind it, which the first was hiding. The drag's
sign was taken from `!Adding()`, and `Adding()` is "distance > 0" — false at the
start of every drag, when the distance is still zero. So the first pixel of an
outward pull would have been read as a push. `Flipped()` — "distance < 0" — is
the honest question, and matches what the extrude arrow already did.

**No drag in this program had ever been tested.** The script runner could click
and it could hover, and every drag-driven tool — the extrude arrow, push/pull,
the move and rotate gizmos, box select — was exercised by calling its internals
directly and never through a press, a run of motion and a release. That is
precisely the path this bug lived on. There is now a `drag` op that does the
real thing, a `kind: "hold"` that stops mid-drag so a shot or a dump can catch
it, and a `drag.release` to finish. The button stays down across ops, because
every op ends by stepping a frame and that frame would otherwise look like a
release.

**Verified:** `TestDraggingTheFaceArrowActuallyPushPulls` drives a real drag and
asserts all three of the reported symptoms — the arrow reads 3 units mid-drag,
`planes drawn` goes 3 → 0 → 3, and the toast says "Pulled the face out" rather
than "Move". Re-armed the old gizmo to confirm the test fails with all three
messages. Full suite green.

---

## 2026-08-26 — Fix: the extrude preview was dimmed with everything else

**Reported by the user, with a screenshot.** Mid-extrude, the pending solid was
as dark as the model behind it and barely readable.

Sketch and extrude modes fade the scene to 30% so the thing being worked on
stands out (SPEC-UX §8.1). The preview solid is a `BodyDraw` in the same list as
every other body, so it was faded along with them — the scene was being dimmed
to highlight something that was also being dimmed. And the three default planes
were not faded at all, so full-strength quads across the viewport were the
brightest thing in a shot that was supposed to be about the extrusion.

**Fixed:** `BodyDraw` grew a `NoDim` flag, set on the extrude and push/pull
previews, and the planes now honour the scene's dim factor. What a mode is
about is the one thing at full strength; everything else, planes included,
recedes.

**Then the user asked for the planes to go entirely while extruding, and they
were right.** Dimming them was not enough — three translucent quads spanning the
viewport still cross the solid being pulled out, and at that moment the only
thing that matters is how far it has come. They are hidden for the duration of
the tool and back the instant it closes; their eye toggles are untouched. The
sketch grid stays, because that is what makes the depth readable.

**And then the planes for push/pull too, with a second screenshot showing why.**
A translucent plane blending against a translucent preview in the same pass
muddles both, and a quad spanning the viewport crosses whatever is being pulled.
Both tools now go through one predicate, `previewOwnsView`.

**That change first went too far and the user sent it back.** Along with hiding
the planes it dimmed the model during a push/pull drag, on the reasoning that
extrude already did and the two tools ask the same question. They do not. An
extrude builds a new solid and the model is background; a push/pull is a direct
edit of a body in place, and what you are judging is the new material *against
the model it is joining*. Dimming that model dims the thing being compared to.
`previewOwnsView` now decides one thing only: whether the planes are in the way.

**Verified:** rendered an extrude in progress on the user's scene — the prism
is now unmistakably the brightest thing in the frame. Sketch mode re-checked:
the sketch still reads first, and the plane quads no longer compete with it.
Full suite green, goldens regenerated.

---

## 2026-08-26 — Fixes: sketch and plane highlighting, sketch camera

**Reported by the user, four things at once.** All four were real.

**A selected sketch did not highlight, and a closed one showed no tint.**
`BuildSketchDraw` had one branch for the sketch being edited and one for
everything else, and the second drew strokes and nothing more. So a finished
sketch showed no region fill — the one thing worth knowing about a sketch you
are not drawing on, since a filled region is one you can extrude — and being
selected changed nothing about how it looked. Idle sketches now tint their
closed regions quietly, and a selected one draws in the accent with its
boundary, like everything else selected.

**A selected plane did not highlight.** It did, technically: the 1.5-pixel
border turned accent. On a quad that fills the viewport that is not a highlight
anybody sees. A selected plane is now tinted across its whole face and bordered
at three pixels.

**Sketching faced the wrong way.** `Camera.LookAlong` takes the direction the
eye sits in, so looking at a face means passing its outward normal. M5 passed
the negation, which put the camera inside the body staring at the back of the
surface you had just asked to draw on. The geometry was right the whole time and
the view was inside out. `TestSketchingLooksAtTheFaceNotThroughIt` asserts the
camera's forward opposes the normal, and fails with the old sign.

**"Add an extrude button top left."** It was already there, in both toolbars.
The real problem is that it never greyed out: in Idle it looked identical
whether or not anything could be extruded, so it taught you nothing and you had
to click it to find out. Toolbar buttons now ask the tool whether it can run and
grey out with the reason when it cannot — Extrude wants a sketch with a closed
profile, Boolean wants two bodies (SPEC-UX §15).

**Verified:** shots read for all four states. Full suite green; goldens
regenerated, since plane tint, sketch tint, toolbar greying and the face-sketch
camera all changed what is on screen.

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

**Fixed twice.** The first attempt let a sketch win over a *plane*, and the user
reported it still broken — correctly. The hull sits on the Top plane, so a
sketch drawn there has a body behind it, not a plane, and my rule of "a body in
front still wins" was simply the wrong rule.

Sketches draw with the depth test off (V-12) so that sketching on a plane
running through a hull is possible at all. That means a sketch is *always* the
thing you can see at its own position, and a click has to agree with the
rendering. A visible sketch under the cursor now wins outright. Bodies away from
the sketch are unaffected; a sketch that gets in the way has an eye in the tree,
and a consumed one hides itself. The test is done in the sketch's own plane
rather than by rendering ids — the same point-in-region code the sketch-mode
hover uses, and exact. Open profiles are pickable by their strokes, closed ones
anywhere inside.

**The hint bar disagreed with the click, and that took longer to find than the
bug.** With the click fixed, hovering a sketch still read "Top plane". The hint
was reaching `TreeHover` first — which is fed from the viewport hover — so the
fix belonged in `viewportHoverRef`, not in a special case inside `HintText`.
Putting it there means the tree row highlights too, which is what hovering
anything else already does.

**Verified:** `TestClickingASketchSelectsIt` runs with every body visible —
which is what the user had, and what the first fix missed. Hover names the
sketch and not the hull behind it, clicking selects it, clicking the body away
from the sketch still selects that body, and E opens the extrude with its region
picked for a committed solid of 108. Full suite green, no golden drift.

**The lesson:** the first fix was tested against a scene with the bodies hidden,
because that is how the other sketch scripts are written. The bug lived exactly
in the case the test scene had been tidied out of.

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
