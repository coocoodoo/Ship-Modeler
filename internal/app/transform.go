package app

import (
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// Move and rotate in the app (R10, R11, SPEC-UX §12.2–§12.4).
//
// The gizmo is a property of the selection rather than a mode: select
// something and it is there. A drag runs live through the bus so the model
// really is moving as you move it, and lands as exactly one undo step
// (SPEC-DATA §2).

// transformState is the app's half of the move and rotate gizmos.
type transformState struct {
	tool *tools.TransformTool
	// live is true between the first UpdateDrag and the commit, which is what
	// tells Escape there is something to cancel.
	live bool
	// warnedOffGrid remembers that the free-rotation warning has been given, so
	// it is said once a session rather than once a drag (SPEC-GEOMETRY §7.3).
	warnedOffGrid bool
}

// InTransform reports whether a gizmo is armed.
func (a *App) InTransform() bool { return a.transform.tool != nil }

// armTransform puts the gizmo on the selection, or takes it away. Called every
// frame from Idle.
func (a *App) armTransform() {
	if a.transform.tool != nil && a.transform.tool.Dragging() {
		return
	}
	// A face armed for push/pull owns its own gizmo, and the two would sit on
	// top of each other: the arrow starts at the face centroid and the move
	// gizmo's centre handle is a disc around that same point. Only one of them
	// is drawn, so leaving both armed means an invisible gizmo quietly eating
	// every click meant for the arrow — which is what it did.
	if a.InPushPull() {
		a.transform.tool = nil
		return
	}
	pivot, ok := a.Sel.Pivot(a.Doc())
	if !ok {
		a.transform.tool = nil
		return
	}
	if t := a.transform.tool; t != nil {
		t.Pivot = pivot
		return
	}
	a.transform.tool = tools.NewTransformTool(pivot)
}

// gizmoView bundles what the scene helpers need.
func (a *App) gizmoView(vp render.Viewport) scene.GizmoView {
	return scene.GizmoView{Tool: a.transform.tool, Cam: a.Camera, Vp: vp}
}

// updateTransform runs one frame of the gizmo's pointer input.
func (a *App) updateTransform(in InputFrame, vp render.Viewport) {
	t := a.transform.tool
	if t == nil {
		return
	}
	view := a.gizmoView(vp)

	if !t.Dragging() {
		t.Hover = scene.GizmoHit(view, in.MouseX, in.MouseY)
		if a.cubeOwnsPointer(in) {
			t.Hover = tools.PartNone
		}
		if in.Pressed[MouseLeft] && t.Hover != tools.PartNone {
			raw, angle := a.rawDrag(t, t.Hover, in, vp)
			t.Begin(t.Hover, raw, angle)
		}
		return
	}

	if in.Down[MouseLeft] {
		raw, angle := a.rawDrag(t, t.Active, in, vp)
		if t.Mode == tools.GizmoRotate {
			t.UpdateRotate(angle, tools.RotateDetentFor(in.Shift, in.Alt))
		} else {
			t.UpdateMove(raw, snapStep(in))
		}
		a.applyTransformLive()
		return
	}
	t.End()
	a.commitTransform()
}

// grabbedGizmo reports that the transform gizmo owns the pointer, so the same
// click does not also re-pick whatever is behind it.
func (a *App) grabbedGizmo() bool {
	t := a.transform.tool
	return t != nil && (t.Dragging() || t.Hover != tools.PartNone)
}

// rawDrag turns the cursor into the quantity the grabbed handle measures: a
// world point for a move, an angle for a rotation.
func (a *App) rawDrag(t *tools.TransformTool, part tools.GizmoPart, in InputFrame, vp render.Viewport) (geom.Vec3, float64) {
	origin, dir := a.Camera.Ray(
		geom.Vec2{X: in.MouseX - float64(vp.X), Y: in.MouseY - float64(vp.Y)},
		float64(vp.W), float64(vp.H))

	switch {
	case part.IsRing():
		axis, _ := part.Axis()
		p, ok := rayPlane(origin, dir, t.Pivot, axis)
		if !ok {
			return geom.Vec3{}, 0
		}
		f := geom.FrameFromNormal(t.Pivot, axis)
		v := f.ToLocal(p)
		return geom.Vec3{}, math.Atan2(v.Y, v.X) * 180 / math.Pi

	case part.IsAxis():
		axis, _ := part.Axis()
		// The closest point on the axis line to the cursor's ray. This is the
		// honest answer to "where along the axis is the pointer" for a line
		// seen at any angle, and it degrades gracefully when the axis points
		// nearly at the camera instead of jumping to infinity.
		d, ok := closestOnLine(t.Pivot, axis, origin, dir)
		if !ok {
			return geom.Vec3{}, 0
		}
		return axis.Mul(d), 0

	case part.IsPlane():
		axis, _ := part.Axis()
		p, ok := rayPlane(origin, dir, t.Pivot, axis)
		if !ok {
			return geom.Vec3{}, 0
		}
		return p.Sub(t.Pivot), 0

	default: // the centre handle moves in the plane facing the camera
		p, ok := rayPlane(origin, dir, t.Pivot, a.Camera.Forward().Neg())
		if !ok {
			return geom.Vec3{}, 0
		}
		return p.Sub(t.Pivot), 0
	}
}

// rayPlane intersects a ray with a plane through a point.
func rayPlane(origin, dir, planeP, planeN geom.Vec3) (geom.Vec3, bool) {
	den := dir.Dot(planeN)
	if math.Abs(den) < 1e-9 {
		return geom.Vec3{}, false
	}
	t := planeP.Sub(origin).Dot(planeN) / den
	return origin.Add(dir.Mul(t)), true
}

// closestOnLine returns how far along a line the closest approach to a ray is.
func closestOnLine(lineP, lineDir, rayO, rayD geom.Vec3) (float64, bool) {
	w := lineP.Sub(rayO)
	a := lineDir.Dot(lineDir)
	b := lineDir.Dot(rayD)
	c := rayD.Dot(rayD)
	d := lineDir.Dot(w)
	e := rayD.Dot(w)
	den := a*c - b*b
	if math.Abs(den) < 1e-9 {
		return 0, false // the line points straight at the camera
	}
	return (b*e - c*d) / den, true
}

// applyTransformLive runs the pending edit through the bus so the model is
// really moving during the drag, not a preview of one.
func (a *App) applyTransformLive() {
	t := a.transform.tool
	if t == nil || !t.Moved() {
		if a.transform.live {
			a.Bus.CancelDrag()
			a.transform.live = false
		}
		return
	}
	cmd := a.transformCommand(t)
	if cmd == nil {
		return
	}
	if err := a.Bus.UpdateDrag(cmd); err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastWarn})
		a.Bus.CancelDrag()
		a.transform.live = false
		t.Reset()
		return
	}
	a.transform.live = true
}

