package mesh

import (
	"testing"

	"modeler/internal/geom"
)

func TestSharedPaintMovesOnceAndLeavesCloneSourceAlone(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		src := unitBox()
		p := &FacePaint{Frame: src.FaceFrame(0), Texel: .25}
		// Split/boolean faces can share one paint allocation.
		src.Faces[0].Paint, src.Faces[1].Paint = p, p
		original := p.Frame
		copy := src.Clone()
		delta := geom.Vec3{X: 3, Y: -2, Z: 5}
		want := original.Translated(delta)
		if rotate {
			x := geom.Translate(delta).Mul(geom.RotateY(.7))
			Transform(copy, x)
			want = original.Transformed(x)
		} else {
			Translate(copy, delta)
		}
		if p.Frame != original {
			t.Fatal("transforming clone changed the source paint")
		}
		if copy.Faces[0].Paint.Frame != want || copy.Faces[1].Paint != copy.Faces[0].Paint {
			t.Fatal("shared paint was moved more than once or lost sharing")
		}
	}
}
