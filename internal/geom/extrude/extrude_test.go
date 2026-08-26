package extrude

import (
	"math"
	"strings"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
)

// TESTING §2 for extrude: analytic volumes for a box, a 16-gon prism, a drafted
// box against the frustum formula, a symmetric drafted solid, and a
// region-with-hole tube. Plus side-quad planarity and the draft clamp.

func u(v float64) int64          { return geom.ToSubunits(v) }
func pt(x, y float64) geom.Vec2i { return geom.Vec2i{X: u(x), Y: u(y)} }

func loopOf(pts ...geom.Vec2i) sketch2d.Loop {
	return sketch2d.Loop{Pts: pts, Src: make([]sketch2d.Source, len(pts))}
}

func rectLoop(x0, y0, x1, y1 float64) sketch2d.Loop {
	return loopOf(pt(x0, y0), pt(x1, y0), pt(x1, y1), pt(x0, y1))
}

func reversed(l sketch2d.Loop) sketch2d.Loop {
	out := sketch2d.Loop{Pts: make([]geom.Vec2i, len(l.Pts)), Src: make([]sketch2d.Source, len(l.Src))}
	for i := range l.Pts {
		out.Pts[i] = l.Pts[len(l.Pts)-1-i]
	}
	return out
}

func ngonLoop(r float64, segs int) sketch2d.Loop {
	pts := make([]geom.Vec2i, segs)
	for i := 0; i < segs; i++ {
		a := 2 * math.Pi * float64(i) / float64(segs)
		pts[i] = geom.Vec2i{
			X: int64(math.Round(float64(u(r)) * math.Cos(a))),
			Y: int64(math.Round(float64(u(r)) * math.Sin(a))),
		}
	}
	return loopOf(pts...)
}

// frontFrame is the Front default plane, which extrudes along +Z.
func frontFrame() geom.Frame { return geom.PlaneFrame(geom.PlaneFront) }

// areaOf converts a loop's exact doubled area into world square units.
func areaOf(l sketch2d.Loop) float64 {
	return math.Abs(float64(l.Area2())) / 2 / (geom.Unit * geom.Unit)
}

func build(t *testing.T, r sketch2d.Region, p Params) Result {
	t.Helper()
	got, err := Build([]sketch2d.Region{r}, p, 1)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := mesh.Validate(got.Mesh); err != nil {
		t.Fatalf("the extruded solid is invalid: %v", err)
	}
	return got
}

func closeTo(t *testing.T, got, want float64, what string) {
	t.Helper()
	scale := math.Max(1, math.Abs(want))
	if math.Abs(got-want)/scale > 1e-4 {
		t.Errorf("%s = %.6f, want %.6f", what, got, want)
	}
}

func TestExtrudeBoxVolume(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(0, 0, 4, 3)}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(2)})

	closeTo(t, mesh.Volume(got.Mesh), 4*3*2, "box volume")
	if n := len(got.Mesh.Faces); n != 6 {
		t.Errorf("a box has %d faces, want 6", n)
	}
	if got.Clamped {
		t.Error("an undrafted extrude reported a clamp")
	}
}

func TestExtrudeNGonPrismVolume(t *testing.T) {
	for _, segs := range []int{3, 8, 16, 32} {
		r := sketch2d.Region{Outer: ngonLoop(2, segs)}
		got := build(t, r, Params{Frame: frontFrame(), Depth: u(5)})

		// The exact profile area comes from the integer shoelace, so this
		// compares the solid against the polygon it was actually built from
		// rather than against an idealised circle.
		closeTo(t, mesh.Volume(got.Mesh), areaOf(r.Outer)*5, "prism volume")
		if n := len(got.Mesh.Faces); n != segs+2 {
			t.Errorf("segs=%d: faces = %d, want %d", segs, n, segs+2)
		}
	}
}

