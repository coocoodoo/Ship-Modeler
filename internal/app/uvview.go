package app

import (
	"fmt"
	"image"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// The UV sheet is a view of the existing face mappings, not another copy of
// the pixels. Sheet placement never changes the model or its export mapping.
type uvIsland struct {
	face        mesh.FaceUID
	index       int
	paint       *mesh.FacePaint
	provisional *mesh.FacePaint
	bounds      image.Rectangle
	at          geom.Vec2
	loops       [][]geom.Vec2
	tris        [][3]geom.Vec2
}

type uvViewState struct {
	open, expanded, grid     bool
	body                     uint32
	doc                      *model.Document
	mesh                     *mesh.Mesh
	res                      int
	dirty                    bool
	islands                  []uvIsland
	size                     geom.Vec2
	zoom                     float64   // screen pixels per texel
	pan                      geom.Vec2 // relative to canvas centre
	input, captured, panning bool
	anchor                   int
	textures                 map[*image.RGBA]rl.Texture2D
	textureDirty             map[*image.RGBA]bool
}

func (a *App) uvVisible() bool { return a.uv.open && a.InPaint() }

func (a *App) toggleUVView() {
	if a.uvVisible() {
		a.closeUVView()
		return
	}
	if a.Mode != ModeIdle && !a.InPaint() {
		return
	}
	body := a.uv.body
	if ref, ok := a.Sel.Primary(); ok && ref.Body != 0 {
		body = ref.Body
	} else if h, ok := a.stickyFace(); ok {
		body = h.body
	}
	if !a.BeginPaint() {
		return
	}
	a.uv.open, a.uv.grid, a.uv.body, a.uv.dirty = true, true, body, true
	a.uv.zoom = 0
	a.UI.ClearFocus()
}

func (a *App) closeUVView() {
	if a.uv.captured {
		a.finishStroke()
		a.finishTileStamp()
		a.paint.pixels.dragging = false
	}
	a.uv.open, a.uv.captured, a.uv.panning, a.uv.input = false, false, false, false
	a.dropUVTextures()
}

func (a *App) uvRects() (panel, canvas rl.Rectangle) {
	v := a.layout.Viewport
	margin := a.px(12)
	w, h := min(a.px(520), v.Width-margin*2), min(a.px(390), v.Height*.62)
	if a.uv.expanded {
		w, h = v.Width-margin*2, v.Height-margin*2
	}
	panel = ui.Rect(v.X+v.Width-w-margin, v.Y+v.Height-h-margin, w, h)
	canvas = ui.Rect(panel.X+a.px(10), panel.Y+a.px(82), panel.Width-a.px(20), max(a.px(20), panel.Height-a.px(110)))
	return
}

func uvContains(r rl.Rectangle, x, y float64) bool {
	return x >= float64(r.X) && y >= float64(r.Y) && x < float64(r.X+r.Width) && y < float64(r.Y+r.Height)
}

func (a *App) uvBlocked() bool {
	return a.notePins.open || a.showSettings || a.showShortcuts || a.UI.ModalOpen() || a.libraryOwnsInput() || a.markers.attachmentOpen || a.InPaletteBrowser() || a.pixelPasteMenuOpen()
}

func (a *App) uvOwnsPointer(in InputFrame) bool {
	if !a.uvVisible() {
		return false
	}
	r, _ := a.uvRects()
	return a.uv.captured || a.uv.panning || uvContains(r, in.MouseX, in.MouseY)
}

func (a *App) onUVEvent(ev model.Event) {
	if !a.uv.open {
		return
	}
	if ev.Kind == model.EvDocReplaced {
		a.closeUVView()
		a.uv.islands, a.uv.doc = nil, nil
		return
	}
	if ev.BodyID != a.uv.body {
		return
	}
	if ev.Kind == model.EvBodyPainted {
		if ev.Paint != nil && ev.Paint.Img != nil {
			if a.uv.textureDirty == nil {
				a.uv.textureDirty = map[*image.RGBA]bool{}
			}
			a.uv.textureDirty[ev.Paint.Img] = true
		}
	} else if ev.Kind == model.EvBodyChanged || ev.Kind == model.EvBodyRemoved {
		a.uv.dirty = true
	}
}

func (a *App) prepareUVView() {
	if !a.uvVisible() {
		return
	}
	st := &a.uv
	b := a.Doc().BodyByID(st.body)
	if b == nil || !b.Visible || b.Mesh == nil {
		b = nil
		for _, candidate := range a.Doc().Bodies {
			if candidate.Visible && candidate.Mesh != nil {
				b = candidate
				break
			}
		}
		st.dirty, st.zoom = true, 0
		if b == nil {
			st.islands = nil
			a.dropUVTextures()
			return
		}
		st.body = b.ID
	}
	res := a.allocResFor(b.ID)
	if st.dirty && st.doc == a.Doc() && st.res == res && a.uvLayoutMatches(b) {
		st.dirty, st.mesh = false, b.Mesh
		for img := range st.textures {
			st.textureDirty[img] = true
		}
	}
	if st.dirty || st.mesh != b.Mesh || st.res != res || st.doc != a.Doc() {
		// Topology or resolution changed: no captured coordinates may outlive it.
		if st.captured {
			a.CancelStroke()
			a.paint.pixels.dragging = false
			st.captured = false
		}
		a.dropUVTextures()
		st.islands = nil
		st.mesh, st.res, st.doc, st.dirty = b.Mesh, res, a.Doc(), false
		area, widest := 0.0, 0.0
		for fi, face := range b.Mesh.Faces {
			p := face.Paint
			var provisional *mesh.FacePaint
			if p == nil {
				p, _ = paint.Allocate(b.Mesh, fi, res)
				provisional = p
			}
			if p == nil {
				continue
			}
			r := paint.FaceRect(b.Mesh, fi, p)
			if r.Empty() {
				continue
			}
			island := uvIsland{face: face.ID, index: fi, paint: p, provisional: provisional, bounds: r}
			for _, loop := range face.Loops {
				var points []geom.Vec2
				for _, vi := range loop {
					points = append(points, p.UV(b.Mesh.Verts[vi]))
				}
				island.loops = append(island.loops, points)
			}
			for _, tri := range b.Mesh.FaceTris(fi) {
				island.tris = append(island.tris, [3]geom.Vec2{p.UV(b.Mesh.Verts[tri.A]), p.UV(b.Mesh.Verts[tri.B]), p.UV(b.Mesh.Verts[tri.C])})
			}
			st.islands = append(st.islands, island)
			area += float64((r.Dx() + 8) * (r.Dy() + 8))
			widest = math.Max(widest, float64(r.Dx()+8))
		}
		width := math.Max(widest, math.Sqrt(area)*1.25)
		x, y, row, maxX := 0.0, 0.0, 0.0, 0.0
		for i := range st.islands {
			is := &st.islands[i]
			w, h := float64(is.bounds.Dx()), float64(is.bounds.Dy())
			if x > 0 && x+w > width {
				x, y, row = 0, y+row+8, 0
			}
			is.at = geom.Vec2{X: x, Y: y}
			x += w + 8
			row = math.Max(row, h)
			maxX = math.Max(maxX, x-8)
		}
		st.size = geom.Vec2{X: maxX, Y: y + row}
		st.zoom = 0
	}
	// A stroke replaces provisional paint on its first pixel. Keep the sheet
	// placement fixed while refreshing the actual image after every command.
	for i := range st.islands {
		is := &st.islands[i]
		p := b.Mesh.Faces[is.index].Paint
		if p == nil {
			if is.provisional == nil {
				is.provisional, _ = paint.Allocate(b.Mesh, is.index, res)
			}
			p = is.provisional
		}
		is.paint = p
	}
	if st.zoom == 0 {
		a.fitUVView(false)
	}
}

func (a *App) fitUVView(face bool) {
	_, c := a.uvRects()
	st := &a.uv
	size, center := st.size, st.size.Mul(.5)
	if face {
		for _, is := range st.islands {
			if is.face == a.paint.sticky.face {
				size = geom.Vec2{X: float64(is.bounds.Dx()), Y: float64(is.bounds.Dy())}
				center = is.at.Add(size.Mul(.5))
				break
			}
		}
	}
	// Fit each dimension separately, preserving the pixel aspect ratio.
	st.zoom = math.Max(.02, math.Min(64, math.Min((float64(c.Width)-32)/math.Max(1, size.X), (float64(c.Height)-32)/math.Max(1, size.Y))))
	st.pan = center.Mul(-st.zoom)
}

func (a *App) uvScreen(p geom.Vec2) geom.Vec2 {
	_, c := a.uvRects()
	return geom.Vec2{X: float64(c.X + c.Width/2), Y: float64(c.Y + c.Height/2)}.Add(a.uv.pan).Add(p.Mul(a.uv.zoom))
}
func (a *App) uvSheet(x, y float64) geom.Vec2 {
	o := a.uvScreen(geom.Vec2{})
	return geom.Vec2{X: (x - o.X) / a.uv.zoom, Y: (y - o.Y) / a.uv.zoom}
}
func (is *uvIsland) sheetUV(p geom.Vec2) geom.Vec2 {
	return is.at.Add(geom.Vec2{X: p.X - float64(is.bounds.Min.X), Y: float64(is.bounds.Max.Y) - p.Y})
}
func (is *uvIsland) texelUV(p geom.Vec2) geom.Vec2 {
	return geom.Vec2{X: p.X - is.at.X + float64(is.bounds.Min.X), Y: float64(is.bounds.Max.Y) - (p.Y - is.at.Y)}
}
func uvInside(is *uvIsland, p geom.Vec2) bool {
	inside := false
	for _, loop := range is.loops {
		for i, a := range loop {
			b := loop[(i+1)%len(loop)]
			if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
				inside = !inside
			}
		}
	}
	return inside
}
func (a *App) uvHit(x, y float64) (int, geom.Vec2) {
	_, c := a.uvRects()
	if !uvContains(c, x, y) {
		return -1, geom.Vec2{}
	}
	p := a.uvSheet(x, y)
	for i := range a.uv.islands {
		is := &a.uv.islands[i]
		uv := is.texelUV(p)
		if uvInside(is, uv) {
			return i, uv
		}
	}
	return -1, geom.Vec2{}
}

