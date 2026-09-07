package app

import (
	"fmt"
	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/ui"
)

type bodyClipboard struct {
	bodies []*model.Body
	pastes int
}

func (a *App) CopyBodies() bool {
	if a.Mode != ModeIdle {
		return false
	}
	var bodies []*model.Body
	seen := map[uint32]bool{}
	for _, ref := range a.Sel.Refs() {
		if seen[ref.Body] {
			continue
		}
		seen[ref.Body] = true
		if b := a.Doc().BodyByID(ref.Body); b != nil && b.Mesh != nil {
			bodies = append(bodies, model.SnapshotBody(b))
		}
	}
	if len(bodies) == 0 {
		a.Toast(ui.Toast{Text: "Select a body to copy", Kind: ui.ToastWarn})
		return false
	}
	a.bodyClipboard = bodyClipboard{bodies: bodies}
	a.Toast(ui.Toast{Text: fmt.Sprintf("Copied %d bodies · Ctrl+V to paste", len(bodies))})
	return true
}
func (a *App) PasteBodies() bool {
	if a.Mode != ModeIdle {
		return false
	}
	if len(a.bodyClipboard.bodies) == 0 {
		a.Toast(ui.Toast{Text: "Copy a body first with Ctrl+C", Kind: ui.ToastWarn})
		return false
	}
	cmd := &model.PasteBodies{Sources: a.bodyClipboard.bodies, Offset: geom.Vec3{X: float64(a.bodyClipboard.pastes + 1)}}
	if !a.Run(cmd) {
		return false
	}
	a.bodyClipboard.pastes++
	a.Sel.Clear()
	for _, b := range cmd.Copies() {
		a.Sel.Add(model.BodyRef(b.ID))
	}
	a.armTransform()
	a.Toast(ui.Toast{Text: "Pasted bodies · Drag the gizmo to position them"})
	return true
}
