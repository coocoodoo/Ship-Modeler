# SPEC-UX — Product, Interaction & Visual Specification

The pillars (PLAN §1) rank Easy > Polished > Crisp > Robust-feel. Every flow below is written so a first-time user succeeds without documentation: the **hint bar always says what to do next**, and **everything pickable pre-highlights on hover**.

## 1. Modes & input model

The app is a state machine of modes; exactly one active:
`Idle` (select/navigate) · `Sketch` · `Extrude` (transient) · `Boolean` (transient) · `Transform` (implicit when dragging gizmos in Idle) · `Paint`.

Camera navigation works identically in **every** mode:
- **RMB-drag** orbit (turntable, world-up locked, no roll)
- **MMB-drag** or **Shift+RMB** pan
- **Wheel** zoom to cursor (ortho: scale; persp: dolly), smoothed ~120 ms
- **F** frame selection (or all if none); **O** toggle ortho/perspective
- View cube & triad always visible (§6)

**LMB is the "do" button** and is mode-owned: select in Idle, draw in Sketch, drag gizmos in Extrude/Transform, paint in Paint. LMB never orbits.

**Esc stack** (one level per press): cancel current drag → cancel pending tool input (e.g., half-placed line chain) → exit transient tool (Extrude/Boolean, discarding preview) → exit mode (Sketch/Paint, keeping work) → clear selection.

Every tool activation sets the hint bar; every invalid click gives feedback (subtle shake of hint text + specific message, e.g. *"That face isn't flat — pick a flat face to sketch on"*). Silence is never the answer.

## 2. Window layout

