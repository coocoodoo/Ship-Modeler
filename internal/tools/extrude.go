// Package tools holds the modelling tools' state and the pure logic behind
// their gizmos: what a drag means, what an option card holds, when a control is
// allowed. The app wires them to input and the renderer draws them; nothing
// here touches either.
package tools

import (
	"fmt"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/csg"
	"modeler/internal/geom/extrude"
)

// Extrude option limits and defaults (SPEC-UX §9.2, §9.3).
const (
	// DefaultDepthUnits is the depth a preview opens at, so something is always
	// visible the moment the tool starts.
	DefaultDepthUnits = 1.0
	// MaxDepthUnits bounds the drag-number field.
	MaxDepthUnits = 4096.0
	// ArrowScreenLength is the gizmo's on-screen length in pixels, kept
	// constant so it is equally grabbable at any zoom.
	ArrowScreenLength = 90.0
	// ArrowGrabRadiusPx is how close the pointer must come to the arrow's axis.
	ArrowGrabRadiusPx = 12.0
	// ThroughAllMarginUnits is how far past the last body a through-all extrude
	// runs. Stopping exactly level with a face would leave the M4 subtract with
	// a coincident-face cut to resolve, which is the hardest case there is and
	// an entirely avoidable one.
	ThroughAllMarginUnits = 1.0
)

// Result is what an extrude does with the solid it builds (R8, SPEC-UX §9.4).
type Result uint8

const (
	// ResultNew makes an independent body.
	ResultNew Result = iota
	// ResultAdd unions into a target body.
	ResultAdd
	// ResultSubtract cuts the solid out of its targets.
	ResultSubtract
	// ResultIntersect keeps only the volume shared with a target.
	ResultIntersect
)

func (r Result) String() string {
	switch r {
	case ResultAdd:
		return "Add"
	case ResultSubtract:
		return "Subtract"
	case ResultIntersect:
		return "Intersect"
	default:
		return "New"
	}
}

// Op is the boolean this result runs, and whether it runs one at all.
func (r Result) Op() (csg.Op, bool) {
	switch r {
	case ResultAdd:
		return csg.Union, true
	case ResultSubtract:
		return csg.Subtract, true
	case ResultIntersect:
		return csg.Intersect, true
	default:
		return csg.Union, false
	}
}

// NeedsTarget reports whether this result has to find a body to act on.
func (r Result) NeedsTarget() bool { _, ok := r.Op(); return ok }

// Available reports whether a result mode can be used given what the extrude
// would actually touch. Add, Subtract and Intersect need something to combine
// with; New never does (SPEC-UX §9.4).
func (r Result) Available(targets int) bool {
	return !r.NeedsTarget() || targets > 0
}

// UnavailableReason is what a disabled result chip's tooltip says. A disabled
// control must always say how to enable it (SPEC-UX §15).
func (r Result) UnavailableReason(targets int) string {
	if r.Available(targets) {
		return ""
	}
	return "Nothing to " + r.String() + " with — the extrude does not reach another body"
}

// ExtrudeTool is the live state of one extrude interaction (SPEC-UX §9.3).
type ExtrudeTool struct {
	// SketchID and Regions say what is being extruded.
	SketchID uint32
	Regions  []int

	// DepthUnits is the signed drag distance. Its sign is folded into Dir when
	// the tool is read, so dragging back through zero flips the extrusion
	// rather than producing a negative solid.
	DepthUnits                float64
	Draft                     float64
	Dir                       extrude.Direction
	Result                    Result
	Extent                    Extent
	TargetReady               bool
	TargetPoint, TargetNormal geom.Vec3

	// ThroughAll replaces the dragged depth with one that clears the whole
	// scene (SPEC-UX §9.3). ThroughDepth is that distance, measured from the
	// scene by the app when the tool opens; the tool only decides when to use
	// it, because measuring a scene is not this package's business.
	ThroughAll   bool
	ThroughDepth float64

	// OnFace records that the sketch was drawn on a body's face, which is what
	// lets the tool know there is an inside and an outside to aim at.
	OnFace bool

	// Straddles records that the scene has material on both sides of the sketch
	// plane along this axis.
	//
	// It exists because of what "past everything" has to mean there. The three
	// default planes all pass through the origin, and so through the middle of
	// most ships; running one direction from a plane inside the material starts
	// the cut in the middle of it and leaves a blind pocket where a hole was
	// asked for. So turning Through all on in that case reaches both ways.
	Straddles bool

	// AchievedDraft and Clamped mirror the last build, so the card can turn the
	// draft field warn-orange when the profile could not take the angle.
	AchievedDraft float64
	Clamped       bool

	// Origin is where the arrow starts, and Axis is the sketch plane's normal.
	Origin geom.Vec3
	Axis   geom.Vec3

	// dragging tracks a live arrow drag.
	dragging   bool
	dragStart  float64
	dragAnchor float64
}

