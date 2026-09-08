package app

import (
	"fmt"
	"image"
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/ui"
)

// Paint allocation and Undo emit geometry notifications too. Compare the
// projected contours before rebuilding, so a first dab cannot move the sheet.
func (a *App) uvLayoutMatches(b *model.Body) bool {
	if len(b.Mesh.Faces) != len(a.uv.islands) {
		return false
	}
	for _, is := range a.uv.islands {
		f := b.Mesh.Faces[is.index]
		p := f.Paint
		if p == nil {
			p = is.paint
		}
		if f.ID != is.face || p == nil || p.Texel != is.paint.Texel || len(f.Loops) != len(is.loops) {
			return false
		}
		for li, loop := range f.Loops {
			if len(loop) != len(is.loops[li]) {
				return false
			}
			for j, vi := range loop {
				u, v := p.UV(b.Mesh.Verts[vi]), is.loops[li][j]
				if math.Hypot(u.X-v.X, u.Y-v.Y) > 1e-6 {
					return false
				}
			}
		}
	}
	return true
}

func (a *App) dropUVTextures() {
	for _, tex := range a.uv.textures {
		if tex.ID != 0 {
			rl.UnloadTexture(tex)
		}
	}
	a.uv.textures, a.uv.textureDirty = nil, nil
}

func (a *App) uvTexture(img *image.RGBA) rl.Texture2D {
	if a.uv.textures == nil {
		a.uv.textures = map[*image.RGBA]rl.Texture2D{}
		a.uv.textureDirty = map[*image.RGBA]bool{}
	}
	tex, ok := a.uv.textures[img]
	if !ok {
		ri := rl.NewImageFromImage(img)
		tex = rl.LoadTextureFromImage(ri)
		rl.UnloadImage(ri)
		rl.SetTextureFilter(tex, rl.FilterPoint)
		a.uv.textures[img] = tex
	} else if a.uv.textureDirty[img] {
		rl.UpdateTexture(tex, img.Pix)
	}
	delete(a.uv.textureDirty, img)
	return tex
}

// Clip textured triangles to the stored image. An edited face can extend beyond
// its painted rectangle; sampling a clamped texture there would smear its edge.
func uvClip(poly []geom.Vec2, r image.Rectangle) []geom.Vec2 {
	for axis := 0; axis < 4; axis++ {
		if len(poly) == 0 {
			break
		}
		coord := func(p geom.Vec2) float64 {
			if axis < 2 {
				return p.X
			}
			return p.Y
		}
		limit := []float64{float64(r.Min.X), float64(r.Max.X), float64(r.Min.Y), float64(r.Max.Y)}[axis]
		inside := func(p geom.Vec2) bool {
			if axis%2 == 0 {
				return coord(p) >= limit
			}
			return coord(p) <= limit
		}
		var out []geom.Vec2
		prev := poly[len(poly)-1]
		for _, p := range poly {
			if inside(p) != inside(prev) {
				t := (limit - coord(prev)) / (coord(p) - coord(prev))
				out = append(out, prev.Add(p.Sub(prev).Mul(t)))
			}
			if inside(p) {
				out = append(out, p)
			}
			prev = p
		}
		poly = out
	}
	return poly
}

func (a *App) uvLine(is *uvIsland, from, to geom.Vec2, width float32, col color.RGBA) {
	f, t := a.uvScreen(is.sheetUV(from)), a.uvScreen(is.sheetUV(to))
	rl.DrawLineEx(rl.Vector2{X: float32(f.X), Y: float32(f.Y)}, rl.Vector2{X: float32(t.X), Y: float32(t.Y)}, width, col)
}

func (a *App) uvImage(is *uvIsland, img *image.RGBA, origin image.Point, alpha uint8) {
	if img == nil {
		return
	}
	tex := a.uvTexture(img)
	bounds := image.Rectangle{Min: origin, Max: origin.Add(img.Bounds().Size())}
	rl.SetTexture(tex.ID)
	rl.Begin(rl.Triangles)
	rl.Color4ub(255, 255, 255, alpha)
	for _, tri := range is.tris {
		poly := uvClip(tri[:], bounds)
		for j := 1; j+1 < len(poly); j++ {
			for _, p := range []geom.Vec2{poly[0], poly[j], poly[j+1]} {
				s := a.uvScreen(is.sheetUV(p))
				rl.TexCoord2f(float32((p.X-float64(origin.X))/float64(bounds.Dx())), float32((p.Y-float64(origin.Y))/float64(bounds.Dy())))
				rl.Vertex2f(float32(s.X), float32(s.Y))
			}
		}
	}
	rl.End()
	rl.SetTexture(0)
}

func (a *App) uvRectOutline(is *uvIsland, r image.Rectangle, col color.RGBA) {
	p := [4]geom.Vec2{{X: float64(r.Min.X), Y: float64(r.Min.Y)}, {X: float64(r.Max.X), Y: float64(r.Min.Y)}, {X: float64(r.Max.X), Y: float64(r.Max.Y)}, {X: float64(r.Min.X), Y: float64(r.Max.Y)}}
	for i := range p {
		a.uvLine(is, p[i], p[(i+1)%4], 1.5, col)
	}
}

