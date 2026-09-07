package extrude

import (
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
	"testing"
)

func TestUpToSlopedPlane(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(-2, -2, 2, 2)}
	for _, dir := range []Direction{Normal, Reverse} {
		sign := 1.0
		if dir == Reverse {
			sign = -1
		}
		p := Params{Frame: frontFrame(), Depth: u(4), Dir: dir, EndPlane: true, EndPoint: geom.Vec3{Z: sign * 4}, EndNormal: geom.Vec3{X: -.25, Z: sign}}
		for _, draft := range []float64{0, 10} {
			p.Draft = draft
			got := build(t, r, p)
			for _, v := range got.Mesh.Verts {
				if math.Abs(v.Z) > 1e-8 && math.Abs(p.EndNormal.Dot(v.Sub(p.EndPoint))) > 1e-8 {
					t.Fatalf("vertex missed target plane: %v", v)
				}
			}
			if draft == 0 && math.Abs(mesh.Volume(got.Mesh)-64) > 1e-7 {
				t.Fatalf("sloped volume=%v", mesh.Volume(got.Mesh))
			}
		}
	}
	// A hole stays open all the way to the sloped cap.
	r.Holes = []sketch2d.Loop{reversed(rectLoop(-1, -1, 1, 1))}
	got := build(t, r, Params{Frame: frontFrame(), Depth: u(4), EndPlane: true, EndPoint: geom.Vec3{Z: 4}, EndNormal: geom.Vec3{X: -.25, Z: 1}})
	if math.Abs(mesh.Volume(got.Mesh)-48) > 1e-7 {
		t.Fatalf("hole volume=%v", mesh.Volume(got.Mesh))
	}
}
func TestUpToPlaneRejectsCrossingOrParallel(t *testing.T) {
	r := sketch2d.Region{Outer: rectLoop(-2, -2, 2, 2)}
	for _, normal := range []geom.Vec3{{X: 1}, {X: 1, Z: 1}} {
		_, err := Build([]sketch2d.Region{r}, Params{Frame: frontFrame(), Depth: u(4), EndPlane: true, EndPoint: geom.Vec3{Z: 1}, EndNormal: normal}, 1)
		if err == nil {
			t.Fatal("invalid target accepted", normal)
		}
	}
}
