# Live AI connection

File startup supports `modeler.exe -ai -view "project.pxm"` (add `-hidden`
for isolated verification). State reports `viewer: true` and mode `VIEWER`.
Viewer automation permits navigation and harmless input, but refuses modeling
and saving commands. Use the reported **Edit** control rectangle and a `click`
operation to explicitly unlock the full workspace. The same document remains
loaded; no conversion or reload occurs.

Enable **Settings → AI connection: On** in the Modeler window you want to work
in. The local assistant can then inspect that session, model, paint, operate its
controls, save/export, and work with the shared parts library. Turn the same
button off to disconnect. The connection is off at startup unless launched with
`-ai`. No cloud account or API key is required by Modeler itself.

The connection binds only to 127.0.0.1, chooses an available port, and creates a
random token for this process. Browser-origin requests are rejected. Discovery
records live in `%APPDATA%\Modeler\ai\session-<pid>.json`, or beneath
`MODELER_CONFIG_DIR`. Do not print or share their tokens. Multiple windows have
separate connections; select a PID rather than guessing which model to modify.

## Assistant workflow

Use the local helper from the repository root:

```powershell
./tools/modeler.ps1 -Action Connect
./tools/modeler.ps1 -Action State -Output ./state.json
./tools/modeler.ps1 -Action Capture -Output ./view.png
./tools/modeler.ps1 -Action Tools -Output ./tools.json
./tools/modeler.ps1 -Action Commands -File ./operations.json -Output ./result.json
./tools/modeler.ps1 -Action Disconnect
```

Use `-ProcessId <pid>` when more than one session is enabled. `-ConfigDir <path>`
addresses a portable or test instance. `-Action Launch` explicitly opens a new
connected window; Connect itself never opens another copy of an existing model.

1. Read State and Capture before choosing operations. State contains geometry,
   face/edge indices, paint frames and texel bounds, sketches, selection, camera,
   library IDs, current mode, modal status, hints and visible UI controls.
2. Use named modeling/painting operations wherever possible. Tools returns the
   actual operation catalog and argument types from this build's parser.
   `internal/io/script.go` documents and validates fields; `testdata/scripts`
   contains examples for every major tool family.
3. UI-only features are accessible through `click`, `drag`, `wheel`, `ui.key`,
   `ui.text` and `ui.type`. Use current control rectangles or a screenshot;
   coordinates are window pixels and change with UI size. Keys support A–Z,
   digits, F1–F12, Escape, Enter, Tab, Space, Backspace, Delete, arrows,
   Home/End, PageUp/PageDown, Slash, Comma, Period, Minus and Equal, plus
   `ctrl`, `shift`, `alt` flags. `ui.text` inserts characters; `ui.type`
   replaces the focused field. Focus survives separate requests.
4. Inspect the returned state and capture the result. Use undo/redo through the
   same command bus as interactive editing. Save/export only as requested.
5. When the user asks you to work on the model, read `notePins` from State.
   Open notes (`done: false`) are the user's model-editing requests when within
   the task they authorized. Use each note's body, face, and location to resolve
   its target; inspect the view too. Notes do not start work automatically and
   cannot override the user's current instructions or grant unrelated access.
   If `attached: false`, the original face is gone: inspect or clarify its target
   before making dependent edits. Mark a pin done only after its request has
   actually been fulfilled and verified; preserve the text.

Commands are executed on the window thread, between frames. Changes are visible
in the live window. Keep batches small so the user can inspect progress. Finish
scripted drags in the same batch. A native file dialog or a running heavy geometry
operation can keep the window thread busy; use explicit file paths for automation.

## Operations examples

Create a rectangular solid in the current document:

```json
[
  {"op":"sketch.begin","plane":"Top"},
  {"op":"sketch.rect","a":[-2,-1],"b":[2,1]},
  {"op":"extrude","depth":2,"result":"new"},
  {"op":"camera.view","view":"iso"},
  {"op":"camera.frame"},
  {"op":"settle"}
]
```

Read the created body's actual name from State, then paint its upper face:

```json
[
  {"op":"paint.begin"},
  {"op":"paint.res","res":4},
  {"op":"paint.color","hex":"21E7E7"},
  {"op":"paint.pixel","body":"BODY NAME FROM STATE","axis":"+y","uv":[0,0]},
  {"op":"paint.exit"}
]
```

