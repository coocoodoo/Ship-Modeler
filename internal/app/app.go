package app

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

// Version is shown in the hint bar's right corner.
const Version = "v1.0.0"

// Mode is the app's state machine (SPEC-UX §1). Exactly one is active. M1 only
// implements Idle; the rest arrive with the tools that own them.
type Mode uint8

const (
	ModeIdle Mode = iota
	ModeSketch
	ModeExtrude
	ModeBoolean
	ModePaint
	ModeChamfer
)

func (m Mode) String() string {
	switch m {
	case ModeSketch:
		return "Sketch"
	case ModeExtrude:
		return "Extrude"
	case ModeBoolean:
		return "Boolean"
	case ModePaint:
		return "Paint"
	case ModeChamfer:
		return "Chamfer"
	default:
		return "Idle"
	}
}

// App is the running application: window, renderer, document and UI.
type App struct {
	workflow workflowState
	ai       *aiConnection
	aiToggle bool
	Renderer *render.Renderer
	Fonts    *ui.Fonts
	UI       *ui.Context
	Scale    float64
	Settings *io.Settings

	Bus *model.Bus
	Sel model.Selection

	Camera render.Camera
	Anim   render.CameraAnim

	Cube  scene.ViewCube
	Triad scene.Triad

	Mode  Mode
	Hover render.PickResult
	// TreeHover mirrors the tree row under the cursor into the viewport, and
	// the viewport's hover back into the tree, so highlighting is bidirectional
	// (SPEC-UX §7).
	TreeHover model.Ref

	// hintOverride is a transient message a widget wants shown instead of the
	// standing hint. It is cleared at the start of every frame.
	hintOverride string

	// gpu caches the render form of each body, dropped when the bus reports the
	// body changed.
	gpu map[uint32]*render.BodyGPU

	tree          treeState
	sketch        sketchState
	extrude       extrudeState
	boolean       booleanState
	pushPull      pushPullState
	transform     transformState
	box           boxSelectState
	paint         paintState
	uv            uvViewState
	files         fileState
	markers       markerState
	notePins      notePinState
	moveTexture   moveTextureState
	material      materialState
	chamfer       chamferState
	bodyClipboard bodyClipboard
	library       partLibraryState

	// hoverSketch is the visible sketch under the pointer, which the ID buffer
	// cannot report because an overlay is not geometry.
	hoverSketch *model.Sketch

	// cubeDrag, orbiting and panning track camera navigation drags.
	cubeDrag          bool
	orbiting, panning bool

	// pickCooldown throttles the ID pass to roughly 30 Hz on hover.
	pickCooldown float64

	// showShortcuts toggles the `?` overlay.
	showShortcuts   bool
	showSettings    bool
	settingsScroll  float64
	settingsEffects bool
	effectsPreview  float64
	pendingUISize   bool

	// lastMouse remembers the cursor so the draw pass can re-run the widget
	// code with the same hover states the update pass saw.
	lastMouseX, lastMouseY float64

	// cursor is the pointer shape set last frame, so it is only changed when it
	// actually changes (SPEC-UX §15).
	cursor int32

	// closing tracks the "save before closing?" prompt.
	closing     closeState
	closeAnswer closeAnswerKind

	// Headless suppresses window presentation and drives a virtual clock.
	Headless bool
	Viewer   bool // Opened from a file association; Edit explicitly unlocks the workspace.

	layout Layout
}

