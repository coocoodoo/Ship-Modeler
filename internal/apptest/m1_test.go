package apptest

import (
	"regexp"
	"strings"
	"testing"
)

// M1 flow tests. The scripts drive the real chrome through synthetic clicks and
// then dump the document state, so a regression is reported as a behaviour
// rather than as a picture that happened to change.

// docLine parses one "doc planes=... bodies=N ..." dump line.
var docLine = regexp.MustCompile(
	`^doc planes=(\S+) bodies=(\d+) sketches=(\d+) undo=(\d+) redo=(\d+) dirty=(\d)$`)

type docDump struct {
	planes           map[string]bool
	bodies, sketches int
	undo, redo       int
	dirty            bool
	sel              int
	selDesc          string
	hint             string
	toasts           []string
	bodyNames        []string
	bodyVisible      map[string]bool
}

// parseDumps splits a script's stdout into one docDump per dump op.
func parseDumps(t *testing.T, stdout string) []docDump {
	t.Helper()
	var out []docDump
	var cur *docDump

	bodyLine := regexp.MustCompile(`^body id=\d+ name="([^"]*)" visible=(\d) `)
	selLine := regexp.MustCompile(`^sel count=(\d+) desc="([^"]*)"$`)
	hintLine := regexp.MustCompile(`^hint "(.*)"$`)
	toastLine := regexp.MustCompile(`^toast "(.*)"$`)

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "doc "):
			m := docLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable doc line: %q", line)
			}
			d := docDump{planes: map[string]bool{}, bodyVisible: map[string]bool{}}
			for _, p := range strings.Split(m[1], ",") {
				parts := strings.Split(p, ":")
				d.planes[parts[0]] = parts[1] == "1"
			}
			d.bodies = atoi(t, m[2])
			d.sketches = atoi(t, m[3])
			d.undo = atoi(t, m[4])
			d.redo = atoi(t, m[5])
			d.dirty = m[6] == "1"
			out = append(out, d)
			cur = &out[len(out)-1]
		case cur == nil:
			continue
		case strings.HasPrefix(line, "body "):
			m := bodyLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable body line: %q", line)
			}
			cur.bodyNames = append(cur.bodyNames, m[1])
			cur.bodyVisible[m[1]] = m[2] == "1"
		case strings.HasPrefix(line, "sel "):
			m := selLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable sel line: %q", line)
			}
			cur.sel = atoi(t, m[1])
			cur.selDesc = m[2]
		case strings.HasPrefix(line, "hint "):
			if m := hintLine.FindStringSubmatch(line); m != nil {
				cur.hint = m[1]
			}
		case strings.HasPrefix(line, "toast "):
			if m := toastLine.FindStringSubmatch(line); m != nil {
				cur.toasts = append(cur.toasts, m[1])
			}
		}
	}
	return out
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func TestGoldenShell(t *testing.T) {
	_, outDir := runScript(t, "m1_shell")
	checkGolden(t, "m1_shell", outDir)
}

func TestGoldenPlanes(t *testing.T) {
	_, outDir := runScript(t, "m1_planes")
	checkGolden(t, "m1_planes", outDir)
}

func TestGoldenTree(t *testing.T) {
	_, outDir := runScript(t, "m1_tree")
	checkGolden(t, "m1_tree", outDir)
}

// TestPlaneVisibilityIsUndoable is the M1 acceptance check for R1: planes hide
// and show, and every toggle goes through the bus so undo restores it.
func TestPlaneVisibilityIsUndoable(t *testing.T) {
	stdout, _ := runScript(t, "m1_planes")
	dumps := parseDumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}

	start, hidden, restored := dumps[0], dumps[1], dumps[2]

	for name, vis := range start.planes {
		if !vis {
			t.Errorf("plane %s starts hidden", name)
		}
	}
	if start.undo != 0 || start.dirty {
		t.Errorf("the starting document already has history (undo=%d dirty=%v)", start.undo, start.dirty)
	}

	if hidden.planes["Top"] || hidden.planes["Right"] {
		t.Error("hiding the Top and Right planes did not take effect")
	}
	if !hidden.planes["Front"] {
		t.Error("hiding two planes also hid the third")
	}
	if hidden.undo != 2 {
		t.Errorf("two toggles produced %d undo steps, want 2", hidden.undo)
	}

	for name, vis := range restored.planes {
		if !vis {
			t.Errorf("undo did not restore the %s plane", name)
		}
	}
	if restored.undo != 0 || restored.redo != 2 {
		t.Errorf("after two undos the history is undo=%d redo=%d, want 0 and 2",
			restored.undo, restored.redo)
	}
}

