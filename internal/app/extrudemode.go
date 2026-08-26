package app

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/extrude"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// Extrude mode (SPEC-UX §9): the arrow drag, the live preview, the options
// card, and committing through the bus as one undoable step.

// PreviewAlpha is how solid the pending body looks (SPEC-UX §9.2).
const PreviewAlpha = 0.55

// extrudeState is the app's half of the tool.
type extrudeState struct {
	tool *tools.ExtrudeTool
	// preview is the solid as it currently stands, rebuilt whenever an option
	// changes. Nil when the parameters cannot make a solid.
	preview *render.BodyGPU
	// previewErr is why there is no preview, shown in the card.
	previewErr string
	// hoverArrow is true when the pointer is over the gizmo.
	hoverArrow bool
	// targets are the bodies the pending solid would combine with, recomputed
	// with the preview. Which of them a commit uses depends on the result mode
	// (SPEC-UX §9.4).
	targets []uint32
	// previewMesh is the pending solid, kept so the targets can be worked out
	// and so a debug dump has something to write.
	previewMesh *mesh.Mesh
	// returnCamera restores the view if the tool is cancelled.
	returnCamera render.Camera
}

// InExtrude reports whether the extrude tool is open.
func (a *App) InExtrude() bool { return a.Mode == ModeExtrude && a.extrude.tool != nil }

// BeginExtrude opens the tool on the selected regions of the active sketch
// (SPEC-UX §9.1).
func (a *App) BeginExtrude() bool {
	s := a.ActiveSketch()
	// From Idle, a sketch picked in the tree is enough to start: re-enter it and
	// take all of its regions (SPEC-UX §9.1).
	fromTree := false
	if s == nil {
		if ref, ok := a.Sel.Primary(); ok && ref.Kind == model.SelSketch {
			if picked := a.Doc().SketchByID(ref.Sketch); picked != nil {
				a.EditSketch(picked)
				s, fromTree = picked, true
			}
		}
	}
	if s == nil {
		a.Toast(ui.Toast{Text: "Open a sketch first, then select a region", Kind: ui.ToastWarn})
		return false
	}
	arr := s.Arrangement()
	regions := a.selectedRegionList()
	if len(regions) == 0 && fromTree {
		for i := range arr.Regions {
			regions = append(regions, i)
		}
	}
	if len(regions) == 0 {
		// Nothing picked: take the whole profile if it is unambiguous, which is
		// the golden path of "draw a square, press E".
		if len(arr.Regions) == 1 {
			regions = []int{0}
		} else if len(arr.Regions) == 0 {
			a.Toast(ui.Toast{
				Text: "Close the red endpoints first — extrude needs a closed region",
				Kind: ui.ToastWarn,
			})
			return false
		} else {
			a.Toast(ui.Toast{Text: "Click a filled region to extrude it", Kind: ui.ToastWarn})
			return false
		}
	}

	frame := s.Frame()
	origin := regionCentroid(arr, regions, frame)
	a.extrude.tool = tools.NewExtrudeTool(s.ID, regions, origin, frame.N)
	a.extrude.tool.ThroughDepth = a.throughAllDepth(origin, frame.N)
	// A sketch drawn on a face is nearly always meant to grow that body, not to
	// start a new one beside it (SPEC-UX §10). Subtract is one chip away, which
	// is the "cut a hole here" path.
	if s.OnFace {
		a.extrude.tool.Result = tools.ResultAdd
	}
	a.extrude.returnCamera = a.targetCamera()
	a.Mode = ModeExtrude
	a.rebuildExtrudePreview()

	a.tiltOffAxis(frame.N)
	return true
}

// throughAllDepth measures how far the extrude has to run to clear the whole
// scene along its axis (SPEC-UX §9.3).
//
// It is the furthest any existing geometry reaches from the sketch origin along
// the axis, in either direction, plus a margin. Taking the larger of the two
// directions means the same number works whichever way the arrow is pointing,
// so flipping a through-all extrude does not silently stop short.
//
// It is measured once, when the tool opens: the scene it must clear is the one
// that was there before this extrude, not one that includes its own preview.
func (a *App) throughAllDepth(origin, axis geom.Vec3) float64 {
	s := a.BuildScene()
	box := scene.FrameAll(&s)
	var reach float64
	for _, c := range box.Corners() {
		if d := math.Abs(c.Sub(origin).Dot(axis)); d > reach {
			reach = d
		}
	}
	return geom.SnapWithStep(reach+tools.ThroughAllMarginUnits, geom.SnapGrid)
}