// transformCommand builds the command for the gizmo's current state.
func (a *App) transformCommand(t *tools.TransformTool) model.Command {
	verts := a.Sel.VertIndices(a.Doc())
	if len(verts) == 0 {
		return nil
	}
	what := a.Sel.Describe(a.Doc())
	if t.Mode == tools.GizmoRotate {
		return model.NewRotateVerts(verts, t.Pivot, t.RotationAxis(), t.Angle, what)
	}
	return model.NewMoveVerts(verts, t.Delta, what)
}

// commitTransform lands the drag as one undo step (SPEC-UX §12.2).
func (a *App) commitTransform() {
	t := a.transform.tool
	if t == nil {
		return
	}
	if !a.transform.live {
		t.Reset()
		return
	}
	label := t.Label()
	moved := a.transform.live
	a.warnIfBent()
	a.warnIfOffGrid()
	if _, ok := a.Bus.CommitDrag(); ok && moved {
		a.Toast(ui.Toast{Text: t.Mode.String() + " " + label})
	}
	a.transform.live = false
	t.Reset()
	a.armTransform()
}

// CancelTransform abandons a live drag, putting everything back (SPEC-DATA §2).
func (a *App) CancelTransform() bool {
	t := a.transform.tool
	if t == nil || !a.transform.live {
		return false
	}
	a.Bus.CancelDrag()
	a.transform.live = false
	t.Reset()
	return true
}

// warnIfBent says once, quietly, when an edit has left faces non-planar
// (SPEC-UX §12.3).
func (a *App) warnIfBent() {
	n := 0
	for _, b := range a.Doc().Bodies {
		if b.Mesh == nil {
			continue
		}
		for i := range b.Mesh.Faces {
			if b.Mesh.Faces[i].NonPlanar {
				n++
			}
		}
	}
	if n > 0 {
		a.Toast(ui.Toast{
			Text: plural(n, "face is", "faces are") + " now bent — " +
				"sketching and push/pull need a flat one",
			Kind: ui.ToastWarn,
		})
	}
}

