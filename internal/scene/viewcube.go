// Package scene composes the viewport: the default planes, the sketch grid,
// the view cube, the axis triad and the camera animator. It turns document
// state into the draw lists that internal/render consumes.
package scene

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// The view cube (SPEC-UX §6.1): 26 hit zones — 6 faces, 12 edges, 8 corners —
// that snap the camera to a canonical orientation on click and orbit it 1:1 on
// drag.
//
// SPEC-RENDER §6.3 describes picking the zones with a dedicated ID render. The
// cube is a unit cube drawn orthographically, so its projected sub-quads are
// exact 2D polygons: hit-testing them directly is both cheaper (no GPU readback
// per hover) and exactly consistent with what is drawn. Logged in DECISIONS.

// ZoneThreshold splits each cube face into its 3x3 grid: a point on the surface
// belongs to a corner or edge zone when its off-axis coordinates exceed it.
const ZoneThreshold = 0.62

// CubeZone identifies one of the 26 zones by a vector of -1, 0 and 1 per axis.
type CubeZone struct{ X, Y, Z int }

// Valid reports whether the zone is one of the 26 (the all-zero vector is not).
func (z CubeZone) Valid() bool { return z != CubeZone{} }

// Direction is the outward direction the camera looks from for this zone.
func (z CubeZone) Direction() geom.Vec3 {
	return geom.Vec3{X: float64(z.X), Y: float64(z.Y), Z: float64(z.Z)}.Normalize()
}

// Label returns the face name for a face zone, or "" for edges and corners.
func (z CubeZone) Label() string {
	switch z {
	case CubeZone{Z: 1}:
		return "FRONT"
	case CubeZone{Z: -1}:
		return "BACK"
	case CubeZone{X: 1}:
		return "RIGHT"
	case CubeZone{X: -1}:
		return "LEFT"
	case CubeZone{Y: 1}:
		return "TOP"
	case CubeZone{Y: -1}:
		return "BOTTOM"
	}
	return ""
}

