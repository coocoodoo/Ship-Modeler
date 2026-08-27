package scene

import (
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// These tests cover the cube's zone maths and the plane draw list. They call no
// GPU function, so they run without a window.

// allZones enumerates the 26 hit zones: every combination of -1, 0 and 1 except
// the all-zero centre.
func allZones() []CubeZone {
	var out []CubeZone
	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			for z := -1; z <= 1; z++ {
				z := CubeZone{X: x, Y: y, Z: z}
				if z.Valid() {
					out = append(out, z)
				}
			}
		}
	}
	return out
}

func TestCubeHasTwentySixZones(t *testing.T) {
	zones := allZones()
	if len(zones) != 26 {
		t.Fatalf("enumerated %d zones, want 26", len(zones))
	}
	faces, edges, corners := 0, 0, 0
	for _, z := range zones {
		switch abs(z.X) + abs(z.Y) + abs(z.Z) {
		case 1:
			faces++
		case 2:
			edges++
		case 3:
			corners++
		}
	}
	if faces != 6 || edges != 12 || corners != 8 {
		t.Errorf("got %d faces, %d edges, %d corners; want 6/12/8", faces, edges, corners)
	}
	if (CubeZone{}).Valid() {
		t.Error("the centre is not a zone")
	}
}

func TestCubeFaceLabelsMatchTheCameraViews(t *testing.T) {
	// Clicking a labelled face must land on the matching standard view: the
	// cube is the user's map of the camera (SPEC-UX §6.1).
	cases := []struct {
		zone  CubeZone
		label string
		view  render.StandardView
	}{
		{CubeZone{Z: 1}, "FRONT", render.ViewFront},
		{CubeZone{Z: -1}, "BACK", render.ViewBack},
		{CubeZone{X: 1}, "RIGHT", render.ViewRight},
		{CubeZone{X: -1}, "LEFT", render.ViewLeft},
		{CubeZone{Y: 1}, "TOP", render.ViewTop},
		{CubeZone{Y: -1}, "BOTTOM", render.ViewBottom},
	}
	for _, c := range cases {
		if got := c.zone.Label(); got != c.label {
			t.Errorf("%v: label = %q, want %q", c.zone, got, c.label)
		}
		if !c.zone.IsFace() {
			t.Errorf("%v should be a face zone", c.zone)
		}

		// The zone direction is where the camera sits; LookAlong turns that into
		// the same orientation the standard view uses.
		cam := render.DefaultCamera()
		cam.LookAlong(c.zone.Direction())

		want := render.DefaultCamera()
		want.Azimuth, want.Elevation = c.view.Angles()
		want.Normalize()

		if math.Abs(cam.Forward().Sub(want.Forward()).Len()) > 0.02 {
			t.Errorf("%s: cube view forward %v does not match the %v view %v",
				c.label, cam.Forward(), c.view, want.Forward())
		}
	}
}

func TestCubeEdgeAndCornerDirections(t *testing.T) {
	for _, z := range allZones() {
		d := z.Direction()
		if math.Abs(d.Len()-1) > 1e-9 {
			t.Errorf("%v: direction %v is not unit", z, d)
		}
		if z.IsFace() && z.Label() == "" {
			t.Errorf("%v is a face but has no label", z)
		}
		if !z.IsFace() && z.Label() != "" {
			t.Errorf("%v is not a face but carries the label %q", z, z.Label())
		}
	}
	// A corner points equally along all three axes.
	corner := CubeZone{X: 1, Y: 1, Z: 1}.Direction()
	if math.Abs(corner.X-corner.Y) > 1e-9 || math.Abs(corner.Y-corner.Z) > 1e-9 {
		t.Errorf("corner direction %v is not symmetric", corner)
	}
}

