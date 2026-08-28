package apptest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Dragging a placed dot (the user's request, 2026-08-28).
//
// The whole gesture, through the real pointer path: hover a dot, click it,
// drag its gizmo, release, undo. Every step here is a bug that has actually
// happened to some other tool in this program — a click swallowed by the face
// behind the target, a gizmo that armed on nothing, a drag that moved the
// wrong thing, a "move" that logged two undo entries.

var markerLine = regexp.MustCompile(
	`^marker (\d+) kind="([a-z]+)" at=([-\d.]+),([-\d.]+),([-\d.]+) ` +
		`dir=([-\d.]+),([-\d.]+),([-\d.]+) sel=(\d) hover=(\d) px=([-\d.]+),([-\d.]+)$`)

type markerDump struct {
	index          int
	kind           string
	x, y, z        float64
	sel, hover     bool
	screenX        float64
	screenY        float64
	dirX, dirY, dz float64
}

// markerDumps groups the marker lines by the dump they came from, so a test can
// say "at the third dump" rather than counting lines.
func markerDumps(t *testing.T, stdout string) [][]markerDump {
	t.Helper()
	var out [][]markerDump
	var cur []markerDump
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := markerLine.FindStringSubmatch(line); m != nil {
			num := func(i int) float64 {
				v, err := strconv.ParseFloat(m[i], 64)
				if err != nil {
					t.Fatalf("marker line %q: %v", line, err)
				}
				return v
			}
			idx, _ := strconv.Atoi(m[1])
			cur = append(cur, markerDump{
				index: idx, kind: m[2],
				x: num(3), y: num(4), z: num(5),
				dirX: num(6), dirY: num(7), dz: num(8),
				sel: m[9] == "1", hover: m[10] == "1",
				screenX: num(11), screenY: num(12),
			})
			continue
		}
		// A dump ends at its sel line; the markers always precede it.
		if strings.HasPrefix(line, "sel count=") && cur != nil {
			out = append(out, cur)
			cur = nil
		}
	}
	return out
}

