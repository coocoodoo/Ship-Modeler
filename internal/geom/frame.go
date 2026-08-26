package geom

// Frame is an orthonormal plane basis: origin O with in-plane axes U, V and
// normal N (U × V = N). Sketches and face paint both anchor to a Frame, and
// both keep their snapshot of it so they survive edits to the geometry that
// produced them (SPEC-GEOMETRY §3, §8.3).
type Frame struct {
	O, U, V, N Vec3
}

// PlaneKind identifies one of the three default planes, which always exist and
// can never be deleted (R1).
type PlaneKind int

const (
	PlaneTop   PlaneKind = iota // XZ, normal +Y
	PlaneFront                  // XY, normal +Z
	PlaneRight                  // YZ, normal +X
	numPlanes
)

// PlaneCount is how many default planes a document has.
const PlaneCount = int(numPlanes)

func (p PlaneKind) String() string {
	switch p {
	case PlaneTop:
		return "Top"
	case PlaneFront:
		return "Front"
	case PlaneRight:
		return "Right"
	default:
		return "Plane"
	}
}

// ParsePlaneKind maps a name (as used in op scripts and save files) to a plane.
func ParsePlaneKind(s string) (PlaneKind, bool) {
	switch s {
	case "Top", "top":
		return PlaneTop, true
	case "Front", "front":
		return PlaneFront, true
	case "Right", "right":
		return PlaneRight, true
	}
	return 0, false
}

// builtinFrames fixes the default planes' bases once, chosen so a sketch reads
// upright from the normal-on camera (SPEC-GEOMETRY §3). Each is right-handed.
var builtinFrames = [PlaneCount]Frame{
	PlaneTop:   {O: Vec3{}, U: Vec3{1, 0, 0}, V: Vec3{0, 0, -1}, N: Vec3{0, 1, 0}},
	PlaneFront: {O: Vec3{}, U: Vec3{1, 0, 0}, V: Vec3{0, 1, 0}, N: Vec3{0, 0, 1}},
	PlaneRight: {O: Vec3{}, U: Vec3{0, 0, -1}, V: Vec3{0, 1, 0}, N: Vec3{1, 0, 0}},
}

// PlaneFrame returns the fixed frame of a default plane.
func PlaneFrame(p PlaneKind) Frame {
	if p < 0 || int(p) >= PlaneCount {
		return builtinFrames[PlaneFront]
	}
	return builtinFrames[p]
}

// FrameFromNormal builds the canonical frame for an arbitrary plane, used for
// face sketches and face paint. The rule (SPEC-GEOMETRY §3) is deterministic:
// pick the world axis least aligned with N (ties X > Y > Z), remove the normal
// component to get U, then V = N × U.
func FrameFromNormal(o, n Vec3) Frame {
	nn, ok := n.NormalizeOK()
	if !ok {
		return Frame{O: o, U: AxisX, V: AxisY, N: AxisZ}
	}
	ref := leastAlignedAxis(nn)
	u, ok := ref.Sub(nn.Mul(ref.Dot(nn))).NormalizeOK()
	if !ok {
		// Unreachable for a unit normal, but never hand back a broken basis.
		u = AxisX
		if absf(nn.X) > 0.9 {
			u = AxisY
		}
		u = u.Sub(nn.Mul(u.Dot(nn))).Normalize()
	}
	return Frame{O: o, U: u, V: nn.Cross(u), N: nn}
}

// leastAlignedAxis returns the world axis with the smallest |dot(n, axis)|,
// breaking ties in the order X, Y, Z.
func leastAlignedAxis(n Vec3) Vec3 {
	ax, ay, az := absf(n.X), absf(n.Y), absf(n.Z)
	if ax <= ay && ax <= az {
		return AxisX
	}
	if ay <= az {
		return AxisY
	}
	return AxisZ
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ToWorld lifts an in-plane point (world units) onto the plane.
func (f Frame) ToWorld(p Vec2) Vec3 {
	return f.O.Add(f.U.Mul(p.X)).Add(f.V.Mul(p.Y))
}

// ToWorldAt lifts an in-plane point and offsets it along the normal by t.
func (f Frame) ToWorldAt(p Vec2, t float64) Vec3 {
	return f.ToWorld(p).Add(f.N.Mul(t))
}

// LiftSub lifts a sketch point (subunits) onto the plane at normal offset t.
func (f Frame) LiftSub(p Vec2i, t float64) Vec3 {
	return f.ToWorldAt(p.Units(), t)
}

// ToLocal projects a world point into plane coordinates (world units),
// discarding the normal component.
func (f Frame) ToLocal(p Vec3) Vec2 {
	d := p.Sub(f.O)
	return Vec2{d.Dot(f.U), d.Dot(f.V)}
}

// Height is the signed distance of p from the plane along N.
func (f Frame) Height(p Vec3) float64 {
	return p.Sub(f.O).Dot(f.N)
}

// Flip returns the same plane with the opposite normal, keeping a right-handed
// basis by mirroring V.
func (f Frame) Flip() Frame {
	return Frame{O: f.O, U: f.U, V: f.V.Neg(), N: f.N.Neg()}
}

// Transformed applies a rigid motion (rotation + translation) to the frame.
// Paint frames ride along with body moves this way (SPEC-GEOMETRY §8.4).
func (f Frame) Transformed(m Mat4) Frame {
	return Frame{
		O: m.TransformPoint(f.O),
		U: m.TransformDir(f.U).Normalize(),
		V: m.TransformDir(f.V).Normalize(),
		N: m.TransformDir(f.N).Normalize(),
	}
}

// Translated shifts the origin, leaving the basis alone.
func (f Frame) Translated(d Vec3) Frame {
	return Frame{O: f.O.Add(d), U: f.U, V: f.V, N: f.N}
}

// IsRightHanded reports whether U × V agrees with N, the invariant every frame
// must hold.
func (f Frame) IsRightHanded() bool {
	return f.U.Cross(f.V).Sub(f.N).Len() < 1e-6
}

// Orthonormal reports whether the basis is unit-length and mutually orthogonal
// within NormalEps-scale slack. Tests assert this after every frame build.
func (f Frame) Orthonormal() bool {
	const eps = 1e-9
	unit := func(v Vec3) bool { return absf(v.LenSq()-1) < 1e-9 }
	return unit(f.U) && unit(f.V) && unit(f.N) &&
		absf(f.U.Dot(f.V)) < eps && absf(f.U.Dot(f.N)) < eps && absf(f.V.Dot(f.N)) < eps
}
