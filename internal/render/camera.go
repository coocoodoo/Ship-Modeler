package render

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
)

// Camera is the turntable camera of SPEC-RENDER §7: world-up locked to +Y,
// no roll ever, orthographic by default (D-08).
type Camera struct {
	Target      geom.Vec3
	Azimuth     float64 // degrees, rotation about world +Y
	Elevation   float64 // degrees, clamped to +-ElevationLimit
	Dist        float64 // eye distance from target
	OrthoScale  float64 // world units spanned by the viewport height
	Perspective bool
}

// Camera limits and defaults.
const (
	ElevationLimit = 89.5
	MinDist        = 0.05
	MaxDist        = 100000
	MinOrthoScale  = 0.05
	MaxOrthoScale  = 100000
	PerspectiveFOV = 45.0 // degrees

	// IsoAzimuth and IsoElevation are the home view (SPEC-UX §6.1).
	IsoAzimuth   = 45.0
	IsoElevation = 30.0

	// OrbitDegPerPixel keeps orbiting precise rather than flingy.
	OrbitDegPerPixel = 0.3

	// FrameMargin is the padding fraction used when framing a bounding box.
	FrameMargin = 0.15
)

// DefaultCamera returns the isometric home view framing a 24 u scene.
func DefaultCamera() Camera {
	return Camera{
		Azimuth:    IsoAzimuth,
		Elevation:  IsoElevation,
		Dist:       40,
		OrthoScale: 24,
	}
}

// Forward is the unit vector from the eye toward the target.
func (c Camera) Forward() geom.Vec3 {
	az := c.Azimuth * math.Pi / 180
	el := c.Elevation * math.Pi / 180
	// Eye sits at azimuth/elevation on a sphere around the target; forward is
	// the negation of that offset direction.
	return geom.Vec3{
		X: -math.Cos(el) * math.Sin(az),
		Y: -math.Sin(el),
		Z: -math.Cos(el) * math.Cos(az),
	}
}

// Eye returns the camera position.
func (c Camera) Eye() geom.Vec3 {
	return c.Target.Sub(c.Forward().Mul(c.Dist))
}

// Right is the camera's screen-right unit vector.
func (c Camera) Right() geom.Vec3 {
	r, ok := c.Forward().Cross(geom.AxisY).NormalizeOK()
	if !ok {
		// Looking straight up or down: fall back to azimuth alone so panning
		// stays sane at the elevation limits.
		az := c.Azimuth * math.Pi / 180
		return geom.Vec3{X: math.Cos(az), Y: 0, Z: -math.Sin(az)}
	}
	return r
}

// Up is the camera's screen-up unit vector; always roll-free.
func (c Camera) Up() geom.Vec3 {
	return c.Right().Cross(c.Forward()).Normalize()
}

// View returns the view matrix.
func (c Camera) View() geom.Mat4 {
	return geom.LookAt(c.Eye(), c.Target, geom.AxisY)
}

// NearFar returns depth planes sized to the current distance, wide enough for
// big scenes and tight enough to keep depth precision usable.
func (c Camera) NearFar() (near, far float64) {
	span := math.Max(c.Dist, c.OrthoScale) * 4
	if c.Perspective {
		return math.Max(0.01, c.Dist/1000), span + 1000
	}
	return -span - 1000, span + 1000
}

// Proj returns the projection matrix for a viewport aspect ratio (width/height).
func (c Camera) Proj(aspect float64) geom.Mat4 {
	near, far := c.NearFar()
	if c.Perspective {
		return geom.Perspective(PerspectiveFOV*math.Pi/180, aspect, near, far)
	}
	h := c.OrthoScale / 2
	w := h * aspect
	return geom.Ortho(-w, w, -h, h, near, far)
}

// ViewProj is the combined transform from world space to clip space.
func (c Camera) ViewProj(aspect float64) geom.Mat4 {
	return c.Proj(aspect).Mul(c.View())
}

