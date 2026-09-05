# Modeler

A 3D modelling tool for pixel-art spaceships. Sketch a profile on a plane, pull
it into a solid, cut and combine, then paint the faces pixel by pixel — and take
the result out as glTF, OBJ, STL or a PNG.

Everything is grid-snapped and every texture is sampled nearest-neighbour, all
the way out to the exported file. The pixels you place are the pixels you get.

![The sample ship](docs/shots/v1_hero.png)

## Quickstart

Run it, and press **Sample ship** on the welcome card to see what it makes.
To build one yourself:

1. **Click a plane** in the viewport or the tree — Top, Front or Right. The
   camera turns to face it and a grid appears.
2. **Draw a closed profile** with Line (`L`), Rectangle (`R`), Circle (`C`),
   Arc (`A`), Polygon (`P`), Slot (`O`) or Spline (`S`) — sixteen tools in
   eight groups, each group's chevron opening its variants.
   Closed shapes fill in; loose ends show as red rings, because only a closed
   profile can be pulled into a solid. The **Grid** chips on the sketch card
   set the grid and snap spacing, from a quarter unit to two. `Q` marks
   geometry as construction: guides that snap but close no region. With
   something selected, the card's **Modify** section fillets and chamfers
   corners, offsets a shape, mirrors about the sketch axes, and repeats a
   selection in a linear or circular pattern.
3. **Press `E`** and drag the arrow. The Extrude card sets the depth, a draft
   angle, and whether the result is a new body or is added to, cut out of, or
   intersected with what it reaches.
4. **Click a flat face** and press `S` to sketch on it, or drag its arrow to
   push and pull it directly.
5. **Press `P`** to paint. Pick a resolution — **Res** is in texels per unit, so
   8 means eight pixels to the unit on every face — then pick a colour and draw
   on the model. The texels under the brush are outlined on the surface, so you
   always know which pixel you are about to hit.
6. **Ctrl+S** to save, **Ctrl+E** to export.

The hint bar at the bottom always says what to do next. If a control is greyed
out, its tooltip says how to turn it on.

## Keyboard

| | |
|---|---|
| **Navigate** | |
| Right-drag / Middle-drag / Wheel | Orbit / Pan / Zoom to cursor |
| `F` / `O` | Frame selection / Orthographic ⇄ perspective |
| **Tools** | `S` sketch · `E` extrude · `B` boolean · `M` move · `P` paint |
| **Sketch** | `V` select · `L` line · `R` rectangle · `C` circle · `A` arc |
| | `P` polygon · `O` slot · `S` spline · `.` point · `Q` construction |
| | a group's key again steps to its next variant (Line → Midpoint line) |
| **Paint** | `D` pencil · `B` soft brush · `E` eraser · `G` fill · `I` pick |
| | `L` line · `R` rectangle · `C` circle · `N` gradient · `X` swap colours |
| | `K` edge line — pick edges, set a width, bake a band along them |
| | `W` magic wand — select similar colours; other tools then paint only inside (Shift adds, Esc clears) |
| | `U` rectangular pixel selector · `Ctrl+C` copy pixels · `Ctrl+V` paste onto a face |
| **Files** | `Ctrl+N` new · `Ctrl+O` open · `Ctrl+S` save · `Ctrl+Shift+S` save as |
| | `Ctrl+E` export · `Ctrl+I` import an STL or OBJ mesh |
| **Edit** | `Ctrl+Z` undo · `Ctrl+Y` redo · `Del` delete · `H` hide |
| **Held** | `Alt` no snapping, or eyedropper in paint · `Ctrl` fine snap · `Shift` add to selection, constrain a shape |
| `Esc` | Back one level · `?` the shortcut sheet |

