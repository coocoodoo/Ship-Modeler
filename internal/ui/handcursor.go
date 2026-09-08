package ui

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Both hands share the palm hotspot (16, 18), so closing the
// fingers never makes the grabbed position appear to jump. Curved silhouettes
// are supersampled and cached with the rest of the vector artwork.
const openHandPath = `M11 25
 C8 23 5.2 19.4 3.8 16.5 C2.6 14 5 12.7 6.4 14.2 L10 18
 L8.8 7.5 C8.5 4.8 11.8 4.3 12.3 6.8 L13.5 13.5
 L13.4 4.5 C13.4 1.6 16.9 1.6 16.9 4.5 L17 13
 L18.2 5.4 C18.5 2.9 21.8 3.4 21.5 5.9 L20.7 14
 L22.6 8.7 C23.5 6.5 26.3 7.5 25.5 9.9 L23.8 18.8
 C23.5 21.5 22 23.4 20.8 25 C18.5 26 13.5 26 11 25 Z`

const closedHandPath = `M11 25
 C8.4 23.5 6.2 21.3 5.3 18.7 C4.5 16.4 5.4 14.8 7 15
 L8.7 15.9 L8.7 11.6 C8.7 8.7 12.4 8.7 12.4 11.6
 L12.4 10.3 C12.4 7.4 16.2 7.4 16.2 10.3
 C16.2 7.6 20 7.8 20 10.5 L20 11.8
 C20 9.4 23.7 9.5 23.7 12.2 L23.7 18.5
 C23.7 21.4 22.2 23.4 20.8 25 C18.5 26 13.5 26 11 25 Z`

const openHandDetail = `M10 18 C12 17.5 14 19 14.5 21`
const closedHandDetail = `M8.7 15.9 L12.8 17.7 C14.8 18.6 14 21.2 12.2 20.6
 M12.4 11.6 L12.4 14.2 M16.2 10.3 L16.2 14 M20 11.8 L20 14.6`

// DrawHandCursor paints a white hand with a dark keyline and restrained palm
// creases. The outline stays visible against both bright faces and dark space.
func DrawHandCursor(x, y, scale float64, closed bool) {
	name, path, detail := "cursor-open", openHandPath, openHandDetail
	if closed {
		name, path, detail = "cursor-closed", closedHandPath, closedHandDetail
	}
	px := int(math.Round(32 * scale))
	contour := handCursorContour(path)
	creases := handCursorContour(detail)
	view := [4]float64{0, 0, 32, 32}
	layers := []struct {
		name  string
		shape svgShape
		color rl.Color
	}{
		{"outline", svgShape{contours: contour, strokes: contour, strokeWidth: 2.2, view: view}, rl.Color{R: 22, G: 26, B: 32, A: 255}},
		{"fill", svgShape{contours: contour, view: view}, rl.Color{R: 250, G: 251, B: 253, A: 255}},
		{"creases", svgShape{strokes: creases, strokeWidth: 1, view: view}, rl.Color{R: 83, G: 91, B: 104, A: 255}},
	}
	for _, layer := range layers {
		key := svgTexKey{name: name + "-" + layer.name, px: px}
		tex, ok := svgTextures[key]
		if !ok {
			tex = rasterizeSVG(&layer.shape, px)
			svgTextures[key] = tex
		}
		rl.DrawTexture(tex, int32(math.Round(x-float64(px)*.5)),
			int32(math.Round(y-float64(px)*18/32)), layer.color)
	}
}

var handCursorPaths = map[string][][]svgPt{}

func handCursorContour(path string) [][]svgPt {
	if pts, ok := handCursorPaths[path]; ok {
		return pts
	}
	pts, err := parsePathData(path)
	if err != nil {
		panic(err) // Built-in artwork: an invalid path is an authoring error.
	}
	handCursorPaths[path] = pts
	return pts
}
