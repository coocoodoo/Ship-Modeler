package render

import (
	"math"
	"testing"

	"modeler/internal/geom"
)

// These tests exercise the camera and animation maths only; they touch no GPU
// call, so they run without a window.

func testViewport() Viewport { return Viewport{X: 0, Y: 0, W: 1280, H: 720} }

func nearVec(t *testing.T, got, want geom.Vec3, eps float64, what string) {
	t.Helper()
	if !got.NearEq(want, eps) {
		t.Fatalf("%s = %+v, want %+v", what, got, want)
	}
}

func TestCameraBasisIsRollFree(t *testing.T) {
	for _, az := range []float64{0, 37, 90, 180, 270, 359} {
		for _, el := range []float64{-89, -45, 0, 30, 89} {
			c := Camera{Azimuth: az, Elevation: el, Dist: 10, OrthoScale: 20}
			f, r, u := c.Forward(), c.Right(), c.Up()
			for _, v := range []geom.Vec3{f, r, u} {
				if math.Abs(v.Len()-1) > 1e-9 {
					t.Fatalf("az=%v el=%v: basis vector %v is not unit", az, el, v)
				}
			}
			if math.Abs(f.Dot(r)) > 1e-9 || math.Abs(f.Dot(u)) > 1e-9 || math.Abs(r.Dot(u)) > 1e-9 {
				t.Fatalf("az=%v el=%v: basis is not orthogonal", az, el)
			}
			// No roll: screen-right always stays level with the world.
			if math.Abs(r.Y) > 1e-9 {
				t.Fatalf("az=%v el=%v: right vector has roll (Y=%v)", az, el, r.Y)
			}
			// Screen-up always points into the upper hemisphere.
			if u.Y <= 0 {
				t.Fatalf("az=%v el=%v: up vector points down (%v)", az, el, u)
			}
		}
	}
}

func TestCameraStandardViews(t *testing.T) {
	cases := []struct {
		view    StandardView
		forward geom.Vec3
	}{
		{ViewFront, geom.Vec3{Z: -1}},
		{ViewBack, geom.Vec3{Z: 1}},
		{ViewRight, geom.Vec3{X: -1}},
		{ViewLeft, geom.Vec3{X: 1}},
	}
	for _, c := range cases {
		cam := Camera{Dist: 10, OrthoScale: 20}
		cam.Azimuth, cam.Elevation = c.view.Angles()
		nearVec(t, cam.Forward(), c.forward, 1e-9, "forward")
		// The eye sits on the opposite side of the target.
		nearVec(t, cam.Eye(), c.forward.Mul(-10), 1e-9, "eye")
	}
	// Top and bottom stop just short of the pole so the up vector stays defined.
	top := Camera{Dist: 10, OrthoScale: 20}
	top.Azimuth, top.Elevation = ViewTop.Angles()
	if top.Forward().Y > -0.99 {
		t.Errorf("top view forward = %v, want nearly straight down", top.Forward())
	}
	if math.Abs(top.Up().Len()-1) > 1e-9 {
		t.Error("up vector degenerates at the top view")
	}
}

func TestParseStandardView(t *testing.T) {
	for _, name := range []string{"iso", "front", "back", "left", "right", "top", "bottom"} {
		if _, ok := ParseStandardView(name); !ok {
			t.Errorf("ParseStandardView(%q) failed", name)
		}
	}
	if _, ok := ParseStandardView("sideways"); ok {
		t.Error("ParseStandardView accepted a bogus name")
	}
}

func TestPlaneViewMatchesPlaneNormals(t *testing.T) {
	// Sketch mode animates normal-on to the plane: the camera must end up
	// looking straight down the plane's normal (SPEC-UX §8.1).
	for _, k := range []geom.PlaneKind{geom.PlaneTop, geom.PlaneFront, geom.PlaneRight} {
		cam := Camera{Dist: 10, OrthoScale: 20}
		cam.Azimuth, cam.Elevation = PlaneView(k)
		want := geom.PlaneFrame(k).N.Neg()
		got := cam.Forward()
		// The top view is clamped just off the pole, so allow a hair.
		if got.Sub(want).Len() > 0.01 {
			t.Errorf("%s: forward = %v, want %v", k, got, want)
		}
	}
}

