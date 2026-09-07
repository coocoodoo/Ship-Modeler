# Live AI connection

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
