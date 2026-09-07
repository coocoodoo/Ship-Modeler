package app

import (
	"math"
	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

func (a *App) setExtrudeExtent(e tools.Extent) {
	if !a.InExtrude() {
		return
	}
	a.extrude.tool.SetExtent(e)
	a.extrude.targetRef = render.PickRef{}
	a.extrude.previewKey = ""
	a.Hover = render.PickResult{}
	a.rebuildExtrudePreview()
}

func (a *App) onExtrudeTargetEvent(ev model.Event) {
	if !a.InExtrude() || a.extrude.tool.Extent == tools.ExtentDistance {
		return
	}
	if ev.Kind != model.EvDocReplaced && (ev.BodyID != a.extrude.targetRef.BodyID || (ev.Kind != model.EvBodyChanged && ev.Kind != model.EvBodyRemoved && ev.Kind != model.EvBodyAdded)) {
		return
	}
	a.extrude.tool.TargetReady = false
	a.extrude.targetRef = render.PickRef{}
	a.Hover = render.PickResult{}
	a.dropExtrudePreview()
	a.extrude.previewMesh = nil
	a.extrude.previewErr = "Target changed — select it again"
}
func extentPick(e tools.Extent) render.PickKind {
	switch e {
	case tools.ExtentFace:
		return render.PickFace
	case tools.ExtentVertex:
		return render.PickVert
	case tools.ExtentEdge:
		return render.PickEdge
	}
	return render.PickNone
}

// Pick original document bodies; boolean result previews must not hide the
// very face being used as the stop. The pending extrusion never picks itself.
func (a *App) updateExtrudeTarget(in InputFrame, vp render.Viewport) {
	if a.cubeOwnsPointer(in) {
		return
	}
	if !in.Pressed[MouseLeft] && in.MouseDX == 0 && in.MouseDY == 0 && in.Wheel == 0 {
		return
	}
	t := a.extrude.tool
	s := render.Scene{Camera: a.Camera, PickOnly: extentPick(t.Extent)}
	for _, b := range a.Doc().Bodies {
		if b.Visible && b.Mesh != nil {
			s.Bodies = append(s.Bodies, render.BodyDraw{GPU: a.bodyGPU(b), BodyID: b.ID, Pickable: true, Transform: geom.Identity()})
		}
	}
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	if !hit.Hit || hit.Kind != extentPick(t.Extent) {
		a.Hover = render.PickResult{}
		return
	}
	a.Hover = hit
	if !in.Pressed[MouseLeft] {
		return
	}
	b := a.Doc().BodyByID(hit.BodyID)
	point, normal, ok := extrudeTargetGeometry(b, hit.PickRef)
	if !ok {
		return
	}
	if hit.Kind == render.PickEdge {
		edge := b.Mesh.Topo().Edges[hit.Edge]
		p, q := b.Mesh.Verts[edge.A], b.Mesh.Verts[edge.B]
		origin, dir := a.Renderer.ScreenRay(a.Camera, vp, in.MouseX, in.MouseY)
		v, w := q.Sub(p), p.Sub(origin)
		den := v.LenSq() - math.Pow(v.Dot(dir), 2)
		if den > 1e-12 {
			u := (v.Dot(dir)*w.Dot(dir) - v.Dot(w)) / den
			u = math.Max(0, math.Min(1, u))
			point = p.Add(v.Mul(u))
		}
	}
	if err := t.SetTarget(point, normal); err != nil {
		a.Toast(ui.Toast{Text: err.Error(), Kind: ui.ToastWarn})
		return
	}
	a.extrude.targetRef = hit.PickRef
	a.rebuildExtrudePreview()
}

func extrudeTargetGeometry(b *model.Body, ref render.PickRef) (geom.Vec3, geom.Vec3, bool) {
	if b == nil || b.Mesh == nil {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	m := b.Mesh
	switch ref.Kind {
	case render.PickFace:
		for i, f := range m.Faces {
			if f.ID == ref.FaceUID {
				return m.FaceCentroid(i), m.FaceNormal(i), true
			}
		}
	case render.PickVert:
		if ref.Vert >= 0 && ref.Vert < len(m.Verts) {
			return m.Verts[ref.Vert], geom.Vec3{}, true
		}
	case render.PickEdge:
		es := m.Topo().Edges
		if ref.Edge >= 0 && ref.Edge < len(es) {
			e := es[ref.Edge]
			return m.Verts[e.A].Add(m.Verts[e.B]).Mul(.5), geom.Vec3{}, true
		}
	}
	return geom.Vec3{}, geom.Vec3{}, false
}

func (a *App) extrudeTargetOverlay() *render.Overlay {
	t := a.extrude.tool
	d := &render.Overlay{}
	ref := a.extrude.targetRef
	if a.Hover.Hit && a.Hover.Kind == extentPick(t.Extent) {
		ref = a.Hover.PickRef
	}
	b := a.Doc().BodyByID(ref.BodyID)
	if b != nil && b.Mesh != nil {
		if ref.Kind == render.PickEdge {
			e := b.Mesh.Topo().Edges[ref.Edge]
			d.Lines = append(d.Lines, render.OverlayLine{A: b.Mesh.Verts[e.A], B: b.Mesh.Verts[e.B], Color: ui.ColorSuccess, WidthPx: 3})
		}
		if ref.Kind == render.PickFace {
			for _, f := range b.Mesh.Faces {
				if f.ID == ref.FaceUID {
					for _, loop := range f.Loops {
						for i, v := range loop {
							d.Lines = append(d.Lines, render.OverlayLine{A: b.Mesh.Verts[v], B: b.Mesh.Verts[loop[(i+1)%len(loop)]], Color: ui.ColorSuccess, WidthPx: 2})
						}
					}
				}
			}
		}
		if ref.Kind == render.PickVert {
			d.Markers = append(d.Markers, render.OverlayMarker{P: b.Mesh.Verts[ref.Vert], Kind: render.MarkerRing, Color: ui.ColorSuccess, SizePx: 12})
		}
	}
	if t.TargetReady {
		d.Markers = append(d.Markers, render.OverlayMarker{P: t.TargetPoint, Kind: render.MarkerRing, Color: ui.ColorSuccess, SizePx: 10})
	}
	return d
}