// tiltOffAxis swings the camera away from an axis it is looking straight down.
//
// Sketch mode leaves the camera normal-on to the plane, which is exactly the
// worst angle for an extrude: the depth develops along the view direction and
// the arrow gizmo projects to a single point. This is the "camera pulls back
// slightly to show depth developing" of SPEC-UX §9.1, tested geometrically so
// it works on every plane rather than only the vertical ones.
func (a *App) tiltOffAxis(axis geom.Vec3) {
	const tooCloseToTheAxis = 0.98
	to := a.targetCamera()
	if math.Abs(to.Forward().Dot(axis)) < tooCloseToTheAxis {
		return
	}
	// Look from a three-quarter angle built in the axis's own frame, so the
	// result is the same familiar view whichever plane this is.
	f := geom.FrameFromNormal(geom.Vec3{}, axis)
	to.LookAlong(f.N.Mul(0.7).Add(f.U.Mul(0.5)).Add(f.V.Mul(0.45)))
	a.Anim.Start(a.Camera, to)
}

// CancelExtrude drops the pending solid and goes back to the sketch.
func (a *App) CancelExtrude() {
	a.dropExtrudePreview()
	a.extrude.tool = nil
	a.Mode = ModeSketch
	if a.ActiveSketch() == nil {
		a.Mode = ModeIdle
	}
}

// CommitExtrude runs the extrude through the bus as one step (SPEC-UX §9.5).
func (a *App) CommitExtrude() bool {
	t := a.extrude.tool
	s := a.ActiveSketch()
	if t == nil || s == nil {
		return false
	}
	if ok, why := t.Valid(); !ok {
		a.Toast(ui.Toast{Text: why, Kind: ui.ToastWarn})
		return false
	}
	// Enter reaches here even when the card's button is disabled, so the same
	// refusal has to be made twice.
	if a.extrude.previewErr != "" {
		a.Toast(ui.Toast{Text: a.extrude.previewErr, Kind: ui.ToastWarn})
		return false
	}

	cmd := &model.Extrude{
		Sketch:  t.SketchID,
		Regions: t.Regions,
		Params:  t.BuildParams(s.Frame()),
	}
	// Add and Intersect with nothing to reach fall back to a new body rather
	// than refusing, and say so (SPEC-UX §9.4).
	result := t.Result
	targets := a.extrudeTargets(result)
	if result.NeedsTarget() && len(targets) == 0 {
		result = tools.ResultNew
		a.Toast(ui.Toast{Text: "Nothing to combine with — created a new body"})
	}
	if op, combining := result.Op(); combining {
		cmd.Combine, cmd.Targets = &op, targets
	}

	if err := a.Bus.Run(cmd); err != nil {
		// A boolean that fails gets the standard message of SPEC-UX §11.3: it
		// promises nothing was lost and suggests something to try, which is more
		// use than the solver's own words. Anything else — a profile that will
		// not build, regions that touch — already says exactly what is wrong.
		text := capitalize(err.Error())
		if cmd.Combine != nil {
			text = csgFailureToast(err)
			a.dumpCSGRepro("extrude", a.extrude.previewMesh, targets)
		}
		a.Toast(ui.Toast{Text: text, Kind: ui.ToastError})
		return false
	}

	if cmd.Clamped() {
		a.Toast(ui.Toast{
			Text: fmt.Sprintf("Draft clamped to %.1f° — the profile was too tight",
				cmd.AchievedDraft()),
			Kind: ui.ToastWarn,
		})
	}
	for _, b := range cmd.Emptied() {
		a.Toast(ui.Toast{Text: b.Name + " was cut away entirely — undo brings it back"})
	}
	a.Toast(ui.Toast{Text: s.Name + " hidden — find it in the tree"})

	a.dropExtrudePreview()
	a.extrude.tool = nil
	a.sketch.session = nil
	a.Mode = ModeIdle
	a.selectExtrudeResult(cmd)
	return true
}

// selectExtrudeResult leaves the thing the extrude produced selected, whether
// that is a new body or the one it merged into.
func (a *App) selectExtrudeResult(cmd *model.Extrude) {
	if b := cmd.Body(); b != nil {
		a.Sel.Set(model.BodyRef(b.ID))
		return
	}
	for _, b := range cmd.CombinedInto() {
		if a.Doc().BodyByID(b.ID) != nil {
			a.Sel.Set(model.BodyRef(b.ID))
			return
		}
	}
	a.Sel.Clear()
}