func (a *App) drawUVCanvas(canvas rl.Rectangle) {
	rl.DrawRectangleRec(canvas, ui.ColorBG)
	rl.BeginScissorMode(int32(canvas.X), int32(canvas.Y), int32(canvas.Width), int32(canvas.Height))
	defer rl.EndScissorMode()
	rl.DisableBackfaceCulling()
	defer rl.EnableBackfaceCulling()
	b := a.Doc().BodyByID(a.uv.body)
	if b == nil {
		return
	}
	used := map[*image.RGBA]bool{}
	for i := range a.uv.islands {
		is := &a.uv.islands[i]
		lo := a.uvScreen(is.at)
		hi := a.uvScreen(is.at.Add(geom.Vec2{X: float64(is.bounds.Dx()), Y: float64(is.bounds.Dy())}))
		if hi.X < float64(canvas.X) || lo.X > float64(canvas.X+canvas.Width) || hi.Y < float64(canvas.Y) || lo.Y > float64(canvas.Y+canvas.Height) {
			continue
		}
		for _, tri := range is.tris {
			var pts [3]rl.Vector2
			for j, p := range tri {
				s := a.uvScreen(is.sheetUV(p))
				pts[j] = rl.Vector2{X: float32(s.X), Y: float32(s.Y)}
			}
			rl.DrawTriangle(pts[0], pts[1], pts[2], b.Color)
		}
		if p := is.paint; p != nil && b.Mesh.Faces[is.index].Paint != nil {
			a.uvImage(is, p.Img, p.Off, 255)
			used[p.Img] = true
		}
		h := a.paint.hover
		if h.ok && h.body == b.ID && h.face == is.face {
			var img *image.RGBA
			origin := h.texel
			if a.paint.tool == paint.ToolPaste {
				img = a.orientedPastePixels()
				origin = a.pixelPasteOrigin(origin)
			}
			if a.paint.tool == paint.ToolTile {
				img = a.armedTile()
				if img != nil && !a.paint.tiles.free {
					origin = paint.SnapToTileGrid(origin, img.Bounds().Dx(), img.Bounds().Dy())
				}
			}
			if img != nil {
				a.uvImage(is, img, origin, 170)
				used[img] = true
			}
		}
		if a.uv.grid && a.uv.zoom >= 6 {
			// Only iterate visible texels, even when zoomed into a large sheet.
			v0 := is.texelUV(a.uvSheet(float64(canvas.X), float64(canvas.Y)))
			v1 := is.texelUV(a.uvSheet(float64(canvas.X+canvas.Width), float64(canvas.Y+canvas.Height)))
			col := color.RGBA{R: 20, G: 25, B: 30, A: 65}
			for x := max(is.bounds.Min.X, int(math.Ceil(v0.X))); x <= min(is.bounds.Max.X, int(math.Floor(v1.X))); x++ {
				a.uvLine(is, geom.Vec2{X: float64(x), Y: float64(is.bounds.Min.Y)}, geom.Vec2{X: float64(x), Y: float64(is.bounds.Max.Y)}, 1, col)
			}
			for y := max(is.bounds.Min.Y, int(math.Ceil(v1.Y))); y <= min(is.bounds.Max.Y, int(math.Floor(v0.Y))); y++ {
				a.uvLine(is, geom.Vec2{X: float64(is.bounds.Min.X), Y: float64(y)}, geom.Vec2{X: float64(is.bounds.Max.X), Y: float64(y)}, 1, col)
			}
		}
		col := ui.ColorTextDim
		if a.paint.sticky.body == b.ID && a.paint.sticky.face == is.face {
			col = ui.ColorAccent
		}
		for _, loop := range is.loops {
			for j, p := range loop {
				a.uvLine(is, p, loop[(j+1)%len(loop)], 1.5, col)
			}
		}
		if a.InEdgePaint() {
			for ei, e := range b.Mesh.Topo().Edges {
				if !a.edgeSelected(b.ID, ei) && !(a.paint.hoverEdgeBody == b.ID && a.paint.hoverEdge == ei) {
					continue
				}
				for _, use := range e.Uses {
					if use.Face == is.index {
						a.uvLine(is, is.paint.UV(b.Mesh.Verts[e.A]), is.paint.UV(b.Mesh.Verts[e.B]), 3, ui.ColorAccent)
						break
					}
				}
			}
		}
		if hi.X-lo.X > float64(a.px(65)) {
			a.UI.Text(ui.Rect(float32(lo.X)+4, float32(lo.Y)+3, float32(hi.X-lo.X)-8, a.px(18)), fmt.Sprintf("Face %d", is.index+1), ui.FontSizeSmall, ui.ColorText)
		}
		if sel, _ := a.currentPixelSelection(); sel != nil && sel.body == b.ID && sel.face == is.face {
			a.uvRectOutline(is, sel.rect, ui.ColorAccent)
		}
		if a.paint.wandMask != nil && a.paint.wandBody == b.ID && a.paint.wandFace == is.face {
			for _, seg := range a.paint.wandMask.Boundary() {
				a.uvLine(is, geom.Vec2{X: float64(seg[0].X), Y: float64(seg[0].Y)}, geom.Vec2{X: float64(seg[1].X), Y: float64(seg[1].Y)}, 1.5, ui.ColorAccent)
			}
		}
		if h.ok && h.body == b.ID && h.face == is.face && a.paint.tool != paint.ToolPaste && a.paint.tool != paint.ToolTile {
			n := max(1, a.paint.size)
			if a.paint.tool == paint.ToolPick || a.paint.tool == paint.ToolSelect || a.paint.tool == paint.ToolWand {
				n = 1
			}
			origin := h.texel.Sub(image.Pt(n/2, n/2))
			a.uvRectOutline(is, image.Rectangle{Min: origin, Max: origin.Add(image.Pt(n, n))}, rl.White)
		}
	}
	for img, tex := range a.uv.textures {
		if !used[img] {
			rl.UnloadTexture(tex)
			delete(a.uv.textures, img)
			delete(a.uv.textureDirty, img)
		}
	}
}
