# SPEC-UX — Product, Interaction & Visual Specification

The pillars (PLAN §1) rank Easy > Polished > Crisp > Robust-feel. Every flow below is written so a first-time user succeeds without documentation: the **hint bar always says what to do next**, and **everything pickable pre-highlights on hover**.

## 1. Modes & input model

The app is a state machine of modes; exactly one active:
`Idle` (select/navigate) · `Sketch` · `Extrude` (transient) · `Boolean` (transient) · `Transform` (implicit when dragging gizmos in Idle) · `Paint`.

Camera navigation works identically in **every** mode:
- **RMB-drag** orbit (turntable, world-up locked, no roll)
- **MMB-drag** or **Shift+RMB** pan
- **Wheel** zoom to cursor (ortho: scale; persp: dolly), smoothed ~120 ms
- **F** frame selection (or all if none); **Shift+F** look square-on at the selected plane or face and frame it (V-153; falls back to plain framing when nothing flat is selected, and declines when two faces disagree about which way is out); **O** toggle ortho/perspective
- View cube & triad always visible (§6)

**LMB is the "do" button** and is mode-owned: select in Idle, draw in Sketch, drag gizmos in Extrude/Transform, paint in Paint. LMB never orbits.

**Esc stack** (one level per press): cancel current drag → cancel pending tool input (e.g., half-placed line chain) → exit transient tool (Extrude/Boolean, discarding preview) → exit mode (Sketch/Paint, keeping work) → clear selection.

Every tool activation sets the hint bar; every invalid click gives feedback (subtle shake of hint text + specific message, e.g. *"That face isn't flat — pick a flat face to sketch on"*). Silence is never the answer.

## 2. Window layout

The file buttons sit at the right end of the toolbar: **New · Open · Save · Export**, laid out from the right edge inward and read left to right. Each is the toolbar's face of a shortcut (Ctrl+N / O / S / E) rather than a second way to do something — New and Open go through the same unsaved-work gate the keyboard does (§2.1).

### 2.1 The unsaved-work gate
Anything that would replace the open document — New, Open, the sample ship, the window's close button — asks first when the document is dirty, and asks with **three answers, because the question has three**: *Save* (the confirm, so Enter is the answer that keeps the work), *Discard*, and *Keep working*. Escape is not an answer: it dismisses the question and changes nothing (V-81, V-143). Save runs first and the parked action follows **only if the save succeeded** — a dialog waved away is not a save.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ TOOLBAR [Sketch S][Extrude E][Boolean B][Move M][Paint P]│[↶][↷]│[+][📂][💾][⬆]│
├─────────────┬────────────────────────────────────────────────────────────┤
│ TREE PANEL  │ VIEWPORT                                        ┌────────┐ │
│ ▾ Planes    │                                                 │  cube  │ │
│   👁 Top    │        (3D scene)                               └────────┘ │
│   👁 Front  │                                                            │
│   👁 Right  │                                    ┌ contextual card ┐     │
│ ▾ Sketches  │                                    │ (extrude opts…) │     │
│   👁 Sketch1│                                    └─────────────────┘     │
│ ▾ Bodies    │                                                            │
│   👁 Body1 ■│                                                   [triad]  │
├─────────────┴────────────────────────────────────────────────────────────┤
│ HINT BAR: "Click a plane or flat face to start a sketch"          v1.0.0 │
└──────────────────────────────────────────────────────────────────────────┘
```

- Toolbar 40 px; tree panel 240 px (collapsible to 0 with ‹ handle); hint bar 26 px; viewport = remainder. Min window 1280×720.
- **Contextual card**: floating panel inside the viewport, right side, for the active tool's options (extrude opts, boolean opts, paint palette docks here as a taller panel). Never covers the cube/triad.
- Paint mode swaps the contextual card for the **palette panel** (§13).
- View cube top-right **inside the viewport** (R15), axis triad bottom-right (R14).

## 3. Theme (dark; tokens centralized in `ui/theme.go`, retunable from `theme.json`)

| Token | Value | Use |
|---|---|---|
| bg | #111319 | window bg |
| panel | #1A1D25 | toolbar/tree/hint |
| card | #252A35 | floating cards, fields |
| stroke | #3A4150 | hairlines, borders |
| text | #E8EAF0 | primary text |
| textDim | #9CA6B6 | secondary, hints |
| accent | *the current mode's accent, §3.1* | selection, active tool, links |
| accentSoft | accent at α 0x33 | region fills, soft highlights |
| warn | #FFA63C | clamped draft, non-planar chip |
| error | #FF5D5D | open ends, failed ops |
| success | #3DD68C | confirm ✓, valid states |
| viewportBg | vertical gradient #2A303C (top) → #12141A | 3D background |
| gridMinor / gridMajor | #FFFFFF0F / #FFFFFF24 | sketch grid |
| edgeLine | body color × 0.35, α 0.85 | mesh crease/silhouette edges |
| hover | #FFFFFF14 overlay | any hoverable |

Axis colors: X #E5484D, Y #46A758, Z #3E63DD (triad, gizmo arrows, plane tints: Right=YZ→X-tint, Top=XZ→Y-tint, Front=XY→Z-tint at 10% alpha + 30% alpha border + small label).

### 3.1 Mode accents (V-149)

The accent is not one colour: it is whichever of these the current mode owns, and the app swaps it in at the top of every frame. Everything that draws "the accent" — selection glow, active-tool underline, chips, sliders, toggles, card titles and their edge stripe, the sketch overlay, the extrude arrow, the hint bar's mode chip — follows without knowing.

| Mode | Accent | Hue |
|---|---|---|
| Model (idle, move, selection, files) | #53A4FF | blue |
| Sketch | #FFD94A | gold |
| Extrude / push-pull | #2ED0CC | teal |
| Boolean | #B48CFF | violet |
| Paint | #FF6FB5 | pink |
| Markers (placing a dot) | #A6F04E | lime |

Rules: the hues sit far apart from each other and from warn/error/success (pinned by test), so a warning still reads as a warning inside any mode. The toolbar's mode buttons wear their own colour always — the toolbar is the legend. Tree icons wear their kind's colour: planes their axis colour, sketches gold, bodies blue, markers lime. Paint tools group by what they do: pink puts pixels down, blue picks and selects (eyedropper, wand), teal builds structure (edge lines, tiles). The hint bar carries a chip naming the mode in its accent. The **viewport is never tinted** — painting needs a colour-true view — so accents live in the chrome only.

### 3.2 theme.json

Every token above, as `#RRGGBB` / `#RRGGBBAA` strings, in `theme.json` beside `settings.json`. Written out with the built-in values on first launch so every key is there to edit; read at launch. A missing key keeps its built-in value; a key that will not parse rejects the whole file (toast) and the built-in palette stands — a half-applied palette is worse than the default one. Headless runs never read it, so goldens depend on nothing outside the repo.

