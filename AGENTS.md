# Modeler collaboration

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
