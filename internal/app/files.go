package app

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"modeler/assets"
	"modeler/internal/geom"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

// Files (SPEC-DATA §4–§6): new, open, save, export, autosave and recovery.
//
// Everything that puts a dialog on screen is *requested* during a frame and
// *run* after it. A native file dialog is modal and pumps its own message loop,
// and doing that between BeginDrawing and EndDrawing means running somebody
// else's loop with a frame half submitted. The queue is one field and it keeps
// that from ever being a question.

// ThumbnailSize is the square render stored in a .ship for the welcome screen.
const ThumbnailSize = 256

// PNGExportScales are the whole-number upscales the export offers, so the
// pixels stay square (SPEC-DATA §5).
var PNGExportScales = []int{1, 2, 4}

// fileAction is a dialog-driven operation waiting for the frame to end.
type fileAction uint8

const (
	fileNone fileAction = iota
	fileNew
	fileOpen
	fileOpenPath
	fileSave
	fileSaveAs
	fileExport
	fileImportPalette
	fileSample
)

// fileState is where the document lives and what is queued against it.
type fileState struct {
	path     string
	readOnly bool

	pending     fileAction
	pendingPath string
	// confirm holds an action that would replace unsaved work, parked behind
	// the "discard unsaved changes?" modal until it is answered.
	confirm     fileAction
	confirmPath string
	// welcomeDismissed marks that the welcome card has been answered or waved
	// away this session, so it stays down even while the document is empty.
	welcomeDismissed bool
	// exportOpen is the options card; exportFormat indexes io.ExportFormats,
	// and exportScale and exportAlpha apply to the PNG one.
	exportOpen   bool
	exportFormat int
	exportScale  int
	exportAlpha  bool

	// sinceAutosave counts up in milliseconds while the document is dirty.
	sinceAutosave float64
	// autosaveDue defers the actual write to after EndDrawing: zipping the
	// document is a disk-and-encode job, and doing it mid-frame was a hitch.
	autosaveDue bool
	// autosavePath is the recovery file this session has written, if any.
	autosavePath string

	// recovery is what was found on disk at startup, waiting to be offered.
	recovery []io.Autosave
}

// initFiles restores what the settings remember and looks for anything left
// behind by a session that did not end cleanly.
func (a *App) initFiles() {
	a.files.exportScale = 1
	// A headless run normally looks nowhere: a golden shot must not depend on
	// whether whoever is running it happens to have crashed this week, and a
	// recovery card over the viewport would do exactly that. Given its own
	// config directory it has been pointed somewhere isolated on purpose, and
	// then it looks — which is how the recovery flow is tested at all.
	if a.Headless && os.Getenv(io.ConfigDirEnv) == "" {
		return
	}
	found, err := io.FindRecoverable()
	if err == nil {
		a.files.recovery = found
	}
}

// DocumentName is what the document is called, for the title bar and the hint.
func (a *App) DocumentName() string {
	if a.files.path == "" {
		return "Untitled"
	}
	return strings.TrimSuffix(filepath.Base(a.files.path), io.ShipExtension)
}

// DocumentTitle is the name with the marks that say what state it is in.
func (a *App) DocumentTitle() string {
	name := a.DocumentName()
	if a.files.readOnly {
		name += " (read-only)"
	}
	if a.Doc().DirtySinceSave {
		name += " •"
	}
	return name
}

// RequestFile queues a file operation for the end of the frame.
//
// An action that would replace unsaved work stops at a question first. The
// window's close button has always asked; Ctrl+N and Open reached the same
// cliff without the fence, and one reflexive "new ship" threw the old one away.
func (a *App) RequestFile(act fileAction) {
	if a.guardUnsaved(act, "") {
		return
	}
	a.files.pending = act
}

// RequestOpenPath queues opening a specific file, which is what the welcome
// screen's recent list does.
func (a *App) RequestOpenPath(path string) {
	if a.guardUnsaved(fileOpenPath, path) {
		return
	}
	a.files.pending = fileOpenPath
	a.files.pendingPath = path
}

