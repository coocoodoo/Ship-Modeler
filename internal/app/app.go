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
const Version = "v0.2.0-m1"

// Mode is the app's state machine (SPEC-UX §1). Exactly one is active. M1 only
// implements Idle; the rest arrive with the tools that own them.
type Mode uint8

const (
	ModeIdle Mode = iota
	ModeSketch
	ModeExtrude
	ModeBoolean
	ModePaint
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
	default:
		return "Idle"
	}
}

// App is the running application: window, renderer, document and UI.
type App struct {
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

	tree      treeState
	sketch    sketchState
	extrude   extrudeState
	boolean   booleanState
	pushPull  pushPullState
	transform transformState
	box       boxSelectState

	// hoverSketch is the visible sketch under the pointer, which the ID buffer
	// cannot report because an overlay is not geometry.
	hoverSketch *model.Sketch

	// cubeDrag, orbiting and panning track camera navigation drags.
	cubeDrag          bool
	orbiting, panning bool

	// pickCooldown throttles the ID pass to roughly 30 Hz on hover.
	pickCooldown float64

	// showShortcuts toggles the `?` overlay.
	showShortcuts bool

	// lastMouse remembers the cursor so the draw pass can re-run the widget
	// code with the same hover states the update pass saw.
	lastMouseX, lastMouseY float64

	// Headless suppresses window presentation and drives a virtual clock.
	Headless bool

	layout Layout
}

// New creates the application state. A GL context must already exist.
func New(headless bool) *App {
	// A headless run must not inherit the user's saved panel width or collapse
	// state: golden shots have to depend on the document alone.
	settings, settingsErr := io.DefaultSettings(), error(nil)
	if !headless {
		settings, settingsErr = io.LoadSettings()
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
	a.Bus.Events.Listen(a.onDocumentEvent)

	if settingsErr != nil && !headless {
		a.Toast(ui.Toast{
			Text: "Settings couldn't be read — using defaults",
			Kind: ui.ToastWarn,
		})
	}
	return a
}

// Close releases GPU resources and persists the settings.
func (a *App) Close() {
	a.saveSettings()
	for _, g := range a.gpu {
		g.Unload()
	}
	a.gpu = nil
	a.Fonts.Unload()
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
	if !rl.IsWindowFullscreen() && !rl.IsWindowMinimized() {
		pos := rl.GetWindowPosition()
		a.Settings.Window = io.WindowRect{
			X: int(pos.X), Y: int(pos.Y),
			Width: rl.GetScreenWidth(), Height: rl.GetScreenHeight(),
		}
	}
	a.Settings.TreePanelWidth = int(a.tree.width)
	a.Settings.TreeCollapsed = a.tree.collapsed
	_ = a.Settings.Save()
}

// onDocumentEvent keeps the derived GPU caches in step with the document.
func (a *App) onDocumentEvent(ev model.Event) {
	switch ev.Kind {
	case model.EvBodyChanged, model.EvBodyAdded, model.EvBodyRemoved:
		a.dropGPU(ev.BodyID)
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
	g.Upload()
	a.gpu[b.ID] = g
	return g
}

// Toast shows a transient message.
func (a *App) Toast(t ui.Toast) { a.UI.ShowToast(t) }

// Run executes a command through the bus, reporting a failure as an error toast
// rather than swallowing it (SPEC-DATA §3.1).
func (a *App) Run(cmd model.Command) bool {
	if err := a.Bus.Run(cmd); err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	return true
}

// Undo and Redo drive the history and announce what moved.
func (a *App) Undo() {
	if name, ok := a.Bus.Undo(); ok {
		a.Toast(ui.Toast{Text: "Undid: " + name})
	}
}

func (a *App) Redo() {
	if name, ok := a.Bus.Redo(); ok {
		a.Toast(ui.Toast{Text: "Redid: " + name})
	}
}

// Layout computes this frame's chrome geometry.
func (a *App) Layout(fbW, fbH int) Layout {
	return ComputeLayout(fbW, fbH, a.Scale, a.tree.width, a.tree.collapsed)
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
	a.update(in)
	a.draw(in)
}

// update advances the non-drawing logic: camera, picking and keys.
//
// Whether the pointer belongs to the chrome or to the viewport is decided
// geometrically here rather than by asking the widget kit, so this can run
// before the widgets do and still be exact.
func (a *App) update(in InputFrame) {
	a.lastMouseX, a.lastMouseY = in.MouseX, in.MouseY
	a.hintOverride = ""
	a.layout = a.Layout(in.WindowW, in.WindowH)
	vp := a.layout.RenderViewport()

	// The camera animation runs first so input takes over from wherever it has
	// reached rather than snapping (SPEC-RENDER §7).
	if cam, changed := a.Anim.Step(a.Camera, in.DeltaMillis); changed {
		a.Camera = cam
	}

	a.Cube.Layout(a.Camera, vp, a.Scale)
	a.Triad.Layout(vp, a.Scale)
	a.Cube.Update(in.MouseX, in.MouseY)

	if !a.chromeOwnsPointer(in) {
		a.handleCubeInput(in, vp)
		a.handleCameraInput(in, vp)
		if a.InExtrude() {
			a.updateExtrude(in, vp)
		} else if a.InBoolean() {
			a.updateBoolean(in, vp)
		} else if a.InSketch() {
			a.updateSketch(in, vp)
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
		a.Hover = render.PickResult{}
		a.sketch.hasSnap = false
	}

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
	if !a.UI.WantKeyboard() {
		switch {
		case a.InExtrude():
			a.handleGlobalKeys(in, vp)
			a.handleExtrudeKeys(in)
		case a.InBoolean():
			a.handleGlobalKeys(in, vp)
			a.handleBooleanKeys(in)
		case a.InSketch():
			a.handleGlobalKeys(in, vp)
			a.handleSketchKeys(in)
		default:
			a.handleKeys(in, vp)
			a.handleTransformKeys(in)
		}
	}
}

// chromeOwnsPointer reports whether the toolbar, tree, an overlay or a live
// widget drag has the pointer, in which case the viewport ignores it.
func (a *App) chromeOwnsPointer(in InputFrame) bool {
	if a.showShortcuts || a.UI.ModalOpen() {
		return true
	}
	if a.UI.Dragging() {
		return true
	}
	if a.UI.OverlayCapturesPointer(in.MouseX, in.MouseY) {
		return true
	}
	return !rl.CheckCollisionPointRec(
		rl.Vector2{X: float32(in.MouseX), Y: float32(in.MouseY)}, a.layout.Viewport)
}

// draw paints the 3D view and then the chrome over it.
func (a *App) draw(in InputFrame) {
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

	a.UI.Begin(in.ToUI())
	a.buildShell(l)
	a.UI.DrawToasts(l.Viewport)
	if a.showShortcuts {
		a.UI.DrawShortcutOverlay(l.Screen, shortcutSheet())
	}
	a.UI.DrawModal(l.Screen)
	a.UI.End()
}

// HintText is what the hint bar says right now. It never returns an empty
// string: silence is never the answer (SPEC-UX §1).
func (a *App) HintText() string {
	if a.hintOverride != "" {
		return a.hintOverride
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
		{Keys: "O", Description: "Orthographic / perspective"},
		{Section: "Tools"},
		{Keys: "S", Description: "Sketch"},
		{Keys: "E", Description: "Extrude"},
		{Keys: "B", Description: "Boolean"},
		{Keys: "M", Description: "Move / transform"},
		{Keys: "P", Description: "Paint"},
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
