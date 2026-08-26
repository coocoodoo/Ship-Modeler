// Package extrude turns closed sketch regions into solids
// (SPEC-GEOMETRY §5). It bridges the exact 2D world of sketch2d and the
// float64 mesh world, and every solid it produces is put through the same
// validation gate a boolean result is.
package extrude

import (
	"fmt"
	"math"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
)

// Direction is which way an extrude runs from its sketch plane (SPEC-UX §9.3).
type Direction uint8

const (
	// Normal extrudes along the plane's normal.
	Normal Direction = iota
	// Reverse extrudes the other way.
	Reverse
	// Symmetric splits the depth either side of the plane.
	Symmetric
)

func (d Direction) String() string {
	switch d {
	case Reverse:
		return "Reverse"
	case Symmetric:
		return "Symmetric"
	default:
		return "Normal"
	}
}

// MaxDraftDegrees bounds the draft angle either way (SPEC-GEOMETRY §5).
const MaxDraftDegrees = 45.0

// Params describe one extrude.
type Params struct {
	// Frame is the sketch plane the profile lives on.
	Frame geom.Frame
	// Depth is the total extrusion in subunits, always positive; Dir decides
	// which way it goes.
	Depth int64
	// Draft tapers the far cap, in degrees. Positive shrinks it.
	Draft float64
	Dir   Direction
}

// Result is a built solid.
type Result struct {
	Mesh *mesh.Mesh
	// AchievedDraft is the angle actually used. It differs from the requested
	// one when the profile was too tight to take it (SPEC-UX §9.3).
	AchievedDraft float64
	Clamped       bool
}

// Build constructs a closed solid from one or more regions.
//
// Regions that do not touch become separate shells of one mesh, which is legal
// (SPEC-GEOMETRY §6.5) and is what "extrude this whole selection" means.
func Build(regions []sketch2d.Region, p Params, bodyID uint32) (Result, error) {
	if len(regions) == 0 {
		return Result{}, fmt.Errorf("nothing selected to extrude")
	}
	if p.Depth <= 0 {
		return Result{}, fmt.Errorf("depth must be more than zero")
	}
	if math.Abs(p.Draft) > MaxDraftDegrees {
		return Result{}, fmt.Errorf("draft must be between %.0f and %.0f degrees",
			-MaxDraftDegrees, MaxDraftDegrees)
	}
	for i, r := range regions {
		if len(r.Outer.Pts) < 3 {
			return Result{}, fmt.Errorf("region %d is not a closed profile", i)
		}
		if r.Outer.Area2() <= 0 {
			return Result{}, fmt.Errorf("region %d has no area", i)
		}
	}
	if a, b, touch := touchingPair(regions); touch {
		return Result{}, fmt.Errorf(
			"regions %d and %d touch — extrude them one at a time, "+
				"or combine them once booleans arrive", a, b)
	}

	out := Result{Mesh: &mesh.Mesh{}, AchievedDraft: p.Draft}
	seq := uint32(0)

	// One clamp decision covers the whole selection, so a taper that one region
	// cannot take is not silently applied to its neighbours.
	rings, achieved, clamped := planRings(regions, p)
	out.AchievedDraft, out.Clamped = achieved, clamped

	for ri := range regions {
		buildShell(out.Mesh, rings[ri], p.Frame, bodyID, &seq)
	}

	mesh.Weld(out.Mesh)
	if err := mesh.Validate(out.Mesh); err != nil {
		return Result{}, fmt.Errorf("extrude produced an invalid solid: %w", err)
	}
	return out, nil
}

// touchingPair finds two selected regions that share any boundary point, and
// reports their indices.
//
// Each region becomes its own closed shell, and the shells are then welded into
// one mesh. Two regions that touch anywhere — a shared wall where a disc fills a
// ring, or a single corner where two squares meet — weld into edges belonging to
// four faces, which is not a manifold solid and cannot be made into one by
// extruding harder. Catching it here means the refusal can name the regions and
// say what to do, instead of the validator reporting a vertex number.
//
// The regions come from one arrangement, so a shared boundary is shared exactly:
// integer equality is the right test, and no tolerance belongs anywhere near it.
func touchingPair(regions []sketch2d.Region) (int, int, bool) {
	if len(regions) < 2 {
		return 0, 0, false
	}
	// owner maps a boundary point to the first region that claimed it.
	owner := make(map[geom.Vec2i]int)
	for i := range regions {
		for _, l := range allLoops(regions[i]) {
			for _, p := range l.Pts {
				if prev, seen := owner[p]; seen && prev != i {
					return prev, i, true
				}
				owner[p] = i
			}
		}
	}
	return 0, 0, false
}

// ring is one profile at one height along the extrusion.
type ring struct {
	region sketch2d.Region
	height float64 // world units along the frame normal
}

