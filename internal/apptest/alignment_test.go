package apptest

import (
	"strings"
	"testing"
)

func TestSketchEndpointAlignment(t *testing.T) {
	out, dir := runScript(t, "sketch_alignment")
	if !strings.Contains(out, `sketch active="Sketch 1" entities=5 construction=0 regions=0 openends=2`) {
		t.Fatalf("aligned click did not extend the open sketch: %s", out)
	}
	checkGolden(t, "sketch_alignment", dir)
}
