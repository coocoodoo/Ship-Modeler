package app

import (
	"fmt"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
)

// The edge-line tool of paint mode (the user's request, 2026-08-27).
//
// Pick the edges you want a line along, set how thick, press the button. The
// line is baked into the faces' own pictures, so from that moment it is
// ordinary paint: it saves, exports, survives a boolean and can be painted
// over. Nothing about it is a separate kind of thing the rest of the program
// has to learn about.
//
// It is a paint tool rather than a mode of its own because that is where the
// colour and the thickness already live, and because the answer to "what does
// clicking do" has to stay one thing per tool (SPEC-UX §1).

// EdgeWidths are the band thicknesses the panel offers, in texels on each
// face. One is a hairline panel seam; four is a painted stripe.
var EdgeWidths = []int{1, 2, 3, 4, 6}

// edgeRef names one edge of one body, which is what the tool collects.
type edgeRef struct {
	body uint32
	edge int
}

// InEdgePaint reports whether the edge tool is the armed one.
func (a *App) InEdgePaint() bool {
	return a.InPaint() && a.paint.tool == paint.ToolEdge
}

// toggleEdge adds an edge to the selection, or takes it out again.
func (a *App) toggleEdge(body uint32, edge int) {
	ref := edgeRef{body: body, edge: edge}
	for i, had := range a.paint.edges {
		if had == ref {
			a.paint.edges = append(a.paint.edges[:i], a.paint.edges[i+1:]...)
			return
		}
	}
	a.paint.edges = append(a.paint.edges, ref)
}

// edgeSelected reports whether an edge is in the tool's selection.
func (a *App) edgeSelected(body uint32, edge int) bool {
	for _, had := range a.paint.edges {
		if had.body == body && had.edge == edge {
			return true
		}
	}
	return false
}

// ClearEdgeSelection empties it, which is what Escape does first in this tool.
func (a *App) ClearEdgeSelection() bool {
	if len(a.paint.edges) == 0 {
		return false
	}
	a.paint.edges = a.paint.edges[:0]
	return true
}

// pickPaintEdge resolves the edge under the pointer and, on a press, toggles
// it into the selection.
//
// It runs its own pick rather than sharing the face hover: paint mode asks the
// renderer for faces only (V-42), because a brush paints faces and letting an
// edge win the pixel under the cursor would be a stroke that landed on
// nothing. This tool wants exactly the opposite, so it says so.
func (a *App) pickPaintEdge(in InputFrame, vp render.Viewport) {
	a.paint.hoverEdge = -1
	a.paint.hoverEdgeBody = 0
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) ||
		a.Cube.Contains(in.MouseX, in.MouseY) || a.orbiting || a.panning || a.cubeDrag {
		return
	}

	a.paint.pickCooldown -= in.DeltaMillis
	if a.paint.pickCooldown > 0 && !in.Pressed[MouseLeft] {
		return
	}
	a.paint.pickCooldown = PaintPickIntervalMillis

	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	a.Hover = hit
	if !hit.Hit || hit.Kind != render.PickEdge {
		return
	}
	a.paint.hoverEdge, a.paint.hoverEdgeBody = hit.Edge, hit.BodyID

	if in.Pressed[MouseLeft] {
		a.toggleEdge(hit.BodyID, hit.Edge)
	}
}

// PaintSelectedEdges bakes the line and clears the selection.
func (a *App) PaintSelectedEdges() bool {
	if len(a.paint.edges) == 0 {
		a.Toast(ui.Toast{
			Text: "Click the edges you want a line along first",
			Kind: ui.ToastWarn,
		})
		return false
	}

	// One command per body, because a command names the body it edits — but
	// still one press, so a run that spans two bodies is two entries and says
	// so rather than pretending to be one.
	byBody := map[uint32][]int{}
	order := []uint32{}
	for _, ref := range a.paint.edges {
		if _, seen := byBody[ref.body]; !seen {
			order = append(order, ref.body)
		}
		byBody[ref.body] = append(byBody[ref.body], ref.edge)
	}

	painted := 0
	for _, id := range order {
		cmd := &paint.StrokeEdges{
			Body:  id,
			Edges: byBody[id],
			Color: a.paint.color,
			Size:  a.paint.edgeWidth,
			Res:   a.paint.res,
		}
		if !a.Run(cmd) {
			return false
		}
		painted += len(byBody[id])
	}

	a.paint.recents.Add(a.paint.color)
	a.paint.edges = a.paint.edges[:0]
	a.Toast(ui.Toast{
		Text: fmt.Sprintf("Painted %s, %s wide",
			plural(painted, "edge", "edges"),
			plural(a.paint.edgeWidth, "texel", "texels")),
		Action:   "Undo",
		OnAction: func() { a.Undo() },
	})
	return true
}

