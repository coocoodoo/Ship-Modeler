package geom

import "math"

// Mat4 is a 4×4 float64 matrix in ROW-MAJOR storage: M[row*4+col].
// Vectors are treated as columns, so transforms compose left-to-right as
// A.Mul(B) meaning "apply B, then A" — the usual convention.
//
// raylib's own Matrix type is float32 and column-major-ish; conversion lives in
// internal/render, never here (PLAN §4: no raylib types below render).
type Mat4 [16]float64

func Identity() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

func (m Mat4) At(row, col int) float64 { return m[row*4+col] }

// Mul returns m·n.
func (m Mat4) Mul(n Mat4) Mat4 {
	var out Mat4
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			out[r*4+c] = m[r*4+0]*n[0*4+c] +
				m[r*4+1]*n[1*4+c] +
				m[r*4+2]*n[2*4+c] +
				m[r*4+3]*n[3*4+c]
		}
	}
	return out
}

// TransformPoint applies the full transform including translation and the
// perspective divide.
func (m Mat4) TransformPoint(v Vec3) Vec3 {
	x := m[0]*v.X + m[1]*v.Y + m[2]*v.Z + m[3]
	y := m[4]*v.X + m[5]*v.Y + m[6]*v.Z + m[7]
	z := m[8]*v.X + m[9]*v.Y + m[10]*v.Z + m[11]
	w := m[12]*v.X + m[13]*v.Y + m[14]*v.Z + m[15]
	if w != 0 && w != 1 {
		inv := 1 / w
		return Vec3{x * inv, y * inv, z * inv}
	}
	return Vec3{x, y, z}
}

// TransformVec4 applies the transform to (v,w) and returns the raw homogeneous
// result — needed for clip-space work (picking, projection of gizmos).
func (m Mat4) TransformVec4(v Vec3, w float64) (float64, float64, float64, float64) {
	return m[0]*v.X + m[1]*v.Y + m[2]*v.Z + m[3]*w,
		m[4]*v.X + m[5]*v.Y + m[6]*v.Z + m[7]*w,
		m[8]*v.X + m[9]*v.Y + m[10]*v.Z + m[11]*w,
		m[12]*v.X + m[13]*v.Y + m[14]*v.Z + m[15]*w
}

// TransformDir ignores translation (for normals under rigid motions and for
// direction vectors generally).
func (m Mat4) TransformDir(v Vec3) Vec3 {
	return Vec3{
		m[0]*v.X + m[1]*v.Y + m[2]*v.Z,
		m[4]*v.X + m[5]*v.Y + m[6]*v.Z,
		m[8]*v.X + m[9]*v.Y + m[10]*v.Z,
	}
}

func (m Mat4) Transpose() Mat4 {
	var out Mat4
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			out[c*4+r] = m[r*4+c]
		}
	}
	return out
}

func Translate(v Vec3) Mat4 {
	m := Identity()
	m[3], m[7], m[11] = v.X, v.Y, v.Z
	return m
}

func Scale(v Vec3) Mat4 {
	m := Identity()
	m[0], m[5], m[10] = v.X, v.Y, v.Z
	return m
}

func RotateX(rad float64) Mat4 {
	s, c := math.Sin(rad), math.Cos(rad)
	m := Identity()
	m[5], m[6] = c, -s
	m[9], m[10] = s, c
	return m
}

func RotateY(rad float64) Mat4 {
	s, c := math.Sin(rad), math.Cos(rad)
	m := Identity()
	m[0], m[2] = c, s
	m[8], m[10] = -s, c
	return m
}

func RotateZ(rad float64) Mat4 {
	s, c := math.Sin(rad), math.Cos(rad)
	m := Identity()
	m[0], m[1] = c, -s
	m[4], m[5] = s, c
	return m
}

// RotateAxis builds a rotation of rad radians about a (not necessarily unit)
// axis, Rodrigues style.
func RotateAxis(axis Vec3, rad float64) Mat4 {
	a, ok := axis.NormalizeOK()
	if !ok {
		return Identity()
	}
	s, c := math.Sin(rad), math.Cos(rad)
	t := 1 - c
	m := Identity()
	m[0], m[1], m[2] = t*a.X*a.X+c, t*a.X*a.Y-s*a.Z, t*a.X*a.Z+s*a.Y
	m[4], m[5], m[6] = t*a.X*a.Y+s*a.Z, t*a.Y*a.Y+c, t*a.Y*a.Z-s*a.X
	m[8], m[9], m[10] = t*a.X*a.Z-s*a.Y, t*a.Y*a.Z+s*a.X, t*a.Z*a.Z+c
	return m
}

// LookAt builds a right-handed view matrix (camera looks down -Z in eye space).
func LookAt(eye, target, up Vec3) Mat4 {
	f := target.Sub(eye).Normalize() // forward
	s := f.Cross(up).Normalize()     // right
	u := s.Cross(f)                  // true up
	return Mat4{
		s.X, s.Y, s.Z, -s.Dot(eye),
		u.X, u.Y, u.Z, -u.Dot(eye),
		-f.X, -f.Y, -f.Z, f.Dot(eye),
		0, 0, 0, 1,
	}
}