Body auto-color cycle (8): #8EA3B0, #B08E8E, #8EB09B, #A38EB0, #B0A98E, #8E9BB0, #B08EA6, #96B08E (desaturated so paint reads on top).

Type: embedded Go Regular — 13 px UI, 15 px section headers, 11 px hints/badges; line height 1.4; UI scale follows the OS display scale (raylib `GetWindowScaleDPI`) rounded to 1.0/1.25/1.5/2.0, fonts loaded at the scaled pixel size (no blurry scaling). Spacing unit 8 px; corner radius 6 px (cards 8 px); icons 18 px stroke-drawn, 1.5 px lines, round caps.

## 4. Widget set (FROZEN at M1 — additions need a DECISIONS entry)

button · icon button (with tooltip) · toggle/eye · slider · **drag-number field** (drag to scrub w/ snap, click to type, Enter commit / Esc revert, unit suffix "u"/"°"/"px") · text field (rename) · tree row (icon, eye, label, swatch, hover actions) · color swatch + HSV popover picker · chip group (exclusive, e.g. 16/32/128/256/512) · tooltip (600 ms delay, shows shortcut) · toast (bottom-center, 3 s, max 3 stacked, may carry one action e.g. **Undo**) · floating card (title, body, ✓/✕ footer) · hint bar · modal confirm (rare — only recovery prompt & overwrite-on-export) · shortcut overlay (`?`) · **swatch row** (label + strip of its colours, `+` when the set is longer than the strip; added V-152 for the palette library).

Toolbar buttons show: icon + label + shortcut in tooltip; active tool = accent underline + tinted icon.

## 5. Default planes (R1)

Top (XZ, normal +Y), Front (XY, normal +Z), Right (YZ, normal +X). Rendered as 32×32 u bounded translucent quads centered at origin with name tag at a corner, only when visible — 32 so the edges land exactly on the sketch grid's 8 u major lines (V-129). They fade out as the camera zooms in past them (fully visible while the plane spans ≤ 1.5× the view height, gone past 3×, unpickable once nearly gone): a reference plane whose boundary is far outside the view is a translucent wall over the model, not a reference. A hovered or selected plane never fades. Tree rows have eye toggles; **no delete, no rename** (rows simply lack those affordances; Del on a selected plane → toast *"Default planes can't be deleted — you can hide them"*). Hover in tree ↔ glow in viewport (bidirectional). Clicking a plane in Idle mode = implicit "start sketch here?" affordance: single click selects, hint bar offers **Enter/click again to sketch**; double-click or `S` then click starts the sketch directly.

Planes auto-hide while a body is being painted (reduce clutter), restore after.

## 6. View cube & axis triad

