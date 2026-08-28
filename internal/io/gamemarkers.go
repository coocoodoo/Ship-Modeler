package io

import (
	"encoding/json"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// The game payload of a .pxm (the user's request, 2026-08-28).
//
// A .pxm carries, beside the document, a `game/` folder made for an engine to
// consume without knowing anything about this program: `ship.glb` is the
// render-ready model (the same bytes the glTF export writes), and
// `markers.json` is this file — the authored orientation dots, plus every
// derived number an engine would otherwise have to compute: an orthonormal
// basis, the centre, the extents. Iron Drift keeps hand-measured tables of
// exactly these facts per hull ("model_nose_negative_z", THR_* thruster
// tables); the point of the payload is that a .pxm ship needs none of that.

// GameMarkersFormat names the schema, so a reader can refuse what it does not
// know rather than misread it.
const GameMarkersFormat = "pxm-markers/1"

// GameMarkers is game/markers.json. All positions and vectors are in the
// model's own units and axes — the same space the vertices in ship.glb use —
// so a point can be compared with the geometry directly and scales with it.
type GameMarkers struct {
	Format string `json:"format"`

	// Center and HalfExtents describe the visible geometry's bounding box.
	Center      [3]float64 `json:"center"`
	HalfExtents [3]float64 `json:"halfExtents"`

	// Forward, Up and Right are an orthonormal, right-handed basis derived
	// from the front and top dots: forward points out the nose, up out the
	// roof. Present only when the front dot exists; Right = Up × Forward.
	Forward *[3]float64 `json:"forward,omitempty"`
	Up      *[3]float64 `json:"up,omitempty"`
	Right   *[3]float64 `json:"right,omitempty"`

	// Nose and Top are the authored dots themselves, when placed.
	Nose *[3]float64 `json:"nose,omitempty"`
	Top  *[3]float64 `json:"top,omitempty"`

	// Thrusters are the engine ports, in the order they were placed.
	Thrusters []GameThruster `json:"thrusters,omitempty"`
}

// GameThruster is one engine port: where the effect plays, which way the
// exhaust points (the outward normal of the face the dot was placed on), and
// the bell radius, all in model units.
type GameThruster struct {
	At  [3]float64 `json:"at"`
	Dir [3]float64 `json:"dir"`
	R   float64    `json:"r"`
}

// BuildGameMarkers derives the payload from the document.
func BuildGameMarkers(doc *model.Document) GameMarkers {
	out := GameMarkers{Format: GameMarkersFormat}

	box := geom.Empty()
	for _, b := range visibleBodies(doc) {
		box = box.Union(b.Mesh.AABB())
	}
	centre := geom.Vec3{}
	if box.Valid() {
		centre = box.Center()
		half := box.Max.Sub(box.Min).Mul(0.5)
		out.Center = vec3Arr(centre)
		out.HalfExtents = vec3Arr(half)
	}

	if front, ok := doc.FrontMarker(); ok {
		out.Nose = vec3Ptr(front.At)
		// Forward is nose-ward from the centre. A dot placed exactly on the
		// centre has no direction to give, so the face normal it was placed
		// with stands in — the front of a ship faces out of its front face.
		forward, ok := front.At.Sub(centre).NormalizeOK()
		if !ok {
			forward, ok = front.Dir.NormalizeOK()
		}
		if ok {
			up := fallbackUp(forward)
			if top, hasTop := doc.TopMarker(); hasTop {
				out.Top = vec3Ptr(top.At)
				// Up is top-ward, squared against forward so the basis stays
				// orthonormal however the two dots were placed.
				raw := top.At.Sub(centre)
				raw = raw.Sub(forward.Mul(raw.Dot(forward)))
				if u, uok := raw.NormalizeOK(); uok {
					up = u
				}
			}
			right := up.Cross(forward)
			out.Forward = vec3PtrOf(forward)
			out.Up = vec3PtrOf(up)
			out.Right = vec3PtrOf(right)
		}
	} else if top, ok := doc.TopMarker(); ok {
		// A top dot alone still names itself, even without a basis.
		out.Top = vec3Ptr(top.At)
	}

	for _, t := range doc.Thrusters() {
		dir := t.Dir
		if d, ok := dir.NormalizeOK(); ok {
			dir = d
		}
		r := t.R
		if r <= 0 {
			r = model.DefaultThrusterRadius
		}
		out.Thrusters = append(out.Thrusters, GameThruster{
			At: vec3Arr(t.At), Dir: vec3Arr(dir), R: r,
		})
	}
	return out
}

// fallbackUp is an up vector square to forward when no top dot says better:
// world +Y where possible, +Z when the ship flies straight up or down.
func fallbackUp(forward geom.Vec3) geom.Vec3 {
	up := geom.Vec3{Y: 1}
	up = up.Sub(forward.Mul(up.Dot(forward)))
	if u, ok := up.NormalizeOK(); ok {
		return u
	}
	up = geom.Vec3{Z: 1}
	up = up.Sub(forward.Mul(up.Dot(forward)))
	u, _ := up.NormalizeOK()
	return u
}

func marshalGameMarkers(doc *model.Document) ([]byte, error) {
	return json.MarshalIndent(BuildGameMarkers(doc), "", "  ")
}

func vec3Arr(v geom.Vec3) [3]float64  { return [3]float64{v.X, v.Y, v.Z} }
func vec3Ptr(v geom.Vec3) *[3]float64 { a := vec3Arr(v); return &a }
func vec3PtrOf(v geom.Vec3) *[3]float64 {
	return vec3Ptr(v)
}
