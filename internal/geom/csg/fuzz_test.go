package csg

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// TESTING §3.5: two hundred random ops with an oracle that shares no code with
// the thing it is checking.
//
// The per-step check is cheap and catches drift: our reading of the solid has
// to agree with Manifold's. But both of those are downstream of the same
// boolean, so agreeing proves only that the converters are consistent, not that
// the solid is the one the ops asked for. That is what the voxel oracle is
// for — it replays the op history as pure arithmetic on points and asks the
// mesh, by ray casting, whether it agrees about what is solid.

// tool is one step of the fuzz history, kept in a form the oracle can evaluate
// without going anywhere near the mesh.
type tool struct {
	op Op
	// For a box: the local-space extent. For a prism: radius, segments and the
	// half-height along local Z.
	min, max geom.Vec3
	radius   float64
	segs     int
	// inv maps a world point into the tool's local space. Identity for a tool
	// that was never rotated.
	inv geom.Mat4
}

// contains answers "is this world point inside this tool", analytically.
func (t tool) contains(p geom.Vec3) bool {
	q := t.inv.TransformPoint(p)
	if q.Z < t.min.Z || q.Z > t.max.Z {
		return false
	}
	if t.segs == 0 {
		return q.X >= t.min.X && q.X <= t.max.X && q.Y >= t.min.Y && q.Y <= t.max.Y
	}
	// A regular polygon is the intersection of its edges' half-planes, and the
	// prism is that swept along Z.
	for i := 0; i < t.segs; i++ {
		a := 2 * math.Pi * float64(i) / float64(t.segs)
		b := 2 * math.Pi * float64(i+1) / float64(t.segs)
		ax, ay := t.radius*math.Cos(a), t.radius*math.Sin(a)
		bx, by := t.radius*math.Cos(b), t.radius*math.Sin(b)
		if (bx-ax)*(q.Y-ay)-(by-ay)*(q.X-ax) < 0 {
			return false
		}
	}
	return true
}

// insideHistory replays the whole op history for one point.
func insideHistory(history []tool, p geom.Vec3) bool {
	in := false
	for i, t := range history {
		hit := t.contains(p)
		switch {
		case i == 0:
			in = hit
		case t.op == Union:
			in = in || hit
		case t.op == Subtract:
			in = in && !hit
		case t.op == Intersect:
			in = in && hit
		}
	}
	return in
}

// occupancy counts how much of a grid over box the predicate calls solid, and
// converts that to a volume.
func occupancy(b geom.AABB, n int, inside func(geom.Vec3) bool) float64 {
	size := b.Max.Sub(b.Min)
	cell := geom.Vec3{X: size.X / float64(n), Y: size.Y / float64(n), Z: size.Z / float64(n)}
	hits := 0
	for iz := 0; iz < n; iz++ {
		for iy := 0; iy < n; iy++ {
			for ix := 0; ix < n; ix++ {
				p := geom.Vec3{
					X: b.Min.X + (float64(ix)+0.5)*cell.X,
					Y: b.Min.Y + (float64(iy)+0.5)*cell.Y,
					Z: b.Min.Z + (float64(iz)+0.5)*cell.Z,
				}
				if inside(p) {
					hits++
				}
			}
		}
	}
	return float64(hits) * cell.X * cell.Y * cell.Z
}

// meshOccupancy measures the mesh the same way, by ray casting.
//
// Deliberately naive and deliberately unrelated to anything in this package:
// for each row of the grid it fires one ray along +X, collects every triangle
// it crosses, and flips a parity bit at each crossing. Nothing here knows what
// a boolean is.
func meshOccupancy(m *mesh.Mesh, b geom.AABB, n int) float64 {
	tris := m.Triangulate()
	size := b.Max.Sub(b.Min)
	cell := geom.Vec3{X: size.X / float64(n), Y: size.Y / float64(n), Z: size.Z / float64(n)}

	hits := 0
	xs := make([]float64, 0, 32)
	for iz := 0; iz < n; iz++ {
		z := b.Min.Z + (float64(iz)+0.5)*cell.Z
		for iy := 0; iy < n; iy++ {
			y := b.Min.Y + (float64(iy)+0.5)*cell.Y
			xs = xs[:0]
			for _, t := range tris {
				if x, ok := rayCrossX(m.Verts[t.A], m.Verts[t.B], m.Verts[t.C], y, z); ok {
					xs = append(xs, x)
				}
			}
			if len(xs) == 0 {
				continue
			}
			for ix := 0; ix < n; ix++ {
				x := b.Min.X + (float64(ix)+0.5)*cell.X
				crossings := 0
				for _, cx := range xs {
					if cx > x {
						crossings++
					}
				}
				if crossings%2 == 1 {
					hits++
				}
			}
		}
	}
	return float64(hits) * cell.X * cell.Y * cell.Z
}