func TestCubeLayoutSitsTopRight(t *testing.T) {
	var c ViewCube
	vp := render.Viewport{X: 240, Y: 40, W: 1000, H: 600}
	c.Layout(render.DefaultCamera(), vp, 1)

	if c.Rect.Width != ui.ViewCubeSize || c.Rect.Height != ui.ViewCubeSize {
		t.Errorf("cube is %vx%v, want %d square", c.Rect.Width, c.Rect.Height, ui.ViewCubeSize)
	}
	right := float32(vp.X+vp.W) - c.Rect.X - c.Rect.Width
	if right != ui.ViewCubeMargin {
		t.Errorf("right margin = %v, want %d", right, ui.ViewCubeMargin)
	}
	if c.Rect.Y-float32(vp.Y) != ui.ViewCubeMargin {
		t.Errorf("top margin = %v, want %d", c.Rect.Y-float32(vp.Y), ui.ViewCubeMargin)
	}
	// The home button sits below the cube and is part of the widget.
	if c.HomeRect.Y <= c.Rect.Y+c.Rect.Height {
		t.Error("home button overlaps the cube")
	}
	if !c.Contains(float64(c.HomeRect.X+2), float64(c.HomeRect.Y+2)) {
		t.Error("Contains misses the home button")
	}
	if c.Contains(float64(vp.X+10), float64(vp.Y+10)) {
		t.Error("Contains claims the far side of the viewport")
	}
}

// TestCubeHitTestMatchesWhatIsDrawn walks the drawn sub-quads and checks that
// hit-testing their centres returns the zone they represent.
func TestCubeHitTestMatchesWhatIsDrawn(t *testing.T) {
	var c ViewCube
	vp := render.Viewport{X: 0, Y: 0, W: 1280, H: 694}
	c.Layout(render.DefaultCamera(), vp, 1)

	if len(c.quads) != 27 {
		t.Fatalf("iso view drew %d sub-quads, want 27 (three visible faces of nine)", len(c.quads))
	}
	seen := map[CubeZone]bool{}
	for _, q := range c.quads {
		cx := float64(q.pts[0].X+q.pts[2].X) / 2
		cy := float64(q.pts[0].Y+q.pts[2].Y) / 2
		got, home := c.HitTest(cx, cy)
		if home {
			t.Errorf("centre of %v hit the home button", q.zone)
			continue
		}
		if got != q.zone {
			t.Errorf("centre of %v hit %v instead", q.zone, got)
		}
		seen[q.zone] = true
	}
	// The 27 cells cover 19 distinct zones, because an edge or corner zone is
	// drawn on every visible face that touches it: the corner nearest the eye
	// appears as a cell on all three faces, and each near edge on two. Hitting
	// any of those cells must give the same zone, which the loop above checked.
	if len(seen) != 19 {
		t.Errorf("hit %d distinct zones, want 19", len(seen))
	}
	shared := 0
	for _, z := range []CubeZone{{X: 1, Y: 1, Z: 1}, {X: 1, Z: 1}, {X: 1, Y: 1}, {Y: 1, Z: 1}} {
		if seen[z] {
			shared++
		}
	}
	if shared != 4 {
		t.Errorf("the near corner and its three edges are not all reachable (%d of 4)", shared)
	}
	// Outside the cube nothing is hit.
	if z, home := c.HitTest(10, 10); z.Valid() || home {
		t.Errorf("a click far from the cube hit %v (home=%v)", z, home)
	}
}

func TestCubeShowsOneFaceHeadOn(t *testing.T) {
	var c ViewCube
	vp := render.Viewport{X: 0, Y: 0, W: 1280, H: 694}
	cam := render.DefaultCamera()
	cam.Azimuth, cam.Elevation = render.ViewFront.Angles()
	c.Layout(cam, vp, 1)

	if len(c.quads) != 9 {
		t.Fatalf("front view drew %d sub-quads, want 9 (one face)", len(c.quads))
	}
	for _, q := range c.quads {
		if q.face != (CubeZone{Z: 1}) {
			t.Errorf("front view shows a sub-quad of face %v", q.face)
		}
	}
}

func TestCubeUpdateTracksHover(t *testing.T) {
	var c ViewCube
	vp := render.Viewport{X: 0, Y: 0, W: 1280, H: 694}
	c.Layout(render.DefaultCamera(), vp, 1)

	q := c.quads[0]
	cx := float64(q.pts[0].X+q.pts[2].X) / 2
	cy := float64(q.pts[0].Y+q.pts[2].Y) / 2
	c.Update(cx, cy)
	if c.Hover != q.zone {
		t.Errorf("hover = %v, want %v", c.Hover, q.zone)
	}
	c.Update(float64(c.HomeRect.X+3), float64(c.HomeRect.Y+3))
	if !c.HoverHome {
		t.Error("hovering the home button did not register")
	}
	c.Update(5, 5)
	if c.Hover.Valid() || c.HoverHome {
		t.Error("hover survived moving away from the widget")
	}
}

