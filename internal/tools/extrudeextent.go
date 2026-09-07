package tools

import (
	"fmt"
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/extrude"
)

type Extent uint8

const (
	ExtentDistance Extent = iota
	ExtentFace
	ExtentVertex
	ExtentEdge
)

func (e Extent) TargetName() string {
	switch e {
	case ExtentFace:
		return "face"
	case ExtentVertex:
		return "vertex"
	case ExtentEdge:
		return "edge"
	}
	return "distance"
}
func (t *ExtrudeTool) SetExtent(e Extent) {
	t.EndDrag()
	t.Extent = e
	t.TargetReady = false
	t.TargetPoint, t.TargetNormal = geom.Vec3{}, geom.Vec3{}
	if e != ExtentDistance {
		t.ThroughAll = false
		t.Dir = extrude.Normal
	}
}
func (t *ExtrudeTool) SetTarget(point, normal geom.Vec3) error {
	if t.Extent == ExtentDistance {
		return fmt.Errorf("choose an up-to mode first")
	}
	if t.Extent != ExtentFace {
		normal = t.Axis
	}
	den := normal.Dot(t.Axis)
	if math.Abs(den) < 1e-8 {
		return fmt.Errorf("choose a face that crosses the extrusion direction")
	}
	depth := normal.Dot(point.Sub(t.Origin)) / den
	if math.IsNaN(depth) || math.IsInf(depth, 0) || math.Abs(depth) < 1.0/geom.Unit || math.Abs(depth) > MaxDepthUnits {
		return fmt.Errorf("target must be away from the sketch plane and within %g u", MaxDepthUnits)
	}
	t.DepthUnits = depth
	t.Dir = extrude.Normal
	t.ThroughAll = false
	t.TargetPoint, t.TargetNormal = point, normal
	t.TargetReady = true
	return nil
}
