package sketch

// Tool groups (Sketch_func.md §4.2).
//
// The toolbar shows one button per group rather than one per tool: the
// variants of a group are refinements of a single idea — a rectangle is a
// rectangle however you place it — and eight competing letters would be worse
// than four that step through their own alternatives. The group is also what
// the flyout lists and what a key cycles.

// ToolGroup names one toolbar slot.
type ToolGroup uint8

const (
	GroupSelect ToolGroup = iota
	GroupLine
	GroupRect
	GroupCircle
	GroupArc
	GroupPolygon
	GroupSlot
	GroupSpline
	GroupPoint
)

// Groups lists the groups in toolbar order.
func Groups() []ToolGroup {
	return []ToolGroup{
		GroupSelect, GroupLine, GroupRect, GroupCircle, GroupArc,
		GroupPolygon, GroupSlot, GroupSpline, GroupPoint,
	}
}

// Tools are the group's members, in the order the flyout lists them and the
// order its key steps through them. The first is the group's default.
func (g ToolGroup) Tools() []Tool {
	switch g {
	case GroupLine:
		return []Tool{ToolLine, ToolMidLine}
	case GroupRect:
		return []Tool{ToolRect, ToolCenterRect, ToolAlignedRect}
	case GroupCircle:
		return []Tool{ToolCircle, ToolCircle3, ToolEllipse}
	case GroupArc:
		return []Tool{ToolArc3, ToolArcTangent, ToolArcCenter}
	case GroupPolygon:
		return []Tool{ToolPolygon, ToolPolygonCirc}
	case GroupSlot:
		return []Tool{ToolSlot}
	case GroupSpline:
		return []Tool{ToolSpline, ToolBezier}
	case GroupPoint:
		return []Tool{ToolPoint}
	default:
		return []Tool{ToolSelect}
	}
}

// Name is what the group is called when its own members are not being listed.
func (g ToolGroup) Name() string {
	switch g {
	case GroupLine:
		return "Line"
	case GroupRect:
		return "Rectangle"
	case GroupCircle:
		return "Circle"
	case GroupArc:
		return "Arc"
	case GroupPolygon:
		return "Polygon"
	case GroupSlot:
		return "Slot"
	case GroupSpline:
		return "Spline"
	case GroupPoint:
		return "Point"
	default:
		return "Select"
	}
}

// Key is the group's shortcut, which is the shortcut of every tool in it.
func (g ToolGroup) Key() string { return g.Tools()[0].Shortcut() }

// GroupOf reports which group a tool belongs to.
func GroupOf(t Tool) ToolGroup {
	for _, g := range Groups() {
		for _, member := range g.Tools() {
			if member == t {
				return g
			}
		}
	}
	return GroupSelect
}