// TestExtrudeDraftedBoxMatchesTheFrustumFormula is the analytic check TESTING §2
// names. A constant-distance inset of a square is a uniform scale about its
// centre, so the solid is a true frustum and V = h/3 (A1 + A2 + sqrt(A1 A2))
// holds exactly.
func TestExtrudeDraftedBoxMatchesTheFrustumFormula(t *testing.T) {
	const side, depth = 8.0, 4.0
	for _, deg := range []float64{5, 15, 30} {
		r := sketch2d.Region{Outer: rectLoop(0, 0, side, side)}
		got := build(t, r, Params{Frame: frontFrame(), Depth: u(depth), Draft: deg})

		if got.Clamped {
			t.Fatalf("%.0f degrees on an 8 u square was clamped to %.3f", deg, got.AchievedDraft)
		}
		// The top shrinks by delta on every side, so its side is
		// side - 2*depth*tan(theta).
		delta := float64(sketch2d.DeltaForDraft(u(depth), deg)) / geom.Unit
		top := side - 2*delta
		a1, a2 := side*side, top*top
		want := depth / 3 * (a1 + a2 + math.Sqrt(a1*a2))

		closeTo(t, mesh.Volume(got.Mesh), want, "drafted box volume")
	}
}

// TestExtrudeNegativeDraftFlares covers the other sign: the far cap grows.
func TestExtrudeNegativeDraftFlares(t *testing.T) {
	const side, depth = 4.0, 3.0
	r := sketch2d.Region{Outer: rectLoop(0, 0, side, side)}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(depth), Draft: -20})

	straight := side * side * depth
	if v := mesh.Volume(got.Mesh); v <= straight {
		t.Errorf("a negative draft gave %v, want more than the straight %v", v, straight)
	}
}

// TestExtrudeSymmetricIsWidestAtTheSketchPlane covers SPEC-GEOMETRY §5.4: the
// profile sits in the middle and both caps taper away from it.
func TestExtrudeSymmetricIsWidestAtTheSketchPlane(t *testing.T) {
	const side, depth = 8.0, 4.0
	const deg = 15.0

	r := sketch2d.Region{Outer: rectLoop(-side/2, -side/2, side/2, side/2)}
	got := build(t, r, Params{
		Frame: frontFrame(), Depth: u(depth), Draft: deg, Dir: Symmetric,
	})

	// Two frusta of height depth/2 back to back, each from the full profile at
	// the sketch plane out to the inset cap.
	delta := float64(sketch2d.DeltaForDraft(u(depth/2), deg)) / geom.Unit
	top := side - 2*delta
	a0, a1 := side*side, top*top
	want := depth / 3 * (a0 + a1 + math.Sqrt(a0*a1))
	closeTo(t, mesh.Volume(got.Mesh), want, "symmetric drafted volume")

	// It really is centred: the bounding box straddles the sketch plane evenly.
	b := got.Mesh.AABB()
	closeTo(t, b.Min.Z, -depth/2, "near cap")
	closeTo(t, b.Max.Z, depth/2, "far cap")
}

func TestExtrudeSymmetricWithoutDraftIsAPlainPrism(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(-2, -2, 2, 2)}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(6), Dir: Symmetric})

	closeTo(t, mesh.Volume(got.Mesh), 4*4*6, "symmetric prism volume")
	if n := len(got.Mesh.Faces); n != 6 {
		t.Errorf("an undrafted symmetric box has %d faces, want 6", n)
	}
	b := got.Mesh.AABB()
	closeTo(t, b.Min.Z, -3, "near cap")
	closeTo(t, b.Max.Z, 3, "far cap")
}

// TestExtrudeTube covers a region with a hole: the solid is a tube and its
// volume is the ring area times the depth.
func TestExtrudeTube(t *testing.T) {
	r := sketch2d.Region{
		Outer: rectLoop(0, 0, 8, 8),
		Holes: []sketch2d.Loop{reversed(rectLoop(2, 2, 6, 6))},
	}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(3)})

	closeTo(t, mesh.Volume(got.Mesh), (8*8-4*4)*3, "tube volume")

	// Two capped ends plus four outer walls and four inner walls.
	if n := len(got.Mesh.Faces); n != 10 {
		t.Errorf("a square tube has %d faces, want 10", n)
	}
	// The hole really goes through: genus one.
	rep := mesh.Check(got.Mesh)
	if len(rep.GenusPer) != 1 || rep.GenusPer[0] != 1 {
		t.Errorf("genus = %v, want [1]", rep.GenusPer)
	}
}