// selectedRegionList turns the sketch's region selection into sorted indices.
func (a *App) selectedRegionList() []int {
	out := make([]int, 0, len(a.sketch.selectedRegions))
	for i := range a.sketch.selectedRegions {
		out = append(out, i)
	}
	// A stable order keeps the built solid identical run to run.
	for i := 1; i < len(out); i++ {
		v := out[i]
		j := i - 1
		for j >= 0 && out[j] > v {
			out[j+1] = out[j]
			j--
		}
		out[j+1] = v
	}
	return out
}

// regionCentroid is where the arrow attaches: the average of the selected
// regions' outer loops, lifted onto the plane.
func regionCentroid(arr sketch2d.Arrangement, regions []int, frame geom.Frame) geom.Vec3 {
	var sx, sy, n float64
	for _, i := range regions {
		if i < 0 || i >= len(arr.Regions) {
			continue
		}
		for _, p := range arr.Regions[i].Outer.Pts {
			sx += float64(p.X)
			sy += float64(p.Y)
			n++
		}
	}
	if n == 0 {
		return frame.O
	}
	return frame.ToWorld(geom.Vec2{X: sx / n / geom.Unit, Y: sy / n / geom.Unit})
}

// rebuildExtrudePreview rebuilds the pending solid after any option change.
//
// The preview is real geometry, built by the same code the commit uses, so what
// you see is exactly what you get — and a profile that cannot take the draft
// says so before you commit rather than after.
func (a *App) rebuildExtrudePreview() {
	t := a.extrude.tool
	s := a.ActiveSketch()
	a.dropExtrudePreview()
	a.extrude.previewErr = ""
	a.extrude.previewMesh, a.extrude.targets = nil, nil
	if t == nil || s == nil {
		return
	}
	if ok, why := t.Valid(); !ok {
		a.extrude.previewErr = why
		return
	}

	arr := s.Arrangement()
	picked := make([]sketch2d.Region, 0, len(t.Regions))
	for _, i := range t.Regions {
		if i >= 0 && i < len(arr.Regions) {
			picked = append(picked, arr.Regions[i])
		}
	}
	built, err := extrude.Build(picked, t.BuildParams(s.Frame()), 0)
	if err != nil {
		a.extrude.previewErr = capitalize(err.Error())
		return
	}
	t.AchievedDraft, t.Clamped = built.AchievedDraft, built.Clamped
	a.extrude.previewMesh = built.Mesh
	a.extrude.targets = a.bodiesReachedBy(built.Mesh)

	g := render.BuildBodyGPU(built.Mesh)
	g.Upload()
	a.extrude.preview = g
}

// bodiesReachedBy lists the visible bodies the pending solid runs into, newest
// last, which is the order SPEC-UX §9.4 calls topmost.
//
// The test is bounding boxes, not geometry. Running a real intersection every
// time the draft slider moves would cost more than the extrude itself, and a
// box that overlaps but does not touch costs only a chip being offered that
// turns out to do nothing — whereas missing a real overlap would disable the
// chip the user wanted.
func (a *App) bodiesReachedBy(m *mesh.Mesh) []uint32 {
	if m == nil {
		return nil
	}
	box := m.AABB()
	var out []uint32
	for _, b := range a.Doc().Bodies {
		if !b.Visible || b.Mesh == nil {
			continue
		}
		if b.Mesh.AABB().Intersects(box) {
			out = append(out, b.ID)
		}
	}
	return out
}

// extrudeTargets narrows the reachable bodies to the ones a given result acts
// on: everything it meets for Subtract, the topmost for Add and Intersect.
func (a *App) extrudeTargets(r tools.Result) []uint32 {
	all := a.extrude.targets
	if len(all) == 0 || !r.NeedsTarget() {
		return nil
	}
	if r == tools.ResultSubtract {
		return append([]uint32(nil), all...)
	}
	// A face sketch belongs to the body it was drawn on, so that body is the
	// target whether or not it happens to be topmost (SPEC-UX §10).
	if s := a.ActiveSketch(); s != nil && s.OnFace {
		for _, id := range all {
			if id == s.Body {
				return []uint32{id}
			}
		}
	}
	return []uint32{all[len(all)-1]}
}

func (a *App) dropExtrudePreview() {
	if a.extrude.preview != nil {
		a.extrude.preview.Unload()
		a.extrude.preview = nil
	}
}

