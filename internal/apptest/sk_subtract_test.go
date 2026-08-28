package apptest

import (
	"math"
	"strings"
	"testing"
)

// The user's report, 2026-08-27: "subtract in extruding on a sketch on a model
// is not working."
//
// Two separate faults, and the script covers both.
//
// The one that made it fail every time: a sketch drawn on a face opens with
// Result=Add and the arrow pointing outward, which is right for adding. Picking
// Subtract left the arrow pointing the same way, so the solid sat against the
// *outside* of the body — the boolean ran, and took nothing away.
//
// The one that made it silent: bodiesReachedBy offers a target when bounding
// boxes overlap, which its own comment says can mean "a chip being offered that
// turns out to do nothing". When that happens the operation succeeds, lands in
// the history, and leaves the model identical — indistinguishable, without a
// word, from a broken tool or a click that missed.

func TestSubtractOnAFaceSketchCutsIntoTheBody(t *testing.T) {
	stdout, _ := runScript(t, "sk_subtract_nothing")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	beforeFace, afterFace := dumps[3], dumps[4]

	// The profile is 2 x 1 and the depth 3, so a cut that goes the right way
	// takes 6 units out. Going the wrong way takes none.
	start := beforeFace.body(t, "Hull").vol
	got := afterFace.body(t, "Hull").vol
	if want := start - 6; math.Abs(got-want) > 1e-6 {
		t.Errorf("the face-sketch subtract left %.4f, want %.4f\n"+
			"  removed %.4f; zero means the solid was still aimed out of the body",
			got, want, start-got)
	}
	// It worked, so it must not claim otherwise.
	if mentionsEmptyChange(afterFace.toasts) {
		t.Errorf("a subtract that really cut was reported as removing nothing:\n%q",
			afterFace.toasts)
	}
}

func TestASubtractThatRemovesNothingSaysSo(t *testing.T) {
	stdout, _ := runScript(t, "sk_subtract_nothing")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	start, afterMiss := dumps[0], dumps[2]

	// The hull is an L: a slab with a block raised off one end. The script cuts
	// at a spot inside the hull's bounding box but out in the air beside that
	// block, so the boxes overlap and the geometry does not. Nothing is removed
	// — which is legitimate, and has to be said.
	before := start.body(t, "Hull").vol
	after := afterMiss.body(t, "Hull").vol
	if math.Abs(after-before) > 1e-6 {
		t.Fatalf("the cut removed %.4f — this case needs one that removes nothing",
			before-after)
	}
	if !mentionsEmptyChange(afterMiss.toasts) {
		t.Errorf("a subtract that removed nothing said nothing.\ntoasts: %q",
			afterMiss.toasts)
	}
}

// mentionsEmptyChange reports whether any toast tells the user the operation
// left the model as it was.
func mentionsEmptyChange(toasts []string) bool {
	for _, s := range toasts {
		l := strings.ToLower(s)
		if strings.Contains(l, "removed nothing") ||
			strings.Contains(l, "added nothing") ||
			strings.Contains(l, "changed nothing") {
			return true
		}
	}
	return false
}
