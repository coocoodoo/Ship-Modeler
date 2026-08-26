package geom

import (
	"math"
	"math/rand"
	"testing"
)

func vecNear(t *testing.T, got, want Vec3, eps float64, what string) {
	t.Helper()
	if !got.NearEq(want, eps) {
		t.Fatalf("%s = %+v, want %+v", what, got, want)
	}
}

func TestVec3Basics(t *testing.T) {
	a := Vec3{1, 2, 3}
	b := Vec3{4, 5, 6}
	vecNear(t, a.Add(b), Vec3{5, 7, 9}, 0, "Add")
	vecNear(t, b.Sub(a), Vec3{3, 3, 3}, 0, "Sub")
	vecNear(t, a.Mul(2), Vec3{2, 4, 6}, 0, "Mul")
	if got := a.Dot(b); got != 32 {
		t.Fatalf("Dot = %v, want 32", got)
	}
	vecNear(t, a.Cross(b), Vec3{-3, 6, -3}, 0, "Cross")
	vecNear(t, AxisX.Cross(AxisY), AxisZ, 0, "X cross Y")
	vecNear(t, AxisY.Cross(AxisZ), AxisX, 0, "Y cross Z")
	vecNear(t, AxisZ.Cross(AxisX), AxisY, 0, "Z cross X")

	if got := (Vec3{3, 4, 0}).Len(); got != 5 {
		t.Fatalf("Len = %v, want 5", got)
	}
	if _, ok := (Vec3{}).NormalizeOK(); ok {
		t.Fatal("normalizing the zero vector should report failure")
	}
	if l := (Vec3{0.001, -2, 30}).Normalize().Len(); math.Abs(l-1) > 1e-12 {
		t.Fatalf("normalized length = %v", l)
	}
}

func TestVec3MaxAbsAxisTies(t *testing.T) {
	cases := []struct {
		v    Vec3
		want int
	}{
		{Vec3{1, 1, 1}, 0},
		{Vec3{0, 1, 1}, 1},
		{Vec3{0, 0, 1}, 2},
		{Vec3{-5, 4, 4}, 0},
		{Vec3{1, -9, 2}, 1},
	}
	for _, tc := range cases {
		if got := tc.v.MaxAbsAxis(); got != tc.want {
			t.Errorf("%v: MaxAbsAxis = %d, want %d", tc.v, got, tc.want)
		}
	}
}

func TestVec2iUnits(t *testing.T) {
	p := Vec2i{SubunitsPerUnit * 3, -SubunitsPerUnit / 2}
	u := p.Units()
	if u.X != 3 || u.Y != -0.5 {
		t.Fatalf("Units = %+v, want {3 -0.5}", u)
	}
	if got := (Vec2i{3, 4}).LenSq(); got != 25 {
		t.Fatalf("LenSq = %d, want 25", got)
	}
	if got := (Vec2i{2, 3}).CrossZ(Vec2i{4, 5}); got != 2*5-3*4 {
		t.Fatalf("CrossZ = %d", got)
	}
}

func TestMat4Identity(t *testing.T) {
	id := Identity()
	p := Vec3{3, -4, 5}
	vecNear(t, id.TransformPoint(p), p, 0, "identity point")
	vecNear(t, id.Mul(id).TransformPoint(p), p, 0, "identity squared")
}

func TestMat4TranslateScaleRotate(t *testing.T) {
	p := Vec3{1, 0, 0}
	vecNear(t, Translate(Vec3{2, 3, 4}).TransformPoint(p), Vec3{3, 3, 4}, 1e-12, "translate")
	// Translation must not affect directions.
	vecNear(t, Translate(Vec3{2, 3, 4}).TransformDir(p), p, 1e-12, "translate dir")
	vecNear(t, Scale(Vec3{2, 2, 2}).TransformPoint(p), Vec3{2, 0, 0}, 1e-12, "scale")

	// Right-handed rotations: +90 deg about Z takes +X to +Y.
	vecNear(t, RotateZ(math.Pi/2).TransformPoint(AxisX), AxisY, 1e-12, "rotZ")
	vecNear(t, RotateX(math.Pi/2).TransformPoint(AxisY), AxisZ, 1e-12, "rotX")
	vecNear(t, RotateY(math.Pi/2).TransformPoint(AxisZ), AxisX, 1e-12, "rotY")
	vecNear(t, RotateAxis(AxisZ, math.Pi/2).TransformPoint(AxisX), AxisY, 1e-12, "rotAxis")
}

