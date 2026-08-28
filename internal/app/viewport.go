package app

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
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
	s := render.Scene{
		Camera:    a.Camera,
		DimFactor: 1,
		// A brush paints faces. Letting an edge or a vertex win the pixel under
		// the cursor would be a stroke that lands on nothing, on exactly the
		// meshes where the wires are densest — except for the edge tool, whose
		// whole job is picking them.
		PickFacesOnly: a.InPaint() && !a.InEdgePaint(),
	}

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
			// The Textures eye shows the bare geometry under the pixels
			// (SPEC-UX §13.2). It is a view setting, not an edit: the images
			// are untouched and the eye puts them straight back.
			HideTexture: a.paint.hideTextures,
		}
		// Hovering a body's row in the tree pre-highlights it in the viewport
		// (SPEC-UX §7).
		if a.TreeHover == ref {
			d.EdgeColor = ui.Fade(ui.ColorAccent, 0.7)
		}
		if a.Hover.Hit && a.Hover.Kind == render.PickFace && a.Hover.BodyID == b.ID {
			d.HoverFace = a.Hover.FaceUID
		}
		if faces := a.selectedFacesOf(b.ID); len(faces) > 0 {
			d.SelectedFaces = faces
		}
		// A body picked for a boolean is tinted by the part it plays, so the
		// viewport and the card always agree about what is about to happen.
		if tint, ok := a.booleanTint(b.ID); ok {
			d.Tint = tint
		}
		s.Bodies = append(s.Bodies, d)
	}

	// The default planes step out of the way entirely while material is being
	// dragged out or in. Dimming them was not enough: three translucent quads
	// spanning the viewport still cross the solid you are pulling, they blend
	// against a translucent preview in the same pass, and the one thing that
	// matters at that moment is how far it has come. They are back the instant
	// the drag ends, and their eye toggles are untouched.
	if !a.previewOwnsView() {
		s.Planes = a.buildPlaneDraws()
	}

	// Every visible sketch draws, in or out of sketch mode, so the eye toggle
	// on a sketch row means something.
	s.Sketches = a.buildSketchDraws()

	// The pending extrude solid rides along as a translucent body, and so does
	// the material a push/pull drag would add or remove.
	if d, ok := a.extrudePreviewDraw(); ok {
		s.Bodies = append(s.Bodies, d)
	}
	if d, ok := a.pushPullPreviewDraw(); ok {
		s.Bodies = append(s.Bodies, d)
	}
	vpr := a.layout.RenderViewport()
	// Exactly one gizmo is live at a time, and they are checked in the order a
	// tool takes over from the selection underneath it.
	switch {
	case a.InExtrude():
		s.Gizmo = a.buildExtrudeGizmo(vpr)
	case a.InEdgePaint():
		s.Gizmo = a.edgeOverlay()
	case a.InPaint():
		s.Gizmo = a.paintCursorOverlay()
	case a.InPushPull():
		s.Gizmo = a.buildPushPullGizmo(vpr)
	default:
		s.Gizmo = a.buildTransformGizmo(vpr)
	}
	// Selected edges and vertices draw as their own overlay: the body pass
	// knows about faces and silhouettes, not about sub-elements.
	if sel := a.buildSelectionOverlay(); sel != nil {
		s.Sketches = append(s.Sketches, sel)
	}
	// The orientation dots ride the same overlay layer (V-131).
	if mk := a.buildMarkerOverlay(); mk != nil && !mk.Empty() {
		s.Sketches = append(s.Sketches, mk)
	}

	// Sketch mode dims the rest of the model and puts the grid on the sketch
	// plane, so the profile being drawn is what the eye lands on (SPEC-UX §8.1).
	// A push/pull drag deliberately does not: it is a direct edit of a body in
	// place, and you are judging the new material against the model it is
	// joining. Dimming that model is dimming the thing you are comparing to.
	if a.InSketch() || a.InExtrude() {
		s.DimFactor = SketchDimFactor
		if sk := a.ActiveSketch(); sk != nil {
			s.Grid = scene.SketchGridFor(sk, a.gridStep())
		}
		for i := range s.Bodies {
			s.Bodies[i].Pickable = false
		}
	}
	return s
}

