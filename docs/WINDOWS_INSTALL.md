# Windows installation and PXM viewer

Opening a `.pxm` file through Windows launches Modeler Viewer. Right-drag
orbits, middle-drag (or Shift + right-drag) pans, the wheel zooms, and F fits
the model. The view cube also works. Click **Edit** in the top-right to show
the normal modeling workspace with the same document, textures, PBR, pins,
file path and camera. Viewer navigation does not dirty or save the project.

Launch `modeler.exe` without arguments for the normal editor. Command-line
file paths default to viewer mode; `-view "path.pxm"` is explicit and
`-edit "path.pxm"` opens directly in the editor. One file per window is
supported. Headless script flags cannot be combined with a project path.

## Install

`tools/install-modeler.ps1` builds both executables, copies them and the icon
to `C:\Program Files\Modeler`, copies the parts and tileset libraries into
its `Library` folder, and registers `.pxm` for the current Windows user.
Program Files requires administrator access. To keep file installation and
per-user association registration separate, run the following steps:

```powershell
# Build from the repository in your normal account.
./tools/modeler.ps1 -Action Build
# Run this step as administrator (pass -LibrarySource for the original user's
# AppData\Modeler folder if elevating as a different Windows account).
./tools/install-modeler.ps1 -SkipBuild -FilesOnly
# Run this step in the original user's account.
./tools/install-modeler.ps1 -RegisterOnly
```

The association has **View in Modeler** as its default open action and an
**Edit in Modeler** shell verb. An existing Windows UserChoice is never
modified; Windows Default Apps can change it if another program already owns
the extension.

`Library\parts` and `Library\tilesets` are installation-time copies. The
working library remains in `%APPDATA%\Modeler`, so ordinary sessions can
save/update parts without administrator privileges. Settings, AI connection
tokens and recovery files are not copied into Program Files. The static
executables contain their fonts and UI assets and need no adjacent runtime
DLLs.
