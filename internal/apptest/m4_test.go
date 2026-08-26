package apptest

import (
	"regexp"
	"strings"
	"testing"
)

// M4 flow tests: the boolean kernel reaching the app. The kernel's own
// correctness is settled by the acceptance matrix in internal/geom/csg; what
// these check is the wiring — that the right bodies are chosen, the right one
// survives, the tools are consumed, and one Ctrl+Z puts it all back.

var m4BooleanLine = regexp.MustCompile(
	`^boolean op="(\w+)" target=(\d+) tools=(\d+) keep=(\d)$`)

type booleanDump struct {
	op      string
	target  int
	tools   int
	keep    bool
	present bool
}

// parseBooleanDumps pulls the open tool's line out of each dump block.
func parseBooleanDumps(t *testing.T, stdout string) []booleanDump {
	t.Helper()
	var out []booleanDump
	var cur *booleanDump
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "doc "):
			out = append(out, booleanDump{})
			cur = &out[len(out)-1]
		case cur != nil && strings.HasPrefix(line, "boolean op="):
			m := m4BooleanLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable boolean line: %q", line)
			}
			cur.op = m[1]
			cur.target = atoi(t, m[2])
			cur.tools = atoi(t, m[3])
			cur.keep = m[4] == "1"
			cur.present = true
		}
	}
	return out
}

// openBooleanTool finds the dump taken while the tool was open.
func openBooleanTool(dumps []booleanDump) (booleanDump, bool) {
	for _, d := range dumps {
		if d.present {
			return d, true
		}
	}
	return booleanDump{}, false
}

func TestGoldenBooleanTool(t *testing.T) {
	_, outDir := runScript(t, "m4_boolean")
	checkGolden(t, "m4_boolean", outDir)
}

func TestGoldenExtrudeCut(t *testing.T) {
	_, outDir := runScript(t, "m4_cut")
	checkGolden(t, "m4_cut", outDir)
}

func TestGoldenExtrudeUnion(t *testing.T) {
	_, outDir := runScript(t, "m4_union")
	checkGolden(t, "m4_union", outDir)
}

// TestBooleanToolCutsAndConsumes is M4's acceptance through the UI: pick the
// body to keep, pick the tool, apply. What survives keeps its name, what was
// used up is gone, and the volume says the cut actually happened.
func TestBooleanToolCutsAndConsumes(t *testing.T) {
	stdout, _ := runScript(t, "m4_boolean")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	built, picked, applied := dumps[0], dumps[1], dumps[2]

	if !built.hasBody("Body 4") {
		t.Fatal("the block to cut with was not built")
	}
	before := built.body(t, "Hull").vol
	block := built.body(t, "Body 4").vol

	// Mid-pick: the tool knows what it is about to do.
	tool, ok := openBooleanTool(parseBooleanDumps(t, stdout))
	if !ok {
		t.Fatal("no dump caught the boolean tool open")
	}
	if tool.op != "Subtract" || tool.tools != 1 || tool.keep {
		t.Errorf("tool reports op=%q tools=%d keep=%v, want Subtract, 1, false",
			tool.op, tool.tools, tool.keep)
	}

	// Applied: the hull lost the overlap and kept its identity.
	after := applied.body(t, "Hull")
	if after.vol >= before {
		t.Errorf("the hull's volume did not fall: %v then %v", before, after.vol)
	}
	if after.vol <= before-block {
		t.Errorf("the hull lost %v, which is the whole block (%v) or more — "+
			"the tool was not clipped to the overlap", before-after.vol, block)
	}
	if applied.hasBody("Body 4") {
		t.Error("the tool body was not consumed")
	}
	if !picked.hasBody("Body 4") {
		t.Error("the tool body vanished before the boolean ran")
	}

	var summarised bool
	for _, msg := range applied.toasts {
		if strings.Contains(msg, "Subtract") && strings.Contains(msg, "Hull") {
			summarised = true
		}
	}
	if !summarised {
		t.Errorf("no toast summarised the operation; toasts were %q", applied.toasts)
	}
}

// TestExtrudeSubtractCutsAWindow is the R8 wiring: an extrude whose Result is
// Subtract cuts into the body it reaches instead of making a new one.
func TestExtrudeSubtractCutsAWindow(t *testing.T) {
	stdout, _ := runScript(t, "m4_cut")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, open, after := dumps[0], dumps[1], dumps[2]

	if open.extrude.result != "Subtract" {
		t.Errorf("the tool reports result %q, want Subtract", open.extrude.result)
	}
	if open.extrude.targets == 0 {
		t.Error("the tool found nothing to cut into, so the chip should be disabled")
	}

	hullBefore := before.body(t, "Hull").vol
	hullAfter := after.body(t, "Hull")
	if hullAfter.vol >= hullBefore {
		t.Errorf("the hull's volume did not fall: %v then %v", hullBefore, hullAfter.vol)
	}
	// A 3x3 column straight through: whatever thickness it passed through, the
	// loss has to be a whole number of 9-unit slices.
	lost := hullBefore - hullAfter.vol
	if lost <= 0 || lost/9 != float64(int(lost/9)) {
		t.Errorf("the cut removed %v, which is not a run of 3x3 column", lost)
	}
	// No new body: the whole point of Subtract is that it edits what is there.
	if len(after.bodies) != len(before.bodies) {
		t.Errorf("body count went from %d to %d", len(before.bodies), len(after.bodies))
	}
	if after.sketchVisible["Sketch 1"] {
		t.Error("the consumed sketch should have hidden itself")
	}
}