// RL converts to the raylib camera used by BeginMode3D. Orthographic mode takes
// Fovy as the viewport height in world units, which is exactly OrthoScale.
func (c Camera) RL() rl.Camera3D {
	cam := rl.Camera3D{
		Position: toRLVec3(c.Eye()),
		Target:   toRLVec3(c.Target),
		Up:       toRLVec3(geom.AxisY),
	}
	if c.Perspective {
		cam.Projection = rl.CameraPerspective
		cam.Fovy = PerspectiveFOV
	} else {
		cam.Projection = rl.CameraOrthographic
		cam.Fovy = float32(c.OrthoScale)
	}
	return cam
}

// Normalize clamps the camera back into its legal range. Every mutator ends
// with this, so no code path can produce a broken view.
func (c *Camera) Normalize() {
	c.Elevation = clamp(c.Elevation, -ElevationLimit, ElevationLimit)
	c.Azimuth = math.Mod(c.Azimuth, 360)
	if c.Azimuth < 0 {
		c.Azimuth += 360
	}
	c.Dist = clamp(c.Dist, MinDist, MaxDist)
	c.OrthoScale = clamp(c.OrthoScale, MinOrthoScale, MaxOrthoScale)
}

// Orbit rotates the camera by a mouse delta in pixels.
func (c *Camera) Orbit(dxPixels, dyPixels float64) {
	c.Azimuth -= dxPixels * OrbitDegPerPixel
	c.Elevation += dyPixels * OrbitDegPerPixel
	c.Normalize()
}

// Pan slides the target so the world point under the cursor tracks the mouse.
func (c *Camera) Pan(dxPixels, dyPixels float64, viewW, viewH float64) {
	if viewH <= 0 {
		return
	}
	worldPerPixel := c.worldHeight() / viewH
	delta := c.Right().Mul(-dxPixels * worldPerPixel).
		Add(c.Up().Mul(dyPixels * worldPerPixel))
	c.Target = c.Target.Add(delta)
}

// worldHeight is how many world units the viewport height spans at the target
// depth, which is what makes panning and zooming feel identical in both
// projections.
func (c Camera) worldHeight() float64 {
	if c.Perspective {
		return 2 * c.Dist * math.Tan(PerspectiveFOV*math.Pi/360)
	}
	return c.OrthoScale
}

// ZoomStep is the multiplicative zoom applied per wheel notch.
const ZoomStep = 1.12

// Zoom scales the view by wheel notches without moving the target.
func (c *Camera) Zoom(notches float64) {
	f := math.Pow(1/ZoomStep, notches)
	if c.Perspective {
		c.Dist *= f
	} else {
		c.OrthoScale *= f
	}
	c.Normalize()
}

// ZoomToCursor zooms while keeping the world point under the cursor pinned
// there (SPEC-RENDER §7). cursor is in viewport pixels with the origin at the
// top-left of the viewport.
func (c *Camera) ZoomToCursor(notches float64, cursor geom.Vec2, viewW, viewH float64) {
	if viewW <= 0 || viewH <= 0 || notches == 0 {
		return
	}
	// Offset of the cursor from the viewport centre, in world units on the
	// target plane, before the zoom.
	before := c.cursorOffset(cursor, viewW, viewH)
	c.Zoom(notches)
	after := c.cursorOffset(cursor, viewW, viewH)
	// Shift the target by the difference so the same world point stays put.
	c.Target = c.Target.Add(before.Sub(after))
}

// cursorOffset converts a viewport pixel into a world offset from the target,
// measured on the plane through the target facing the camera.
func (c Camera) cursorOffset(cursor geom.Vec2, viewW, viewH float64) geom.Vec3 {
	worldPerPixel := c.worldHeight() / viewH
	dx := (cursor.X - viewW/2) * worldPerPixel
	dy := (cursor.Y - viewH/2) * worldPerPixel
	return c.Right().Mul(dx).Add(c.Up().Mul(-dy))
}

// Ray returns the world-space ray under a viewport pixel.
func (c Camera) Ray(cursor geom.Vec2, viewW, viewH float64) (origin, dir geom.Vec3) {
	offset := c.cursorOffset(cursor, viewW, viewH)
	if c.Perspective {
		eye := c.Eye()
		point := c.Target.Add(offset)
		return eye, point.Sub(eye).Normalize()
	}
	return c.Eye().Add(offset), c.Forward()
}