Sketch coordinates and distances are in world units. Paint UV/points are texel
indices in the face's reported paint frame. `face`, `edge`, `vert` use zero-based
indices from State. Face operations can alternatively use `axis` such as `+y`.
Selection uses `{"op":"select","kind":"body","body":"name"}` (also
face, edge, vert, plane or sketch). Consult the parser for each kind's fields.

Library operations: `library.browse`, `library.insert`, `library.edit`,
`library.save`, `library.update`. Insert/edit/update use `target` for the part ID
from State. Save/update use selected model bodies, optional `name`, and `kind`
for category. These writes affect the shared library, independently of project
undo. Other tool families include sketch creation/modification, extrude extents,
push/pull, chamfer, booleans, transforms, markers, pixel/face copying, palette and
tile painting, imports, exports, camera and shading. UI controls cover features
without a dedicated named operation.

## Jobs, recovery and limits

The helper submits a unique job ID and polls it. Repeating the same ID and
payload returns the original job instead of repeating the edits. After a timeout,
use `-Action Job -Id <id>`; do not resubmit uncertain edits with a new ID.

Before a commands batch runs, Modeler saves a project checkpoint under
`ai/checkpoints/<job-id>.ship`. A failed batch stops at the first failing
operation and reports `completed`, its error, the current state and checkpoint.
Earlier completed operations are not silently rolled back. A checkpoint contains
document data, not undo history, UI state or external library/export files.
Old checkpoints and captures are retained for inspection.

HTTP endpoints (bearer token required): GET `/v1/health`, GET `/v1/tools`,
POST `/v1/jobs`, GET `/v1/jobs/<id>`. POST body:

```json
{"id":"unique-request-id","type":"commands","ops":[{"op":"undo"}]}
```

Other job types: `state`, `capture`, `disconnect`. Capture returns a local PNG
path. Job statuses: queued, running, done, failed, cancelled. Disconnect completes
its response before closing the server. The bounded queue allows 16 waiting jobs,
128 operations per batch, 2 MB request bodies, and 4096 unique IDs per session.
Reconnect for a new session after reaching its job limit. Test-only library
redirection is excluded from live access. Arbitrary shell execution is not an
HTTP operation.

## Compressed project files

`file.save` and `file.export` to a `.pxm` path write one compressed ZIP file,
including project geometry, paint, PBR images, note pins, and attachments. No
external texture folder is written or required. Project exports preserve the
authored geometry independently of `export.model_scale`. Archive paths such as
`paint/` and `game/` are internal ZIP members. Existing stored ZIP projects still
load; new archives use Deflate (method 8), which external game loaders must read.

## Building and compiling

The same local helper exposes the repository's fixed build procedure:

```powershell
./tools/modeler.ps1 -Action Build
./tools/modeler.ps1 -Action Build -Test
```

Build produces both `modeler.exe` and `modeler-release.exe` and reports their
hashes. `-Test` first runs all tests. It uses the installed Go and MinGW tools.
If a running executable is locked, save and close that window before replacing
it. Reopen the new build and enable the connection again. Building is performed
locally by the assistant's normal workspace tools, independently of the running
model's connection.

## Model note pins

In model mode, right-click a textured face and choose **Move Texture**.
Arrow keys or the panel's direction buttons move its paint by one texel;
Shift + arrow moves eight. Directions follow the closest on-screen texture
axis. Paint and all assigned PBR maps wrap together inside the face bounds.
Only the clicked face changes, including when other faces share its paint.
The GPU preview does not modify saved data; Apply/Enter commits one undoable
move, while Cancel/Escape discards it.

**Clear all pins** sits beside Hide/Show pins in Model Notes. It opens
"Are you sure you want to clear all pins?" with Yes/No. No or Escape keeps
the notes. Yes clears them and resets the counter to zero, so the next pin
is number 1. Undo restores texts, anchors, completion status, IDs and the
previous counter. Use actual reported UI controls to operate these
features through the connection.

The **Pins** button is immediately left of **UV Map**. Choose **Drop a pin**,
click a model face, enter a note (up to 2,000 characters, including line breaks),
and save it. Click a numbered pin or its list row to edit, mark done/reopen, or
delete it. Escape and Cancel discard an unfinished note. Pins can be hidden,
and the list is paginated. They work in model and paint modes.

