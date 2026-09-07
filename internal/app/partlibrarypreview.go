package app

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/geom"
	"modeler/internal/io"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// A library preview owns its meshes and camera; it never enters the document
// or its export scene. Only the selected asset is loaded and uploaded.
type partLibraryPreview struct {
	id, err string
	scene   render.Scene
	bounds  geom.AABB
	angle   float64
}

func (a *App) dropLibraryPreview() {
	for _, b := range a.library.preview.scene.Bodies {
		b.GPU.Unload()
	}
	a.library.preview = partLibraryPreview{}
}

func (a *App) loadLibraryPreview(part io.LibraryPart) {
	if a.library.preview.id == part.ID {
		return
	}
	a.dropLibraryPreview()
	p := &a.library.preview
	p.id, p.angle = part.ID, render.IsoAzimuth
	p.bounds = geom.Empty()
	bodies, err := io.LoadLibraryPart(a.library.dir, part)
	if err != nil {
		p.err = "Preview unavailable. Refresh to try again."
		return
	}
	p.scene.DimFactor = 1
	for _, b := range bodies {
		if b.Mesh == nil || b.Mesh.TriangleCount() == 0 {
			continue
		}
		g := render.BuildBodyGPU(b.Mesh)
		g.Upload()
		p.bounds = p.bounds.Union(g.Bounds)
		p.scene.Bodies = append(p.scene.Bodies, render.BodyDraw{
			GPU: g, Color: b.Color, Alpha: 1, Transform: geom.Identity(),
		})
	}
	if len(p.scene.Bodies) == 0 {
		p.err = "This part has no geometry to preview."
	}
}

func libraryPreviewCamera(bounds geom.AABB, aspect, angle float64) render.Camera {
	c := render.DefaultCamera()
	c.Target = bounds.Center()
	c.Azimuth = angle
	// Fit a sphere around the entire assembly, including off-origin parts.
	// Its size stays constant throughout the orbit, avoiding zoom pulses.
	diameter := math.Max(bounds.Diagonal(), render.MinOrthoScale)
	c.OrthoScale = diameter * (1 + render.FrameMargin) / math.Min(1, math.Max(aspect, 0.01))
	c.Dist = diameter * 2
	return c
}

func (a *App) buildLibraryPreview(rect rl.Rectangle, parts []io.LibraryPart) {
	var selected *io.LibraryPart
	for i := range parts {
		if parts[i].ID == a.library.selected {
			selected = &parts[i]
			break
		}
	}
	if selected == nil {
		a.dropLibraryPreview()
		a.UI.TextCentered(rect, "Select a part for a 3D preview", ui.FontSizeSmall, ui.ColorTextDim)
		return
	}
	a.loadLibraryPreview(*selected)
	p := &a.library.preview
	if p.err != "" {
		a.UI.TextCentered(rect, p.err, ui.FontSizeSmall, ui.ColorError)
		return
	}
	label, view := ui.SplitTop(rect, a.px(24))
	caption := "3D preview · Rotating"
	if a.UI.MotionFactor() == 0 || !a.UI.Effects.PreviewRotation {
		caption = "3D preview · Still"
	}
	a.UI.Text(label, caption, ui.FontSizeSmall, ui.ColorTextDim)
	if view.Width < 1 || view.Height < 1 {
		return
	}
	if a.UI.Effects.PreviewRotation {
		p.angle = math.Mod(p.angle+math.Max(0, a.UI.In.DeltaMillis)*0.025*a.UI.MotionFactor(), 360)
	}
	vp := render.Viewport{X: int(view.X), Y: int(view.Y), W: int(view.Width), H: int(view.Height)}
	p.scene.Camera = libraryPreviewCamera(p.bounds, vp.Aspect(), p.angle)
	a.Renderer.DrawPreview(&p.scene, vp)
	a.UI.StrokeRounded(view, 0, ui.ColorStroke)
}
