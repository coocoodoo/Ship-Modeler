package apptest

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// M6 flow tests: selection, direct edits and transforms through the app.
//
// The measurements are volumes and vertex counts, because those are what a
// wrong answer shows up in. A move that grabs the wrong vertices still looks
// like a move; a hull that grew by the wrong amount does not.

var (
	m6SelLine = regexp.MustCompile(`^sel count=(\d+) desc="([^"]*)"$`)
	m6Gizmo   = regexp.MustCompile(
		`^gizmo mode="(\w+)" pivot=(-?[\d.]+),(-?[\d.]+),(-?[\d.]+)$`)
	m6BoxFilter = regexp.MustCompile(`^boxselect filter="(\w+)"$`)
)

type selDump struct {
	count int
	desc  string
}

type gizmoDump struct {
	mode    string
	pivot   [3]float64
	filter  string
	present bool
}

func parseM6Dumps(t *testing.T, stdout string) ([]selDump, []gizmoDump) {
	t.Helper()
	var sels []selDump
	var gizmos []gizmoDump
	var curSel *selDump
	var curGiz *gizmoDump
	num := func(s string) float64 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("unparsable number %q", s)
		}
		return v
	}

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "doc "):
			sels = append(sels, selDump{})
			curSel = &sels[len(sels)-1]
			gizmos = append(gizmos, gizmoDump{})
			curGiz = &gizmos[len(gizmos)-1]
		case curSel != nil && strings.HasPrefix(line, "sel "):
			if m := m6SelLine.FindStringSubmatch(line); m != nil {
				curSel.count = atoi(t, m[1])
				curSel.desc = m[2]
			}
		case curGiz != nil && strings.HasPrefix(line, "gizmo "):
			m := m6Gizmo.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable gizmo line: %q", line)
			}
			curGiz.mode = m[1]
			curGiz.pivot = [3]float64{num(m[2]), num(m[3]), num(m[4])}
			curGiz.present = true
		case curGiz != nil && strings.HasPrefix(line, "boxselect "):
			if m := m6BoxFilter.FindStringSubmatch(line); m != nil {
				curGiz.filter = m[1]
			}
		}
	}
	return sels, gizmos
}

func TestGoldenBoxSelect(t *testing.T) {
	_, outDir := runScript(t, "m6_stretch")
	checkGolden(t, "m6_stretch", outDir)
}

// TestStretchAHullAndRotateAWing is M6's acceptance, as arithmetic.
//
// Drag a box round the nose of a hull, drag the four vertices it caught four
// units out, and the hull grows by exactly its end face times four. Then turn a
// wing a quarter turn and its volume does not move at all, because a rotation
// is rigid and a rotation that is not is a bug you find six milestones later.
func TestStretchAHullAndRotateAWing(t *testing.T) {
	stdout, _ := runScript(t, "m6_stretch")
	dumps := parseM3Dumps(t, stdout)
	sels, gizmos := parseM6Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	start, boxed, moved, turned := dumps[0], dumps[1], dumps[2], dumps[3]

	// Box select: the four corners of the hull's end, and nothing else. Two of
	// them are directly behind the other two from this angle, which is exactly
	// the case an ID-buffer scan cannot answer.
	if sels[1].count != 4 {
		t.Errorf("the box caught %d elements (%q), want the 4 nose vertices",
			sels[1].count, sels[1].desc)
	}
	if !gizmos[1].present {
		t.Fatal("no gizmo appeared on the box selection")
	}
	if math.Abs(gizmos[1].pivot[0]-6) > 1e-9 {
		t.Errorf("the gizmo sits at x=%v, want the end face at 6", gizmos[1].pivot[0])
	}
	if gizmos[1].filter != "Verts" {
		t.Errorf("the box filter reads %q, want the Verts default", gizmos[1].filter)
	}

	// The stretch: the hull's end is 4 by 6, so four units of pull is 96.
	hullBefore := start.body(t, "Hull").vol
	hullAfter := moved.body(t, "Hull").vol
	if math.Abs((hullAfter-hullBefore)-96) > 1e-9 {
		t.Errorf("stretching added %v, want 24 square units times 4",
			hullAfter-hullBefore)
	}
	if boxed.body(t, "Hull").vol != hullBefore {
		t.Error("selecting changed the hull")
	}
	// Moving a whole end face straight along an axis bends nothing.
	for _, msg := range moved.toasts {
		if strings.Contains(msg, "bent") {
			t.Errorf("an axial stretch reported bent faces: %q", msg)
		}
	}

	// The rotation is rigid.
	wingBefore := moved.body(t, "Wing pod").vol
	wingAfter := turned.body(t, "Wing pod").vol
	if wingAfter != wingBefore {
		t.Errorf("rotating the wing changed its volume from %v to %v",
			wingBefore, wingAfter)
	}
	if gizmos[3].mode != "Rotate" {
		t.Errorf("the gizmo is in %q mode after a rotation, want Rotate", gizmos[3].mode)
	}

	var told int
	for _, msg := range turned.toasts {
		if strings.Contains(msg, "Move ") || strings.Contains(msg, "Rotate ") {
			told++
		}
	}
	if told < 2 {
		t.Errorf("the edits were not both announced; toasts were %q", turned.toasts)
	}
}

