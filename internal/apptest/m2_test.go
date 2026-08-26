package apptest

import (
	"regexp"
	"strings"
	"testing"
)

// M2 flow tests. The scripts draw with the real tools — some through the op
// set, some through synthetic clicks that go down the same path a mouse does —
// and then assert on the arrangement the region engine produced.

var sketchLine = regexp.MustCompile(
	`^sketch active="([^"]*)" entities=(\d+) regions=(\d+) openends=(\d+) tool="(\w+)"$`)

type sketchDump struct {
	name     string
	entities int
	regions  int
	openEnds int
	tool     string
	present  bool
}

// parseSketchDumps pulls the active-sketch line out of each dump block. A dump
// with no sketch being edited yields a zero value with present false.
func parseSketchDumps(t *testing.T, stdout string) []sketchDump {
	t.Helper()
	var out []sketchDump
	var cur *sketchDump

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "doc "):
			out = append(out, sketchDump{})
			cur = &out[len(out)-1]
		case cur != nil && strings.HasPrefix(line, "sketch active="):
			m := sketchLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable sketch line: %q", line)
			}
			cur.name = m[1]
			cur.entities = atoi(t, m[2])
			cur.regions = atoi(t, m[3])
			cur.openEnds = atoi(t, m[4])
			cur.tool = m[5]
			cur.present = true
		}
	}
	return out
}

func TestGoldenSketch(t *testing.T) {
	_, outDir := runScript(t, "m2_sketch")
	checkGolden(t, "m2_sketch", outDir)
}

func TestGoldenSketchDrawing(t *testing.T) {
	_, outDir := runScript(t, "m2_draw")
	checkGolden(t, "m2_draw", outDir)
}

func TestGoldenSketchTools(t *testing.T) {
	_, outDir := runScript(t, "m2_tools")
	checkGolden(t, "m2_tools", outDir)
}

// TestRectAndCircleGiveTwoRegions is M2's acceptance, stated directly: draw a
// rectangle with a circle inside it and get two filled regions.
func TestRectAndCircleGiveTwoRegions(t *testing.T) {
	stdout, _ := runScript(t, "m2_sketch")
	dumps := parseSketchDumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}

	empty, drawn, afterFinish := dumps[0], dumps[1], dumps[2]

	if !empty.present {
		t.Fatal("sketch.begin did not enter sketch mode")
	}
	if empty.entities != 0 || empty.regions != 0 {
		t.Errorf("a new sketch already has %d entities and %d regions",
			empty.entities, empty.regions)
	}

	if drawn.entities != 2 {
		t.Errorf("entities = %d, want the rectangle and the circle", drawn.entities)
	}
	if drawn.regions != 2 {
		t.Errorf("regions = %d, want 2: the ring and the disc", drawn.regions)
	}
	if drawn.openEnds != 0 {
		t.Errorf("open ends = %d, want none on two closed shapes", drawn.openEnds)
	}

	// Finishing leaves sketch mode but keeps the sketch (SPEC-UX §8.7).
	if afterFinish.present {
		t.Error("sketch.finish did not leave sketch mode")
	}
	docs := parseDumps(t, stdout)
	if docs[len(docs)-1].sketches != 1 {
		t.Error("finishing the sketch discarded it")
	}
}

// TestOpenProfileGatesExtrude is R3 through the whole app: an open chain shows
// its loose ends and offers no region, and closing it produces one.
func TestOpenProfileGatesExtrude(t *testing.T) {
	stdout, _ := runScript(t, "m2_draw")
	dumps := parseSketchDumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	open, closed, undone := dumps[0], dumps[1], dumps[2]

	// Three clicks place two segments and leave both ends loose.
	if open.entities != 2 {
		t.Errorf("three clicks produced %d entities, want 2", open.entities)
	}
	if open.regions != 0 {
		t.Errorf("an open chain produced %d regions, want none", open.regions)
	}
	if open.openEnds != 2 {
		t.Errorf("open ends = %d, want 2", open.openEnds)
	}

	// Clicking the chain's own start closes it.
	if closed.entities != 3 {
		t.Errorf("after closing, entities = %d, want 3", closed.entities)
	}
	if closed.regions != 1 {
		t.Errorf("the closed triangle gives %d regions, want 1", closed.regions)
	}
	if closed.openEnds != 0 {
		t.Errorf("the closed triangle still has %d open ends", closed.openEnds)
	}

	// Every stroke is its own undo step, so undoing the closing click reopens
	// the profile rather than wiping the sketch.
	if undone.entities != 2 || undone.regions != 0 || undone.openEnds != 2 {
		t.Errorf("after undo: %d entities, %d regions, %d open ends; want 2, 0, 2",
			undone.entities, undone.regions, undone.openEnds)
	}
}

// TestSketchToolsDrawShapes covers the rectangle and circle tools through
// synthetic clicks, including the tool switch between them.
func TestSketchToolsDrawShapes(t *testing.T) {
	stdout, _ := runScript(t, "m2_tools")
	dumps := parseSketchDumps(t, stdout)
	if len(dumps) != 1 {
		t.Fatalf("expected 1 dump, got %d:\n%s", len(dumps), stdout)
	}
	d := dumps[0]

	if d.entities != 2 {
		t.Errorf("entities = %d, want a rectangle and a circle", d.entities)
	}
	if d.regions != 2 {
		t.Errorf("regions = %d, want the ring and the disc", d.regions)
	}
	if d.openEnds != 0 {
		t.Errorf("open ends = %d, want none", d.openEnds)
	}
	if d.tool != "Circle" {
		t.Errorf("active tool = %q, want Circle", d.tool)
	}
}

// TestEveryDrawnStrokeIsUndoable keeps the promise that undo works everywhere:
// the whole sketch can be unwound one stroke at a time.
func TestEveryDrawnStrokeIsUndoable(t *testing.T) {
	stdout, _ := runScript(t, "m2_draw")
	docs := parseDumps(t, stdout)
	if len(docs) < 2 {
		t.Fatalf("expected several dumps, got %d", len(docs))
	}
	// One step for the sketch itself plus one per stroke.
	if docs[1].undo < 4 {
		t.Errorf("undo depth after drawing = %d, want at least 4", docs[1].undo)
	}
	if docs[len(docs)-1].redo < 1 {
		t.Error("undoing a stroke left nothing to redo")
	}
}

// TestSketchAppearsInTheTree covers the tree section of SPEC-UX §7.
func TestSketchAppearsInTheTree(t *testing.T) {
	stdout, _ := runScript(t, "m2_sketch")
	docs := parseDumps(t, stdout)
	for i, d := range docs {
		if d.sketches != 1 {
			t.Errorf("dump %d reports %d sketches, want 1", i, d.sketches)
		}
	}
}
