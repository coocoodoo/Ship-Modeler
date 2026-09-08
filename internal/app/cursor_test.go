package app

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/tools"
)

func TestExtrusionUsesGrabCursors(t *testing.T) {
	a := bareApp()
	a.Scale = 1
	a.layout = a.Layout(1280, 720)
	in := InputFrame{MouseX: 640, MouseY: 360, WindowW: 1280, WindowH: 720}

	a.Mode = ModeExtrude
	a.extrude.tool = tools.NewExtrudeTool(1, []int{0}, geom.Vec3{}, geom.AxisY)
	a.extrude.hoverArrow = true
	if got := a.wantedCursor(in); got != cursorOpenHand {
		t.Fatalf("extrude arrow cursor = %d, want open hand", got)
	}
	a.extrude.tool.BeginDrag(0)
	a.extrude.hoverArrow = false
	if got := a.wantedCursor(in); got != cursorClosedHand {
		t.Fatalf("extrude drag cursor = %d, want closed hand", got)
	}

	a.Mode = ModeIdle
	a.extrude.tool = nil
	a.pushPull.tool = tools.NewPushPullTool(1, 1, geom.Vec3{}, geom.AxisX)
	a.pushPull.hoverArrow = true
	if got := a.wantedCursor(in); got != cursorOpenHand {
		t.Fatalf("face arrow cursor = %d, want open hand", got)
	}
	a.pushPull.tool.BeginDrag(0)
	a.pushPull.hoverArrow = false
	if got := a.wantedCursor(in); got != cursorClosedHand {
		t.Fatalf("face drag cursor = %d, want closed hand", got)
	}

	a.pushPull.tool.EndDrag()
	if got := a.wantedCursor(in); got != rl.MouseCursorDefault {
		t.Fatalf("released cursor = %d, want default", got)
	}
}
