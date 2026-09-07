package app

import (
	"image/color"
	"os"
	"path/filepath"

	"modeler/internal/geom"
	"modeler/internal/io"
	"modeler/internal/paint"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

// The colour system (SPEC-UX §3.1, V-149): which accent the chrome wears is a
// function of what you are doing, and the theme file lets a player retune
// every token.

// ThemeFileName is the theme's file, beside settings.json.
const ThemeFileName = "theme.json"

// modeAccent is the colour the whole chrome takes on right now. The order is
// the order the modes nest: a sketch being drawn on the way into an extrude
// is still a sketch until the extrude tool opens.
func (a *App) modeAccent() color.RGBA {
	switch {
	case a.InSketch() || a.sketch.awaitingPlane:
		return ui.AccentSketch
	case a.InExtrude() || a.InPushPull():
		return ui.AccentExtrude
	case a.InBoolean():
		return ui.AccentBoolean
	case a.InPaint():
		return ui.AccentPaint
	case a.markers.armed:
		return ui.AccentMarker
	default:
		return ui.AccentModel
	}
}

// modeName is the word the hint bar's chip carries — the same partition as
// modeAccent, so the chip and the colour never disagree.
func (a *App) modeName() string {
	switch {
	case a.InChamfer():
		return "CHAMFER"
	case a.InSketch() || a.sketch.awaitingPlane:
		return "SKETCH"
	case a.InExtrude() || a.InPushPull():
		return "EXTRUDE"
	case a.InBoolean():
		return "BOOLEAN"
	case a.InPaint():
		return "PAINT"
	case a.markers.armed:
		return "MARKERS"
	case a.InTransform():
		return "MOVE"
	default:
		return "MODEL"
	}
}

// modeColor is a mode's own accent — what its toolbar button wears whether
// or not it is running.
func modeColor(m Mode) color.RGBA {
	switch m {
	case ModeSketch:
		return ui.AccentSketch
	case ModeExtrude:
		return ui.AccentExtrude
	case ModeBoolean:
		return ui.AccentBoolean
	case ModePaint:
		return ui.AccentPaint
	default:
		return ui.AccentModel
	}
}

// planeAccent is a plane's tree colour: the axis it faces, the same tint the
// viewport and the view cube give it.
func planeAccent(k geom.PlaneKind) color.RGBA {
	return scene.PlaneColor(k)
}

// paintToolAccent groups the paint tools by what they do to the picture: pink
// puts pixels down, blue picks and selects (the eyedropper and the wand are
// selection tools that happen to live in paint), teal builds structure along
// edges and tiles.
func paintToolAccent(t paint.Tool) color.RGBA {
	switch t {
	case paint.ToolPick, paint.ToolWand:
		return ui.AccentModel
	case paint.ToolEdge, paint.ToolTile:
		return ui.AccentExtrude
	default:
		return ui.AccentPaint
	}
}

// loadThemeFile applies theme.json from the settings directory. On a first
// run there is none, so the built-in palette is written out with every key
// present: a file that shows what can be changed beats an empty one. Returns
// a warning for the toast, or "" when there is nothing to say.
func loadThemeFile() string {
	dir, err := io.SettingsDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, ThemeFileName)
	applied, err := ui.LoadTheme(path)
	if err != nil {
		return "theme.json couldn't be read — using the built-in colours"
	}
	if !applied {
		if err := os.MkdirAll(dir, 0o755); err == nil {
			_ = ui.WriteTheme(path, ui.DefaultThemeFile())
		}
	}
	return ""
}
