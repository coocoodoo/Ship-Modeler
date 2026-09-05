package app

import (
	"fmt"
	"image"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/geom/mesh"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

type pixelSelection struct {
	body    uint32
	face    mesh.FaceUID
	mesh    *mesh.Mesh
	mapping mesh.FacePaint
	rect    image.Rectangle
}

type pixelClipboardState struct {
	selection    *pixelSelection
	clipboard    *image.RGBA // independent snapshot; survives edits and mode changes
	rotation     uint8
	flipH, flipV bool
	oriented     *image.RGBA
	dragging     bool
	anchor       image.Point
	corner       pasteCorner
	menu         pasteMenuState
}

// pixelSelectionRect includes both endpoint pixels, whichever way the drag goes.
func pixelSelectionRect(anchor, end image.Point, bounds image.Rectangle, square bool) image.Rectangle {
	end.X = clampInt(end.X, bounds.Min.X, bounds.Max.X-1)
	end.Y = clampInt(end.Y, bounds.Min.Y, bounds.Max.Y-1)
	if square {
		dx, dy := end.X-anchor.X, end.Y-anchor.Y
		sx, sy := 1, 1
		if dx < 0 {
			sx = -1
			dx = -dx
		}
		if dy < 0 {
			sy = -1
			dy = -dy
		}
		n := max(dx, dy)
		xRoom, yRoom := bounds.Max.X-1-anchor.X, bounds.Max.Y-1-anchor.Y
		if sx < 0 {
			xRoom = anchor.X - bounds.Min.X
		}
		if sy < 0 {
			yRoom = anchor.Y - bounds.Min.Y
		}
		n = min(n, xRoom, yRoom)
		end = anchor.Add(image.Pt(sx*n, sy*n))
	}
	return image.Rect(min(anchor.X, end.X), min(anchor.Y, end.Y), max(anchor.X, end.X)+1, max(anchor.Y, end.Y)+1).Intersect(bounds)
}

func (a *App) selectPixelRect(body uint32, face mesh.FaceUID, rect image.Rectangle) bool {
	f, ok := a.resolveFace(body, face)
	if !ok {
		return false
	}
	p, _ := a.mappingFor(f)
	if p == nil {
		return false
	}
	rect = rect.Intersect(paint.FaceRect(f.body.Mesh, f.face, p))
	if rect.Empty() {
		return false
	}
	a.paint.pixels.selection = &pixelSelection{body: body, face: face, mesh: f.body.Mesh, mapping: *p, rect: rect}
	return true
}

// Geometry or density changes invalidate the marquee, never the copied pixels.
func (a *App) currentPixelSelection() (*pixelSelection, *mesh.FacePaint) {
	s := a.paint.pixels.selection
	if s == nil {
		return nil, nil
	}
	f, ok := a.resolveFace(s.body, s.face)
	if !ok || f.body.Mesh != s.mesh {
		a.paint.pixels.selection = nil
		a.paint.pixels.dragging = false
		return nil, nil
	}
	p, _ := a.mappingFor(f)
	if p == nil || p.Texel != s.mapping.Texel || p.Frame != s.mapping.Frame {
		a.paint.pixels.selection = nil
		a.paint.pixels.dragging = false
		return nil, nil
	}
	return s, p
}

func (a *App) updatePixelClipboard(in InputFrame, vp render.Viewport) {
	if a.pixelPasteMenuOpen() {
		return
	}
	st := &a.paint.pixels
	if st.dragging {
		s, p := a.currentPixelSelection()
		if s == nil {
			return
		}
		if w, ok := a.pointOnFace(in.MouseX, in.MouseY, vp, p.Frame); ok {
			f, _ := a.resolveFace(s.body, s.face)
			s.rect = pixelSelectionRect(st.anchor, paint.Texel(p, w), paint.FaceRect(f.body.Mesh, f.face, p), in.Shift)
		}
		if !in.Down[MouseLeft] {
			st.dragging = false
		}
		return
	}
	a.refreshPaintHover(in, vp)
	h := a.paint.hover
	if !h.ok || !in.Pressed[MouseLeft] {
		return
	}
	if a.paint.tool == paint.ToolPaste {
		a.PastePixelsAt(h.body, h.face, h.texel)
		return
	}
	if a.selectPixelRect(h.body, h.face, image.Rectangle{Min: h.texel, Max: h.texel.Add(image.Pt(1, 1))}) {
		st.anchor, st.dragging = h.texel, true
	}
}

func (a *App) CopySelectedPixels() bool {
	a.finishStroke()
	a.finishTileStamp()
	s, p := a.currentPixelSelection()
	if s == nil {
		a.Toast(ui.Toast{Text: "Select pixels first — press U and drag a box on a face"})
		return false
	}
	f, _ := a.resolveFace(s.body, s.face)
	pixels := paint.CopyFacePixels(f.body.Mesh, f.face, p, s.rect)
	if pixels == nil {
		a.Toast(ui.Toast{Text: "That selection has no painted pixels"})
		return false
	}
	a.paint.pixels.clipboard = pixels
	a.paint.pixels.rotation, a.paint.pixels.oriented = 0, nil
	a.paint.pixels.flipH, a.paint.pixels.flipV = false, false
	a.paint.pixels.dragging = false
	a.Toast(ui.Toast{Text: fmt.Sprintf("Copied %d × %d pixels — Ctrl+V to place on a face", pixels.Bounds().Dx(), pixels.Bounds().Dy())})
	return true
}

func (a *App) BeginPixelPaste() bool {
	if a.paint.pixels.clipboard == nil {
		a.Toast(ui.Toast{Text: "Copy a pixel selection first — U to select, Ctrl+C to copy"})
		return false
	}
	a.setPaintTool(paint.ToolPaste)
	// Pasting is explicitly a transfer to any face, including from a locked source.
	a.paint.locked, a.paint.awaitingLock = false, false
	a.paint.hideTextures = false
	a.clearPaintHover(false)
	return true
}

func (a *App) PastePixelsAt(body uint32, face mesh.FaceUID, at image.Point) bool {
	if a.paint.pixels.clipboard == nil {
		return false
	}
	return a.Run(&paint.StampFace{Body: body, Face: face, Res: a.allocResFor(body),
		Tile: a.orientedPastePixels(), Cells: []image.Point{a.pixelPasteOrigin(at)}, PreserveAlpha: true})
}

// rotatePixelPasteWheel consumes the gesture before camera zoom can use it.
func (a *App) rotatePixelPasteWheel(in InputFrame) bool {
	if !a.InPaint() || a.paint.tool != paint.ToolPaste || a.paint.awaitingLock ||
		!in.Ctrl || in.Wheel == 0 || a.paint.pixels.clipboard == nil {
		return false
	}
	// Wheel magnitude varies with device settings and coalesced input. Treat
	// each event as one visible quarter turn: a delta of four must not wrap
	// straight back to the same preview and appear to do nothing.
	turns := 1
	if in.Wheel < 0 {
		turns = -1
	}
	st := &a.paint.pixels
	a.setPixelPasteRotation(int(st.rotation) + turns)
	return true
}

func (a *App) setPixelPasteRotation(quarterTurns int) {
	st := &a.paint.pixels
	st.rotation = uint8((quarterTurns%4 + 4) % 4)
	st.oriented = nil
}

func (a *App) orientedPastePixels() *image.RGBA {
	st := &a.paint.pixels
	if st.rotation == 0 && !st.flipH && !st.flipV {
		return st.clipboard
	}
	if st.oriented == nil {
		orientation := paint.Orientation{Rot: st.rotation}
		// Mirrors act on the rotated preview's axes, so H always exchanges
		// its left/right pixels and V its top/bottom pixels.
		if st.flipH {
			orientation = orientation.Flipped()
		}
		if st.flipV {
			orientation = orientation.FlippedVertical()
		}
		st.oriented = paint.OrientPixels(st.clipboard, orientation)
	}
	return st.oriented
}

func (a *App) togglePixelPasteMirror(vertical bool) {
	st := &a.paint.pixels
	if st.clipboard == nil {
		return
	}
	if vertical {
		st.flipV = !st.flipV
	} else {
		st.flipH = !st.flipH
	}
	st.oriented = nil
}

func (a *App) cancelPixelClipboard() bool {
	if a.paint.tool == paint.ToolPaste {
		a.setPaintTool(paint.ToolSelect)
		return true
	}
	if a.paint.pixels.selection != nil {
		a.paint.pixels.selection = nil
		a.paint.pixels.dragging = false
		return true
	}
	return false
}

func (a *App) pixelSelectionOverlay() *render.Overlay {
	if !a.InPaint() || a.paint.tool == paint.ToolPaste {
		return nil
	}
	s, p := a.currentPixelSelection()
	if s == nil {
		return nil
	}
	return scene.BuildPixelSelection(p, s.rect)
}

func (a *App) pixelPastePreview() *render.Overlay {
	h := a.paint.hover
	if a.pixelPasteMenuOpen() {
		h = a.paint.pixels.menu.hover
	}
	if !h.ok || h.paint == nil || a.paint.pixels.clipboard == nil {
		return nil
	}
	f, ok := a.resolveFace(h.body, h.face)
	if !ok {
		return nil
	}
	clip := paint.FaceRect(f.body.Mesh, f.face, h.paint)
	return scene.BuildTileGhost(scene.TileGhostView{Paint: h.paint, Cell: a.pixelPasteOrigin(h.texel), Tile: a.orientedPastePixels(), PreserveAlpha: true, Clip: &clip})
}

func (a *App) buildPixelClipboardActions(title rl.Rectangle) {
	w := a.px(26)
	x := title.X + title.Width - w*3 - a.px(8)
	box := func(i float32) rl.Rectangle { return ui.Rect(x+i*(w+a.px(4)), title.Y, w, title.Height) }
	if a.UI.IconButton(ui.MakeID("paint.selectpixels"), box(0), ui.DrawPixelSelectIcon, ui.IconOpts{
		Active: a.paint.tool == paint.ToolSelect, Tooltip: "Select pixels — drag a box on a face", Shortcut: "U"}) {
		a.setPaintTool(paint.ToolSelect)
	}
	s, _ := a.currentPixelSelection()
	if a.UI.IconButton(ui.MakeID("paint.copypixels"), box(1), ui.DrawCopyPixelsIcon, ui.IconOpts{
		Disabled: s == nil, DisabledWhy: "Drag a pixel selection on a face first", Tooltip: "Copy selected pixels", Shortcut: "Ctrl+C"}) {
		a.CopySelectedPixels()
	}
	if a.UI.IconButton(ui.MakeID("paint.pastepixels"), box(2), ui.DrawPastePixelsIcon, ui.IconOpts{
		Active: a.paint.tool == paint.ToolPaste, Disabled: a.paint.pixels.clipboard == nil, DisabledWhy: "Copy selected pixels first", Tooltip: "Paste pixels onto any face", Shortcut: "Ctrl+V"}) {
		a.BeginPixelPaste()
	}
}

func (a *App) buildPixelClipboardSection(row func(float32) rl.Rectangle, line float32) {
	a.UI.Text(row(line), "Pixel selection", ui.FontSizeUI, ui.ColorText)
	s, _ := a.currentPixelSelection()
	label := "Drag a box on a painted face"
	if s != nil {
		label = fmt.Sprintf("Selected: %d × %d px", s.rect.Dx(), s.rect.Dy())
	}
	a.UI.Text(row(line), label, ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.Text(row(line), "Hold Shift for a square", ui.FontSizeSmall, ui.ColorTextDim)
	if a.UI.Button(ui.MakeID("paint.copyselection"), row(a.px(26)), "Copy pixels", ui.ButtonOpts{Disabled: s == nil, Shortcut: "Ctrl+C", DisabledWhy: "Select pixels on a face first"}) {
		a.CopySelectedPixels()
	}
	row(a.px(10))
	label = "Clipboard is empty"
	if p := a.orientedPastePixels(); p != nil {
		label = fmt.Sprintf("Clipboard: %d × %d px · %d°", p.Bounds().Dx(), p.Bounds().Dy(), int(a.paint.pixels.rotation)*90)
	}
	a.UI.Text(row(line), label, ui.FontSizeSmall, ui.ColorTextDim)
	if a.UI.Button(ui.MakeID("paint.placepixels"), row(a.px(26)), "Paste onto a face", ui.ButtonOpts{Disabled: a.paint.pixels.clipboard == nil, Shortcut: "Ctrl+V", DisabledWhy: "Copy pixels first"}) {
		a.BeginPixelPaste()
	}
	if a.paint.tool == paint.ToolPaste && a.paint.pixels.clipboard != nil {
		row(a.px(6))
		a.UI.Text(row(line), "Rotation", ui.FontSizeSmall, ui.ColorTextDim)
		if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.pasterotation"), row(a.px(26)),
			[]string{"0°", "90°", "180°", "270°"}, int(a.paint.pixels.rotation), ui.ChipGroupOpts{
				Tooltip: "Choose the paste angle — Ctrl+wheel steps through these angles",
			}); changed {
			a.setPixelPasteRotation(pick)
		}
		row(a.px(6))
		mirrors := row(a.px(26))
		horizontal, vertical := ui.SplitLeft(mirrors, (mirrors.Width-a.px(6))/2)
		vertical.X += a.px(6)
		vertical.Width -= a.px(6)
		hOpts := ui.ButtonOpts{Tooltip: "Mirror the paste left to right; press again to restore"}
		vOpts := ui.ButtonOpts{Tooltip: "Mirror the paste top to bottom; press again to restore"}
		if a.paint.pixels.flipH {
			hOpts.Style = ui.ButtonPrimary
		}
		if a.paint.pixels.flipV {
			vOpts.Style = ui.ButtonPrimary
		}
		if a.UI.Button(ui.MakeID("paint.pastefliph"), horizontal, "Flip H", hOpts) {
			a.togglePixelPasteMirror(false)
		}
		if a.UI.Button(ui.MakeID("paint.pasteflipv"), vertical, "Flip V", vOpts) {
			a.togglePixelPasteMirror(true)
		}
		row(a.px(6))
	}
	a.UI.Text(row(line), "Click to place · Esc to cancel", ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.Text(row(line), "Right-click: corner / rotation", ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.Text(row(line), "Empty pixels leave paint underneath", ui.FontSizeSmall, ui.ColorTextDim)
}
