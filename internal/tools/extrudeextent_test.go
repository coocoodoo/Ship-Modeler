package tools

import (
	"modeler/internal/geom"
	"modeler/internal/geom/extrude"
	"testing"
)

func TestExtrudeExtentsRequireTargetAndResolveDirection(t *testing.T) {
	for _, e := range []Extent{ExtentFace, ExtentVertex, ExtentEdge} {
		tool := NewExtrudeTool(1, []int{0}, geom.Vec3{}, geom.Vec3{Z: 1})
		tool.SetThroughAll(true)
		tool.SetExtent(e)
		if valid, _ := tool.Valid(); valid || tool.ThroughAll {
			t.Fatal("missing target was valid")
		}
		if err := tool.SetTarget(geom.Vec3{X: 2, Y: 7, Z: -3.123}, geom.Vec3{Z: 1}); err != nil {
			t.Fatal(err)
		}
		p := tool.BuildParams(geom.PlaneFrame(geom.PlaneFront))
		if valid, _ := tool.Valid(); !valid || p.Dir != extrude.Reverse || !p.EndPlane || p.EndPoint.Z != -3.123 {
			t.Fatalf("bad extent %+v", p)
		}
		tool.OnFace = true
		tool.SetResult(ResultAdd)
		if tool.EffectiveDir() != extrude.Reverse {
			t.Fatal("result selection changed target direction")
		}
		tool.SetExtent(ExtentDistance)
		if tool.BuildParams(geom.PlaneFrame(geom.PlaneFront)).EndPlane {
			t.Fatal("regular extrusion kept target plane")
		}
	}
}
