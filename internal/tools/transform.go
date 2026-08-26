package tools

import (
	"fmt"
	"math"

	"modeler/internal/geom"
)

// The move and rotate gizmos (R10, R11, SPEC-UX §12.2, §12.4).
//
// This package holds what the gizmo *is* — which part is grabbed, how far the
// drag has got, what the snap makes of it. Where those parts land on screen is
// the camera's business and lives in package scene; turning a pointer position
// into a distance along an axis or an angle about a ring is the app's, because
// only it has both. What comes back here is a raw world-space quantity, and
// what leaves is a snapped one.

// Gizmo sizes, in screen pixels so the gizmo is equally usable at any zoom.
const (
	// GizmoAxisLengthPx is how long each axis arrow is drawn.
	GizmoAxisLengthPx = 84.0
	// GizmoPlaneOffsetPx and GizmoPlaneSizePx place the three planar squares.
	GizmoPlaneOffsetPx = 26.0
	GizmoPlaneSizePx   = 22.0
	// GizmoCenterRadiusPx is the screen-plane handle at the pivot.
	GizmoCenterRadiusPx = 9.0
	// GizmoRingRadiusPx is the rotation rings' radius.
	GizmoRingRadiusPx = 76.0
	// GizmoGrabPx is how close the pointer must come to a part to grab it.
	GizmoGrabPx = 10.0
)

// GizmoMode is which gizmo the card is showing (SPEC-UX §12.4).
type GizmoMode uint8

const (
	// GizmoMove translates the selection.
	GizmoMove GizmoMode = iota
	// GizmoRotate turns it about the pivot.
	GizmoRotate
)

func (m GizmoMode) String() string {
	if m == GizmoRotate {
		return "Rotate"
	}
	return "Move"
}

// GizmoPart identifies one handle.
type GizmoPart uint8

const (
	PartNone GizmoPart = iota
	PartAxisX
	PartAxisY
	PartAxisZ
	PartPlaneYZ
	PartPlaneZX
	PartPlaneXY
	PartScreen
	PartRingX
	PartRingY
	PartRingZ
)

// MoveParts and RotateParts are the handles each mode offers, in draw order.
func MoveParts() []GizmoPart {
	return []GizmoPart{
		PartPlaneYZ, PartPlaneZX, PartPlaneXY,
		PartAxisX, PartAxisY, PartAxisZ, PartScreen,
	}
}

func RotateParts() []GizmoPart { return []GizmoPart{PartRingX, PartRingY, PartRingZ} }

// Axis is the world axis a part constrains to. For the planar handles it is the
// plane's normal; for the screen handle there is none.
func (p GizmoPart) Axis() (geom.Vec3, bool) {
	switch p {
	case PartAxisX, PartPlaneYZ, PartRingX:
		return geom.AxisX, true
	case PartAxisY, PartPlaneZX, PartRingY:
		return geom.AxisY, true
	case PartAxisZ, PartPlaneXY, PartRingZ:
		return geom.AxisZ, true
	}
	return geom.Vec3{}, false
}

// IsAxis, IsPlane and IsRing say which kind of handle a part is, which is what
// decides how a drag on it is interpreted.
func (p GizmoPart) IsAxis() bool  { return p >= PartAxisX && p <= PartAxisZ }
func (p GizmoPart) IsPlane() bool { return p >= PartPlaneYZ && p <= PartPlaneXY }
func (p GizmoPart) IsRing() bool  { return p >= PartRingX && p <= PartRingZ }

func (p GizmoPart) String() string {
	switch p {
	case PartAxisX:
		return "X"
	case PartAxisY:
		return "Y"
	case PartAxisZ:
		return "Z"
	case PartPlaneYZ:
		return "YZ"
	case PartPlaneZX:
		return "ZX"
	case PartPlaneXY:
		return "XY"
	case PartScreen:
		return "screen"
	case PartRingX:
		return "about X"
	case PartRingY:
		return "about Y"
	case PartRingZ:
		return "about Z"
	}
	return ""
}

// TransformTool is the live state of a move or rotate interaction.
type TransformTool struct {
	Mode  GizmoMode
	Pivot geom.Vec3

	// Delta is the accumulated translation, and Angle the accumulated turn in
	// degrees about the grabbed ring's axis. Both are snapped.
	Delta geom.Vec3
	Angle float64

	// Hover is the part under the pointer, and Active the one being dragged.
	Hover  GizmoPart
	Active GizmoPart

	dragging bool
	// anchor is the raw quantity at the moment the drag began, subtracted from
	// every update so the drag is absolute rather than a running sum.
	anchorDelta geom.Vec3
	anchorAngle float64
}