// guardUnsaved parks a destructive action behind the discard prompt when there
// is unsaved work to lose. It reports whether it did.
func (a *App) guardUnsaved(act fileAction, path string) bool {
	switch act {
	case fileNew, fileOpen, fileOpenPath, fileSample:
	default:
		return false
	}
	if !a.Doc().DirtySinceSave || a.Headless {
		return false
	}
	a.files.confirm, a.files.confirmPath = act, path
	a.UI.ShowModal(ui.ModalState{
		Title:       "Discard unsaved changes?",
		Body:        a.DocumentName() + " has changes that are not saved.",
		ConfirmText: "Discard",
		CancelText:  "Keep working",
		Danger:      true,
	})
	return true
}

// RunPendingFile performs whatever the frame asked for. It must be called
// outside BeginDrawing.
func (a *App) RunPendingFile() {
	act := a.files.pending
	a.files.pending = fileNone
	switch act {
	case fileNew:
		a.NewDocument()
	case fileOpen:
		a.openWithDialog()
	case fileOpenPath:
		a.OpenPath(a.files.pendingPath)
	case fileSave:
		a.Save()
	case fileSaveAs:
		a.SaveAs()
	case fileExport:
		a.runExport()
	case fileImportPalette:
		a.importPaletteWithDialog()
	case fileSample:
		a.BuildSampleShip()
	}
}

// NewDocument replaces everything with an empty document.
func (a *App) NewDocument() {
	a.leaveModes()
	a.Bus.Replace(model.NewDocument())
	a.Sel.Clear()
	a.files.path = ""
	a.files.readOnly = false
	// Asking for a new ship answers the welcome card, whether the ask came
	// from the card's own button or from Ctrl+N. An empty document is the
	// card's show condition, so without this the button appeared to do
	// nothing: it replaced empty with empty and the card stayed up.
	a.dismissWelcome()
	a.clearAutosave()
	// A new ship starts at the home view, not at whatever zoom the last one
	// ended on (V-129): planes seen from 200 u away and planes filling the
	// window are both wrong answers to "where do I start".
	a.GoHome(a.layout.RenderViewport())
	a.Toast(ui.Toast{Text: "New ship"})
}

// openWithDialog asks for a file and opens it.
func (a *App) openWithDialog() {
	path, ok, err := io.AskOpenShip(a.Settings.LastDir)
	if err != nil {
		a.Toast(ui.Toast{Text: "The file dialog could not open", Kind: ui.ToastError})
		return
	}
	if !ok {
		return
	}
	a.OpenPath(path)
}

// OpenPath loads a document from disk and remembers where it came from.
func (a *App) OpenPath(path string) bool { return a.openShip(path, true) }

// openShip is the load itself. remember controls whether the path joins the
// recents and the last-used directory: a document the user chose does, a
// recovery file does not — its path is a hidden folder and a generated name,
// and neither belongs on the welcome card.
func (a *App) openShip(path string, remember bool) bool {
	res, err := io.LoadShip(path)
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		a.forgetRecent(path)
		return false
	}
	a.leaveModes()
	a.Bus.Replace(res.Doc)
	a.Sel.Clear()
	a.files.path = path
	a.files.readOnly = res.ReadOnly
	a.applyCameraState(res.Doc.Camera)
	a.clearAutosave()

	if remember {
		a.Settings.LastDir = filepath.Dir(path)
		a.Settings.AddRecentFile(path)
		a.saveSettings()
		a.Toast(ui.Toast{Text: "Opened " + filepath.Base(path)})
	}
	for _, w := range res.Warnings {
		a.Toast(ui.Toast{Text: w, Kind: ui.ToastWarn})
	}
	return true
}

// Save writes the document where it already lives, asking for a place the
// first time.
func (a *App) Save() bool {
	if a.files.path == "" || a.files.readOnly {
		return a.SaveAs()
	}
	return a.saveTo(a.files.path)
}

