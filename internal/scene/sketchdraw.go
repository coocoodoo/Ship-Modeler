package scene

import (
	"image/color"

	"modeler/internal/geom"
	"modeler/internal/geom/sketch2d"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/sketch"
	"modeler/internal/ui"
)

// Turning a sketch into a draw list (SPEC-UX §8.6): filled regions, strokes,
// the rubber band, red rings on loose ends, and the snap glyph under the
// cursor.

// Stroke widths and glyph sizes in screen pixels.
const (
	EntityWidthPx    = 1.8
	SelectedWidthPx  = 3.0
	PreviewWidthPx   = 1.8
	GuideWidthPx     = 1.2
	OpenEndSizePx    = 10.0
	SnapGlyphSizePx  = 11.0
	CloseRingSizePx  = 13.0
	VertexDotSizePx  = 4.0
	SketchGridHalf   = 40.0
	RegionFillAlpha  = 0x33
	RegionHoverAlpha = 0x4D
	// IdleSketchAlpha fades a sketch that is not being edited.
	IdleSketchAlpha = 0.55
	// IdleRegionFillAlpha tints a closed profile on a sketch nobody is editing,
	// which is how you tell at a glance which sketches can be extruded.
	IdleRegionFillAlpha = 0x1F
)

// SketchView is everything the app knows about the sketch being drawn, handed
// to the scene layer in one struct so the draw list is built from a single
// consistent snapshot.
type SketchView struct {
	Sketch *model.Sketch
	// Editing is true for the sketch the user is currently drawing on. An
	// idle sketch still draws — leaving it invisible the moment you finish
	// would make the tree's eye toggle a lie — but it draws as quiet strokes
	// only: no region fills, no red rings, no snap glyphs. Those all answer
	// "what am I about to do", which is a question only the active sketch has.
	Editing bool
	Session *sketch.Session
	// Snap is the resolved cursor, when the pointer is over the viewport.
	Snap    sketch.Snap
	HasSnap bool
	// Cursor is the snapped position the preview is drawn to.
	Cursor geom.Vec2i
	// HoverRegion is the region index under the cursor, or -1.
	HoverRegion int
	// SelectedRegions are the regions picked for extrude.
	SelectedRegions map[int]bool
	// Reference is the anchored face's boundary in sketch coordinates, drawn as
	// dim solid lines you can snap to but never select (SPEC-UX §10). Empty for
	// a sketch on a default plane, or one whose face has since been cut away.
	Reference [][]geom.Vec2i
	// Selected is true when this sketch is the current selection. An idle
	// sketch you have picked has to look picked, the same as anything else.
	Selected bool
}

// BuildSketchDraw assembles the overlay.
func BuildSketchDraw(v SketchView) *render.Overlay {
	if v.Sketch == nil {
		return nil
	}
	d := &render.Overlay{Frame: v.Sketch.Frame()}

	if !v.Editing {
		// An idle sketch still shows which of its profiles are closed. That is
		// the one thing a person needs to know about a sketch they are not
		// drawing on — a closed profile is one they can extrude — and leaving
		// it out made a finished sketch look like a handful of loose lines.
		appendRegionFills(d, v.Sketch.Arrangement(), v)
		appendEntityStrokes(d, v)
		return d
	}

	appendReference(d, v)
	arr := v.Sketch.Arrangement()
	appendRegionFills(d, arr, v)
	appendEntityStrokes(d, v)
	appendPreview(d, v)
	appendOpenEnds(d, arr)
	appendSnapGlyph(d, v)
	return d
}

// appendReference draws the face the sketch sits on.
//
// It is drawn first and dimly on purpose: it is there to line things up
// against, not to be part of the drawing. Nothing here is selectable — the
// entities the user actually drew are the only things a click can find.
func appendReference(d *render.Overlay, v SketchView) {
	col := ui.WithAlpha(ui.ColorTextDim, 0x80)
	for _, loop := range v.Reference {
		for i := range loop {
			a := loop[i]
			b := loop[(i+1)%len(loop)]
			if a == b {
				continue
			}
			d.Lines = append(d.Lines, render.OverlayLine{
				A: d.Lift(a), B: d.Lift(b), Color: col, WidthPx: 1,
			})
		}
	}
}