// New creates the application state. A GL context must already exist.
func New(headless bool) *App {
	// A headless run must not inherit the user's saved panel width or collapse
	// state: golden shots have to depend on the document alone.
	settings, settingsErr := io.DefaultSettings(), error(nil)
	themeWarn := ""
	ui.ResetTheme()
	if !headless {
		settings, settingsErr = io.LoadSettings()
		// The theme file lives beside the settings. Headless runs keep the
		// built-in palette so golden shots depend on nothing outside the repo.
		themeWarn = loadAppearance(settings.Theme)
		if p, ok := ui.FindPalette(settings.AppearancePalette); ok {
			ui.ApplyPalette(p.Name)
			settings.Theme = "light"
			if p.Dark {
				settings.Theme = "dark"
			}
		} else {
			settings.AppearancePalette = ""
		}
	}

	uiScale := settings.UIScaleOverride
	if uiScale == 0 {
		uiScale = float64(rl.GetWindowScaleDPI().X)
	}
	uiScale = ui.Scale(uiScale)

	fonts := ui.LoadFonts(uiScale)
	a := &App{
		Renderer: render.NewRenderer(),
		Fonts:    fonts,
		UI:       ui.NewContext(fonts, uiScale),
		Scale:    uiScale,
		Settings: settings,
		Bus:      model.NewBus(model.NewDocument()),
		Camera:   render.DefaultCamera(),
		gpu:      map[uint32]*render.BodyGPU{},
		Headless: headless,
	}
	a.tree.init(settings)
	a.UI.Motion = settings.UIMotion
	if settings.UIEffects != nil {
		a.UI.Effects = *settings.UIEffects
	}
	// No dot is under the pointer until one is, and index 0 is a real dot.
	a.markers.hover = -1
	a.initPaint()
	if !headless {
		// The tile setup persists like the palette does; a headless run must
		// not inherit it, for the same reason it ignores the settings file.
		a.restoreTileset()
	}
	a.restorePaint()
	a.initFiles()
	a.initPartLibrary()
	a.Bus.Events.Listen(a.onDocumentEvent)

	if settingsErr != nil && !headless {
		a.Toast(ui.Toast{
			Text: "Settings couldn't be read — using defaults",
			Kind: ui.ToastWarn,
		})
	}
	if themeWarn != "" {
		a.Toast(ui.Toast{Text: themeWarn, Kind: ui.ToastWarn})
	}
	return a
}

// Close releases GPU resources and persists the settings.
func (a *App) Close() {
	for _, t := range a.workflow.thumbnails {
		if t.ID != 0 {
			rl.UnloadTexture(t)
		}
	}
	a.closeMoveTexture()
	a.closeMaterialPanel()
	a.dropUVTextures()
	a.stopAI()
	a.dropLibraryPreview()
	a.dropChamferPreview()
	a.storeTileSettings()
	a.saveSettings()
	a.dropTilePickerTexture()
	for _, g := range a.gpu {
		g.Unload()
	}
	a.gpu = nil
	a.Fonts.Unload()
	ui.UnloadIcons()
	a.Renderer.Close()
}

// Doc is the live document.
func (a *App) Doc() *model.Document { return a.Bus.Doc() }

// saveSettings writes the window rect and panel state back to disk. A failure
// here is not worth interrupting a shutdown over.
func (a *App) saveSettings() {
	if a.Headless {
		return
	}
	if !rl.IsWindowFullscreen() && !rl.IsWindowMinimized() && !rl.IsWindowMaximized() {
		pos := rl.GetWindowPosition()
		a.Settings.Window = io.WindowRect{
			X: int(pos.X), Y: int(pos.Y),
			Width: rl.GetScreenWidth(), Height: rl.GetScreenHeight(),
		}
	}
	a.Settings.TreePanelWidth = int(a.tree.width)
	a.Settings.TreeCollapsed = a.tree.collapsed
	a.gatherPaintSettings()
	_ = a.Settings.Save()
}

