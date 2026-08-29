# SPEC-DATA — Document Model, Commands/Undo, Files, Export, Scripts

## 1. IDs & naming
- Per-document monotonic `uint32` sequences per kind (bodies, sketches, faces-within-body); **never reused**, survive save/load (sequences persisted).
- `FaceUID = uint64(bodyID)<<32 | faceSeq` — globally unique per document, the anchor for paint, provenance, pick refs.
- Auto-names: "Sketch N", "Body N" (N = sequence, not count). Rename is free-text (trimmed, non-empty).

## 2. Document model (`internal/model`)

```go
type Document struct {
    FormatVersion int
    Seq           Sequences
    Planes        [3]PlaneState        // Top/Front/Right: Visible bool (existence is hardcoded — R1)
    Sketches      []*Sketch            // GEOM §3
    Bodies        []*Body
    Features      []FeatureRec         // append-only log: {Kind, ParamsJSON, Time} — debug/forward-compat, not replayed in v1
    Camera        CameraState          // saved view
    DirtySinceSave bool
}
type Body struct {
    ID uint32; Name string; Color RGBA; Visible bool
    Mesh mesh.Mesh                     // GEOM §2
}
```
- **Selection** lives outside the undo-able document: `Selection{Kind, refs…}` in app state (cleared/remapped on structural changes).
- Derived caches (topology, edge lists, GPU meshes, region results) are never serialized; invalidated by command execution via change notifications (`model.Events`: BodyChanged(id), SketchChanged(id), DocReplaced…).

## 3. Command bus & undo (R17)

### 3.1 Bus
```go
type Command interface {
    Name() string                       // for toasts/logs: "Extrude", "Move 3 vertices"
    Do(doc *Document) error             // validate-then-mutate; error ⇒ NOTHING changed (atomicity contract)
    Undo(doc *Document)
}
```
- `Bus.Run(cmd)`: on success push to undo stack (cap 200, drop oldest), clear redo, emit events, append FeatureRec, set dirty. On error: toast, nothing else.
- **Atomicity rule:** commands compute the full result (e.g., boolean output mesh + validation) *before* touching the document; mutation is a pointer/field swap. This is what makes "failed boolean changes nothing" true by construction (UX §11.3).

### 3.2 Undo strategy
Targeted snapshots: each command captures before-states of exactly what it mutates (a body's mesh pointer, a sketch's entity slice, a name string…). Bodies are small — cloning a mesh is cheap. Drag interactions coalesce: the drag opens a pending command, updates live state each frame, and commits ONE command on release (undo restores pre-drag). Escape during drag = revert to pre-drag, no history entry.

### 3.3 Paint strokes
Per mouse-down stroke: one command holding dirty-rect before/after subimages (rect = stroke bbox ∪ brush radius). Memory cap: paint undo entries ≤ 40 MB total (evict oldest beyond cap with a PROGRESS-noted heuristic). Palette/recents are settings, not document state — not undoable.

### 3.4 Undo UX
Ctrl+Z / Ctrl+Y (and Ctrl+Shift+Z); toast shows "Undid: Extrude". Undo/redo also restore visibility toggles, renames, colors — *everything* through the bus, no side-channel mutations. (Camera moves are NOT commands.)

## 4. `.ship` project file

Zip (stored or deflate), extension `.ship`:
```
manifest.json    {"app":"modeler","formatVersion":1,"savedAt":RFC3339}
document.json    the Document: sketch coords as int subunits, mesh verts as float64 arrays,
                 faces as loop index arrays + srcFace + paint refs, sequences, planes, camera
paint/<faceUID>.png   RGBA, one per FacePaint (Off/Texel/Frame/Res serialized in document.json)
thumbnail.png    256×256 render of the saved view (also used by welcome screen)
```
- Writer: temp file + atomic rename; never clobber the target until the zip is fully written.
- Reader: tolerant — unknown JSON fields ignored; missing paint PNG ⇒ face falls back to body color with a load warning list shown as one toast; `formatVersion` > known ⇒ open read-only copy with warning. Never crash on malformed input (fuzz the reader lightly in tests).
- Round-trip invariant (tested): save→load→save produces byte-identical `document.json`.