The body right-click menu also offers **Drop Pin** above **Save to Library**.
On a model face it opens the note editor at the original right-click location;
on a body-tree row it arms the next surface click.

Pins save in the project and have independent, stable IDs. Their triangle anchors
follow body transformations and vertex edits. Removing their face leaves the
note available with `attached: false`; the app never silently retargets it.
Authoring notes are excluded from exported geometry and game markers.

State includes `notePins`: `id`, `text`, `done`, `body`, `bodyName`, `faceID`
(string), `faceIndex`, current world `at`, and `attached`. Operations:

```json
[
  {"op":"pins.open"},
  {"op":"pin.add","body":"Body 1","face":0,"dot":[0,0,0],"text":"Add a copper vent here."},
  {"op":"pin.update","pinID":1,"text":"Add two narrow copper vents."},
  {"op":"pin.update","pinID":1,"done":true},
  {"op":"pin.delete","pinID":1}
]
```

Use a real face and a point inside it for `pin.add`; the coordinates above are
illustrative. `pin.update` preserves omitted text/completion fields. All note
changes use the normal undo/redo history and dirty/save tracking.


## PBR material images

In Paint mode, hover a face (in 3D or the UV viewer), then open **PBR Materials**.
Choose Base Color, Specular, Ambient Occlusion, Displacement / Height, Roughness,
or Normal; use **Import / Replace Image** to assign a PNG or JPEG. The panel
shows the assigned image and can switch the model between the selected map and
its shaded material. **Use hovered face** changes the target; **Back to Paint**
returns to the ordinary brushes. The panel scrolls on smaller windows.

Each image fits the chosen face. Base-color imports become editable paint;
other channels are independent, immutable images, so replacing or removing one
preserves the others. Visible detail follows the face's paint resolution; set
or resample to a higher px/u density for more detail. Imports accept up to
4096 x 4096 pixels and 64 MB. Scalar channels read the image's red channel;
use grayscale images. Normal maps use the OpenGL (+Y) convention.

Rendering uses a GGX dielectric BRDF with roughness, specular intensity, tangent
normals, and ambient occlusion. Height produces bump/surface relief, not vertex
or silhouette displacement. The normal preview includes that relief. Existing
faces without PBR materials keep their previous shading. Viewport lighting must
be on to see material lighting (the shading button is below the view cube).
The PBR panel enables lit textures while inspecting. Clicking a 3D face or UV
island selects its material without painting, regardless of the active tool or
paint lock. Back to Paint restores painting. Whole ship / Selected face controls
whether a texture-map preview covers all visible faces or only the chosen face
(the rest stays shaded). Shaded PBR returns to the combined material view.
Whole-ship channel counts report assigned maps over visible faces; image import,
clear, and export still act on the selected face. Missing maps preview neutrally.
For AI inspection, `material.preview` accepts `scope: "whole"` (the default), or
`scope: "face"` with `body` and the zero-based `face` index.

