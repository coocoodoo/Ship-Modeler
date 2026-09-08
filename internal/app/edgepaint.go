package app

import (
	"fmt"
	"image/color"
	"sort"

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

// The band's width runs from one texel — the thinnest a face can draw — up to
// a stripe. A slider rather than chips because the useful value depends on the
// model's resolution and the look wanted, and a range asks to be swept rather
// than chosen from a list (the user's request, 2026-08-27).
const (
	MinEdgeWidth = 1
	MaxEdgeWidth = 16
)

// edgeRef names one edge of one body, which is what the tool collects.
type edgeRef struct {
	body uint32
	edge int
}

// InEdgePaint reports whether the edge tool is the armed one.
func (a *App) InEdgePaint() bool {
	return a.InPaint() && !a.material.open && a.paint.tool == paint.ToolEdge
}

// toggleEdge adds an edge to the selection, or takes it out again — and it
// works on the whole straight line, not the one topology segment (the user's
// report, 2026-08-28: "it skipped this end"). A boundary that history split
// at a vertex — a union seam, a fold chord landing on it — is one line to the
// eye, and a click on a line means the line. Whether the click adds or
// removes is decided by the segment actually clicked, and the whole chain
// follows it either way.
func (a *App) toggleEdge(body uint32, edge int) {
	b := a.Doc().BodyByID(body)
	if b == nil || b.Mesh == nil {
		return
	}
	chain := paint.EdgeChain(b.Mesh, edge, EdgeChainDegrees)
	if a.edgeSelected(body, edge) {
		for _, e := range chain {
			a.removeEdge(body, e)
		}
		return
	}
	for _, e := range chain {
		if !a.edgeSelected(body, e) {
			a.paint.edges = append(a.paint.edges, edgeRef{body: body, edge: e})
		}
	}
}

// EdgeChainDegrees is how far a continuation may turn at a vertex and still
// count as the same line. Tight enough that a chamfer's oblique meeting is a
// corner, loose enough that float noise on a seam vertex is not.
const EdgeChainDegrees = 25

// removeEdge drops one edge from the tool's selection if present.
func (a *App) removeEdge(body uint32, edge int) {
	ref := edgeRef{body: body, edge: edge}
	for i, had := range a.paint.edges {
		if had == ref {
			a.paint.edges = append(a.paint.edges[:i], a.paint.edges[i+1:]...)
			return
		}
	}
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
	if a.uv.input {
		a.pickUVEdge(in)
		return
	}
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
	widest := 0.0
	for _, id := range order {
		cmd := &paint.StrokeEdges{
			Body:  id,
			Edges: byBody[id],
			Color: a.brushColor(),
			Size:  a.paint.edgeWidth,
			Res:   a.allocResFor(id),
		}
		if !a.Run(cmd) {
			return false
		}
		painted += len(byBody[id])
		if _, hi := cmd.WorldWidth(); hi > widest {
			widest = hi
		}
	}

	a.paint.recents.Add(a.paint.color)
	a.paint.edges = a.paint.edges[:0]
	// The real size is worth saying out loud: the same number of pixels is a
	// different thickness on faces of different resolutions, and "one pixel is
	// still fat" is answered by that number rather than by the tool.
	text := fmt.Sprintf("Painted %s, %s wide",
		plural(painted, "edge", "edges"), plural(a.paint.edgeWidth, "pixel", "pixels"))
	if widest > 0 {
		text += fmt.Sprintf(" (%.3g u)", widest)
	}
	a.Toast(ui.Toast{Text: text, Action: "Undo", OnAction: func() { a.Undo() }})
	return true
}

// edgeTexelSize is how much world one texel covers on the faces under the
// picked edges: the smallest and largest, since faces differ.
//
// It is what the panel shows beside the width slider, so the thickness on
// screen is explained before it is painted rather than after.
func (a *App) edgeTexelSize() (lo, hi float64, ok bool) {
	for _, ref := range a.paint.edges {
		b := a.Doc().BodyByID(ref.body)
		if b == nil || b.Mesh == nil {
			continue
		}
		for _, fi := range paint.FacesOfEdge(b.Mesh, ref.edge) {
			t := 0.0
			if p := b.Mesh.Faces[fi].Paint; p != nil {
				t = p.Texel
			} else if p, err := paint.Allocate(b.Mesh, fi, a.paint.res); err == nil {
				// The face has no picture yet, so report the one it would get.
				t = p.Texel
			}
			if t <= 0 {
				continue
			}
			if !ok || t < lo {
				lo = t
			}
			if !ok || t > hi {
				hi = t
			}
			ok = true
		}
	}
	return lo, hi, ok
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
		picked := map[int]bool{}
		for i := range b.Mesh.Topo().Edges {
			if paint.EdgeIsCrease(b.Mesh, i, CreaseDegrees) {
				picked[i] = true
			}
		}
		// A picked line keeps its whole length: a stretch that continues
		// straight through a vertex joins even where its own crease has gone
		// shallow — a fold piece that leaned does not cut the line short.
		for i := range picked {
			for _, e := range paint.EdgeChain(b.Mesh, i, EdgeChainDegrees) {
				picked[e] = true
			}
		}
		idx := make([]int, 0, len(picked))
		for i := range picked {
			idx = append(idx, i)
		}
		sort.Ints(idx)
		for _, i := range idx {
			a.paint.edges = append(a.paint.edges, edgeRef{body: b.ID, edge: i})
			found++
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

	// The width, in pixels, on a slider. Beside it, what that comes to in real
	// size on the faces involved — because the same pixel count is a different
	// thickness on faces of different resolutions, and that is the answer to
	// "one pixel is still too fat" (V-126).
	head := row(line)
	sizeNote, headLabel := ui.SplitRight(head, a.px(120))
	a.UI.Text(headLabel, "Width", ui.FontSizeSmall, ui.ColorTextDim)
	if lo, hi, ok := a.edgeTexelSize(); ok {
		text := fmt.Sprintf("%d px = %.3g u", st.edgeWidth, lo*float64(st.edgeWidth))
		if hi > lo*1.01 {
			text = fmt.Sprintf("%d px = %.3g–%.3g u",
				st.edgeWidth, lo*float64(st.edgeWidth), hi*float64(st.edgeWidth))
		}
		a.UI.Text(sizeNote, text, ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.9))
	}

	r := row(a.px(24))
	valueBox, sliderBox := ui.SplitRight(r, a.px(44))
	if v, changed := a.UI.Slider(ui.MakeID("paint.edgewidth"), sliderBox,
		float64(st.edgeWidth), MinEdgeWidth, MaxEdgeWidth, ui.ButtonOpts{
			Tooltip: "How many pixels wide the line is, on each face that meets the edge",
		}); changed {
		st.edgeWidth = clampInt(int(v+0.5), MinEdgeWidth, MaxEdgeWidth)
	}
	a.UI.Text(valueBox, fmt.Sprintf("%d px", st.edgeWidth), ui.FontSizeUI, ui.ColorText)
	space(6)

	r = row(a.px(26))
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

// clampInt keeps a value inside a range.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
