# Sketch_func.md — The full sketch toolset

> The plan for growing sketch mode from four tools into a real 2D drafting
> environment: lines (midpoint), rectangles (corner / centre / aligned),
> circles (centre / 3-point), ellipses, arcs (3-point / tangent / centre /
> elliptical), conics, polygons (inscribed / circumscribed), splines, beziers,
> points, project/convert, fillet, chamfer, offset, slot, mirror, construction
> geometry, linear/circular patterns, transform, and a reference image
> underlay. Requested by the user 2026-08-27 from Onshape's sketch toolbar;
> this document supersedes the priority order of PLAN.md §M10's backlog for
> sketch work specifically.
>
> **Audience: the executor model.** This plan is written to be followed
> without re-deriving anything. Where a decision could go two ways, it has
> been made here; deviate only with a DECISIONS entry saying why. Everything
> in *PLAN.md §Milestone protocol* applies: specs are truth, tests before
> code, one PROGRESS entry per session, commit per milestone, never push,
> keep the app runnable, verify headlessly.

---

## 0. How to read this plan

Milestones are **SK1–SK6**, in dependency order — commit prefix `SK<N>:`.
SK1 is infrastructure and cheap wins; each later milestone stands on it.
SK2–SK5 are each independently shippable. SK6 (image underlay) needs the
user's go-ahead before starting; the "stretch" items inside SK2 (elliptical
arc, conic) may be deferred with a DECISIONS entry and a line in the card's
tooltip copy — ship the milestone without them rather than stalling on them.

Before writing any code, re-read: `docs/SPEC-UX.md §8`, `docs/SPEC-GEOMETRY
§3–4`, `internal/model/sketch.go`, `internal/sketch/session.go`,
`internal/geom/sketch2d/region.go`, and the **traps** in §2 below.

---

## 1. The architecture you are extending (verified 2026-08-27)

The whole sketch system funnels through one seam, and every feature in this
plan is either a new producer for that seam or a manipulation of what is
already in it:

```
model.Entity ──Points()──▶ []geom.Vec2i ──AppendSegments()──▶ []sketch2d.Seg
                                                              │
                                              sketch2d.Build ─┴─▶ Arrangement
                                                        (regions, open ends)
```

- **`model.Entity`** (internal/model/sketch.go) is a flat struct — `Kind`,
  `A`, `B`, `C geom.Vec2i`, `R int64`, `Segs int` — with per-kind meaning.
  Kinds today: `EntLine`, `EntRect`, `EntCircle`.
- **`Points()`** returns the polyline (a circle is its n-gon, D-06). This is
  the *only* place curve maths lives. **Every curve in this plan is a
  tessellation by definition** — arcs, ellipses, splines, conics all become
  short integer segments here, exactly as circles already do. Nothing
  downstream (regions, extrude, booleans, paint) changes at all. This is the
  single most important sentence in the plan.
- **`Closed()`** decides whether the last point joins the first;
  **`Degenerate()`** decides whether a finished entity is kept;
  **`AppendSegments`** explodes via Points(); **`Sketch.Segments()`** feeds
  `sketch2d.Build`. All coordinates are integer subunits (256/u,
  `geom.SubunitsPerUnit`); the only rounding allowed is inside `Points()`
  (SPEC-GEOMETRY §3).
- **`sketch.Session`** (internal/sketch/session.go) is the tool state
  machine: `Tool` enum, `Click(p) ClickResult` per-tool, `chain []Vec2i` for
  multi-click tools, `anchor + hasAnchor` for two-click tools, `RubberFrom()`
  for the preview, `Escape()` unwinds. Entities are committed by the **app**
  through the bus (`model.AddEntity`), never by the session — that is what
  makes drawing undoable with no special case.
- **Snapping**: `sketch.Resolve(raw, entities, rubberFrom, config)` in the
  same package; endpoints/midpoints come from entity `Points()`.
  `EntityAt(p, ents, radius)` is pick; `distanceToEntity` is its metric.
- **Drawing**: `scene.BuildSketchDraw` renders entities (from their points),
  the rubber band, snap glyphs, region fills.
- **Script ops**: `sketch.line/rect/circle/grid/...` in
  `internal/app/headless.go`, validated against `knownOps` in
  `internal/io/script.go` (op names must be registered THERE too — forgetting
  it produces "unknown op" at load, found the hard way for `sketch.grid`).
