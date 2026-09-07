package apptest

import (
	"strings"
	"testing"
)

func TestBodyCopyAndExtrudeTargets(t *testing.T) {
	out, dir := runScript(t, "body_copy_extrude_targets")
	for _, kind := range []string{"face", "vertex", "edge"} {
		if !strings.Contains(out, `extent mode="`+kind+`" ready=1`) {
			t.Fatalf("missing target %s: %s", kind, out)
		}
	}
	checkGolden(t, "body_copy_extrude_targets", dir)
}
