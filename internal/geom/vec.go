package geom

import "math"

// Vec2i is a point in sketch space, measured in subunits (SPEC-GEOMETRY §1.1).
// All sketch arithmetic is exact: no float ever touches a Vec2i.
type Vec2i struct{ X, Y int64 }

func (a Vec2i) Add(b Vec2i) Vec2i { return Vec2i{a.X + b.X, a.Y + b.Y} }
func (a Vec2i) Sub(b Vec2i) Vec2i { return Vec2i{a.X - b.X, a.Y - b.Y} }
func (a Vec2i) Neg() Vec2i        { return Vec2i{-a.X, -a.Y} }

// Dot is exact for coordinates inside SketchClamp.
func (a Vec2i) Dot(b Vec2i) int64 { return a.X*b.X + a.Y*b.Y }

// CrossZ is the z component of a×b, exact inside SketchClamp.
func (a Vec2i) CrossZ(b Vec2i) int64 { return a.X*b.Y - a.Y*b.X }

// LenSq is exact; it can reach ~2^47 for clamped coordinates.
func (a Vec2i) LenSq() int64 { return a.X*a.X + a.Y*a.Y }

// Len is the only lossy Vec2i operation and exists for UI readouts only.
func (a Vec2i) Len() float64 { return math.Hypot(float64(a.X), float64(a.Y)) }

// Units converts subunits to world units (for lifting into 3D).
func (a Vec2i) Units() Vec2 { return Vec2{float64(a.X) / Unit, float64(a.Y) / Unit} }

// Vec2 is a float 2D vector: screen space, UVs, and scratch math.
type Vec2 struct{ X, Y float64 }

func (a Vec2) Add(b Vec2) Vec2       { return Vec2{a.X + b.X, a.Y + b.Y} }
func (a Vec2) Sub(b Vec2) Vec2       { return Vec2{a.X - b.X, a.Y - b.Y} }
func (a Vec2) Mul(s float64) Vec2    { return Vec2{a.X * s, a.Y * s} }
func (a Vec2) Neg() Vec2             { return Vec2{-a.X, -a.Y} }
func (a Vec2) Dot(b Vec2) float64    { return a.X*b.X + a.Y*b.Y }
func (a Vec2) CrossZ(b Vec2) float64 { return a.X*b.Y - a.Y*b.X }
func (a Vec2) LenSq() float64        { return a.X*a.X + a.Y*a.Y }
func (a Vec2) Len() float64          { return math.Hypot(a.X, a.Y) }

func (a Vec2) Normalize() Vec2 {
	l := a.Len()
	if l < NormalEps {
		return Vec2{}
	}
	return Vec2{a.X / l, a.Y / l}
}

// Vec3 is a point or direction in model space, in world units (float64).
type Vec3 struct{ X, Y, Z float64 }

// Axis unit vectors, used pervasively (plane normals, gizmos, snapping).
var (
	AxisX = Vec3{1, 0, 0}
	AxisY = Vec3{0, 1, 0}
	AxisZ = Vec3{0, 0, 1}
)

func (a Vec3) Add(b Vec3) Vec3    { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3    { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Mul(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Neg() Vec3          { return Vec3{-a.X, -a.Y, -a.Z} }

// MulV multiplies component-wise (scale vectors, mirror factors).
func (a Vec3) MulV(b Vec3) Vec3   { return Vec3{a.X * b.X, a.Y * b.Y, a.Z * b.Z} }
func (a Vec3) Dot(b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

func (a Vec3) LenSq() float64 { return a.X*a.X + a.Y*a.Y + a.Z*a.Z }
func (a Vec3) Len() float64   { return math.Sqrt(a.LenSq()) }

// Normalize returns the unit vector, or the zero vector for a degenerate input.
// Callers that must distinguish those cases use NormalizeOK.
func (a Vec3) Normalize() Vec3 {
	v, _ := a.NormalizeOK()
	return v
}

func (a Vec3) NormalizeOK() (Vec3, bool) {
	l := a.Len()
	if l < NormalEps {
		return Vec3{}, false
	}
	return Vec3{a.X / l, a.Y / l, a.Z / l}, true
}

func (a Vec3) Dist(b Vec3) float64 { return a.Sub(b).Len() }

// MinV / MaxV are component-wise, for AABB accumulation.
func (a Vec3) MinV(b Vec3) Vec3 {
	return Vec3{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Min(a.Z, b.Z)}
}

func (a Vec3) MaxV(b Vec3) Vec3 {
	return Vec3{math.Max(a.X, b.X), math.Max(a.Y, b.Y), math.Max(a.Z, b.Z)}
}

// NearEq reports whether a and b are within eps in every component.
func (a Vec3) NearEq(b Vec3, eps float64) bool {
	return math.Abs(a.X-b.X) <= eps && math.Abs(a.Y-b.Y) <= eps && math.Abs(a.Z-b.Z) <= eps
}

// Lerp interpolates linearly; t is not clamped.
func (a Vec3) Lerp(b Vec3, t float64) Vec3 {
	return Vec3{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t, a.Z + (b.Z-a.Z)*t}
}

// MaxAbsAxis returns the index (0,1,2) of the largest |component|, ties X>Y>Z.
func (a Vec3) MaxAbsAxis() int {
	ax, ay, az := math.Abs(a.X), math.Abs(a.Y), math.Abs(a.Z)
	if ax >= ay && ax >= az {
		return 0
	}
	if ay >= az {
		return 1
	}
	return 2
}

// Axis returns component i (0=X, 1=Y, 2=Z); it panics only on programmer error,
// which is why callers pass literals or MaxAbsAxis results.
func (a Vec3) Axis(i int) float64 {
	switch i {
	case 0:
		return a.X
	case 1:
		return a.Y
	default:
		return a.Z
	}
}

// UnitAxis returns the unit vector for axis i.
func UnitAxis(i int) Vec3 {
	switch i {
	case 0:
		return AxisX
	case 1:
		return AxisY
	default:
		return AxisZ
	}
}