// NewExtrudeTool opens the tool on a selection, at the default depth so a
// preview appears immediately (SPEC-UX §9.2).
func NewExtrudeTool(sketchID uint32, regions []int, origin, axis geom.Vec3) *ExtrudeTool {
	return &ExtrudeTool{
		SketchID:   sketchID,
		Regions:    append([]int(nil), regions...),
		DepthUnits: DefaultDepthUnits,
		Origin:     origin,
		Axis:       axis,
	}
}

// Flipped reports whether the drag has been pulled back past zero. Through-All
// changes how far the extrude runs, never which way, so this stays the sign of
// the drag either way.
func (t *ExtrudeTool) Flipped() bool { return t.DepthUnits < 0 }

// EffectiveDepth is the unsigned distance the geometry will actually run: the
// dragged depth, or the scene-spanning one while Through-All is on. Symmetric
// splits the depth around the plane, so clearing the scene in both directions
// takes twice the reach.
func (t *ExtrudeTool) EffectiveDepth() float64 {
	if t.Extent != ExtentDistance {
		return math.Abs(t.DepthUnits)
	}
	if !t.ThroughAll {
		return math.Abs(t.DepthUnits)
	}
	if t.Dir == extrude.Symmetric {
		return 2 * t.ThroughDepth
	}
	return t.ThroughDepth
}

// SetThroughAll turns the toggle on or off, choosing a direction that makes
// "past everything" true when the sketch plane sits inside the model.
//
// The direction stays the user's to change afterwards: picking Normal with
// Through all on then means "past everything ahead of the plane", which is a
// real thing to want and is one chip away.
func (t *ExtrudeTool) SetThroughAll(on bool) {
	t.ThroughAll = on
	if on && t.Straddles && t.Dir != extrude.Symmetric {
		t.Dir = extrude.Symmetric
	}
}

// ThroughAllNote is what the card says the toggle resolved to, so the choice is
// visible rather than inferred from a number.
func (t *ExtrudeTool) ThroughAllNote() string {
	if !t.ThroughAll {
		return ""
	}
	if t.Dir == extrude.Symmetric {
		return "both ways"
	}
	if t.Straddles {
		return "one way only"
	}
	return "past everything"
}

// ArrowDirection is the way the gizmo points right now, which follows the sign
// of the depth so the arrow visibly flips when the drag crosses zero.
func (t *ExtrudeTool) ArrowDirection() geom.Vec3 {
	if t.Flipped() {
		return t.Axis.Neg()
	}
	return t.Axis
}

// BuildParams turns the tool's state into geometry parameters.
//
// A negative depth is not passed through as a negative extrusion: it is folded
// into the direction, because the geometry only ever builds along a positive
// run and "dragging through zero flips it" is a UI idea, not a geometric one.
func (t *ExtrudeTool) BuildParams(frame geom.Frame) extrude.Params {
	return extrude.Params{
		EndPlane: t.Extent != ExtentDistance && t.TargetReady,
		EndPoint: t.TargetPoint, EndNormal: t.TargetNormal,
		Frame: frame,
		Depth: geom.ToSubunits(t.EffectiveDepth()),
		Draft: t.Draft,
		Dir:   t.EffectiveDir(),
	}
}

// EffectiveDir is the direction the geometry will actually run in, with the
// sign of the dragged depth already folded in.
func (t *ExtrudeTool) EffectiveDir() extrude.Direction {
	dir := t.Dir
	if t.Flipped() {
		switch dir {
		case extrude.Normal:
			dir = extrude.Reverse
		case extrude.Reverse:
			dir = extrude.Normal
		}
	}
	return dir
}

// SetResult picks what the extrude does with its solid, aiming the solid the
// only way the choice can mean anything when the sketch is on a face.
//
// A face's normal points out of the body, so a sketch drawn on one opens
// pointing outward — right for Add, and useless for Subtract: the solid sits
// against the outside and the boolean takes nothing away. Choosing "cut this
// out of the body I am drawn on" has to mean cutting into it, so the sense of
// the depth follows the choice.
//
// This is the same shape as SetThroughAll (V-76): the option decides a
// direction the user cannot have meant otherwise, and an explicit flip
// afterwards is still theirs to make.
func (t *ExtrudeTool) SetResult(r Result) {
	t.Result = r
	if t.Extent != ExtentDistance {
		return
	}
	if !t.OnFace {
		return
	}
	want := extrude.Normal // outward: growing the body, or a new one beside it
	if r == ResultSubtract {
		want = extrude.Reverse // inward: cutting into it
	}
	if t.Dir != extrude.Symmetric && t.EffectiveDir() != want {
		t.Flip()
	}
}