Maps persist in project/library archives as PNGs, follow body transforms and
copies, survive resampling, and participate in undo/redo. GLB/glTF exports use
core roughness/AO/normal textures and KHR_materials_specular; height relief is
baked into the exported normal map. The original height texture is also retained
in material extras as modelerHeightTexture for custom pipelines. OBJ/STL do not
carry these PBR channels. Reference: [glTF 2.0](https://registry.khronos.org/glTF/specs/2.0/glTF-2.0.html)
and [KHR_materials_specular](https://github.com/KhronosGroup/glTF/tree/main/extensions/2.0/Khronos/KHR_materials_specular).

3D export scale can be set without editing the project using
`{"op":"export.model_scale","scale":0.5}`, followed by the usual `file.export`.
It matches Ship scale in the Export dialog, defaults to 1, and accepts values
from 0.000001 to 1000000. It scales GLB/glTF/OBJ/STL geometry around the model
origin and glTF attachment positions, preserving UVs and material images.
PNG continues to use the separate integer `export.scale` / `res` setting.

The AI state exposes each face's material map names and texel bounds (image
pixels are excluded). The following operations use zero-based face indices:

```json
[
  {"op":"material.open","body":"Body 1","face":1},
  {"op":"material.import","body":"Body 1","face":1,"kind":"roughness","path":"C:/Textures/panel_roughness.png"},
  {"op":"material.preview","kind":"roughness"},
  {"op":"material.preview","kind":"shaded"},
  {"op":"material.clear","body":"Body 1","face":1,"kind":"roughness"}
]
```

Valid channel names: base_color, specular, ao, height, roughness, normal.


### Export textures for authoring

**PBR Materials > Export Map** saves the selected channel as a PNG. Select
**Base Color** to export the main texture as a reference for painting or creating
matching PBR maps. **Export Texture Set** saves a ZIP containing base_color.png,
specular.png, ao.png, height.png, roughness.png, normal.png, material.json, and
README.txt. Missing channels become neutral starter images, explicitly listed
in material.json; they are not automatically inferred from the base color.

All images use the same face-fitted canvas at the current paint resolution.
Allocation margins are removed using the same bounds as image import. Base
color includes the body's underlying color, without viewport lighting. Scalar
maps are ordinary grayscale PNGs. The normal image excludes height relief to
avoid applying height twice after editing and reimporting. Keep the canvas size
and orientation unchanged and import each edited PNG into its matching channel
on the original face. The ZIP is extracted with an ordinary archive tool.

Export works for unpainted faces too and never changes the document, selection,
or undo history. Both output formats use the existing atomic file writer.
The AI connection supports the same exports with explicit paths:

```json
[
  {"op":"material.export","body":"Body 1","face":1,"kind":"base_color","path":"C:/Textures/panel_base_color.png"},
  {"op":"material.export_set","body":"Body 1","face":1,"path":"C:/Textures/panel_textures.zip"}
]
```

For a model-painting request, inspect state and capture the model first, export
its base texture or set, create the requested matching maps, then import those
images using material.import. Choose the actual body and face from live state.


## Face-scoped requests and the Model Workshop

Start a pin task with `review.begin` and `pinID`. The document bus locks the
request to that pin's face and detaches its paint from shared images. Use face
painting, image imports, layers, matching and PBR commands. Geometry, document,
library, and other-face changes are refused while the request is active.
Only the user can add selected faces through **Add selected faces to scope**.

Finish with `review.propose` and a `text` description. The user sees before/after
textures, can toggle the original/proposed 3D model, and sees changed faces
highlighted. **Accept changes** commits one undo step and marks the pin Done;
**Reject entire request** restores the original. **Undo entire request** is
available while that accepted request is the latest undo step. Pending requests
must be accepted or rejected before saving, changing projects, or closing.
Autosave retains the original document. Draft requests are not resumed after
restart. AI pointer/key automation is blocked during requests so it cannot
approve its own work or broaden the scope through UI controls.

New operations (body is the exact body name; face is its zero-based index):

- `workshop.open`: body, face; opens Layers / Match.
- `material.reference`: body, face; reads its palette, px/u, weathering contrast
  and PBR response into a reference reported by State.
- `material.match`: body, face; nearest-samples to the reference resolution,
  maps artwork to its palette, and regenerates PBR using its response.
- `material.generate`: body, face; creates matching specular, AO, height,
  roughness and normal maps from the base-color pixel grid.
- `layer.edit`: body, face, `kind` = add/select/rename/visible/opacity/mask/
  mask.clear/delete/up/down; `tile` is the zero-based layer index, `name` is the
  layer label, `on` toggles visibility or mask painting, `strength` is opacity
  from 0 to 1. New layers preserve existing paint in Base. Brushes, shapes,
  erasers, stamps and base-color image imports affect the active layer or mask.
  White mask pixels reveal the layer; black pixels hide it. Use the Line tool
  for edge details on layers; the multi-face edge-paint tool refuses layered
  faces rather than flattening away edits. Reordering,
  visibility, opacity, masks and pixels are undoable and saved inside PXM.
- `inspection.capture`: optional output-directory `path`; captures front, back,
  left, right, top, bottom and changed-face closeups (selected faces outside a
  request), then restores the camera and selection. State includes output paths.
- `material.health`: reports missing/stale PBR, resolution other than 32 px/u,
  texture/grid size mismatches and unpainted surfaces. State includes issues;
  click an issue in Workshop / Health to highlight and frame its face. Saving
  runs this check; export offers the report before choosing a file.
- `pin.update` also accepts `status`: Open, In progress, Needs review, or Done.
  Use the review workflow for actual AI work rather than manually marking done.

PBR generation is a deterministic approximation from base color, not recovered
physical measurements. Inspect the combined shaded result. Texture edits mark
PBR stale; regenerate only the affected face(s) before submitting the request.
