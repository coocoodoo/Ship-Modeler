package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/render"
	"modeler/internal/ui"
)

// Layout is the window's chrome geometry for one frame, in device pixels
// (SPEC-UX §2): a 40 px toolbar on top, a 240 px tree panel on the left that
// collapses to nothing, a 26 px hint bar at the bottom, and the viewport
// filling whatever is left.
type Layout struct {
	Screen   rl.Rectangle
	Toolbar  rl.Rectangle
	Tree     rl.Rectangle
	Viewport rl.Rectangle
	HintBar  rl.Rectangle
	// Handle is the little chevron that collapses and restores the tree panel.
	Handle rl.Rectangle
	// PaintBar is the palette sidebar on the right, zero-width when shut. It
	// is real chrome like the tree, not a card over the model: the viewport
	// ends where it begins, so nothing you are painting hides under it.
	PaintBar rl.Rectangle
}

// TreeHandleWidth is the collapse handle's width in logical pixels.
const TreeHandleWidth = 12

// ComputeLayout lays the chrome out for a framebuffer size. paintBar is the
// palette sidebar's live width in logical pixels, 0 when it is shut.
func ComputeLayout(fbW, fbH int, scale float64, treeWidth float64, collapsed bool, paintBar float64) Layout {
	px := func(v float64) float32 { return float32(v * scale) }

	var l Layout
	l.Screen = ui.Rect(0, 0, float32(fbW), float32(fbH))

	rest := l.Screen
	l.Toolbar, rest = ui.SplitTop(rest, px(ui.ToolbarHeight))
	l.HintBar, rest = ui.SplitBottom(rest, px(ui.HintBarHeight))

	// The sidebar comes off the right before the tree takes its share, so a
	// narrow window squeezes the tree — which can be collapsed — rather than
	// the palette, which cannot.
	if paintBar > 0 {
		w := px(paintBar)
		if max := rest.Width - px(160); w > max {
			w = max
		}
		if w > 0 {
			l.PaintBar, rest = ui.SplitRight(rest, w)
		}
	}

	if collapsed {
		l.Tree = ui.Rect(rest.X, rest.Y, 0, rest.Height)
		l.Handle = ui.Rect(rest.X, rest.Y, px(TreeHandleWidth), rest.Height)
		l.Viewport = ui.Rect(rest.X+l.Handle.Width, rest.Y, rest.Width-l.Handle.Width, rest.Height)
		return l
	}

	w := px(treeWidth)
	if w > rest.Width-px(200) {
		w = rest.Width - px(200) // never squeeze the viewport away entirely
	}
	l.Tree, rest = ui.SplitLeft(rest, w)
	l.Handle = ui.Rect(l.Tree.X+l.Tree.Width-px(TreeHandleWidth), l.Tree.Y, px(TreeHandleWidth), l.Tree.Height)
	l.Viewport = rest
	return l
}

// RenderViewport converts the layout's viewport rectangle into the renderer's
// integer form.
func (l Layout) RenderViewport() render.Viewport {
	return render.Viewport{
		X: int(l.Viewport.X),
		Y: int(l.Viewport.Y),
		W: int(l.Viewport.Width),
		H: int(l.Viewport.Height),
	}
}