// TestExtrudeAddMergesIntoTheBodyItReaches covers the other half of R8.
func TestExtrudeAddMergesIntoTheBodyItReaches(t *testing.T) {
	stdout, _ := runScript(t, "m4_union")
	dumps := parseM3Dumps(t, stdout)
	before, after := dumps[0], dumps[len(dumps)-1]

	hullBefore := before.body(t, "Hull").vol
	hullAfter := after.body(t, "Hull")
	if hullAfter.vol <= hullBefore {
		t.Errorf("the hull did not grow: %v then %v", hullBefore, hullAfter.vol)
	}
	if len(after.bodies) != len(before.bodies) {
		t.Errorf("Add created a body instead of merging: %d then %d",
			len(before.bodies), len(after.bodies))
	}
	if hullAfter.tris <= before.body(t, "Hull").tris {
		t.Error("the hull's mesh did not change")
	}
}

// TestBooleanIsOneUndoStep: a boolean has to be as reversible as anything else,
// tools and all.
func TestBooleanIsOneUndoStep(t *testing.T) {
	stdout, _ := runScript(t, "m4_undo")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	done, undone, redone := dumps[0], dumps[1], dumps[2]

	if done.hasBody("Wing pod") {
		t.Error("the tool body survived the boolean")
	}
	if !undone.hasBody("Wing pod") {
		t.Error("undo did not bring the tool body back")
	}
	if undone.body(t, "Hull").vol != 348 {
		t.Errorf("undo left the hull at %v, want its original 348",
			undone.body(t, "Hull").vol)
	}
	if undone.undo != done.undo-1 || undone.redo != 1 {
		t.Errorf("after undo: undo=%d redo=%d, want %d and 1",
			undone.undo, undone.redo, done.undo-1)
	}
	if redone.hasBody("Wing pod") {
		t.Error("redo did not consume the tool body again")
	}
	if redone.body(t, "Hull").vol != done.body(t, "Hull").vol {
		t.Error("redo produced a different solid")
	}
}

// TestKeepOriginalBodiesLeavesTheTools is the toggle of SPEC-UX §11.
func TestKeepOriginalBodiesLeavesTheTools(t *testing.T) {
	stdout, _ := runScript(t, "m4_keep")
	dumps := parseM3Dumps(t, stdout)
	before, after := dumps[0], dumps[len(dumps)-1]

	if !after.hasBody("Wing pod") {
		t.Error("Keep original bodies was on and the tool was consumed anyway")
	}
	if len(after.bodies) != len(before.bodies) {
		t.Errorf("body count changed from %d to %d with Keep on",
			len(before.bodies), len(after.bodies))
	}
	tool, ok := openBooleanTool(parseBooleanDumps(t, stdout))
	if !ok {
		t.Fatal("no dump caught the boolean tool open")
	}
	if !tool.keep {
		t.Error("the tool does not report Keep as on")
	}
}

// TestAddWithNothingInReachFallsBackToNew is the rule of SPEC-UX §9.4: an Add
// that finds no body to combine with makes one instead of refusing, and says
// what it did rather than leaving you to work it out.
func TestAddWithNothingInReachFallsBackToNew(t *testing.T) {
	stdout, _ := runScript(t, "m4_fallback")
	dumps := parseM3Dumps(t, stdout)
	open, after := dumps[0], dumps[len(dumps)-1]

	if open.extrude.targets != 0 {
		t.Errorf("the tool found %d bodies in reach, want none", open.extrude.targets)
	}
	if !after.hasBody("Body 4") {
		t.Fatal("no new body was created")
	}
	if after.body(t, "Body 4").vol != 48 {
		t.Errorf("the new body's volume is %v, want 4*4*3", after.body(t, "Body 4").vol)
	}

	var explained bool
	for _, msg := range after.toasts {
		if strings.Contains(msg, "Nothing to combine with") {
			explained = true
		}
	}
	if !explained {
		t.Errorf("the fallback was silent; toasts were %q", after.toasts)
	}
}

// TestAnEmptyResultIsLegalAndUndoable is SPEC-GEOMETRY §6.4: intersecting
// solids that do not meet leaves nothing, which is an answer rather than an
// error. The body goes, a toast says so, and undo brings it back.
func TestAnEmptyResultIsLegalAndUndoable(t *testing.T) {
	stdout, _ := runScript(t, "m4_empty")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	done, undone := dumps[0], dumps[1]

	if done.hasBody("Hull") || done.hasBody("Wing pod") {
		t.Error("an empty intersection left bodies behind")
	}
	var said bool
	for _, msg := range done.toasts {
		if strings.Contains(msg, "nothing left") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing explained the empty result; toasts were %q", done.toasts)
	}

	if !undone.hasBody("Hull") || !undone.hasBody("Wing pod") {
		t.Error("undo did not restore both bodies")
	}
	if undone.body(t, "Hull").vol != 348 {
		t.Errorf("the restored hull has volume %v, want 348", undone.body(t, "Hull").vol)
	}
}
