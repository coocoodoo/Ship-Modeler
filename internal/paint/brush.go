package paint

import (
	"image"
	"image/color"

	"modeler/internal/geom/mesh"
)

// The brush of SPEC-UX §13.2: pencil, eraser, fill and eyedropper, all working
// in texel space so the result is exactly the pixels you were shown under the
// cursor.

// BrushSizes are the size chips: one texel, a 2x2 block, a 4x4 block.
var BrushSizes = []int{1, 2, 4}

// Tool is which of the four paint tools is armed.
type Tool uint8

const (
	ToolPencil Tool = iota
	ToolEraser
	ToolFill
	ToolPick
)

func (t Tool) String() string {
	switch t {
	case ToolEraser:
		return "Eraser"
	case ToolFill:
		return "Fill"
	case ToolPick:
		return "Pick"
	default:
		return "Pencil"
	}
}

// Shortcut is the key that arms a tool, listed in the palette panel.
func (t Tool) Shortcut() string {
	switch t {
	case ToolEraser:
		return "E"
	case ToolFill:
		return "G"
	case ToolPick:
		return "I"
	default:
		return "D"
	}
}

// Brush is one dab's worth of settings.
type Brush struct {
	Color color.RGBA
	Size  int
	// Erase writes transparency instead of colour, which restores the body's
	// own colour rather than painting over it in a colour that looks like it.
	Erase bool
}

// value is what a dab writes.
func (b Brush) value() color.RGBA {
	if b.Erase {
		return color.RGBA{}
	}
	c := b.Color
	c.A = 255
	return c
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

// dab paints one brush square anchored so the sample sits in its top-left,
// and returns the rectangle it wrote.
func dab(p *mesh.FacePaint, b Brush, at image.Point, size int) image.Rectangle {
	r := image.Rectangle{Min: at, Max: at.Add(image.Point{X: size, Y: size})}
	c := b.value()
	wrote := image.Rectangle{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			t := image.Point{X: x, Y: y}
			// An erase outside the image has nothing to rub out, and growing
			// the image to store transparency would be pure waste.
			if b.Erase && !t.In(Bounds(p)) {
				continue
			}
			if Set(p, t, c) {
				wrote = union(wrote, image.Rectangle{Min: t, Max: t.Add(image.Point{X: 1, Y: 1})})
			}
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
func Fill(p *mesh.FacePaint, region image.Rectangle, start image.Point, to color.RGBA) int {
	if p == nil || region.Empty() || !start.In(region) {
		return 0
	}
	to.A = 255
	from := At(p, start)
	if from == to {
		return 0
	}
	seen := map[image.Point]bool{start: true}
	queue := []image.Point{start}
	changed := 0
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		if !Set(p, t, to) {
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
		c.A = 255
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