// rayCrossX intersects the +X ray through (y,z) with one triangle.
func rayCrossX(a, b, c geom.Vec3, y, z float64) (float64, bool) {
	// Barycentric solve in the YZ plane, then interpolate X.
	d := (b.Y-a.Y)*(c.Z-a.Z) - (c.Y-a.Y)*(b.Z-a.Z)
	if math.Abs(d) < 1e-12 {
		return 0, false
	}
	u := ((y-a.Y)*(c.Z-a.Z) - (c.Y-a.Y)*(z-a.Z)) / d
	v := ((b.Y-a.Y)*(z-a.Z) - (y-a.Y)*(b.Z-a.Z)) / d
	if u < 0 || v < 0 || u+v > 1 {
		return 0, false
	}
	return a.X + u*(b.X-a.X) + v*(c.X-a.X), true
}

// TestFuzzAgainstAnIndependentOracle is TESTING §3.5.
func TestFuzzAgainstAnIndependentOracle(t *testing.T) {
	if testing.Short() {
		t.Skip("the fuzz run is long; -short skips it")
	}
	const (
		seed      = 20260826
		steps     = 200
		voxelGrid = 96
		everyNth  = 20
	)
	rng := rand.New(rand.NewSource(seed))

	// Start from a plate big enough that later ops have somewhere to land.
	body := box(-8, -8, -2, 8, 8, 2, 1)
	history := []tool{{op: Union, min: geom.Vec3{X: -8, Y: -8, Z: -2},
		max: geom.Vec3{X: 8, Y: 8, Z: 2}, inv: geom.Identity()}}

	bodyID := uint32(2)
	for step := 1; step <= steps; step++ {
		op := []Op{Union, Subtract, Intersect}[rng.Intn(3)]
		// Intersect swallows everything far too easily to be worth a third of
		// the run; it gets a fair share of attention in the matrix.
		if op == Intersect && rng.Intn(4) != 0 {
			op = Union
		}
		m, tl := randomTool(rng, op, bodyID)
		bodyID++

		res, err := Boolean(op, body, m)
		if err != nil {
			t.Fatalf("seed %d step %d (%v): %v", seed, step, op, err)
		}
		if res.Empty {
			// A legal nothing ends the run: there is no body left to operate on.
			t.Logf("seed %d: step %d (%v) emptied the body; stopping", seed, step, op)
			return
		}
		if err := mesh.Validate(res.Mesh); err != nil {
			t.Fatalf("seed %d step %d (%v): invalid solid: %v", seed, step, op, err)
		}
		ours := mesh.Volume(res.Mesh)
		if rel(ours, res.Volume) > geom.VolumeRelTol {
			t.Fatalf("seed %d step %d (%v): our volume %.9f, Manifold's %.9f",
				seed, step, op, ours, res.Volume)
		}
		body = res.Mesh
		history = append(history, tl)

		if step%everyNth != 0 {
			continue
		}
		// The oracle: replay the history arithmetically and ray-cast the mesh,
		// then compare what each thinks is solid.
		b := body.AABB()
		pad := geom.Vec3{X: 0.5, Y: 0.5, Z: 0.5}
		b = geom.AABB{Min: b.Min.Sub(pad), Max: b.Max.Add(pad)}

		want := occupancy(b, voxelGrid, func(p geom.Vec3) bool {
			return insideHistory(history, p)
		})
		got := meshOccupancy(body, b, voxelGrid)
		if math.Abs(got-want) > 0.03*math.Max(want, 1) {
			t.Fatalf("seed %d step %d: the mesh encloses %.3f but the op history says %.3f",
				seed, step, got, want)
		}
	}
}

