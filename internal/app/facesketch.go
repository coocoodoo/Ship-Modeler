package app

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/ui"
)

// Sketching on a face (R9, SPEC-UX §10).
//
// A face sketch is an ordinary sketch with its plane taken from a face instead
// of from the default three. The only thing that makes it special is what it
// remembers: the face it came from, so its edges can be snapped to and its
// outline projected, and a snapshot of the plane, so the sketch survives that
// face being cut away by a later boolean.

// faceRef names a face and the body it belongs to.
type faceRef struct {
	body *model.Body
	face int // index into body.Mesh.Faces
	uid  mesh.FaceUID
}

// resolveFace finds a face by body and identity. The identity is what selection
// and sketches store, because a face's index moves every time the body is
// edited and its FaceUID does not.
func (a *App) resolveFace(bodyID uint32, uid mesh.FaceUID) (faceRef, bool) {
	b := a.Doc().BodyByID(bodyID)
	if b == nil || b.Mesh == nil {
		return faceRef{}, false
	}
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].ID == uid {
			return faceRef{body: b, face: i, uid: uid}, true
		}
	}
	return faceRef{}, false
}

// selectedFace is the face a face-only selection points at.
func (a *App) selectedFace() (faceRef, bool) {
	ref, ok := a.Sel.Primary()
	if !ok || ref.Kind != model.SelFace {
		return faceRef{}, false
	}
	return a.resolveFace(ref.Body, ref.Face)
}

// flatEnoughToSketchOn reports whether a face can carry a sketch, and why not
// if it cannot (SPEC-UX §10).
func flatEnoughToSketchOn(m *mesh.Mesh, fi int) (bool, string) {
	if m.Faces[fi].NonPlanar || m.Planarity(fi) > geom.PlanarDist {
		return false, "That face is not flat — sketching needs a flat face"
	}
	if m.FaceArea(fi) <= 0 {
		return false, "That face has no area to sketch on"
	}
	return true, ""
}

// BeginSketchOnFace starts a sketch anchored to a body's face (SPEC-UX §10).
func (a *App) BeginSketchOnFace(bodyID uint32, uid mesh.FaceUID) bool {
	f, ok := a.resolveFace(bodyID, uid)
	if !ok {
		a.Toast(ui.Toast{Text: "That face is no longer there", Kind: ui.ToastWarn})
		return false
	}
	if ok, why := flatEnoughToSketchOn(f.body.Mesh, f.face); !ok {
		a.Toast(ui.Toast{Text: why, Kind: ui.ToastWarn})
		return false
	}

	cmd := &model.AddSketch{
		OnFace: true,
		Body:   bodyID,
		Face:   uid,
		Frame:  f.body.Mesh.FaceFrame(f.face),
	}
	if !a.Run(cmd) {
		return false
	}
	s := cmd.AddedSketch()
	a.enterSketch(s)
	a.Toast(ui.Toast{Text: "Started " + s.Name + " on a face of " + f.body.Name})
	return true
}

// faceReference is the face a sketch is anchored to, if it is still there.
//
// A sketch outlives its face — that is what the frame snapshot is for — so this
// answers "can we still snap to it", not "is the sketch valid".
func (a *App) faceReference(s *model.Sketch) (faceRef, bool) {
	if s == nil || !s.OnFace {
		return faceRef{}, false
	}
	return a.resolveFace(s.Body, s.FaceID)
}

// faceOutline2D is the anchored face's boundary in the sketch's own coordinates,
// as one loop per boundary. It feeds both the reference geometry the sketch
// draws and the Project outline button.
func (a *App) faceOutline2D(s *model.Sketch) [][]geom.Vec2i {
	f, ok := a.faceReference(s)
	if !ok {
		return nil
	}
	frame := s.Frame()
	m := f.body.Mesh
	out := make([][]geom.Vec2i, 0, len(m.Faces[f.face].Loops))
	for _, loop := range m.Faces[f.face].Loops {
		pts := make([]geom.Vec2i, 0, len(loop))
		for _, vi := range loop {
			uv := frame.ToLocal(m.Verts[vi])
			pts = append(pts, geom.Vec2i{
				X: geom.ToSubunits(uv.X),
				Y: geom.ToSubunits(uv.Y),
			})
		}
		out = append(out, pts)
	}
	return out
}

// ProjectFaceOutline copies the anchored face's boundary into real sketch
// entities (SPEC-UX §10), which is how an offset hull or a step in the armour
// starts: from the shape that is already there.
func (a *App) ProjectFaceOutline() bool {
	s := a.ActiveSketch()
	loops := a.faceOutline2D(s)
	if len(loops) == 0 {
		a.Toast(ui.Toast{
			Text: "The face this sketch was made on is gone — nothing to project",
			Kind: ui.ToastWarn,
		})
		return false
	}

	var added int
	for _, pts := range loops {
		for i := range pts {
			b := pts[(i+1)%len(pts)]
			if pts[i] == b {
				continue
			}
			cmd := &model.AddEntity{Sketch: s.ID, Entity: model.NewLine(pts[i], b)}
			if !a.Run(cmd) {
				return false
			}
			added++
		}
	}
	a.Toast(ui.Toast{Text: "Projected the face outline — " + plural(added, "edge", "edges")})
	return true
}

// faceSketchState answers the two questions the card asks about a sketch: is it
// anchored to a face at all, and is that face still there.
func (a *App) faceSketchState(s *model.Sketch) (onFace, faceGone bool) {
	if s == nil || !s.OnFace {
		return false, false
	}
	_, ok := a.faceReference(s)
	return true, !ok
}

// faceRefAlive reports whether the face a sketch was drawn on is still there,
// which is what decides if its outline can still be snapped to or projected.
func (a *App) faceRefAlive(s *model.Sketch) bool {
	_, ok := a.faceReference(s)
	return ok
}