func TestExtrudeReverseGoesTheOtherWay(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(0, 0, 4, 4)}
	fwd := build(t, r, Params{Frame: frontFrame(), Depth: u(3)})
	rev := build(t, r, Params{Frame: frontFrame(), Depth: u(3), Dir: Reverse})

	closeTo(t, mesh.Volume(fwd.Mesh), mesh.Volume(rev.Mesh), "reverse volume")

	if b := fwd.Mesh.AABB(); b.Min.Z != 0 || math.Abs(b.Max.Z-3) > 1e-9 {
		t.Errorf("forward spans z %v..%v, want 0..3", b.Min.Z, b.Max.Z)
	}
	if b := rev.Mesh.AABB(); math.Abs(b.Min.Z+3) > 1e-9 || b.Max.Z != 0 {
		t.Errorf("reverse spans z %v..%v, want -3..0", b.Min.Z, b.Max.Z)
	}
}

// TestSideQuadsArePlanar is the invariant that makes the mitre offset worth
// having: parallel near and far edges mean every side face is flat
// (SPEC-GEOMETRY §5.4).
func TestSideQuadsArePlanar(t *testing.T) {
	cases := []struct {
		name  string
		r     sketch2d.Region
		draft float64
		dir   Direction
	}{
		{"box", sketch2d.Region{Outer: rectLoop(0, 0, 6, 4)}, 0, Normal},
		{"drafted box", sketch2d.Region{Outer: rectLoop(0, 0, 6, 4)}, 12, Normal},
		{"drafted 16-gon", sketch2d.Region{Outer: ngonLoop(3, 16)}, 10, Normal},
		{"symmetric drafted", sketch2d.Region{Outer: rectLoop(-3, -3, 3, 3)}, 18, Symmetric},
		{"drafted tube", sketch2d.Region{
			Outer: rectLoop(0, 0, 10, 10),
			Holes: []sketch2d.Loop{reversed(rectLoop(3, 3, 7, 7))},
		}, 8, Normal},
		{"concave L", sketch2d.Region{Outer: loopOf(
			pt(0, 0), pt(6, 0), pt(6, 2), pt(2, 2), pt(2, 6), pt(0, 6))}, 6, Normal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := build(t, tc.r, Params{
				Frame: frontFrame(), Depth: u(4), Draft: tc.draft, Dir: tc.dir,
			})
			for i := range got.Mesh.Faces {
				if d := got.Mesh.Planarity(i); d > geom.PlanarDist {
					t.Errorf("face %d is out of plane by %g, tolerance is %g",
						i, d, geom.PlanarDist)
				}
			}
		})
	}
}

// TestDraftClampOnATightProfile is the UX clamp of SPEC-UX §9.3 seen from the
// geometry side: too much draft for the profile comes back clamped, and what
// comes back is still a valid solid.
func TestDraftClampOnATightProfile(t *testing.T) {
	// A 2 u square drafted 45 degrees over 4 u of depth would inset by 4 u,
	// far past its own half-width.
	r := sketch2d.Region{Outer: rectLoop(0, 0, 2, 2)}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(4), Draft: 45})

	if !got.Clamped {
		t.Fatal("45 degrees on a 2 u square was not clamped")
	}
	if got.AchievedDraft >= 45 {
		t.Errorf("achieved draft = %v, want less than the requested 45", got.AchievedDraft)
	}
	if got.AchievedDraft <= 0 {
		t.Errorf("achieved draft = %v, want a usable positive angle", got.AchievedDraft)
	}
	if v := mesh.Volume(got.Mesh); v <= 0 {
		t.Errorf("the clamped solid has volume %v", v)
	}
}