// Perspective builds a right-handed projection into OpenGL clip space
// (z ∈ [-1,1]). fovY is in radians.
func Perspective(fovY, aspect, near, far float64) Mat4 {
	t := 1 / math.Tan(fovY/2)
	return Mat4{
		t / aspect, 0, 0, 0,
		0, t, 0, 0,
		0, 0, (far + near) / (near - far), (2 * far * near) / (near - far),
		0, 0, -1, 0,
	}
}

// Ortho builds a right-handed orthographic projection into OpenGL clip space.
func Ortho(left, right, bottom, top, near, far float64) Mat4 {
	return Mat4{
		2 / (right - left), 0, 0, -(right + left) / (right - left),
		0, 2 / (top - bottom), 0, -(top + bottom) / (top - bottom),
		0, 0, -2 / (far - near), -(far + near) / (far - near),
		0, 0, 0, 1,
	}
}

// Invert returns the general inverse and whether the matrix was invertible.
// Used for unprojection (screen ray) and frame math.
func (m Mat4) Invert() (Mat4, bool) {
	// Cofactor expansion on the row-major layout.
	a := &m
	var inv Mat4
	inv[0] = a[5]*a[10]*a[15] - a[5]*a[11]*a[14] - a[9]*a[6]*a[15] +
		a[9]*a[7]*a[14] + a[13]*a[6]*a[11] - a[13]*a[7]*a[10]
	inv[4] = -a[4]*a[10]*a[15] + a[4]*a[11]*a[14] + a[8]*a[6]*a[15] -
		a[8]*a[7]*a[14] - a[12]*a[6]*a[11] + a[12]*a[7]*a[10]
	inv[8] = a[4]*a[9]*a[15] - a[4]*a[11]*a[13] - a[8]*a[5]*a[15] +
		a[8]*a[7]*a[13] + a[12]*a[5]*a[11] - a[12]*a[7]*a[9]
	inv[12] = -a[4]*a[9]*a[14] + a[4]*a[10]*a[13] + a[8]*a[5]*a[14] -
		a[8]*a[6]*a[13] - a[12]*a[5]*a[10] + a[12]*a[6]*a[9]
	inv[1] = -a[1]*a[10]*a[15] + a[1]*a[11]*a[14] + a[9]*a[2]*a[15] -
		a[9]*a[3]*a[14] - a[13]*a[2]*a[11] + a[13]*a[3]*a[10]
	inv[5] = a[0]*a[10]*a[15] - a[0]*a[11]*a[14] - a[8]*a[2]*a[15] +
		a[8]*a[3]*a[14] + a[12]*a[2]*a[11] - a[12]*a[3]*a[10]
	inv[9] = -a[0]*a[9]*a[15] + a[0]*a[11]*a[13] + a[8]*a[1]*a[15] -
		a[8]*a[3]*a[13] - a[12]*a[1]*a[11] + a[12]*a[3]*a[9]
	inv[13] = a[0]*a[9]*a[14] - a[0]*a[10]*a[13] - a[8]*a[1]*a[14] +
		a[8]*a[2]*a[13] + a[12]*a[1]*a[10] - a[12]*a[2]*a[9]
	inv[2] = a[1]*a[6]*a[15] - a[1]*a[7]*a[14] - a[5]*a[2]*a[15] +
		a[5]*a[3]*a[14] + a[13]*a[2]*a[7] - a[13]*a[3]*a[6]
	inv[6] = -a[0]*a[6]*a[15] + a[0]*a[7]*a[14] + a[4]*a[2]*a[15] -
		a[4]*a[3]*a[14] - a[12]*a[2]*a[7] + a[12]*a[3]*a[6]
	inv[10] = a[0]*a[5]*a[15] - a[0]*a[7]*a[13] - a[4]*a[1]*a[15] +
		a[4]*a[3]*a[13] + a[12]*a[1]*a[7] - a[12]*a[3]*a[5]
	inv[14] = -a[0]*a[5]*a[14] + a[0]*a[6]*a[13] + a[4]*a[1]*a[14] -
		a[4]*a[2]*a[13] - a[12]*a[1]*a[6] + a[12]*a[2]*a[5]
	inv[3] = -a[1]*a[6]*a[11] + a[1]*a[7]*a[10] + a[5]*a[2]*a[11] -
		a[5]*a[3]*a[10] - a[9]*a[2]*a[7] + a[9]*a[3]*a[6]
	inv[7] = a[0]*a[6]*a[11] - a[0]*a[7]*a[10] - a[4]*a[2]*a[11] +
		a[4]*a[3]*a[10] + a[8]*a[2]*a[7] - a[8]*a[3]*a[6]
	inv[11] = -a[0]*a[5]*a[11] + a[0]*a[7]*a[9] + a[4]*a[1]*a[11] -
		a[4]*a[3]*a[9] - a[8]*a[1]*a[7] + a[8]*a[3]*a[5]
	inv[15] = a[0]*a[5]*a[10] - a[0]*a[6]*a[9] - a[4]*a[1]*a[10] +
		a[4]*a[2]*a[9] + a[8]*a[1]*a[6] - a[8]*a[2]*a[5]

	det := a[0]*inv[0] + a[1]*inv[4] + a[2]*inv[8] + a[3]*inv[12]
	if math.Abs(det) < 1e-300 {
		return Identity(), false
	}
	d := 1 / det
	for i := range inv {
		inv[i] *= d
	}
	return inv, true
}
