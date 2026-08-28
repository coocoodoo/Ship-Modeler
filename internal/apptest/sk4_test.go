package apptest

import (
	"math"
	"testing"
)

// SK4: splines and beziers (Sketch_func.md §5 SK4).

func TestGoldenSplineProfile(t *testing.T) {
	_, outDir := runScript(t, "sk4_spline")
	checkGolden(t, "sk4_spline", outDir)
}

// pt is a plain 2D point for the ground-truth maths below.
type pt struct{ X, Y float64 }

// shoelace is the area of a closed polygon.
func shoelace(p []pt) float64 {
	var a float64
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return math.Abs(a) / 2
}

// The two curves the script draws, sampled densely from their textbook
// definitions rather than from the program's own code. That independence is
// the point: a spline that interpolated the wrong control points, or a bezier
// that read its handles in the wrong order, would still tessellate into
// something closed and plausible — and only a number derived from the
// definition catches it.

// catmullRomDense samples a closed Catmull-Rom through its points.
func catmullRomDense(through []pt, steps int) []pt {
	n := len(through)
	at := func(i int) pt { return through[((i%n)+n)%n] }
	var out []pt
	for i := 0; i < n; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		for k := 0; k < steps; k++ {
			t := float64(k) / float64(steps)
			t2, t3 := t*t, t*t*t
			ax := func(a, b, c, d float64) float64 {
				return 0.5 * (2*b + (-a+c)*t +
					(2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
			}
			out = append(out, pt{
				X: ax(p0.X, p1.X, p2.X, p3.X),
				Y: ax(p0.Y, p1.Y, p2.Y, p3.Y),
			})
		}
	}
	return out
}

// cubicDense samples one cubic bezier.
func cubicDense(p0, p1, p2, p3 pt, steps int) []pt {
	out := make([]pt, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		u := 1 - t
		a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		out = append(out, pt{
			X: a*p0.X + b*p1.X + c*p2.X + d*p3.X,
			Y: a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y,
		})
	}
	return out
}

// The acceptance: a closed spline hull and a bezier canopy, both extruded.
//
// The volume is checked against the curves' own definitions, sampled two
// hundred times more finely than the program draws them. The program's coarser
// tessellation sits slightly inside a convex curve, so it is expected to come
// in a little under — but only a little, and never over.
func TestSplineAndBezierExtrudeToTheirTrueSize(t *testing.T) {
	stdout, _ := runScript(t, "sk4_spline")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	body := dumps[1].body(t, "Body 4")

	hull := shoelace(catmullRomDense([]pt{
		{-9, 0}, {-5, 2.5}, {1, 3}, {7, 1.5}, {9, 0}, {7, -1.5}, {1, -3}, {-5, -2.5},
	}, 400))
	// The canopy is the bezier closed by the straight line back to its start.
	canopy := shoelace(cubicDense(pt{-9, 4}, pt{-4, 7}, pt{4, 7}, pt{9, 4}, 2000))

	const depth = 3.0
	want := (hull + canopy) * depth

	if body.vol > want {
		t.Errorf("extruded volume = %.4f, more than the true %.4f — "+
			"a tessellation inside a curve cannot enclose more than the curve",
			body.vol, want)
	}
	if d := (want - body.vol) / want; d > 0.03 {
		t.Errorf("extruded volume = %.4f, %.1f%% under the true %.4f\n"+
			"  hull %.4f + canopy %.4f, %v deep",
			body.vol, d*100, want, hull, canopy, depth)
	}
}

// The SK4 contract, and the reason curves are worth having: a closed spline is
// a region on its own, and an open bezier closes one with a line drawn to its
// ends. Both need their tessellations to land exactly on the placed points.
func TestCurvesCloseProfiles(t *testing.T) {
	stdout, _ := runScript(t, "sk4_spline")
	dumps := parseSketchDumps(t, stdout)
	if len(dumps) == 0 || !dumps[0].present {
		t.Fatalf("expected an active sketch in the first dump:\n%s", stdout)
	}
	s := dumps[0]

	if s.entities != 3 {
		t.Errorf("the sketch holds %d entities, want 3", s.entities)
	}
	if s.regions != 2 {
		t.Errorf("the region engine found %d regions, want 2 — "+
			"the closed spline, and the bezier closed by its line", s.regions)
	}
	if s.openEnds != 0 {
		t.Errorf("the sketch has %d open ends — a curve did not land on its endpoints",
			s.openEnds)
	}
}