func TestMat4CompositionOrder(t *testing.T) {
	// A.Mul(B) must mean "apply B, then A".
	rot := RotateZ(math.Pi / 2)
	tr := Translate(Vec3{10, 0, 0})
	rotThenTranslate := tr.Mul(rot)
	vecNear(t, rotThenTranslate.TransformPoint(AxisX), Vec3{10, 1, 0}, 1e-12, "rot then translate")
	translateThenRot := rot.Mul(tr)
	vecNear(t, translateThenRot.TransformPoint(AxisX), Vec3{0, 11, 0}, 1e-12, "translate then rot")
}

func TestMat4Invert(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	mats := []Mat4{
		Identity(),
		Translate(Vec3{3, -4, 5}),
		Scale(Vec3{2, 0.5, -3}),
		RotateAxis(Vec3{1, 2, 3}, 0.7),
		LookAt(Vec3{5, 6, 7}, Vec3{0, 1, 0}, AxisY),
		Perspective(math.Pi/4, 16.0/9.0, 0.1, 1000),
		Ortho(-8, 8, -4.5, 4.5, -100, 100),
		Translate(Vec3{1, 2, 3}).Mul(RotateY(0.3)).Mul(Scale(Vec3{2, 2, 2})),
	}
	for i, m := range mats {
		inv, ok := m.Invert()
		if !ok {
			t.Fatalf("matrix %d reported singular", i)
		}
		prod := m.Mul(inv)
		for r := 0; r < 4; r++ {
			for c := 0; c < 4; c++ {
				want := 0.0
				if r == c {
					want = 1
				}
				if math.Abs(prod.At(r, c)-want) > 1e-9 {
					t.Fatalf("matrix %d: (M*Minv)[%d][%d] = %v, want %v", i, r, c, prod.At(r, c), want)
				}
			}
		}
	}
	// Singular matrices are reported, not silently wrong.
	if _, ok := Scale(Vec3{1, 0, 1}).Invert(); ok {
		t.Fatal("singular matrix reported invertible")
	}
	// Round-tripping random points through M and M-inverse.
	m := Translate(Vec3{2, 3, 4}).Mul(RotateAxis(Vec3{0.3, 1, -0.2}, 1.1))
	inv, _ := m.Invert()
	for i := 0; i < 100; i++ {
		p := Vec3{rng.Float64()*20 - 10, rng.Float64()*20 - 10, rng.Float64()*20 - 10}
		vecNear(t, inv.TransformPoint(m.TransformPoint(p)), p, 1e-9, "round trip")
	}
}

func TestLookAtAndProjection(t *testing.T) {
	eye := Vec3{0, 0, 10}
	view := LookAt(eye, Vec3{}, AxisY)
	// The target lands at the eye-space origin offset along -Z.
	vecNear(t, view.TransformPoint(Vec3{}), Vec3{0, 0, -10}, 1e-12, "target in eye space")
	vecNear(t, view.TransformPoint(eye), Vec3{}, 1e-12, "eye in eye space")
	// +X in world stays +X on screen for this camera.
	vecNear(t, view.TransformDir(AxisX), AxisX, 1e-12, "right stays right")

	proj := Ortho(-2, 2, -1, 1, 0.1, 100)
	// A point at the near-plane centre maps to clip z = -1.
	got := proj.TransformPoint(Vec3{0, 0, -0.1})
	if math.Abs(got.Z+1) > 1e-9 {
		t.Fatalf("ortho near z = %v, want -1", got.Z)
	}
	got = proj.TransformPoint(Vec3{2, 1, -0.1})
	if math.Abs(got.X-1) > 1e-9 || math.Abs(got.Y-1) > 1e-9 {
		t.Fatalf("ortho corner = %+v, want x=1 y=1", got)
	}

	persp := Perspective(math.Pi/2, 1, 1, 100)
	// With a 90 degree fov, the point (1,0,-1) sits on the right clip edge.
	x, _, _, w := persp.TransformVec4(Vec3{1, 0, -1}, 1)
	if math.Abs(x/w-1) > 1e-9 {
		t.Fatalf("perspective edge x/w = %v, want 1", x/w)
	}
}