// appendRegionFills paints each closed region translucent, brighter under the
// cursor and brighter still when selected (SPEC-UX §8.6).
func appendRegionFills(d *render.Overlay, arr sketch2d.Arrangement, v SketchView) {
	for i, r := range arr.Regions {
		fill := ui.WithAlpha(ui.ColorAccent, RegionFillAlpha)
		switch {
		case !v.Editing && v.Selected:
			fill = ui.WithAlpha(ui.ColorAccent, 0x4D)
		case !v.Editing:
			// Quieter than the active sketch's, but unmistakably a filled area.
			fill = ui.WithAlpha(ui.ColorAccent, IdleRegionFillAlpha)
		case v.SelectedRegions[i]:
			fill = ui.WithAlpha(ui.ColorAccent, 0x66)
		case i == v.HoverRegion:
			fill = ui.WithAlpha(ui.ColorAccent, RegionHoverAlpha)
		}
		for _, t := range sketch2d.Triangulate(r) {
			d.Fills = append(d.Fills, render.OverlayTri{
				A: d.Lift(t.A), B: d.Lift(t.B), C: d.Lift(t.C), Color: fill,
			})
		}
		// A selected or hovered region also gets its outline drawn, so the
		// boundary reads even where fills overlap.
		if (!v.Editing && v.Selected) || v.SelectedRegions[i] || i == v.HoverRegion {
			appendLoopOutline(d, r.Outer, ui.ColorAccent)
			for _, h := range r.Holes {
				appendLoopOutline(d, h, ui.ColorAccent)
			}
		}
	}
}

func appendLoopOutline(d *render.Overlay, l sketch2d.Loop, col color.RGBA) {
	for i := range l.Pts {
		a := l.Pts[i]
		b := l.Pts[(i+1)%len(l.Pts)]
		d.Lines = append(d.Lines, render.OverlayLine{
			A: d.Lift(a), B: d.Lift(b), Color: col, WidthPx: SelectedWidthPx,
		})
	}
}

// appendEntityStrokes draws what the user actually placed.
func appendEntityStrokes(d *render.Overlay, v SketchView) {
	selected := map[int]bool{}
	if v.Session != nil {
		for _, i := range v.Session.Selected {
			selected[i] = true
		}
	}
	for i := range v.Sketch.Entities {
		e := &v.Sketch.Entities[i]
		col, width := ui.ColorText, float64(EntityWidthPx)
		switch {
		case !v.Editing && v.Selected:
			// A selected sketch reads as selected, in the same accent
			// everything else selected uses.
			col, width = ui.ColorAccent, SelectedWidthPx
		case !v.Editing:
			// An idle sketch recedes so the model and the active sketch read
			// first, but it is unmistakably still there.
			col = ui.Fade(ui.ColorTextDim, IdleSketchAlpha)
		}
		if selected[i] {
			col, width = ui.ColorAccent, SelectedWidthPx
		}
		pts := e.Points()
		n := len(pts)
		if !e.Closed() {
			n--
		}
		for j := 0; j < n; j++ {
			d.Lines = append(d.Lines, render.OverlayLine{
				A: d.Lift(pts[j]), B: d.Lift(pts[(j+1)%len(pts)]),
				Color: col, WidthPx: width,
			})
		}
		// Endpoints of open entities get a dot, so a line's ends are visible
		// even before the region engine judges them.
		if !e.Closed() && v.Editing {
			for _, p := range pts {
				d.Markers = append(d.Markers, render.OverlayMarker{
					P: d.Lift(p), Kind: render.MarkerVertex,
					Color: ui.Fade(col, 0.8), SizePx: VertexDotSizePx,
				})
			}
		}
	}
}

