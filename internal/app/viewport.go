package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

// The viewport half of the app: turning the document into a draw list, routing
// navigation input, and translating a pick result into a selection.

// BuildScene assembles this frame's draw list from the document.
func (a *App) BuildScene() render.Scene {
	doc := a.Doc()
	s := render.Scene{Camera: a.Camera, DimFactor: 1}

	for _, b := range doc.Bodies {
		if !b.Visible || b.Mesh == nil {
			continue
		}
		ref := model.BodyRef(b.ID)
		d := render.BodyDraw{
			GPU:       a.bodyGPU(b),
			BodyID:    b.ID,
			Color:     b.Color,
			Alpha:     1,
			Transform: geom.Identity(),
			Pickable:  true,
			Selected:  a.Sel.Contains(ref),
		}
		// Hovering a body's row in the tree pre-highlights it in the viewport
		// (SPEC-UX §7).
		if a.TreeHover == ref {
			d.EdgeColor = ui.Fade(ui.ColorAccent, 0.7)
		}
		if a.Hover.Hit && a.Hover.Kind == render.PickFace && a.Hover.BodyID == b.ID {
			d.HoverFace = a.Hover.FaceUID
		}
		s.Bodies = append(s.Bodies, d)
	}

	s.Planes = a.buildPlaneDraws()

	// Sketch mode dims the rest of the model and puts the grid on the sketch
	// plane, so the profile being drawn is what the eye lands on (SPEC-UX §8.1).
	if a.InSketch() {
		s.DimFactor = SketchDimFactor
		if sk := a.ActiveSketch(); sk != nil {
			s.Grid = scene.SketchGridFor(sk)
		}
		s.Sketch = a.buildSketchDraw()
		for i := range s.Bodies {
			s.Bodies[i].Pickable = false
		}
	}
	return s
}

// buildPlaneDraws turns plane visibility, selection and hover into draw specs.
func (a *App) buildPlaneDraws() []render.PlaneDraw {
	doc := a.Doc()
	out := make([]render.PlaneDraw, 0, geom.PlaneCount)
	for i := 0; i < geom.PlaneCount; i++ {
		k := geom.PlaneKind(i)
		if !doc.PlaneVisible(k) {
			continue
		}
		hovered := (a.TreeHover.Kind == model.SelPlane && a.TreeHover.Plane == k) ||
			(a.Hover.Hit && a.Hover.Kind == render.PickPlane && a.Hover.Plane == k)
		out = append(out, render.PlaneDraw{
			Kind:     k,
			Frame:    geom.PlaneFrame(k),
			HalfSize: scene.PlaneHalfSize,
			Color:    ui.WithAlpha(scene.PlaneColor(k), scene.PlaneTintAlpha),
			Label:    k.String(),
			Hovered:  hovered,
			Selected: a.Sel.Contains(model.PlaneRef(k)),
			Pickable: true,
		})
	}
	return out
}