// buildPlaneDraws turns plane visibility, selection and hover into draw specs.
//
// Sketch mode shows only the plane being sketched on (V-129). The other two
// arrive edge-on — coloured bands crossing the graph paper, with their labels
// stranded mid-grid — and an edge-on reference plane references nothing. A
// face sketch shows none: its sheet belongs to the body, not to the origin.
func (a *App) buildPlaneDraws() []render.PlaneDraw {
	doc := a.Doc()
	only, limited := geom.PlaneKind(0), false
	if a.InSketch() {
		sk := a.ActiveSketch()
		if sk == nil || sk.OnFace {
			return nil
		}
		only, limited = sk.Plane, true
	}
	fade := a.planeZoomFade()
	out := make([]render.PlaneDraw, 0, geom.PlaneCount)
	for i := 0; i < geom.PlaneCount; i++ {
		k := geom.PlaneKind(i)
		if !doc.PlaneVisible(k) || (limited && k != only) {
			continue
		}
		hovered := (a.TreeHover.Kind == model.SelPlane && a.TreeHover.Plane == k) ||
			(a.Hover.Hit && a.Hover.Kind == render.PickPlane && a.Hover.Plane == k)
		selected := a.Sel.Contains(model.PlaneRef(k))
		f := fade
		if hovered || selected {
			// A plane the user is pointing at or has picked shows itself
			// whatever the zoom: fading the thing being asked about would
			// answer the hover with nothing.
			f = 1
		}
		out = append(out, render.PlaneDraw{
			Kind:     k,
			Frame:    geom.PlaneFrame(k),
			HalfSize: scene.PlaneHalfSize,
			Color:    ui.WithAlpha(scene.PlaneColor(k), scene.PlaneTintAlpha),
			Label:    k.String(),
			Hovered:  hovered,
			Selected: selected,
			Pickable: f > 0.15,
			Fade:     f,
		})
	}
	return out
}

// planeZoomFade is how visible the default planes are at the current zoom
// (V-129). Zoomed out, a plane is a bounded sheet whose edges you can see;
// zoomed in past it, the same quad is a translucent wall across the entire
// view, washing over the model that is actually being worked on. It starts
// fading once the plane spans twice the view height and is gone by four times
// — and it stops being pickable on the way, because an invisible wall that
// still eats clicks would be worse than the wall.
func (a *App) planeZoomFade() float64 {
	viewH := a.Camera.OrthoScale
	if a.Camera.Perspective {
		viewH = 2 * a.Camera.Dist * math.Tan(render.PerspectiveFOV*math.Pi/360)
	}
	if viewH <= 0 {
		return 1
	}
	k := 2 * scene.PlaneHalfSize / viewH
	switch {
	case k <= 1.5:
		return 1
	case k >= 3:
		return 0
	default:
		return (3 - k) / 1.5
	}
}