func (a *App) refreshUVHover(in InputFrame) {
	a.clearPaintHover(false)
	i, uv := a.uvHit(in.MouseX, in.MouseY)
	if i < 0 {
		return
	}
	is := &a.uv.islands[i]
	if a.paint.locked && !a.material.open && (a.paint.lockBody != a.uv.body || a.paint.lockFace != is.face) {
		return
	}
	b := a.Doc().BodyByID(a.uv.body)
	if b == nil || is.paint == nil {
		return
	}
	h := paintHover{ok: true, body: b.ID, face: is.face, index: is.index, paint: is.paint,
		allocated: b.Mesh.Faces[is.index].Paint != nil, texel: image.Pt(int(math.Floor(uv.X)), int(math.Floor(uv.Y)))}
	a.paint.hover, a.paint.sticky = h, h
}

func (a *App) uvPointOnFace(x, y float64) (geom.Vec3, bool) {
	_, c := a.uvRects()
	if !uvContains(c, x, y) {
		return geom.Vec3{}, false
	}
	i := a.uv.anchor
	if i < 0 || i >= len(a.uv.islands) {
		return geom.Vec3{}, false
	}
	is := &a.uv.islands[i]
	if is.paint == nil {
		return geom.Vec3{}, false
	}
	uv := is.texelUV(a.uvSheet(x, y))
	return is.paint.Frame.ToWorld(uv.Mul(is.paint.Texel)), true
}