The viewport uses **live screen-space ambient occlusion**: inside corners,
pockets and contact between separate parts receive soft shadows as you edit
and orbit. Use **AO On / AO Off** at the bottom left; the setting is remembered.
Enabling AO from flat view also restores lighting. The effect uses visible
scene depth, so surfaces hidden from the camera cannot contribute shadows.
It does not alter paint or exported geometry; rendered PNGs include the shading.

The interface uses consistent outline icons, native Windows typography with
embedded fallbacks, and restrained active-tool highlights. The chrome's colour
follows what you are doing: gold while sketching, teal
while extruding, violet for booleans, pink in paint, lime while placing
markers, blue otherwise. The hint bar names the mode in a chip of the same
colour. Every colour is yours to change in `theme.json` beside the settings
file - it is written out with the defaults on first launch, so open it and
edit any `#RRGGBB`. The viewport itself never tints, so painted colours stay
true.

## Painting, and why the exports look right

Face paint is anchored to a frame that belongs to the face, not to the
triangles under it, so pixels stay where you put them when the geometry
underneath changes. Cut a window through a painted hull and the paint on both
sides of the cut is still exactly where it was.

To **copy pixels between faces**, press `U` in Paint and drag a box around the
paint you want; hold `Shift` for a square. Press `Ctrl+C`, then `Ctrl+V`, hover
another face to preview the placement, and click to paste. The three icons
beside the Paint heading provide the same controls. Hold `Ctrl` and scroll
the mouse wheel to rotate the paste in 90° steps; scrolling back reverses it.
You can also choose **0° / 90° / 180° / 270°** directly under **Rotation**.
The **Flip H** and **Flip V** buttons mirror the rotated paste left/right or
top/bottom. They highlight when enabled; click again to restore. Both can be
combined, and copying a new selection resets the flips.
Right-click during placement to choose which of the paste's **four corners**
follows the cursor, or choose **0° / 90° / 180° / 270°** in the mini menu.
**Cancel**, clicking outside, or `Esc` closes the menu without placing pixels
or changing the current paste. Right-drag still moves the camera.
You can place the copied
pattern repeatedly; `Esc` stops placement and `Ctrl+Z` undoes each paste.
Pixels keep their colors and alpha, one copied pixel per destination pixel.
Empty pixels leave existing paint underneath, and the destination face clips
the result at its edges. The clipboard lasts for the current app session.

The **Edge tool** (`K`) paints a band along edges you pick — panel seams,
plating lines, an outlined hull. **All corners** takes every sharp edge of a
body at once. The width slider is in pixels, 1 to 16, and the panel shows what
that comes to in units beside it — at Res 8 a one-pixel line is 0.125 u, and it
is that same 0.125 u on every face it touches. If one pixel is still thicker
than you want, raise the Res — with paint on the model, choosing a Res chip
resamples every painted face to the new density in one undoable step, so the
model always has exactly one pixel size. The band is laid inside each face and meets its
other half at the corner, so it reads as one line rather than two that nearly
line up, and it is baked into the faces own pictures: it saves, exports and can
be painted over like anything else you drew.

The **Tile tool** (`T`) stamps pixel-art tiles from an imported sheet. Import
a PNG, pick the grid that separates its tiles — 8, 16, 32, 64, or Custom with
margin and spacing for Tiled-style sheets — click a tile in the panel, and
click the model to stamp it. Stamps snap to a tile grid so they butt
seamlessly (hold `Alt` to place free), dragging lays a trail, and the rotate
and mirror buttons turn the stamp. One tile pixel is one texel at the face's
own resolution, transparent tile pixels leave the surface alone, and a whole
trail is a single undo step. The sheet and its grid are remembered between
sessions, like the palette.

The **dither modes** (2×2, 4×4, 8×8 Bayer) are how a gradient or a soft brush
gets a middle without leaving the palette: the shading is spent on how many
whole texels are painted, not on inventing colours between two of them.

Exports carry the same promise as far as the format allows:

- **glTF** (`.glb` or `.gltf`) writes `NEAREST` filtering into the file, so an
  engine loads it looking right without being told.