```
┌──────────────────────────────────────────────────────────────────────────┐
│ TOOLBAR  [Sketch S][Extrude E][Boolean B][Move M][Paint P] │ [↶][↷] │ ⚙ │
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

## 3. Theme (dark, v1's only theme; tokens centralized in `ui/theme.go`)

| Token | Value | Use |
|---|---|---|
| bg | #16181D | window bg |
| panel | #1E2128 | toolbar/tree/hint |
| card | #262A33 | floating cards, fields |
| stroke | #343945 | hairlines, borders |
| text | #E8EAF0 | primary text |
| textDim | #9AA3B2 | secondary, hints |
| accent | #4C9AFF | selection, active tool, links |
| accentSoft | #4C9AFF33 | region fills, soft highlights |
| warn | #FFB454 | clamped draft, non-planar chip |
| error | #FF5D5D | open ends, failed ops |
| success | #3DD68C | confirm ✓, valid states |
| viewportBg | vertical gradient #1A1D23 → #22262E | 3D background |
| gridMinor / gridMajor | #FFFFFF0F / #FFFFFF24 | sketch grid |
| edgeLine | body color × 0.35, α 0.85 | mesh crease/silhouette edges |
| hover | #FFFFFF14 overlay | any hoverable |

Axis colors: X #E5484D, Y #46A758, Z #3E63DD (triad, gizmo arrows, plane tints: Right=YZ→X-tint, Top=XZ→Y-tint, Front=XY→Z-tint at 10% alpha + 30% alpha border + small label).

Body auto-color cycle (8): #8EA3B0, #B08E8E, #8EB09B, #A38EB0, #B0A98E, #8E9BB0, #B08EA6, #96B08E (desaturated so paint reads on top).

Type: embedded Go Regular — 13 px UI, 15 px section headers, 11 px hints/badges; line height 1.4; UI scale follows the OS display scale (raylib `GetWindowScaleDPI`) rounded to 1.0/1.25/1.5/2.0, fonts loaded at the scaled pixel size (no blurry scaling). Spacing unit 8 px; corner radius 6 px (cards 8 px); icons 18 px stroke-drawn, 1.5 px lines, round caps.

## 4. Widget set (FROZEN at M1 — additions need a DECISIONS entry)

button · icon button (with tooltip) · toggle/eye · slider · **drag-number field** (drag to scrub w/ snap, click to type, Enter commit / Esc revert, unit suffix "u"/"°"/"px") · text field (rename) · tree row (icon, eye, label, swatch, hover actions) · color swatch + HSV popover picker · chip group (exclusive, e.g. 16/32/128/256/512) · tooltip (600 ms delay, shows shortcut) · toast (bottom-center, 3 s, max 3 stacked, may carry one action e.g. **Undo**) · floating card (title, body, ✓/✕ footer) · hint bar · modal confirm (rare — only recovery prompt & overwrite-on-export) · shortcut overlay (`?`).

Toolbar buttons show: icon + label + shortcut in tooltip; active tool = accent underline + tinted icon.

## 5. Default planes (R1)

Top (XZ, normal +Y), Front (XY, normal +Z), Right (YZ, normal +X). Rendered as 24×24 u bounded translucent quads centered at origin with name tag at a corner, only when visible. Tree rows have eye toggles; **no delete, no rename** (rows simply lack those affordances; Del on a selected plane → toast *"Default planes can't be deleted — you can hide them"*). Hover in tree ↔ glow in viewport (bidirectional). Clicking a plane in Idle mode = implicit "start sketch here?" affordance: single click selects, hint bar offers **Enter/click again to sketch**; double-click or `S` then click starts the sketch directly.

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

### 8.1 Entering
From Idle: `S` then click a plane/flat face, or double-click a plane, or tree-row double-click. On enter: camera animates to look squarely at the plane (normal-on, 220 ms; nearest cardinal up), rest of scene dims to 30% + non-target geometry becomes non-pickable, sketch grid fades in (minor 1 u, major 8 u, both fade by zoom so ~≥8 px spacing), origin cross + U/V axis lines in soft X/Y colors. Toolbar swaps to sketch tools; contextual card shows sketch info (entity count, region count, open ends count).

### 8.2 Sketch toolbar
Select (V) · Line (L) · Rectangle (R) · Circle (C) · Delete (Del works on selection too). Active tool highlighted; hint bar per tool:
- Line: *"Click to place points — click the first point or double-click to finish · Esc to cancel chain"*
- Rectangle: *"Click two corners"* · Circle: *"Click center, then radius — segments: 16 (edit in card)"*

### 8.3 Drawing behaviors
- **Line** draws a chain; clicking the chain's start point closes it (start point shows a snap ring + "close" glyph when hovered). Double-click ends without closing.
- **Rectangle** = 4 line entities grouped logically (stored as Rect, exploded to segments for regions; dragging a whole rect later moves all 4).
- **Circle** = regular N-gon (default 16 segments, editable 3–64 in the contextual card while the tool is active or for a selected circle). Radius live-snap to grid.
- Live rubber-band preview with snapped endpoint; segment length + angle readout floats near cursor (11 px, dim) — *this is how users learn units*.

### 8.4 Snapping & inference (Alt suppresses all while held)
Priority: existing endpoint (7 px radius, strongest) → midpoint (5 px) → grid intersection (always available) → H/V inference. Snap target shows a glyph: ○ endpoint, ◇ midpoint, + grid. H/V inference: while placing, if within 4° of horizontal/vertical from previous point, snap the angle and draw a dashed guide line; small ⊥ glyph at right angles. On-face sketches also snap to the face's edges/vertices (§10).

### 8.5 Editing entities
Select tool: click entity (3 px pick tolerance via sketch ID layer) or drag box; drag entity/endpoint moves with snapping; Del removes. Moving/deleting re-runs the region engine live.

### 8.6 Regions & open ends (R3)
After every change the region engine (GEOM §4) recomputes:
- **Closed regions fill** with accentSoft; hovering a region raises fill to 30% + outline; regions are click-selectable (for extrude).
- **Open endpoints** (degree-1 nodes) render as 5 px **error-red rings**; contextual card shows "2 open ends" (clicking that message zooms to the nearest one). Extrude button disabled while selected regions is empty; tooltip explains: *"Select a closed region — close the red endpoints first"*.

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
- **Through all**: replaces depth with "past everything" (scene bbox + margin); pairs naturally with Subtract for cutting windows.
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
Res    (16)(32)(128)(256)(512)
Palette  [32 swatches, 8×4]
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

### 13.3 Palette
Default: embedded original 32-color palette tuned for spaceship greys/hull/accent/glow ramps (author in code, document in README). Custom colors via HSV popover; recents auto-track 8. **Import .hex** (Lospec format: one RRGGBB per line) via file dialog, replaces "custom" page, never the built-in page.

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
| Ctrl+E | Export dialog |
| Ctrl+D | Duplicate body |
| H | Hide selection (eye back on in tree) |
| Alt (held) | Suppress snapping / eyedropper in Paint |
| Ctrl (held) | Fine snap ¼ u |
| Shift (held) | Add to selection; 15° rotate detents |
| ? | Shortcut overlay |

Hold-key behaviors must work with either L/R modifier key.