func (a *App) zoomUV(factor, x, y float64) {
	before := a.uvSheet(x, y)
	a.uv.zoom = math.Max(.02, math.Min(64, a.uv.zoom*factor))
	after := a.uvScreen(before)
	a.uv.pan = a.uv.pan.Add(geom.Vec2{X: x - after.X, Y: y - after.Y})
}

func (a *App) updateUVView(in InputFrame, vp render.Viewport) {
	// A stroke started in 3D keeps its original projection until release.
	if !a.uv.captured && (a.paint.stroking || a.paint.tiles.stamping || a.paint.pixels.dragging) {
		a.paintChromeFrame(in)
		return
	}
	defer func() { a.uv.captured = a.paint.stroking || a.paint.tiles.stamping || a.paint.pixels.dragging }()
	if a.uvBlocked() {
		a.paintChromeFrame(in)
		return
	}
	_, c := a.uvRects()
	inside := uvContains(c, in.MouseX, in.MouseY)
	if a.UI.OverlayCapturesPointer(in.MouseX, in.MouseY) || a.UI.Dragging() {
		a.paintChromeFrame(in)
		return
	}
	st := &a.uv
	if in.FocusLost {
		a.CancelStroke()
		a.paint.pixels.dragging = false
		st.captured, st.panning = false, false
		return
	}
	if inside && !st.captured && (in.Pressed[MouseMiddle] || (in.KeyDown(rl.KeySpace) && in.Pressed[MouseLeft])) {
		st.panning = true
	}
	if st.panning {
		if in.Down[MouseMiddle] || in.Down[MouseLeft] {
			st.pan = st.pan.Add(geom.Vec2{X: in.MouseDX, Y: in.MouseDY})
		} else {
			st.panning = false
		}
		a.clearPaintHover(false)
		return
	}
	st.input = true
	defer func() { st.input = false }()
	if inside && in.Wheel != 0 && !st.captured {
		if !a.rotatePixelPasteWheel(in) {
			a.zoomUV(math.Pow(1.2, in.Wheel), in.MouseX, in.MouseY)
		}
	}
	if !inside && !st.captured {
		a.paintChromeFrame(in)
		return
	}
	if !st.captured {
		st.anchor, _ = a.uvHit(in.MouseX, in.MouseY)
		a.refreshUVHover(in)
		if a.handlePixelPasteRightClick(&in, vp) {
			return
		}
	} else {
		a.clearPaintHover(false)
		if i, _ := a.uvHit(in.MouseX, in.MouseY); i == st.anchor {
			a.refreshUVHover(in)
		}
	}
	a.updatePaint(in, vp)
}