// SaveAs asks where to write and remembers the answer.
func (a *App) SaveAs() bool {
	suggested := a.files.path
	if suggested == "" {
		suggested = filepath.Join(a.Settings.LastDir, a.DocumentName()+io.ShipExtension)
	}
	path, ok, err := io.AskSaveShip(suggested)
	if err != nil {
		a.Toast(ui.Toast{Text: "The file dialog could not open", Kind: ui.ToastError})
		return false
	}
	if !ok {
		return false
	}
	return a.saveTo(path)
}

// saveTo does the write, with a fresh thumbnail of the current view.
func (a *App) saveTo(path string) bool {
	a.captureCameraState()
	if err := io.SaveShip(path, a.Doc(), a.thumbnail()); err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	a.files.path = path
	a.files.readOnly = false
	a.Doc().DirtySinceSave = false
	a.files.sinceAutosave = 0
	// The work is on disk under its own name now, so the recovery copy has
	// nothing left to recover.
	a.clearAutosave()

	a.Settings.LastDir = filepath.Dir(path)
	a.Settings.AddRecentFile(path)
	a.saveSettings()
	a.Toast(ui.Toast{Text: "Saved " + filepath.Base(path)})
	return true
}

// thumbnail renders the square preview a .ship carries (SPEC-DATA §4).
//
// It is a real render of the saved view rather than a crop of the window, so
// the picture in the welcome list is the ship and not whatever panel happened
// to be over it.
func (a *App) thumbnail() *image.RGBA {
	if a.Headless || a.Renderer == nil {
		return nil
	}
	s := a.thumbnailScene()
	return a.Renderer.Capture(&s, render.CaptureOpts{
		W: ThumbnailSize, H: ThumbnailSize,
	})
}

// thumbnailScene is the document with the working aids taken out: no planes,
// no grid, no gizmo, no sketch overlays. A thumbnail is of the ship.
func (a *App) thumbnailScene() render.Scene {
	s := a.BuildScene()
	s.Planes = nil
	s.Sketches = nil
	s.Gizmo = nil
	s.Grid = nil
	s.DimFactor = 1
	// Square, and framed on whatever is there, because the saved camera was
	// framed for a viewport of a different shape.
	cam := a.targetCamera()
	cam.FrameBox(scene.FrameAll(&s), 1)
	s.Camera = cam
	for i := range s.Bodies {
		s.Bodies[i].Selected = false
		s.Bodies[i].HoverFace = 0
		s.Bodies[i].SelectedFaces = nil
	}
	return s
}

// captureCameraState copies the live camera into the document, so a file opens
// looking the way it was left (SPEC-DATA §2).
func (a *App) captureCameraState() {
	c := a.targetCamera()
	a.Doc().Camera = model.CameraState{
		Target:      [3]float64{c.Target.X, c.Target.Y, c.Target.Z},
		Azimuth:     c.Azimuth,
		Elevation:   c.Elevation,
		Dist:        c.Dist,
		OrthoScale:  c.OrthoScale,
		Perspective: c.Perspective,
	}
}

// applyCameraState puts a loaded view back, without an animation: there is
// nothing to animate from.
func (a *App) applyCameraState(s model.CameraState) {
	if s.OrthoScale <= 0 && s.Dist <= 0 {
		return // a document saved before there was a camera to save
	}
	a.Anim.Cancel()
	cam := a.Camera
	cam.Target = geom.Vec3{X: s.Target[0], Y: s.Target[1], Z: s.Target[2]}
	cam.Azimuth, cam.Elevation = s.Azimuth, s.Elevation
	if s.Dist > 0 {
		cam.Dist = s.Dist
	}
	if s.OrthoScale > 0 {
		cam.OrthoScale = s.OrthoScale
	}
	cam.Perspective = s.Perspective
	cam.Normalize()
	a.Camera = cam
}

// leaveModes puts every transient tool away before the document underneath it
// is replaced.
func (a *App) leaveModes() {
	a.CancelStroke()
	a.ExitPaint()
	if a.InExtrude() {
		a.CancelExtrude()
	}
	if a.InBoolean() {
		a.CancelBoolean()
	}
	if a.InSketch() {
		a.ExitSketch(false)
	}
	a.dropPushPull()
	a.transform.tool = nil
}

