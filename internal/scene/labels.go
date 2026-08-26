package scene

import (
	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// Plane name tags (SPEC-UX §5, SPEC-RENDER §4). The tag is drawn as 2D text at
// the projected position of one plane corner rather than as world-space
// geometry, so it stays upright and crisp at every camera angle.

// planeLabelInset pulls the tag in from the corner so it sits on the quad
// rather than on its border.
const planeLabelInset = 1.6

// DrawPlaneLabels paints one name tag per visible plane. It must be called
// after the 3D pass, while 2D drawing is active.
func DrawPlaneLabels(cam render.Camera, vp render.Viewport, planes []render.PlaneDraw, fonts *ui.Fonts, scale float64) {
	if vp.W <= 0 || vp.H <= 0 {
		return
	}
	for i := range planes {
		p := &planes[i]
		if p.Label == "" {
			continue
		}
		// The tag rides the corner nearest the camera, so it is never hidden
		// behind the model at the centre of the view.
		corner, ok := nearestCorner(cam, p, vp)
		if !ok {
			continue
		}
		col := ui.WithAlpha(PlaneColor(p.Kind), 0xE0)
		if p.Selected {
			col = ui.ColorAccent
		} else if p.Hovered {
			col = ui.ColorText
		}
		fonts.DrawCentered(fonts.Small, p.Label,
			float32(corner.X+float64(vp.X)), float32(corner.Y+float64(vp.Y)),
			ui.FontSizeSmall, col)
	}
}

// nearestCorner projects the plane's four inset corners and returns the one
// closest to the camera that also lands inside the viewport.
func nearestCorner(cam render.Camera, p *render.PlaneDraw, vp render.Viewport) (geom.Vec2, bool) {
	h := p.HalfSize - planeLabelInset
	if h <= 0 {
		h = p.HalfSize
	}
	fwd := cam.Forward()

	best := geom.Vec2{}
	bestDepth := 0.0
	found := false
	for _, c := range [][2]float64{{-h, -h}, {h, -h}, {h, h}, {-h, h}} {
		world := p.Frame.ToWorld(geom.Vec2{X: c[0], Y: c[1]})
		screen, ok := cam.WorldToViewport(world, float64(vp.W), float64(vp.H))
		if !ok {
			continue
		}
		if screen.X < 0 || screen.Y < 0 || screen.X > float64(vp.W) || screen.Y > float64(vp.H) {
			continue
		}
		// A larger dot with the forward vector is further away, so the smallest
		// wins.
		depth := world.Dot(fwd)
		if !found || depth < bestDepth {
			best, bestDepth, found = screen, depth, true
		}
	}
	return best, found
}