func (a *App) pickUVEdge(in InputFrame) {
	a.paint.hoverEdge, a.paint.hoverEdgeBody = -1, 0
	b := a.Doc().BodyByID(a.uv.body)
	if b == nil {
		return
	}
	_, c := a.uvRects()
	if !uvContains(c, in.MouseX, in.MouseY) {
		return
	}
	best := 8 * math.Max(1, a.Scale)
	for _, is := range a.uv.islands {
		if is.paint == nil || (a.paint.locked && (a.paint.lockBody != b.ID || a.paint.lockFace != is.face)) {
			continue
		}
		for ei, e := range b.Mesh.Topo().Edges {
			used := false
			for _, use := range e.Uses {
				if use.Face == is.index {
					used = true
				}
			}
			if !used {
				continue
			}
			x := a.uvScreen(is.sheetUV(is.paint.UV(b.Mesh.Verts[e.A])))
			y := a.uvScreen(is.sheetUV(is.paint.UV(b.Mesh.Verts[e.B])))
			d, _ := tools.AxisDistancePxClamped(in.MouseX, in.MouseY, x.X, x.Y, y.X, y.Y)
			if d < best {
				best = d
				a.paint.hoverEdge, a.paint.hoverEdgeBody = ei, b.ID
			}
		}
	}
	if in.Pressed[MouseLeft] && a.paint.hoverEdge >= 0 {
		a.toggleEdge(b.ID, a.paint.hoverEdge)
	}
}

