package model

import (
	"fmt"
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

type MaterialIssue struct {
	Body      uint32       `json:"body"`
	Face      mesh.FaceUID `json:"face"`
	FaceIndex int          `json:"faceIndex"`
	Kind      string       `json:"kind"`
	Message   string       `json:"message"`
}

func MaterialHealth(d *Document) []MaterialIssue {
	issues := []MaterialIssue{}
	for _, b := range d.Bodies {
		if b.Mesh == nil {
			continue
		}
		for fi, f := range b.Mesh.Faces {
			add := func(kind, msg string) {
				issues = append(issues, MaterialIssue{b.ID, f.ID, fi, kind, fmt.Sprintf("%s · face %d: %s", b.Name, fi, msg)})
			}
			p := f.Paint
			if p == nil || p.Img == nil {
				add("unpainted", "unpainted face")
				continue
			}
			if p.Res != 32 || math.Abs(p.Texel-1.0/32) > 1e-10 {
				add("pixel_resolution", fmt.Sprintf("%d px/u; expected 32 px/u", p.Res))
			}
			if p.PBRStale {
				add("stale_pbr", "base color changed; regenerate PBR")
			}
			for _, k := range mesh.MaterialChannels[1:] {
				if p.Material == nil || p.Material.Maps[k] == nil || p.Material.Maps[k].Image == nil {
					add("missing_pbr", "missing "+k)
					continue
				}
				m := p.Material.Maps[k]
				if m.Bounds.Empty() || m.Image.Rect.Dx() != m.Bounds.Dx() || m.Image.Rect.Dy() != m.Bounds.Dy() {
					add("dimensions", k+" dimensions do not match the texel grid")
				}
			}
			// Only sample inside the polygon; allocation margins are intentionally clear.
			rect := p.MaterialBounds(b.Mesh, fi).Sub(p.Off)
			bare := false
			for y := rect.Min.Y; y < rect.Max.Y && !bare; y++ {
				for x := rect.Min.X; x < rect.Max.X; x++ {
					if p.Img.RGBAAt(x, y).A == 0 {
						if insideFaceUV(b.Mesh, fi, p, geom.Vec2{X: float64(x+p.Off.X) + .5, Y: float64(y+p.Off.Y) + .5}) {
							bare = true
							break
						}
					}
				}
			}
			if bare {
				add("coverage", "texture includes transparent / unpainted pixels")
			}
		}
	}
	return issues
}

func insideFaceUV(m *mesh.Mesh, fi int, p *mesh.FacePaint, q geom.Vec2) bool {
	inside := false
	for _, loop := range m.Faces[fi].Loops {
		for i, vi := range loop {
			a := p.UV(m.Verts[vi])
			b := p.UV(m.Verts[loop[(i+1)%len(loop)]])
			if (a.Y > q.Y) != (b.Y > q.Y) && q.X < (b.X-a.X)*(q.Y-a.Y)/(b.Y-a.Y)+a.X {
				inside = !inside
			}
		}
	}
	return inside
}