// viewportHoverRef maps the pick result into a document reference, so the tree
// can highlight the row for whatever the cursor is over in 3D.
func (a *App) viewportHoverRef() model.Ref {
	// A dot wins here for the same reason it wins a click: it is drawn over
	// everything and it is what you can see there.
	if a.markers.hover >= 0 {
		return model.MarkerRef(a.markers.hover)
	}
	// A sketch under the pointer wins, for the same reason a click does: it is
	// drawn over everything, so it is what you can see there.
	if a.hoverSketch != nil {
		return model.SketchRef(a.hoverSketch.ID)
	}
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

// cubeOwnsPointer reports that the view cube is under the pointer or holds a
// drag, so the viewport tools must leave this click alone.
//
// The cube floats inside the viewport, and every tool that consumed presses by
// geometry alone acted straight through it: clicking FRONT while sketching
// snapped the camera and put a line point down, both from the one click. Idle
// mode always knew this (handleViewportClick has checked the cube since M1) —
// the tools that came later each had to learn it, so now it has a name.
func (a *App) cubeOwnsPointer(in InputFrame) bool {
	return a.cubeDrag || a.Cube.Contains(in.MouseX, in.MouseY)
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

// previewOwnsView reports that a translucent preview is being dragged: an open
// extrude, or a push/pull drag that has moved.
//
// It decides one thing only — whether the default planes are in the way. A
// translucent plane blending against a translucent preview in the same pass
// muddles both, and a quad spanning the viewport crosses whatever is being
// pulled. The model itself stays exactly as it was.
func (a *App) previewOwnsView() bool {
	if a.InExtrude() {
		return true
	}
	// Paint mode hides them for the same reason and for its whole duration
	// (SPEC-UX §13.1): a translucent quad across a face is exactly the thing
	// that makes you doubt the colour you are looking at.
	if a.InPaint() {
		return true
	}
	t := a.pushPull.tool
	return t != nil && (t.Dragging() || t.Active())
}

// selectedFacesOf collects the selected faces belonging to one body, which is
// what the overlay pass highlights.
func (a *App) selectedFacesOf(bodyID uint32) map[mesh.FaceUID]bool {
	var out map[mesh.FaceUID]bool
	for _, ref := range a.Sel.Refs() {
		if ref.Kind != model.SelFace || ref.Body != bodyID {
			continue
		}
		if out == nil {
			out = map[mesh.FaceUID]bool{}
		}
		out[ref.Face] = true
	}
	return out
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
		case model.SelMarker:
			if ref.Marker >= 0 && ref.Marker < len(a.Doc().Markers) {
				// A point has no size, so frame a small box around it or the
				// camera would try to fit nothing and zoom to infinity.
				at := a.Doc().Markers[ref.Marker].At
				box = box.Union(geom.AABBOf([]geom.Vec3{at}).Expand(1.5))
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
	if !in.Pressed[MouseLeft] || a.cubeOwnsPointer(in) {
		return
	}
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) {
		return
	}
	// Starting work in the viewport is an answer to the welcome card too: a
	// click that reached here missed the card, and whatever it was aiming at —
	// a plane, a body, empty space — the user has begun.
	a.dismissWelcome()
	// Pick right before acting on a click, never trusting the throttled hover
	// result (SPEC-RENDER §6.1).
	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	a.Hover = hit

	// A placed dot outranks everything under it (the user's request,
	// 2026-08-28). It is the smallest thing in the viewport and it is drawn on
	// top of all of it, so a click that lands on one is unambiguous — and
	// unless it wins, a dot sitting on a hull could never be grabbed at all,
	// because the face behind it always would. Placing a new dot still comes
	// first: an armed pick was promised the next click.
	if !a.markers.armed {
		if i := a.markerAt(in.MouseX, in.MouseY, vp); i >= 0 {
			a.selectMarker(i)
			return
		}
	}

	// A visible sketch wins over anything behind it, including a body.
	//
	// Sketches are drawn with the depth test off (V-12), so one is always on top
	// of whatever it overlaps — that is what makes sketching on a plane running
	// through a hull possible at all. Clicking has to agree with that: if the
	// sketch is the thing you can see there, it is the thing you get. A sketch
	// that gets in the way has an eye in the tree, and a consumed one hides
	// itself.
	if sk := a.sketchAt(in.MouseX, in.MouseY, vp); sk != nil {
		a.selectRef(model.SketchRef(sk.ID))
		return
	}

	// An armed marker pick spends the click on placing the dot (V-131). It
	// outranks selection for the same reason the plane pick does: the tree row
	// promised this click would place, and a promise the click does not keep
	// teaches people to stop trusting the rows.
	if a.markers.armed {
		if hit.Hit && hit.Kind == render.PickFace {
			a.placeMarkerAt(hit, in, vp)
		} else {
			a.Toast(ui.Toast{
				Text: "Click a face of the model to place the dot",
				Kind: ui.ToastWarn,
			})
		}
		return
	}

	if !hit.Hit {
		// Empty space starts a rectangle. It only becomes a box select if the
		// pointer actually travels; a press and release in the same place is
		// still a click, and still deselects (SPEC-UX §12.1).
		a.beginBoxSelect(in.MouseX, in.MouseY)
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
	case render.PickFace:
		// A second click on an already-selected face takes the whole body,
		// which is SPEC-UX §12.1's double-click without needing to time one.
		ref := model.FaceRef(hit.BodyID, hit.FaceUID)
		if a.Sel.Contains(ref) && a.Sel.Len() == 1 && !in.Shift && !in.Ctrl {
			a.selectRef(model.BodyRef(hit.BodyID))
			return
		}
		a.selectRef(ref)
	case render.PickEdge:
		a.selectRef(model.EdgeRef(hit.BodyID, hit.Edge))
	case render.PickVert:
		a.selectRef(model.VertRef(hit.BodyID, hit.Vert))
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
	// The file map of SPEC-UX §16. Each one only queues: the dialog runs after
	// the frame has finished drawing.
	if in.Ctrl {
		switch {
		case in.KeyPressed(rl.KeyN):
			a.RequestFile(fileNew)
		case in.KeyPressed(rl.KeyO):
			a.RequestFile(fileOpen)
		case in.KeyPressed(rl.KeyS) && in.Shift:
			a.RequestFile(fileSaveAs)
		case in.KeyPressed(rl.KeyS):
			a.RequestFile(fileSave)
		case in.KeyPressed(rl.KeyE):
			a.BeginExport()
		}
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
	if in.KeyPressed(rl.KeyE) {
		a.BeginExtrude()
	}
	if in.KeyPressed(rl.KeyB) {
		a.BeginBoolean()
	}
	if in.KeyPressed(rl.KeyM) {
		a.focusMove()
	}
	if in.KeyPressed(rl.KeyP) {
		a.togglePaint()
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
		case model.SelFace:
			a.BeginSketchOnFace(ref.Body, ref.Face)
			return
		}
	}
	a.sketch.awaitingPlane = true
}

// escape walks one level back up the interaction stack (SPEC-UX §1).
func (a *App) escape() {
	switch {
	case a.CancelTransform():
		// A live drag is the innermost thing there is: Escape puts the model
		// back where it started and records nothing (SPEC-DATA §2).
	case a.InExport():
		a.CancelExport()
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
	case a.CancelMarkerPick():
		// An armed marker pick unwinds first, like every armed pick does.
	case a.sketch.awaitingPlane:
		// The hint promises "Esc to cancel" while a plane pick is armed, and a
		// promise the key does not keep teaches people to stop reading it.
		a.sketch.awaitingPlane = false
	case !a.Sel.Empty():
		a.Sel.Clear()
	case a.showWelcome():
		// Escape waves the welcome card away; the hint bar says the same
		// things from then on.
		a.dismissWelcome()
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
		case model.SelMarker:
			// A dot has no eye of its own: it is the answer to "which way is
			// forward", and an answer you cannot see is worse than none.
			a.Toast(ui.Toast{
				Text: "Dots can't be hidden — Del removes one",
				Kind: ui.ToastWarn,
			})
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
		a.hoverSketch = nil
		a.markers.hover = -1
		return
	}
	// A dot under the pointer swells and takes the hint bar, which is how you
	// learn it can be grabbed before you try. Not while a gizmo handle is under
	// the cursor: the handle was there first and owns the click.
	a.markers.hover = -1
	if !a.grabbedGizmo() {
		a.markers.hover = a.markerAt(in.MouseX, in.MouseY, vp)
	}
	a.hoverSketch = a.sketchAt(in.MouseX, in.MouseY, vp)
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
