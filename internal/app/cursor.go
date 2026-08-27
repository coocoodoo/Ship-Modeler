package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/paint"
	"modeler/internal/tools"
)

// The cursor set of SPEC-UX §15.
//
// The cursor is the cheapest thing in the program that says what the pointer
// will do, and the only one that is right under where the user is looking. It
// is set once per frame from the mode and what is under the pointer, so there
// is one place to read and no chance of two tools disagreeing.
//
// The set is what the platform actually has. There is no pencil cursor and no
// grabbing hand in the standard set, so paint takes the crosshair — a brush is
// aimed, and that is what a crosshair means — and the view cube takes the
// pointing hand.

// updateCursor picks this frame's pointer.
func (a *App) updateCursor(in InputFrame) {
	if a.Headless {
		return
	}
	want := a.wantedCursor(in)
	if want == a.cursor {
		return
	}
	a.cursor = want
	rl.SetMouseCursor(want)
}

func (a *App) wantedCursor(in InputFrame) int32 {
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

	switch {
	case a.InPaint():
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
			return rl.MouseCursorResizeNS
		}
		return rl.MouseCursorDefault

	case a.InBoolean():
		return rl.MouseCursorPointingHand
	}

	// Idle: the handles say what they are for.
	if a.pushPull.hoverArrow {
		return rl.MouseCursorResizeNS
	}
	if a.transformHoverCursor() != rl.MouseCursorDefault {
		return a.transformHoverCursor()
	}
	if a.sketch.awaitingPlane {
		return rl.MouseCursorPointingHand
	}
	return rl.MouseCursorDefault
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
