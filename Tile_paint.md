# Tile_paint.md — Tile stamping in paint mode

> The plan for a tile mode in paint: import a tileset image, adjust how it
> slices into tiles, pick a tile, and stamp it onto the model's faces —
> snapped to a tile grid so stamps butt together seamlessly. Requested by the
> user 2026-08-28 ("can you add a tile mode where i can import a tile set,
> adjust the tile pattern, i select the tile and stamp it to the 3D model?").
>
> **Audience: the executor model.** Written to be followed without
> re-deriving anything; where a decision could go two ways it has been made
> here, and deviating takes a DECISIONS entry saying why. Everything in
> *PLAN.md §Milestone protocol* applies: tests before code, one PROGRESS
> entry per session, commit per milestone (prefix `TP<N>:`), verify
> headlessly, keep the app runnable. Pushing to GitHub after each milestone
> is the owner's standing instruction (V-131).

---

## 0. How to read this plan

Milestones are **TP1–TP3** in dependency order, each independently
shippable; **TP4** is stretch and needs the user's go-ahead. "Adjust the
tile pattern" is confirmed by the user (2026-08-28) to mean **the tile
grid that separates the sheet into tiles**, offered as preset chips —
**8x8, 16x16, 32x32, 64x64, or Custom** — the same chip idiom as the Res
and Size rows. Presets are square with margin 0 and spacing 0; Custom
reveals the W, H, Margin and Spacing fields for sheets that need them
(Tiled-style gutters). Rotate/flip of the stamp stays in scope as its own
small thing — pixel-art stamping wants it regardless.

Before writing any code, re-read: `internal/paint/mapping.go` (texel space,
`FaceRect`, `At`/`Set`, growth), `internal/paint/command.go` (the stroke
command's dirty-rect undo — the stamp command mirrors it),
`internal/app/paintmode.go` (`paintState`, the per-tool panel sections,
`sticky` hover), `internal/app/edgepaint.go` §buildEdgeSection (the pattern
for a tool-specific panel section), `internal/io/settings.go`
(`PaintSettings`), and the traps in §2.

---

## 1. The architecture being extended (verified 2026-08-28)

- **Texel space is the whole game.** A face's `mesh.FacePaint` is an RGBA
  image anchored to the face by a frame; `paint.Texel(p, world)` maps a
  cursor hit to an integer texel, `paint.Set` writes one, images grow on
  demand to a 1024² cap, and the renderer shows texels clipped to the face
  polygon. A stamp is nothing but a batch of `Set` calls at an offset — all
  of the hard mapping work already exists.
- **Resolution is texels per unit** (V-128): the res chip fixes physical
  pixel size document-wide. Therefore **1 tile pixel = 1 texel, never
  scaled.** A 16 px tile at 8 px/u spans exactly 2 u. Stamping onto a face
  allocated at a different res uses that face's own texels — same rule as
  every brush (V-126) — and the existing res-mismatch prompt already covers
  the "this face is coarser" conversation.
- **Undo for paint is dirty-rect snapshots** (SPEC-DATA §3.3): a command
  holds before/after sub-images of the rect it touched, coalesced per
  mouse-down, under the 40 MB paint-undo cap. The stamp command copies this
  shape exactly (see `StrokeEdges` for the multi-face variant; a stamp
  touches one face).
- **Palette-adjacent state is settings, not document** (SPEC-DATA §3.3):
  picking a colour is not an undo step, and the palette lives in
  `io.Settings`. The tileset is the same kind of thing: the *pixels a stamp
  wrote* are document state (baked into FacePaint, saved in `.ship`/`.pxm`
  automatically, nothing new to persist); the *sheet, its slicing and the
  selected tile* are settings. A file opened on a machine without the sheet
  shows every stamped pixel; only re-stamping needs the sheet re-imported.
- **The paint package is raylib-free.** Sheet loading and slicing use
  `image/png` from the standard library. Only the app layer may upload the
  sheet as a GPU texture for the panel picker.
- **The UI kit draws no images today.** The picker is the one genuinely new
  UI capability: drawing a texture region in the panel, nearest-filtered.
  The app layer already imports raylib; keep the texture handling there
  (upload on import, unload on replace), not in `internal/ui`.
- **Glyphs**: panel labels are ASCII-only (`TestEveryMessageIsRenderable`
  sweeps every script); the Tile tool icon is a stroke icon in
  `internal/ui/icons.go` (D-11). No `⟳` or `⇄` — the rotate/flip buttons
  use stroke icons too.
- **Dialogs** are zenity (D-10), like Import .hex — and headless scripts
  never open one: the import op takes a path.

## 2. Traps

- **Margin/spacing off-by-ones.** Tiled's convention: `margin` is the border
  around the whole sheet, `spacing` the gutter between tiles. Column count is
  `(sheetW - 2*margin + spacing) / (tileW + spacing)`, and the last partial
  column is dropped, not stretched. Table-test this against hand-computed
  sheets before anything draws.
- **The D4 group, not "rot then maybe flip sometimes".** Rotate and flip
  compose; store orientation as `rot 0..3` + `flipX bool` with one
  documented application order (flip first, then rotate) and test all eight
  against hand-rotated 2×2 pixel tiles. Ad-hoc composition is how stamps end
  up mirrored on the fourth click.
- **Stamp clipping is to `FaceRect`, painting outside the face polygon but
  inside the rect is fine** — the renderer clips (V-135 proved this for edge
  bands). Do NOT try to polygon-clip the stamp; a tile overhanging a slanted
  boundary shows exactly its on-face part, which is correct.
- **The preview ghost is per-frame overlay geometry.** A 64×64 tile is 4096
  translucent quads. Cap tile size at 64×64 in v1 and build the ghost only
  when the hovered cell or tile changes is unnecessary — but if profiling
  shows a cost, coarsen the ghost (outline + corner pixels), don't drop it.
- **Texture leaks in the panel.** Re-importing a sheet must unload the old
  GPU texture; app shutdown already unloads via `Close` — add the sheet
  texture to that path.
- **`paint.pixel`-style ops act at face texels, not window pixels** — keep
  `tile.stamp` in the same vocabulary (body, face index, uv texel of the
  cell's min corner before snapping) so flow tests are deterministic.

## 3. Product decisions (made here, once)

1. **Snap by default, Alt for free placement.** Stamps snap to a grid of
   tile-sized cells anchored at the face's texel origin (0,0) — that is what
   makes adjacent stamps butt seamlessly. Alt suppresses snapping, exactly
   as it suppresses snapping everywhere else in the program (SPEC-UX §8.4).
2. **Transparent tile pixels leave the surface alone.** Alpha ≥ 128 paints
   the pixel opaque (A=255, like every brush); below leaves the existing
   texel untouched. A stamp never erases — the eraser erases.
3. **One sheet loaded at a time.** Importing replaces it. The sheet PNG is
   copied into the app config dir (`tilesets/<name>.png`) so the original
   can move or vanish; settings remember the copied path, slicing, selected
   tile and orientation. Sheet cap 1024×1024, tile cap 64×64; over-cap
   imports are refused with a toast that names the limits.
4. **Drag stamps a trail.** Press stamps the cell under the cursor; dragging
   stamps each new cell the pointer enters (last-cell tracking, no
   duplicates); the whole drag is one undo step, like a stroke.
5. **Stamping an unpainted face allocates its paint at the current res
   chip**, exactly like a first brush stroke.
6. **Face lock applies** — a locked stamp clips to the locked face; without
   the lock it clips to the face under the cursor. No cross-face wrapping
   (non-goal below).
7. **Tile selection is one tile in v1.** Multi-tile rectangular selection
   (stamp a 2×2 block from the sheet in one press) is TP4, behind the
   user's go-ahead — it doubles the picker's interaction surface.
8. **Keyboard: `T` arms the Tile tool** if free in `paintToolKeys`
   (executor verifies; next free letter otherwise, documented in the `?`
   sheet and README key table like SK5 did).

**Non-goals (v1):** cross-face wrap of a stamp; auto-tiling (Wang/blob);
multiple simultaneous sheets; tile animation; palette-remapping of tiles.

## 4. Data model

```go
// internal/paint/tileset.go — raylib-free
type Tileset struct {
    Img            *image.RGBA // the sheet, decoded once
    TileW, TileH   int         // in pixels == texels
    Margin, Spacing int
}
func (t *Tileset) Cols() int / Rows() int / Count() int
func (t *Tileset) TileRect(i int) image.Rectangle   // sheet coords
type Orientation struct{ Rot uint8; FlipX bool }    // D4, flip-then-rotate
func TilePixels(t *Tileset, i int, o Orientation) *image.RGBA // w×h copy

// internal/paint/stamp.go
func StampRect(p *mesh.FacePaint, tile *image.RGBA, at image.Point) image.Rectangle
    // writes alpha>=128 pixels at 'at' (tile's min corner in texel space),
    // clipped to a caller-supplied rect; returns the dirty rect
func SnapToTileGrid(t image.Point, tw, th int) image.Point

// internal/paint/stampcommand.go — mirrors command.go's stroke command:
// one face, dirty-rect before/after, UndoBytes, coalesce per mouse-down.
```

Settings: `TileSettings{Path string; TileW, TileH, Margin, Spacing, Selected
int; Rot uint8; FlipX bool}` on `io.Settings`, round-trip tested. The armed
*tool* is still not persisted (same reasoning as PaintSettings).

Ops: `tile.import {path}`, `tile.grid {w,h,margin,spacing}` (presets are
UI sugar for w=h=N, margin=spacing=0 — the op speaks the general case), `tile.select
{index}`, `tile.orient {rot, flip}`, `tile.stamp {body, face, uv}`. Dump
line: `tileset w= h= tiles= sel= rot= flip=` plus the existing `facepaint
… opaque=` lines carrying the stamped-pixel assertions.

## 5. Milestones

### TP1 — Tileset core and the stamp (paint package only)
- [ ] `Tileset` slicing with margin/spacing; table tests against
      hand-computed sheets, including partial-column drop and degenerate
      (tile bigger than sheet → 0 tiles, refused upstream)
- [ ] `Orientation` D4: all 8 orientations of an asymmetric 2×2 test tile,
      composition table (rotate of a flip is the mirrored rotation)
- [ ] `StampRect`: alpha threshold, clip, dirty rect exact; stamping over
      paint overwrites only opaque pixels; grow-on-demand at image edges
- [ ] `SnapToTileGrid` incl. negative texels (floor, not truncate — the
      margin rows are negative)
- [ ] Stamp command: undo restores before-pixels exactly, one entry per
      mouse-down, UndoBytes counted
- [ ] Settings round-trip
**Accept:** unit tests green; nothing visible yet.

### TP2 — Ops, flow test, golden
- [ ] A committed test sheet: `testdata/tileset_test.png`, generated by a
      Go test helper (deterministic pixels — four distinct 8×8 tiles with
      margin 1 spacing 1, asymmetric so orientation shows), NOT hand-drawn
- [ ] `tile.*` ops + dump line; `tile.stamp` goes through the same command
      the pointer path will use
- [ ] Flow test: import → grid → select → stamp two adjacent cells → dump
      asserts `opaque=` counts and seamless adjacency (128 px per 8×8 tile
      etc.); stamp with rot/flip → different `sum=` hash; undo → counts
      return; invariants suite still green
- [ ] Golden: stamped face from iso view
**Accept:** flow test fail-checked against a deliberately broken snap.

### TP3 — The panel, the picker and the ghost
- [ ] Tile tool: stroke icon, `T` key, tool button in the paint tools row
- [ ] Tile section (replaces the palette section while the Tile tool is
      armed — tiles carry their own colours; pattern: buildEdgeSection):
      [Import…] via zenity → copy to config dir; Grid chips
      [8][16][32][64][Custom] — presets square, margin/spacing 0; Custom
      reveals W/H/Margin/Spacing DragNumbers — re-slicing live either
      way; the sheet drawn nearest-filtered at
      integer zoom fit to panel width (wheel scrolls if tall), selected
      tile outlined in the accent; rotate / flip stroke-icon buttons
      showing the current orientation
- [ ] Ghost preview: hovered cell's tile drawn as translucent per-texel
      fills in the paint cursor overlay, snapped (or free with Alt); the
      hint bar names the tool's verbs
- [ ] Press stamps; drag trails; Alt places free; face lock respected
- [ ] GPU texture lifecycle: import replaces, Close unloads
- [ ] Goldens: panel with sheet loaded + ghost on a face + stamped result;
      `TestEveryMessageIsRenderable` still green
- [ ] Settings persist across restart (headless flag honoured — headless
      never loads user settings, V-… existing rule in `New`)
**Accept:** the user's loop works end to end by hand: import a sheet,
adjust the grid until tiles line up, click a tile, stamp the hull, stamps
butt seamlessly, Ctrl+Z removes a stamp.

### TP4 — Stretch (user go-ahead before starting)
- Multi-tile rectangular selection in the picker, stamped as a block
- Grid phase offset controls (shift the snap grid on a face)
- Anything learned in TP3 that wants a second pass

## 6. Risk register

- **The picker widget** is the only new UI surface; if panel space fights
  back, the sheet view collapses to a fixed-height scroll region before
  anything else gives.
- **Preview cost** at 64×64 tiles — measured before shipped; coarsen the
  ghost rather than dropping it.
- **Zenity in headless** — never reached: scripts import by path.
- **Undo memory** — a 64×64 stamp's before/after is ~32 KB; trails of
  hundreds of stamps stay well under the 40 MB cap, but the cap machinery
  is already there and applies.

## 7. Definition of done

Per milestone: build/vet/gofmt clean, full suite green, new tests
fail-checked against broken code where a bug is being prevented, goldens
read before commit, PROGRESS entry, DECISIONS entries for anything that
deviates from this plan, commit `TP<N>:` and push.

Overall: the loop in TP3's accept line, plus a README section with the key
table row and a screenshot in `docs/shots/`.