// SelectBodyCreases takes every sharp edge of the bodies already involved, or
// of the whole visible document when nothing is picked yet.
//
// It is the "all of them" a hull needs: clicking forty edges one at a time to
// outline a ship is not a tool, it is a chore. Flat joins inside a plane are
// left out, because a line along one would be a line drawn across a face for
// no reason.
func (a *App) SelectBodyCreases() bool {
	bodies := map[uint32]bool{}
	for _, ref := range a.paint.edges {
		bodies[ref.body] = true
	}
	if len(bodies) == 0 {
		for _, ref := range a.Sel.Refs() {
			if ref.Body != 0 {
				bodies[ref.Body] = true
			}
		}
	}
	if len(bodies) == 0 {
		for _, b := range a.Doc().Bodies {
			if b.Visible && b.Mesh != nil {
				bodies[b.ID] = true
			}
		}
	}

	a.paint.edges = a.paint.edges[:0]
	found := 0
	for _, b := range a.Doc().Bodies {
		if !bodies[b.ID] || b.Mesh == nil || !b.Visible {
			continue
		}
		for i := range b.Mesh.Topo().Edges {
			if paint.EdgeIsCrease(b.Mesh, i, CreaseDegrees) {
				a.paint.edges = append(a.paint.edges, edgeRef{body: b.ID, edge: i})
				found++
			}
		}
	}
	if found == 0 {
		a.Toast(ui.Toast{Text: "No sharp edges to pick", Kind: ui.ToastWarn})
		return false
	}
	a.Toast(ui.Toast{Text: fmt.Sprintf("Picked %s", plural(found, "edge", "edges"))})
	return true
}

// CreaseDegrees is how far two faces must turn before the edge between them
// counts as a corner worth drawing along. It matches the renderer's own crease
// threshold, so what the tool picks is what the model already draws as an edge.
const CreaseDegrees = 20

// edgeOverlay draws the picked edges, and the one under the pointer.
func (a *App) edgeOverlay() *render.Overlay {
	if !a.InEdgePaint() {
		return nil
	}
	d := &render.Overlay{}
	add := func(body uint32, edge int, col color.RGBA, width float64) {
		b := a.Doc().BodyByID(body)
		if b == nil || b.Mesh == nil {
			return
		}
		wa, wb, ok := paint.EdgeEndsOf(b.Mesh, edge)
		if !ok {
			return
		}
		d.Lines = append(d.Lines, render.OverlayLine{A: wa, B: wb, Color: col, WidthPx: width})
	}
	for _, ref := range a.paint.edges {
		// In the colour it will be painted, so the preview is the answer.
		add(ref.body, ref.edge, a.paint.color, 4)
	}
	if a.paint.hoverEdge >= 0 && !a.edgeSelected(a.paint.hoverEdgeBody, a.paint.hoverEdge) {
		add(a.paint.hoverEdgeBody, a.paint.hoverEdge, ui.Fade(ui.ColorAccent, 0.8), 3)
	}
	if d.Empty() {
		return nil
	}
	return d
}

// buildEdgeSection is the panel's edge controls.
func (a *App) buildEdgeSection(row func(float32) rl.Rectangle, space func(float64), line float32) {
	st := &a.paint

	a.UI.Text(row(line), "Edge line", ui.FontSizeSmall, ui.ColorTextDim)

	// Width chips, in texels on each face.
	labels := make([]string, len(EdgeWidths))
	sel := 0
	for i, w := range EdgeWidths {
		labels[i] = itoa(w)
		if w == st.edgeWidth {
			sel = i
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("paint.edgewidth"), row(a.px(24)),
		labels, sel, ui.ChipGroupOpts{
			Tooltip: "How thick the line is, in texels on each face",
		}); changed {
		st.edgeWidth = EdgeWidths[pick]
	}
	space(6)

	r := row(a.px(26))
	allBox, paintBox := ui.SplitLeft(r, r.Width*0.42)
	paintBox.X += a.px(6)
	paintBox.Width -= a.px(6)

	if a.UI.Button(ui.MakeID("paint.alledges"), allBox, "All corners", ui.ButtonOpts{
		Tooltip: "Pick every sharp edge of the body",
	}) {
		a.SelectBodyCreases()
	}

	count := len(st.edges)
	label := "Paint edges"
	if count > 0 {
		label = fmt.Sprintf("Paint %d", count)
	}
	if a.UI.Button(ui.MakeID("paint.bakeedges"), paintBox, label, ui.ButtonOpts{
		Style:       ui.ButtonPrimary,
		Disabled:    count == 0,
		Tooltip:     "Bake the line into the faces along these edges",
		DisabledWhy: "Click the edges you want a line along",
	}) {
		a.PaintSelectedEdges()
	}
}
