package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// Viewer is a presentation of the same document, never a converted copy.
// Switching to Edit therefore retains textures, PBR, pins and the file path.
func (a *App) enterViewer() {
	a.leaveModes()
	a.Mode = ModeIdle
	a.Sel.Clear()
	a.Hover = render.PickResult{}
	a.TreeHover = model.Ref{}
	a.Viewer = true
	a.paint.hideTextures = false
	a.Settings.FlatShading = false
	a.files.welcomeDismissed = true
	a.UI.ClearFocus()
}

func (a *App) editViewer() {
	a.Viewer = false
	a.orbiting, a.panning, a.cubeDrag = false, false, false
	a.UI.ClearFocus()
}

func viewerLayout(w, h int, scale float64) Layout {
	l := ComputeLayout(w, h, scale, 0, true, 0)
	l.Viewport.X = 0
	l.Viewport.Width = float32(w)
	l.Tree, l.Handle = rl.Rectangle{}, rl.Rectangle{}
	return l
}

func (a *App) updateViewer(in InputFrame, vp render.Viewport) {
	if in.FocusLost {
		a.orbiting, a.panning, a.cubeDrag = false, false, false
		return
	}
	if !a.chromeOwnsPointer(in) || a.orbiting || a.panning || a.cubeDrag {
		a.handleCubeInput(in, vp)
		a.handleCameraInput(in, vp)
	}
	if !a.UI.WantKeyboard() && !a.UI.ModalOpen() {
		if in.KeyPressed(rl.KeyF) {
			a.FrameSelection(vp)
		}
		if in.KeyPressed(rl.KeyHome) {
			a.GoHome(vp)
		}
	}
}

func (a *App) buildViewerShell(l Layout) {
	a.UI.Panel(l.Toolbar)
	a.UI.HairlineH(0, l.Toolbar.Height-1, l.Toolbar.Width, ui.ColorStroke)
	edit, title := ui.SplitRight(ui.InsetXY(l.Toolbar, a.px(12), a.px(5)), a.px(100))
	a.UI.Text(title, a.DocumentName(), ui.FontSizeSmall, ui.ColorText)
	if a.UI.IconButton(ui.MakeID("viewer.edit"), edit, ui.DrawSketchIcon, ui.IconOpts{
		Label: "Edit", Active: true, Accent: ui.AccentModel,
		Tooltip: "Open the full modeling and painting workspace",
	}) {
		a.editViewer()
	}
	a.UI.HintBar(l.HintBar, "VIEWER", "Right drag: orbit  ·  Middle drag: pan  ·  Wheel: zoom  ·  F: fit", Version)
}

// Automation can navigate and press the visible Edit button, but cannot
// bypass viewer mode with a modeling command or a save operation.
func viewerAllowsOp(name string) bool {
	switch name {
	case "camera.view", "camera.frame", "camera.orbit", "camera.lookat", "camera.zoom", "camera.project",
		"hover", "click", "drag", "drag.release", "wheel", "ui.key", "wait", "settle", "shot", "pick", "dump", "view.ao", "view.shading":
		return true
	}
	return false
}
