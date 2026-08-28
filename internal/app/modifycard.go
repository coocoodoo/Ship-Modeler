package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/ui"
)

// The modify section of the sketch card (Sketch_func.md §5 SK5).
//
// A card rather than a set of pointer modes: fillet, offset, mirror and the
// patterns all act on a selection and need a number, and inventing four new
// click gestures to collect numbers the card can hold would be four more
// things to learn for no gain. Select, set, press. It is also what makes every
// one of them scriptable through the same call the button makes.
//
// Buttons that do not apply are disabled with a reason rather than hidden, so
// the section's shape does not change as the selection does — a card whose
// rows move under the pointer is a card that gets mis-clicked (SPEC-UX §15).

// modifyState is the section's own numbers, which persist across selections so
// filleting a dozen corners at the same radius is a dozen clicks and not a
// dozen retypings.
type modifyState struct {
	radius   float64
	distance float64
	axis     MirrorAxis
	pattern  PatternKind
	count    int
	deltaX   float64
	deltaY   float64
	sweep    float64
}

func (m *modifyState) init() {
	m.radius = 1
	m.distance = 0.5
	m.count = 4
	m.deltaX = 4
	m.sweep = 360
}

// modifyCardHeight is how much room the section needs, so the card can be
// sized before it is drawn.
func (a *App) modifyCardHeight(line float32) float32 {
	rows := line + a.px(26) + a.px(6) + // header + fillet/chamfer
		a.px(26) + a.px(6) + // offset
		a.px(26) + a.px(6) + // mirror
		a.px(26) + a.px(6) + // pattern kind + apply
		a.px(24) + a.px(6) // pattern parameters
	return rows
}

