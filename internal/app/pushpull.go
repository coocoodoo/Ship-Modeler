package app

import (
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

// Push/pull in the app (R11, SPEC-UX §12.5).
//
// Select a flat face in Idle and an arrow appears on it. Drag the arrow and a
// translucent preview of the material being added or removed follows the
// pointer; the boolean runs once, on release. Previews never run CSG
// (SPEC-GEOMETRY §6.6), and a tool this immediate is exactly why: at sixty
// frames a second a boolean per frame would turn a fluid drag into a slideshow.

// PushPullTeachCount is how many times the hint bar volunteers what the arrow
// is for before assuming it has been understood (SPEC-UX §12.5).
const PushPullTeachCount = 3

// pushPullState is the app's half of the tool.
type pushPullState struct {
	tool *tools.PushPullTool
	// preview is the material the drag would add or remove.
	preview     *render.BodyGPU
	previewMesh *mesh.Mesh
	// hoverArrow is true when the pointer is over the gizmo.
	hoverArrow bool
	// taught counts how often the hint has been offered.
	taught int
}

// InPushPull reports whether a face is armed for push/pull.
func (a *App) InPushPull() bool { return a.pushPull.tool != nil }

// armPushPull puts the arrow on the selected face, or takes it away when the
// selection is not a single flat face. Called every frame from Idle: the tool
// is a property of the selection, not a mode you enter.
func (a *App) armPushPull() {
	f, ok := a.selectedFace()
	if !ok {
		a.dropPushPull()
		return
	}
	if flat, _ := flatEnoughToSketchOn(f.body.Mesh, f.face); !flat {
		a.dropPushPull()
		return
	}
	if t := a.pushPull.tool; t != nil && t.Body == f.body.ID && t.Face == f.uid {
		return // already armed on this face
	}
	a.dropPushPull()
	a.pushPull.tool = tools.NewPushPullTool(f.body.ID, f.uid,
		f.body.Mesh.FaceCentroid(f.face), f.body.Mesh.FaceNormal(f.face))
	if a.pushPull.taught < PushPullTeachCount {
		a.pushPull.taught++
	}
}

// dropPushPull disarms the tool and releases its preview.
func (a *App) dropPushPull() {
	if a.pushPull.preview != nil {
		a.pushPull.preview.Unload()
		a.pushPull.preview = nil
	}
	a.pushPull.previewMesh = nil
	a.pushPull.tool = nil
	a.pushPull.hoverArrow = false
}

// updatePushPull runs one frame of the arrow drag.
func (a *App) updatePushPull(in InputFrame, vp render.Viewport) {
	t := a.pushPull.tool
	if t == nil {
		return
	}
	length := a.arrowLength(vp)
	ax, ay, bx, by, ok := scene.ArrowScreenEnds(a.Camera, vp, t.Origin, t.ArrowDirection(), length)
	if !ok {
		return
	}
	dist, along := tools.AxisDistancePx(in.MouseX, in.MouseY, ax, ay, bx, by)
	a.pushPull.hoverArrow = dist <= tools.ArrowGrabRadiusPx

	if in.Pressed[MouseLeft] && a.pushPull.hoverArrow {
		t.BeginDrag(along)
	}
	if !t.Dragging() {
		return
	}
	if in.Down[MouseLeft] {
		unitsPerPixel := length / tools.ArrowScreenLength
		if !t.Adding() {
			unitsPerPixel = -unitsPerPixel
		}
		t.UpdateDrag(along, unitsPerPixel, snapStep(in))
		a.rebuildPushPullPreview()
		return
	}
	// Released: this is the one moment CSG runs.
	t.EndDrag()
	a.CommitPushPull()
}

// grabbedArrow reports whether a push/pull drag owns the pointer, which keeps
// the same click from also re-picking whatever is behind the arrow.
func (a *App) grabbedArrow() bool {
	t := a.pushPull.tool
	return t != nil && (t.Dragging() || a.pushPull.hoverArrow)
}

// rebuildPushPullPreview rebuilds the prism the drag would add or remove.
func (a *App) rebuildPushPullPreview() {
	t := a.pushPull.tool
	if a.pushPull.preview != nil {
		a.pushPull.preview.Unload()
		a.pushPull.preview = nil
	}
	a.pushPull.previewMesh = nil
	if t == nil || !t.Active() {
		return
	}
	f, ok := a.resolveFace(t.Body, t.Face)
	if !ok {
		return
	}
	frame := f.body.Mesh.FaceFrame(f.face)
	region, err := model.FaceRegion(f.body.Mesh, f.face, frame)
	if err != nil {
		return
	}
	dir := extrude.Normal
	if !t.Adding() {
		dir = extrude.Reverse
	}
	depth := t.Distance()
	if depth < 0 {
		depth = -depth
	}
	built, err := extrude.Build([]sketch2d.Region{region},
		extrude.Params{Frame: frame, Depth: depth, Dir: dir}, 0)
	if err != nil {
		return
	}
	a.pushPull.previewMesh = built.Mesh
	g := render.BuildBodyGPU(built.Mesh)
	g.Upload()
	a.pushPull.preview = g
}

// CommitPushPull runs the move through the bus as one step.
func (a *App) CommitPushPull() bool {
	t := a.pushPull.tool
	if t == nil || !t.Active() {
		if t != nil {
			t.Cancel()
		}
		a.rebuildPushPullPreview()
		return false
	}
	cmd := &model.PushPull{Body: t.Body, Face: t.Face, Distance: t.Distance()}
	verb := "Pulled the face out "
	if !t.Adding() {
		verb = "Pushed the face in "
	}

	if err := a.Bus.Run(cmd); err != nil {
		a.Toast(ui.Toast{Text: csgFailureToast(err), Kind: ui.ToastError})
		a.dumpCSGRepro("pushpull", a.pushPull.previewMesh, []uint32{t.Body})
		t.Cancel()
		a.rebuildPushPullPreview()
		return false
	}
	a.Toast(ui.Toast{Text: verb + t.Label() + " u"})
	if cmd.Emptied() {
		a.Toast(ui.Toast{Text: "That took the whole body — undo brings it back"})
		a.Sel.Clear()
	}

	// The face has been replaced by whatever the boolean produced, so the tool
	// disarms; the next click re-arms it on whatever face is there now.
	a.dropPushPull()
	return true
}

// pushPullPreviewDraw is the translucent material the drag would add or remove.
func (a *App) pushPullPreviewDraw() (render.BodyDraw, bool) {
	if a.pushPull.preview == nil || a.pushPull.tool == nil {
		return render.BodyDraw{}, false
	}
	// Adding shows in the accent, cutting in the error colour: the same
	// vocabulary the boolean tool's tinting uses.
	col := ui.ColorAccent
	if !a.pushPull.tool.Adding() {
		col = ui.ColorError
	}
	return render.BodyDraw{
		GPU:       a.pushPull.preview,
		Color:     col,
		Alpha:     PreviewAlpha,
		Transform: geom.Identity(),
	}, true
}

// buildPushPullGizmo assembles the face's arrow for this frame.
func (a *App) buildPushPullGizmo(vp render.Viewport) *render.Overlay {
	t := a.pushPull.tool
	if t == nil {
		return nil
	}
	return scene.BuildArrowGizmo(scene.ArrowView{
		Origin:      t.Origin,
		Dir:         t.ArrowDirection(),
		LengthWorld: a.arrowLength(vp),
		Hovered:     a.pushPull.hoverArrow,
		Dragging:    t.Dragging(),
	})
}

// pushPullHint is what the hint bar says when a face is armed.
func (a *App) pushPullHint() string {
	t := a.pushPull.tool
	if t == nil {
		return ""
	}
	if t.Dragging() {
		return t.Hint()
	}
	if a.pushPull.taught <= PushPullTeachCount {
		// Volunteered the first few times and then left alone, because a hint
		// that never stops is not a hint (SPEC-UX §12.5).
		return "S to sketch on this face · drag the arrow to push/pull"
	}
	return "S to sketch on this face"
}