func (a *App) buildUVToggle(r rl.Rectangle) {
	a.UI.Panel(r)
	a.UI.HairlineH(r.X, r.Y, r.Width, ui.ColorStroke)
	if a.UI.IconButton(ui.MakeID("view.uv"), ui.InsetXY(r, a.px(5), a.px(3)), ui.DrawPixelSelectIcon, ui.IconOpts{
		Label: "UV Map", Active: a.uvVisible(), Tooltip: "UV map — view and paint the body's faces in 2D",
		Disabled: a.Mode != ModeIdle && !a.InPaint(),
	}) {
		a.toggleUVView()
	}
}

func (a *App) buildUVView() {
	if !a.uvVisible() {
		return
	}
	a.prepareUVView()
	panel, canvas := a.uvRects()
	a.UI.FillRounded(panel, 10, ui.ColorPanel)
	a.UI.StrokeRounded(panel, 10, ui.ColorStroke)
	header := ui.Rect(panel.X+a.px(12), panel.Y+a.px(8), panel.Width-a.px(24), a.px(28))
	close, title := ui.SplitRight(header, a.px(28))
	title, bodyBox := ui.SplitLeft(title, a.px(78))
	a.UI.Text(title, "UV Map", ui.FontSizeUI, ui.ColorText)
	if a.UI.IconButton(ui.MakeID("uv.close"), close, ui.DrawCrossIcon, ui.IconOpts{Tooltip: "Close UV map"}) {
		a.closeUVView()
		return
	}
	var labels []string
	var ids []uint32
	selected := 0
	for _, b := range a.Doc().Bodies {
		if b.Visible && b.Mesh != nil {
			if b.ID == a.uv.body {
				selected = len(ids)
			}
			ids = append(ids, b.ID)
			labels = append(labels, b.Name)
		}
	}
	row := ui.Rect(panel.X+a.px(10), panel.Y+a.px(43), panel.Width-a.px(20), a.px(28))
	if next, changed := a.UI.Select(ui.MakeID("uv.body"), bodyBox, labels, selected); changed && next < len(ids) {
		a.uv.body, a.uv.dirty = ids[next], true
		a.prepareUVView()
	}
	for i, label := range []string{"-", "+", "Fit", "Face", "Grid", "Size"} {
		r := ui.Rect(row.X+float32(i)*row.Width/6, row.Y, row.Width/6-a.px(3), row.Height)
		style := ui.ButtonNormal
		if label == "Grid" && a.uv.grid {
			style = ui.ButtonPrimary
		}
		if a.UI.Button(ui.MakeID("uv."+label), r, label, ui.ButtonOpts{Style: style, Tooltip: map[string]string{"-": "Zoom out", "+": "Zoom in", "Fit": "Fit all faces", "Face": "Zoom to the last pointed face", "Grid": "Pixel grid", "Size": "Expand or restore the UV viewer"}[label]}) {
			switch label {
			case "-":
				a.zoomUV(1/1.25, float64(canvas.X+canvas.Width/2), float64(canvas.Y+canvas.Height/2))
			case "+":
				a.zoomUV(1.25, float64(canvas.X+canvas.Width/2), float64(canvas.Y+canvas.Height/2))
			case "Fit":
				a.fitUVView(false)
			case "Face":
				a.fitUVView(true)
			case "Grid":
				a.uv.grid = !a.uv.grid
			case "Size":
				a.uv.expanded = !a.uv.expanded
				a.fitUVView(false)
			}
		}
	}
	a.drawUVCanvas(canvas)
	footer := ui.Rect(panel.X+a.px(12), panel.Y+panel.Height-a.px(24), panel.Width-a.px(24), a.px(20))
	text := fmt.Sprintf("%.0f%% · Wheel: zoom · Middle drag: pan", a.uv.zoom*100)
	if len(a.uv.islands) == 0 {
		text = "Select a visible body to see its faces"
	}
	a.UI.Text(footer, text, ui.FontSizeSmall, ui.ColorTextDim)
}