### 6.1 View cube (top-right, 84 px + margin)
- Mini 3D cube rendered by the same rasterizer into its own layer, labeled faces: FRONT, BACK, TOP, BOTTOM, RIGHT, LEFT (11 px text, always upright).
- **26 hit zones** (6 faces, 12 edges, 8 corners) via its own ID buffer; hover = zone tint + cursor pointer.
- Click zone → camera animates (220 ms cubic ease) to that exact orientation; face views align to axes (and, being ortho by default, read like blueprints).
- **LMB-drag on the cube orbits** the main camera 1:1 (grab feel).
- Small ⌂ (home) icon button below → default isometric (azimuth 45°, elevation 30°) framing all visible bodies.
- Cube orientation always mirrors the live camera, including during animations.

### 6.2 Axis triad (bottom-right, 72 px)
Three colored arrows + X/Y/Z letters, orthographically projected with the live camera orientation, depth-sorted so rear arrows dim. Display-only in v1 (no click), never occludes model (drawn in overlay layer).

## 7. Tree panel (left) — "everything in the document" (R16)

Sections **Planes / Sketches / Bodies**, each collapsible, with counts (e.g. "Bodies · 3").

Row anatomy: `[eye] [type icon] [name] [color swatch (bodies)] […hover: rename ✎, delete 🗑]`.
- Eye toggles visibility (planes: hide only; sketches & bodies: hide/show). Hidden rows dim.
- Click row = select (viewport highlights + frames nothing; F frames). Shift-click range, Ctrl-click toggle. Double-click name = inline rename (bodies, sketches). Del = delete (bodies/sketches; undo-able, toast with Undo action — no modal).
- Body swatch click → color popover (affects unpainted faces).
- Sketch rows: double-click row (not name) = re-enter sketch editing (§8.8). Sketches consumed by an extrude get auto-hidden, not deleted.
- Bottom of panel: subtle stats line "3 bodies · 1,204 tris".
- Selection is bidirectional: viewport click selects tree row and vice versa; hover in tree pre-highlights in viewport.

## 8. Sketch mode (R2, R3)

> **Non-goal: no constraint or dimension solver.** Onshape's sketch power is
> its solver; this program's is the pixel grid. We take the tools, not the
> solver — snapping (§8.4), Shift constraints, and the grid step (§8.1) are the
> whole inference story. Anything that would need to *solve* for a position is
> out of scope by design (Sketch_func.md §2).


### 8.1 Entering
From Idle: `S` then click a plane/flat face, or double-click a plane, or tree-row double-click. On enter: camera animates to look squarely at the plane (normal-on, 220 ms; nearest cardinal up), rest of scene dims to 30% + non-target geometry becomes non-pickable, sketch grid fades in (minor every grid step, major every 8th line, both fade by zoom so ~≥8 px spacing; the grid's extent equals the plane's, so the plane is the sheet of graph paper you draw on, V-129), origin cross + U/V axis lines in soft X/Y colors. Only the plane being sketched on is drawn — the other two arrive edge-on as bands across the paper and are hidden; a face sketch shows none. The entry camera frames the whole sheet with a 10% margin. The grid step is a Grid chip row on the contextual card — 0.25 / 0.5 / 1 / 2 u, default 1, persisted in settings — and the snap grid follows it, so the drawn lines and the landing points are always the same lines (V-89). Toolbar swaps to sketch tools; contextual card shows sketch info (entity count, region count, open ends count).

### 8.2 Sketch toolbar
Tools are grouped, one button per group, showing the variant last used with a chevron opening the rest (Sketch_func.md §4.1). A group's key cycles its members, so the flyout is discovery and the key is speed:

- **Select** (V)
- **Line** (L) — Line · Midpoint line
- **Rectangle** (R) — Corner · Centre · Aligned
- **Circle** (C) — Centre circle · 3 point circle · Ellipse
- **Arc** (A) — 3 point arc · Tangent arc · Centre point arc
- **Polygon** (P) — Inscribed · Circumscribed
- **Slot** (O)
- **Spline** (S) - Spline / Bezier
- **Point** (.) — places a position to snap to; makes no segment and closes no region
- **Construction** (Q) — a mode, not a tool: with a selection it converts those entities, with none it arms whatever is drawn next

Below a measured width the group labels drop and the toolbar goes icon-only rather than clipping; tooltips and the `?` sheet carry the names. Delete (Del) works on the selection. Active tool highlighted; hint bar per tool:
- Line: *"Click to place points — click the first point or double-click to finish · Esc to cancel chain"*
- Rectangle: *"Click two corners"* · Circle: *"Click center, then radius — segments: 16 (edit in card)"*

