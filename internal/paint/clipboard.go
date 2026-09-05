package paint

import (
	"image"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// CopyFacePixels takes a detached snapshot of painted pixels. Hidden texture
// outside the face's polygon (including holes left by cuts) is excluded.
// A pixel belongs to the face when its centre is inside the polygon.
func CopyFacePixels(m *mesh.Mesh, fi int, p *mesh.FacePaint, rect image.Rectangle) *image.RGBA {
	rect = rect.Intersect(FaceRect(m, fi, p))
	if rect.Empty() {
		return nil
	}
	loops := make([][]geom.Vec2, len(m.Faces[fi].Loops))
	for i, loop := range m.Faces[fi].Loops {
		for _, vi := range loop {
			loops[i] = append(loops[i], UV(p, m.Verts[vi]))
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	painted := false
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			px := At(p, image.Pt(x, y))
			if px.A == 0 {
				continue
			}
			inside := false
			q := geom.Vec2{X: float64(x) + 0.5, Y: float64(y) + 0.5}
			for _, loop := range loops {
				for i, a := range loop {
					b := loop[(i+1)%len(loop)]
					if (a.Y > q.Y) != (b.Y > q.Y) && q.X < (b.X-a.X)*(q.Y-a.Y)/(b.Y-a.Y)+a.X {
						inside = !inside
					}
				}
			}
			if inside {
				out.SetRGBA(x-rect.Min.X, y-rect.Min.Y, px)
				painted = true
			}
		}
	}
	if !painted {
		return nil
	}
	return out
}