func TestAABB(t *testing.T) {
	b := Empty()
	if b.Valid() {
		t.Fatal("empty box reported valid")
	}
	b = b.AddPoint(Vec3{1, 2, 3}).AddPoint(Vec3{-1, 5, 0})
	if !b.Valid() {
		t.Fatal("box with points reported invalid")
	}
	vecNear(t, b.Min, Vec3{-1, 2, 0}, 0, "min")
	vecNear(t, b.Max, Vec3{1, 5, 3}, 0, "max")
	vecNear(t, b.Size(), Vec3{2, 3, 3}, 0, "size")
	vecNear(t, b.Center(), Vec3{0, 3.5, 1.5}, 0, "center")
	if got := b.LongestSide(); got != 3 {
		t.Fatalf("LongestSide = %v, want 3", got)
	}
	if !b.Contains(Vec3{0, 3, 1}) || b.Contains(Vec3{0, 1, 1}) {
		t.Fatal("Contains is wrong")
	}

	other := AABBOf([]Vec3{{10, 10, 10}, {12, 12, 12}})
	if b.Intersects(other) {
		t.Fatal("disjoint boxes reported intersecting")
	}
	u := b.Union(other)
	vecNear(t, u.Min, Vec3{-1, 2, 0}, 0, "union min")
	vecNear(t, u.Max, Vec3{12, 12, 12}, 0, "union max")
	// Union with an empty box is a no-op in both directions.
	vecNear(t, b.Union(Empty()).Max, b.Max, 0, "union empty")
	vecNear(t, Empty().Union(b).Max, b.Max, 0, "empty union")

	e := b.Expand(1)
	vecNear(t, e.Min, Vec3{-2, 1, -1}, 0, "expand min")

	// Transforming by a 90 degree rotation about Y is exact for the corners.
	rot := RotateY(math.Pi / 2)
	unit := AABB{Min: Vec3{0, 0, 0}, Max: Vec3{1, 1, 1}}
	tb := unit.Transform(rot)
	vecNear(t, tb.Min, Vec3{0, 0, -1}, 1e-12, "rotated min")
	vecNear(t, tb.Max, Vec3{1, 1, 0}, 1e-12, "rotated max")

	if len(unit.Corners()) != 8 {
		t.Fatal("wrong corner count")
	}
}

func TestSnapSubunits(t *testing.T) {
	cases := []struct{ v, step, want int64 }{
		{0, 256, 0},
		{127, 256, 0},
		{128, 256, 256},
		{129, 256, 256},
		{255, 256, 256},
		{-127, 256, 0},
		{-128, 256, -256},
		{-300, 256, -256},
		{-400, 256, -512},
		{100, 64, 128},
		{31, 64, 0},
		{32, 64, 64},
		{999, 0, 999},
	}
	for _, tc := range cases {
		if got := SnapSubunits(tc.v, tc.step); got != tc.want {
			t.Errorf("SnapSubunits(%d,%d) = %d, want %d", tc.v, tc.step, got, tc.want)
		}
	}
}

func TestSnapWorldUnits(t *testing.T) {
	if got := SnapWithStep(3.4, SnapGrid); got != 3 {
		t.Errorf("grid snap 3.4 = %v, want 3", got)
	}
	if got := SnapWithStep(3.6, SnapGrid); got != 4 {
		t.Errorf("grid snap 3.6 = %v, want 4", got)
	}
	if got := SnapWithStep(3.3, SnapFine); got != 3.25 {
		t.Errorf("fine snap 3.3 = %v, want 3.25", got)
	}
	if got := SnapWithStep(-3.3, SnapFine); got != -3.25 {
		t.Errorf("fine snap -3.3 = %v, want -3.25", got)
	}
	// Free snapping still lands on the subunit lattice.
	free := SnapWithStep(1.0/3.0, SnapNone)
	if math.Abs(free*Unit-math.Round(free*Unit)) > 1e-9 {
		t.Errorf("free snap %v is not on the subunit lattice", free)
	}
	if math.Abs(free-1.0/3.0) > 1.0/512.0 {
		t.Errorf("free snap moved the value too far: %v", free)
	}
	vecNear(t, SnapVec3(Vec3{1.2, -0.6, 4.5}, SnapGrid), Vec3{1, -1, 5}, 0, "SnapVec3")
}

func TestSubunitConversionAndClamp(t *testing.T) {
	if got := ToSubunits(2.5); got != 640 {
		t.Errorf("ToSubunits(2.5) = %d, want 640", got)
	}
	if got := ToUnits(640); got != 2.5 {
		t.Errorf("ToUnits(640) = %v, want 2.5", got)
	}
	if got := ToSubunits(1e9); got != SketchClamp {
		t.Errorf("clamp high = %d, want %d", got, SketchClamp)
	}
	if got := ToSubunits(-1e9); got != -SketchClamp {
		t.Errorf("clamp low = %d, want %d", got, -SketchClamp)
	}
	if !InSketchRange(Vec2i{SketchClamp, -SketchClamp}) {
		t.Error("boundary point reported out of range")
	}
	if InSketchRange(Vec2i{SketchClamp + 1, 0}) {
		t.Error("out-of-range point reported in range")
	}
	if got := ClampSketchVec(Vec2i{SketchClamp * 2, -SketchClamp * 2}); got != (Vec2i{SketchClamp, -SketchClamp}) {
		t.Errorf("ClampSketchVec = %v", got)
	}
}