// forgetRecent drops a file that would not open from the recent list, so the
// welcome screen stops offering it.
func (a *App) forgetRecent(path string) {
	kept := a.Settings.RecentFiles[:0]
	for _, p := range a.Settings.RecentFiles {
		if !strings.EqualFold(p, path) {
			kept = append(kept, p)
		}
	}
	a.Settings.RecentFiles = kept
	a.saveSettings()
}

// --- autosave and recovery (SPEC-DATA §6) ---

// stepAutosave counts down the interval and writes a recovery copy when the
// document has unsaved work in it.
func (a *App) stepAutosave(dtMillis float64) {
	if a.Headless || !a.Doc().DirtySinceSave {
		a.files.sinceAutosave = 0
		return
	}
	interval := float64(a.Settings.AutosaveSeconds) * 1000
	if interval <= 0 {
		return
	}
	a.files.sinceAutosave += dtMillis
	if a.files.sinceAutosave < interval {
		return
	}
	a.files.sinceAutosave = 0
	a.files.autosaveDue = true
}

// writeDueAutosave runs the write stepAutosave scheduled. It is called from
// the frame loop after EndDrawing, where a slow disk cannot stall a stroke.
func (a *App) writeDueAutosave() {
	if !a.files.autosaveDue {
		return
	}
	a.files.autosaveDue = false
	if a.Doc().DirtySinceSave {
		a.writeAutosave(false)
	}
}

// writeAutosave puts the whole document somewhere it can be found again.
//
// A whole .ship rather than a journal: recovery is then the ordinary load path,
// which is already tested, rather than a second reader that only ever runs on
// somebody's worst day.
func (a *App) writeAutosave(crash bool) {
	path, err := io.AutosavePath(a.files.path, crash)
	if err != nil {
		return
	}
	a.captureCameraState()
	// No thumbnail: a recovery file is opened from a list of names, and
	// rendering one costs a frame that a crash handler does not have.
	if err := io.SaveShip(path, a.Doc(), nil); err != nil {
		return
	}
	_ = io.WriteSidecar(path, a.files.path, crash)
	a.files.autosavePath = path
}

// CrashSave is what the panic handler calls (SPEC-DATA §6).
func (a *App) CrashSave() string {
	if a == nil || a.Bus == nil || !a.Doc().DirtySinceSave {
		return ""
	}
	a.writeAutosave(true)
	return a.files.autosavePath
}

// clearAutosave removes this session's recovery files: there is nothing left
// to recover once the work is on disk under its own name.
func (a *App) clearAutosave() {
	io.ClearOwnAutosaves()
	a.files.autosavePath = ""
	a.files.sinceAutosave = 0
}

// HasRecovery reports whether something was found to offer back.
func (a *App) HasRecovery() bool { return len(a.files.recovery) > 0 }

// RecoverNewest opens the most recent thing found, and forgets the rest.
func (a *App) RecoverNewest() bool {
	if len(a.files.recovery) == 0 {
		return false
	}
	rec := a.files.recovery[0]
	if !a.openShip(rec.Path, false) {
		return false
	}
	// It came back under the autosave's name; the document it belongs to is
	// the one the user thinks they are editing.
	a.files.path = rec.Origin
	a.Doc().DirtySinceSave = true
	a.DiscardRecovery()
	a.Toast(ui.Toast{Text: "Recovered your unsaved work — save it somewhere safe"})
	return true
}

// DiscardRecovery throws away everything that was found.
func (a *App) DiscardRecovery() {
	for _, rec := range a.files.recovery {
		io.DiscardAutosave(rec.Path)
	}
	a.files.recovery = nil
}

// --- exports (SPEC-DATA §5) ---

