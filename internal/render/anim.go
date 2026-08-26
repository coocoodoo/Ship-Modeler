package render

import "math"

// CameraAnimMillis is the duration of every scripted camera move
// (SPEC-RENDER §7): view-cube clicks, F-framing and sketch normal-on.
const CameraAnimMillis = 220.0

// CameraAnim interpolates the camera between two states with a cubic
// ease-in-out. Any manual camera input takes over from the animation's current
// state rather than snapping, which Cancel expresses.
//
// SPEC-RENDER §7 describes the orientation blend as a quaternion slerp. This
// camera has no roll by construction, and slerping two up-locked orientations
// introduces roll mid-path; interpolating azimuth along the shorter arc and
// elevation linearly is the roll-free equivalent. Logged in DECISIONS.
type CameraAnim struct {
	active   bool
	from, to Camera
	elapsed  float64 // milliseconds
}

// Active reports whether an animation is running.
func (a *CameraAnim) Active() bool { return a.active }

// Start begins an animation from the camera's current state to target.
// Starting from an already-matching state completes immediately.
func (a *CameraAnim) Start(from, to Camera) {
	to.Normalize()
	a.from, a.to, a.elapsed, a.active = from, to, 0, true
	if cameraNearlyEqual(from, to) {
		a.active = false
	}
}

// Cancel abandons the animation, leaving the camera wherever it reached.
func (a *CameraAnim) Cancel() { a.active = false }

// Step advances the animation by dtMillis and returns the camera for this
// frame plus whether it changed anything.
func (a *CameraAnim) Step(current Camera, dtMillis float64) (Camera, bool) {
	if !a.active {
		return current, false
	}
	a.elapsed += dtMillis
	t := a.elapsed / CameraAnimMillis
	if t >= 1 {
		a.active = false
		return a.to, true
	}
	return lerpCamera(a.from, a.to, easeInOutCubic(t)), true
}

// Target returns the state the animation is heading to.
func (a *CameraAnim) Target() Camera { return a.to }

// easeInOutCubic is the app's single easing curve (SPEC-UX §15).
func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	f := -2*t + 2
	return 1 - f*f*f/2
}

func lerpCamera(a, b Camera, t float64) Camera {
	out := Camera{
		Target:      a.Target.Lerp(b.Target, t),
		Azimuth:     a.Azimuth + shortestAngleDelta(a.Azimuth, b.Azimuth)*t,
		Elevation:   a.Elevation + (b.Elevation-a.Elevation)*t,
		Dist:        lerpLog(a.Dist, b.Dist, t),
		OrthoScale:  lerpLog(a.OrthoScale, b.OrthoScale, t),
		Perspective: b.Perspective,
	}
	out.Normalize()
	return out
}

// lerpLog interpolates a scale geometrically, so a 10x zoom feels even
// throughout instead of rushing at one end.
func lerpLog(a, b, t float64) float64 {
	if a <= 0 || b <= 0 {
		return a + (b-a)*t
	}
	return a * math.Pow(b/a, t)
}

// shortestAngleDelta returns the signed degrees from a to b through the
// shorter arc, so orbiting past 360 never spins the long way round.
func shortestAngleDelta(a, b float64) float64 {
	d := math.Mod(b-a, 360)
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d
}

func cameraNearlyEqual(a, b Camera) bool {
	const angEps, relEps = 1e-4, 1e-9
	return math.Abs(shortestAngleDelta(a.Azimuth, b.Azimuth)) < angEps &&
		math.Abs(a.Elevation-b.Elevation) < angEps &&
		math.Abs(a.Dist-b.Dist) < relEps*math.Max(1, a.Dist) &&
		math.Abs(a.OrthoScale-b.OrthoScale) < relEps*math.Max(1, a.OrthoScale) &&
		a.Target.NearEq(b.Target, 1e-9) &&
		a.Perspective == b.Perspective
}

// Smoothed is an exponentially smoothed scalar, used for the wheel zoom so it
// glides over roughly 120 ms instead of stepping (SPEC-RENDER §7).
type Smoothed struct {
	Value  float64
	target float64
	tauMs  float64
}

// NewSmoothed returns a smoother seeded at v with a time constant in ms.
func NewSmoothed(v, tauMs float64) Smoothed {
	return Smoothed{Value: v, target: v, tauMs: tauMs}
}

// Set jumps both the value and the target, cancelling any in-flight easing.
func (s *Smoothed) Set(v float64) { s.Value, s.target = v, v }

// Aim moves the target without touching the current value.
func (s *Smoothed) Aim(v float64) { s.target = v }

// Target returns the value being eased toward.
func (s *Smoothed) Target() float64 { return s.target }

// Step advances the smoothing and reports whether the value is still moving.
func (s *Smoothed) Step(dtMillis float64) bool {
	if s.tauMs <= 0 {
		s.Value = s.target
		return false
	}
	diff := s.target - s.Value
	if math.Abs(diff) < 1e-9 {
		s.Value = s.target
		return false
	}
	s.Value += diff * (1 - math.Exp(-dtMillis/s.tauMs))
	return true
}
