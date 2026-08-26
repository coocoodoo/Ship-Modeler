package scene

import (
	"image/color"
	"math"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// The extrude arrow gizmo (SPEC-UX §9.2): a shaft and a cone at the region's
// centroid along the sketch normal, a constant ~90 px long so it is equally
// grabbable at any zoom, brightening when the pointer is on it.

// Arrow proportions, as fractions of its screen length.
const (
	arrowShaftWidthPx = 3.0
	arrowHeadFraction = 0.28
	arrowHeadWidthPx  = 11.0
	arrowBaseDotPx    = 7.0
	arrowConeSegments = 10
)

// ArrowView is what the gizmo needs to know to draw itself.
type ArrowView struct {
	// Origin is where the shaft starts and Dir is the way it points.
	Origin geom.Vec3
	Dir    geom.Vec3
	// LengthWorld is the shaft's length in world units, worked out by the
	// caller from the camera so it comes out a constant size on screen.
	LengthWorld float64
	// Hovered and Dragging brighten the arrow as the pointer engages it.
	Hovered  bool
	Dragging bool
}

// BuildArrowGizmo assembles the arrow's overlay.
func BuildArrowGizmo(v ArrowView) *render.Overlay {
	dir, ok := v.Dir.NormalizeOK()
	if !ok || v.LengthWorld <= 0 {
		return nil
	}
	col := ui.ColorAccent
	switch {
	case v.Dragging:
		col = ui.ColorSuccess
	case v.Hovered:
		col = ui.Shade(ui.ColorAccent, 1.35)
	}

	d := &render.Overlay{}
	tip := v.Origin.Add(dir.Mul(v.LengthWorld))
	neck := v.Origin.Add(dir.Mul(v.LengthWorld * (1 - arrowHeadFraction)))

	width := float64(arrowShaftWidthPx)
	if v.Hovered || v.Dragging {
		width += 1
	}
	d.Lines = append(d.Lines, render.OverlayLine{
		A: v.Origin, B: neck, Color: col, WidthPx: width,
	})

	// A dot at the base makes the origin readable even when the arrow points
	// straight at the camera and the shaft collapses to a point.
	d.Markers = append(d.Markers, render.OverlayMarker{
		P: v.Origin, Kind: render.MarkerVertex, Color: col, SizePx: arrowBaseDotPx,
	})

	appendCone(d, neck, tip, dir, col)
	return d
}

// appendCone builds the arrowhead as a fan of triangles around the axis, so it
// reads as a solid cone from any angle.
func appendCone(d *render.Overlay, base, tip, dir geom.Vec3, col color.RGBA) {
	// Any two vectors perpendicular to the axis will do; the frame convention
	// gives a deterministic pair.
	f := geom.FrameFromNormal(base, dir)
	// The head's radius is set in world units by the caller's scale, taken from
	// the head's own length so the proportions hold at every zoom.
	radius := tip.Sub(base).Len() * (arrowHeadWidthPx / (arrowHeadFraction * 90 * 2))

	prev := base.Add(f.U.Mul(radius))
	for i := 1; i <= arrowConeSegments; i++ {
		a := 2 * math.Pi * float64(i) / arrowConeSegments
		cur := base.
			Add(f.U.Mul(radius * math.Cos(a))).
			Add(f.V.Mul(radius * math.Sin(a)))
		// The side of the cone, and the disc that closes its base.
		d.Fills = append(d.Fills,
			render.OverlayTri{A: prev, B: cur, C: tip, Color: col},
			render.OverlayTri{A: cur, B: prev, C: base, Color: col})
		prev = cur
	}
}

// ArrowScreenEnds projects the arrow's shaft to window pixels, which is what
// the hit test and the drag both measure against.
func ArrowScreenEnds(cam render.Camera, vp render.Viewport, origin, dir geom.Vec3, lengthWorld float64) (ax, ay, bx, by float64, ok bool) {
	a, okA := cam.WorldToViewport(origin, float64(vp.W), float64(vp.H))
	b, okB := cam.WorldToViewport(origin.Add(dir.Mul(lengthWorld)), float64(vp.W), float64(vp.H))
	if !okA || !okB {
		return 0, 0, 0, 0, false
	}
	return a.X + float64(vp.X), a.Y + float64(vp.Y),
		b.X + float64(vp.X), b.Y + float64(vp.Y), true
}

// ArrowLengthWorld converts the gizmo's fixed screen length into world units at
// the camera's current zoom.
func ArrowLengthWorld(cam render.Camera, vp render.Viewport) float64 {
	px := cam.PixelsPerWorldUnit(float64(vp.H))
	if px <= 0 {
		return 1
	}
	return tools.ArrowScreenLength / px
}