// onDocumentEvent keeps the derived GPU caches in step with the document.
func (a *App) onDocumentEvent(ev model.Event) {
	if a.moveTexture.open && (ev.Kind == model.EvDocReplaced || ((ev.Kind == model.EvBodyChanged || ev.Kind == model.EvBodyRemoved || ev.Kind == model.EvBodyPainted) && ev.BodyID == a.moveTexture.body)) {
		a.closeMoveTexture()
	}
	a.onUVEvent(ev)
	a.onExtrudeTargetEvent(ev)
	if a.InChamfer() {
		a.CancelEdgeChamfer()
	}
	switch ev.Kind {
	case model.EvBodyChanged, model.EvBodyAdded, model.EvBodyRemoved:
		a.dropGPU(ev.BodyID)
	case model.EvBodyPainted:
		// Texels changed and nothing else did, so the body's mesh and its atlas
		// layout still stand: re-upload the rectangle the stroke wrote rather
		// than rebuilding and re-uploading the whole body. That difference is
		// what makes a stroke feel like a stroke.
		if g, ok := a.gpu[ev.BodyID]; ok && !g.UpdatePaint(ev.Paint, ev.Rect) {
			a.dropGPU(ev.BodyID)
		}
	case model.EvDocReplaced:
		for id := range a.gpu {
			a.dropGPU(id)
		}
	}
	a.Sel.Prune(a.Doc())
}

func (a *App) dropGPU(id uint32) {
	if g, ok := a.gpu[id]; ok {
		g.Unload()
		delete(a.gpu, id)
	}
}

// bodyGPU returns a body's uploaded render form, building it on first use.
func (a *App) bodyGPU(b *model.Body) *render.BodyGPU {
	if g, ok := a.gpu[b.ID]; ok {
		return g
	}
	g := render.BuildBodyGPU(b.Mesh)
	// Ambient occlusion is evaluated by the renderer each frame across bodies.
	g.Upload()
	a.gpu[b.ID] = g
	return g
}

// Toast shows a transient message.
func (a *App) Toast(t ui.Toast) { a.UI.ShowToast(t) }

// Run executes a command through the bus, reporting a failure as an error toast
// rather than swallowing it (SPEC-DATA §3.1).
func (a *App) Run(cmd model.Command) bool {
	if a.Viewer {
		return false
	}
	if err := a.Bus.Run(cmd); err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	return true
}

// Undo and Redo drive the history and announce what moved.
func (a *App) Undo() {
	if a.Viewer {
		return
	}
	if name, ok := a.Bus.Undo(); ok {
		a.Toast(ui.Toast{Text: "Undid: " + name})
	}
}

func (a *App) Redo() {
	if a.Viewer {
		return
	}
	if name, ok := a.Bus.Redo(); ok {
		a.Toast(ui.Toast{Text: "Redid: " + name})
	}
}

// Layout computes this frame's chrome geometry.
func (a *App) Layout(fbW, fbH int) Layout {
	if a.Viewer {
		return viewerLayout(fbW, fbH, a.Scale)
	}
	return ComputeLayout(fbW, fbH, a.Scale, a.tree.width, a.tree.collapsed, a.paintBarWidth())
}

// Viewport returns the 3D viewport for a framebuffer size.
func (a *App) Viewport(fbW, fbH int) render.Viewport {
	return a.Layout(fbW, fbH).RenderViewport()
}

// Frame runs one whole frame: logic, then the 3D view, then the chrome. It
// must be called between BeginDrawing and EndDrawing.
//
// The widget kit runs exactly once per frame. An immediate-mode kit holds real
// state — which widget is being dragged, where the caret is, how long a tooltip
// has waited — so running it twice would corrupt that, and running it outside
// the drawing block would queue geometry into the wrong framebuffer.
func (a *App) Frame(in InputFrame) {
	a.applyPendingUISize()
	a.update(in)
	a.draw(in)
	// After both, because what the pointer will do depends on what the widgets
	// decided this frame as well as on the mode.
	a.updateCursor(in)
	a.drawCustomCursor(in)
}

