package paint

import (
	"image"
	"image/color"
	"strings"

	"modeler/internal/geom/mesh"
)

// The brush of SPEC-UX §13.2: pencil, eraser, fill and eyedropper, all working
// in texel space so the result is exactly the pixels you were shown under the
// cursor.

// BrushSizes are the size chips. The three of SPEC-UX §13.1 plus two larger
// ones, because a soft brush needs room to be soft: at four texels across there
// is nowhere for a falloff to happen (DECISIONS V-55).
var BrushSizes = []int{1, 2, 4, 8, 16}

// Tool is which of the four paint tools is armed.
type Tool uint8

const (
	ToolPencil Tool = iota
	ToolBrush
	ToolEraser
	ToolFill
	ToolPick
	ToolLine
	ToolRect
	ToolCircle
	ToolGradient
	// ToolEdge selects edges of the model and bakes a line along them onto the
	// faces that meet there, rather than painting where the pointer goes.
	ToolEdge
	// ToolTile stamps the armed tile of the imported sheet, snapped to a
	// tile grid so stamps butt seamlessly (Tile_paint.md).
	ToolTile
	// ToolWand selects a region of similar colour by flooding from a click,
	// within a tolerance; the other tools then paint only inside it (V-145).
	ToolWand
	ToolSelect
	ToolPaste
)

// Tools lists them in the order the panel draws them: the ones that paint where
// the pointer goes first, then the ones decided by two points.
var Tools = []Tool{
	ToolPencil, ToolBrush, ToolEraser, ToolFill, ToolPick, ToolWand,
	ToolLine, ToolRect, ToolCircle, ToolGradient, ToolEdge, ToolTile,
	ToolSelect, ToolPaste,
}

func (t Tool) String() string {
	switch t {
	case ToolSelect:
		return "Select"
	case ToolPaste:
		return "Paste"
	case ToolBrush:
		return "Brush"
	case ToolEraser:
		return "Eraser"
	case ToolFill:
		return "Fill"
	case ToolPick:
		return "Pick"
	case ToolLine:
		return "Line"
	case ToolRect:
		return "Rect"
	case ToolCircle:
		return "Circle"
	case ToolGradient:
		return "Gradient"
	case ToolEdge:
		return "Edge"
	case ToolTile:
		return "Tile"
	case ToolWand:
		return "Wand"
	default:
		return "Pencil"
	}
}

// ParseTool reads a tool name as the op scripts spell it, in lower case.
func ParseTool(s string) (Tool, bool) {
	if s == "" {
		return ToolPencil, true
	}
	for _, t := range Tools {
		if strings.EqualFold(t.String(), s) {
			return t, true
		}
	}
	// "square" and "ellipse" are what people call them; accept both.
	switch strings.ToLower(s) {
	case "square":
		return ToolRect, true
	case "ellipse":
		return ToolCircle, true
	case "soft", "softbrush":
		return ToolBrush, true
	}
	return ToolPencil, false
}

// Shortcut is the key that arms a tool, listed in the palette panel.
func (t Tool) Shortcut() string {
	switch t {
	case ToolSelect:
		return "U"
	case ToolPaste:
		return "Ctrl+V"
	case ToolBrush:
		return "B"
	case ToolEraser:
		return "E"
	case ToolFill:
		return "G"
	case ToolPick:
		return "I"
	case ToolLine:
		return "L"
	case ToolRect:
		return "R"
	case ToolCircle:
		return "C"
	case ToolGradient:
		return "N"
	case ToolEdge:
		return "K"
	case ToolTile:
		return "T"
	case ToolWand:
		return "W"
	default:
		return "D"
	}
}

// TwoPoint reports whether a tool is decided by where the drag started and
// where it is now, rather than by the path between them. Those are the tools
// that rubber-band, and the ones a click without a drag cannot use.
func (t Tool) TwoPoint() bool {
	switch t {
	case ToolLine, ToolRect, ToolCircle, ToolGradient:
		return true
	}
	return false
}

// Shape reports whether a tool draws an outline that can also be filled.
func (t Tool) Shape() bool {
	return t == ToolRect || t == ToolCircle
}

// Brush is one dab's worth of settings.
type Brush struct {
	Color color.RGBA
	Size  int
	// Erase writes transparency instead of colour, which restores the body's
	// own colour rather than painting over it in a colour that looks like it.
	Erase bool
	// Soft makes the dab a disc that fades out rather than a square that does
	// not (SPEC-UX §13.4).
	Soft bool
	// Dither spends a partial coverage on whole texels instead of on a blend,
	// which is how softness stays inside the palette.
	Dither Dither
	// Mask, when set, confines every texel the brush writes to the wand's
	// selection. Nil means unmasked, which is the ordinary case.
	Mask *Mask
	// Under is the colour a partial dab blends into where nothing is painted:
	// the body's own colour, because that is what shows through.
	Under color.RGBA
	// Once, when set, records how much translucent paint each texel has already
	// taken during this one stroke, so a texel two overlapping dabs both cover
	// ends up the same shade as a texel only one of them reached (V-158). A
	// freehand drag is dozens of dabs that share their endpoints, and without
	// this a half-transparent line comes out blotchy along its own joins.
	//
	// Nil for an opaque brush, which has nothing to accumulate: laying the same
	// solid colour twice is already the same as laying it once.
	Once map[image.Point]float64
}