// planRings works out the stack of profiles each region passes through, and the
// draft angle they all share.
//
// A plain extrude has two rings: the profile and the tapered far cap. A
// symmetric drafted one has three, because it is widest at the sketch plane and
// tapers away in both directions (SPEC-GEOMETRY §5.4), which makes it two
// frusta back to back rather than one.
func planRings(regions []sketch2d.Region, p Params) ([][]ring, float64, bool) {
	depth := float64(p.Depth) / geom.Unit
	sign := 1.0
	if p.Dir == Reverse {
		sign = -1
	}

	// The offset distance is measured over the run each taper actually covers:
	// the full depth normally, half of it either side when symmetric.
	taperDepth := p.Depth
	if p.Dir == Symmetric {
		taperDepth = p.Depth / 2
	}
	want := sketch2d.DeltaForDraft(taperDepth, p.Draft)

	// Take the tightest clamp any region needs, so one selection extrudes as
	// one shape.
	delta := want
	for _, r := range regions {
		if delta == 0 {
			break
		}
		_, got := sketch2d.ClampOffsetRegion(r, delta)
		if absI(got) < absI(delta) {
			delta = got
		}
	}
	clamped := delta != want
	achieved := p.Draft
	if clamped {
		achieved = sketch2d.DraftForDelta(taperDepth, delta)
	}

	// Rings are always ordered along the frame normal, lowest first. The face
	// winding rule below depends on that: the first ring caps the solid looking
	// back down the normal, the last one looking along it. A reverse extrude is
	// then just the same construction with its rings the other way round.
	out := make([][]ring, len(regions))
	for i, r := range regions {
		switch {
		case p.Dir == Symmetric && delta != 0:
			capped, _ := sketch2d.OffsetRegion(r, delta)
			out[i] = []ring{
				{region: capped, height: -depth / 2},
				{region: r, height: 0},
				{region: capped, height: depth / 2},
			}
		case p.Dir == Symmetric:
			out[i] = []ring{
				{region: r, height: -depth / 2},
				{region: r, height: depth / 2},
			}
		case delta != 0:
			capped, _ := sketch2d.OffsetRegion(r, delta)
			out[i] = []ring{
				{region: r, height: 0},
				{region: capped, height: sign * depth},
			}
		default:
			out[i] = []ring{
				{region: r, height: 0},
				{region: r, height: sign * depth},
			}
		}
		if len(out[i]) > 1 && out[i][0].height > out[i][len(out[i])-1].height {
			reverseRings(out[i])
		}
	}
	return out, achieved, clamped
}

func reverseRings(r []ring) {
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
}

func absI(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// buildShell emits one closed solid from a stack of rings.
func buildShell(m *mesh.Mesh, rings []ring, frame geom.Frame, bodyID uint32, seq *uint32) {
	if len(rings) < 2 {
		return
	}
	// Lift every ring's every loop, remembering where each landed.
	type loopSpan struct{ start, count int }
	spans := make([][]loopSpan, len(rings))
	for ri, r := range rings {
		loops := allLoops(r.region)
		spans[ri] = make([]loopSpan, len(loops))
		for li, l := range loops {
			spans[ri][li] = loopSpan{start: len(m.Verts), count: len(l.Pts)}
			for _, p := range l.Pts {
				m.Verts = append(m.Verts, liftSnapped(frame, p, r.height))
			}
		}
	}

	next := func() mesh.FaceUID {
		*seq++
		return mesh.MakeFaceUID(bodyID, *seq)
	}

	// The near cap looks back down the extrusion, so its loops run the other
	// way round from the sketch; the far cap keeps the sketch's own winding.
	// Reversing every loop of a face together keeps holes opposite their outer.
	near := spans[0]
	nearLoops := make([][]int, len(near))
	for li, s := range near {
		nearLoops[li] = reverseIdx(indexRange(s.start, s.count))
	}
	m.Faces = append(m.Faces, mesh.Face{ID: next(), Loops: nearLoops})

	far := spans[len(spans)-1]
	farLoops := make([][]int, len(far))
	for li, s := range far {
		farLoops[li] = indexRange(s.start, s.count)
	}
	m.Faces = append(m.Faces, mesh.Face{ID: next(), Loops: farLoops})

	// Side walls, one quad per profile edge per band between consecutive rings.
	for band := 0; band+1 < len(rings); band++ {
		lo, hi := spans[band], spans[band+1]
		for li := range lo {
			n := lo[li].count
			for e := 0; e < n; e++ {
				a0 := lo[li].start + e
				a1 := lo[li].start + (e+1)%n
				b0 := hi[li].start + e
				b1 := hi[li].start + (e+1)%n
				// Walking the near edge in the loop's own direction and
				// returning along the far one puts the outward normal on the
				// material's outside — for holes too, because their loops run
				// the other way.
				m.Faces = append(m.Faces, mesh.Face{
					ID:    next(),
					Loops: [][]int{{a0, a1, b1, b0}},
				})
			}
		}
	}
}

// allLoops flattens a region into outer-then-holes order.
func allLoops(r sketch2d.Region) []sketch2d.Loop {
	out := make([]sketch2d.Loop, 0, 1+len(r.Holes))
	out = append(out, r.Outer)
	out = append(out, r.Holes...)
	return out
}

func indexRange(start, count int) []int {
	out := make([]int, count)
	for i := range out {
		out[i] = start + i
	}
	return out
}

func reverseIdx(s []int) []int {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// liftSnapped places a sketch point on the plane at a height, snapping the
// result back to the subunit lattice when the frame is axis aligned.
//
// On a default plane the lift is a pure axis mapping, so snapping costs nothing
// and keeps the snap-on-author promise exactly (SPEC-GEOMETRY §1.2, §5.1). On
// an arbitrary face frame the result is genuinely off-lattice and is left alone,
// because rounding it would be drift rather than precision.
func liftSnapped(f geom.Frame, p geom.Vec2i, height float64) geom.Vec3 {
	v := f.LiftSub(p, height)
	if axisAligned(f) {
		return geom.SnapVec3ToSubunits(v)
	}
	return v
}

// axisAligned reports whether a frame's axes all lie along world axes, which is
// true of every default plane and of any face parallel to one.
func axisAligned(f geom.Frame) bool {
	return isAxis(f.U) && isAxis(f.V) && isAxis(f.N)
}

func isAxis(v geom.Vec3) bool {
	near := func(a, b float64) bool { return math.Abs(a-b) < geom.NormalEps }
	ones := 0
	for _, c := range []float64{v.X, v.Y, v.Z} {
		switch {
		case near(c, 0):
		case near(math.Abs(c), 1):
			ones++
		default:
			return false
		}
	}
	return ones == 1
}