### 8.3 Drawing behaviors
- **Line** draws a chain; clicking the chain's start point closes it (start point shows a snap ring + "close" glyph when hovered). Double-click ends without closing.
- **Rectangle** = 4 line entities grouped logically (stored as Rect, exploded to segments for regions; dragging a whole rect later moves all 4).
- **Circle** = regular N-gon (default 16 segments, editable 3–64 in the contextual card while the tool is active or for a selected circle). Radius live-snap to grid. **3 point circle** fits one through three clicks; three points in a line are refused with a reason.
- **Arc** = part of an n-gon, spending segments at the same density a whole circle would, so a quarter arc is as smooth as a quarter circle and no smoother. Three gestures build the same entity: **centre** (centre, start, sweep), **3 point** (start, end, a point on the way), **tangent** (a loose endpoint to continue from, then the far end — without an endpoint the tool refuses rather than inventing a direction). An arc's tessellation begins and ends *exactly* on the points it was built from, which is what lets lines drawn to those points close a region with it.
- **Spline** = a smooth curve *through* the points you click, Catmull-Rom, 8 subdivisions per span. Click the first point again to close it into a region, or double-click to finish it open. **Bezier** = one cubic from four clicks: the start, two handles, the end - pulled toward the handles without reaching them, with the cage previewed while they are placed.
- **Polygon** = regular n-gon, 3–24 sides, set by the card's Sides row while a polygon tool is armed or a polygon is selected. **Inscribed** measures the click to a corner; **circumscribed** measures it to the middle of a flat side, the way a nut is measured. Both store the same entity — the circumscribed radius is normalized to its inscribed equivalent, so there is one kind and no variant flag to carry through saving, selection and every later tool.
- **Slot** = a capsule: two clicks for the ends of the track, a third for how wide across. The width is measured perpendicular, so sliding the third click along the track does not change it. The rounded ends are what a router of that width would leave, 8 segments each.
- **Ellipse** = closed n-gon on two axes: centre, long axis, then how far across. The third click is measured perpendicular to the long axis, so anywhere along a parallel line gives the same oval.
- Live rubber-band preview with snapped endpoint; segment length + angle readout floats near cursor (11 px, dim) — *this is how users learn units*.

### 8.4 Snapping & inference (Alt suppresses all while held)
Priority: existing endpoint (7 px radius, strongest) → midpoint (5 px) → grid intersection (always available) → H/V inference. Snap target shows a glyph: ○ endpoint, ◇ midpoint, + grid. H/V inference: while placing, if within 4° of horizontal/vertical from previous point, snap the angle and draw a dashed guide line; small ⊥ glyph at right angles. On-face sketches also snap to the face's edges/vertices (§10).

### 8.5 Editing entities
Select tool: click entity (3 px pick tolerance via sketch ID layer) or drag box; drag entity/endpoint moves with snapping; Del removes. Moving/deleting re-runs the region engine live.

### 8.6 Regions & open ends (R3)
After every change the region engine (GEOM §4) recomputes:
- **Closed regions fill** with accentSoft; hovering a region raises fill to 30% + outline; regions are click-selectable (for extrude).
- **Open endpoints** (degree-1 nodes) render as 5 px **error-red rings**; contextual card shows "2 open ends" (clicking that message zooms to the nearest one). Extrude button disabled while selected regions is empty; tooltip explains: *"Select a closed region — close the red endpoints first"*.

### 8.10 Modifying what is drawn (SK5)
With the Select tool and a selection, the sketch card grows a **Modify** section. Each control acts on the selection and lands as one undo step; a button that does not apply is disabled with the reason rather than hidden, so the rows never move under the pointer.

- **Fillet** / **Chamfer** — select the two lines that meet at a corner, set a radius, press. A fillet is always the *minor* arc between its tangent points. A radius that does not fit is refused with the size that would.
- **Offset** — select one closed shape and give a distance, outward positive. Corners are mitred; an offset that would turn the shape inside out is refused rather than emitting a bowtie. The result is a closed spline, because a mitred offset of a polygon is no longer a regular polygon.
- **Mirror** — reflects the selection about the sketch's Vertical or Horizontal axis, *copying* rather than moving. This copies entities; live model symmetry is a separate mode.
- **Pattern** — Linear takes a count and a step; Circular takes a count and a sweep about the sketch origin. The count includes the original.

### 8.9 Construction geometry
`Q` marks entities as guides: things to snap to and measure against that are deliberately not part of the shape. Construction entities draw dashed and dimmed, are skipped when the sketch expands to segments — so they never close a region and never ring as an open end — and are still drawn, snapped to and selectable. With a selection, `Q` converts it (all-to-construction if any of it is still geometry, else back); with nothing selected, `Q` arms the mode so everything drawn next is a guide.

### 8.7 Leaving
✓ green button (or `E` with a region selected → straight into Extrude, the golden path) · ✕ / Esc exits keeping the sketch as drawn (sketches are never lost by exiting). Camera animates back to prior view unless extrude continues.

### 8.8 Re-editing consumed sketches (pseudo-parametric honesty)
Re-entering a sketch that fed an extrude shows a persistent dim banner in the contextual card: *"Editing this sketch won't change existing bodies. Extrude again to create new geometry."* No silent surprises.

## 9. Extrude (R4–R8)

### 9.1 Entry
In Sketch with ≥1 region selected: `E` / toolbar Extrude / ✓-then-Extrude. Also from Idle: select a sketch in tree with closed regions → Extrude picks its regions (all preselected). Camera pulls back slightly (animated) to show depth developing if it was normal-on.

