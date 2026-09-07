package app

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/ui"
)

type chamferState struct {
	distance float64
	edges    map[uint32][]int
	command  *model.ChamferEdges
	preview  map[uint32]*render.BodyGPU
	err      string
	dirty    bool
	wait     float64
	gizmo    chamferGizmoState
}

func (a *App) InChamfer() bool { return a.Mode == ModeChamfer }

func (a *App) BeginEdgeChamfer() bool {
	if a.Mode != ModeIdle || a.Sel.Kind() != model.SelEdge || a.transform.live {
		return false
	}
	distance := a.chamfer.distance
	if distance <= 0 {
		distance = .25
	}
	a.chamfer = chamferState{distance: distance, edges: map[uint32][]int{}}
	for _, ref := range a.Sel.Refs() {
		a.chamfer.edges[ref.Body] = append(a.chamfer.edges[ref.Body], ref.Edge)
	}
	a.dropPushPull()
	a.transform.tool = nil
	a.Mode = ModeChamfer
	a.rebuildChamferPreview()
	return true
}

func (a *App) dropChamferPreview() {
	for _, g := range a.chamfer.preview {
		g.Unload()
	}
	a.chamfer.preview = nil
	a.chamfer.command = nil
}

func (a *App) CancelEdgeChamfer() {
	a.chamfer.gizmo = chamferGizmoState{}
	a.dropChamferPreview()
	a.chamfer.dirty = false
	if a.InChamfer() {
		a.Mode = ModeIdle
	}
}

func (a *App) setChamferDistance(distance float64) {
	if !a.InChamfer() {
		return
	}
	a.chamfer.distance = distance
	if !a.chamfer.gizmo.dragging || !a.chamfer.dirty {
		a.chamfer.wait = 150
	}
	a.chamfer.dirty = true
	// Keep the last valid shape while dragging, but disable Apply until the
	// new distance has been validated. The kernel never runs every frame.
}

func (a *App) rebuildChamferPreview() {
	st := &a.chamfer
	a.dropChamferPreview()
	st.dirty, st.err = false, ""
	cmd := &model.ChamferEdges{Edges: st.edges, Distance: st.distance}
	if err := cmd.Prepare(a.Doc()); err != nil {
		st.err = err.Error()
		return
	}
	st.command = cmd
	st.preview = map[uint32]*render.BodyGPU{}
	if a.Renderer != nil {
		for id := range st.edges {
			g := render.BuildBodyGPU(cmd.Preview(id))
			g.Upload()
			st.preview[id] = g
		}
	}
}

func (a *App) CommitEdgeChamfer() bool {
	if !a.InChamfer() {
		return false
	}
	if a.chamfer.dirty {
		a.rebuildChamferPreview()
	}
	cmd := a.chamfer.command
	if cmd == nil {
		return false
	}
	if !a.Run(cmd) {
		return false
	}
	a.CancelEdgeChamfer()
	a.Sel.Clear()
	for id := range cmd.Edges {
		a.Sel.Add(model.BodyRef(id))
	}
	a.Toast(ui.Toast{Text: "Chamfer applied — Ctrl+Z to undo"})
	return true
}

func (a *App) updateChamfer(in InputFrame, vp render.Viewport) {
	if a.updateChamferGizmo(in, vp) {
		return
	}
	if !in.Pressed[MouseLeft] || a.cubeOwnsPointer(in) {
		return
	}
	// Selection stays anchored to original edges while the final shape is
	// previewed; the command cannot accidentally pick a generated bevel edge.
	s := a.BuildScene()
	for i := range s.Bodies {
		b := a.Doc().BodyByID(s.Bodies[i].BodyID)
		if b != nil {
			s.Bodies[i].GPU = a.bodyGPU(b)
			s.Bodies[i].Pickable = true
		}
	}
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	if !hit.Hit || hit.Kind != render.PickEdge {
		return
	}
	st := &a.chamfer
	list := st.edges[hit.BodyID]
	found := false
	for i, ei := range list {
		if ei == hit.Edge {
			list = append(list[:i], list[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		list = append(list, hit.Edge)
	}
	if len(list) == 0 {
		delete(st.edges, hit.BodyID)
	} else {
		st.edges[hit.BodyID] = list
	}
	a.Sel.Clear()
	for id, edges := range st.edges {
		for _, ei := range edges {
			a.Sel.Add(model.EdgeRef(id, ei))
		}
	}
	a.rebuildChamferPreview()
}

func (a *App) handleChamferKeys(in InputFrame) {
	if in.Ctrl {
		return
	}
	if in.KeyPressed(rl.KeyEscape) {
		a.CancelEdgeChamfer()
		return
	}
	if in.KeyPressed(rl.KeyEnter) || in.KeyPressed(rl.KeyKpEnter) {
		a.CommitEdgeChamfer()
	}
}

func (a *App) buildChamferCard(viewport rl.Rectangle) {
	st := &a.chamfer
	w, h := a.px(280), a.px(284)
	box := ui.Rect(viewport.X+viewport.Width-w-a.px(ui.Spacing*2), viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34), w, h)
	card := a.UI.FloatingCard(ui.MakeID("chamfer.card"), box, "Chamfer", ui.FloatingCardOpts{
		Footer: true, ConfirmLabel: "Apply", CancelLabel: "Cancel", ConfirmDisabled: st.command == nil || st.dirty, ConfirmWhy: st.err,
	})
	body := card.Body
	row := func(h float32) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, h); return r }
	count := 0
	for _, edges := range st.edges {
		count += len(edges)
	}
	label := fmt.Sprintf("%d selected edges", count)
	if count == 1 {
		label = "1 selected edge"
	}
	a.UI.Text(row(a.px(24)), label, ui.FontSizeUI, ui.ColorText)
	a.UI.Text(row(a.px(24)), "Equal distance", ui.FontSizeSmall, ui.ColorTextDim)
	if v, res := a.UI.DragNumber(ui.MakeID("chamfer.distance"), row(a.px(28)), st.distance, ui.NumberOpts{
		Unit: "u", Step: .1, FineStep: .01, Decimals: 3, Min: .004, Max: 10000, Tooltip: "Distance along each adjoining face",
	}); res.Changed {
		a.setChamferDistance(v)
	}
	row(a.px(8))
	a.UI.TextWrapped(row(a.px(40)), "Drag the arrow in 0.25 u steps. Click edges to add/remove, or type a distance above.", ui.FontSizeSmall, ui.ColorTextDim)
	message, col := "Preview ready · Enter to apply", ui.ColorTextDim
	if st.dirty {
		message = "Updating preview…"
	} else if st.err != "" {
		message, col = st.err, ui.ColorError
	}
	a.UI.TextWrapped(row(a.px(64)), message, ui.FontSizeSmall, col)
	if card.Cancelled {
		a.CancelEdgeChamfer()
	} else if card.Confirmed {
		a.CommitEdgeChamfer()
	}
}