func TestCameraNormalizeClamps(t *testing.T) {
	c := Camera{Azimuth: 720 + 45, Elevation: 200, Dist: -5, OrthoScale: 0}
	c.Normalize()
	if c.Azimuth < 0 || c.Azimuth >= 360 {
		t.Errorf("azimuth = %v, want [0,360)", c.Azimuth)
	}
	if math.Abs(c.Azimuth-45) > 1e-9 {
		t.Errorf("azimuth = %v, want 45 after wrapping", c.Azimuth)
	}
	if c.Elevation != ElevationLimit {
		t.Errorf("elevation = %v, want clamped to %v", c.Elevation, ElevationLimit)
	}
	if c.Dist < MinDist || c.OrthoScale < MinOrthoScale {
		t.Errorf("distance/scale not clamped: %v %v", c.Dist, c.OrthoScale)
	}
}

func TestOrbitIsLevelAndClamped(t *testing.T) {
	c := DefaultCamera()
	before := c.Azimuth
	c.Orbit(100, 0)
	if math.Abs((before-c.Azimuth)-100*OrbitDegPerPixel) > 1e-9 {
		t.Errorf("orbit did not apply %v deg per pixel", OrbitDegPerPixel)
	}
	// Dragging far past the pole stops at the limit rather than flipping over.
	c.Orbit(0, 10000)
	if c.Elevation != ElevationLimit {
		t.Errorf("elevation = %v, want %v", c.Elevation, ElevationLimit)
	}
	c.Orbit(0, -100000)
	if c.Elevation != -ElevationLimit {
		t.Errorf("elevation = %v, want %v", c.Elevation, -ElevationLimit)
	}
}

// TestZoomToCursorPinsThePointUnderTheCursor is the behaviour SPEC-RENDER §7
// asks for: after zooming, the world point that was under the cursor is still
// under it.
func TestZoomToCursorPinsThePointUnderTheCursor(t *testing.T) {
	vp := testViewport()
	for _, persp := range []bool{false, true} {
		for _, notches := range []float64{1, -1, 3, -2.5} {
			c := DefaultCamera()
			c.Perspective = persp
			cursor := geom.Vec2{X: 900, Y: 200}

			origin, dir := c.Ray(cursor, float64(vp.W), float64(vp.H))
			// A point on the ray at the target's depth: that is the point the
			// user believes is under the cursor.
			depth := c.Target.Sub(origin).Dot(c.Forward())
			pinned := origin.Add(dir.Mul(depth / dir.Dot(c.Forward())))

			c.ZoomToCursor(notches, cursor, float64(vp.W), float64(vp.H))

			after, ok := c.WorldToViewport(pinned, float64(vp.W), float64(vp.H))
			if !ok {
				t.Fatalf("persp=%v notches=%v: pinned point fell behind the camera", persp, notches)
			}
			if math.Abs(after.X-cursor.X) > 0.5 || math.Abs(after.Y-cursor.Y) > 0.5 {
				t.Errorf("persp=%v notches=%v: point moved from %v to %v",
					persp, notches, cursor, after)
			}
		}
	}
}

func TestZoomChangesScaleNotTarget(t *testing.T) {
	c := DefaultCamera()
	before := c.OrthoScale
	target := c.Target
	c.Zoom(1)
	if c.OrthoScale >= before {
		t.Error("one notch forward should zoom in")
	}
	if !c.Target.NearEq(target, 0) {
		t.Error("plain Zoom moved the target")
	}
	c.Zoom(-1)
	if math.Abs(c.OrthoScale-before) > 1e-9 {
		t.Errorf("zoom in then out did not return to %v (got %v)", before, c.OrthoScale)
	}
}

func TestPanMovesWithTheCursor(t *testing.T) {
	vp := testViewport()
	c := DefaultCamera()
	before := c.Target
	c.Pan(100, 0, float64(vp.W), float64(vp.H))
	moved := c.Target.Sub(before)
	// Dragging right moves the model right, so the target moves left.
	if moved.Dot(c.Right()) >= 0 {
		t.Errorf("panning right moved the target the wrong way: %v", moved)
	}
	// Panning does not rotate or zoom.
	if c.Azimuth != DefaultCamera().Azimuth || c.OrthoScale != DefaultCamera().OrthoScale {
		t.Error("panning changed orientation or scale")
	}
}

