package app

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// Orientation markers in the app (the user's request, 2026-08-28).
//
// Three dots a game engine reads off a .pxm: the front of the ship, its top,
// and one per thruster. Placing one is the arm-then-pick gesture the program
// already speaks (V-63a): press the row in the tree, then click the model —
// the dot lands on the face under the click, and carries that face's outward
// normal, which for a thruster is the direction the exhaust plays.

// markerState is the placement machinery.
type markerState struct {
	// awaiting is the kind the next viewport click will place.
	awaiting model.MarkerKind
	armed    bool
}

// AwaitingMarker reports whether a marker pick is armed.
func (a *App) AwaitingMarker() bool { return a.markers.armed }

// BeginMarkerPick arms placement: the next click on a face places the dot.
func (a *App) BeginMarkerPick(kind model.MarkerKind) {
	a.markers.armed = true
	a.markers.awaiting = kind
}

// CancelMarkerPick disarms it, reporting whether there was anything to disarm.
func (a *App) CancelMarkerPick() bool {
	if !a.markers.armed {
		return false
	}
	a.markers.armed = false
	return true
}

// placeMarkerAt spends an armed pick on a picked face: the dot lands where the
// cursor ray meets the face's plane, and keeps the face's outward normal.
func (a *App) placeMarkerAt(hit render.PickResult, in InputFrame, vp render.Viewport) {
	f, ok := a.resolveFace(hit.BodyID, hit.FaceUID)
	if !ok {
		a.Toast(ui.Toast{Text: "That face is no longer there", Kind: ui.ToastWarn})
		return
	}
	frame := f.body.Mesh.FaceFrame(f.face)
	at, hitPlane := a.pointOnFace(in.MouseX, in.MouseY, vp, frame)
	if !hitPlane {
		return
	}
	kind := a.markers.awaiting
	a.markers.armed = false
	cmd := &model.PlaceMarker{Marker: model.Marker{
		Kind: kind,
		At:   at,
		Dir:  f.body.Mesh.FaceNormal(f.face),
	}}
	if !a.Run(cmd) {
		return
	}
	a.toastWithUndo(fmt.Sprintf("%s dot placed on %s", kind.Label(), f.body.Name))
}

// markerHint is the hint bar while a pick is armed.
func (a *App) markerHint() string {
	switch a.markers.awaiting {
	case model.MarkerFront:
		return "Click the model where the FRONT of the ship is · Esc cancels"
	case model.MarkerTop:
		return "Click the model where the TOP of the ship is · Esc cancels"
	default:
		return "Click the model where a thruster fires from · Esc cancels"
	}
}

// markerColor is each kind's dot colour, shared by the overlay and the tree.
func markerColor(k model.MarkerKind) rl.Color {
	switch k {
	case model.MarkerFront:
		return ui.ColorSuccess
	case model.MarkerTop:
		return ui.ColorAccent
	default:
		return ui.ColorWarn
	}
}

// buildMarkerOverlay draws the placed dots: a filled square each, in the
// kind's colour, with a short tick along a thruster's exhaust direction.
// Idle only — the working modes have their own overlays to keep legible.
func (a *App) buildMarkerOverlay() *render.Overlay {
	if a.Mode != ModeIdle || len(a.Doc().Markers) == 0 {
		return nil
	}
	d := &render.Overlay{}
	for _, m := range a.Doc().Markers {
		col := markerColor(m.Kind)
		size := 9.0
		if m.Kind == model.MarkerThruster {
			size = 8
		}
		d.Markers = append(d.Markers, render.OverlayMarker{
			P: m.At, Kind: render.MarkerVertex, Color: col, SizePx: size,
		})
		if m.Kind == model.MarkerThruster {
			d.Lines = append(d.Lines, render.OverlayLine{
				A: m.At, B: m.At.Add(m.Dir.Mul(1.2)),
				Color: ui.Fade(col, 0.8), WidthPx: 2,
			})
		}
	}
	return d
}

// buildMarkerRows is the tree's Markers section: the placed dots with delete
// buttons, then one arming row per kind.
func (a *App) buildMarkerRows(row func() rl.Rectangle, open bool) {
	if !open {
		return
	}
	doc := a.Doc()
	thruster := 0
	for i, m := range doc.Markers {
		label := m.Kind.Label()
		if m.Kind == model.MarkerThruster {
			thruster++
			label = fmt.Sprintf("Thruster %d", thruster)
		}
		res := a.UI.TreeRow(ui.MakeID("tree.marker."+itoa(i)), row(), ui.TreeRowSpec{
			Label:     label,
			Icon:      ui.DrawMarkerIcon,
			CanDelete: true,
			Indent:    1,
		})
		if res.ClickedDelete {
			idx := i
			if a.Run(&model.DeleteMarker{Index: idx}) {
				a.toastWithUndo("Deleted the " + m.Kind.String() + " dot")
			}
		}
	}

	arm := func(kind model.MarkerKind, label, tip string) {
		armed := a.markers.armed && a.markers.awaiting == kind
		if armed {
			label = "Click the model…"
		}
		res := a.UI.TreeRow(ui.MakeID("tree.marker.add."+kind.String()), row(), ui.TreeRowSpec{
			Label:    label,
			Icon:     ui.DrawMarkerIcon,
			Selected: armed,
			Indent:   1,
			Dim:      !armed,
		})
		if res.Clicked {
			if armed {
				a.CancelMarkerPick()
			} else {
				a.BeginMarkerPick(kind)
			}
		}
		_ = tip
	}
	_, hasFront := doc.FrontMarker()
	_, hasTop := doc.TopMarker()
	frontLabel, topLabel := "Set front dot…", "Set top dot…"
	if hasFront {
		frontLabel = "Move front dot…"
	}
	if hasTop {
		topLabel = "Move top dot…"
	}
	arm(model.MarkerFront, frontLabel, "The direction the ship flies")
	arm(model.MarkerTop, topLabel, "Pins the ship's roll")
	arm(model.MarkerThruster, "Add thruster dot…", "Where an exhaust effect plays")
}