// warnIfOffGrid is the once-per-session note of SPEC-GEOMETRY §7.3.
func (a *App) warnIfOffGrid() {
	cmd, ok := a.Bus.Pending().(*model.RotateVerts)
	if !ok || !cmd.LeftTheGrid() || a.transform.warnedOffGrid {
		return
	}
	a.transform.warnedOffGrid = true
	a.Toast(ui.Toast{Text: "Free rotation leaves the pixel grid", Kind: ui.ToastWarn})
}

// buildTransformGizmo assembles the gizmo overlay for this frame.
func (a *App) buildTransformGizmo(vp render.Viewport) *render.Overlay {
	if a.transform.tool == nil {
		return nil
	}
	return scene.BuildTransformGizmo(a.gizmoView(vp))
}

// focusMove is what the Move button and the M key do. The gizmo is a property
// of the selection rather than a mode, so there is nothing to enter: with
// something selected this points the armed gizmo at moving, and with nothing
// selected it says what to select — which is more than the button managed for
// three milestones, disabled behind a tooltip claiming Move "arrives with M6".
func (a *App) focusMove() {
	a.ExitPaint()
	if t := a.transform.tool; t != nil {
		t.Mode = tools.GizmoMove
		return
	}
	a.Toast(ui.Toast{
		Text: "Select a body to move it — or drag a box around vertices",
		Kind: ui.ToastWarn,
	})
}

// canMove reports whether the Move button has anything to act on: a gizmo is
// armed. Asking the armed state rather than the selection keeps the button
// honest for the selections that cannot move — a plane, a sketch — which have
// rows in the tree but no vertices to carry.
func (a *App) canMove() (bool, string) {
	if a.InTransform() {
		return true, ""
	}
	return false, "Select a body, face, edge or vertex — the gizmo appears on it"
}

// handleTransformKeys implements the shortcuts that act on a selection.
func (a *App) handleTransformKeys(in InputFrame) {
	if in.Ctrl && in.KeyPressed(rl.KeyD) {
		a.duplicateSelection()
		return
	}
	if in.Ctrl {
		return
	}
	if in.KeyPressed(rl.KeyR) && a.transform.tool != nil {
		// R toggles the gizmo between move and rotate, which is the card's tab
		// without reaching for it.
		t := a.transform.tool
		if t.Mode == tools.GizmoMove {
			t.Mode = tools.GizmoRotate
		} else {
			t.Mode = tools.GizmoMove
		}
	}
}

// duplicateSelection is Ctrl+D (SPEC-UX §12.4).
func (a *App) duplicateSelection() {
	ref, ok := a.Sel.Primary()
	if !ok || ref.Body == 0 {
		a.Toast(ui.Toast{Text: "Select a body to duplicate", Kind: ui.ToastWarn})
		return
	}
	cmd := &model.DuplicateBody{ID: ref.Body}
	if !a.Run(cmd) {
		return
	}
	a.Sel.Set(model.BodyRef(cmd.Copy().ID))
	a.Toast(ui.Toast{Text: "Duplicated as " + cmd.Copy().Name})
}

// transformHint is what the hint bar says while a gizmo is armed.
func (a *App) transformHint() string {
	t := a.transform.tool
	if t == nil {
		return ""
	}
	if t.Dragging() {
		return t.Hint()
	}
	return "Drag the gizmo to " + strings.ToLower(t.Mode.String()) +
		" · R switches move and rotate · Ctrl+D duplicates · Del removes"
}

