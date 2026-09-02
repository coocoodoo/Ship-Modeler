package app

import (
	"testing"

	"modeler/internal/paint"
	"modeler/internal/ui"
)

// The colour system (SPEC-UX §3.1): the chip and the accent are one
// partition of the modes, and every tool button knows its own colour.

func TestIdleWearsTheModelAccent(t *testing.T) {
	a := bareApp()
	if a.modeAccent() != ui.AccentModel {
		t.Errorf("idle accent = %+v, want the model accent", a.modeAccent())
	}
	if a.modeName() != "MODEL" {
		t.Errorf("idle chip = %q, want MODEL", a.modeName())
	}
}

func TestEveryModeButtonHasItsOwnColour(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range toolbarTools() {
		c := modeColor(tool.mode)
		if c.A == 0 {
			t.Errorf("%s has no colour", tool.label)
		}
		key := ui.WithAlpha(c, 0xFF)
		k := string([]byte{key.R, key.G, key.B})
		// Move is idle's colour on purpose: it is not a mode.
		if seen[k] && tool.label != "Move" {
			t.Errorf("%s shares its colour with another mode", tool.label)
		}
		seen[k] = true
	}
}

func TestPaintToolsGroupByWhatTheyDo(t *testing.T) {
	if paintToolAccent(paint.ToolPencil) != ui.AccentPaint || paintToolAccent(paint.ToolRect) != ui.AccentPaint {
		t.Error("the tools that put pixels down should wear the paint accent")
	}
	if paintToolAccent(paint.ToolWand) != ui.AccentModel || paintToolAccent(paint.ToolPick) != ui.AccentModel {
		t.Error("the pickers select, so they should wear the selection colour")
	}
	if paintToolAccent(paint.ToolEdge) != ui.AccentExtrude || paintToolAccent(paint.ToolTile) != ui.AccentExtrude {
		t.Error("edges and tiles build structure, so they should wear the extrude accent")
	}
}
