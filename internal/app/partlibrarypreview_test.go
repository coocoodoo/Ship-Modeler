package app

import (
	"testing"

	"modeler/internal/geom"
)

func TestLibraryPreviewFitsEntireOrbit(t *testing.T) {
	for _, box := range []geom.AABB{
		{Min: geom.Vec3{X: 100, Y: -20, Z: 50}, Max: geom.Vec3{X: 140, Y: -19, Z: 52}},
		{Min: geom.Vec3{X: -1, Y: -30, Z: -4}, Max: geom.Vec3{X: 1, Y: 30, Z: 4}},
	} {
		for _, aspect := range []float64{0.5, 1, 2.5} {
			for angle := 0.0; angle < 720; angle += 5 {
				cam := libraryPreviewCamera(box, aspect, angle)
				for _, corner := range box.Corners() {
					p, ok := cam.WorldToViewport(corner, 200*aspect, 200)
					if !ok || p.X < 0 || p.X > 200*aspect || p.Y < 0 || p.Y > 200 {
						t.Fatalf("preview clipped at angle %g, aspect %g: %v", angle, aspect, p)
					}
				}
			}
		}
	}
}
