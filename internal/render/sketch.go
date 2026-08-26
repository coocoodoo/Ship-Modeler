package render

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
)

// Sketch overlay drawing (SPEC-UX §8.6, SPEC-RENDER §4). The renderer stays
// dumb here: the scene layer hands it world-space triangles, lines and markers
// already lifted onto the sketch plane, and this decides only how they look on
// screen.

// SketchLiftUnits is how far region fills and strokes sit above the sketch
// plane, so they never z-fight the plane quad or the geometry under it.
const SketchLiftUnits = 0.05

// MarkerKind selects the glyph drawn at a sketch point (SPEC-UX §8.4).
type MarkerKind uint8

const (
	// MarkerRing is the red circle on a loose endpoint (R3).
	MarkerRing MarkerKind = iota
	// MarkerEndpoint is the snap circle over an existing endpoint.
	MarkerEndpoint
	// MarkerMidpoint is the snap diamond over a segment's middle.
	MarkerMidpoint
	// MarkerGrid is the small cross over a lattice intersection.
	MarkerGrid
	// MarkerVertex is a plain filled dot on a drawn point.
	MarkerVertex
	// MarkerClose is the ring that says clicking here closes the profile.
	MarkerClose
)

// SketchTri is one triangle of a filled region, in world space.
type SketchTri struct {
	A, B, C geom.Vec3
	Color   color.RGBA
}

// SketchLine is one stroke: an entity edge, a preview, or a dashed guide.
type SketchLine struct {
	A, B    geom.Vec3
	Color   color.RGBA
	WidthPx float64
	// Dashed draws the line as an on-off run, which is what an inference guide
	// looks like.
	Dashed bool
}

// SketchMarker is one glyph at a point.
type SketchMarker struct {
	P      geom.Vec3
	Kind   MarkerKind
	Color  color.RGBA
	SizePx float64
}

// SketchDraw is everything the sketch overlay paints for one frame.
type SketchDraw struct {
	Frame geom.Frame
	// Fills come first, then Lines, then Markers, so a glyph is never buried
	// under a stroke.
	Fills   []SketchTri
	Lines   []SketchLine
	Markers []SketchMarker
}

// Empty reports whether there is nothing to draw.
func (s *SketchDraw) Empty() bool {
	return len(s.Fills) == 0 && len(s.Lines) == 0 && len(s.Markers) == 0
}

// Lift places a sketch-space point on the plane, raised clear of it.
func (s *SketchDraw) Lift(p geom.Vec2i) geom.Vec3 {
	return s.Frame.LiftSub(p, SketchLiftUnits)
}

// dashLengthPx and gapLengthPx set the rhythm of a guide line.
const (
	dashLengthPx = 6.0
	gapLengthPx  = 4.0
)

// drawSketchPass paints the sketch overlay over the scene.
func (r *Renderer) drawSketchPass(s *Scene, vp Viewport) {
	sk := s.Sketch
	if sk == nil || sk.Empty() {
		return
	}
	c := r.ribbonCtx(s.Camera, vp)

	// The sketch being edited always draws on top of the model. The rest of the
	// scene is already dimmed to 30% (SPEC-UX §8.1); leaving the profile to
	// fight the depth buffer as well would hide it behind whatever the plane
	// happens to pass through, which makes sketching on an interior plane
	// impossible. Logged in DECISIONS.
	rl.DisableDepthTest()
	rl.DisableBackfaceCulling()
	for _, t := range sk.Fills {
		tri3(t.A, t.B, t.C, t.Color)
	}
	rl.DrawRenderBatchActive()

	for _, l := range sk.Lines {
		if l.Dashed {
			r.drawDashed(c, l)
			continue
		}
		r.drawRibbon(c, l.A, l.B, l.WidthPx, l.Color, EyeShrinkEdgeFactor)
	}
	rl.DrawRenderBatchActive()

	for _, m := range sk.Markers {
		r.drawMarker(c, m)
	}
	rl.DrawRenderBatchActive()
	rl.EnableBackfaceCulling()
	rl.EnableDepthTest()
}

// drawDashed splits a line into on-off runs of constant screen length.
func (r *Renderer) drawDashed(c ribbonContext, l SketchLine) {
	dir := l.B.Sub(l.A)
	length := dir.Len()
	if length <= 0 {
		return
	}
	unit := dir.Mul(1 / length)
	perPx := c.worldPerPixel(l.A)
	dash := dashLengthPx * perPx
	gap := gapLengthPx * perPx
	if dash <= 0 || gap <= 0 {
		return
	}

	for t := 0.0; t < length; t += dash + gap {
		end := math.Min(t+dash, length)
		r.drawRibbon(c, l.A.Add(unit.Mul(t)), l.A.Add(unit.Mul(end)),
			l.WidthPx, l.Color, EyeShrinkEdgeFactor)
	}
}

// drawMarker paints one snap or endpoint glyph, screen-sized so it reads the
// same at every zoom.
func (r *Renderer) drawMarker(c ribbonContext, m SketchMarker) {
	size := m.SizePx
	if size <= 0 {
		size = 6
	}
	half := size / 2

	switch m.Kind {
	case MarkerRing, MarkerEndpoint, MarkerClose:
		width := 1.5
		if m.Kind == MarkerClose {
			width = 2.0
		}
		r.drawRingMarker(c, m.P, half, width, m.Color)
	case MarkerMidpoint:
		r.drawDiamondMarker(c, m.P, half, m.Color)
	case MarkerGrid:
		r.drawCrossMarker(c, m.P, half, m.Color)
	default:
		r.drawBillboardQuad(c, m.P, size, m.Color, EyeShrinkVertFactor)
	}
}

// markerSegments is how many chords approximate a ring glyph.
const markerSegments = 12

func (r *Renderer) drawRingMarker(c ribbonContext, p geom.Vec3, radiusPx, widthPx float64, col color.RGBA) {
	right := c.cam.Right()
	up := c.cam.Up()
	rad := radiusPx * c.worldPerPixel(p)

	prev := p.Add(right.Mul(rad))
	for i := 1; i <= markerSegments; i++ {
		a := 2 * math.Pi * float64(i) / markerSegments
		cur := p.Add(right.Mul(rad * math.Cos(a))).Add(up.Mul(rad * math.Sin(a)))
		r.drawRibbon(c, prev, cur, widthPx, col, EyeShrinkVertFactor)
		prev = cur
	}
}

func (r *Renderer) drawDiamondMarker(c ribbonContext, p geom.Vec3, radiusPx float64, col color.RGBA) {
	right := c.cam.Right()
	up := c.cam.Up()
	rad := radiusPx * c.worldPerPixel(p)
	pts := [4]geom.Vec3{
		p.Add(up.Mul(rad)),
		p.Add(right.Mul(rad)),
		p.Sub(up.Mul(rad)),
		p.Sub(right.Mul(rad)),
	}
	for i := 0; i < 4; i++ {
		r.drawRibbon(c, pts[i], pts[(i+1)%4], 1.5, col, EyeShrinkVertFactor)
	}
}

func (r *Renderer) drawCrossMarker(c ribbonContext, p geom.Vec3, radiusPx float64, col color.RGBA) {
	right := c.cam.Right()
	up := c.cam.Up()
	rad := radiusPx * c.worldPerPixel(p)
	r.drawRibbon(c, p.Sub(right.Mul(rad)), p.Add(right.Mul(rad)), 1.5, col, EyeShrinkVertFactor)
	r.drawRibbon(c, p.Sub(up.Mul(rad)), p.Add(up.Mul(rad)), 1.5, col, EyeShrinkVertFactor)
}