- **Serialization**: entities marshal by their JSON field tags with
  `omitempty`; the .ship reader is tolerant. New fields are automatically
  backward-compatible to read; **old builds opening new files** is handled by
  the manifest version warning (io/ship.go) — bump the document version once,
  in SK1, and note "newer file" copy already exists.

### 1.1 The new-entity-kind checklist

Any milestone that adds an entity kind touches exactly these places. Work
through the list top to bottom; it is the plan's guarantee of "utterly":

1. `internal/model/sketch.go` — const `Ent<Kind>`, `String()`, struct fields
   (see §3), constructor `New<Kind>`, `Points()` case, `Closed()` case,
   `Degenerate()` case. **Unit tests first** in `internal/model`: points,
   closedness, degeneracy, JSON round-trip (marshal→unmarshal→equal).
2. `internal/geom/sketch2d` — usually nothing (it only sees segments). Add a
   region test proving the new kind's segments close where they should.
3. `internal/sketch/session.go` — `Tool<Name>` const, `String`, `Shortcut`,
   `Hint`, a `click<Name>` state machine, `RubberFrom` case, `CancelDraw`
   coverage, `distanceToEntity` case. **State-machine tests first** in
   `internal/sketch`: the full click sequence, Esc at every stage, degenerate
   refusal with its message.
4. `internal/scene/sketchdraw.go` — preview while drawing (rubber band shows
   the real tessellation, not a stand-in) and finished rendering. Construction
   entities draw dashed (SK1 adds the dashed style once, for everyone).
5. `internal/app/sketchmode.go` / `shell.go` — keyboard case, toolbar flyout
   entry, Shift-constraint if the tool has one, hint copy.
6. `internal/app/headless.go` + `internal/io/script.go` — op
   `sketch.<name>` (register in `knownOps`!), op fields on `io.Op`.
7. `testdata/scripts/sk<N>_<name>.json` + golden via
   `internal/apptest/sk<N>_test.go` — draw it, extrude it, shot both. The
   extrude half is not optional: a shape that renders but will not extrude is
   half a feature.
8. `docs/SPEC-UX.md §8` amendment + `docs/DECISIONS.md` entry (numbering
   continues from V-92) + PROGRESS.

### 1.2 Traps (each one has already cost a session)

- **One parser per dump line.** If you extend a `dump()` line in
  headless.go, exactly one regex in `internal/apptest` parses it — update
  that one, add none. (PROGRESS, M7/M9.)
- **Golden policy.** Same-machine baselines; on a deliberate visual change,
  measure/eyeball the diff first, regenerate with `GOLDEN_UPDATE=1`, record
  the cause in PROGRESS. `docs/shots/` is a historical diary — add new
  names, never regenerate old ones.
- **Widget kit is frozen** (SPEC-UX §4). SK1's flyout is a new widget and
  needs its DECISIONS entry.
- **PowerShell 5.1** on this machine has no `&&`; give commands with `;`.
- **Session tests are GPU-free**; app-level logic tests can be too (see
  `bareApp` in internal/app/guard_test.go). Only apptest needs the exe.
- **`Segs` is clamped to [3,64]** for circles; new tessellations must chose
  their own clamps and document them (see §3 table).
- The **Esc order** in sketch mode is: cancel half-drawn → clear selection →
  exit mode. New multi-stage tools must unwind one *stage* per press, not
  bail out entirely (three-click tools: third stage → second → gone).

---

## 2. Product decisions (made here, once)

- **No constraint/dimension solver.** Onshape's sketch power is its solver;
  this program's is the pixel grid. We take the *tools*, not the solver.
  Snapping, Shift-constraints and grid steps are the whole inference story.
  Explicit non-goal; put it in SPEC-UX §8's preamble.
- **Everything tessellates.** No true curves anywhere downstream (§1). The
  circle-segments card control generalizes into a per-tool "smoothness"
  where noted in §3.
- **Modify tools are commands over entities** (`ReplaceEntities`, SK1), so
  fillet/offset/mirror/patterns are one undo step each, atomic
  compute-then-swap like everything else on the bus.
- **Mirror here is a sketch-entity copy**, not the live model symmetry of
  PLAN M10 item 1. They are different features; say so in the card tooltip
  ("copies entities — live model symmetry is a separate mode").
