package scene

import (
	"image/color"
	"math"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

// The move and rotate gizmos on screen (SPEC-UX §12.2, §12.4).
//
// Everything is sized in world units derived from the camera so the gizmo comes
// out the same size in pixels at any zoom, exactly as the extrude arrow does.
// Hit testing happens in screen space against the same geometry that is drawn,
// so what you can grab is always what you can see.

// GizmoView is what the gizmo needs to know to draw and be hit.
type GizmoView struct {
	Tool *tools.TransformTool
	Cam  render.Camera
	Vp   render.Viewport
}

// scale converts the gizmo's pixel sizes into world units.
func (v GizmoView) scale() float64 {
	px := v.Cam.PixelsPerWorldUnit(float64(v.Vp.H))
	if px <= 0 {
		return 1
	}
	return 1 / px
}

// axisOf is a part's world direction, with the two in-plane directions for a
// planar handle.
func axisDirs(p tools.GizmoPart) (a, b geom.Vec3) {
	switch p {
	case tools.PartAxisX, tools.PartRingX:
		return geom.AxisX, geom.Vec3{}
	case tools.PartAxisY, tools.PartRingY:
		return geom.AxisY, geom.Vec3{}
	case tools.PartAxisZ, tools.PartRingZ:
		return geom.AxisZ, geom.Vec3{}
	case tools.PartPlaneYZ:
		return geom.AxisY, geom.AxisZ
	case tools.PartPlaneZX:
		return geom.AxisZ, geom.AxisX
	case tools.PartPlaneXY:
		return geom.AxisX, geom.AxisY
	}
	return geom.Vec3{}, geom.Vec3{}
}

// partColor is the axis colour a handle is drawn in, brightened when it is
// hovered and turned to the success colour while it is being dragged.
func partColor(p tools.GizmoPart, v GizmoView) color.RGBA {
	base := ui.ColorAccent
	if axis, ok := p.Axis(); ok {
		base = ui.AxisColor(axis.MaxAbsAxis())
	}
	switch {
	case v.Tool.Active == p:
		return ui.ColorSuccess
	case v.Tool.Hover == p:
		return ui.Shade(base, 1.4)
	}
	return base
}

// BuildTransformGizmo assembles the overlay for whichever gizmo is showing.
func BuildTransformGizmo(v GizmoView) *render.Overlay {
	if v.Tool == nil {
		return nil
	}
	if v.Tool.Mode == tools.GizmoRotate {
		return buildRotateGizmo(v)
	}
	return buildMoveGizmo(v)
}

func buildMoveGizmo(v GizmoView) *render.Overlay {
	s := v.scale()
	o := v.Tool.Pivot
	d := &render.Overlay{}

	// Planar handles first, so the arrows draw over them.
	for _, p := range []tools.GizmoPart{tools.PartPlaneYZ, tools.PartPlaneZX, tools.PartPlaneXY} {
		a, b := axisDirs(p)
		off, size := tools.GizmoPlaneOffsetPx*s, tools.GizmoPlaneSizePx*s
		c0 := o.Add(a.Mul(off)).Add(b.Mul(off))
		c1 := c0.Add(a.Mul(size))
		c2 := c1.Add(b.Mul(size))
		c3 := c0.Add(b.Mul(size))
		col := partColor(p, v)
		fill := ui.WithAlpha(col, 0x55)
		d.Fills = append(d.Fills,
			render.OverlayTri{A: c0, B: c1, C: c2, Color: fill},
			render.OverlayTri{A: c0, B: c2, C: c3, Color: fill})
		for _, e := range [4][2]geom.Vec3{{c0, c1}, {c1, c2}, {c2, c3}, {c3, c0}} {
			d.Lines = append(d.Lines, render.OverlayLine{
				A: e[0], B: e[1], Color: col, WidthPx: 1.5,
			})
		}
	}

	for _, p := range []tools.GizmoPart{tools.PartAxisX, tools.PartAxisY, tools.PartAxisZ} {
		axis, _ := axisDirs(p)
		length := tools.GizmoAxisLengthPx * s
		col := partColor(p, v)
		tip := o.Add(axis.Mul(length))
		neck := o.Add(axis.Mul(length * 0.78))
		width := 3.0
		if v.Tool.Hover == p || v.Tool.Active == p {
			width = 4.0
		}
		d.Lines = append(d.Lines, render.OverlayLine{A: o, B: neck, Color: col, WidthPx: width})
		appendCone(d, neck, tip, axis, col)
	}

	d.Markers = append(d.Markers, render.OverlayMarker{
		P: o, Kind: render.MarkerVertex, Color: partColor(tools.PartScreen, v),
		SizePx: tools.GizmoCenterRadiusPx,
	})
	return d
}

// ringSegments is how many chords each rotation ring is drawn with. Enough that
// the circle reads as one at any size the gizmo is ever drawn.
const ringSegments = 64

func buildRotateGizmo(v GizmoView) *render.Overlay {
	s := v.scale()
	o := v.Tool.Pivot
	r := tools.GizmoRingRadiusPx * s
	d := &render.Overlay{}

	for _, p := range tools.RotateParts() {
		axis, _ := axisDirs(p)
		f := geom.FrameFromNormal(o, axis)
		col := partColor(p, v)
		width := 2.5
		if v.Tool.Hover == p || v.Tool.Active == p {
			width = 4.0
		}
		prev := o.Add(f.U.Mul(r))
		for i := 1; i <= ringSegments; i++ {
			a := 2 * math.Pi * float64(i) / ringSegments
			cur := o.Add(f.U.Mul(r * math.Cos(a))).Add(f.V.Mul(r * math.Sin(a)))
			d.Lines = append(d.Lines, render.OverlayLine{
				A: prev, B: cur, Color: col, WidthPx: width,
			})
			prev = cur
		}
	}
	d.Markers = append(d.Markers, render.OverlayMarker{
		P: o, Kind: render.MarkerVertex, Color: ui.ColorTextDim, SizePx: 5,
	})
	return d
}

// GizmoHit finds the handle under the pointer, in screen space.
//
// The order matters and matches the drawing: the centre handle sits on top of
// everything, then the axis arrows, then the planar squares underneath. A
// pointer near where two handles overlap gets the one drawn on top, which is
// the one it looks like it is on.
func GizmoHit(v GizmoView, mouseX, mouseY float64) tools.GizmoPart {
	if v.Tool == nil {
		return tools.PartNone
	}
	s := v.scale()
	o := v.Tool.Pivot

	if v.Tool.Mode == tools.GizmoRotate {
		return ringHit(v, s, mouseX, mouseY)
	}

	if p, ok := v.Cam.WorldToViewport(o, float64(v.Vp.W), float64(v.Vp.H)); ok {
		cx, cy := p.X+float64(v.Vp.X), p.Y+float64(v.Vp.Y)
		if math.Hypot(mouseX-cx, mouseY-cy) <= tools.GizmoCenterRadiusPx {
			return tools.PartScreen
		}
	}

	best, bestDist := tools.PartNone, math.Inf(1)
	for _, part := range []tools.GizmoPart{tools.PartAxisX, tools.PartAxisY, tools.PartAxisZ} {
		axis, _ := axisDirs(part)
		ax, ay, bx, by, ok := ArrowScreenEnds(v.Cam, v.Vp, o, axis, tools.GizmoAxisLengthPx*s)
		if !ok {
			continue
		}
		dist, along := tools.AxisDistancePx(mouseX, mouseY, ax, ay, bx, by)
		length := math.Hypot(bx-ax, by-ay)
		// Only the drawn part of the shaft is grabbable: the projection runs on
		// past the tip, and grabbing thin air out there would be baffling.
		if along < -tools.GizmoGrabPx || along > length+tools.GizmoGrabPx {
			continue
		}
		if dist <= tools.GizmoGrabPx && dist < bestDist {
			best, bestDist = part, dist
		}
	}
	if best != tools.PartNone {
		return best
	}

	for _, part := range []tools.GizmoPart{tools.PartPlaneYZ, tools.PartPlaneZX, tools.PartPlaneXY} {
		if planeHandleHit(v, part, s, mouseX, mouseY) {
			return part
		}
	}
	return tools.PartNone
}

// planeHandleHit tests the pointer against a planar square, projected.
func planeHandleHit(v GizmoView, part tools.GizmoPart, s, mouseX, mouseY float64) bool {
	a, b := axisDirs(part)
	off, size := tools.GizmoPlaneOffsetPx*s, tools.GizmoPlaneSizePx*s
	o := v.Tool.Pivot
	corners := [4]geom.Vec3{
		o.Add(a.Mul(off)).Add(b.Mul(off)),
		o.Add(a.Mul(off + size)).Add(b.Mul(off)),
		o.Add(a.Mul(off + size)).Add(b.Mul(off + size)),
		o.Add(a.Mul(off)).Add(b.Mul(off + size)),
	}
	var poly [4]geom.Vec2
	for i, c := range corners {
		p, ok := v.Cam.WorldToViewport(c, float64(v.Vp.W), float64(v.Vp.H))
		if !ok {
			return false
		}
		poly[i] = geom.Vec2{X: p.X + float64(v.Vp.X), Y: p.Y + float64(v.Vp.Y)}
	}
	return pointInProjectedQuad(poly, geom.Vec2{X: mouseX, Y: mouseY})
}

// pointInProjectedQuad is a winding test over a projected quad.
func pointInProjectedQuad(q [4]geom.Vec2, p geom.Vec2) bool {
	in := false
	for i := range q {
		a, b := q[i], q[(i+1)%4]
		if (a.Y > p.Y) != (b.Y > p.Y) &&
			p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// ringHit finds which rotation ring the pointer is on, by measuring against the
// drawn chords rather than against an idealised circle: a ring seen edge-on is
// a line, and the ellipse it projects to has no closed-form distance worth
// writing when the thing it is approximating is already a polyline.
func ringHit(v GizmoView, s, mouseX, mouseY float64) tools.GizmoPart {
	o := v.Tool.Pivot
	r := tools.GizmoRingRadiusPx * s
	best, bestDist := tools.PartNone, math.Inf(1)

	for _, part := range tools.RotateParts() {
		axis, _ := axisDirs(part)
		f := geom.FrameFromNormal(o, axis)
		prev, ok := v.Cam.WorldToViewport(o.Add(f.U.Mul(r)), float64(v.Vp.W), float64(v.Vp.H))
		if !ok {
			continue
		}
		for i := 1; i <= ringSegments; i++ {
			a := 2 * math.Pi * float64(i) / ringSegments
			world := o.Add(f.U.Mul(r * math.Cos(a))).Add(f.V.Mul(r * math.Sin(a)))
			cur, ok := v.Cam.WorldToViewport(world, float64(v.Vp.W), float64(v.Vp.H))
			if !ok {
				continue
			}
			d, _ := tools.AxisDistancePxClamped(mouseX, mouseY,
				prev.X+float64(v.Vp.X), prev.Y+float64(v.Vp.Y),
				cur.X+float64(v.Vp.X), cur.Y+float64(v.Vp.Y))
			if d < bestDist {
				best, bestDist = part, d
			}
			prev = cur
		}
	}
	if bestDist <= tools.GizmoGrabPx {
		return best
	}
	return tools.PartNone
}