// appendPreview draws the rubber band and, when a chain can close, the ring on
// its start point.
func appendPreview(d *render.Overlay, v SketchView) {
	if v.Session == nil || !v.HasSnap {
		return
	}
	p := v.Session.PreviewAt(v.Cursor)
	if p.Show {
		pts := p.Entity.Points()
		n := len(pts)
		if !p.Entity.Closed() {
			n--
		}
		for j := 0; j < n; j++ {
			d.Lines = append(d.Lines, render.OverlayLine{
				A: d.Lift(pts[j]), B: d.Lift(pts[(j+1)%len(pts)]),
				Color: ui.Fade(ui.ColorAccent, 0.85), WidthPx: PreviewWidthPx,
			})
		}
	}

	// The inference guide runs from the anchor point through the snapped one.
	if v.Snap.HasGuide() {
		d.Lines = append(d.Lines, render.OverlayLine{
			A: d.Lift(v.Snap.From), B: d.Lift(v.Snap.Point),
			Color: ui.Fade(ui.ColorAccent, 0.55), WidthPx: GuideWidthPx, Dashed: true,
		})
	}

	if start, ok := v.Session.ChainStart(); ok {
		col := ui.Fade(ui.ColorSuccess, 0.8)
		if p.ClosesChain {
			col = ui.ColorSuccess
		}
		d.Markers = append(d.Markers, render.OverlayMarker{
			P: d.Lift(start), Kind: render.MarkerClose, Color: col, SizePx: CloseRingSizePx,
		})
	}
}

// appendOpenEnds rings every loose endpoint in error red, which is how R3 shows
// a profile is not closed yet.
func appendOpenEnds(d *render.Overlay, arr sketch2d.Arrangement) {
	for _, p := range arr.OpenEnds {
		d.Markers = append(d.Markers, render.OverlayMarker{
			P: d.Lift(p), Kind: render.MarkerRing,
			Color: ui.ColorError, SizePx: OpenEndSizePx,
		})
	}
}

// appendSnapGlyph marks what the cursor latched onto (SPEC-UX §8.4).
func appendSnapGlyph(d *render.Overlay, v SketchView) {
	if !v.HasSnap || v.Session == nil || v.Session.Tool == sketch.ToolSelect {
		return
	}
	var kind render.MarkerKind
	col := ui.ColorAccent
	switch v.Snap.Kind {
	case sketch.SnapEndpoint:
		kind = render.MarkerEndpoint
	case sketch.SnapMidpoint:
		kind = render.MarkerMidpoint
	case sketch.SnapGrid:
		kind, col = render.MarkerGrid, ui.Fade(ui.ColorTextDim, 0.9)
	default:
		return // Alt held: nothing latched, so nothing to advertise
	}
	d.Markers = append(d.Markers, render.OverlayMarker{
		P: d.Lift(v.Snap.Point), Kind: kind, Color: col, SizePx: SnapGlyphSizePx,
	})
}

// SketchGrid builds the grid for a sketch plane at full strength.
func SketchGridFor(s *model.Sketch) *render.GridDraw {
	return SketchGrid(s.Frame(), SketchGridHalf, 1)
}

// RegionAt returns the index of the region containing a sketch point, or -1.
// The test is the exact ray-parity one, so a click on a hole falls through to
// whatever is really underneath.
func RegionAt(arr sketch2d.Arrangement, p geom.Vec2i) int {
	best, bestArea := -1, int64(0)
	for i, r := range arr.Regions {
		inside, onEdge := geom.PointInPolygon(p, r.Outer.Pts)
		if !inside && !onEdge {
			continue
		}
		inHole := false
		for _, h := range r.Holes {
			if in, _ := geom.PointInPolygon(p, h.Pts); in {
				inHole = true
				break
			}
		}
		if inHole {
			continue
		}
		// Prefer the smallest region containing the point, so an island inside
		// a hole wins over anything enclosing it.
		if best < 0 || r.Area2() < bestArea {
			best, bestArea = i, r.Area2()
		}
	}
	return best
}