func TestPlaneDrawsFollowVisibility(t *testing.T) {
	vis := AllVisible()
	draws := BuildPlaneDraws(vis, 0, 0, false, false)
	if len(draws) != geom.PlaneCount {
		t.Fatalf("%d plane draws, want %d", len(draws), geom.PlaneCount)
	}
	for _, d := range draws {
		if !d.Pickable {
			t.Errorf("%s is not pickable", d.Kind)
		}
		if d.HalfSize != PlaneHalfSize {
			t.Errorf("%s half size = %v, want %v", d.Kind, d.HalfSize, PlaneHalfSize)
		}
		if d.Color.A != PlaneTintAlpha {
			t.Errorf("%s fill alpha = %d, want %d", d.Kind, d.Color.A, PlaneTintAlpha)
		}
		if d.Label != d.Kind.String() {
			t.Errorf("%s label = %q", d.Kind, d.Label)
		}
		if !d.Frame.IsRightHanded() {
			t.Errorf("%s frame is not right-handed", d.Kind)
		}
	}

	vis[geom.PlaneFront] = false
	draws = BuildPlaneDraws(vis, 0, 0, false, false)
	if len(draws) != geom.PlaneCount-1 {
		t.Fatalf("hiding a plane left %d draws", len(draws))
	}
	for _, d := range draws {
		if d.Kind == geom.PlaneFront {
			t.Error("a hidden plane is still drawn")
		}
	}
}

func TestPlaneHoverAndSelection(t *testing.T) {
	draws := BuildPlaneDraws(AllVisible(), geom.PlaneRight, geom.PlaneTop, true, true)
	for _, d := range draws {
		if (d.Kind == geom.PlaneRight) != d.Hovered {
			t.Errorf("%s hovered = %v", d.Kind, d.Hovered)
		}
		if (d.Kind == geom.PlaneTop) != d.Selected {
			t.Errorf("%s selected = %v", d.Kind, d.Selected)
		}
	}
}

func TestPlaneColorsFollowTheirAxis(t *testing.T) {
	if PlaneColor(geom.PlaneTop) != ui.ColorAxisY {
		t.Error("the Top plane should carry the Y axis colour")
	}
	if PlaneColor(geom.PlaneFront) != ui.ColorAxisZ {
		t.Error("the Front plane should carry the Z axis colour")
	}
	if PlaneColor(geom.PlaneRight) != ui.ColorAxisX {
		t.Error("the Right plane should carry the X axis colour")
	}
}

func TestSketchGridSteps(t *testing.T) {
	g := SketchGrid(geom.PlaneFrame(geom.PlaneFront), 24, 1, 1)
	if g.MinorStep != 1 || g.MajorStep != 8 {
		t.Errorf("grid steps = %v/%v, want 1/8", g.MinorStep, g.MajorStep)
	}
	// The step scales both lines together, keeping the every-eighth rhythm.
	if g := SketchGrid(geom.PlaneFrame(geom.PlaneFront), 24, 1, 0.5); g.MinorStep != 0.5 || g.MajorStep != 4 {
		t.Errorf("half grid steps = %v/%v, want 0.5/4", g.MinorStep, g.MajorStep)
	}
	// A broken step falls back to the unit grid rather than to no lines.
	if g := SketchGrid(geom.PlaneFrame(geom.PlaneFront), 24, 1, 0); g.MinorStep != 1 {
		t.Errorf("zero step drew at %v, want the 1 u fallback", g.MinorStep)
	}
	if !g.ShowAxes {
		t.Error("the sketch grid should show its origin axes")
	}
	// The Front plane's U axis is world X and its V axis is world Y.
	if g.AxisUColor.R != ui.ColorAxisX.R || g.AxisVColor.G != ui.ColorAxisY.G {
		t.Errorf("axis tints = %v / %v", g.AxisUColor, g.AxisVColor)
	}
}

func TestFrameAllFallsBackToThePlanes(t *testing.T) {
	empty := &render.Scene{}
	b := FrameAll(empty)
	if !b.Valid() {
		t.Fatal("an empty document has no framing box")
	}
	if b.LongestSide() != 2*PlaneHalfSize {
		t.Errorf("empty framing box spans %v, want the plane extent %v",
			b.LongestSide(), 2*PlaneHalfSize)
	}
}