// arrowLength is the gizmo's world length at the current zoom.
func (a *App) arrowLength(vp render.Viewport) float64 {
	return scene.ArrowLengthWorld(a.Camera, vp)
}

// updateExtrude runs one frame of the tool's pointer input.
func (a *App) updateExtrude(in InputFrame, vp render.Viewport) {
	t := a.extrude.tool
	if t == nil {
		return
	}
	length := a.arrowLength(vp)
	ax, ay, bx, by, ok := scene.ArrowScreenEnds(a.Camera, vp, t.Origin, t.ArrowDirection(), length)
	if !ok {
		return
	}

	dist, along := tools.AxisDistancePx(in.MouseX, in.MouseY, ax, ay, bx, by)
	a.extrude.hoverArrow = dist <= tools.ArrowGrabRadiusPx

	if in.Pressed[MouseLeft] && a.extrude.hoverArrow {
		t.BeginDrag(along)
	}
	if t.Dragging() {
		if in.Down[MouseLeft] {
			// One screen pixel along the arrow is this much depth.
			unitsPerPixel := length / tools.ArrowScreenLength
			if t.Flipped() {
				unitsPerPixel = -unitsPerPixel
			}
			t.UpdateDrag(along, unitsPerPixel, snapStep(in))
			a.rebuildExtrudePreview()
		} else {
			t.EndDrag()
		}
	}
}

// snapStep maps the held modifiers onto the snap granularity (SPEC-UX §16).
func snapStep(in InputFrame) geom.SnapStep {
	switch {
	case in.Alt:
		return geom.SnapNone
	case in.Ctrl:
		return geom.SnapFine
	default:
		return geom.SnapGrid
	}
}

// handleExtrudeKeys implements the tool's shortcuts.
func (a *App) handleExtrudeKeys(in InputFrame) {
	if in.Ctrl {
		return
	}
	if in.KeyPressed(rl.KeyEscape) {
		a.CancelExtrude()
	}
	if in.KeyPressed(rl.KeyEnter) || in.KeyPressed(rl.KeyKpEnter) {
		a.CommitExtrude()
	}
}

// buildExtrudeGizmo assembles the arrow for this frame.
func (a *App) buildExtrudeGizmo(vp render.Viewport) *render.Overlay {
	t := a.extrude.tool
	if t == nil {
		return nil
	}
	return scene.BuildArrowGizmo(scene.ArrowView{
		Origin:      t.Origin,
		Dir:         t.ArrowDirection(),
		LengthWorld: a.arrowLength(vp),
		Hovered:     a.extrude.hoverArrow,
		Dragging:    t.Dragging(),
	})
}

// extrudePreviewDraw is the translucent pending body.
func (a *App) extrudePreviewDraw() (render.BodyDraw, bool) {
	if a.extrude.preview == nil {
		return render.BodyDraw{}, false
	}
	return render.BodyDraw{
		GPU:       a.extrude.preview,
		Color:     model.AutoBodyColor(int(a.Doc().Seq.Body)),
		Alpha:     PreviewAlpha,
		Transform: geom.Identity(),
	}, true
}

// extrudeHint is what the hint bar says while the tool is open.
func (a *App) extrudeHint() string {
	t := a.extrude.tool
	if t == nil {
		return ""
	}
	if t.Dragging() {
		return fmt.Sprintf("Depth %s u · Ctrl for quarter units · Alt for free", t.DepthLabel())
	}
	if a.extrude.previewErr != "" {
		return a.extrude.previewErr
	}
	return "Drag the arrow to set the depth · Enter to confirm · Esc to cancel"
}

// canExtrude reports whether the Extrude button would do anything right now,
// and what to do about it if not (SPEC-UX §15).
//
// The answer is exactly the set of things BeginExtrude accepts: a sketch being
// edited with a closed region in it, or one selected in the tree or the
// viewport with the same.
func (a *App) canExtrude() (bool, string) {
	const why = "Select a sketch with a closed profile — or press S to draw one"
	s := a.ActiveSketch()
	if s == nil {
		if ref, ok := a.Sel.Primary(); ok && ref.Kind == model.SelSketch {
			s = a.Doc().SketchByID(ref.Sketch)
		}
	}
	if s == nil {
		return false, why
	}
	if len(s.Arrangement().Regions) == 0 {
		return false, "Close the red endpoints first — extrude needs a closed region"
	}
	return true, ""
}