// runExport asks where to write and writes it.
func (a *App) runExport() {
	formats := io.ExportFormats()
	if a.files.exportFormat < 0 || a.files.exportFormat >= len(formats) {
		a.files.exportFormat = 0
	}
	f := formats[a.files.exportFormat]

	suggested := filepath.Join(a.Settings.LastDir, a.DocumentName()+f.Extension)
	path, ok, err := io.AskExport("Export "+f.Name, suggested, f.Extension, f.Name)
	if err != nil {
		a.Toast(ui.Toast{Text: "The file dialog could not open", Kind: ui.ToastError})
		return
	}
	if !ok {
		return
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		err = a.exportPNG(path)
	case ".stl":
		err = io.ExportSTL(path, a.Doc())
	case ".glb", ".gltf":
		err = io.ExportGLTF(path, a.Doc())
	default:
		err = io.ExportOBJ(path, a.Doc())
	}
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return
	}
	a.Settings.LastDir = filepath.Dir(path)
	a.saveSettings()
	a.Toast(ui.Toast{Text: "Exported " + filepath.Base(path)})
}

// exportPNG renders the current view at the chosen scale.
func (a *App) exportPNG(path string) error {
	img := a.captureView(a.files.exportAlpha)
	if img == nil {
		return fmt.Errorf("the view could not be captured")
	}
	return io.WritePNG(path, img, a.files.exportScale)
}

// captureView renders the viewport off-screen at its own size, without the
// chrome: an exported picture is of the ship, not of the program.
func (a *App) captureView(transparent bool) *image.RGBA {
	if a.Renderer == nil {
		return nil
	}
	vp := a.layout.RenderViewport()
	if vp.W <= 0 || vp.H <= 0 {
		vp = render.Viewport{W: DefaultWindowW, H: DefaultWindowH}
	}
	s := a.BuildScene()
	s.Gizmo = nil
	return a.Renderer.Capture(&s, render.CaptureOpts{
		W: vp.W, H: vp.H, Transparent: transparent,
	})
}

// --- palette import (SPEC-UX §13.3) ---

func (a *App) importPaletteWithDialog() {
	path, ok, err := io.AskOpenPalette(a.Settings.LastDir)
	if err != nil {
		a.Toast(ui.Toast{Text: "The file dialog could not open", Kind: ui.ToastError})
		return
	}
	if !ok {
		return
	}
	if a.ImportPalette(path) {
		a.Settings.LastDir = filepath.Dir(path)
		a.persistPaint()
	}
}

// gatherPaintSettings copies the parts of the brush that live in settings into
// the settings struct, ready to be written (SPEC-DATA §6).
func (a *App) gatherPaintSettings() {
	st := &a.paint
	a.Settings.CustomPalette = append([]color.RGBA(nil), st.custom...)
	a.Settings.RecentColors = st.recents.List()
	a.Settings.Paint = io.PaintSettings{
		Res:    st.res,
		Size:   st.size,
		Dither: st.dither.String(),
		Color:  st.color,
		ColorB: st.colorB,
	}
}

// persistPaint writes them out now rather than at exit, which is what an
// imported palette wants: it took a dialog to get, and losing it to a crash
// would be a poor reward.
func (a *App) persistPaint() { a.saveSettings() }

// restorePaint puts them back at startup.
func (a *App) restorePaint() {
	p := a.Settings.Paint
	st := &a.paint
	if paint.ValidRes(p.Res) {
		st.res = p.Res
	}
	for _, s := range paint.BrushSizes {
		if s == p.Size {
			st.size = p.Size
		}
	}
	if d, ok := paint.ParseDither(p.Dither); ok {
		st.dither = d
	}
	if p.Color.A != 0 {
		st.color = p.Color
	}
	if p.ColorB.A != 0 {
		st.colorB = p.ColorB
	}
}

// WriteCrashLog records what went wrong beside the recovery file, so a crash
// leaves something to read as well as something to reopen (SPEC-DATA §6).
func WriteCrashLog(panicValue any, savedTo string) string {
	dir, err := io.AutosaveDir()
	if err != nil {
		return ""
	}
	name := fmt.Sprintf("crash-%s.log", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)

	var b strings.Builder
	fmt.Fprintf(&b, "Modeler crashed at %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%v\n\n", panicValue)
	if savedTo != "" {
		fmt.Fprintf(&b, "Your work was saved to:\n  %s\nIt is offered back next time the program starts.\n\n", savedTo)
	} else {
		fmt.Fprintf(&b, "There was no unsaved work to recover.\n\n")
	}
	b.Write(debug.Stack())

	if err := io.WriteTextFile(path, b.String()); err != nil {
		return ""
	}
	return path
}

