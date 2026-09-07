package ui

import (
	"embed"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// These masks are generated from Patina's bundled SVGs by Patina's resvg
// renderer. The runtime reuses Modeler's texture cache and 3D window.
//
//go:embed patina/*.png patina/LICENSE
var patinaIcons embed.FS

var patinaIconNames = map[string]bool{
	"home": true, "check": true, "cross": true, "pencil": true,
	"trash": true, "settings": true, "import": true, "export": true,
	"new": true, "open": true, "copy": true, "lock": true,
	"search": true, "refresh": true,
}

func DrawSearchIcon(cx, cy, size float64, col rl.Color) { drawPatinaIcon("search", cx, cy, size, col) }
func DrawRefreshIcon(cx, cy, size float64, col rl.Color) {
	drawPatinaIcon("refresh", cx, cy, size, col)
}

func drawPatinaIcon(name string, cx, cy, size float64, col rl.Color) bool {
	if !patinaIconNames[name] || size < 2 {
		return false
	}
	px := int(math.Round(size))
	key := svgTexKey{name: "patina:" + name, px: px}
	tex, ok := svgTextures[key]
	if !ok {
		data, err := patinaIcons.ReadFile("patina/" + name + ".png")
		if err != nil {
			return false
		}
		img := rl.LoadImageFromMemory(".png", data, int32(len(data)))
		if img == nil || img.Data == nil {
			return false
		}
		rl.ImageResize(img, int32(px), int32(px))
		tex = rl.LoadTextureFromImage(img)
		rl.UnloadImage(img)
		svgTextures[key] = tex
	}
	if tex.ID == 0 {
		return false
	}
	rl.DrawTexture(tex, int32(math.Round(cx-size/2)), int32(math.Round(cy-size/2)), col)
	return true
}