func TestExtrudeRejectsBadInput(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(0, 0, 4, 4)}
	f := frontFrame()

	if _, err := Build(nil, Params{Frame: f, Depth: u(1)}, 1); err == nil {
		t.Error("extruding no regions was accepted")
	}
	if _, err := Build([]sketch2d.Region{r}, Params{Frame: f, Depth: 0}, 1); err == nil {
		t.Error("a zero depth was accepted")
	}
	if _, err := Build([]sketch2d.Region{r}, Params{Frame: f, Depth: -5}, 1); err == nil {
		t.Error("a negative depth was accepted")
	}
	if _, err := Build([]sketch2d.Region{r}, Params{Frame: f, Depth: u(1), Draft: 80}, 1); err == nil {
		t.Error("a draft past 45 degrees was accepted")
	}
	empty := sketch2d.Region{Outer: loopOf(pt(0, 0), pt(1, 0))}
	if _, err := Build([]sketch2d.Region{empty}, Params{Frame: f, Depth: u(1)}, 1); err == nil {
		t.Error("a two-point loop was accepted")
	}
}

// TestTouchingRegionsAreRefusedInPlainWords covers the case a sketch produces
// naturally and a solid modeller cannot: a circle inside a rectangle makes two
// regions that share the circle, and extruding both would put two shells wall
// to wall along a whole surface. That is not a 2-manifold solid, so the answer
// has to be no — but it has to be a no that says which regions and what to do,
// not one that leaks the validator's opinion of vertex 23.
func TestTouchingRegionsAreRefusedInPlainWords(t *testing.T) {
	circle := ngonLoop(1.5, 16)
	ring := sketch2d.Region{
		Outer: rectLoop(-4, -4, 4, 4),
		Holes: []sketch2d.Loop{reversed(circle)},
	}
	disc := sketch2d.Region{Outer: circle}
	p := Params{Frame: frontFrame(), Depth: u(2)}

	// Each on its own is perfectly buildable.
	if _, err := Build([]sketch2d.Region{ring}, p, 1); err != nil {
		t.Fatalf("the ring alone failed: %v", err)
	}
	if _, err := Build([]sketch2d.Region{disc}, p, 1); err != nil {
		t.Fatalf("the disc alone failed: %v", err)
	}

	_, err := Build([]sketch2d.Region{ring, disc}, p, 1)
	if err == nil {
		t.Fatal("a ring and the disc filling it were extruded together")
	}
	msg := err.Error()
	for _, want := range []string{"touch", "one at a time"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal %q does not mention %q", msg, want)
		}
	}
	if strings.Contains(msg, "vertex") || strings.Contains(msg, "invalid mesh") {
		t.Errorf("the refusal leaks mesh internals: %q", msg)
	}
}

// TestSeparateRegionsAreStillFine keeps the adjacency check from over-reaching:
// two regions that merely sit near each other are the ordinary multi-region
// case and must still build.
func TestSeparateRegionsAreStillFine(t *testing.T) {
	a := sketch2d.Region{Outer: rectLoop(0, 0, 4, 4)}
	b := sketch2d.Region{Outer: rectLoop(6, 0, 10, 4)}
	got, err := Build([]sketch2d.Region{a, b}, Params{Frame: frontFrame(), Depth: u(3)}, 1)
	if err != nil {
		t.Fatalf("two disjoint squares failed: %v", err)
	}
	closeTo(t, mesh.Volume(got.Mesh), 2*16*3, "volume of two disjoint prisms")
}

// TestRegionsTouchingAtOneCornerAreAlsoRefused: a single shared point is enough
// to break the solid. Two squares meeting corner to corner weld into a mesh
// whose shared column of edges belongs to four faces, which is not a manifold
// and not something an extrude can fix. Refusing is the whole of the answer
// until booleans arrive.
func TestRegionsTouchingAtOneCornerAreAlsoRefused(t *testing.T) {
	a := sketch2d.Region{Outer: rectLoop(0, 0, 4, 4)}
	b := sketch2d.Region{Outer: rectLoop(4, 4, 8, 8)}
	_, err := Build([]sketch2d.Region{a, b}, Params{Frame: frontFrame(), Depth: u(2)}, 1)
	if err == nil {
		t.Fatal("two squares meeting at a corner were extruded together")
	}
	if !strings.Contains(err.Error(), "touch") {
		t.Errorf("the refusal %q does not say they touch", err)
	}
}