func TestClickingADotSelectsItAndTheGizmoMovesIt(t *testing.T) {
	stdout, _ := runScript(t, "marker_drag")
	dumps := markerDumps(t, stdout)
	if len(dumps) != 6 {
		t.Fatalf("got %d dumps with markers, want 6 — the script changed", len(dumps))
	}
	const front, thruster = 0, 1
	at := func(d int) []markerDump { return dumps[d] }

	// 1. Placed, untouched.
	if m := at(0)[front]; m.sel || m.hover {
		t.Errorf("a freshly placed dot reports sel=%v hover=%v, want neither", m.sel, m.hover)
	}

	// 2. Hovering it swells it and takes the hint bar.
	if m := at(1)[front]; !m.hover {
		t.Error("the pointer is on the front dot and it does not report a hover")
	}
	if want := `hint "Front dot · drag the gizmo to move it"`; !strings.Contains(stdout, want) {
		t.Errorf("the hint bar never said what the dot under the cursor was\nwant a line: %s", want)
	}

	// 3. Clicking selects the dot — not the face it is sitting on.
	//
	// The front dot is placed at x=+6, which is exactly on the hull's +X face.
	// Unless the dot outranks the face in the click order it can never be
	// picked at all, because the face behind it always wins the ID pass.
	m := at(2)[front]
	if !m.sel {
		t.Fatal("clicking the dot did not select it — the face behind it took the click")
	}
	if !strings.Contains(stdout, `sel count=1 desc="Front dot"`) {
		t.Error("the selection is not the dot")
	}
	if !strings.Contains(stdout, "gizmo mode=\"Move\" pivot=6.0000,0.0000,0.0000") {
		t.Error("no gizmo armed on the dot, or it armed somewhere other than on it")
	}

	// 4. Mid-drag the dot has actually moved, and so has its gizmo.
	moved := at(3)[front]
	if moved.y != 3 {
		t.Errorf("mid-drag the dot is at y=%v, want 3", moved.y)
	}
	if moved.screenY >= at(2)[front].screenY {
		t.Errorf("the dot did not move up the screen: %v then %v",
			at(2)[front].screenY, moved.screenY)
	}
	if !strings.Contains(stdout, "gizmo mode=\"Move\" pivot=6.0000,3.0000,0.0000") {
		t.Error("the gizmo did not follow the dot it is moving")
	}
	// The dot's authored direction is untouched: sliding a dot along a hull
	// must never silently repoint a thruster's exhaust.
	if moved.dirX != 1 || moved.dirY != 0 || moved.dz != 0 {
		t.Errorf("the drag changed the dot's direction to %v,%v,%v",
			moved.dirX, moved.dirY, moved.dz)
	}
	// And exactly one dot moved.
	if o := at(3)[thruster]; o.x != -6 || o.y != 0 || o.z != 0 {
		t.Errorf("the other dot moved too, to %v,%v,%v", o.x, o.y, o.z)
	}

	// 5. Releasing keeps it there and says so.
	if m := at(4)[front]; m.y != 3 {
		t.Errorf("after release the dot is at y=%v, want it to stay at 3", m.y)
	}
	if !strings.Contains(stdout, `toast "Move +0, +3, +0"`) {
		t.Error("releasing the drag reported nothing")
	}

	// 6. One undo puts it back — the drag coalesced into a single step.
	if m := at(5)[front]; m.y != 0 {
		t.Errorf("one undo left the dot at y=%v, want 0 — the drag was not coalesced", m.y)
	}
	if !strings.Contains(stdout, `toast "Undid: Move Front dot"`) {
		t.Error("undo did not name the dot it moved")
	}
	// The dot stays selected across the undo, so the gizmo is still there to
	// try again with.
	if m := at(5)[front]; !m.sel {
		t.Error("undo dropped the selection, taking the gizmo with it")
	}
}

// The dump's screen pixel has to be the truth, or every scripted click in this
// file is aimed by luck. Hovering exactly where the dump says the dot is must
// register as a hover on that dot.
func TestTheDumpedDotPixelIsWhereTheDotIs(t *testing.T) {
	stdout, _ := runScript(t, "marker_drag")
	dumps := markerDumps(t, stdout)
	if len(dumps) == 0 {
		t.Fatal("no marker dumps")
	}
	first := dumps[0][0]
	hovered := dumps[1][0]
	if first.screenX != hovered.screenX || first.screenY != hovered.screenY {
		t.Fatalf("the dot moved between dumps: %v,%v then %v,%v",
			first.screenX, first.screenY, hovered.screenX, hovered.screenY)
	}
	if !hovered.hover {
		t.Errorf("hovering the dumped pixel %v,%v did not hit the dot",
			first.screenX, first.screenY)
	}
	// Guard the script against silently drifting off the dot: the coordinates
	// it hovers must still be within a pixel of where the dot reports itself.
	want := fmt.Sprintf("[%.0f,%.0f]", first.screenX, first.screenY)
	if !strings.Contains(readScript(t, "marker_drag"), want) {
		t.Errorf("marker_drag.json no longer aims at %s — re-probe it", want)
	}
}

// readScript returns an op script's text, so a test can check that the pixels
// it aims at are still the pixels the program reports.
func readScript(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "scripts", name+".json"))
	if err != nil {
		t.Fatalf("read script %s: %v", name, err)
	}
	return string(b)
}

// The mid-drag shot: the dot selected and ringed in the accent, its gizmo on
// it, and the dot itself lifted clear of where it started. None of that is
// visible in a number, and all of it is what the feature looks like.
func TestGoldenMarkerDrag(t *testing.T) {
	_, outDir := runScript(t, "marker_drag")
	checkGolden(t, "marker_drag", outDir)
}
