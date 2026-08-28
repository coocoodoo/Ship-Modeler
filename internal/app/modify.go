package app

import (
	"fmt"

	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/sketch"
	"modeler/internal/ui"
)

// The modify operations of Sketch_func.md §5 SK5, at the app layer: they take
// a selection, ask package sketch for the geometry, and land the result as one
// ReplaceEntities (V-98).
//
// Every one is written so the card and the script op call the same function.
// A scripted fillet and a clicked one are then the same fillet, which is what
// makes the goldens worth anything.

// MirrorAxis names the line a mirror reflects about.
type MirrorAxis uint8

const (
	// MirrorVertical reflects left to right about the sketch's V axis, which
	// is the one a symmetrical hull is built on.
	MirrorVertical MirrorAxis = iota
	// MirrorHorizontal reflects top to bottom about the U axis.
	MirrorHorizontal
)

func (a MirrorAxis) String() string {
	if a == MirrorHorizontal {
		return "Horizontal"
	}
	return "Vertical"
}

// axisLine is the two points defining the axis, in sketch coordinates.
func (a MirrorAxis) axisLine() (geom.Vec2i, geom.Vec2i) {
	const far = 64 * geom.SubunitsPerUnit
	if a == MirrorHorizontal {
		return geom.Vec2i{X: -far}, geom.Vec2i{X: far}
	}
	return geom.Vec2i{Y: -far}, geom.Vec2i{Y: far}
}

// PatternKind is which way a pattern repeats.
type PatternKind uint8

const (
	PatternLinear PatternKind = iota
	PatternCircular
)

func (k PatternKind) String() string {
	if k == PatternCircular {
		return "Circular"
	}
	return "Linear"
}

// selectedEntities is the sketch entities the Select tool has picked, with
// their indices, validated against the sketch as it is now.
func (a *App) selectedEntities() (*model.Sketch, []int, []model.Entity, bool) {
	s := a.ActiveSketch()
	sess := a.sketch.session
	if s == nil || sess == nil || len(sess.Selected) == 0 {
		return nil, nil, nil, false
	}
	idx := make([]int, 0, len(sess.Selected))
	ents := make([]model.Entity, 0, len(sess.Selected))
	for _, i := range sess.Selected {
		if i < 0 || i >= len(s.Entities) {
			continue
		}
		idx = append(idx, i)
		ents = append(ents, s.Entities[i])
	}
	if len(idx) == 0 {
		return nil, nil, nil, false
	}
	return s, idx, ents, true
}

// FilletSelection rounds the corner between two selected lines.
func (a *App) FilletSelection(radius float64) bool {
	return a.cornerOp("Fillet", radius, func(x, y model.Entity, r int64) (sketch.FilletResult, error) {
		return sketch.Fillet(x, y, r)
	})
}

// ChamferSelection cuts the corner between two selected lines.
func (a *App) ChamferSelection(distance float64) bool {
	return a.cornerOp("Chamfer", distance, func(x, y model.Entity, r int64) (sketch.FilletResult, error) {
		return sketch.Chamfer(x, y, r)
	})
}

// cornerOp is the shared half of fillet and chamfer: both need exactly two
// selected lines, both replace them, and both refuse with a reason.
func (a *App) cornerOp(name string, size float64,
	build func(model.Entity, model.Entity, int64) (sketch.FilletResult, error)) bool {

	s, idx, ents, ok := a.selectedEntities()
	if !ok || len(ents) != 2 {
		a.Toast(ui.Toast{
			Text: "Select the two lines that meet at the corner",
			Kind: ui.ToastWarn,
		})
		return false
	}
	r := geom.ToSubunits(size)
	res, err := build(ents[0], ents[1], r)
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastWarn})
		return false
	}

	add := append([]model.Entity(nil), res.Lines...)
	if res.Arc.Kind == model.EntArc {
		add = append(add, res.Arc)
	}
	// A guide corner stays a guide.
	if ents[0].Construction && ents[1].Construction {
		for i := range add {
			add[i].Construction = true
		}
	}
	if !a.Run(&model.ReplaceEntities{
		Sketch: s.ID, Remove: idx, Add: add, Label: name,
	}) {
		return false
	}
	a.sketch.session.ClearSelection()
	a.Toast(ui.Toast{Text: name + "ed the corner"})
	return true
}

