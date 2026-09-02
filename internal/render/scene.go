package render

import (
	"image/color"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The draw-list types below are what a frame hands to the renderer. Higher
// layers (internal/scene) translate the document into these; the renderer never
// reaches back into the document.

// BodyDraw is one body to render this frame.
type BodyDraw struct {
	GPU       *BodyGPU
	BodyID    uint32
	Color     color.RGBA
	Alpha     float64    // 1 for solid bodies, less for previews
	Tint      color.RGBA // A is the blend strength: hover, selection
	Transform geom.Mat4
	Pickable  bool

	// HoverFace and SelectedFaces drive the overlay pass.
	HoverFace     mesh.FaceUID
	SelectedFaces map[mesh.FaceUID]bool
	Selected      bool // whole-body selection: accent silhouette
	EdgeColor     color.RGBA

	// HideTexture draws the body in its own colour with the paint suppressed,
	// which is what the Textures eye in the paint panel is for: seeing the bare
	// geometry under the pixels (SPEC-UX §13.2).
	HideTexture bool

	// NoDim exempts this body from the scene's DimFactor. A mode that dims the
	// scene is dimming it to make one thing stand out, and that thing is
	// usually a body in this list — dimming it along with everything else
	// defeats the whole point.
	NoDim bool

	// XRay draws a translucent body without the depth test, so it shows
	// through whatever is in front of it. It exists for one thing: the ghost
	// of material a cut is about to remove, which by definition sits inside
	// the body being cut and would otherwise never be seen (V-150).
	XRay bool
}

// dimFor is how far a body fades, honouring its exemption.
func (s *Scene) dimFor(b *BodyDraw) float64 {
	if b.NoDim {
		return 1
	}
	return s.DimFactor
}

// PlaneDraw is one default plane, drawn as a bounded translucent quad with a
// corner label (SPEC-UX §5).
type PlaneDraw struct {
	Kind     geom.PlaneKind
	Frame    geom.Frame
	HalfSize float64
	Color    color.RGBA
	Label    string
	Hovered  bool
	Selected bool
	Pickable bool
	// Fade scales the plane's fill, border and label, 0..1, and every
	// constructor must set it — zero means invisible, not "unset", because a
	// sentinel that happens to equal a legal value is how the first version of
	// this drew fully-faded planes at full strength instead. The app fades
	// planes out as the camera zooms in past them (V-129): a reference plane
	// whose boundary is far outside the view is not a reference any more, it
	// is a translucent wall across everything you are actually looking at.
	Fade float64
}

// GridDraw is the sketch-mode grid: minor and major lines on a frame, fading
// out as the spacing drops below a few pixels (SPEC-UX §8.1).
type GridDraw struct {
	Frame     geom.Frame
	HalfSize  float64
	MinorStep float64
	MajorStep float64
	Alpha     float64
	ShowAxes  bool
	// The frame's U and V axes are tinted with the world axis they follow.
	AxisUColor, AxisVColor color.RGBA
}

// Scene is everything the viewport draws for one frame.
type Scene struct {
	Camera Camera
	Bodies []BodyDraw
	Planes []PlaneDraw
	Grid   *GridDraw
	// Sketches are the sketch overlays, drawn in order so the one being edited
	// can be listed last and land on top. Every visible sketch appears here,
	// not just the active one.
	Sketches []*Overlay
	// Gizmo is the active tool's handles, drawn above everything else.
	Gizmo *Overlay

	// DimFactor fades non-focus geometry while a mode owns the view, e.g.
	// sketch mode dims the rest of the model to 30% (SPEC-UX §8.1).
	DimFactor float64

	// AO is the baked ambient occlusion's strength, 0 to 1. The bake lives in
	// the vertex data; this only scales how dark it reads, so the setting can
	// change without rebuilding anything.
	AO float64

	// Flat turns the two-light model and the AO term off, so painted texels
	// read exactly as authored. Hover and selection tints still apply — a
	// view without feedback would strand the tools.
	Flat bool

	// PickFacesOnly keeps edges and vertices out of the ID pass. A mode that
	// can only act on surfaces must not have its cursor captured by the wire
	// running across one: the pick ribbons are five pixels wide and sit in
	// front of the faces they belong to (SPEC-RENDER §6.1), so on a busy mesh
	// they would swallow a brush stroke aimed at the face behind them.
	PickFacesOnly bool
}

// Viewport is the sub-rectangle of the window the 3D scene occupies, in device
// pixels with the origin at the window's top-left.
type Viewport struct {
	X, Y, W, H int
}

// Aspect returns width over height, guarding against a collapsed layout.
func (v Viewport) Aspect() float64 {
	if v.H <= 0 {
		return 1
	}
	return float64(v.W) / float64(v.H)
}

// Contains reports whether a window pixel lies inside the viewport.
func (v Viewport) Contains(x, y int) bool {
	return x >= v.X && x < v.X+v.W && y >= v.Y && y < v.Y+v.H
}

// Local converts a window pixel into viewport-local coordinates.
func (v Viewport) Local(x, y float64) geom.Vec2 {
	return geom.Vec2{X: x - float64(v.X), Y: y - float64(v.Y)}
}

// SceneBounds returns the bounding box of every visible body, used by F-framing
// and by the through-all extrude depth.
func (s *Scene) SceneBounds() geom.AABB {
	b := geom.Empty()
	for i := range s.Bodies {
		if s.Bodies[i].GPU == nil {
			continue
		}
		b = b.Union(s.Bodies[i].GPU.Bounds.Transform(s.Bodies[i].Transform))
	}
	return b
}