// TestEachEditIsOneUndoStep is the coalescing rule of SPEC-DATA §2: a drag,
// however many frames it took, is one entry in the history.
func TestEachEditIsOneUndoStep(t *testing.T) {
	stdout, _ := runScript(t, "m6_undo")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 4 {
		t.Fatalf("expected 4 dumps, got %d:\n%s", len(dumps), stdout)
	}
	start, moved, undone, redone := dumps[0], dumps[1], dumps[2], dumps[3]

	if moved.undo != start.undo+1 {
		t.Errorf("a move pushed %d history entries, want 1", moved.undo-start.undo)
	}
	if math.Abs(moved.body(t, "Hull").vol-start.body(t, "Hull").vol) < 1e-9 {
		t.Fatal("the move changed nothing, so there is nothing to undo")
	}
	if undone.body(t, "Hull").vol != start.body(t, "Hull").vol {
		t.Errorf("undo left the hull at %v, want its original %v",
			undone.body(t, "Hull").vol, start.body(t, "Hull").vol)
	}
	if undone.redo != 1 {
		t.Errorf("redo depth is %d after one undo, want 1", undone.redo)
	}
	if redone.body(t, "Hull").vol != moved.body(t, "Hull").vol {
		t.Error("redo produced a different shape")
	}
}

// TestBoxFilterPicksWhatItSays covers the chips of SPEC-UX §12.1: the same
// rectangle collects vertices, edges or faces depending on the filter.
func TestBoxFilterPicksWhatItSays(t *testing.T) {
	stdout, _ := runScript(t, "m6_filters")
	sels, gizmos := parseM6Dumps(t, stdout)
	if len(sels) != 4 {
		t.Fatalf("expected 4 dumps, got %d:\n%s", len(sels), stdout)
	}

	want := []struct {
		filter string
		noun   string
	}{
		{"Verts", "vertices"},
		{"Edges", "edges"},
		{"Faces", "faces"},
	}
	for i, w := range want {
		got := sels[i+1]
		if got.count == 0 {
			t.Errorf("the %s filter caught nothing", w.filter)
			continue
		}
		if !strings.Contains(got.desc, w.noun) {
			t.Errorf("the %s filter selected %q, want %s", w.filter, got.desc, w.noun)
		}
		if gizmos[i+1].filter != w.filter {
			t.Errorf("the card reports filter %q, want %q", gizmos[i+1].filter, w.filter)
		}
	}
}

// TestDuplicateOffsetsACopy is Ctrl+D (SPEC-UX §12.4).
func TestDuplicateOffsetsACopy(t *testing.T) {
	stdout, _ := runScript(t, "m6_duplicate")
	dumps := parseM3Dumps(t, stdout)
	before, after := dumps[0], dumps[1]

	if len(after.bodies) != len(before.bodies)+1 {
		t.Fatalf("%d bodies then %d, want one more", len(before.bodies), len(after.bodies))
	}
	orig := before.body(t, "Hull")
	var copyVol float64
	for _, b := range after.bodies {
		if b.name != "Hull" && b.vol == orig.vol {
			copyVol = b.vol
		}
	}
	if copyVol != orig.vol {
		t.Errorf("no copy with the original's volume %v", orig.vol)
	}
	var said bool
	for _, msg := range after.toasts {
		if strings.Contains(msg, "Duplicated as") {
			said = true
		}
	}
	if !said {
		t.Errorf("the duplicate was silent; toasts were %q", after.toasts)
	}
}