// update advances the non-drawing logic: camera, picking and keys.
//
// Whether the pointer belongs to the chrome or to the viewport is decided
// geometrically here rather than by asking the widget kit, so this can run
// before the widgets do and still be exact.
func (a *App) update(in InputFrame) {
	a.sketch.fineGrid = in.Ctrl
	a.lastMouseX, a.lastMouseY = in.MouseX, in.MouseY
	a.hintOverride = ""
	// The sidebar's width is part of the layout, so it is eased before the
	// layout is measured rather than a frame behind it.
	a.stepPaintBar(in.DeltaMillis)
	a.layout = a.Layout(in.WindowW, in.WindowH)
	vp := a.layout.RenderViewport()

	// The camera animation runs first so input takes over from wherever it has
	// reached rather than snapping (SPEC-RENDER §7).
	if cam, changed := a.Anim.Step(a.Camera, in.DeltaMillis); changed {
		a.Camera = cam
	}

	a.stepAutosave(in.DeltaMillis)
	a.Cube.Layout(a.Camera, vp, a.Scale)
	a.Cube.Flat = a.Settings.FlatShading
	a.Triad.Layout(vp, a.Scale)
	a.Cube.Update(in.MouseX, in.MouseY)
	if a.Viewer {
		a.updateViewer(in, vp)
		return
	}
	a.prepareUVView()
	if a.moveTexture.open {
		a.updateMoveTexture(in)
		return
	}
	if a.notePins.armed && in.KeyPressed(rl.KeyEscape) {
		a.notePins.armed = false
		return
	}
	if a.showSettings {
		return
	}
	a.chamfer.gizmo.hovered, a.chamfer.gizmo.captured = false, false
	if a.InChamfer() && a.chamfer.dirty {
		a.chamfer.wait -= in.DeltaMillis
		if a.chamfer.wait <= 0 {
			a.rebuildChamferPreview()
		}
	}

	if a.InChamfer() && a.chamfer.gizmo.dragging {
		// A captured drag must finish even if released over a panel or outside
		// the viewport. Its release cannot activate a control beneath it.
		a.updateChamferGizmo(in, vp)
	} else if a.uvOwnsPointer(in) {
		a.updateUVView(in, vp)
	} else if !a.chromeOwnsPointer(in) {
		a.handleCubeInput(in, vp)
		a.handleCameraInput(in, vp)
		if a.updateNotePinInput(in, vp) {
			// Note placement owns the left button.
		} else if a.InChamfer() {
			a.updateChamfer(in, vp)
		} else if a.InExtrude() {
			a.updateExtrude(in, vp)
		} else if a.InBoolean() {
			a.updateBoolean(in, vp)
		} else if a.InSketch() {
			a.updateSketch(in, vp)
		} else if a.InPaint() {
			// Paint mode owns the left button outright. Nothing else may be
			// hit-tested under the cursor: the selection tools, the box
			// rectangle and both gizmos are all disarmed rather than merely
			// undrawn, which is the trap M6 fell into.
			a.updatePaint(in, vp)
		} else {
			// The handles get first refusal on a click, in the order they are
			// drawn: nothing armed on the selection may be picked out from
			// under its own gizmo.
			a.updateTransform(in, vp)
			if !a.grabbedGizmo() {
				a.updatePushPull(in, vp)
			}
			if !a.grabbedGizmo() && !a.grabbedArrow() {
				a.updateBoxSelect(in, vp)
				if !a.box.active {
					a.handleViewportClick(in, vp)
				}
			}
			a.updateHover(in, vp)
		}
	} else {
		// A camera drag that is already running keeps running wherever the
		// pointer goes. An orbit is decided when the button goes down, not
		// re-litigated every pixel: without this it froze the moment the
		// pointer crossed the toolbar or the tree and resumed on the way back,
		// which read as the camera stuttering. New drags still cannot start
		// over chrome — Pressed is only honoured inside the viewport.
		navDragging := (a.orbiting || a.panning || a.cubeDrag) &&
			!a.UI.ModalOpen() && !a.showShortcuts
		if a.cardOnlyOwnsPointer(in) || navDragging {
			a.handleCubeInput(in, vp)
			a.handleCameraInput(in, vp)
		}
		a.Hover = render.PickResult{}
		a.sketch.hasSnap = false
		// A stroke that runs off the viewport and is released over the palette
		// still ends there. Leaving it open until the pointer wandered back
		// would put the next click's texels on the end of the last stroke.
		if a.InPaint() {
			a.paintChromeFrame(in)
		}
	}
	a.handleDroppedFiles(in)

	// The gizmos follow the selection, not the pointer. Arming them inside the
	// branch above would leave them stale whenever the cursor happened to be
	// over the tree — including in every headless run, where it always is.
	if a.Mode == ModeIdle {
		a.armPushPull()
		a.armTransform()
	} else {
		a.dropPushPull()
		a.transform.tool = nil
	}
	// Keyboard shortcuts never depend on where the pointer is: pressing L while
	// the cursor rests over the tree panel must still pick the Line tool.
	//
	// A modal owns the keyboard outright. Without this, Escape on "Save before
	// closing?" also fell through to the mode underneath — and in paint mode
	// that left the modal to read the same Escape as its own answer, closing
	// the program without saving. The shortcut sheet owns it the same way: a
	// sheet explaining the S key must not be the thing the S key acts through.
	if !a.workflow.open && !a.notePins.open && !a.markers.attachmentOpen && !a.libraryOwnsInput() && !a.pixelPasteMenuOpen() && !a.UI.WantKeyboard() && !a.UI.ModalOpen() {
		if a.showShortcuts {
			if in.KeyPressed(rl.KeyEscape) || (in.KeyPressed(rl.KeySlash) && in.Shift) {
				a.showShortcuts = false
			}
			return
		}
		switch {
		case a.InChamfer():
			a.handleGlobalKeys(in, vp)
			a.handleChamferKeys(in)
		case a.InExtrude():
			a.handleGlobalKeys(in, vp)
			a.handleExtrudeKeys(in)
		case a.InBoolean():
			a.handleGlobalKeys(in, vp)
			a.handleBooleanKeys(in)
		case a.InSketch():
			a.handleGlobalKeys(in, vp)
			a.handleSketchKeys(in)
		case a.InPaint():
			a.handleGlobalKeys(in, vp)
			a.handlePaintKeys(in)
		default:
			a.handleKeys(in, vp)
			a.handleTransformKeys(in)
		}
	}
}