// ExportTo writes an export chosen by its extension, which is what the export
// op and the dialog both come down to.
func (a *App) ExportTo(path string, scale int, transparent bool) error {
	if scale < 1 {
		scale = 1
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		a.files.exportScale, a.files.exportAlpha = scale, transparent
		return a.exportPNG(path)
	case ".stl":
		return io.ExportSTL(path, a.Doc())
	case ".glb", ".gltf":
		return io.ExportGLTF(path, a.Doc())
	case ".obj":
		return io.ExportOBJ(path, a.Doc())
	}
	return fmt.Errorf("%s is not a format this exports", filepath.Ext(path))
}

// BuildSampleShip runs the embedded op script against the live document
// (SPEC-UX §14).
//
// It runs through the same ScriptRunner the headless tests use, which is the
// whole reason the sample is a script: what a first-time user is shown is
// built by the same path a test drives, so it cannot rot without a test going
// red. It must be called outside BeginDrawing — the runner draws its own
// frames.
func (a *App) BuildSampleShip() bool {
	script, err := io.ParseScript(assets.SampleShip)
	if err != nil {
		a.Toast(ui.Toast{Text: "The sample ship could not be read", Kind: ui.ToastError})
		return false
	}
	w, h := a.Renderer.FramebufferSize()
	if w <= 0 || h <= 0 {
		w, h = DefaultWindowW, DefaultWindowH
	}
	runner := NewScriptRunner(a, "", ShotSize{W: w, H: h})
	defer runner.Close()

	if err := runner.Run(script); err != nil {
		a.Toast(ui.Toast{Text: "The sample ship could not be built", Kind: ui.ToastError})
		return false
	}
	// It is a sample, not a document: it has no home on disk, and it is unsaved
	// work from the moment it appears, so Ctrl+S asks where to put it.
	a.files.path = ""
	a.Doc().DirtySinceSave = true
	a.Toast(ui.Toast{Text: "Sample ship — take it apart and see how it was made"})
	return true
}

// --- closing with unsaved work (SPEC-UX §15) ---

// closeState is how far through the "are you sure" the window close has got.
type closeState uint8

const (
	closeNotAsked closeState = iota
	closeAsking
	closeConfirmed
)

// RequestClose is what the window's X and Alt+F4 come down to.
//
// The autosave is a safety net, not an answer: it lives in a folder the user
// has never seen, under a name they did not choose. Work that has a name it
// could be saved under deserves to be asked about.
func (a *App) RequestClose() {
	if !a.Doc().DirtySinceSave {
		a.closing = closeConfirmed
		return
	}
	if a.closing == closeNotAsked {
		a.closing = closeAsking
		a.UI.ShowModal(ui.ModalState{
			Title:       "Save before closing?",
			Body:        a.DocumentName() + " has changes that are not saved.",
			ConfirmText: "Save",
			CancelText:  "Close without saving",
		})
	}
}

// ShouldClose reports whether the window may go now.
func (a *App) ShouldClose() bool { return a.closing == closeConfirmed }

// stepClose runs the prompt's answer. It is called after the frame, where the
// save dialog is allowed to open.
func (a *App) stepClose() {
	if a.closing != closeAsking || a.UI.ModalOpen() {
		return
	}
	switch a.closeAnswer {
	case closeAnswerSave:
		if a.Save() {
			a.closing = closeConfirmed
			return
		}
		// The save was cancelled or failed, so neither has the close.
		a.closing = closeNotAsked
	case closeAnswerDiscard:
		a.closing = closeConfirmed
	default:
		a.closing = closeNotAsked
	}
	a.closeAnswer = closeAnswerNone
}

// closeAnswer is what the prompt was told.
type closeAnswerKind uint8

const (
	closeAnswerNone closeAnswerKind = iota
	closeAnswerSave
	closeAnswerDiscard
)
