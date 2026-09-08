package model

import (
	"image"
	"math"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

func TestBodyTransformsCarryPaintAndRestoreHistory(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		doc, b, _ := boxDoc(t)
		for i := range b.Mesh.Faces {
			b.Mesh.Faces[i].Paint = &mesh.FacePaint{Frame: b.Mesh.FaceFrame(i), Texel: .125,
				Img: image.NewRGBA(image.Rect(0, 0, 4, 4)), Off: image.Pt(-1, -2)}
		}
		before := b.Mesh.Clone()
		bus := NewBus(doc)
		var sel Selection
		sel.Set(BodyRef(b.ID))
		verts := sel.VertIndices(doc)
		// UpdateDrag replaces the previous preview by undoing and replaying;
		// texture anchors must follow exactly once, never accumulate the deltas.
		for _, amount := range []float64{1, 2, -3, 90, 37} {
			var cmd Command
			var transform geom.Mat4
			if rotate {
				pivot := geom.Vec3{X: 2, Y: 1, Z: -3}
				cmd = NewRotateVerts(verts, pivot, geom.AxisY, amount, "body")
				transform = geom.Translate(pivot).Mul(geom.RotateY(amount * math.Pi / 180)).Mul(geom.Translate(pivot.Neg()))
			} else {
				delta := geom.Vec3{X: amount, Y: amount / 2, Z: -amount}
				cmd = NewMoveVerts(verts, delta, "body")
				transform = geom.Translate(delta)
			}
			if err := bus.UpdateDrag(cmd); err != nil {
				t.Fatal(err)
			}
			for fi, f := range before.Faces {
				p := b.Mesh.Faces[fi].Paint
				for _, vi := range f.Outer() {
					want, got := f.Paint.UV(before.Verts[vi]), p.UV(b.Mesh.Verts[vi])
					if math.Hypot(want.X-got.X, want.Y-got.Y) > 1e-8 {
						t.Fatalf("rotate=%t amount=%g face=%d: texture slipped: %v -> %v", rotate, amount, fi, want, got)
					}
				}
				probe := geom.Vec2{X: .3, Y: .7}
				if !p.Frame.ToWorld(probe).NearEq(transform.TransformPoint(f.Paint.Frame.ToWorld(probe)), 1e-9) {
					t.Fatal("paint frame failed to follow the same rigid transform")
				}
				if p == f.Paint || p.Img != f.Paint.Img || p.Off != f.Paint.Off || p.Texel != f.Paint.Texel {
					t.Fatal("transform must copy the anchor and preserve pixels/density/offset")
				}
			}
		}
		after := b.Mesh.Clone()
		bus.CommitDrag()
		bus.Undo()
		for fi := range before.Faces {
			if b.Mesh.Faces[fi].Paint != before.Faces[fi].Paint {
				t.Fatal("undo did not restore the exact original paint anchor")
			}
		}
		bus.Redo()
		for fi := range after.Faces {
			if b.Mesh.Faces[fi].Paint.Frame != after.Faces[fi].Paint.Frame {
				t.Fatal("redo changed the paint anchor")
			}
		}
		bus.UpdateDrag(NewMoveVerts(verts, geom.Vec3{X: 8}, "body"))
		bus.CancelDrag()
		for fi := range after.Faces {
			if b.Mesh.Faces[fi].Paint.Frame != after.Faces[fi].Paint.Frame {
				t.Fatal("cancel left the paint displaced")
			}
		}
	}
}