// chromeOwnsPointer reports whether the toolbar, tree, an overlay or a live
// widget drag has the pointer, in which case the viewport ignores it.
func (a *App) chromeOwnsPointer(in InputFrame) bool {
	if a.uvOwnsPointer(in) {
		return true
	}
	if a.showSettings {
		return true
	}
	if a.workflow.open || a.notePins.open || a.markers.attachmentOpen || a.libraryOwnsInput() {
		return true
	}
	if a.pixelPasteMenuOpen() {
		return true
	}
	if a.showShortcuts || a.UI.ModalOpen() {
		return true
	}
	if a.UI.Dragging() {
		return true
	}
	if a.UI.OverlayCapturesPointer(in.MouseX, in.MouseY) {
		return true
	}
	// A floating card sits inside the viewport but owns what is under it. Without
	// this a click on the palette panel also painted the face behind the panel,
	// and every chip left a dab on the model.
	if a.UI.CardCapturesPointer(in.MouseX, in.MouseY) {
		return true
	}
	// The sketch toolbar's variant list hangs off the toolbar and over both the
	// tree and the viewport. It has to be found here, in update, because a card
	// is only known to the kit a frame after it is drawn — and a menu that
	// appeared this frame would otherwise let its own clicks through to
	// whatever is behind it (V-117).
	if a.flyoutOwnsPointer(in.MouseX, in.MouseY) {
		return true
	}
	return !rl.CheckCollisionPointRec(
		rl.Vector2{X: float32(in.MouseX), Y: float32(in.MouseY)}, a.layout.Viewport)
}