// viewportHoverRef maps the pick result into a document reference, so the tree
// can highlight the row for whatever the cursor is over in 3D.
func (a *App) viewportHoverRef() model.Ref {
	if !a.Hover.Hit {
		return model.Ref{}
	}
	switch a.Hover.Kind {
	case render.PickPlane:
		return model.PlaneRef(a.Hover.Plane)
	case render.PickFace, render.PickEdge, render.PickVert:
		return model.BodyRef(a.Hover.BodyID)
	}
	return model.Ref{}
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

// targetCamera is the state the camera is heading for: the running animation's
// destination, or the live camera when nothing is animating.
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

// GoHome animates to the isometric home view framing everything visible.
func (a *App) GoHome(vp render.Viewport) {
	to := a.targetCamera()
	to.Azimuth, to.Elevation = render.IsoAzimuth, render.IsoElevation
	s := a.BuildScene()
	to.FrameBox(scene.FrameAll(&s), vp.Aspect())
	a.Anim.Start(a.Camera, to)
}

// SetView animates to a named standard view.
func (a *App) SetView(v render.StandardView) {
	to := a.targetCamera()
	to.Azimuth, to.Elevation = v.Angles()
	a.Anim.Start(a.Camera, to)
}

// FrameSelection fits the selection, or everything visible, into the viewport.
func (a *App) FrameSelection(vp render.Viewport) {
	to := a.targetCamera()
	s := a.BuildScene()
	box := a.selectionBounds(&s)
	if !box.Valid() {
		box = scene.FrameAll(&s)
	}
	to.FrameBox(box, vp.Aspect())
	a.Anim.Start(a.Camera, to)
}

// selectionBounds is the bounding box of the selected bodies and planes, or an
// invalid box when nothing usable is selected.
func (a *App) selectionBounds(s *render.Scene) geom.AABB {
	if a.Sel.Empty() {
		return geom.Empty()
	}
	box := geom.Empty()
	for _, ref := range a.Sel.Refs() {
		switch ref.Kind {
		case model.SelBody, model.SelFace, model.SelEdge, model.SelVert:
			for i := range s.Bodies {
				if s.Bodies[i].BodyID == ref.Body && s.Bodies[i].GPU != nil {
					box = box.Union(s.Bodies[i].GPU.Bounds.Transform(s.Bodies[i].Transform))
				}
			}
		case model.SelPlane:
			f := geom.PlaneFrame(ref.Plane)
			h := scene.PlaneHalfSize
			for _, c := range [][2]float64{{-h, -h}, {h, -h}, {h, h}, {-h, h}} {
				box = box.AddPoint(f.ToWorld(geom.Vec2{X: c[0], Y: c[1]}))
			}
		}
	}
	return box
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

// handleViewportClick turns a left click in the 3D view into a selection.
func (a *App) handleViewportClick(in InputFrame, vp render.Viewport) {
	if !in.Pressed[MouseLeft] || a.cubeDrag || a.Cube.Contains(in.MouseX, in.MouseY) {
		return
	}
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) {
		return
	}
	// Pick right before acting on a click, never trusting the throttled hover
	// result (SPEC-RENDER §6.1).
	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	a.Hover = hit

	if !hit.Hit {
		if !in.Shift && !in.Ctrl {
			a.Sel.Clear()
		}
		return
	}
	switch hit.Kind {
	case render.PickPlane:
		if a.sketch.awaitingPlane {
			a.sketch.awaitingPlane = false
			a.BeginSketchOnPlane(hit.Plane)
			return
		}
		// A second click on an already-selected plane starts a sketch there,
		// which is the implicit affordance of SPEC-UX §5.
		if a.Sel.Contains(model.PlaneRef(hit.Plane)) && !in.Shift && !in.Ctrl {
			a.BeginSketchOnPlane(hit.Plane)
			return
		}
		a.selectRef(model.PlaneRef(hit.Plane))
	case render.PickFace, render.PickEdge, render.PickVert:
		// Sub-element selection arrives with the selection model in M6; until
		// then a click in the viewport selects the whole body.
		a.selectRef(model.BodyRef(hit.BodyID))
	}
}

// handleGlobalKeys are the shortcuts that work in every mode: history,
// projection, framing and the shortcut sheet (SPEC-UX §16).
func (a *App) handleGlobalKeys(in InputFrame, vp render.Viewport) {
	switch {
	case in.Ctrl && in.KeyPressed(rl.KeyZ) && in.Shift:
		a.Redo()
	case in.Ctrl && in.KeyPressed(rl.KeyZ):
		a.Undo()
	case in.Ctrl && in.KeyPressed(rl.KeyY):
		a.Redo()
	}
	if in.Ctrl {
		return
	}
	if in.KeyPressed(rl.KeyO) {
		a.Anim.Cancel()
		a.Camera.Perspective = !a.Camera.Perspective
		a.Camera.Normalize()
	}
	if in.KeyPressed(rl.KeyF) {
		a.FrameSelection(vp)
	}
	if in.KeyPressed(rl.KeySlash) && in.Shift {
		a.showShortcuts = !a.showShortcuts
	}
}

// handleKeys implements the Idle-mode keyboard map (SPEC-UX §16).
func (a *App) handleKeys(in InputFrame, vp render.Viewport) {
	a.handleGlobalKeys(in, vp)
	if in.Ctrl {
		return
	}

	if in.KeyPressed(rl.KeyS) {
		a.beginSketchFromSelection()
	}
	if in.KeyPressed(rl.KeyH) {
		a.hideSelection()
	}
	if in.KeyPressed(rl.KeyDelete) {
		a.deleteSelection()
	}
	if in.KeyPressed(rl.KeyEscape) {
		a.escape()
	}
}

// beginSketchFromSelection is what pressing S does in Idle: sketch on the
// selected plane, or arm the pointer to pick one (SPEC-UX §8.1).
func (a *App) beginSketchFromSelection() {
	if ref, ok := a.Sel.Primary(); ok {
		switch ref.Kind {
		case model.SelPlane:
			a.BeginSketchOnPlane(ref.Plane)
			return
		case model.SelSketch:
			a.EditSketch(a.Doc().SketchByID(ref.Sketch))
			return
		}
	}
	a.sketch.awaitingPlane = true
}

// escape walks one level back up the interaction stack (SPEC-UX §1).
func (a *App) escape() {
	switch {
	case a.showShortcuts:
		a.showShortcuts = false
	case a.UI.ModalOpen():
		a.UI.CloseModal()
	case a.tree.pickerFor != 0:
		a.UI.CloseColorPicker()
		a.tree.pickerFor = 0
	case a.tree.renaming.Kind != model.SelNone:
		a.tree.renaming = model.Ref{}
		a.UI.ClearFocus()
	case !a.Sel.Empty():
		a.Sel.Clear()
	}
}

// hideSelection hides everything selected, as one command per object so each is
// individually undoable.
func (a *App) hideSelection() {
	if a.Sel.Empty() {
		a.Toast(ui.Toast{Text: "Nothing selected to hide"})
		return
	}
	for _, ref := range a.Sel.Refs() {
		switch ref.Kind {
		case model.SelPlane:
			a.Run(&model.SetPlaneVisible{Plane: ref.Plane, Visible: false})
		case model.SelBody, model.SelFace, model.SelEdge, model.SelVert:
			a.Run(&model.SetBodyVisible{ID: ref.Body, Visible: false})
		case model.SelSketch:
			a.Run(&model.SetSketchVisible{ID: ref.Sketch, Visible: false})
		}
	}
}

// deleteSelection removes the selected objects, refusing on the default planes
// with an explanation rather than silence.
func (a *App) deleteSelection() {
	if a.Sel.Empty() {
		return
	}
	refs := append([]model.Ref(nil), a.Sel.Refs()...)
	for _, ref := range refs {
		a.deleteRef(ref)
	}
	a.Sel.Prune(a.Doc())
}

// updateHover refreshes the pick result, throttled so the ID pass runs at most
// every 33 ms while merely hovering (SPEC-RENDER §6.1).
func (a *App) updateHover(in InputFrame, vp render.Viewport) {
	if a.UI.WantMouse() || a.Cube.Contains(in.MouseX, in.MouseY) ||
		a.orbiting || a.panning || a.cubeDrag {
		a.Hover = render.PickResult{}
		return
	}
	a.pickCooldown -= in.DeltaMillis
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) {
		a.Hover = render.PickResult{}
		return
	}
	moved := in.MouseDX != 0 || in.MouseDY != 0
	if !moved || a.pickCooldown > 0 {
		return
	}
	a.pickCooldown = 33
	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	a.Hover = a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
}
