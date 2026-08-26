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
	// Sketch is the sketch-mode overlay: region fills, strokes and snap glyphs.
	Sketch *SketchDraw

	// DimFactor fades non-focus geometry while a mode owns the view, e.g.
	// sketch mode dims the rest of the model to 30% (SPEC-UX §8.1).
	DimFactor float64
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