// randomTool builds one random cutter and the analytic description of it.
func randomTool(rng *rand.Rand, op Op, bodyID uint32) (*mesh.Mesh, tool) {
	// Grid-aligned sizes and positions, which is what a person draws.
	at := func(span float64) float64 { return math.Round(rng.Float64()*span*2 - span) }
	sz := func() float64 { return 1 + math.Round(rng.Float64()*5) }

	cx, cy, cz := at(7), at(7), at(4)
	if rng.Intn(6) == 0 {
		// A prism now and then, so round cuts are in the mix too.
		r := sz()
		segs := []int{6, 8, 16}[rng.Intn(3)]
		h := sz() * 2
		f := geom.Frame{
			O: geom.Vec3{X: cx, Y: cy, Z: cz - h/2},
			U: geom.AxisX, V: geom.AxisY, N: geom.AxisZ,
		}
		m := mesh.NGonPrism(f, r, segs, h, bodyID)
		inv := geom.Translate(geom.Vec3{X: -cx, Y: -cy, Z: -cz})
		return m, tool{
			op: op, radius: r, segs: segs, inv: inv,
			min: geom.Vec3{Z: -h / 2}, max: geom.Vec3{Z: h / 2},
		}
	}

	hx, hy, hz := sz(), sz(), sz()
	lo := geom.Vec3{X: -hx, Y: -hy, Z: -hz}
	hi := geom.Vec3{X: hx, Y: hy, Z: hz}
	m := mesh.Box(lo, hi, bodyID)

	xform := geom.Translate(geom.Vec3{X: cx, Y: cy, Z: cz})
	if rng.Intn(8) == 0 {
		// Occasionally off the grid entirely, which is where a kernel that
		// leans on axis alignment falls over.
		rot := geom.RotateY(rng.Float64() * math.Pi).
			Mul(geom.RotateX(rng.Float64() * math.Pi))
		xform = xform.Mul(rot)
	}
	mesh.Transform(m, xform)
	inv, ok := xform.Invert()
	if !ok {
		inv = geom.Identity()
	}
	return m, tool{op: op, min: lo, max: hi, inv: inv}
}

// TestPaintSurvivesACut is TESTING §3.4: a face's picture has to outlive an
// edit that cuts the face into pieces, or painting is worthless the moment you
// change your mind about a window.
func TestPaintSurvivesACut(t *testing.T) {
	plate := box(-5, -5, 0, 5, 5, 2, 1)

	top := -1
	for i := range plate.Faces {
		if plate.FaceNormal(i).Dot(geom.AxisZ) > 0.99 {
			top = i
		}
	}
	if top < 0 {
		t.Fatal("the plate has no top face")
	}
	topUID := plate.Faces[top].ID

	// A 32-texel checkerboard anchored to that face.
	const res = 32
	img := image.NewRGBA(image.Rect(0, 0, res, res))
	for y := 0; y < res; y++ {
		for x := 0; x < res; x++ {
			c := color.RGBA{R: 20, G: 20, B: 20, A: 255}
			if (x+y)%2 == 0 {
				c = color.RGBA{R: 220, G: 180, B: 60, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	paint := &mesh.FacePaint{
		Res:   res,
		Texel: 10.0 / res,
		Frame: plate.FaceFrame(top),
		Img:   img,
		Off:   image.Point{X: -res / 2, Y: -res / 2},
	}
	plate.Faces[top].Paint = paint

	// Probe points on the top face, well away from where the hole will go.
	probes := []geom.Vec3{
		{X: -4, Y: -4, Z: 2}, {X: 4, Y: -4, Z: 2},
		{X: -4, Y: 4, Z: 2}, {X: 4, Y: 4, Z: 2},
		{X: -3.5, Y: 0.5, Z: 2},
	}
	before := make([]color.RGBA, len(probes))
	for i, p := range probes {
		before[i] = sampleAt(paint, p)
	}

	res2, err := Boolean(Subtract, plate, box(-1, -1, -1, 1, 1, 3, 2))
	if err != nil {
		t.Fatalf("Subtract: %v", err)
	}
	if err := mesh.Validate(res2.Mesh); err != nil {
		t.Fatalf("invalid solid: %v", err)
	}

	// The fragments of the painted face still carry the same picture, by
	// identity: they share the FacePaint, they do not each get a copy.
	var painted int
	for i := range res2.Mesh.Faces {
		f := &res2.Mesh.Faces[i]
		if f.SrcFace != topUID {
			continue
		}
		painted++
		if f.Paint != paint {
			t.Errorf("a fragment of the painted face carries a different FacePaint")
		}
	}
	if painted == 0 {
		t.Fatal("no surviving fragment traces back to the painted face")
	}

	// And the picture reads the same at every probe: the cut moved geometry,
	// not paint.
	for i, p := range probes {
		if got := sampleAt(paint, p); got != before[i] {
			t.Errorf("probe %v changed from %v to %v", p, before[i], got)
		}
	}
}

// sampleAt reads the paint colour at a world point, which is the projection of
// SPEC-GEOMETRY §8: into the anchor frame, divided by the texel size, offset.
func sampleAt(p *mesh.FacePaint, world geom.Vec3) color.RGBA {
	uv := p.Frame.ToLocal(world)
	x := int(math.Floor(uv.X/p.Texel)) - p.Off.X
	y := int(math.Floor(uv.Y/p.Texel)) - p.Off.Y
	if x < 0 || y < 0 || x >= p.Img.Bounds().Dx() || y >= p.Img.Bounds().Dy() {
		return color.RGBA{}
	}
	return p.Img.RGBAAt(x, y)
}