### 9.2 The arrow drag (R5)
- Preview body appears immediately at depth 1 u, 55% alpha, in the pending body color.
- **Arrow gizmo** at region centroid along the sketch-plane normal: shaft + cone, screen-constant size (~90 px), accent-colored, brightens on hover, cursor changes to ↕ along-axis.
- LMB-drag moves depth along the normal with **grid snap 1 u** (Ctrl: ¼ u fine; Alt: free). Dragging through zero flips direction (arrow flips, preview flips). Depth readout rides the arrow tip; the card's drag-number field stays in sync (type exact values, arrows step 1 u).
- The preview + gizmo also work from the pulled-back 3D view (not just normal-on).
- The palette is a **right-hand sidebar**, not a floating card (V-151): real chrome like the tree, so the viewport ends where it begins and nothing you are painting hides under it. It slides open on entering paint mode (170 ms) and shut on leaving (120 ms), smoothstepped; while it moves, its contents are laid out at their final width and clipped, so the panel arrives as one piece rather than reflowing. A **Stop painting** button is pinned to its bottom edge — always in the same place however tall the tool's controls grow — and leaves paint mode (shortcut P; Esc backs out one level at a time and only reaches the mode last). Headless runs snap it open instantly: a golden captures a state, never a transition.
- Add, Subtract and Intersect preview the **result** (V-150): each target body is drawn as it would stand after the commit — the same boolean the commit runs — and the pending solid rides along as a ghost, red for material leaving (drawn x-ray, through the body, because removed material lives inside it), accent for material joining. The result body is undimmed; the active sketch's region fills stand down so they do not cover the opening. A body the cut would take entirely draws as nothing. Rebuilt only when a parameter actually changes (the drag snaps, so most frames change nothing). A boolean that fails says so in the card and disables Extrude before Enter, not after.

### 9.3 Options card (floating, right)
```
Extrude ────────────────────────
Depth      [ 6 u   ] (drag/type)
Direction  (• Normal)(Reverse)(Symmetric)
Draft      [ 0 °   ] slider −45…45
Result     (• New)(Add)(Subtract)(Intersect)
           Through all  [toggle]
                      [✕ Cancel] [✓ Extrude]
```
- **Symmetric** (R7): total depth split ± around the sketch plane; draft applies outward from the plane both ways (widest at the sketch plane).
- **Draft** (R6): degrees; positive tapers the far cap smaller. Live preview. If the requested angle would break the profile (GEOM §5.3), the field clamps to the max valid value and turns warn-orange with tooltip *"Clamped — profile too tight for more draft"*.
- **Through all**: replaces depth with "past everything" (scene bbox + margin); pairs naturally with Subtract for cutting windows. **When the sketch plane has material on both sides of it, turning Through all on selects Symmetric**, because past everything from a plane inside the model means both ways — the three default planes all pass through the origin, so a one-directional cut there starts inside the material and leaves a blind pocket where a hole was asked for. Direction stays free to change afterwards, and the toggle says which it resolved to ("both ways" / "one way only").
- **Result** (R8): **New** = independent body. **Add** = union into target. **Subtract** = cut from target(s). **Intersect** = keep common volume with target. Target rules: sketch-on-face → that face's body; sketch-on-plane → the body the preview intersects (topmost if several for Add/Intersect; **all intersected** for Subtract). If Add/Intersect finds no target: falls back to New with toast *"Nothing to combine with — created a new body"*. Chips disable with tooltip when no target applies.
- Confirm: ✓ or Enter. Cancel: ✕ or Esc (sketch untouched, still in Sketch mode).

### 9.4 Semantics note for M4 wiring
Subtract with "through all" must cut every intersected body. Add unions and keeps the **target's** name/color/paint; tool bodies' paint on surviving fragments is preserved via srcFace lineage.

### 9.5 After confirm
CSG runs (async spinner overlay if >120 ms, Esc cancels → nothing changed); new/updated body selected + counted in tree ("Body 3"); source sketch auto-hides with toast *"Sketch 2 hidden — find it in the tree"*; mode returns to Idle; camera stays put. Single undo step reverts the whole extrude.

## 10. Sketch on a face (R9)

- In Idle, hovering a **flat** face shows a subtle pencil-able affordance (face pre-highlight + cursor hint); click selects it; hint bar: *"S to sketch on this face · drag arrow to push/pull"*.
- `S` (or toolbar Sketch → click face): sketch plane = face plane with a **persistent frame snapshot**; camera normal-on; the face's boundary renders as reference geometry (dim solid lines) whose vertices/edges are snap targets; **"Project outline"** button in the card copies the boundary into real sketch entities (for offset hulls / stepped armor).
- Non-flat faces (flagged non-planar after vertex edits) refuse with the §1 invalid-click feedback.
- Extruding from a face sketch defaults **Result=Add**, target = owning body; choosing Subtract + dragging into the body is the "cut a hole here" path. Both directions preview correctly.

## 11. Boolean tool (R12)