// cardOnlyOwnsPointer reports that the one thing between the pointer and the
// model is a floating card.
//
// Camera navigation still runs there. A card is inside the viewport, and
// orbiting from wherever the pointer happens to be is how this program moves
// (SPEC-UX §1); it is only the left button that belongs to the card. Over the
// toolbar or the tree — real chrome — nothing does.
func (a *App) cardOnlyOwnsPointer(in InputFrame) bool {
	if a.uvOwnsPointer(in) {
		return false
	}
	if a.showSettings {
		return false
	}
	if a.workflow.open || a.notePins.open || a.markers.attachmentOpen || a.libraryOwnsInput() {
		return false
	}
	if a.pixelPasteMenuOpen() {
		return false
	}
	if a.showShortcuts || a.UI.ModalOpen() || a.UI.Dragging() {
		return false
	}
	if a.flyoutOwnsPointer(in.MouseX, in.MouseY) {
		return false
	}
	if a.UI.OverlayCapturesPointer(in.MouseX, in.MouseY) {
		return false
	}
	return rl.CheckCollisionPointRec(
		rl.Vector2{X: float32(in.MouseX), Y: float32(in.MouseY)}, a.layout.Viewport)
}

// draw paints the 3D view and then the chrome over it.
func (a *App) draw(in InputFrame) {
	a.UI.AdvanceAppearance(in.DeltaMillis)
	// The accent is the mode's, and everything below — the renderer's
	// selection tint, the widgets, the cards — reads it (SPEC-UX §3.1).
	ui.SetAccent(a.modeAccent())
	fbW, fbH := in.WindowW, in.WindowH
	a.Renderer.SetFramebuffer(fbW, fbH)
	l := a.layout
	vp := l.RenderViewport()

	a.Renderer.ClearColor()
	s := a.BuildScene()
	a.Renderer.DrawViewport(&s, vp)

	scene.DrawPlaneLabels(a.Camera, vp, s.Planes, a.Fonts, a.Scale)
	a.Cube.Draw(a.Fonts, a.Scale)
	a.Triad.Draw(a.Camera, a.Fonts, a.Scale)

	a.UI.Screen = l.Screen
	a.UI.Begin(in.ToUI())
	chrome := func() {
		if a.Viewer {
			a.buildViewerShell(l)
			a.UI.DrawToasts(l.Viewport)
			return
		}
		a.drawAttachmentLabels(vp)
		a.drawNotePins(vp)
		if a.InChamfer() {
			a.drawChamferGizmoLabel(vp)
			if a.chamfer.gizmo.captured {
				a.UI.ClaimPointer(l.Screen)
			}
		}
		if a.uvBlocked() {
			a.UI.DrawBackground(a.buildUVView)
		} else {
			a.buildUVView()
		}
		a.buildPixelPasteMenu()
		a.buildBodyMenu()
		a.buildAttachmentDialog()
		a.buildLibraryDialogs()
		if a.workflow.open || a.notePins.open || a.moveTexture.open {
			a.UI.DrawBackground(func() { a.buildShell(l) })
		} else {
			a.buildShell(l)
		}
		a.buildNotePinsDialog()
		a.buildWorkflow()
		a.buildMoveTexture()
		if a.workflow.open {
			// Workshop notices are shown inline in its fixed footer.
		} else if a.libraryOwnsInput() {
			a.UI.DrawLatestToast(l.Viewport)
		} else {
			a.UI.DrawToasts(l.Viewport)
		}
		if a.showShortcuts {
			a.UI.DrawShortcutOverlay(l.Screen, shortcutSheet())
		}
	}
	if a.showSettings {
		a.UI.DrawBackground(chrome)
	} else {
		chrome()
	}
	a.buildSettingsDialog()
	a.routeModalAnswer(a.UI.DrawModal(l.Screen))
	a.UI.End()
}