- **Aligned (rotated) rectangle emits four lines**, not a new entity kind:
  `EntRect` is axis-aligned by definition and a rotated rect is exactly its
  edges. Tradeoff (not movable as one unit) goes in DECISIONS.
- **Inscribed vs circumscribed polygon normalizes at creation**: the
  circumscribed variant stores the inscribed-equivalent radius
  (`R' = R/cos(π/n)`). One entity kind, no variant flag.
- **Projected geometry is not associative.** `Use` copies edges as plain
  entities (marked construction by default); they do not update when the
  source moves. Matches the existing consumed-sketch honesty (SPEC-UX §8.8).
- **Conic and elliptical arc are stretch goals** inside SK2. Implement last
  within the milestone; defer freely with a DECISIONS entry.

---

## 3. Data model

New/changed fields on `model.Entity` (all `omitempty`, all backward
compatible to read):

```go
type Entity struct {
    Kind EntityKind `json:"kind"`
    A    geom.Vec2i `json:"a,omitempty"`
    B    geom.Vec2i `json:"b,omitempty"`
    C    geom.Vec2i `json:"c,omitempty"`
    D    geom.Vec2i `json:"d,omitempty"`    // NEW: 4th anchor (arc mid, slot width pt…)
    R    int64      `json:"r,omitempty"`
    W    int64      `json:"w,omitempty"`    // NEW: a second scalar (slot width, ellipse minor)
    Segs int        `json:"segs,omitempty"`
    Pts  []geom.Vec2i `json:"pts,omitempty"` // NEW: spline/bezier control points
    Construction bool `json:"cx,omitempty"`  // NEW: guides, not geometry
}
```

Kind-by-kind storage and tessellation (append kinds to the enum **in this
order** and never reorder — the values are serialized):

| Kind | Fields | Closed | Points() tessellation |
|---|---|---|---|
| `EntPoint` | A | no | `[A]` — AppendSegments already skips len<2; snapping sees it |
| `EntArc` | C centre, A start pt, B end pt, Segs, `R` = sweep sign (+1 ccw / −1 cw) | no | angles from A,B about C; radius = |A−C|; steps = `max(2, round(Segs·sweep/2π))`, Segs defaulting from session CircleSegs |
| `EntEllipse` | C centre, A major-axis endpoint, W semi-minor, Segs | yes | rotated parametric ellipse; angle of A−C is the rotation |
| `EntArcE` (elliptical arc, stretch) | ellipse fields + B start pt + D end pt | no | ellipse params clipped to [t0,t1] |
| `EntPolygon` | C centre, A first vertex, Segs = n (clamp 3..24) | yes | n exact vertices rotating from A |
| `EntSlot` | A centre 1, B centre 2, W half-width | yes | two parallel edges + two semicircle caps at `Segs`-derived resolution (8 per cap default) |
| `EntSpline` | Pts (through-points, ≥2), Segs = subdivisions per span (clamp 2..16, default 8) | no (unless Pts[0]==Pts[last]: then closed) | Catmull-Rom through Pts, uniform parameterization |
| `EntBezier` | Pts = 4 control points, Segs per span | no | single cubic; multi-segment beziers are several entities |
| `EntConic` (stretch) | A, B endpoints, C shoulder, W = rho in per-mille (0..1000), Segs | no | rational quadratic bezier |