func TestFrameBoxFitsTheContent(t *testing.T) {
	vp := testViewport()
	box := geom.AABB{Min: geom.Vec3{X: -6, Y: -2, Z: -3}, Max: geom.Vec3{X: 6, Y: 2, Z: 3}}
	for _, persp := range []bool{false, true} {
		c := DefaultCamera()
		c.Perspective = persp
		c.FrameBox(box, vp.Aspect())
		if !c.Target.NearEq(box.Center(), 1e-9) {
			t.Errorf("persp=%v: target = %v, want the box centre %v", persp, c.Target, box.Center())
		}
		// Every corner must project inside the viewport.
		for _, corner := range box.Corners() {
			p, ok := c.WorldToViewport(corner, float64(vp.W), float64(vp.H))
			if !ok {
				t.Fatalf("persp=%v: corner %v is behind the camera", persp, corner)
			}
			if p.X < 0 || p.X > float64(vp.W) || p.Y < 0 || p.Y > float64(vp.H) {
				t.Errorf("persp=%v: corner %v projects off screen at %v", persp, corner, p)
			}
		}
	}
	// An invalid box leaves the camera alone rather than producing NaNs.
	c := DefaultCamera()
	c.FrameBox(geom.Empty(), vp.Aspect())
	if c != DefaultCamera() {
		t.Error("framing an empty box moved the camera")
	}
}

func TestWorldToViewportRoundTripsWithRay(t *testing.T) {
	vp := testViewport()
	for _, persp := range []bool{false, true} {
		c := DefaultCamera()
		c.Perspective = persp
		for _, px := range []geom.Vec2{{X: 10, Y: 10}, {X: 640, Y: 360}, {X: 1270, Y: 700}} {
			origin, dir := c.Ray(px, float64(vp.W), float64(vp.H))
			p := origin.Add(dir.Mul(c.Dist))
			back, ok := c.WorldToViewport(p, float64(vp.W), float64(vp.H))
			if !ok {
				t.Fatalf("persp=%v: point from %v projected behind the camera", persp, px)
			}
			if math.Abs(back.X-px.X) > 0.01 || math.Abs(back.Y-px.Y) > 0.01 {
				t.Errorf("persp=%v: %v round tripped to %v", persp, px, back)
			}
		}
	}
}

func TestLookAlongMatchesTheDirection(t *testing.T) {
	dirs := []geom.Vec3{
		{X: 1}, {X: -1}, {Z: 1}, {Z: -1},
		{X: 1, Y: 1, Z: 1}, {X: -1, Y: 0.5, Z: 2},
	}
	for _, d := range dirs {
		c := DefaultCamera()
		c.LookAlong(d)
		want := d.Normalize().Neg()
		if c.Forward().Sub(want).Len() > 1e-9 {
			t.Errorf("LookAlong(%v): forward = %v, want %v", d, c.Forward(), want)
		}
	}
	// A degenerate direction is ignored rather than producing NaNs.
	c := DefaultCamera()
	c.LookAlong(geom.Vec3{})
	if c != DefaultCamera() {
		t.Error("LookAlong on a zero vector changed the camera")
	}
}