// routeModalAnswer connects a dialog's outcome to whichever question put it up.
//
// Confirm and the labelled cancel button each mean what they say. Escape is
// neither: it dismisses the question and keeps things exactly as they were,
// because a reflex must never be the thing that throws work away.
func (a *App) routeModalAnswer(res ui.ModalResult) {
	if !res.Confirmed && !res.Alt && !res.Cancelled && !res.Dismissed {
		return
	}
	switch {
	case a.notePins.clearing:
		a.notePins.clearing = false
		if res.Confirmed {
			a.Run(&model.ClearNotePins{})
			a.notePins.page = 0
		}
	case a.closing == closeAsking:
		switch {
		case res.Confirmed:
			a.closeAnswer = closeAnswerSave
		case res.Cancelled:
			a.closeAnswer = closeAnswerDiscard
		default:
			a.closeAnswer = closeAnswerNone
		}
	case a.files.confirm != fileNone:
		// The gate in front of New, Open and the sample. Save runs first and
		// keeps the action parked until it succeeds; Discard lets it straight
		// through; anything else drops it (V-143).
		switch {
		case res.Confirmed:
			a.files.pending = fileSaveThen
		case res.Alt:
			a.files.pending, a.files.pendingPath = a.files.confirm, a.files.confirmPath
			a.files.confirm, a.files.confirmPath = fileNone, ""
		default:
			a.files.confirm, a.files.confirmPath = fileNone, ""
		}
	}
}

// HintText is what the hint bar says right now. It never returns an empty
// string: silence is never the answer (SPEC-UX §1).
func (a *App) HintText() string {
	if a.InChamfer() {
		return "Chamfer · drag arrow: 0.25 u steps · click edges to add/remove · Enter applies · Esc cancels"
	}
	if a.hintOverride != "" {
		return a.hintOverride
	}
	if a.notePins.armed {
		return "Click a model face to drop a note pin - Esc cancels"
	}
	if a.markers.armed {
		return a.markerHint()
	}
	if a.sketch.awaitingPlane {
		return "Click a plane to sketch on it · Esc to cancel"
	}
	if a.InExtrude() {
		return a.extrudeHint()
	}
	if a.InBoolean() {
		return a.booleanHint()
	}
	if a.InPaint() {
		return a.paintHint()
	}
	if a.InPushPull() {
		return a.pushPullHint()
	}
	if a.InTransform() {
		return a.transformHint()
	}
	if a.InSketch() {
		return a.sketchHint()
	}
	if a.Cube.HoverHome {
		return "Home view"
	}
	if a.Cube.HoverShade {
		if a.Settings.FlatShading {
			return "Flat view — click for shading"
		}
		return "Shaded view — click for flat colours"
	}
	if a.Cube.Hover.Valid() {
		if l := a.Cube.Hover.Label(); l != "" {
			return "View: " + l
		}
		return "Snap the camera to this corner"
	}
	if a.TreeHover.Kind != model.SelNone {
		return a.describeRef(a.TreeHover)
	}
	if a.Hover.Hit {
		switch a.Hover.Kind {
		case render.PickFace:
			return fmt.Sprintf("Face %d of %s", a.Hover.FaceUID.Seq(), a.bodyName(a.Hover.BodyID))
		case render.PickEdge:
			return "Edge of " + a.bodyName(a.Hover.BodyID)
		case render.PickVert:
			return "Vertex of " + a.bodyName(a.Hover.BodyID)
		case render.PickPlane:
			return a.Hover.Plane.String() + " plane · click to select it"
		}
	}
	if !a.Sel.Empty() {
		return "Selected: " + a.Sel.Describe(a.Doc())
	}
	if a.Doc().IsEmpty() {
		return "Click a plane (or press S) to start your first sketch"
	}
	return "Click a plane or a flat face to start a sketch"
}

