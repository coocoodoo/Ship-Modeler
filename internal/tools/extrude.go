// Package tools holds the modelling tools' state and the pure logic behind
// their gizmos: what a drag means, what an option card holds, when a control is
// allowed. The app wires them to input and the renderer draws them; nothing
// here touches either.
package tools

import (
	"fmt"
	"math"

	"modeler/internal/geom"
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

// Available reports whether a result mode can be used yet. Add, Subtract and
// Intersect all need the boolean kernel, which arrives with M4; until then they
// are shown but disabled, with a tooltip that says so (SPEC-UX §9.3).
func (r Result) Available() bool { return r == ResultNew }

// UnavailableReason is what a disabled result chip's tooltip says.
func (r Result) UnavailableReason() string {
	if r.Available() {
		return ""
	}
	return r.String() + " needs the boolean kernel, which arrives with milestone M4"
}

// ExtrudeTool is the live state of one extrude interaction (SPEC-UX §9.3).
type ExtrudeTool struct {
	// SketchID and Regions say what is being extruded.
	SketchID uint32
	Regions  []int

	// DepthUnits is the signed drag distance. Its sign is folded into Dir when
	// the tool is read, so dragging back through zero flips the extrusion
	// rather than producing a negative solid.
	DepthUnits float64
	Draft      float64
	Dir        extrude.Direction
	Result     Result

	// ThroughAll replaces the dragged depth with one that clears the whole
	// scene (SPEC-UX §9.3). ThroughDepth is that distance, measured from the
	// scene by the app when the tool opens; the tool only decides when to use
	// it, because measuring a scene is not this package's business.
	ThroughAll   bool
	ThroughDepth float64

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
	if !t.ThroughAll {
		return math.Abs(t.DepthUnits)
	}
	if t.Dir == extrude.Symmetric {
		return 2 * t.ThroughDepth
	}
	return t.ThroughDepth
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
	depth := t.EffectiveDepth()
	dir := t.Dir
	if t.Flipped() {
		switch dir {
		case extrude.Normal:
			dir = extrude.Reverse
		case extrude.Reverse:
			dir = extrude.Normal
		}
	}
	return extrude.Params{
		Frame: frame,
		Depth: geom.ToSubunits(depth),
		Draft: t.Draft,
		Dir:   dir,
	}
}

// Valid reports whether the tool has enough to build, and why not if it does
// not. Every disabled control has to say how to enable it (SPEC-UX §15).
func (t *ExtrudeTool) Valid() (bool, string) {
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
