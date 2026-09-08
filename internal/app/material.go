package app

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
	modelio "modeler/internal/io"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
)

type materialState struct {
	scroll    float32
	open      bool
	body      uint32
	face      mesh.FaceUID
	channel   int
	faceOnly  bool
	thumbnail rl.Texture2D
	source    *image.RGBA
}

var materialLabels = []string{"Base Color", "Specular", "Ambient Occlusion", "Displacement / Height", "Roughness", "Normal"}

func (a *App) openMaterialPanel() {
	a.finishStroke()
	a.finishTileStamp()
	a.paint.pixels.dragging = false
	a.paint.pixels.menu = pasteMenuState{}
	a.material.open = true
	a.material.faceOnly = false
	if h, ok := a.stickyFace(); ok {
		a.material.body, a.material.face = h.body, h.face
	}
	if a.Renderer != nil {
		a.Renderer.MaterialView = 0
	}
	a.syncMaterialScope()
}

func (a *App) syncMaterialScope() {
	if a.Renderer == nil {
		return
	}
	a.Renderer.MaterialFaceOnly = a.material.faceOnly
	a.Renderer.MaterialBody = a.material.body
	a.Renderer.MaterialFace = a.material.face
}

// Material inspection owns clicks before any paint tool can consume them.
// Ignore paint locks: choosing a different face must always be possible.
func (a *App) updateMaterialSelection(in InputFrame, vp render.Viewport) {
	if a.uv.input {
		a.refreshUVHover(in)
	} else {
		if !vp.Contains(int(in.MouseX), int(in.MouseY)) || a.Cube.Contains(in.MouseX, in.MouseY) || a.orbiting || a.panning || a.cubeDrag {
			a.clearPaintHover(false)
			return
		}
		a.pickPaintFace(in, vp)
	}
	if h := a.paint.hover; h.ok {
		a.paint.sticky = h
		if in.Pressed[MouseLeft] {
			a.material.body, a.material.face = h.body, h.face
			a.syncMaterialScope()
		}
	}
}

func (a *App) closeMaterialPanel() {
	a.material.open = false
	if a.Renderer != nil {
		a.Renderer.MaterialView = 0
		a.Renderer.MaterialFaceOnly = false
	}
	if a.material.thumbnail.ID != 0 {
		rl.UnloadTexture(a.material.thumbnail)
	}
	a.material.thumbnail = rl.Texture2D{}
	a.material.source = nil
}
func (a *App) buildMaterialPanel(body rl.Rectangle) {
	st := &a.material
	viewport := body
	contentHeight := a.px(830)
	st.scroll = max(0, min(st.scroll-float32(a.UI.ScrollWheel(viewport))*a.px(36), max(0, contentHeight-viewport.Height)))
	body.Y -= st.scroll
	body.Height = max(viewport.Height, contentHeight)
	a.UI.Clip(viewport, func() { a.buildMaterialPanelContent(body) })
}

