package scene

import (
	"image/color"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// PlaneHalfSize is half the edge length of a default plane's quad: the planes
// are drawn as 24x24 u bounded rectangles centred at the origin (SPEC-UX §5).
const PlaneHalfSize = 12.0

// PlaneTintAlpha and PlaneBorderAlpha are the fill and border opacities of a
// default plane (SPEC-UX §3).
const (
	PlaneTintAlpha   = 0x1A // 10%
	PlaneBorderAlpha = 0x4D // 30%
)

// PlaneColor returns the axis tint of a default plane: each plane takes the
// colour of the axis it faces (SPEC-UX §3).
func PlaneColor(k geom.PlaneKind) color.RGBA {
	switch k {
	case geom.PlaneTop:
		return ui.ColorAxisY
	case geom.PlaneRight:
		return ui.ColorAxisX
	default:
		return ui.ColorAxisZ
	}
}

// PlaneVisibility is the show/hide state of the three default planes, which
// exist forever and can only be hidden (R1).
type PlaneVisibility [geom.PlaneCount]bool

// AllVisible returns the default state: every plane shown.
func AllVisible() PlaneVisibility {
	var v PlaneVisibility
	for i := range v {
		v[i] = true
	}
	return v
}

// BuildPlaneDraws turns plane visibility into the renderer's draw list.
// hovered and selected may be -1 for "none".
func BuildPlaneDraws(vis PlaneVisibility, hovered, selected geom.PlaneKind, hasHover, hasSel bool) []render.PlaneDraw {
	out := make([]render.PlaneDraw, 0, geom.PlaneCount)
	for i := 0; i < geom.PlaneCount; i++ {
		k := geom.PlaneKind(i)
		if !vis[i] {
			continue
		}
		out = append(out, render.PlaneDraw{
			Kind:     k,
			Frame:    geom.PlaneFrame(k),
			HalfSize: PlaneHalfSize,
			Color:    ui.WithAlpha(PlaneColor(k), PlaneTintAlpha),
			Label:    k.String(),
			Hovered:  hasHover && hovered == k,
			Selected: hasSel && selected == k,
			Pickable: true,
		})
	}
	return out
}

// SketchGrid builds the grid draw for a sketch plane: minor lines every unit,
// major every eight, with the frame's U and V axes tinted by the world axis
// they follow (SPEC-UX §8.1).
func SketchGrid(f geom.Frame, halfSize, alpha float64) *render.GridDraw {
	return &render.GridDraw{
		Frame:      f,
		HalfSize:   halfSize,
		MinorStep:  1,
		MajorStep:  8,
		Alpha:      alpha,
		ShowAxes:   true,
		AxisUColor: ui.Fade(axisTint(f.U), 0.8),
		AxisVColor: ui.Fade(axisTint(f.V), 0.8),
	}
}

// axisTint picks the world-axis colour a direction is closest to.
func axisTint(d geom.Vec3) color.RGBA {
	return ui.AxisColor(d.MaxAbsAxis())
}

// FrameAll returns the bounding box the F key should frame: the given scene's
// bodies, or the default planes when the document is empty.
func FrameAll(s *render.Scene) geom.AABB {
	b := s.SceneBounds()
	if b.Valid() {
		return b
	}
	return geom.AABB{
		Min: geom.Vec3{X: -PlaneHalfSize, Y: -PlaneHalfSize, Z: -PlaneHalfSize},
		Max: geom.Vec3{X: PlaneHalfSize, Y: PlaneHalfSize, Z: PlaneHalfSize},
	}
}
