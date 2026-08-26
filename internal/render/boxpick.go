package render

import (
	"image/color"
	"sort"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
)

// Box select (SPEC-RENDER §6.2).
//
// One full-viewport ID render on drag release, then a scan of the rectangle.
// The cursor pick's 64x64 keyhole cannot answer "what is inside this box" —
// it only ever sees a few pixels around the pointer — so this is a second,
// bigger render that happens once per box, not once per frame.

// BoxRTScale is how much the box-select render target is shrunk relative to the
// viewport. Half resolution is four times less to read back and still resolves
// anything a person can deliberately drag a box around.
const BoxRTScale = 2

// BoxRect is a selection rectangle in window pixels.
type BoxRect struct{ X0, Y0, X1, Y1 float64 }

// Normalized returns the rectangle with its corners in order.
func (b BoxRect) Normalized() BoxRect {
	if b.X0 > b.X1 {
		b.X0, b.X1 = b.X1, b.X0
	}
	if b.Y0 > b.Y1 {
		b.Y0, b.Y1 = b.Y1, b.Y0
	}
	return b
}

// Width and Height are the rectangle's size.
func (b BoxRect) Width() float64  { n := b.Normalized(); return n.X1 - n.X0 }
func (b BoxRect) Height() float64 { n := b.Normalized(); return n.Y1 - n.Y0 }

// BoxPick renders the scene's IDs across the whole viewport and reports what
// the rectangle caught.
//
// The membership rule differs by kind, as SPEC-RENDER §6.2 sets out. A vertex
// or an edge counts if any of its pixels are inside — they are small, and
// requiring every pixel would make them nearly unselectable. A face counts only
// if none of its pixels are outside, which is the practical approximation of
// "fully inside": it is the difference between dragging a box over the nose of
// a hull and selecting the whole hull because one of its faces peeked in.
func (r *Renderer) BoxPick(s *Scene, vp Viewport, rect BoxRect, want PickKind) []PickRef {
	rect = rect.Normalized()
	if vp.W <= 0 || vp.H <= 0 || rect.Width() < 1 || rect.Height() < 1 {
		return nil
	}
	// Vertices and edges are answered by projecting them, not by rendering
	// them. See boxPickPoints for why.
	if want == PickVert || want == PickEdge {
		return boxPickPoints(s, vp, rect, want)
	}
	w := vp.W / BoxRTScale
	h := vp.H / BoxRTScale
	if w < 1 || h < 1 {
		return nil
	}
	r.ensureBoxRT(w, h)
	r.Table.Reset()

	rl.BeginTextureMode(r.boxRT)
	rl.ClearBackground(color.RGBA{A: 255})
	rl.DrawRenderBatchActive()

	rl.Viewport(0, 0, int32(w), int32(h))
	savedProj := rl.GetMatrixProjection()
	savedView := rl.GetMatrixModelview()
	rl.SetMatrixProjection(toRLMatrix(s.Camera.Proj(vp.Aspect())))
	rl.SetMatrixModelview(toRLMatrix(s.Camera.View()))

	rl.EnableDepthTest()
	rl.DisableColorBlend()
	rl.SetTexture(r.whiteTex.ID)

	// Only faces reach here, and they keep the depth test: a face you cannot
	// see is not one you meant to put a box around.
	r.pickBodies(s)

	rl.SetTexture(0)
	rl.DrawRenderBatchActive()
	rl.EnableColorBlend()
	rl.DisableDepthTest()
	rl.SetMatrixProjection(savedProj)
	rl.SetMatrixModelview(savedView)
	rl.EndTextureMode()
	rl.Viewport(0, 0, int32(r.fbW), int32(r.fbH))

	return r.scanBox(vp, rect, want, w, h)
}

// ensureBoxRT allocates or resizes the box-select target.
func (r *Renderer) ensureBoxRT(w, h int) {
	if r.boxReady && r.boxW == w && r.boxH == h {
		return
	}
	if r.boxReady {
		rl.UnloadRenderTexture(r.boxRT)
	}
	r.boxRT = rl.LoadRenderTexture(int32(w), int32(h))
	r.boxW, r.boxH, r.boxReady = w, h, true
}