func (a *App) buildMaterialPanelContent(body rl.Rectangle) {
	st := &a.material
	row := func(h float64) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, a.px(h)); return r }
	if a.UI.Button(ui.MakeID("material.back"), row(28), "Back to Paint", ui.ButtonOpts{}) {
		a.closeMaterialPanel()
		return
	}
	row(6)
	a.UI.Text(row(24), "PBR Materials", ui.FontSizeHeader, ui.ColorAccent)
	a.UI.Text(row(24), "Click a face to inspect its maps", ui.FontSizeSmall, ui.ColorTextDim)
	scope := 0
	if st.faceOnly {
		scope = 1
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("material.scope"), row(28), []string{"Whole ship", "Selected face"}, scope, ui.ChipGroupOpts{}); changed {
		st.faceOnly = pick == 1
		a.syncMaterialScope()
	}
	view := 0
	if a.Renderer.MaterialView != 0 {
		view = 1
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("material.view"), row(28), []string{"Shaded PBR", "Texture map"}, view, ui.ChipGroupOpts{}); changed {
		a.Renderer.MaterialView = 0
		if pick == 1 {
			a.Renderer.MaterialView = st.channel + 1
		} else {
			a.Settings.FlatShading = false
		}
	}
	f, valid := a.resolveFace(st.body, st.face)
	label := "No face chosen"
	if valid {
		label = fmt.Sprintf("%s · face %d", f.body.Name, st.face.Seq())
	}
	a.UI.Text(row(24), label, ui.FontSizeSmall, ui.ColorTextDim)
	row(6)
	for i, label := range materialLabels {
		ready := false
		if valid {
			p := f.body.Mesh.Faces[f.face].Paint
			if p != nil {
				if i == 0 {
					ready = p.Img != nil
				} else {
					ready = p.Material != nil && p.Material.Maps[mesh.MaterialChannels[i]] != nil
				}
			}
		}
		if !st.faceOnly {
			set, total := 0, 0
			for _, b := range a.Doc().Bodies {
				if !b.Visible {
					continue
				}
				for _, face := range b.Mesh.Faces {
					total++
					p := face.Paint
					if p != nil && ((i == 0 && p.Img != nil) || (i > 0 && p.Material != nil && p.Material.Maps[mesh.MaterialChannels[i]] != nil)) {
						set++
					}
				}
			}
			label += fmt.Sprintf("  [%d/%d]", set, total)
		} else if ready {
			label += "  [set]"
		}
		style := ui.ButtonNormal
		if st.channel == i {
			style = ui.ButtonPrimary
		}
		if a.UI.Button(ui.MakeID(fmt.Sprintf("material.channel.%d", i)), row(26), label, ui.ButtonOpts{Style: style}) {
			st.channel = i
			if a.Renderer.MaterialView != 0 {
				a.Renderer.MaterialView = i + 1
			}
		}
	}
	row(8)
	var src *image.RGBA
	if valid {
		p := f.body.Mesh.Faces[f.face].Paint
		if p != nil {
			if st.channel == 0 {
				src = p.Img
			} else if p.Material != nil {
				if m := p.Material.Maps[mesh.MaterialChannels[st.channel]]; m != nil {
					src = m.Image
				}
			}
		}
	}
	if src != st.source {
		if st.thumbnail.ID != 0 {
			rl.UnloadTexture(st.thumbnail)
		}
		st.thumbnail = rl.Texture2D{}
		st.source = src
		if src != nil {
			img := rl.GenImageColor(src.Rect.Dx(), src.Rect.Dy(), color.RGBA{})
			st.thumbnail = rl.LoadTextureFromImage(img)
			rl.UnloadImage(img)
			rl.UpdateTexture(st.thumbnail, src)
			rl.SetTextureFilter(st.thumbnail, rl.FilterPoint)
		}
	}
	preview := row(max(32, min(120, float64(body.Height)/a.Scale-168)))
	a.UI.FillRounded(preview, 6, ui.ColorPanel)
	if st.thumbnail.ID != 0 {
		scale := min(preview.Width/float32(st.thumbnail.Width), preview.Height/float32(st.thumbnail.Height))
		w, h := float32(st.thumbnail.Width)*scale, float32(st.thumbnail.Height)*scale
		rl.DrawTexturePro(st.thumbnail, ui.Rect(0, 0, float32(st.thumbnail.Width), float32(st.thumbnail.Height)), ui.Rect(preview.X+(preview.Width-w)/2, preview.Y+(preview.Height-h)/2, w, h), rl.Vector2{}, 0, rl.White)
	} else {
		a.UI.TextCentered(preview, "No image assigned", ui.FontSizeSmall, ui.ColorTextDim)
	}
	row(6)
	if a.UI.Button(ui.MakeID("material.import"), row(28), "Import / Replace Image…", ui.ButtonOpts{Disabled: !valid, Style: ui.ButtonPrimary}) {
		path, ok, err := modelio.AskOpenMaterial()
		if err == nil && ok {
			err = a.ImportMaterialMap(st.body, st.face, mesh.MaterialChannels[st.channel], path)
		}
		if err != nil {
			a.Toast(ui.Toast{Text: err.Error(), Kind: ui.ToastWarn})
		}
	}
	if a.UI.Button(ui.MakeID("material.clear"), row(26), "Remove this map", ui.ButtonOpts{Disabled: !valid || src == nil}) {
		a.Run(&paint.SetMaterialMap{Body: st.body, Face: st.face, Kind: mesh.MaterialChannels[st.channel], Res: a.paint.res})
	}
	row(6)
	if a.UI.Button(ui.MakeID("material.export"), row(28), "Export Map…", ui.ButtonOpts{Disabled: !valid, Tooltip: "Save the selected channel as an unlit, face-fitted PNG"}) {
		a.exportMaterialDialog(false)
	}
	if a.UI.Button(ui.MakeID("material.export_set"), row(28), "Export Texture Set…", ui.ButtonOpts{Disabled: !valid, Tooltip: "Save all six aligned PNGs in a ZIP, including neutral starters for missing maps"}) {
		a.exportMaterialDialog(true)
	}
	a.UI.TextWrapped(row(96), "Clicking the model selects; painting resumes with Back to Paint. Image actions edit the selected face. Missing maps preview with neutral values. Height adds surface relief, not geometry displacement.", ui.FontSizeSmall, ui.ColorTextDim)
}

func (a *App) exportMaterialDialog(all bool) {
	st := &a.material
	kind := mesh.MaterialChannels[st.channel]
	title, ext, describe := "Export "+materialLabels[st.channel], ".png", "PNG image (*.png)"
	if all {
		kind = "textures"
		title = "Export PBR texture set"
		ext = ".zip"
		describe = "Texture set (*.zip)"
	}
	path, ok, err := modelio.AskExport(title, fmt.Sprintf("body_%d_face_%d_%s%s", st.body, st.face.Seq(), kind, ext), ext, describe)
	if err == nil && ok {
		err = a.ExportMaterialImages(st.body, st.face, kind, path)
	}
	if err != nil {
		a.Toast(ui.Toast{Text: err.Error(), Kind: ui.ToastWarn})
	} else if ok {
		a.Toast(ui.Toast{Text: "Textures exported to " + path, Kind: ui.ToastInfo})
	}
}

// ExportMaterialImages is also exposed through the AI connection. Exporting an
// unpainted face uses a temporary mapping and never changes the document.
func (a *App) ExportMaterialImages(body uint32, face mesh.FaceUID, kind, path string) error {
	f, ok := a.resolveFace(body, face)
	if !ok {
		return fmt.Errorf("choose a model face first")
	}
	p := f.body.Mesh.Faces[f.face].Paint
	if p == nil {
		var err error
		res := a.paint.res
		if res == 0 {
			res = paint.DefaultRes
		}
		p, err = paint.Allocate(f.body.Mesh, f.face, res)
		if err != nil {
			return err
		}
	}
	if kind == "textures" {
		return modelio.ExportMaterialSet(path, f.body, f.face, p)
	}
	return modelio.ExportMaterialMap(path, f.body, f.face, p, kind)
}

func (a *App) ImportMaterialMap(body uint32, face mesh.FaceUID, kind, path string) error {
	img, err := modelio.ReadMaterialImage(path)
	if err != nil {
		return err
	}
	if !a.Run(&paint.SetMaterialMap{Body: body, Face: face, Kind: kind, Image: img, Res: a.paint.res}) {
		return fmt.Errorf("could not assign the texture")
	}
	return nil
}