`Degenerate()`: point never (a point is its own reason); arc when sweep≈0 or
R=0; ellipse when either radius 0; polygon when R 0; slot when W 0 (A==B is
*legal* — that is a circle-shaped slot… refuse it anyway for v1, message "a
slot needs two centres"); spline/bezier when all Pts equal.

`Construction`: `Sketch.Segments()` skips construction entities (regions
never see them); `Resolve` and `EntityAt` still do (guides exist to be
snapped to and selected); `BuildSketchDraw` draws them dashed in a dimmer
stroke. One new generic command `SetConstruction{Sketch, Indices, On}`.

New generic command (SK1, in `internal/model`):

```go
// ReplaceEntities removes and adds entities in one atomic, undoable step.
// Fillet, chamfer, offset, mirror, patterns and transform are all this.
type ReplaceEntities struct {
    Sketch  uint32
    Remove  []int          // indices into Entities, sorted, validated
    Add     []model.Entity
    Label   string         // the undo name: "Fillet", "Mirror 4 entities", …
}
```

Unit tests: remove+add atomicity on a failing validate, undo restores exact
order/indices, name passthrough.

---

## 4. UI: tool groups, keys, hints

### 4.1 The flyout (SK1, the one new widget)

`ui.ToolFlyout`: a toolbar button showing the group's *current* tool icon
plus a small chevron. Click the icon = activate current tool; click the
chevron (or long-press: skip, no long-press in this app) = popover listing
the group's tools with icon + name + shortcut, built on the existing
popover/Defer/`OverlayCapturesPointer` machinery exactly like the colour
picker (one open popover at a time — reuse `popoverState`; if that fights
the colour picker's single-slot design, generalize the slot, don't add a
second). Choosing an item re-arms the group button. DECISIONS entry
required (widget list is frozen). Keyboard: pressing a group's key cycles
its members (L, L again = midpoint line); the hint bar names the switch.

### 4.2 Groups (mirroring the user's screenshots)

| Group (key) | Members |
|---|---|
| Line (L) | Line · Midpoint line |
| Rect (R) | Corner rectangle · Centre rectangle · Aligned rectangle |
| Circle (C) | Centre circle · 3-point circle · Ellipse |
| Arc (A) | 3-point arc · Tangent arc · Centre arc · [Elliptical arc] · [Conic] |
| Polygon (P) | Inscribed · Circumscribed |
| Spline (S) | Spline · Bezier |
| Point (.) | Point |
| Use (U) | Project/Convert |
| Fillet (F) | Fillet · Chamfer |
| Offset (O) | Offset · Slot |
| Mirror (M) | Mirror |
| Construction (Q) | toggle armed-construction + convert selection |
| Pattern (X) | Linear · Circular · Transform |
| Image (—) | Insert image (SK6; toolbar overflow or card button) |

All keys are sketch-mode-local (precedent V-50); none of V/L/R/C/E/Del/Esc
change meaning. P and M collide with *idle* keys only, which never run in
sketch mode. The sketch toolbar will overflow a narrow window: groups
collapse to icon-only (drop labels) below a measured width — compute, don't
guess, and the `?` sheet gains a "Sketch tools" section listing every key.

### 4.3 Hint copy

Every tool's `Hint()` says the next click ("Click the arc's start · then
end · then a point on it"), and every refusal is a toast with a reason.
Three-click tools show stage-appropriate hints. Write the copy in the spec
amendment first, then paste it into code — the spec is truth.

---

## 5. Milestones

### SK1 — Foundations: flyout, construction, points, easy variants, ReplaceEntities

The milestone that makes the rest mechanical.

**Build:**
1. `ui.ToolFlyout` + sketch toolbar regrouped (groups of one are plain
   buttons until their siblings arrive).
2. `Entity.Construction` + dashed rendering + `Q` toggle +
   `SetConstruction` command + "construction" chip state in the card.
3. `EntPoint` + Point tool (single click, entity per click).
4. **Midpoint line** (session-only: click midpoint, click an end, emit
   `NewLine(2·mid−end, end)`; Shift constrains like Line).
5. **Centre rectangle** (session-only: click centre, click corner, emit
   `NewRect(2·c−p, p)` normalized).
6. **Aligned rectangle** (three clicks: edge A→B, then height point;
   emits four lines; Esc unwinds one stage).
7. `ReplaceEntities` + `TransformEntities` groundwork (move-by-delta only;
   the full transform card lands in SK5).
8. Document version bump + tolerant-read test (new-field entity in an old
   reader path).

**Tests first:** model round-trips; session state machines incl. staged Esc;
`TestConstructionNeverMakesRegions`; `TestMidpointLineMirrorsItsAnchor`;
`TestAlignedRectangleEmitsFourConnectedLines` (assert the region closes);
apptest golden `sk1_tools` (draw one of each on Front, shot + extrude shot).

**Ops:** `sketch.point`, `sketch.construction` (indices or "armed"),
`sketch.midline`, `sketch.centerrect`, `sketch.alignedrect`. Register all in
`knownOps`.

**Accept:** all four new gestures drawable interactively and by script;
construction never fills; goldens; spec §8 amended; DECISIONS (flyout,
aligned-rect-as-lines, construction semantics).

### SK2 — Circles, arcs, ellipses

1. **3-point circle**: three clicks → circumcentre (integer-safe: compute in
   float, round once, refuse collinear with a toast) → existing `EntCircle`.
   Session-only; no new kind.
2. **`EntArc`** + three input gestures, all constructing the same canonical
   arc: **centre arc** (centre, start, end — sweep follows the cursor's
   winding at the third click), **3-point arc** (start, end, then a point on
   the arc — circumcentre again), **tangent arc** (armed only when the last
   selected/most recent entity endpoint has a direction; starts tangent to
   it — if ambiguity fights you, require clicking an existing endpoint
   first; a Rejected message beats a wrong guess).
3. **`EntEllipse`**: centre, major-axis endpoint, then minor extent
   (projected perpendicular). Shift makes it a circle (then emit EntCircle).
4. Stretch: **elliptical arc**, **conic** (per §3; defer with DECISIONS if
   they threaten the milestone).

Arc smoothness derives from the card's segment control: rename the card row
"Circle segments" → "Curve segments" and let it drive `Segs` for circles,
arcs (pro-rated by sweep) and ellipses alike. Spec §8.3 amendment.

**Tests first:** circumcentre math (collinear, tiny-radius, huge-radius
clamps); arc tessellation endpoint-exactness (first/last point ==
A/B *exactly* — snap targets depend on it); winding both ways; ellipse
closure; per-gesture session tests; golden `sk2_curves` (arcs + ellipse
profile, extruded — the region engine closing across arc-line joints is the
real assertion).

**Ops:** `sketch.circle3`, `sketch.arc` (kind:"center"/"points"/"tangent"),
`sketch.ellipse`.

### SK3 — Polygons and slots

1. **`EntPolygon`** inscribed + circumscribed (normalized, §2); the card
   shows "Sides" (3..24) when the tool or a polygon is selected — same
   pattern as circle segs.
2. **`EntSlot`**: two centre clicks then a width click (or Shift for
   width = last width). Caps tessellate at 8 segments each, `Segs`-driven.

**Tests:** polygon vertex exactness (first vertex == A), circumscribed
normalization equivalence, slot region closure, degenerate slot refusal;
golden `sk3_shapes` with an extrude (a slot cut through a plate is the
canonical use — make that the shot).

**Ops:** `sketch.polygon` (n, kind), `sketch.slot`.

### SK4 — Splines and beziers

1. **`EntSpline`**: click points like the Line chain; double-click or
   clicking the start ends it (closed spline when clicking start). Esc
   drops the last placed point (one per press), then the spline.
   Catmull-Rom through-points, 8 subdivisions per span.
2. **`EntBezier`**: four clicks (end, control, control, end); preview shows
   the control cage dashed.
3. **Control-point editing**: with Select, a selected spline/bezier draws
   its Pts as handles; dragging one runs a `MoveEntityPoint` command
   (extend the existing endpoint-drag machinery in §8.5 — read it first,
   the pattern exists for line endpoints).

**Tests:** spline interpolation passes *through* every Pt exactly (round
Pts to the curve — the through-points are snap targets); closed-spline
region; handle drag undo; golden `sk4_spline` (a curved fuselage profile,
extruded).

**Ops:** `sketch.spline` (pts, closed), `sketch.bezier`.

### SK5 — Modify tools (the multiplier milestone)

All are `ReplaceEntities` productions over the current selection; all live
in a `sketch/modify.go` (pure, GPU-free, heavily unit-tested) with thin app
wiring. Gate each on selection shape with `Rejected` messages.

1. **Fillet / Chamfer**: select two lines sharing an endpoint (or one
   corner click near a shared endpoint with the tool armed), then a radius
   via drag or the card's number field. Fillet = trim both + `EntArc`
   tangent to both; chamfer = trim + line. Refuse when the radius exceeds
   either line ("the radius doesn't fit — N u is the most this corner
   takes", clamp precedent V-? draft clamp).
2. **Offset**: select entities forming a chain or loop (use the
   Arrangement to find the loop containing a clicked region), distance via
   drag/card, sign by cursor side. Miter joins, clamped; refuse
   self-intersecting results for v1 with the draft-clamp-style message.
   Output marked construction if the source was.
3. **Mirror**: select entities → M → pick the mirror line (a selected line
   entity, or the sketch's U/V axes offered as chips) → mirrored copies
   added. Point reflection math is exact in integers.
4. **Linear pattern**: selection → count (card) + delta (two clicks or
   card fields) → copies. **Circular pattern**: centre click + count +
   total angle (default 360°) → copies; rotation rounds each point once.
5. **Transform**: the Select tool's card grows Move Δx/Δy, Rotate angle
   about selection centroid, Scale % — each a single command on commit.
6. **Use/Project**: generalizes `ProjectFaceOutline` (app/facesketch.go):
   with the tool armed, click a face of any visible body → its boundary
   loops project onto the active sketch plane as construction lines; click
   a visible sketch → its entities copy in (converted to this plane's
   coordinates). Non-associative (§2).

**Tests first (the bulk of the milestone):** fillet tangency (arc endpoint
exactly on each trimmed line), chamfer symmetric-distance, offset of a
rect = bigger/smaller rect, offset clamp refusal, mirror exactness, pattern
counts/positions, transform round-trip via undo, project produces closed
construction loops. Goldens `sk5_fillet`, `sk5_offset_mirror`,
`sk5_pattern` with extrudes.

**Ops:** `sketch.fillet`, `sketch.chamfer`, `sketch.offset`,
`sketch.mirror`, `sketch.pattern` (kind linear/circular), `sketch.xform`,
`sketch.use` (body/face or sketch id).

### SK6 — Image underlay (user sign-off before starting)

Not an entity: `Sketch.Image{PNG bytes, C, WidthUnits, Opacity}` stored as
`images/<sketchID>.png` in the .ship zip (tolerant reader: missing file =
no image, one warning — mirror the paint-member pattern in io/ship.go).
Import via drag-drop (extend V-87's router) and a card button (dialog:
`AskOpenImage`). Rendered as a textured quad on the sketch plane under the
grid at the sketch's dim layer, nearest-filtered (this program's whole
aesthetic), with card controls for opacity/scale and a drag handle for
position. Excluded from regions, snapping and exports. Byte-identical
resave test; golden `sk6_image` with an embedded test PNG.

---

## 6. Risk register

| Risk | Sev | Mitigation |
|---|---|---|
| Region engine chokes on many short segments (splines) | Med | It is O(n²)-ish on intersections; benchmark `sketch2d.Build` with 500 segments in SK4 *before* building spline UI; clamp spline Segs; the existing per-edit rebuild cache already amortizes |
| Offset self-intersection | High | v1 refuses with a clamp-style message; never emit a bowtie (assert via Arrangement well-formedness in tests) |
| Tangent-arc ambiguity | Med | Require an existing endpoint as the start; Rejected messages over guesses |
| Tessellation endpoints drifting off snap targets | Med | Points() must return A and B *bit-exactly* for open curves — a test per kind, stated in each milestone |
| Toolbar overflow at MinWindowW | Med | Measure in SK1; icon-only collapse; the `?` sheet is the fallback discoverability |
| Flyout vs single-popover assumption | Low | Generalize `popoverState` if needed; do not add a parallel mechanism |
| Enum/JSON drift | Med | Kinds append-only; round-trip tests per kind; version bump once in SK1 |
| Scope creep toward a constraint solver | High | §2 non-goal; anything smelling of "solve" gets a DECISIONS refusal |

---

## 7. Definition of done (per milestone and overall)

- Every checklist row of §1.1 done for every new kind; `go build ./...`,
  `go vet ./...`, `go test ./... -count=1` green (15+ packages).
- Every new tool: drawable interactively, drivable by script, **extrudable**
  (goldens include an extrude), undoable as one step, refusable with words.
- Spec amended before code; DECISIONS entry per real decision; PROGRESS per
  session; one commit per milestone (`SK<N>: …`); exe rebuilt
  (static flags per PLAN) and left at the repo root; never push.
- The sample ship still builds (`TestTheSampleShipBuildsEndToEnd`) — the
  regression canary for everything here.
- After SK5, update README's keyboard table and the `?` sheet in one pass.

*Overall* done: a user can draft the Onshape screenshot set end to end —
slot-cut a plate, fillet a profile, mirror half a hull, pattern portholes —
without leaving sketch mode, and every one of those verbs has a test that
fails if it rots.
