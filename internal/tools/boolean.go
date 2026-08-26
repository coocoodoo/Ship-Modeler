package tools

import (
	"fmt"

	"modeler/internal/geom/csg"
)

// The Boolean tool (R12, SPEC-UX §11): pick bodies, pick an operation, apply.
//
// Picking is one gesture for all three operations — the first body clicked is
// the one that survives and keeps its name and colour, and everything clicked
// after it is a tool. Only the words in the hint bar change, because that is
// the only thing that actually differs between merging two hulls and cutting a
// window out of one.

// BooleanTool is the live state of one boolean interaction.
type BooleanTool struct {
	Op csg.Op
	// KeepTools leaves the tool bodies behind instead of consuming them. Off by
	// default (SPEC-UX §11).
	KeepTools bool

	// Target is the body that survives, and Tools are the ones combined into
	// it. Zero means nothing picked yet.
	Target uint32
	Tools  []uint32
}

// NewBooleanTool opens the tool with nothing picked.
func NewBooleanTool() *BooleanTool { return &BooleanTool{Op: csg.Union} }

// Pick adds a body to the selection, or takes it out again if it is already
// there. Clicking a picked body to unpick it is the only way back, so it has to
// work at every stage (SPEC-UX §11).
func (t *BooleanTool) Pick(id uint32) {
	if id == 0 {
		return
	}
	if id == t.Target {
		// The target steps aside and the next body picked takes its place, so
		// unpicking never silently reassigns what survives.
		t.Target = 0
		return
	}
	for i, x := range t.Tools {
		if x == id {
			t.Tools = append(t.Tools[:i], t.Tools[i+1:]...)
			return
		}
	}
	if t.Target == 0 {
		t.Target = id
		return
	}
	t.Tools = append(t.Tools, id)
}

// Picked reports whether a body is in the selection, and whether it is the one
// that survives — which is what decides how it is tinted.
func (t *BooleanTool) Picked(id uint32) (picked, isTarget bool) {
	if id != 0 && id == t.Target {
		return true, true
	}
	for _, x := range t.Tools {
		if x == id {
			return true, false
		}
	}
	return false, false
}

// Clear drops the whole selection without closing the tool.
func (t *BooleanTool) Clear() { t.Target, t.Tools = 0, nil }

// Count is how many bodies are picked in total.
func (t *BooleanTool) Count() int {
	if t.Target == 0 {
		return len(t.Tools)
	}
	return 1 + len(t.Tools)
}

// Valid reports whether the tool has enough to apply, and what is missing if
// not (SPEC-UX §15).
func (t *BooleanTool) Valid() (bool, string) {
	if t.Target == 0 {
		return false, t.firstPrompt()
	}
	if len(t.Tools) == 0 {
		return false, t.secondPrompt()
	}
	return true, ""
}

// Hint is what the hint bar says, which is the whole of this tool's guidance
// (SPEC-UX §11).
func (t *BooleanTool) Hint() string {
	if ok, why := t.Valid(); !ok {
		return why + " · Esc to cancel"
	}
	return fmt.Sprintf("%s %d bodies · Enter to apply · Esc to cancel",
		t.Op, t.Count())
}

func (t *BooleanTool) firstPrompt() string {
	switch t.Op {
	case csg.Subtract:
		return "Click the body to keep"
	case csg.Intersect:
		return "Click two or more bodies — the overlap survives"
	default:
		return "Click bodies to merge, then Enter to apply"
	}
}

func (t *BooleanTool) secondPrompt() string {
	switch t.Op {
	case csg.Subtract:
		return "Now click the bodies to remove"
	case csg.Intersect:
		return "Click at least one more body — the overlap survives"
	default:
		return "Click at least one more body to merge in"
	}
}