// WorldToViewport projects a world point to viewport pixels, reporting whether
// it lies in front of the camera.
func (c Camera) WorldToViewport(p geom.Vec3, viewW, viewH float64) (geom.Vec2, bool) {
	clip := c.ViewProj(viewW / viewH)
	x, y, _, w := clip.TransformVec4(p, 1)
	if w <= 1e-9 {
		return geom.Vec2{}, false
	}
	ndcX, ndcY := x/w, y/w
	return geom.Vec2{
		X: (ndcX*0.5 + 0.5) * viewW,
		Y: (1 - (ndcY*0.5 + 0.5)) * viewH,
	}, true
}

// PixelsPerWorldUnit is the screen scale at the target depth, used to keep
// gizmos a constant size on screen.
func (c Camera) PixelsPerWorldUnit(viewH float64) float64 {
	h := c.worldHeight()
	if h <= 0 {
		return 1
	}
	return viewH / h
}

// FrameBox moves the camera to fit a bounding box with FrameMargin padding,
// keeping the current orientation (SPEC-RENDER §7).
func (c *Camera) FrameBox(b geom.AABB, aspect float64) {
	if !b.Valid() {
		return
	}
	c.Target = b.Center()
	radius := math.Max(b.Diagonal()/2, 0.5)
	need := radius * 2 * (1 + FrameMargin)
	if aspect < 1 && aspect > 0 {
		need /= aspect // a tall, narrow viewport needs more vertical span
	}
	c.OrthoScale = need
	if c.Perspective {
		c.Dist = need / (2 * math.Tan(PerspectiveFOV*math.Pi/360))
	} else {
		// Keep the eye outside the scene so nothing clips behind the near plane.
		c.Dist = math.Max(c.Dist, radius*4)
	}
	c.Normalize()
}

// StandardView names the orientations the view cube and camera.view op script
// can jump to (SPEC-DATA §7).
type StandardView int

const (
	ViewIso StandardView = iota
	ViewFront
	ViewBack
	ViewLeft
	ViewRight
	ViewTop
	ViewBottom
)

// ParseStandardView maps an op-script view name to a StandardView.
func ParseStandardView(s string) (StandardView, bool) {
	switch s {
	case "iso":
		return ViewIso, true
	case "front":
		return ViewFront, true
	case "back":
		return ViewBack, true
	case "left":
		return ViewLeft, true
	case "right":
		return ViewRight, true
	case "top":
		return ViewTop, true
	case "bottom":
		return ViewBottom, true
	}
	return 0, false
}

// Angles returns the azimuth and elevation of a standard view.
//
// The azimuths are chosen so a "front" view looks down -Z at the XY plane,
// matching the Front default plane's frame (SPEC-GEOMETRY §3).
func (v StandardView) Angles() (azimuth, elevation float64) {
	switch v {
	case ViewFront:
		return 0, 0
	case ViewBack:
		return 180, 0
	case ViewRight:
		return 90, 0
	case ViewLeft:
		return 270, 0
	case ViewTop:
		return 0, ElevationLimit
	case ViewBottom:
		return 0, -ElevationLimit
	default:
		return IsoAzimuth, IsoElevation
	}
}

// PlaneView returns the camera angles that look squarely at a default plane,
// which is where sketch mode animates to (SPEC-UX §8.1).
func PlaneView(p geom.PlaneKind) (azimuth, elevation float64) {
	switch p {
	case geom.PlaneTop:
		return ViewTop.Angles()
	case geom.PlaneRight:
		return ViewRight.Angles()
	default:
		return ViewFront.Angles()
	}
}

// LookAlong points the camera down a normal direction, keeping the roll-free
// turntable model. Used for sketching on an arbitrary face.
func (c *Camera) LookAlong(n geom.Vec3) {
	n, ok := n.NormalizeOK()
	if !ok {
		return
	}
	// The eye sits on +n from the target, so forward is -n.
	el := math.Asin(clamp(n.Y, -1, 1)) * 180 / math.Pi
	az := math.Atan2(n.X, n.Z) * 180 / math.Pi
	c.Azimuth, c.Elevation = az, el
	c.Normalize()
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