func TestBuiltinPlaneFrames(t *testing.T) {
	want := map[PlaneKind]Frame{
		PlaneTop:   {U: Vec3{1, 0, 0}, V: Vec3{0, 0, -1}, N: Vec3{0, 1, 0}},
		PlaneFront: {U: Vec3{1, 0, 0}, V: Vec3{0, 1, 0}, N: Vec3{0, 0, 1}},
		PlaneRight: {U: Vec3{0, 0, -1}, V: Vec3{0, 1, 0}, N: Vec3{1, 0, 0}},
	}
	for k, w := range want {
		f := PlaneFrame(k)
		vecNear(t, f.U, w.U, 0, k.String()+" U")
		vecNear(t, f.V, w.V, 0, k.String()+" V")
		vecNear(t, f.N, w.N, 0, k.String()+" N")
		if !f.IsRightHanded() {
			t.Errorf("%s frame is not right-handed", k)
		}
		if !f.Orthonormal() {
			t.Errorf("%s frame is not orthonormal", k)
		}
		// Every default plane has V pointing up-ish on screen: for Top the
		// up axis is -Z, for the two vertical planes it is world +Y.
		if k != PlaneTop && f.V != (Vec3{0, 1, 0}) {
			t.Errorf("%s: V = %v, want world up so sketches read upright", k, f.V)
		}
	}
	for _, name := range []string{"Top", "Front", "Right"} {
		if _, ok := ParsePlaneKind(name); !ok {
			t.Errorf("ParsePlaneKind(%q) failed", name)
		}
	}
	if _, ok := ParsePlaneKind("Bogus"); ok {
		t.Error("ParsePlaneKind accepted a bogus name")
	}
}

func TestFrameFromNormalConvention(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for i := 0; i < 500; i++ {
		n := Vec3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}
		if n.Len() < 1e-6 {
			continue
		}
		f := FrameFromNormal(Vec3{1, 2, 3}, n)
		if !f.Orthonormal() {
			t.Fatalf("frame from %v is not orthonormal: %+v", n, f)
		}
		if !f.IsRightHanded() {
			t.Fatalf("frame from %v is not right-handed", n)
		}
	}
	// Determinism: the same normal always yields the same basis.
	a := FrameFromNormal(Vec3{}, Vec3{0.3, 0.4, 0.5})
	b := FrameFromNormal(Vec3{}, Vec3{0.3, 0.4, 0.5}.Mul(7))
	vecNear(t, a.U, b.U, 1e-12, "deterministic U")
	// The axis-aligned tie-break is X > Y > Z.
	f := FrameFromNormal(Vec3{}, AxisY)
	vecNear(t, f.U, AxisX, 1e-12, "tie-break picks X")
	// Degenerate input yields a usable basis instead of NaNs.
	d := FrameFromNormal(Vec3{}, Vec3{})
	if !d.Orthonormal() {
		t.Fatal("degenerate normal produced a broken frame")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	f := FrameFromNormal(Vec3{2, -1, 4}, Vec3{1, 2, 3})
	rng := rand.New(rand.NewSource(5))
	for i := 0; i < 200; i++ {
		p := Vec2{rng.Float64()*20 - 10, rng.Float64()*20 - 10}
		h := rng.Float64()*4 - 2
		w := f.ToWorldAt(p, h)
		back := f.ToLocal(w)
		if math.Abs(back.X-p.X) > 1e-9 || math.Abs(back.Y-p.Y) > 1e-9 {
			t.Fatalf("ToLocal(ToWorld(%v)) = %v", p, back)
		}
		if math.Abs(f.Height(w)-h) > 1e-9 {
			t.Fatalf("Height = %v, want %v", f.Height(w), h)
		}
	}
	// Lifting a sketch point converts subunits to world units.
	w := PlaneFrame(PlaneFront).LiftSub(Vec2i{SubunitsPerUnit * 2, SubunitsPerUnit * 3}, 5)
	vecNear(t, w, Vec3{2, 3, 5}, 1e-12, "LiftSub on Front")
}

func TestFrameFlipAndTransform(t *testing.T) {
	f := PlaneFrame(PlaneFront)
	fl := f.Flip()
	vecNear(t, fl.N, Vec3{0, 0, -1}, 0, "flipped normal")
	if !fl.IsRightHanded() {
		t.Fatal("flipped frame is not right-handed")
	}
	m := Translate(Vec3{5, 0, 0}).Mul(RotateY(math.Pi / 2))
	tf := f.Transformed(m)
	vecNear(t, tf.O, Vec3{5, 0, 0}, 1e-12, "transformed origin")
	vecNear(t, tf.N, AxisX, 1e-12, "transformed normal")
	if !tf.Orthonormal() || !tf.IsRightHanded() {
		t.Fatal("transformed frame lost its invariants")
	}
	vecNear(t, f.Translated(Vec3{1, 1, 1}).O, Vec3{1, 1, 1}, 0, "translated origin")
}
