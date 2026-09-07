package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/tools"
	"modeler/internal/ui"
)

func (a *App) buildExtrudeExtentCard(box rl.Rectangle) {
	t := a.extrude.tool
	card := a.UI.FloatingCard(ui.MakeID("extrude.extent.card"), box, "End condition", ui.FloatingCardOpts{})
	body := card.Body
	row, body := ui.SplitTop(body, a.px(24))
	if pick, changed := a.UI.ChipGroup(ui.MakeID("extrude.extent"), row, []string{"Distance", "Face", "Vertex", "Edge"}, int(t.Extent), ui.ChipGroupOpts{Tooltip: "Distance, up to face, up to vertex, or up to the point clicked on an edge"}); changed {
		a.setExtrudeExtent(tools.Extent(pick))
	}
	body.Y += a.px(6)
	text := "Drag the arrow or enter a depth"
	if t.Extent != tools.ExtentDistance {
		text = "Click a target " + t.Extent.TargetName() + " in the viewport"
		if t.TargetReady {
			text = "Target set · Click another to replace"
		}
	}
	a.UI.Text(body, text, ui.FontSizeSmall, ui.ColorTextDim)
}