// IsFace reports whether exactly one axis is non-zero.
func (z CubeZone) IsFace() bool {
	return abs(z.X)+abs(z.Y)+abs(z.Z) == 1
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ViewCube draws and hit-tests the cube. It holds no camera state of its own:
// the caller passes the live camera every frame so the cube always mirrors it,
// including mid-animation.
type ViewCube struct {
	// Rect is the cube's panel in window pixels, recomputed each frame.
	Rect     rl.Rectangle
	HomeRect rl.Rectangle

	Hover     CubeZone
	HoverHome bool

	quads []cubeQuad
}

type cubeQuad struct {
	zone  CubeZone
	pts   [4]rl.Vector2
	depth float64
	face  CubeZone // the face this sub-quad belongs to
}

// Layout positions the cube in the top-right of the viewport and rebuilds its
// projected geometry for the current camera.
func (c *ViewCube) Layout(cam render.Camera, vp render.Viewport, scale float64) {
	size := float32(ui.ViewCubeSize * scale)
	margin := float32(ui.ViewCubeMargin * scale)
	c.Rect = rl.Rectangle{
		X:      float32(vp.X+vp.W) - size - margin,
		Y:      float32(vp.Y) + margin,
		Width:  size,
		Height: size,
	}
	home := float32(22 * scale)
	c.HomeRect = rl.Rectangle{
		X:      c.Rect.X + c.Rect.Width - home,
		Y:      c.Rect.Y + c.Rect.Height + float32(6*scale),
		Width:  home,
		Height: home,
	}
	c.build(cam)
}

// build projects the cube's visible sub-quads for the camera orientation.
func (c *ViewCube) build(cam render.Camera) {
	c.quads = c.quads[:0]

	center := rl.Vector2{
		X: c.Rect.X + c.Rect.Width/2,
		Y: c.Rect.Y + c.Rect.Height/2,
	}
	// A unit cube spans +-1; leave room for the corner bevels and the label.
	pixPerUnit := float64(c.Rect.Width) / 2.6

	right, up, fwd := cam.Right(), cam.Up(), cam.Forward()
	project := func(p geom.Vec3) rl.Vector2 {
		return rl.Vector2{
			X: center.X + float32(p.Dot(right)*pixPerUnit),
			Y: center.Y - float32(p.Dot(up)*pixPerUnit),
		}
	}

	bands := [3][2]float64{{-1, -ZoneThreshold}, {-ZoneThreshold, ZoneThreshold}, {ZoneThreshold, 1}}
	signOf := [3]int{-1, 0, 1}

	for axis := 0; axis < 3; axis++ {
		for _, s := range [2]float64{1, -1} {
			n := geom.UnitAxis(axis).Mul(s)
			if n.Dot(fwd) > -0.05 {
				continue // facing away from the eye
			}
			u := (axis + 1) % 3
			v := (axis + 2) % 3
			faceZone := zoneFromAxis(axis, int(s))
			for i, bu := range bands {
				for j, bv := range bands {
					corner := func(a, b float64) geom.Vec3 {
						var p geom.Vec3
						setAxis(&p, axis, s)
						setAxis(&p, u, a)
						setAxis(&p, v, b)
						return p
					}
					p0 := corner(bu[0], bv[0])
					p1 := corner(bu[1], bv[0])
					p2 := corner(bu[1], bv[1])
					p3 := corner(bu[0], bv[1])
					zone := faceZone
					setZoneAxis(&zone, u, signOf[i])
					setZoneAxis(&zone, v, signOf[j])
					c.quads = append(c.quads, cubeQuad{
						zone:  zone,
						face:  faceZone,
						pts:   [4]rl.Vector2{project(p0), project(p1), project(p2), project(p3)},
						depth: p0.Add(p1).Add(p2).Add(p3).Mul(0.25).Dot(fwd),
					})
				}
			}
		}
	}
}

func zoneFromAxis(axis, s int) CubeZone {
	var z CubeZone
	setZoneAxis(&z, axis, s)
	return z
}

func setZoneAxis(z *CubeZone, axis, v int) {
	switch axis {
	case 0:
		z.X = v
	case 1:
		z.Y = v
	default:
		z.Z = v
	}
}

func setAxis(p *geom.Vec3, axis int, v float64) {
	switch axis {
	case 0:
		p.X = v
	case 1:
		p.Y = v
	default:
		p.Z = v
	}
}

// HitTest returns the zone under a window pixel, and whether the home button is
// under it. Only the nearest overlapping quad counts.
func (c *ViewCube) HitTest(x, y float64) (CubeZone, bool) {
	pt := rl.Vector2{X: float32(x), Y: float32(y)}
	if rl.CheckCollisionPointRec(pt, c.HomeRect) {
		return CubeZone{}, true
	}
	best := CubeZone{}
	bestDepth := math.Inf(1)
	for i := range c.quads {
		q := &c.quads[i]
		if q.depth < bestDepth && pointInQuad(pt, q.pts) {
			best, bestDepth = q.zone, q.depth
		}
	}
	return best, false
}

// Contains reports whether a window pixel is over the cube widget at all, so
// the viewport can stop treating the drag as an orbit of the model.
func (c *ViewCube) Contains(x, y float64) bool {
	pt := rl.Vector2{X: float32(x), Y: float32(y)}
	return rl.CheckCollisionPointRec(pt, c.Rect) || rl.CheckCollisionPointRec(pt, c.HomeRect)
}

// Update refreshes the hover state from the cursor position.
func (c *ViewCube) Update(x, y float64) {
	c.Hover, c.HoverHome = c.HitTest(x, y)
}

// Draw paints the cube and its home button.
func (c *ViewCube) Draw(fonts *ui.Fonts, scale float64) {
	rl.DisableBackfaceCulling()
	// Painter's algorithm: a convex cube's visible sub-quads never interpenetrate.
	order := make([]int, len(c.quads))
	for i := range order {
		order[i] = i
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && c.quads[order[j]].depth > c.quads[order[j-1]].depth; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}

	for _, idx := range order {
		q := &c.quads[idx]
		fill := cubeFaceColor(q.face)
		if c.Hover.Valid() && q.zone == c.Hover {
			fill = ui.ColorAccent
		}
		fillQuad(q.pts, fill)
		outlineQuad(q.pts, ui.WithAlpha(ui.ColorStroke, 0x80), scale)
	}

	// Labels last so they sit on top of every sub-quad.
	for _, idx := range order {
		q := &c.quads[idx]
		label := q.zone.Label()
		if label == "" || q.zone != q.face {
			continue
		}
		cx := (q.pts[0].X + q.pts[2].X) / 2
		cy := (q.pts[0].Y + q.pts[2].Y) / 2
		fonts.DrawCentered(fonts.Small, label, cx, cy, ui.FontSizeSmall, ui.ColorText)
	}
	rl.EnableBackfaceCulling()

	c.drawHome(scale)
}

func (c *ViewCube) drawHome(scale float64) {
	bg := ui.ColorCard
	if c.HoverHome {
		bg = ui.ColorAccent
	}
	rl.DrawRectangleRounded(c.HomeRect, 0.3, 4, bg)
	rl.DrawRectangleRoundedLines(c.HomeRect, 0.3, 4, ui.ColorStroke)
	ui.DrawHomeIcon(
		float64(c.HomeRect.X+c.HomeRect.Width/2),
		float64(c.HomeRect.Y+c.HomeRect.Height/2),
		12*scale, ui.ColorText)
}

// cubeFaceColor tints each cube face with the axis it faces, dimmed so the
// labels stay legible.
func cubeFaceColor(face CubeZone) color.RGBA {
	var axis int
	switch {
	case face.X != 0:
		axis = 0
	case face.Y != 0:
		axis = 1
	default:
		axis = 2
	}
	c := ui.Shade(ui.AxisColor(axis), 0.42)
	if face.X < 0 || face.Y < 0 || face.Z < 0 {
		c = ui.Shade(c, 0.75) // back-side faces read a touch darker
	}
	return ui.WithAlpha(c, 0xF0)
}

func fillQuad(p [4]rl.Vector2, col color.RGBA) {
	rl.DrawTriangle(p[0], p[1], p[2], col)
	rl.DrawTriangle(p[0], p[2], p[3], col)
	rl.DrawTriangle(p[2], p[1], p[0], col)
	rl.DrawTriangle(p[3], p[2], p[0], col)
}

func outlineQuad(p [4]rl.Vector2, col color.RGBA, scale float64) {
	w := float32(math.Max(1, scale))
	for i := 0; i < 4; i++ {
		rl.DrawLineEx(p[i], p[(i+1)%4], w, col)
	}
}

func pointInQuad(pt rl.Vector2, q [4]rl.Vector2) bool {
	inside := false
	for i, j := 0, 3; i < 4; j, i = i, i+1 {
		if (q[i].Y > pt.Y) != (q[j].Y > pt.Y) {
			x := (q[j].X-q[i].X)*(pt.Y-q[i].Y)/(q[j].Y-q[i].Y) + q[i].X
			if pt.X < x {
				inside = !inside
			}
		}
	}
	return inside
}