// scanBox reads the ID buffer back and applies the per-kind membership rule.
func (r *Renderer) scanBox(vp Viewport, rect BoxRect, want PickKind, w, h int) []PickRef {
	img := rl.LoadImageFromTexture(r.boxRT.Texture)
	if img == nil {
		return nil
	}
	defer rl.UnloadImage(img)
	pixels := rl.LoadImageColors(img)
	if len(pixels) < w*h {
		return nil
	}

	// The rectangle is in window pixels; the target is viewport-local and
	// scaled down.
	x0 := int((rect.X0 - float64(vp.X)) / BoxRTScale)
	x1 := int((rect.X1 - float64(vp.X)) / BoxRTScale)
	y0 := int((rect.Y0 - float64(vp.Y)) / BoxRTScale)
	y1 := int((rect.Y1 - float64(vp.Y)) / BoxRTScale)

	inside := map[int]int{}
	outside := map[int]bool{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			id := decodeID(pixels[y*w+x])
			if id == 0 {
				continue
			}
			ref, ok := r.Table.Get(id)
			if !ok || ref.Kind != want {
				continue
			}
			if x >= x0 && x <= x1 && y >= y0 && y <= y1 {
				inside[id]++
			} else {
				outside[id] = true
			}
		}
	}

	ids := make([]int, 0, len(inside))
	for id := range inside {
		if want == PickFace && outside[id] {
			continue // not fully inside
		}
		ids = append(ids, id)
	}
	// A stable order makes the resulting selection, and anything built from it,
	// the same every time.
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]PickRef, 0, len(ids))
	for _, id := range ids {
		if ref, ok := r.Table.Get(id); ok {
			out = append(out, ref)
		}
	}
	return out
}

// boxPickPoints collects vertices or edges inside the rectangle by projecting
// them, without rendering anything.
//
// The ID-render approach cannot do this job. Two vertices at the same screen
// position — the near and far corners of a hull seen straight on, which is
// exactly the view somebody stretches a nose in — occupy one pixel, and one
// pixel holds one id. Whichever drew last wins and the other is invisible to
// the scan, so half the end of the hull silently stays put and the next drag
// tears it in two.
//
// Projecting instead is exact, has no occlusion to reason about, no readback,
// and is faster than the render it replaces. A face still needs the ID buffer,
// because "is this region inside the rectangle" is a question about area and
// there is no comparably cheap answer for it.
func boxPickPoints(s *Scene, vp Viewport, rect BoxRect, want PickKind) []PickRef {
	inRect := func(p geom.Vec2) bool {
		return p.X >= rect.X0 && p.X <= rect.X1 && p.Y >= rect.Y0 && p.Y <= rect.Y1
	}
	project := func(world geom.Vec3, m geom.Mat4) (geom.Vec2, bool) {
		p, ok := s.Camera.WorldToViewport(m.TransformPoint(world),
			float64(vp.W), float64(vp.H))
		if !ok {
			return geom.Vec2{}, false
		}
		return geom.Vec2{X: p.X + float64(vp.X), Y: p.Y + float64(vp.Y)}, true
	}

	var out []PickRef
	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || !b.Pickable {
			continue
		}
		if want == PickVert {
			for vi, v := range b.GPU.Verts {
				if p, ok := project(v, b.Transform); ok && inRect(p) {
					out = append(out, PickRef{Kind: PickVert, BodyID: b.BodyID, Vert: vi})
				}
			}
			continue
		}
		for _, e := range b.GPU.Edges {
			a, okA := project(e.A, b.Transform)
			z, okB := project(e.B, b.Transform)
			// Both ends inside: an edge half in the box is not in the box, and
			// the alternative is a drag that grabs geometry outside the
			// rectangle the user drew.
			if okA && okB && inRect(a) && inRect(z) {
				out = append(out, PickRef{Kind: PickEdge, BodyID: b.BodyID, Edge: e.Index})
			}
		}
	}
	return out
}
