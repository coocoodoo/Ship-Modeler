package app

import (
	"testing"

	"modeler/internal/geom"
	"modeler/internal/render"
)

// The grid step drives both the drawn grid and the snap, from one setting.
// These pin the snap half; the drawn half is pinned by TestGoldenGridStep.

func TestSnapFollowsTheGridStepSetting(t *testing.T) {
	a := bareApp()
	vp := render.Viewport{W: 1280, H: 720}
	in := NewInputFrame()

	a.Settings.GridStep = 0.5
	if got := a.snapConfig(in, vp).GridStep; got != geom.ToSubunits(0.5) {
		t.Errorf("grid 0.5 u snapped at %d subunits, want %d", got, geom.ToSubunits(0.5))
	}

	// A broken setting falls back to one unit rather than snapping to nothing.
	a.Settings.GridStep = 0
	if got := a.snapConfig(in, vp).GridStep; got != geom.SubunitsPerUnit {
		t.Errorf("grid 0 snapped at %d subunits, want the 1 u fallback %d", got, geom.SubunitsPerUnit)
	}

	// Ctrl is the fine override whatever the chips say (SPEC-UX §16).
	a.Settings.GridStep = 2
	in.Ctrl = true
	if got := a.snapConfig(in, vp).GridStep; got != geom.SubunitsFine {
		t.Errorf("Ctrl snapped at %d subunits, want the fine step %d", got, geom.SubunitsFine)
	}
}