func TestPickMatrixCentresTheCursor(t *testing.T) {
	vp := testViewport()
	cam := DefaultCamera()
	cursor := geom.Vec2{X: 400, Y: 500}
	proj := pickMatrix(cursor, vp, PickRegionPx).Mul(cam.Proj(vp.Aspect()))
	clip := proj.Mul(cam.View())

	// The world point under the cursor must land at the centre of the pick
	// projection, which is what makes the readback's centre meaningful.
	origin, dir := cam.Ray(cursor, float64(vp.W), float64(vp.H))
	p := origin.Add(dir.Mul(cam.Dist))
	x, y, _, w := clip.TransformVec4(p, 1)
	if math.Abs(x/w) > 1e-6 || math.Abs(y/w) > 1e-6 {
		t.Fatalf("cursor point maps to NDC (%v, %v), want the origin", x/w, y/w)
	}

	// A point PickRegionPx/2 pixels away must land on the edge of the region.
	edge := geom.Vec2{X: cursor.X + PickRegionPx/2, Y: cursor.Y}
	o2, d2 := cam.Ray(edge, float64(vp.W), float64(vp.H))
	p2 := o2.Add(d2.Mul(cam.Dist))
	x2, _, _, w2 := clip.TransformVec4(p2, 1)
	if math.Abs(x2/w2-1) > 1e-6 {
		t.Fatalf("region edge maps to NDC x = %v, want 1", x2/w2)
	}
}

func TestCameraAnimEases(t *testing.T) {
	from := DefaultCamera()
	to := from
	to.Azimuth, to.Elevation = ViewFront.Angles()

	var anim CameraAnim
	anim.Start(from, to)
	if !anim.Active() {
		t.Fatal("animation did not start")
	}

	const frameMs = 1000.0 / 60.0
	cam := from
	steps := 0
	for anim.Active() && steps < 1000 {
		cam, _ = anim.Step(cam, frameMs)
		steps++
	}
	wantFrames := int(math.Ceil(CameraAnimMillis/frameMs)) + 1
	if steps == 0 || steps > wantFrames {
		t.Fatalf("animation took %d frames, want at most %d", steps, wantFrames)
	}
	if math.Abs(cam.Azimuth-to.Azimuth) > 1e-9 || math.Abs(cam.Elevation-to.Elevation) > 1e-9 {
		t.Errorf("animation ended at az=%v el=%v, want az=%v el=%v",
			cam.Azimuth, cam.Elevation, to.Azimuth, to.Elevation)
	}
	if anim.Active() {
		t.Error("animation is still active after reaching its target")
	}
}

func TestCameraAnimTakesTheShorterArc(t *testing.T) {
	from := DefaultCamera()
	from.Azimuth = 350
	to := from
	to.Azimuth = 10

	var anim CameraAnim
	anim.Start(from, to)
	cam, _ := anim.Step(from, CameraAnimMillis/2)
	// Halfway round the short arc is 0 degrees, not 180.
	if d := math.Abs(shortestAngleDelta(cam.Azimuth, 0)); d > 5 {
		t.Errorf("midpoint azimuth = %v, want near 0 (took the long way round)", cam.Azimuth)
	}
}

func TestCameraAnimCancelKeepsCurrentState(t *testing.T) {
	from := DefaultCamera()
	to := from
	to.Azimuth = 180

	var anim CameraAnim
	anim.Start(from, to)
	cam, _ := anim.Step(from, CameraAnimMillis/2)
	anim.Cancel()
	if anim.Active() {
		t.Fatal("cancel did not stop the animation")
	}
	after, changed := anim.Step(cam, 16)
	if changed || after != cam {
		t.Error("a cancelled animation still moved the camera")
	}
}

func TestCameraAnimNoOpWhenAlreadyThere(t *testing.T) {
	c := DefaultCamera()
	var anim CameraAnim
	anim.Start(c, c)
	if anim.Active() {
		t.Error("animating to the current state should be a no-op")
	}
}

func TestSmoothedConverges(t *testing.T) {
	s := NewSmoothed(0, 120)
	s.Aim(10)
	moving := true
	for i := 0; i < 500 && moving; i++ {
		moving = s.Step(1000.0 / 60.0)
	}
	if math.Abs(s.Value-10) > 1e-6 {
		t.Errorf("smoothed value = %v, want 10", s.Value)
	}
	if s.Target() != 10 {
		t.Errorf("target = %v, want 10", s.Target())
	}
	s.Set(3)
	if s.Value != 3 || s.Target() != 3 {
		t.Error("Set did not jump both value and target")
	}
}

