# Modeler collaboration

Always launch visible Modeler windows maximized. Preserve hidden windows for
automated captures and tests.

Use a pixel resolution of **32 px/u** in Modeler for all future modeling and
painting work unless the user explicitly requests a different resolution.
Always generate and apply matching PBR maps when creating or painting models
in Modeler. After texture edits, update the affected PBR maps to match the
current Base Color. Preserve 32 px/u and verify the combined shaded material.

When working from a pin note, modify only the face that pin is attached to.
Do not expand the edit to matching textures, other faces, or the whole body
unless the user explicitly requests that wider scope in the conversation.

When the user asks you to model or paint in the running Modeler, read
`docs/AI_CONTROL.md` and use `tools/modeler.ps1` to connect, inspect state and
capture the view before editing. Operate the model only for the user's requested
task. If multiple sessions are enabled, identify the intended window before
sending edits. Never print connection tokens. Use the reported job ID to resolve
an uncertain response instead of repeating edits under a new ID.

For code changes, preserve unrelated work. The user requires compiling after
editing: build both `modeler.exe` and `modeler-release.exe`, as implemented by
`tools/modeler.ps1 -Action Build`. Run tests relevant to the change and record
completed work in `PROGRESS.md`.

For every future Modeler code change, deploy the newly compiled `modeler.exe`
and `modeler-release.exe` to `C:\Program Files\Modeler\` and verify that the
installed executables match the new build. Run the latest build from that
installed directory, not the workspace copy. Use the existing installer when
appropriate and preserve the installed library and PXM association. Preserve
unsaved work before replacing or restarting any running Modeler session.

For pin work, use review.begin before editing and review.propose after matching
PBR and inspection. Accept/Reject and scope expansion belong to the user; do not
try to operate those controls through automation. Use material.reference and
material.match when matching an existing face, and layer.edit for separate
details. Run material.health and inspection.capture before proposing a request.