// buildModifySection draws the section and runs its buttons.
func (a *App) buildModifySection(row func(float32) rl.Rectangle, space func(float64), line float32) {
	m := &a.sketch.modify

	space(6)
	a.UI.Text(row(line), "Modify", ui.FontSizeSmall, ui.ColorTextDim)

	// --- Fillet and chamfer, sharing one radius.
	canCorner, cornerWhy := a.canFillet()
	r := row(a.px(26))
	sizeBox, buttons := ui.SplitRight(r, a.px(64))
	half := (buttons.Width - a.px(4)) / 2
	filletBox := ui.Rect(buttons.X, buttons.Y, half, buttons.Height)
	chamferBox := ui.Rect(buttons.X+half+a.px(4), buttons.Y, half, buttons.Height)

	if a.UI.Button(ui.MakeID("modify.fillet"), filletBox, "Fillet", ui.ButtonOpts{
		Disabled:    !canCorner,
		Tooltip:     "Round the corner where two selected lines meet",
		DisabledWhy: cornerWhy,
	}) {
		a.FilletSelection(m.radius)
	}
	if a.UI.Button(ui.MakeID("modify.chamfer"), chamferBox, "Chamfer", ui.ButtonOpts{
		Disabled:    !canCorner,
		Tooltip:     "Cut the corner off square",
		DisabledWhy: cornerWhy,
	}) {
		a.ChamferSelection(m.radius)
	}
	if v, res := a.UI.DragNumber(ui.MakeID("modify.radius"), sizeBox, m.radius, ui.NumberOpts{
		Unit: "u", Step: 0.25, FineStep: 0.05, Decimals: 2, Min: 0.05, Max: 64,
		Tooltip: "How big the fillet or chamfer is",
	}); res.Changed {
		m.radius = v
	}
	space(6)

	// --- Offset.
	canOff, offWhy := a.canOffset()
	r = row(a.px(26))
	distBox, offBox := ui.SplitRight(r, a.px(64))
	if a.UI.Button(ui.MakeID("modify.offset"), offBox, "Offset", ui.ButtonOpts{
		Disabled:    !canOff,
		Tooltip:     "Replace the selected shape with one a set distance away",
		DisabledWhy: offWhy,
	}) {
		a.OffsetSelection(m.distance)
	}
	if v, res := a.UI.DragNumber(ui.MakeID("modify.distance"), distBox, m.distance, ui.NumberOpts{
		Unit: "u", Step: 0.25, FineStep: 0.05, Decimals: 2, Min: -64, Max: 64,
		Tooltip: "Outward is positive, inward negative",
	}); res.Changed {
		m.distance = v
	}
	space(6)

	// --- Mirror.
	canAny, anyWhy := a.hasSketchSelection()
	r = row(a.px(26))
	axisBox, mirrorBox := ui.SplitRight(r, a.px(64))
	if a.UI.Button(ui.MakeID("modify.mirror"), mirrorBox, "Mirror", ui.ButtonOpts{
		Disabled: !canAny,
		Tooltip: "Add a reflected copy — this copies entities; " +
			"live model symmetry is a separate mode",
		DisabledWhy: anyWhy,
	}) {
		a.MirrorSelection(m.axis)
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("modify.axis"), axisBox,
		[]string{"Vertical", "Horizontal"}, int(m.axis),
		ui.ChipGroupOpts{Tooltip: "Which axis of the sketch to reflect about"}); changed {
		m.axis = MirrorAxis(pick)
	}
	space(6)

	// --- Pattern.
	r = row(a.px(26))
	kindBox, patBox := ui.SplitRight(r, a.px(64))
	if a.UI.Button(ui.MakeID("modify.pattern"), patBox, "Pattern", ui.ButtonOpts{
		Disabled:    !canAny,
		Tooltip:     "Repeat the selection",
		DisabledWhy: anyWhy,
	}) {
		a.PatternSelection(m.pattern, m.count,
			geom.Vec2i{X: geom.ToSubunits(m.deltaX), Y: geom.ToSubunits(m.deltaY)},
			m.sweep)
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("modify.patkind"), kindBox,
		[]string{"Linear", "Circular"}, int(m.pattern),
		ui.ChipGroupOpts{Tooltip: "Along a line, or round the sketch origin"}); changed {
		m.pattern = PatternKind(pick)
	}
	space(6)

	// --- The pattern's own numbers: a count always, then either the step or
	// the sweep. Two layouts rather than five fields, most of them irrelevant.
	r = row(a.px(24))
	third := (r.Width - a.px(8)) / 3
	countBox := ui.Rect(r.X, r.Y, third, r.Height)
	if v, res := a.UI.DragNumber(ui.MakeID("modify.count"), countBox, float64(m.count),
		ui.NumberOpts{
			Step: 1, FineStep: 1, Decimals: 0, Min: 2, Max: 64,
			Tooltip: "How many in total, counting the original",
		}); res.Changed {
		m.count = int(v)
	}
	if m.pattern == PatternCircular {
		sweepBox := ui.Rect(r.X+third+a.px(4), r.Y, r.Width-third-a.px(4), r.Height)
		if v, res := a.UI.DragNumber(ui.MakeID("modify.sweep"), sweepBox, m.sweep,
			ui.NumberOpts{
				Unit: "°", Step: 15, FineStep: 5, Decimals: 0, Min: -360, Max: 360,
				Tooltip: "How far round — a full turn spaces them evenly",
			}); res.Changed {
			m.sweep = v
		}
	} else {
		dxBox := ui.Rect(r.X+third+a.px(4), r.Y, third, r.Height)
		dyBox := ui.Rect(r.X+2*(third+a.px(4)), r.Y, third, r.Height)
		if v, res := a.UI.DragNumber(ui.MakeID("modify.dx"), dxBox, m.deltaX, ui.NumberOpts{
			Unit: "u", Step: 1, FineStep: 0.25, Decimals: 2, Min: -64, Max: 64,
			Tooltip: "Step across",
		}); res.Changed {
			m.deltaX = v
		}
		if v, res := a.UI.DragNumber(ui.MakeID("modify.dy"), dyBox, m.deltaY, ui.NumberOpts{
			Unit: "u", Step: 1, FineStep: 0.25, Decimals: 2, Min: -64, Max: 64,
			Tooltip: "Step up",
		}); res.Changed {
			m.deltaY = v
		}
	}
}
