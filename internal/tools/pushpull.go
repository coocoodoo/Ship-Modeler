package tools

import (
	"fmt"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The push/pull drag (R11, SPEC-UX §12.5).
//
// One arrow, one number, and no mode to choose: the direction you drag decides
// whether material is added or removed. That is the whole idea — a face you can
// grab and move is how a blocky hull actually gets built, and stopping to say
// "now I would like to subtract" every time you want a recess is exactly the
// friction this tool exists to remove.

// PushPullTool is the live state of one face drag.
type PushPullTool struct {
	// Body and Face say which face is being moved.
	Body uint32
	Face mesh.FaceUID

	// Origin is where the arrow starts and Axis is the face's outward normal.
	Origin geom.Vec3
	Axis   geom.Vec3

	// DistanceUnits is the signed drag, positive along the normal.
	DistanceUnits float64

	dragging   bool
	dragStart  float64
	dragAnchor float64
}

// NewPushPullTool arms the tool on a face. It starts at zero: nothing has
// happened yet, and nothing should be previewed until the arrow is dragged.
func NewPushPullTool(body uint32, face mesh.FaceUID, origin, axis geom.Vec3) *PushPullTool {
	return &PushPullTool{Body: body, Face: face, Origin: origin, Axis: axis}
}

// Adding reports whether the current drag would add material rather than cut.
func (t *PushPullTool) Adding() bool { return t.DistanceUnits > 0 }

// Active reports whether the drag has moved far enough to mean anything.
func (t *PushPullTool) Active() bool {
	return math.Abs(t.DistanceUnits) >= 1.0/geom.Unit
}

// Distance is the signed move in subunits, which is what the command takes.
func (t *PushPullTool) Distance() int64 {
	return geom.ToSubunits(t.DistanceUnits)
}

// ArrowDirection is the way the gizmo points: along the face's normal until the
// drag turns inward, at which point it turns round too, because an arrow that
// keeps pointing out while the face goes in is a lie.
func (t *PushPullTool) ArrowDirection() geom.Vec3 {
	if t.DistanceUnits < 0 {
		return t.Axis.Neg()
	}
	return t.Axis
}

// BeginDrag starts a drag from a pointer position along the arrow's screen axis.
func (t *PushPullTool) BeginDrag(alongAxisPx float64) {
	t.dragging = true
	t.dragStart = alongAxisPx
	t.dragAnchor = t.DistanceUnits
}

// Dragging reports whether a drag is live.
func (t *PushPullTool) Dragging() bool { return t.dragging }

// EndDrag finishes the drag. The caller commits; this only stops tracking.
func (t *PushPullTool) EndDrag() { t.dragging = false }

// Cancel abandons a drag and returns the face to where it started.
func (t *PushPullTool) Cancel() {
	t.dragging = false
	t.DistanceUnits = 0
}

// UpdateDrag converts a pointer position into a signed distance, snapped the
// same way an extrude's depth is (SPEC-UX §12.5).
func (t *PushPullTool) UpdateDrag(alongAxisPx, unitsPerPixel float64, snap geom.SnapStep) {
	if !t.dragging {
		return
	}
	moved := (alongAxisPx - t.dragStart) * unitsPerPixel
	t.DistanceUnits = clamp(
		geom.SnapWithStep(t.dragAnchor+moved, snap), -MaxDepthUnits, MaxDepthUnits)
}

// Label is the distance readout, signed so the direction is never in doubt.
func (t *PushPullTool) Label() string {
	return fmt.Sprintf("%+g", round4(t.DistanceUnits))
}

// Hint is what the hint bar says during a drag.
func (t *PushPullTool) Hint() string {
	if !t.Active() {
		return "Drag the arrow to push or pull this face"
	}
	verb := "Pulling out"
	if !t.Adding() {
		verb = "Pushing in"
	}
	return fmt.Sprintf("%s %s u · Ctrl for quarter units · release to apply",
		verb, t.Label())
}