- **OBJ** (`+ .mtl + paint/*.png`) has no way to say that, so set your renderer
  to nearest filtering by hand. The file says so in its header.
- **STL** is geometry only — the format carries no colour at all.
- **PNG** exports the current view at 1×, 2× or 4×, with the option of a
  transparent background. Whole-number scales only, so the pixels stay square.

## Files

Ships are saved as `.pxm` ("pixel model"): a zip holding the document, one
PNG per painted face, a thumbnail — and a `game/` folder made for game
engines: `ship.glb` (the render-ready model, textures embedded, NEAREST
filtering baked in) and `markers.json` (the orientation dots and everything
derived from them). Files saved under the old `.ship` name still open.

The **Markers** section of the tree places the dots a game engine reads: one
for the ship's front, one for its top, and one per thruster. Each dot lands
on the face you click and carries that face's outward normal — for a thruster
that is the exhaust direction. From those the file derives an orthonormal
forward/up/right basis, so an engine loads a `.pxm` already knowing which way
the ship flies, with no per-hull correction tables.

Dropping a `.pxm` (or `.ship`) on the window opens it. Unsaved work
is autosaved every two minutes and again if the program ever crashes; the next
launch offers it back — and anything that would replace unsaved work, from the
toolbar's **New** button to the window's close button, asks first, offering to
save, to discard, or to go back to what you were doing. Save runs first and the
action follows only if it worked, so a dialog waved away costs you nothing.

The toolbar's right end carries **New · Import · Open · Save · Export**, each
the button form of its shortcut.

**Ctrl+I** imports an **STL or OBJ** mesh. It is not a triangle dump: coplanar
triangles are merged back into polygon faces, so an imported box arrives as six
rectangles you can paint, push and sketch on rather than twelve triangles you
cannot. Vertices are welded onto the same grid everything else here uses,
winding is repaired, and the card asks what one file unit is worth — mesh files
carry no units — showing the result in units before you commit. STEP files go
through a converter first (FreeCAD exports STL happily).

Settings, autosaves and crash logs live in `%APPDATA%\Modeler`. Set
`MODELER_CONFIG_DIR` to put them somewhere else.

## Building from source

Needs Go 1.26 and a MinGW-w64 gcc on `PATH` — cgo is required, for raylib and
for the boolean kernel.

```
set CGO_ENABLED=1
go build ./...
go test ./...
go run ./cmd/modeler
```

A release build, which must be statically linked or it will not start on a
machine without MinGW:

```
go build -ldflags "-s -w -H windowsgui -extldflags=-static" -o modeler.exe ./cmd/modeler
```

The program can also drive itself, which is how it is tested: `modeler.exe
-headless -script <ops.json> -out <dir>` runs a script of operations against a
hidden window and writes PNGs. `assets/sample_ship.json` is one such script —
it is the ship on the welcome card, the end-to-end test, and the image at the
top of this file.

## Credits

Modeler stands on:

| | |
|---|---|
| [raylib](https://www.raylib.com/) via [raylib-go](https://github.com/gen2brain/raylib-go) | window, input, GPU rendering and text — zlib |
| [Manifold](https://github.com/elalish/manifold) v3.5.2 | the boolean kernel, the same one OpenSCAD uses — Apache-2.0 |
| [Clipper2](https://github.com/AngusJohnson/Clipper2), vendored inside Manifold | BSL-1.0 |
| [zenity](https://github.com/ncruces/zenity) | native file dialogs — MIT |
| [Go Regular](https://go.dev/blog/go-fonts) via `golang.org/x/image` | the UI typeface — BSD-3-Clause |

Licence texts for the vendored kernel are in `third_party/manifold/`.

The 32-colour palette is original to this program: four ramps of eight, tuned
for hull plating rather than for covering the colour wheel. Lospec `.hex`
palettes can be imported over it.
