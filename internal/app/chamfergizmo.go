package app

import (
	"fmt"
	"math"
	"sort"

	"modeler/internal/geom"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

const chamferGizmoStep = .25

type chamferGizmoState struct {
	dragging, hovered, captured, moved bool
	startX, startY, startDistance      float64
	dx, dy                             float64
	view                               scene.ArrowView
}

// Anchor to a deterministic selected edge; map iteration must never make the
// handle jump between edges while changing a multi-edge chamfer.
func (a *App) chamferArrow(vp render.Viewport) (scene.ArrowView, bool) {
	if !a.InChamfer() {
		return scene.ArrowView{}, false
	}
	g := &a.chamfer.gizmo
	if g.dragging {
		v := g.view
		v.Dragging = true
		return v, true
	}
	ids := make([]uint32, 0, len(a.chamfer.edges))
	for id := range a.chamfer.edges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		b := a.Doc().BodyByID(id)
		if b == nil || b.Mesh == nil || !b.Visible {
			continue
		}
		for _, ei := range a.chamfer.edges[id] {
			t := b.Mesh.Topo()
			if ei < 0 || ei >= len(t.Edges) {
				continue
			}
			e := t.Edges[ei]
			if !e.Manifold() {
				continue
			}
			origin := b.Mesh.Verts[e.A].Add(b.Mesh.Verts[e.B]).Mul(.5)
			// Follow the midpoint of the two equal face offsets, just as the
			// geometry does: into an outside corner, out into an inside corner.
			edgeDir := b.Mesh.Verts[e.B].Sub(b.Mesh.Verts[e.A]).Normalize()
			dir := geom.Vec3{}
			for _, use := range e.Uses {
				along := edgeDir
				if !use.Forward {
					along = along.Neg()
				}
				dir = dir.Add(b.Mesh.FaceNormal(use.Face).Cross(along))
			}
			dir = dir.Normalize()
			length := a.arrowLength(vp) * math.Max(1, a.Scale)
			ax, ay, bx, by, ok := scene.ArrowScreenEnds(a.Camera, vp, origin, dir, length)
			if !ok {
				continue
			}
			pixels := math.Hypot(bx-ax, by-ay)
			if pixels < 20*math.Max(1, a.Scale) {
				// A head-on normal otherwise collapses to an un-draggable dot.
				dir = a.Camera.Up()
				ax, ay, bx, by, ok = scene.ArrowScreenEnds(a.Camera, vp, origin, dir, length)
				if !ok {
					continue
				}
				pixels = math.Hypot(bx-ax, by-ay)
			}
			if pixels < 1 {
				continue
			}
			length *= tools.ArrowScreenLength * math.Max(1, a.Scale) / pixels
			return scene.ArrowView{Origin: origin, Dir: dir, LengthWorld: length, Hovered: g.hovered}, true
		}
	}
	return scene.ArrowView{}, false
}

func chamferDragDistance(start, along, scale float64) float64 {
	// Twelve logical pixels per quarter unit is independent of zoom. Snapping
	// the absolute distance also puts a previously typed value onto this grid.
	v := start + along*chamferGizmoStep/(12*math.Max(1, scale))
	return math.Max(chamferGizmoStep, math.Min(10000, math.Round(v/chamferGizmoStep)*chamferGizmoStep))
}

func (a *App) updateChamferGizmo(in InputFrame, vp render.Viewport) bool {
	g := &a.chamfer.gizmo
	if g.dragging {
		g.captured = true
		along := (in.MouseX-g.startX)*g.dx + (in.MouseY-g.startY)*g.dy
		if math.Abs(along) > 1 {
			g.moved = true
		}
		if g.moved {
			v := chamferDragDistance(g.startDistance, along, a.Scale)
			if v != a.chamfer.distance {
				a.setChamferDistance(v)
			}
		}
		if !in.Down[MouseLeft] {
			g.dragging = false
			if a.chamfer.dirty {
				a.rebuildChamferPreview()
			}
		}
		return true
	}
	view, ok := a.chamferArrow(vp)
	if !ok || a.cubeOwnsPointer(in) || a.orbiting || a.panning {
		return false
	}
	ax, ay, bx, by, ok := scene.ArrowScreenEnds(a.Camera, vp, view.Origin, view.Dir, view.LengthWorld)
	if !ok {
		return false
	}
	distance, along := tools.AxisDistancePx(in.MouseX, in.MouseY, ax, ay, bx, by)
	length := math.Hypot(bx-ax, by-ay)
	// Leave the edge midpoint available for removing it from the selection.
	g.hovered = distance <= tools.ArrowGrabRadiusPx*math.Max(1, a.Scale) && along >= 12*math.Max(1, a.Scale) && along <= length+8*math.Max(1, a.Scale)
	if !g.hovered || !in.Pressed[MouseLeft] {
		return false
	}
	a.Anim.Cancel()
	g.dragging, g.captured = true, true
	g.moved = false
	g.startX, g.startY, g.startDistance = in.MouseX, in.MouseY, a.chamfer.distance
	g.dx, g.dy, g.view = (bx-ax)/length, (by-ay)/length, view
	return true
}

func (a *App) buildChamferGizmo(vp render.Viewport) *render.Overlay {
	view, ok := a.chamferArrow(vp)
	if !ok {
		return nil
	}
	return scene.BuildArrowGizmo(view)
}

func (a *App) drawChamferGizmoLabel(vp render.Viewport) {
	view, ok := a.chamferArrow(vp)
	if !ok {
		return
	}
	_, _, x, y, ok := scene.ArrowScreenEnds(a.Camera, vp, view.Origin, view.Dir, view.LengthWorld)
	if !ok {
		return
	}
	w, h := a.px(152), a.px(24)
	x = math.Max(float64(vp.X), math.Min(x+10*a.Scale, float64(vp.X+vp.W)-float64(w)))
	y = math.Max(float64(vp.Y), math.Min(y-float64(h)/2, float64(vp.Y+vp.H)-float64(h)))
	box := ui.Rect(float32(x), float32(y), w, h)
	a.UI.FillRounded(box, ui.CornerRadius, ui.ColorCard)
	a.UI.Text(ui.InsetXY(box, a.px(6), 0), fmt.Sprintf("%.2f u · 0.25 u steps", a.chamfer.distance), ui.FontSizeSmall, ui.ColorText)
}
