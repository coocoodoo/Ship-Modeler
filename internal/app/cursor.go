package app

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// The cursor set of SPEC-UX §15.
//
// The cursor is the cheapest thing in the program that says what the pointer
// will do, and the only one that is right under where the user is looking. It
// is set once per frame from the mode and what is under the pointer, so there
// is one place to read and no chance of two tools disagreeing.
//
// Most of the set is what the platform actually has. Extrusion uses two small
// drawn cursors because the platform set has no grab/grabbing pair: an open
// hand says the arrow can be taken, and a closed hand confirms the captured
// drag. Paint takes the crosshair — a brush is aimed, and that is what a
// crosshair means — and the view cube takes the pointing hand.

const (
	cursorOpenHand   int32 = -1
	cursorClosedHand int32 = -2
)

func customCursor(cursor int32) bool {
	return cursor == cursorOpenHand || cursor == cursorClosedHand
}

// updateCursor picks this frame's pointer.
func (a *App) updateCursor(in InputFrame) {
	if a.Headless {
		return
	}
	want := a.wantedCursor(in)
	if want == a.cursor {
		return
	}
	wasCustom := customCursor(a.cursor)
	a.cursor = want
	if customCursor(want) {
		if !wasCustom {
			rl.HideCursor()
		}
		return
	}
	if wasCustom {
		rl.ShowCursor()
	}
	rl.SetMouseCursor(want)
}

func (a *App) wantedCursor(in InputFrame) int32 {
	if a.uvOwnsPointer(in) && !a.uvBlocked() && !a.UI.OverlayCapturesPointer(in.MouseX, in.MouseY) {
		_, canvas := a.uvRects()
		if a.uv.panning {
			return rl.MouseCursorResizeAll
		}
		if uvContains(canvas, in.MouseX, in.MouseY) {
			if a.material.open {
				return rl.MouseCursorPointingHand
			}
			return rl.MouseCursorCrosshair
		}
	}
	if a.InChamfer() && a.chamfer.gizmo.dragging {
		return rl.MouseCursorResizeAll
	}
	// A grab remains a grab if the pointer leaves the thin arrow hit area or
	// crosses the viewport edge. The captured tool owns it until release.
	if a.InExtrude() && a.extrude.tool != nil && a.extrude.tool.Dragging() {
		return cursorClosedHand
	}
	if a.pushPull.tool != nil && a.pushPull.tool.Dragging() {
		return cursorClosedHand
	}
	// The chrome is ordinary pointing, whatever the viewport is doing.
	if a.chromeOwnsPointer(in) {
		return rl.MouseCursorDefault
	}
	if a.Cube.Contains(in.MouseX, in.MouseY) {
		return rl.MouseCursorPointingHand
	}
	// Navigation, while it is happening, is a grab in every mode.
	if a.orbiting || a.panning || a.cubeDrag {
		return rl.MouseCursorResizeAll
	}

	if a.notePins.armed {
		return rl.MouseCursorCrosshair
	}
	if a.pinAt(in, render.Viewport{X: int(a.layout.Viewport.X), Y: int(a.layout.Viewport.Y), W: int(a.layout.Viewport.Width), H: int(a.layout.Viewport.Height)}) != nil {
		return rl.MouseCursorPointingHand
	}
	switch {
	case a.InChamfer():
		if a.chamfer.gizmo.hovered {
			return rl.MouseCursorResizeAll
		}
		return rl.MouseCursorPointingHand
	case a.InPaint():
		if a.material.open {
			return rl.MouseCursorPointingHand
		}
		if a.paint.awaitingLock {
			return rl.MouseCursorPointingHand
		}
		// Off a face there is nothing to paint, and the pointer says so before
		// the click does.
		if !a.paint.hover.ok {
			return rl.MouseCursorNotAllowed
		}
		if a.paint.tool == paint.ToolPick || in.Alt {
			return rl.MouseCursorPointingHand
		}
		return rl.MouseCursorCrosshair

	case a.InSketch():
		if a.sketch.session != nil && a.sketch.session.Tool != 0 {
			return rl.MouseCursorCrosshair
		}
		return rl.MouseCursorDefault

	case a.InExtrude():
		if a.extrude.hoverArrow {
			return cursorOpenHand
		}
		return rl.MouseCursorDefault

	case a.InBoolean():
		return rl.MouseCursorPointingHand
	}

	// Idle: the handles say what they are for.
	if a.pushPull.hoverArrow {
		return cursorOpenHand
	}
	if a.transformHoverCursor() != rl.MouseCursorDefault {
		return a.transformHoverCursor()
	}
	if a.notePins.armed || a.sketch.awaitingPlane || a.markers.armed {
		return rl.MouseCursorPointingHand
	}
	return rl.MouseCursorDefault
}

// drawCustomCursor draws the grab pair after all chrome so the hand stays
// legible over geometry, menus and both light and dark themes. A dark keyline
// around white fill gives it the same clarity as a native system cursor.
func (a *App) drawCustomCursor(in InputFrame) {
	if a.Headless || !customCursor(a.cursor) {
		return
	}
	scale := math.Max(1, math.Min(a.Scale, 1.5))
	ui.DrawHandCursor(in.MouseX, in.MouseY, scale, a.cursor == cursorClosedHand)
}

// transformHoverCursor is the gizmo's own cursor, or the default when nothing
// of it is under the pointer.
func (a *App) transformHoverCursor() int32 {
	t := a.transform.tool
	if t == nil || t.Hover == tools.PartNone {
		return rl.MouseCursorDefault
	}
	if t.Mode == tools.GizmoRotate {
		return rl.MouseCursorPointingHand
	}
	return rl.MouseCursorResizeAll
}