func TestEaseInOutCubic(t *testing.T) {
	if got := easeInOutCubic(0); got != 0 {
		t.Errorf("ease(0) = %v, want 0", got)
	}
	if got := easeInOutCubic(1); math.Abs(got-1) > 1e-12 {
		t.Errorf("ease(1) = %v, want 1", got)
	}
	if got := easeInOutCubic(0.5); math.Abs(got-0.5) > 1e-12 {
		t.Errorf("ease(0.5) = %v, want 0.5", got)
	}
	// Monotonic.
	prev := -1.0
	for i := 0; i <= 100; i++ {
		v := easeInOutCubic(float64(i) / 100)
		if v < prev {
			t.Fatalf("ease is not monotonic at t=%v", float64(i)/100)
		}
		prev = v
	}
}

func TestViewportHelpers(t *testing.T) {
	vp := Viewport{X: 240, Y: 40, W: 1000, H: 600}
	if got := vp.Aspect(); math.Abs(got-1000.0/600.0) > 1e-12 {
		t.Errorf("aspect = %v", got)
	}
	if !vp.Contains(240, 40) || !vp.Contains(1239, 639) {
		t.Error("Contains rejects points inside the viewport")
	}
	if vp.Contains(239, 40) || vp.Contains(1240, 40) || vp.Contains(240, 640) {
		t.Error("Contains accepts points outside the viewport")
	}
	l := vp.Local(250, 60)
	if l.X != 10 || l.Y != 20 {
		t.Errorf("Local = %v, want {10 20}", l)
	}
	if (Viewport{}).Aspect() != 1 {
		t.Error("a collapsed viewport should report aspect 1 rather than dividing by zero")
	}
}

func TestPickTable(t *testing.T) {
	var tbl PickTable
	if _, ok := tbl.Get(0); ok {
		t.Error("id 0 must never resolve: a cleared pick buffer means nothing")
	}
	first := tbl.Add(PickRef{Kind: PickFace, BodyID: 7})
	if first != 1 {
		t.Errorf("first id = %d, want 1", first)
	}
	if tbl.Next() != 2 {
		t.Errorf("Next = %d, want 2", tbl.Next())
	}
	second := tbl.Add(PickRef{Kind: PickEdge, BodyID: 9, Edge: 3})
	ref, ok := tbl.Get(second)
	if !ok || ref.Kind != PickEdge || ref.Edge != 3 || ref.BodyID != 9 {
		t.Errorf("Get(%d) = %+v, ok=%v", second, ref, ok)
	}
	if _, ok := tbl.Get(99); ok {
		t.Error("an out-of-range id resolved")
	}
	tbl.Reset()
	if tbl.Len() != 0 || tbl.Next() != 1 {
		t.Error("Reset did not empty the table")
	}
}

func TestIDEncodingRoundTrips(t *testing.T) {
	for _, id := range []int{1, 2, 255, 256, 257, 65535, 65536, 1 << 20} {
		if got := decodeID(encodeID(id)); got != id {
			t.Errorf("id %d round tripped to %d", id, got)
		}
	}
	// The cleared buffer (opaque black) must decode to "nothing".
	if got := decodeID(encodeID(0)); got != 0 {
		t.Errorf("cleared pixel decoded to %d", got)
	}
}

func TestMatrixConversionMatchesRaylibLayout(t *testing.T) {
	// raylib's Matrix declares its fields row by row, so field M{c*4+r} holds
	// our row-major element [r*4+c].
	m := geom.Mat4{
		0, 1, 2, 3,
		4, 5, 6, 7,
		8, 9, 10, 11,
		12, 13, 14, 15,
	}
	r := toRLMatrix(m)
	checks := []struct {
		got  float32
		want float64
	}{
		{r.M0, m[0]}, {r.M4, m[1]}, {r.M8, m[2]}, {r.M12, m[3]},
		{r.M1, m[4]}, {r.M5, m[5]}, {r.M9, m[6]}, {r.M13, m[7]},
		{r.M2, m[8]}, {r.M6, m[9]}, {r.M10, m[10]}, {r.M14, m[11]},
		{r.M3, m[12]}, {r.M7, m[13]}, {r.M11, m[14]}, {r.M15, m[15]},
	}
	for i, c := range checks {
		if float64(c.got) != c.want {
			t.Errorf("field %d = %v, want %v", i, c.got, c.want)
		}
	}
}