// MirrorSelection adds a reflected copy of the selection about an axis.
func (a *App) MirrorSelection(axis MirrorAxis) bool {
	s, _, ents, ok := a.selectedEntities()
	if !ok {
		a.Toast(ui.Toast{Text: "Select what to mirror", Kind: ui.ToastWarn})
		return false
	}
	p, q := axis.axisLine()
	add := make([]model.Entity, 0, len(ents))
	for _, e := range ents {
		m := sketch.MirrorEntity(e, p, q)
		if !m.Degenerate() {
			add = append(add, m)
		}
	}
	if len(add) == 0 {
		a.Toast(ui.Toast{Text: "That selection mirrors to nothing", Kind: ui.ToastWarn})
		return false
	}
	// Nothing is removed: a mirror copies (Sketch_func.md §2).
	if !a.Run(&model.ReplaceEntities{
		Sketch: s.ID, Add: add,
		Label: "Mirror " + plural(len(add), "entity", "entities"),
	}) {
		return false
	}
	a.Toast(ui.Toast{
		Text: fmt.Sprintf("Mirrored %s about the %s axis",
			plural(len(add), "entity", "entities"), axis),
	})
	return true
}

// PatternSelection repeats the selection, linearly or about the origin.
func (a *App) PatternSelection(kind PatternKind, count int, delta geom.Vec2i, sweepDeg float64) bool {
	s, _, ents, ok := a.selectedEntities()
	if !ok {
		a.Toast(ui.Toast{Text: "Select what to repeat", Kind: ui.ToastWarn})
		return false
	}
	if count < 2 {
		a.Toast(ui.Toast{Text: "A pattern needs a count of two or more", Kind: ui.ToastWarn})
		return false
	}

	var add []model.Entity
	if kind == PatternCircular {
		add = sketch.CircularPattern(ents, geom.Vec2i{}, count, sweepDeg)
	} else {
		add = sketch.LinearPattern(ents, delta, count)
	}
	if len(add) == 0 {
		a.Toast(ui.Toast{
			Text: "That pattern makes no copies — check the count and the spacing",
			Kind: ui.ToastWarn,
		})
		return false
	}
	if !a.Run(&model.ReplaceEntities{
		Sketch: s.ID, Add: add,
		Label: kind.String() + " pattern",
	}) {
		return false
	}
	a.Toast(ui.Toast{Text: fmt.Sprintf("%s pattern — %s added",
		kind, plural(len(add), "copy", "copies"))})
	return true
}

// OffsetSelection replaces a selected closed loop with one a distance away.
//
// The loop comes from the selected entities' own points rather than from the
// region engine: what is selected is what is offset, and a selection that is
// not one closed loop is refused with a reason rather than guessed at.
func (a *App) OffsetSelection(distance float64) bool {
	s, idx, ents, ok := a.selectedEntities()
	if !ok {
		a.Toast(ui.Toast{Text: "Select a closed shape to offset", Kind: ui.ToastWarn})
		return false
	}
	if len(ents) != 1 || !ents[0].Closed() {
		a.Toast(ui.Toast{
			Text: "Offset works on one closed shape at a time",
			Kind: ui.ToastWarn,
		})
		return false
	}
	moved, err := sketch.OffsetLoop(ents[0].Points(), geom.ToSubunits(distance))
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastWarn})
		return false
	}

	// The result is a closed spline through the moved corners rather than the
	// original kind: an offset circle is still a circle, but an offset polygon
	// with mitred corners is not a regular polygon any more, and pretending
	// otherwise would store a shape that redraws itself wrong.
	out := model.NewSpline(moved, true, model.MinSplineSegs)
	// MinSplineSegs makes every span a straight run, which is exactly what a
	// mitred offset is: corners joined by straight edges.
	out.Construction = ents[0].Construction
	if !a.Run(&model.ReplaceEntities{
		Sketch: s.ID, Remove: idx, Add: []model.Entity{out}, Label: "Offset",
	}) {
		return false
	}
	a.sketch.session.ClearSelection()
	a.Toast(ui.Toast{Text: fmt.Sprintf("Offset by %.2f u", distance)})
	return true
}

// canFillet reports whether the selection is a corner, and why not if not.
func (a *App) canFillet() (bool, string) {
	_, _, ents, ok := a.selectedEntities()
	if !ok || len(ents) != 2 {
		return false, "Select the two lines that meet at the corner"
	}
	if ents[0].Kind != model.EntLine || ents[1].Kind != model.EntLine {
		return false, "A corner is made of two straight lines"
	}
	return true, ""
}

// canOffset reports whether the selection is one closed shape.
func (a *App) canOffset() (bool, string) {
	_, _, ents, ok := a.selectedEntities()
	if !ok || len(ents) != 1 {
		return false, "Select one closed shape to offset"
	}
	if !ents[0].Closed() {
		return false, "Offset needs a closed shape — this one has ends"
	}
	return true, ""
}

// hasSketchSelection reports whether anything is selected to modify.
func (a *App) hasSketchSelection() (bool, string) {
	if _, _, _, ok := a.selectedEntities(); !ok {
		return false, "Select something first — click entities with the Select tool"
	}
	return true, ""
}