## 4a. Mesh import (Ctrl+I, V-144)
STL (both encodings) and OBJ, read into triangle soup by `io.ReadMeshFile` and assembled by `mesh.Assemble`: weld onto the subunit grid, drop degenerates, orient the shells by walking the surface, merge coplanar triangles into polygon faces, drop collinear boundary points, compact. The merge test is tight (normals within a twentieth of a degree **and** every corner inside `PlanarDist`) so a tessellated cylinder keeps its facets. Binary-vs-ASCII STL is decided by `84 + 50n == size`, never by the leading "solid". NaN/Inf refused at read. Scale is a card decision, applied before snapping; the result may legitimately be an open shell, reported in the toast rather than refused. Geometry only — no materials, colours or UVs.

## 5. Exports (Ctrl+E dialog: format, path via zenity)

- **OBJ + MTL + PNGs** (primary): triangulated; `v` floats; `vt` per painted-face texel mapping; one material per painted face (`map_Kd paint/<uid>.png`), one shared material per body color for unpainted faces; Y-up, -Z forward note in header comment; README documents "set texture filtering to nearest in your engine".
- **STL** (binary): geometry only; note in dialog "no colors in STL".
- **PNG screenshot**: current camera, 1×/2×/4× (nearest upscale — crisp pixel look preserved), optional transparent background (skip gradient, alpha framebuffer RT).
- Export never mutates the document; overwrite prompts via one modal (UX §4's rare-modal allowance).

## 6. Settings, autosave, recovery

- `%APPDATA%\Modeler\settings.json`: window rect, ui scale override, grid step, recent files (≤8, MRU), palette custom page + recents, tips counters (push/pull hint), MSAA toggle, autosave interval (default 120 s).
- Autosave: if dirty, every interval → `%APPDATA%\Modeler\autosave\<docStem>-<pid>.ship`; deleted on clean save/exit.
- Crash-save: `main` wraps the loop in recover → attempt autosave → write `crash-<ts>.log` (stack) → re-panic to OS. Next launch: if autosave newer than its document (or orphaned) → recovery modal: Restore / Discard.

## 7. Op-script format (headless, e2e, sample ship — SPEC-RENDER §9)

JSON array of ops, executed sequentially through the real command bus + camera controller. Keep ops 1:1 with commands/camera calls; extend as tools land. Core set:

```json
[
 {"op":"sketch.begin","plane":"Front"},
 {"op":"sketch.line","from":[0,0],"to":[8,0]},
 {"op":"sketch.rect","a":[0,0],"b":[8,3]},
 {"op":"sketch.circle","c":[4,1],"r":1,"segs":16},
 {"op":"sketch.finish"},
 {"op":"extrude","sketch":"Sketch 1","regions":[0],"depth":6,"draft":5,"dir":"normal","result":"new"},
 {"op":"extrude","sketch":"Sketch 2","regions":[0],"depth":-2,"result":"subtract"},
 {"op":"boolean","kind":"union","target":"Body 1","tools":["Body 2"]},
 {"op":"select","kind":"face","body":"Body 1","face":3},
 {"op":"move","delta":[0,1,0]},
 {"op":"rotate","axis":"y","degrees":90},
 {"op":"paint.res","res":32},
 {"op":"paint.color","hex":"#FFB454"},
 {"op":"paint.pixel","body":"Body 1","face":3,"uv":[4,4]},
 {"op":"body.visible","body":"Body 1","visible":true},
 {"op":"camera.view","view":"iso"},          // iso|front|top|right|back|left|bottom
 {"op":"camera.frame"},
 {"op":"settle"},                             // step virtual clock until animations done
 {"op":"shot","name":"m3_drafted_crate"}
]
```
- Coordinates: sketch ops in world units (floats allowed, snapped like the UI would); `paint.pixel` in texel ints.
- Name-or-ID addressing ("Body 1" or numeric id); unknown names = script error (exit non-zero, message names the op index).
- The **sample ship** (M9) is an op script in `assets/` — doubles as the full-app e2e test and the README hero image generator.