// Valid reports whether the tool has enough to build, and why not if it does
// not. Every disabled control has to say how to enable it (SPEC-UX §15).
func (t *ExtrudeTool) Valid() (bool, string) {
	if t.Extent != ExtentDistance && !t.TargetReady {
		return false, "Select a target " + t.Extent.TargetName() + " in the viewport"
	}
	if len(t.Regions) == 0 {
		return false, "Select a closed region — close the red endpoints first"
	}
	if t.EffectiveDepth() < 1.0/geom.Unit {
		if t.ThroughAll {
			return false, "There is nothing to run through — turn Through all off"
		}
		return false, "Drag the arrow to give the extrude some depth"
	}
	return true, ""
}

// DepthLabel is the drag-number field's text.
func (t *ExtrudeTool) DepthLabel() string {
	depth := t.EffectiveDepth()
	if t.Flipped() {
		depth = -depth
	}
	return fmt.Sprintf("%g", round4(depth))
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// SetDepth stores a depth from the card's number field, clamped to the range
// the field advertises.
func (t *ExtrudeTool) SetDepth(units float64) {
	t.DepthUnits = clamp(units, -MaxDepthUnits, MaxDepthUnits)
}

// SetDraft stores a draft angle, clamped to the legal range.
func (t *ExtrudeTool) SetDraft(deg float64) {
	t.Draft = clamp(deg, -extrude.MaxDraftDegrees, extrude.MaxDraftDegrees)
}

// BeginDrag starts an arrow drag from a pointer position measured along the
// arrow's screen axis.
func (t *ExtrudeTool) BeginDrag(alongAxisPx float64) {
	t.dragging = true
	t.dragStart = alongAxisPx
	t.dragAnchor = t.DepthUnits
}

// Dragging reports whether an arrow drag is live.
func (t *ExtrudeTool) Dragging() bool { return t.dragging }

// EndDrag finishes the drag.
func (t *ExtrudeTool) EndDrag() { t.dragging = false }

// UpdateDrag converts a pointer position into a new depth.
//
// unitsPerPixel is how much world distance one screen pixel covers along the
// arrow, so the extrusion tracks the pointer at any zoom. The result is snapped
// to the grid, a quarter unit with Ctrl held, or left free with Alt
// (SPEC-UX §9.2).
func (t *ExtrudeTool) UpdateDrag(alongAxisPx float64, unitsPerPixel float64, snap geom.SnapStep) {
	if !t.dragging {
		return
	}
	moved := (alongAxisPx - t.dragStart) * unitsPerPixel
	t.SetDepth(geom.SnapWithStep(t.dragAnchor+moved, snap))
}

// Flip reverses the extrusion, which is what the card's flip button does.
func (t *ExtrudeTool) Flip() { t.DepthUnits = -t.DepthUnits }

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// AxisDistancePxClamped measures the distance to a screen-space line *segment*,
// with the projection clamped to its ends. Use it where the segment really is a
// segment — a chord of a ring, say — rather than a ray a drag can run past.
func AxisDistancePxClamped(px, py, ax, ay, bx, by float64) (dist, along float64) {
	dx, dy := bx-ax, by-ay
	den := dx*dx + dy*dy
	if den == 0 {
		return math.Hypot(px-ax, py-ay), 0
	}
	tt := ((px-ax)*dx + (py-ay)*dy) / den
	tt = clamp(tt, 0, 1)
	cx, cy := ax+tt*dx, ay+tt*dy
	return math.Hypot(px-cx, py-cy), tt * math.Sqrt(den)
}

// AxisDistancePx measures how far a point is from a screen-space line segment,
// and how far along it the projection falls. The gizmo uses it to decide
// whether the pointer has grabbed the arrow, and to turn a drag into a depth.
func AxisDistancePx(px, py, ax, ay, bx, by float64) (dist, along float64) {
	dx, dy := bx-ax, by-ay
	den := dx*dx + dy*dy
	if den == 0 {
		return math.Hypot(px-ax, py-ay), 0
	}
	// Unclamped: dragging past the arrow's tip must keep extruding, not stop.
	tt := ((px-ax)*dx + (py-ay)*dy) / den
	cx, cy := ax+tt*dx, ay+tt*dy
	return math.Hypot(px-cx, py-cy), tt * math.Sqrt(den)
}