// NewStamp is the accumulator a translucent stroke passes in Brush.Once. An
// opaque stroke wants nil: the map costs a lookup per texel and buys nothing.
func NewStamp(c color.RGBA) map[image.Point]float64 {
	if c.A == 0 || c.A >= 255 {
		return nil
	}
	return map[image.Point]float64{}
}

// Stroke paints from one texel to another and returns the texel rectangle it
// touched, which is both the region to re-upload and the region to snapshot for
// undo (SPEC-DATA §3.3).
//
// The interpolation is the point. Pointer samples arrive at frame rate, so at
// any real drawing speed consecutive samples are several texels apart; a brush
// that only dabbed at the samples would draw a dotted line.
func Stroke(p *mesh.FacePaint, b Brush, from, to image.Point) image.Rectangle {
	if p == nil {
		return image.Rectangle{}
	}
	size := b.Size
	if size < 1 {
		size = 1
	}
	dirty := image.Rectangle{}
	for _, t := range walk(from, to) {
		dirty = union(dirty, dab(p, b, t, size))
	}
	return dirty
}

// dab paints one brush mark anchored so the sample sits in the top-left of its
// box, and returns the rectangle it wrote.
//
// A hard brush fills that box; a soft one inscribes a disc in it and fades out
// toward the rim. Both go through put, so erasing, dithering and blending are
// decided in one place rather than in each tool.
func dab(p *mesh.FacePaint, b Brush, at image.Point, size int) image.Rectangle {
	r := image.Rectangle{Min: at, Max: at.Add(image.Point{X: size, Y: size})}
	wrote := image.Rectangle{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			t := image.Point{X: x, Y: y}
			coverage := 1.0
			if b.Soft {
				coverage = softCoverage(t, at, size)
			}
			wrote = union(wrote, put(p, b, t, coverage))
		}
	}
	return wrote
}

// walk is an integer Bresenham line, so every step is 8-connected to the last
// and the run is symmetric whichever end it starts from.
func walk(a, b image.Point) []image.Point {
	dx, sx := abs(b.X-a.X), step(a.X, b.X)
	dy, sy := -abs(b.Y-a.Y), step(a.Y, b.Y)
	err := dx + dy

	out := make([]image.Point, 0, dx-dy+1)
	for {
		out = append(out, a)
		if a == b {
			return out
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			a.X += sx
		}
		if e2 <= dx {
			err += dx
			a.Y += sy
		}
	}
}

// Fill flood-fills the connected run of matching texels containing start,
// staying inside region (SPEC-UX §13.2). It is 4-connected, matches on exact
// colour, and returns how many texels changed.
//
// The region is the face's own texel rectangle rather than the image's: the
// image carries a margin that is not on the face at all, and a fill that
// flooded into it would paint texels that can never be seen.
// mask, when non-nil, is the wand's selection: the flood treats its border
// the way it treats a colour change — as a wall.
func Fill(p *mesh.FacePaint, region image.Rectangle, start image.Point, to color.RGBA, mask *Mask) int {
	if p == nil || region.Empty() || !start.In(region) {
		return 0
	}
	if mask != nil && !mask.Contains(start) {
		return 0
	}
	if to.A == 0 {
		to.A = 255
	}
	from := At(p, start)
	// A translucent fill composites onto each texel it reaches rather than
	// replacing it, so the region it floods keeps whatever was under it
	// showing through. The flood still spreads by the colour it started on:
	// what stops it is a change in the picture, not a change in the paint.
	laid := to
	if to.A < 255 {
		laid = Over(from, to)
	}
	if from == laid {
		return 0
	}
	seen := map[image.Point]bool{start: true}
	queue := []image.Point{start}
	changed := 0
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		if !Set(p, t, laid) {
			continue
		}
		changed++
		for _, n := range [4]image.Point{
			{X: t.X + 1, Y: t.Y}, {X: t.X - 1, Y: t.Y},
			{X: t.X, Y: t.Y + 1}, {X: t.X, Y: t.Y - 1},
		} {
			if !n.In(region) || seen[n] || At(p, n) != from {
				continue
			}
			if mask != nil && !mask.Contains(n) {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	return changed
}

// Sample is the eyedropper: the colour actually on screen at a texel, which is
// the paint where there is paint and the body's colour where there is not
// (SPEC-UX §13.2).
func Sample(p *mesh.FacePaint, t image.Point, body color.RGBA) color.RGBA {
	if c := At(p, t); c.A != 0 {
		// The alpha comes back with the colour: picking up a translucent
		// texel and painting it somewhere else has to give the same texel.
		return c
	}
	body.A = 255
	return body
}

func union(a, b image.Rectangle) image.Rectangle {
	if a.Empty() {
		return b
	}
	if b.Empty() {
		return a
	}
	return a.Union(b)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func step(from, to int) int {
	if from < to {
		return 1
	}
	return -1
}