// TestClickingASketchSelectsIt is a regression, reported by the user twice.
//
// A finished sketch is drawn as an overlay, not as geometry, so it was never in
// the ID buffer the pick pass reads. Clicking one selected whatever was behind
// it. The first fix only let the sketch win over a plane, which missed the case
// that was actually being hit: the hull sits on the Top plane, so a sketch drawn
// there has a *body* behind it, not a plane.
//
// The rule now matches the rendering. Sketches draw with the depth test off
// (V-12), so one is always on top of whatever it overlaps — that is what makes
// sketching on a plane that runs through a hull possible. A click has to agree
// with what you can see there.
//
// This test therefore leaves every body visible, which is what the user had.
func TestClickingASketchSelectsIt(t *testing.T) {
	stdout, _ := runScript(t, "m6_sketchclick")
	dumps := parseM3Dumps(t, stdout)
	sels, _ := parseM6Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	hovered, clicked, elsewhere, open, done :=
		dumps[0], sels[1], sels[2], dumps[3], dumps[4]

	// The hull is right behind the sketch, so this is the case that was broken.
	if !hovered.hasBody("Hull") {
		t.Fatal("the hull is not in the scene, so nothing is behind the sketch")
	}
	if !strings.Contains(hovered.hint, "Sketch 1") {
		t.Errorf("hovering a sketch says %q, want it to name the sketch", hovered.hint)
	}
	if strings.Contains(hovered.hint, "Hull") {
		t.Errorf("hovering a sketch says %q, which is the body behind it", hovered.hint)
	}
	if sels[0].count != 0 {
		t.Errorf("hovering selected something: %q", sels[0].desc)
	}

	if clicked.desc != "Sketch 1" {
		t.Errorf("clicking a sketch selected %q, want Sketch 1", clicked.desc)
	}

	// And a body away from the sketch is still perfectly clickable: the sketch
	// wins where it is, not everywhere.
	if !strings.Contains(elsewhere.desc, "face of") {
		t.Errorf("clicking a body away from the sketch selected %q, want a face",
			elsewhere.desc)
	}

	// From there E extrudes it, with its region already picked.
	if !open.extrude.present {
		t.Fatal("the extrude tool did not open on the selected sketch")
	}
	if open.extrude.regions != 1 {
		t.Errorf("the extrude took %d regions, want the sketch's 1", open.extrude.regions)
	}
	if !done.hasBody("Body 4") {
		t.Error("the extrude produced no body")
	} else if done.body(t, "Body 4").vol != 108 {
		t.Errorf("volume = %v, want 36 square units pulled 3", done.body(t, "Body 4").vol)
	}
}

// TestDraggingTheFaceArrowActuallyPushPulls is a regression, reported by the
// user: selecting a face and dragging its arrow did nothing, and the planes
// stayed put.
//
// Selecting a face armed two tools at the same point. The push/pull arrow was
// the one drawn, but the move gizmo was armed too, and its screen-plane handle
// is a disc centred exactly where the arrow starts. Being armed, it was
// hit-tested; being hit-tested first, it swallowed the press. So the arrow
// never got the input, its distance stayed at zero, and the drag silently
// became a vertex move that deformed the face instead of push/pulling it. The
// planes stayed because they hide on a drag that never began.
//
// Note what this needed to catch it: a real press, a run of motion, and a
// release. There was no way to script one until this bug demanded it, which is
// exactly why every drag in the program had gone untested.
func TestDraggingTheFaceArrowActuallyPushPulls(t *testing.T) {
	stdout, _ := runScript(t, "m6_arrowdrag")
	dumps := parseM3Dumps(t, stdout)
	pp, _ := parseM5Dumps(t, stdout)
	planes := parsePlaneCounts(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	armed, mid, done := dumps[0], dumps[1], dumps[2]

	// Armed but not yet dragged: nothing has happened and the planes are there.
	if pp[0].dist != 0 {
		t.Errorf("the arrow starts at %v, want 0", pp[0].dist)
	}
	if planes[0] != 3 {
		t.Errorf("%d planes before the drag, want 3", planes[0])
	}

	// Mid-drag: the arrow has moved with the pointer, and the planes are gone.
	if pp[1].dist != 3 {
		t.Errorf("mid-drag the arrow reads %v, want the 3 units it was dragged — "+
			"a zero here means something else took the press", pp[1].dist)
	}
	if !pp[1].adding {
		t.Error("dragging away from the face reads as a cut, not a pull")
	}
	if planes[1] != 0 {
		t.Errorf("%d planes drawn mid-drag, want none", planes[1])
	}
	// CSG runs on release only, so the body has not changed yet.
	if mid.body(t, "Hull").vol != armed.body(t, "Hull").vol {
		t.Error("the body changed before the drag was released")
	}

	// Released: the boolean ran, and the planes came back.
	grew := done.body(t, "Hull").vol - armed.body(t, "Hull").vol
	if math.Abs(grew-60) > 1e-9 {
		t.Errorf("the pull added %v, want the 5x4 face times 3", grew)
	}
	if planes[2] != 3 {
		t.Errorf("%d planes after the drag, want them back", planes[2])
	}
	var told bool
	for _, msg := range done.toasts {
		if strings.Contains(msg, "Pulled the face out") {
			told = true
		}
		if strings.Contains(msg, "Move ") {
			t.Errorf("the drag was handled as a move, not a push/pull: %q", msg)
		}
	}
	if !told {
		t.Errorf("the push/pull was silent; toasts were %q", done.toasts)
	}
}

// parsePlaneCounts pulls how many default planes each dump was drawing.
func parsePlaneCounts(t *testing.T, stdout string) []int {
	t.Helper()
	line := regexp.MustCompile(`^planes drawn=(\d+)$`)
	var out []int
	for _, raw := range strings.Split(stdout, "\n") {
		if m := line.FindStringSubmatch(strings.TrimRight(raw, "\r")); m != nil {
			out = append(out, atoi(t, m[1]))
		}
	}
	return out
}