// buildSelectionOverlay draws the selected edges and vertices.
//
// Faces and whole bodies are highlighted by the body pass, which already knows
// their geometry. An edge or a vertex has no surface to tint, so it is drawn
// here as a stroke or a square handle, in the accent, over everything — a
// selected vertex behind the hull still has to be findable (SPEC-UX §12.1).
func (a *App) buildSelectionOverlay() *render.Overlay {
	d := &render.Overlay{}
	for _, ref := range a.Sel.Refs() {
		b := a.Doc().BodyByID(ref.Body)
		if b == nil || b.Mesh == nil {
			continue
		}
		switch ref.Kind {
		case model.SelVert:
			if ref.Vert >= 0 && ref.Vert < len(b.Mesh.Verts) {
				d.Markers = append(d.Markers, render.OverlayMarker{
					P: b.Mesh.Verts[ref.Vert], Kind: render.MarkerVertex,
					Color: ui.ColorAccent, SizePx: 6,
				})
			}
		case model.SelEdge:
			t := b.Mesh.Topo()
			if ref.Edge >= 0 && ref.Edge < len(t.Edges) {
				e := t.Edges[ref.Edge]
				d.Lines = append(d.Lines, render.OverlayLine{
					A: b.Mesh.Verts[e.A], B: b.Mesh.Verts[e.B],
					Color: ui.ColorAccent, WidthPx: 2.5,
				})
			}
		}
	}
	if d.Empty() {
		return nil
	}
	return d
}

// buildTransformCard is the panel of SPEC-UX §12.2 and §12.4: a Move/Rotate
// tab, the numeric offset fields, and the box-select filter.
func (a *App) buildTransformCard(viewport rl.Rectangle) {
	t := a.transform.tool
	if t == nil {
		return
	}
	w := a.px(232)
	h := a.px(206)
	box := ui.Rect(
		viewport.X+viewport.Width-w-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		w, h)

	card := a.UI.FloatingCard(ui.MakeID("transform.card"), box,
		a.Sel.Describe(a.Doc()), ui.FloatingCardOpts{})
	body := card.Body
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	row := func(h float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, h)
		return r
	}

	modes := []tools.GizmoMode{tools.GizmoMove, tools.GizmoRotate}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("transform.mode"), row(a.px(24)),
		[]string{"Move", "Rotate"}, int(t.Mode),
		ui.ChipGroupOpts{Tooltip: "R switches these"}); changed {
		t.Mode = modes[pick]
	}

	body.Y += a.px(6)
	body.Height -= a.px(6)

	if t.Mode == tools.GizmoMove {
		// Typing here moves precisely, which is the whole point of having the
		// fields as well as the arrows (SPEC-UX §12.2).
		a.UI.Text(row(line), "Offset", ui.FontSizeSmall, ui.ColorTextDim)
		d := t.Delta
		axes := []struct {
			label string
			value *float64
		}{{"X", &d.X}, {"Y", &d.Y}, {"Z", &d.Z}}
		r := row(a.px(24))
		fieldW := (r.Width - a.px(8)) / 3
		for i, ax := range axes {
			f := ui.Rect(r.X+float32(i)*(fieldW+a.px(4)), r.Y, fieldW, r.Height)
			if v, res := a.UI.DragNumber(ui.MakeID("transform.d"+ax.label), f, *ax.value,
				ui.NumberOpts{
					Unit: "u", Step: 1, FineStep: 0.25, Decimals: 2,
					Min: -tools.MaxDepthUnits, Max: tools.MaxDepthUnits,
					Tooltip: "Offset along " + ax.label,
				}); res.Changed {
				*ax.value = v
				t.SetDelta(d)
				a.applyTransformLive()
			}
		}
	} else {
		a.UI.Text(row(line), "Angle", ui.FontSizeSmall, ui.ColorTextDim)
		a.UI.Text(row(a.px(20)), t.Label()+" "+t.Active.String(),
			ui.FontSizeUI, ui.ColorText)
	}

	body.Y += a.px(8)
	body.Height -= a.px(8)

	a.UI.Text(row(line), "Box select", ui.FontSizeSmall, ui.ColorTextDim)
	filters := BoxFilters()
	labels := make([]string, len(filters))
	sel := 0
	for i, f := range filters {
		labels[i] = BoxFilterLabel(f)
		if f == a.box.Filter {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("transform.boxfilter"), row(a.px(24)),
		labels, sel, ui.ChipGroupOpts{
			Tooltip: "What dragging a box on empty space collects",
		}); changed {
		a.box.Filter = filters[pick]
	}
}

// drawBoxRect paints the selection rectangle.
func (a *App) drawBoxRect() {
	r := a.BoxRect()
	rect := ui.Rect(float32(r.X0), float32(r.Y0),
		float32(r.X1-r.X0), float32(r.Y1-r.Y0))
	a.UI.FillRounded(rect, 0, ui.WithAlpha(ui.ColorAccent, 0x33))
	a.UI.StrokeRounded(rect, 0, ui.ColorAccent)
}