- Toolbar Boolean / `B`. Card: op chips **Union / Subtract / Intersect**, "Keep original bodies" toggle (off), then guided picking driven by the hint bar:
  - Union: *"Click bodies to merge (2+), Enter to apply"* — result keeps first body's name/color.
  - Subtract: *"Click the body to keep"* → *"Click bodies to remove (tools)"* → Enter.
  - Intersect: *"Click two or more bodies — the overlap survives"* → Enter.
- Picked bodies tint (target accent, tools error-red at 30%); click again to unpick; live result preview when ≤2 bodies and geometry is small (else preview on Enter with spinner).
- Success: result selected, toast summarizing (*"Union: Body 1 + Body 2 → Body 1"*). Tools consumed unless Keep on.

### 11.3 Failure UX (Robust pillar)
If CSG fails validation: **document untouched**, error toast *"Boolean failed — nothing was changed. This shape combo hit a solver edge; try nudging one body 1 subunit."* with a **Report** action that dumps repro geometry to `debug/csg/` (dev flag writes automatically). Never partial results, never corrupt meshes.

## 12. Selection, direct editing & Transform (R10, R11)

### 12.1 Selection model (Idle)
Pick priority within cursor radius: **vertex (9 px) > edge (5 px) > face > body-through-tree**. Click face selects face; double-click face selects its body; click empty deselects. Shift adds, Ctrl toggles. **Box select**: LMB-drag on empty space → rect; selects verts/edges/faces fully inside (chips in the card switch the box-select filter: Verts/Edges/Faces/Bodies, default Verts — the ship-stretching workflow).
Highlights: hover face = 8% white + edge glow; selected face = accentSoft fill + accent edges; selected body = accent silhouette outline (2 px); selected edges accent 2 px; verts 5 px squares (hover ring). All via overlay pass, always visible through other geometry at 20% ghost.

### 12.2 Move gizmo
Appears at selection centroid: 3 axis arrows (world axes, axis-colored), 3 planar squares, center dot (screen-plane move). Grid snap 1 u (Ctrl ¼ u, Alt free). Live numeric offset readout; card shows ΔX/ΔY/ΔZ drag-number fields (typing moves precisely). Single-face selection leads with its **normal arrow** enlarged (see push/pull §12.5). Drag = one undo step.

### 12.3 Direct edits (R10)
Moving verts/edges/faces edits the mesh in place (GEOM §7): edges move their 2 verts, faces their loop verts. If a planar face would go non-planar → it auto-triangulates for rendering, keeps logical identity, and shows a tiny warn chip on hover (*"bent face"*); sketching/push-pull on bent faces is refused with guidance. Paint stays anchored (frame unchanged).

### 12.4 Rotate & body ops (R11)
With a body (or sub-selection) selected, tab in card switches gizmo Move ⇄ Rotate: 3 rings, **detents at 90°** (Shift 15°, Alt free) around the selection pivot; angle readout; 90° rotations are exact/grid-preserving. Ctrl+D duplicates a body (offset +1 u X, selected, name "Body N"). Del deletes with undo toast.

### 12.5 Push/pull (the hero tool)
Face selected → its normal arrow drag **re-extrudes the face**: drag out = Add material, drag in = Subtract, auto-chosen by direction, grid-snapped, full preview, CSG on release only. Hint bar teaches it the first 3 times a face is selected (*"Tip: drag the arrow to push/pull this face"*; counter in settings). This plus face-sketching is 90% of blocky ship modeling.

## 13. Paint mode (R13)

### 13.1 Entering & panel
`P` / toolbar Paint. Planes auto-hide; palette panel docks right:
```
Paint ──────────────────────────
Tools  [✏ Pencil][◻ Eraser][▨ Fill][💧 Pick]
Size   (1)(2)(4)
Res    (1)(2)(4)(8)(16)(32)      texels per unit
Palette  [32 swatches, 8×4]
Alpha    [------o---]  43%        how much of the colour a stroke lays down
         [+ custom] [recents ×8]
[Import .hex]      [Textures 👁]
```
### 13.2 Painting
- Hovering any face of any visible body shows the **texel cursor**: the exact texel(s) under the brush outlined crisply on the 3D surface (projected quad outlines) — you always know which pixel you're about to hit.
- LMB paints (drag = stroke, straight-line interpolation between samples in UV space); Eraser restores base body color (alpha 0 in texture); Fill flood-fills within the face's texture (4-connected, matching color); Alt or Pick tool = eyedropper (samples composited color incl. body color).
- First stroke on an unpainted face allocates its texture at the selected **Res** chip (density from face bbox at that moment — GEOM §8.2). Painting a face whose texture has a different res → inline prompt in the card: *"This face is 32 px — switch brush to 32, or resample face to 128?"* (two buttons; resample = nearest).
- If the view angle to the face is >70° oblique, a hint chip offers **Face view** (camera normal-on animation). Manual button too. Painting still allowed at any angle.
- Textures render nearest-neighbor always (Crisp pillar). "Textures 👁" toggles paint visibility (inspect bare geometry).
- Strokes are undo steps (coalesced per mouse-down); palette edits are not document state (app settings).