// TestFaceIdentitiesAreStableAndUnique covers SPEC-GEOMETRY §5.5: every face
// gets its own never-reused id, and the side faces trace back to the profile
// edge that made them.
func TestFaceIdentitiesAreStableAndUnique(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(0, 0, 4, 4)}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(2)})

	seen := map[mesh.FaceUID]bool{}
	for i := range got.Mesh.Faces {
		uid := got.Mesh.Faces[i].ID
		if uid == mesh.NoFace {
			t.Errorf("face %d has no id", i)
		}
		if seen[uid] {
			t.Errorf("face id %d was handed out twice", uid)
		}
		seen[uid] = true
		if uid.BodyID() != 1 {
			t.Errorf("face %d belongs to body %d, want 1", i, uid.BodyID())
		}
	}

	// Building the same profile again gives a different body its own ids.
	other, err := Build([]sketch2d.Region{r}, Params{Frame: frontFrame(), Depth: u(2)}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range other.Mesh.Faces {
		if seen[other.Mesh.Faces[i].ID] {
			t.Errorf("body 2 reused body 1's face id %d", other.Mesh.Faces[i].ID)
		}
	}
}

// TestMultipleRegionsBecomeMultipleShells covers extruding a selection: two
// separate regions come out as one mesh with two shells.
func TestMultipleRegionsBecomeMultipleShells(t *testing.T) {
	a := sketch2d.Region{Outer: rectLoop(0, 0, 2, 2)}
	b := sketch2d.Region{Outer: rectLoop(5, 5, 8, 8)}

	got, err := Build([]sketch2d.Region{a, b}, Params{Frame: frontFrame(), Depth: u(2)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := mesh.Validate(got.Mesh); err != nil {
		t.Fatalf("two-region extrude is invalid: %v", err)
	}
	rep := mesh.Check(got.Mesh)
	if rep.Shells != 2 {
		t.Errorf("shells = %d, want 2", rep.Shells)
	}
	closeTo(t, mesh.Volume(got.Mesh), 2*2*2+3*3*2, "two-region volume")
}

// TestExtrudeOnEveryDefaultPlane checks the lift is right for all three frames,
// not just the one that happens to be axis-aligned the convenient way.
func TestExtrudeOnEveryDefaultPlane(t *testing.T) {
	for _, k := range []geom.PlaneKind{geom.PlaneTop, geom.PlaneFront, geom.PlaneRight} {
		r := sketch2d.Region{Outer: rectLoop(0, 0, 3, 2)}
		got := build(t, r, Params{Frame: geom.PlaneFrame(k), Depth: u(5)})

		closeTo(t, mesh.Volume(got.Mesh), 3*2*5, k.String()+" volume")

		// The solid extends along the plane's normal by exactly the depth.
		n := geom.PlaneFrame(k).N
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range got.Mesh.Verts {
			d := v.Dot(n)
			lo, hi = math.Min(lo, d), math.Max(hi, d)
		}
		closeTo(t, hi-lo, 5, k.String()+" extent along the normal")
	}
}

// TestAuthoredVerticesStayOnTheGrid is the snap-on-author policy of
// SPEC-GEOMETRY §1.2: an extrude of grid-aligned input on an axis-aligned plane
// must not leave anything off the subunit lattice.
func TestAuthoredVerticesStayOnTheGrid(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(0, 0, 4, 4)}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(3), Draft: 20})

	for i, v := range got.Mesh.Verts {
		for _, c := range []float64{v.X, v.Y, v.Z} {
			if math.Abs(c*geom.Unit-math.Round(c*geom.Unit)) > 1e-6 {
				t.Errorf("vertex %d component %v is off the subunit lattice", i, c)
			}
		}
	}
}
