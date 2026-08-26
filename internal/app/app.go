package app

import (
	"fmt"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

// Version is shown in the title bar and the hint bar.
const Version = "v0.1.0-m0"

// Body is a document body. M1 moves this into internal/model with the rest of
// the document; for M0 it is the minimum the render path needs.
type Body struct {
	ID      uint32
	Name    string
	Color   color.RGBA
	Visible bool
	Mesh    *mesh.Mesh
	gpu     *render.BodyGPU
}

// gpuMesh builds and uploads the body's GPU form on first use.
func (b *Body) gpuMesh() *render.BodyGPU {
	if b.gpu == nil {
		b.gpu = render.BuildBodyGPU(b.Mesh)
		b.gpu.Upload()
	}
	return b.gpu
}

// invalidate drops the GPU form after a mesh edit.
func (b *Body) invalidate() {
	if b.gpu != nil {
		b.gpu.Unload()
		b.gpu = nil
	}
}

// App is the running application: window, renderer, camera and document.
type App struct {
	Renderer *render.Renderer
	Fonts    *ui.Fonts
	Scale    float64

	Camera render.Camera
	Anim   render.CameraAnim

	Cube  scene.ViewCube
	Triad scene.Triad

	Planes  scene.PlaneVisibility
	Bodies  []*Body
	nextID  uint32
	Hover   render.PickResult
	HintBar string

	// cubeDrag is true while the left button is orbiting via the view cube.
	cubeDrag bool
	// orbiting and panning track the camera navigation drags.
	orbiting, panning bool

	// pickCooldown throttles the ID pass to roughly 30 Hz on hover
	// (SPEC-RENDER §6.1).
	pickCooldown float64

	// Headless suppresses window presentation and drives a virtual clock.
	Headless bool
}

// New creates the application state. A GL context must already exist.
func New(headless bool) *App {
	scaleV := rl.GetWindowScaleDPI()
	uiScale := ui.Scale(float64(scaleV.X))
	a := &App{
		Renderer: render.NewRenderer(),
		Fonts:    ui.LoadFonts(uiScale),
		Scale:    uiScale,
		Camera:   render.DefaultCamera(),
		Planes:   scene.AllVisible(),
		nextID:   1,
		Headless: headless,
		HintBar:  "Click a plane (or press S) to start your first sketch",
	}
	return a
}

// Close releases GPU resources.
func (a *App) Close() {
	for _, b := range a.Bodies {
		b.invalidate()
	}
	a.Fonts.Unload()
	a.Renderer.Close()
}

// AddBody appends a body, assigning the next id and auto colour (SPEC-UX §3).
func (a *App) AddBody(m *mesh.Mesh) *Body {
	b := &Body{
		ID:      a.nextID,
		Name:    fmt.Sprintf("Body %d", a.nextID),
		Color:   ui.BodyColor(int(a.nextID) - 1),
		Visible: true,
		Mesh:    m,
	}
	a.nextID++
	a.Bodies = append(a.Bodies, b)
	return b
}

// FindBody looks a body up by name or by decimal id.
func (a *App) FindBody(nameOrID string) *Body {
	for _, b := range a.Bodies {
		if b.Name == nameOrID {
			return b
		}
	}
	var id uint32
	if _, err := fmt.Sscanf(nameOrID, "%d", &id); err == nil {
		for _, b := range a.Bodies {
			if b.ID == id {
				return b
			}
		}
	}
	return nil
}

// Viewport returns the 3D viewport rectangle for a framebuffer size. M1 shrinks
// it for the toolbar, tree panel and hint bar; M0 uses the whole window minus
// the hint bar so the layout maths is exercised from the start.
func (a *App) Viewport(fbW, fbH int) render.Viewport {
	hint := int(ui.HintBarHeight * a.Scale)
	return render.Viewport{X: 0, Y: 0, W: fbW, H: fbH - hint}
}

// BuildScene assembles this frame's draw list.
func (a *App) BuildScene() render.Scene {
	s := render.Scene{Camera: a.Camera, DimFactor: 1}
	for _, b := range a.Bodies {
		if !b.Visible {
			continue
		}
		g := b.gpuMesh()
		d := render.BodyDraw{
			GPU:       g,
			BodyID:    b.ID,
			Color:     b.Color,
			Alpha:     1,
			Transform: geom.Identity(),
			Pickable:  true,
		}
		// The hovered face is recorded for the overlay pass. It is deliberately
		// not tinted body-wide here: hovering one face must not light up the
		// whole body. The per-face highlight arrives with the overlay pass and
		// the selection model in M6 (SPEC-UX §12.1); until then the hint bar
		// names whatever is under the cursor.
		if a.Hover.Hit && a.Hover.Kind == render.PickFace && a.Hover.BodyID == b.ID {
			d.HoverFace = a.Hover.FaceUID
		}
		s.Bodies = append(s.Bodies, d)
	}
	hoverPlane, hasHoverPlane := a.hoveredPlane()
	s.Planes = scene.BuildPlaneDraws(a.Planes, hoverPlane, 0, hasHoverPlane, false)
	return s
}

func (a *App) hoveredPlane() (geom.PlaneKind, bool) {
	if a.Hover.Hit && a.Hover.Kind == render.PickPlane {
		return a.Hover.Plane, true
	}
	return 0, false
}

// Update advances one frame of app logic from an input frame.
func (a *App) Update(in InputFrame) {
	vp := a.Viewport(in.WindowW, in.WindowH)

	// The camera animation runs first so input can take over from wherever it
	// has reached rather than snapping (SPEC-RENDER §7).
	if cam, changed := a.Anim.Step(a.Camera, in.DeltaMillis); changed {
		a.Camera = cam
	}

	a.Cube.Layout(a.Camera, vp, a.Scale)
	a.Triad.Layout(vp, a.Scale)
	a.Cube.Update(in.MouseX, in.MouseY)

	a.handleCubeInput(in, vp)
	a.handleCameraInput(in, vp)
	a.handleKeys(in, vp)
	a.updateHover(in, vp)
}

// handleCubeInput implements clicking a zone to snap and dragging to orbit.
func (a *App) handleCubeInput(in InputFrame, vp render.Viewport) {
	overCube := a.Cube.Contains(in.MouseX, in.MouseY)

	if in.Pressed[MouseLeft] && overCube {
		a.cubeDrag = true
		return
	}
	if a.cubeDrag {
		if in.Down[MouseLeft] {
			if in.MouseDX != 0 || in.MouseDY != 0 {
				a.Anim.Cancel()
				a.Camera.Orbit(in.MouseDX, in.MouseDY)
			}
			return
		}
		// Release without a drag counts as a click on the zone under it.
		a.cubeDrag = false
		if zone, home := a.Cube.HitTest(in.MouseX, in.MouseY); home {
			a.GoHome(vp)
		} else if zone.Valid() {
			a.SnapToZone(zone)
		}
	}
}

// targetCamera is the state the camera is heading for: the running
// animation's destination, or the live camera when nothing is animating.
//
// Every scripted camera move composes from this rather than from the live
// camera, so "look front, then frame it" frames the front view instead of
// freezing the transition halfway.
func (a *App) targetCamera() render.Camera {
	if a.Anim.Active() {
		return a.Anim.Target()
	}
	return a.Camera
}

// SnapToZone animates the camera to a view cube zone's canonical orientation.
func (a *App) SnapToZone(z scene.CubeZone) {
	to := a.targetCamera()
	to.LookAlong(z.Direction())
	a.Anim.Start(a.Camera, to)
}

// GoHome animates to the isometric home view framing everything visible
// (SPEC-UX §6.1).
func (a *App) GoHome(vp render.Viewport) {
	to := a.targetCamera()
	to.Azimuth, to.Elevation = render.IsoAzimuth, render.IsoElevation
	s := a.BuildScene()
	to.FrameBox(scene.FrameAll(&s), vp.Aspect())
	a.Anim.Start(a.Camera, to)
}

// SetView animates to a named standard view (op scripts and the toolbar).
func (a *App) SetView(v render.StandardView) {
	to := a.targetCamera()
	to.Azimuth, to.Elevation = v.Angles()
	a.Anim.Start(a.Camera, to)
}

// FrameSelection fits everything visible into the viewport.
func (a *App) FrameSelection(vp render.Viewport) {
	to := a.targetCamera()
	s := a.BuildScene()
	to.FrameBox(scene.FrameAll(&s), vp.Aspect())
	a.Anim.Start(a.Camera, to)
}

// handleCameraInput implements the navigation of SPEC-UX §1: right-drag orbits,
// middle-drag or shift-right-drag pans, the wheel zooms to the cursor.
func (a *App) handleCameraInput(in InputFrame, vp render.Viewport) {
	if a.cubeDrag {
		return
	}
	inViewport := vp.Contains(int(in.MouseX), int(in.MouseY))

	if in.Pressed[MouseRight] && inViewport {
		a.orbiting = !in.Shift
		a.panning = in.Shift
	}
	if in.Pressed[MouseMiddle] && inViewport {
		a.panning = true
	}
	if !in.Down[MouseRight] && !in.Down[MouseMiddle] {
		a.orbiting, a.panning = false, false
	}

	if a.orbiting && (in.MouseDX != 0 || in.MouseDY != 0) {
		a.Anim.Cancel()
		a.Camera.Orbit(in.MouseDX, in.MouseDY)
	}
	if a.panning && (in.MouseDX != 0 || in.MouseDY != 0) {
		a.Anim.Cancel()
		a.Camera.Pan(in.MouseDX, in.MouseDY, float64(vp.W), float64(vp.H))
	}
	if in.Wheel != 0 && inViewport {
		a.Anim.Cancel()
		a.Camera.ZoomToCursor(in.Wheel, vp.Local(in.MouseX, in.MouseY),
			float64(vp.W), float64(vp.H))
	}
}

// handleKeys implements the M0 slice of the keyboard map (SPEC-UX §16).
func (a *App) handleKeys(in InputFrame, vp render.Viewport) {
	if in.KeyPressed(rl.KeyO) {
		a.Anim.Cancel()
		a.Camera.Perspective = !a.Camera.Perspective
		a.Camera.Normalize()
	}
	if in.KeyPressed(rl.KeyF) {
		a.FrameSelection(vp)
	}
	if in.KeyPressed(rl.KeyH) && !in.Ctrl {
		for i := range a.Planes {
			a.Planes[i] = false
		}
	}
}

// updateHover refreshes the pick result, throttled so the ID pass runs at most
// every 33 ms while merely hovering (SPEC-RENDER §6.1).
func (a *App) updateHover(in InputFrame, vp render.Viewport) {
	if a.Cube.Contains(in.MouseX, in.MouseY) || a.orbiting || a.panning || a.cubeDrag {
		a.Hover = render.PickResult{}
		return
	}
	a.pickCooldown -= in.DeltaMillis
	moved := in.MouseDX != 0 || in.MouseDY != 0
	clicked := in.Pressed[MouseLeft]
	if !clicked && (!moved || a.pickCooldown > 0) {
		return
	}
	a.pickCooldown = 33
	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	a.Hover = a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
}

// Draw renders one frame into the current target of the given size.
func (a *App) Draw(fbW, fbH int) {
	a.Renderer.SetFramebuffer(fbW, fbH)
	vp := a.Viewport(fbW, fbH)

	a.Renderer.ClearColor()
	s := a.BuildScene()
	a.Renderer.DrawViewport(&s, vp)

	a.Cube.Layout(a.Camera, vp, a.Scale)
	a.Triad.Layout(vp, a.Scale)
	a.Cube.Draw(a.Fonts, a.Scale)
	a.Triad.Draw(a.Camera, a.Fonts, a.Scale)

	a.drawHintBar(fbW, fbH)
}

// drawHintBar paints the always-present "what to do next" strip (SPEC-UX §2).
func (a *App) drawHintBar(fbW, fbH int) {
	h := int32(ui.HintBarHeight * a.Scale)
	y := int32(fbH) - h
	rl.DrawRectangle(0, y, int32(fbW), h, ui.ColorPanel)
	rl.DrawLine(0, y, int32(fbW), y, ui.ColorStroke)

	pad := float32(ui.Spacing * a.Scale)
	textY := float32(y) + (float32(h)-a.Fonts.LineHeight(ui.FontSizeSmall))/2
	a.Fonts.Draw(a.Fonts.Small, a.hintText(), pad, textY, ui.FontSizeSmall, ui.ColorTextDim)

	vw, _ := a.Fonts.Measure(a.Fonts.Small, Version, ui.FontSizeSmall)
	a.Fonts.Draw(a.Fonts.Small, Version, float32(fbW)-vw-pad, textY, ui.FontSizeSmall, ui.ColorTextDim)
}

// hintText prefers describing whatever is under the cursor, falling back to the
// mode's standing hint. Silence is never the answer (SPEC-UX §1).
func (a *App) hintText() string {
	if a.Hover.Hit {
		switch a.Hover.Kind {
		case render.PickFace:
			return fmt.Sprintf("Face %d of Body %d · S to sketch on this face",
				a.Hover.FaceUID.Seq(), a.Hover.BodyID)
		case render.PickEdge:
			return fmt.Sprintf("Edge %d of Body %d", a.Hover.Edge, a.Hover.BodyID)
		case render.PickVert:
			return fmt.Sprintf("Vertex %d of Body %d", a.Hover.Vert, a.Hover.BodyID)
		case render.PickPlane:
			return a.Hover.Plane.String() + " plane · click again to sketch here"
		}
	}
	if a.Cube.HoverHome {
		return "Home view"
	}
	if a.Cube.Hover.Valid() {
		if l := a.Cube.Hover.Label(); l != "" {
			return "View: " + l
		}
		return "Snap to this corner"
	}
	return a.HintBar
}
