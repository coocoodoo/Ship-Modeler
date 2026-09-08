package app

import (
	"fmt"
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/tools"
)

// Exercise the app's projection and sign handling as well as the tool math.
// A fixed pointer must give the same depth on every frame, including after
// crossing zero and after grabbing an arrow that already points inward.
func TestExtrusionDragDoesNotFightPointer(t *testing.T) {
	axes := []geom.Vec3{geom.AxisX, geom.AxisY, geom.AxisZ,
		geom.AxisX.Neg(), geom.AxisY.Neg(), geom.AxisZ.Neg(),
		(geom.Vec3{X: 1, Y: 2, Z: -1}).Normalize()}
	for _, perspective := range []bool{false, true} {
		for axisIndex, axis := range axes {
			for _, kind := range []string{"face", "extrude", "extrude-flipped"} {
				for _, snap := range []geom.SnapStep{geom.SnapGrid, geom.SnapFine, geom.SnapNone} {
					t.Run(fmt.Sprintf("%s/axis%d/perspective%t/snap%d", kind, axisIndex, perspective, snap), func(t *testing.T) {
						a := bareApp()
						a.Scale, a.Camera = 1, render.DefaultCamera()
						a.Camera.Perspective = perspective
						vp := a.Viewport(1280, 720)
						origin := geom.Vec3{X: 2, Y: 1, Z: -1}
						var initial float64
						var distance func() float64
						var dragging func() bool
						var update func(InputFrame, render.Viewport)
						if kind == "face" {
							tool := tools.NewPushPullTool(1, 1, origin, axis)
							a.pushPull.tool = tool
							distance = func() float64 { return tool.DistanceUnits }
							dragging, update = tool.Dragging, a.updatePushPull
						} else {
							tool := tools.NewExtrudeTool(1, []int{0}, origin, axis)
							if kind == "extrude-flipped" {
								tool.SetDepth(-2)
							}
							initial = tool.DepthUnits
							a.extrude.tool = tool
							distance = func() float64 { return tool.DepthUnits }
							dragging, update = tool.Dragging, a.updateExtrude
						}
						// No document geometry is needed to test pointer tracking;
						// preview builders return without allocating GPU resources.
						length := a.arrowLength(vp)
						ax, ay, bx, by, ok := scene.ArrowScreenEnds(a.Camera, vp, origin, axis, length)
						if !ok {
							t.Fatal("axis not visible")
						}
						pixels := math.Hypot(bx-ax, by-ay)
						dx, dy := (bx-ax)/pixels, (by-ay)/pixels
						grab := pixels * .6
						if initial < 0 {
							grab = -grab
						}
						x, y := ax+dx*grab, ay+dy*grab
						in := InputFrame{MouseX: x, MouseY: y, WindowW: 1280, WindowH: 720,
							Ctrl: snap == geom.SnapFine, Alt: snap == geom.SnapNone}
						in.Pressed[MouseLeft], in.Down[MouseLeft] = true, true
						update(in, vp)
						if !dragging() {
							t.Fatal("arrow did not grab")
						}
						in.Pressed[MouseLeft] = false
						for _, offset := range []float64{0, .6, 1.2, -1.2, -3.1, 0, 2.9, -4.2, -initial} {
							moved := offset * tools.ArrowScreenLength / length
							want := geom.SnapWithStep(initial+offset, snap)
							for frame := 0; frame < 8; frame++ {
								// Moving perpendicular, away from the hit region, must
								// also preserve capture and the same signed depth.
								cross := float64(frame%2) * 37
								in.MouseX, in.MouseY = x+dx*moved-dy*cross, y+dy*moved+dx*cross
								update(in, vp)
								if math.Abs(distance()-want) > 1e-8 {
									t.Fatalf("offset %g, held frame %d: depth %g, want %g", offset, frame, distance(), want)
								}
							}
						}
						in.Down[MouseLeft], in.Released[MouseLeft] = false, true
						update(in, vp)
						if dragging() {
							t.Fatal("release did not end drag")
						}
					})
				}
			}
		}
	}
}
