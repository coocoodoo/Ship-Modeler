package apptest

import (
	"math"
	"testing"
)

func TestFaceDragCrossesZeroWithoutOscillating(t *testing.T) {
	stdout, _ := runScript(t, "extrude_drag_direction")
	dumps := parseM3Dumps(t, stdout)
	pp, _ := parseM5Dumps(t, stdout)
	if len(dumps) != 9 {
		t.Fatalf("expected 9 dumps, got %d:\n%s", len(dumps), stdout)
	}
	start := dumps[0].body(t, "Hull").vol
	for i, want := range []float64{0, 3, -1, -1, -1, 1, -1} {
		if !pp[i].present || pp[i].dist != want || pp[i].adding != (want > 0) {
			t.Errorf("drag frame %d: %+v, want signed depth %g", i, pp[i], want)
		}
		if dumps[i].body(t, "Hull").vol != start {
			t.Errorf("drag frame %d changed the body before release", i)
		}
	}
	if removed := start - dumps[7].body(t, "Hull").vol; math.Abs(removed-20) > 1e-9 {
		t.Errorf("release removed %g, want the 5x4 face pushed in by 1", removed)
	}
	if restored := dumps[8].body(t, "Hull").vol; restored != start {
		t.Errorf("one undo restored volume %g, want %g", restored, start)
	}
}