// NewTransformTool arms the gizmo at a pivot.
func NewTransformTool(pivot geom.Vec3) *TransformTool {
	return &TransformTool{Pivot: pivot}
}

// Dragging reports whether a handle is being dragged.
func (t *TransformTool) Dragging() bool { return t.dragging }

// Moved reports whether the drag has come to anything worth committing.
func (t *TransformTool) Moved() bool {
	if t.Mode == GizmoRotate {
		return math.Abs(t.Angle) >= 1e-6
	}
	return t.Delta.Len() >= 1.0/geom.Unit
}

// Begin starts a drag on a part, anchored to the raw quantity under the pointer.
func (t *TransformTool) Begin(part GizmoPart, rawDelta geom.Vec3, rawAngle float64) {
	if part == PartNone {
		return
	}
	t.dragging = true
	t.Active = part
	t.anchorDelta = rawDelta.Sub(t.Delta)
	t.anchorAngle = rawAngle - t.Angle
}

// End finishes the drag, leaving the accumulated value for the caller to commit.
func (t *TransformTool) End() { t.dragging = false; t.Active = PartNone }

// Reset clears the accumulated transform, which is what happens after a commit
// or a cancel.
func (t *TransformTool) Reset() {
	t.dragging = false
	t.Active = PartNone
	t.Delta = geom.Vec3{}
	t.Angle = 0
}

// UpdateMove takes the raw world-space point the pointer maps to on the grabbed
// handle's constraint and turns it into a snapped delta.
func (t *TransformTool) UpdateMove(rawDelta geom.Vec3, snap geom.SnapStep) {
	if !t.dragging {
		return
	}
	d := rawDelta.Sub(t.anchorDelta)
	t.Delta = geom.Vec3{
		X: geom.SnapWithStep(d.X, snap),
		Y: geom.SnapWithStep(d.Y, snap),
		Z: geom.SnapWithStep(d.Z, snap),
	}
}

// SetDelta stores a translation typed into the card's number fields.
func (t *TransformTool) SetDelta(d geom.Vec3) { t.Delta = d }

// UpdateRotate turns a raw angle into a detented one (SPEC-UX §12.4).
func (t *TransformTool) UpdateRotate(rawDegrees float64, detent RotateDetent) {
	if !t.dragging {
		return
	}
	t.Angle = detent.snap(rawDegrees - t.anchorAngle)
}

// RotateDetent is how firmly a rotation snaps.
type RotateDetent uint8

const (
	// DetentQuarter is the default: quarter turns, which stay exactly on the
	// grid (SPEC-GEOMETRY §7.3).
	DetentQuarter RotateDetent = iota
	// DetentFifteen is Shift held.
	DetentFifteen
	// DetentFree is Alt held.
	DetentFree
)

func (d RotateDetent) snap(deg float64) float64 {
	switch d {
	case DetentFifteen:
		return math.Round(deg/15) * 15
	case DetentFree:
		return deg
	default:
		return math.Round(deg/90) * 90
	}
}

// RotateDetentFor maps the held modifiers onto the detent, matching the snap
// vocabulary the rest of the tools use (SPEC-UX §16).
func RotateDetentFor(shift, alt bool) RotateDetent {
	switch {
	case alt:
		return DetentFree
	case shift:
		return DetentFifteen
	default:
		return DetentQuarter
	}
}

// Axis is the world axis of a live rotation.
func (t *TransformTool) RotationAxis() geom.Vec3 {
	if axis, ok := t.Active.Axis(); ok {
		return axis
	}
	return geom.AxisY
}

// Label is the readout: an offset for a move, an angle for a rotation.
func (t *TransformTool) Label() string {
	if t.Mode == GizmoRotate {
		return fmt.Sprintf("%+g°", round4(t.Angle))
	}
	return fmt.Sprintf("%+g, %+g, %+g",
		round4(t.Delta.X), round4(t.Delta.Y), round4(t.Delta.Z))
}

// Hint is what the hint bar says while a handle is grabbed.
func (t *TransformTool) Hint() string {
	if !t.dragging {
		return ""
	}
	if t.Mode == GizmoRotate {
		return fmt.Sprintf("Rotating %s %s · Shift for 15° · Alt for free",
			t.Active, t.Label())
	}
	return fmt.Sprintf("Moving along %s: %s · Ctrl for quarter units · Alt for free",
		t.Active, t.Label())
}
