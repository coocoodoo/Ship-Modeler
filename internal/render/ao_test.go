package render

import (
	"modeler/internal/geom"
	"testing"
)

func TestScreenSpaceRadiusTracksModelNotCamera(t *testing.T) {
	s := &Scene{Bodies: []BodyDraw{{GPU: &BodyGPU{Bounds: geom.AABB{Min: geom.Vec3{}, Max: geom.Vec3{X: 10, Y: 10, Z: 10}}}, Transform: geom.Identity()}}}
	radius := aoSceneRadius(s)
	s.Camera.OrthoScale = 1000
	if aoSceneRadius(s) != radius {
		t.Fatal("zoom changed the contact-shadow radius")
	}
}