func (a *App) describeRef(r model.Ref) string {
	switch r.Kind {
	case model.SelPlane:
		return r.Plane.String() + " plane · click to select it"
	case model.SelBody:
		return a.bodyName(r.Body)
	case model.SelMarker:
		return a.Doc().MarkerLabel(r.Marker) + " · drag the gizmo to move it"
	case model.SelSketch:
		if s := a.Doc().SketchByID(r.Sketch); s != nil {
			if len(s.Arrangement().Regions) > 0 {
				return s.Name + " · click to select it, then E to extrude"
			}
			return s.Name
		}
	}
	return r.Kind.String()
}

func (a *App) bodyName(id uint32) string {
	if b := a.Doc().BodyByID(id); b != nil {
		return b.Name
	}
	return fmt.Sprintf("Body %d", id)
}

// SetHint overrides the hint bar for this frame.
func (a *App) SetHint(s string) { a.hintOverride = s }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// shortcutSheet is the keyboard map the `?` overlay lists (SPEC-UX §16).
func shortcutSheet() []ui.Shortcut {
	return []ui.Shortcut{
		{Section: "Navigate"},
		{Keys: "Right-drag", Description: "Orbit"},
		{Keys: "Middle-drag", Description: "Pan"},
		{Keys: "Shift+Right-drag", Description: "Pan"},
		{Keys: "Wheel", Description: "Zoom to cursor"},
		{Keys: "F", Description: "Frame selection"},
		{Keys: "Shift+F", Description: "Look square-on at the selected plane or face"},
		{Keys: "O", Description: "Orthographic / perspective"},
		{Section: "Tools"},
		{Keys: "S", Description: "Sketch"},
		{Keys: "E", Description: "Extrude"},
		{Keys: "B", Description: "Boolean"},
		{Keys: "M", Description: "Move / transform"},
		{Keys: "P", Description: "Paint"},
		{Section: "Sketch tools"},
		{Keys: "V", Description: "Select"},
		{Keys: "L / R / C", Description: "Line / rectangle / circle"},
		{Keys: "A / P / O", Description: "Arc / polygon / slot"},
		{Keys: "S / .", Description: "Spline / point"},
		{Keys: "same key again", Description: "Next variant in that group"},
		{Keys: "Q", Description: "Construction geometry"},
		{Section: "Paint tools"},
		{Keys: "D / B", Description: "Pencil / soft brush"},
		{Keys: "E / G / I", Description: "Eraser / fill / pick"},
		{Keys: "L / R / C", Description: "Line / rectangle / circle"},
		{Keys: "N", Description: "Gradient"},
		{Keys: "X", Description: "Swap the two colours"},
		{Keys: "W", Description: "Magic wand: select by colour, then paint inside"},
		{Keys: "U", Description: "Select pixels: drag a rectangle; Shift makes a square"},
		{Keys: "Ctrl+C / Ctrl+V", Description: "Copy/paste bodies, or selected pixels in Paint"},
		{Keys: "K", Description: "Edge line: pick edges, bake a band"},
		{Section: "Files"},
		{Keys: "Ctrl+N", Description: "New ship"},
		{Keys: "Ctrl+O", Description: "Open"},
		{Keys: "Ctrl+S", Description: "Save"},
		{Keys: "Ctrl+Shift+S", Description: "Save as"},
		{Keys: "Ctrl+E", Description: "Export"},
		{Keys: "Ctrl+I", Description: "Import an STL or OBJ mesh"},
		{Section: "Edit"},
		{Keys: "Ctrl+Z", Description: "Undo"},
		{Keys: "Ctrl+Y", Description: "Redo"},
		{Keys: "Del", Description: "Delete selection"},
		{Keys: "H", Description: "Hide selection"},
		{Keys: "Esc", Description: "Cancel / back one level"},
		{Keys: "Enter", Description: "Confirm"},
		{Section: "Held"},
		{Keys: "Alt", Description: "Suppress snapping"},
		{Keys: "Ctrl", Description: "Fine snap (1/4 u)"},
		{Keys: "Shift", Description: "Add to selection"},
		{Keys: "?", Description: "This sheet"},
	}
}
