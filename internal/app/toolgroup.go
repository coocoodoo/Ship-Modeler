package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/sketch"
	"modeler/internal/ui"
)

// The sketch toolbar's tool groups (Sketch_func.md §4.1).
//
// A group is one button showing the variant you last used, plus a chevron that
// opens the list. Pressing the group's key cycles the same list, so the flyout
// is discovery and the key is speed — neither is the only way in.

// chevronWidth is the strip on the right of a group button that opens its
// list, in logical pixels.
const chevronWidth = 14

// currentTool is the variant a group button is armed with: whatever was last
// chosen from it, or the group's default.
func (a *App) currentTool(g sketch.ToolGroup) sketch.Tool {
	if t, ok := a.sketch.groupPick[g]; ok {
		return t
	}
	return g.Tools()[0]
}

// sketchGroupWidth measures a group's button.
func (a *App) sketchGroupWidth(g sketch.ToolGroup, labelled bool) float32 {
	w := a.px(ui.IconSize + ui.Spacing*2)
	if labelled {
		w = a.px(ui.IconSize+ui.Spacing) +
			a.UI.TextWidth(a.currentTool(g).String(), ui.FontSizeUI) + a.px(ui.Spacing)
	}
	if len(g.Tools()) > 1 {
		w += a.px(chevronWidth)
	}
	return w
}

// sketchToolbarFitsLabels reports whether the labelled groups fit the space
// available, leaving room for the construction toggle and the right-hand
// buttons. Below that width the toolbar goes icon-only rather than clipping,
// and the tooltips and the `?` sheet carry the names.
func (a *App) sketchToolbarFitsLabels(avail float32, groups []sketch.ToolGroup) bool {
	// Extrude, Finish and Close on the right, plus the construction toggle.
	reserved := a.px(96*2 + 6 + ui.IconSize + ui.Spacing*6)
	reserved += a.px(ui.IconSize+ui.Spacing) + a.UI.TextWidth("Extrude", ui.FontSizeUI)
	var need float32
	for _, g := range groups {
		need += a.sketchGroupWidth(g, true) + a.px(2)
	}
	return need+reserved <= avail
}

// buildToolGroup draws one group button and, when its list is open, the list.
func (a *App) buildToolGroup(box rl.Rectangle, g sketch.ToolGroup, labelled bool) {
	sess := a.sketch.session
	if sess == nil {
		return
	}
	tool := a.currentTool(g)
	members := g.Tools()
	active := sketch.GroupOf(sess.Tool) == g

	// An active group shows the tool that is actually running, not the one the
	// button remembers: they differ for one frame after a key cycles the group.
	if active {
		tool = sess.Tool
	}

	main := box
	var chev rl.Rectangle
	if len(members) > 1 {
		chev, main = ui.SplitRight(box, a.px(chevronWidth))
	}

	label := ""
	if labelled {
		label = tool.String()
	}
	if a.UI.IconButton(ui.MakeID("sketchgroup."+g.Name()), main, sketchToolIcon(tool),
		ui.IconOpts{
			Label:    label,
			Active:   active,
			Tooltip:  tool.String(),
			Shortcut: g.Key(),
		}) {
		sess.SetTool(tool)
		a.sketch.groupPick[g] = tool
		a.sketch.flyoutUp = false
	}

	if len(members) <= 1 {
		return
	}
	open := a.sketch.flyoutUp && a.sketch.flyoutOpen == g
	if a.UI.ChevronButton(ui.MakeID("sketchgroup.more."+g.Name()), chev, open,
		"More "+g.Name()+" tools") {
		if open {
			a.sketch.flyoutUp = false
		} else {
			a.sketch.flyoutOpen, a.sketch.flyoutUp = g, true
		}
	}
	if open {
		a.buildToolFlyout(box, g)
	}
}

// buildToolFlyout lists a group's variants under its button.
func (a *App) buildToolFlyout(anchor rl.Rectangle, g sketch.ToolGroup) {
	sess := a.sketch.session
	members := g.Tools()
	rowH := a.px(26)
	width := a.px(180)

	items := make([]ui.MenuItem, len(members))
	for i, t := range members {
		items[i] = ui.MenuItem{
			Label:    t.String(),
			Icon:     sketchToolIcon(t),
			Shortcut: t.Shortcut(),
			Selected: sess.Tool == t,
		}
	}
	res := a.UI.Menu(ui.MakeID("sketchflyout."+g.Name()),
		ui.Rect(anchor.X, anchor.Y+anchor.Height+a.px(2), width, rowH*float32(len(items))+a.px(8)),
		items)
	if res.Chosen >= 0 && res.Chosen < len(members) {
		sess.SetTool(members[res.Chosen])
		a.sketch.groupPick[g] = members[res.Chosen]
		a.sketch.flyoutUp = false
	}
	if res.Dismissed {
		a.sketch.flyoutUp = false
	}
}

// sketchToolIcon is the glyph for a tool.
func sketchToolIcon(t sketch.Tool) ui.IconFunc {
	switch t {
	case sketch.ToolLine:
		return ui.DrawLineToolIcon
	case sketch.ToolMidLine:
		return ui.DrawMidLineIcon
	case sketch.ToolRect:
		return ui.DrawRectToolIcon
	case sketch.ToolCenterRect:
		return ui.DrawCenterRectIcon
	case sketch.ToolAlignedRect:
		return ui.DrawAlignedRectIcon
	case sketch.ToolCircle:
		return ui.DrawCircleToolIcon
	case sketch.ToolCircle3:
		return ui.DrawCircle3Icon
	case sketch.ToolEllipse:
		return ui.DrawEllipseIcon
	case sketch.ToolArc3:
		return ui.DrawArc3Icon
	case sketch.ToolArcTangent:
		return ui.DrawArcTangentIcon
	case sketch.ToolArcCenter:
		return ui.DrawArcCenterIcon
	case sketch.ToolPolygon:
		return ui.DrawPolygonIcon
	case sketch.ToolPolygonCirc:
		return ui.DrawPolygonCircIcon
	case sketch.ToolSlot:
		return ui.DrawSlotIcon
	case sketch.ToolPoint:
		return ui.DrawPointToolIcon
	default:
		return ui.DrawCursorIcon
	}
}