// TestTreeClicksToggleAndSelect drives the tree panel with real synthetic
// clicks: one on a plane's eye toggle, one on a body row.
func TestTreeClicksToggleAndSelect(t *testing.T) {
	stdout, _ := runScript(t, "m1_tree")
	dumps := parseDumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	afterClicks := dumps[0]

	if afterClicks.planes["Top"] {
		t.Error("clicking the Top plane's eye did not hide it")
	}
	if !afterClicks.planes["Front"] || !afterClicks.planes["Right"] {
		t.Error("clicking one eye toggle affected the other planes")
	}
	if afterClicks.undo != 1 {
		t.Errorf("the eye toggle produced %d undo steps, want 1", afterClicks.undo)
	}
	if afterClicks.sel != 1 || afterClicks.selDesc != "Hull" {
		t.Errorf("clicking the Hull row selected %d item(s) (%q), want just Hull",
			afterClicks.sel, afterClicks.selDesc)
	}
	// Selecting is not a document change, so it must not join the history.
	if afterClicks.undo != 1 {
		t.Error("selecting a row pushed an undo step")
	}
	if afterClicks.hint == "" {
		t.Error("the hint bar is empty, and silence is never the answer")
	}
}

// TestDefaultPlanesCannotBeDeleted covers R1's other half: deleting a plane is
// refused, and the refusal explains what to do instead.
func TestDefaultPlanesCannotBeDeleted(t *testing.T) {
	stdout, _ := runScript(t, "m1_tree")
	dumps := parseDumps(t, stdout)
	final := dumps[len(dumps)-1]

	if !final.planes["Front"] {
		t.Fatal("the Front plane was deleted")
	}
	found := false
	for _, msg := range final.toasts {
		if strings.Contains(msg, "can't be deleted") && strings.Contains(msg, "hide") {
			found = true
		}
	}
	if !found {
		t.Errorf("no toast explained why the plane survived; toasts were %q", final.toasts)
	}
}

// TestTreeCollapses covers the panel handle: the viewport grows to fill the
// space and the shell still renders.
func TestTreeCollapses(t *testing.T) {
	_, outDir := runScript(t, "m1_shell")
	// Both shots exist, and the collapsed one differs from the expanded one:
	// the panel really went away rather than the op being ignored.
	expanded, err := LoadPNG(outDir + "/m1_shell.png")
	if err != nil {
		t.Fatal(err)
	}
	collapsed, err := LoadPNG(outDir + "/m1_tree_collapsed.png")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Compare(expanded, collapsed)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Differs() {
		t.Errorf("collapsing the tree panel changed nothing on screen: %s", res)
	}
}

// TestStatsLineCountsTheDocument checks the tree footer's arithmetic against
// the bodies the dump reports.
func TestStatsLineCountsTheDocument(t *testing.T) {
	stdout, _ := runScript(t, "m1_planes")
	d := parseDumps(t, stdout)[0]
	if d.bodies != 3 {
		t.Fatalf("the test scene has %d bodies, want 3", d.bodies)
	}
	want := []string{"Hull", "Engine pod", "Wing pod"}
	for i, name := range want {
		if i >= len(d.bodyNames) || d.bodyNames[i] != name {
			t.Errorf("body %d is %q, want %q", i, d.bodyNames, want)
			break
		}
	}
}