### 13.2a Alpha (V-158)

The **Alpha** slider sits between the palette grid and the recents, and runs
1..255 shown as a percentage. It is a property of the brush, like the size,
not of the colour: choosing a swatch never changes it. Below full, a dab
composites source-over onto whatever is already on the texel rather than
replacing it, and its coverage rides on the same alpha — half a dab of
half-transparent paint is a quarter laid down. Dithering spends the dab's *coverage* on
whole texels, as it always did, and never the alpha: those are different
questions, and a hard dab at half alpha is an even glaze under every dither
mode rather than paint on every other texel.

A stroke lays its alpha once. Overlapping dabs within one stroke accumulate to
the alpha you asked for and no further, so a translucent line is even along its
own joins; the same holds for two chosen edges meeting at a corner. A gradient
takes the brush's alpha at both ends. The slider cannot reach 0 — that is
not faint paint but no paint, which is the eraser.

The two armed chips draw over a transparency checkerboard, and the texel
cursor's colour preview fades with the brush, so what you are about to lay
down is visible before you lay it. The palette grid stays opaque: it holds
hues. The eyedropper reads a texel's alpha back into the brush.

Exports flatten: a face's picture is composited over the body's own colour on
the way into a `.gltf`, `.glb` or `.obj`, because those materials have nothing
to blend a transparent texel with and would draw it black. What the file shows
is what the viewport showed.

### 13.3 Palette
Default: embedded original 32-color palette tuned for spaceship greys/hull/accent/glow ramps (author in code, document in README). Custom colors via HSV popover; recents auto-track 8. **Import .hex** (Lospec format: one RRGGBB per line) via file dialog, replaces "custom" page, never the built-in page.

### 13.3a Palette library (V-152)

**Palettes…** in the paint sidebar opens the library: a search box over one scrolling list of every palette on offer — the ~4,400 bundled with the program, plus anything in `%APPDATA%\Modeler\palettes\` (`.hex` or `.txt`, one `#RRGGBB` per line), which is listed first. Each row is the palette's name beside a strip of its colours, with a `+` when it holds more than the strip shows. Search matches every word in any order. The wheel scrolls by whole rows; the scrollbar is a position readout, not a control. Clicking a row puts it on the custom page — never over the built-in one — and the page's chip takes the palette's name. The choice is remembered between sessions. Esc or **Close** dismisses the list; **Import .hex…** inside it is the old file dialog, for a palette that is not in the collection.

Names are folded to glyphs the font atlas can draw before they are shown (D-11): near twins map across, anything else is dropped, and a name left empty becomes "Untitled palette".

### 13.4 Shapes, the soft brush and gradients (added 2026-08-26 at the user's request)

Four two-point tools and one more freehand one. A two-point tool is decided by
where the press landed and where the pointer is now, so it rubber-bands: the
shape you let go of is the shape you were looking at, and nothing it passed
through on the way is left behind.

```
Tools  [pencil][brush][eraser][fill][pick]
       [line][rect][circle][gradient]
Size   (1)(2)(4)(8)(16)
[Fill the shape]                 · rect and circle only
Dither (None)(2x2)(4x4)(8x8)     · brush and gradient only
```

- **Line** — drag from one texel to another. Shift keeps it horizontal, vertical
  or at 45°.
- **Rect** / **Circle** — drag the box they are inscribed in; Shift makes it a
  square or a circle. **Fill the shape** switches between a solid and an
  outline; the outline is drawn with the brush, so size widens it.
- **Soft brush** — a round dab that fades out toward its rim, as against the
  pencil's hard square. The fade blends into whatever is under the texel — the
  paint already there, or the body's own colour where there is none — and the
  result is stored fully opaque. Sizes 8 and 16 exist for this tool: at four
  texels across there is nowhere for a falloff to happen.
- **Gradient** — drag to set the direction and the distance; the ramp runs from
  the near colour to the far one, perpendicular to the drag, across the whole
  face. Past either end it holds the colour it arrived at.

**Two armed colours.** The panel shows a near and a far swatch with a swap
button (**X**). Clicking a swatch points the palette, the eyedropper and the HSV
mixer at it; clicking the armed one again opens the mixer. Every tool but the
gradient uses the near colour.

**Dither modes** are ordered (Bayer) matrices of order 2, 4 and 8, and they
decide how a coverage between nothing and everything is spent:

- **None** blends — a gradient ramps through real colours, a soft edge fades
  through them. Smooth, and off the palette.
- **2x2 / 4x4 / 8x8** spend it on *how many* texels are painted instead, so a
  ramp between two palette colours stays two palette colours and a soft edge
  stays one. This is the pixel-native mode and the reason the feature exists;
  the larger the matrix the finer the gradation it can express.

Thresholds are indexed by texel position in the face's own grid, never by
position within the shape, so two passes over the same area line up instead of
seaming.

