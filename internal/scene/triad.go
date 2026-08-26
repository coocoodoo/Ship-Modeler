package scene

import (
	"image/color"
	"math"
	"sort"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// The axis triad (SPEC-UX §6.2): three coloured arrows with X/Y/Z letters,
// bottom-right of the viewport, mirroring the live camera. Display only in v1,
// drawn in the overlay layer so the model never occludes it.
//
// Like the view cube it is projected orthographically and drawn with 2D
// primitives, with the arrows depth-sorted so the ones pointing away dim.

// Triad draws the axis indicator.
type Triad struct {
	Rect rl.Rectangle
}

// Layout positions the triad in the bottom-right of the viewport.
func (t *Triad) Layout(vp render.Viewport, scale float64) {
	size := float32(ui.TriadSize * scale)
	margin := float32(ui.TriadMargin * scale)
	t.Rect = rl.Rectangle{
		X:      float32(vp.X+vp.W) - size - margin,
		Y:      float32(vp.Y+vp.H) - size - margin,
		Width:  size,
		Height: size,
	}
}

type triadAxis struct {
	dir   geom.Vec3
	col   color.RGBA
	label string
	depth float64
	tip   rl.Vector2
}

// Draw paints the triad for the given camera.
func (t *Triad) Draw(cam render.Camera, fonts *ui.Fonts, scale float64) {
	center := rl.Vector2{X: t.Rect.X + t.Rect.Width/2, Y: t.Rect.Y + t.Rect.Height/2}
	radius := float64(t.Rect.Width) * 0.36

	right, up, fwd := cam.Right(), cam.Up(), cam.Forward()
	project := func(p geom.Vec3) rl.Vector2 {
		return rl.Vector2{
			X: center.X + float32(p.Dot(right)*radius),
			Y: center.Y - float32(p.Dot(up)*radius),
		}
	}

	axes := make([]triadAxis, 0, 3)
	for i := 0; i < 3; i++ {
		d := geom.UnitAxis(i)
		axes = append(axes, triadAxis{
			dir:   d,
			col:   ui.AxisColor(i),
			label: []string{"X", "Y", "Z"}[i],
			depth: d.Dot(fwd),
			tip:   project(d),
		})
	}
	// Back to front: the largest dot with the forward vector is furthest away.
	sort.SliceStable(axes, func(a, b int) bool { return axes[a].depth > axes[b].depth })

	lineW := float32(math.Max(1.5, 2*scale))
	for _, a := range axes {
		col := a.col
		// Arrows pointing away from the viewer dim, which is what makes the
		// triad readable at a glance.
		if a.depth > 0 {
			col = ui.Fade(ui.Shade(col, 0.6), 0.75)
		}
		rl.DrawLineEx(center, a.tip, lineW, col)
		rl.DrawCircleV(a.tip, float32(5*scale), col)
		fonts.DrawCentered(fonts.Small, a.label, a.tip.X, a.tip.Y-float32(11*scale),
			ui.FontSizeSmall, col)
	}
	rl.DrawCircleV(center, float32(2.5*scale), ui.ColorTextDim)
}