**Shortcuts** (mode-local, as sketch mode's are): D pencil · B brush · E eraser ·
G fill · I pick · L line · R rect · C circle · N gradient · X swap colours.

### 13.5 Face lock (added 2026-08-26 at the user's request)

**Lock to a face…** arms the pick; the click that follows chooses the face,
points the camera squarely at it and confines every stroke to it until you
unlock. It exists because the brush has the widest hit area in the program: run
the pointer over an edge while painting a hull side and the next dab lands on
the neighbouring face.

- **Armed first, picked second**, the way pressing S with no plane selected
  waits for one (§8.1). A button that acted on the face already under the
  pointer could not be reached — moving the pointer to the button is exactly
  what takes it off the face — so it is live whatever the pointer is doing. The
  button reads **Click a face…** while armed; pressing it again, or Esc,
  cancels. The click is spent on the choice and paints nothing.
- The camera frames the face itself rather than a sphere around it, and offsets
  it clear of the palette panel, so a hull side fills the space it is worked in.
- While locked the panel names the face and offers **Recentre** (point the
  camera back at it) and **Unlock**. **Esc** unlocks before it leaves the mode.
- **The lock is on painting, not on the camera.** Orbit, pan and zoom work
  exactly as they do everywhere else (§1) — checking your work from an angle is
  part of painting. What is fixed is where the paint can land.
- The cursor is resolved against the locked face's own plane instead of the ID
  pass, so a body drifting in front of it cannot steal a stroke, and the pointer
  running off the face simply shows no cursor.
- If the face is cut away by a later boolean, the lock releases with a toast.

### 13.6 Edge lines
The **Edge** tool (K) paints a band along edges you pick rather than where the pointer goes. Click edges to add or remove them from the selection — they highlight in the colour they will be painted — or press **All corners** to take every sharp edge of the body at once. The width chips set the band in texels *on each face*; press **Paint** and it is baked into the faces the edges meet.

Baked means baked: it is ordinary paint from that moment, in the faces own pictures. It saves, exports, survives booleans and can be painted over. The band is laid inside each face rather than across the boundary, so the two halves meet at the corner and read as one line. Esc clears the picked edges before it leaves the tool. One press is one undo, however many edges it covered.

## 14. Welcome & empty states

- First launch / Ctrl+N with nothing: viewport shows dim center card — **New ship** (starts empty doc + pulses the Sketch button subtly), **Open…**, **Sample ship** (loads embedded op-script-built model), recent files list. Dismisses on any action.
- Empty document hint bar: *"Click a plane (or press S) to start your first sketch"*.
- Deleted-everything state returns to that hint, not to a blank stare.

## 15. Polish checklist (M9 work list; user sign-off list)

- [ ] Every interactive element: distinct hover, pressed, disabled(+tooltip why) states
- [ ] Cursor set: default, crosshair (sketch), along-axis ↕ (gizmo hover), grab (cube), pencil (paint), not-allowed (invalid)
- [ ] All camera moves animated (220 ms cubic), interruptible by input; no snaps
- [ ] Tooltips everywhere incl. shortcut labels; `?` opens shortcut overlay
- [ ] Toasts for every completed op with meaningful copy; Undo action where applicable
- [ ] No dead-end states; every disabled control's tooltip says how to enable it
- [ ] Hint bar copy pass (a first-timer can build the sample ship without docs)
- [ ] 60 fps orbit with sample ship + paint on this machine; no GC hitches >4 ms (profile)
- [ ] Resize/DPI: layout reflows, framebuffer resizes, crispness at 1.25/1.5/2.0 scales
- [ ] Undo/redo audit: every mutating action, incl. paint, visibility, rename, color
- [ ] Sample ship ships embedded; README quickstart with screenshots (from headless renders)

## 16. Keyboard & mouse reference (final map)

| Input | Action |
|---|---|
| RMB / MMB / Wheel | Orbit / Pan / Zoom |
| LMB | Mode action (select, draw, drag, paint) |
| S / E / B / M / P | Sketch / Extrude / Boolean / Move-Transform / Paint |
| V, L, R, C | Sketch: Select, Line, Rect, Circle |
| O | Ortho ⇄ Perspective |
| F | Frame selection (or all) |
| Esc | Cancel/back one level (§1 stack) |
| Enter | Confirm current card (✓) |
| Del | Delete selection |
| Ctrl+Z / Ctrl+Y or Ctrl+Shift+Z | Undo / Redo |
| Ctrl+N / O / S / Shift+S | New / Open / Save / Save As |
| Ctrl+E / Ctrl+I | Export dialog / import an STL or OBJ mesh |
| Ctrl+D | Duplicate body |
| H | Hide selection (eye back on in tree) |
| Alt (held) | Suppress snapping / eyedropper in Paint |
| Ctrl (held) | Fine snap ¼ u |
| Shift (held) | Add to selection; 15° rotate detents |
| ? | Shortcut overlay |

Hold-key behaviors must work with either L/R modifier key.
