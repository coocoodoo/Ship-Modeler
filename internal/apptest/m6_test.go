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
		`^gizmo mode="(\w+)" pivot=(-?[\d.]+),(-?[\d.]+),(-?[\d.]+) boxfilter="(\w+)"$`)
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
			*curGiz = gizmoDump{
				mode:   m[1],
				pivot:  [3]float64{num(m[2]), num(m[3]), num(m[4])},
				filter: m[5], present: true,
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

// TestClickingASketchSelectsIt is a regression, reported by the user: a closed
// sketch could not be selected to extrude it.
//
// A finished sketch is drawn as an overlay, not as geometry, so it was never in
// the ID buffer the pick pass reads. Clicking one selected whatever was behind
// it — which, on the plane it was drawn on, is that plane. The sketch was
// perfectly selectable from the tree, so nothing failed loudly; it just looked
// as though closed profiles could not be picked.
func TestClickingASketchSelectsIt(t *testing.T) {
	stdout, _ := runScript(t, "m6_sketchclick")
	dumps := parseM3Dumps(t, stdout)
	sels, _ := parseM6Dumps(t, stdout)
	if len(dumps) != 4 {
		t.Fatalf("expected 4 dumps, got %d:\n%s", len(dumps), stdout)
	}
	hovered, clicked, open, done := dumps[0], dumps[1], dumps[2], dumps[3]

	// Hovering names the sketch, not the plane under it, so the hint bar
	// describes what a click will actually do.
	if !strings.Contains(hovered.hint, "Sketch 1") {
		t.Errorf("hovering a sketch says %q, want it to name the sketch", hovered.hint)
	}
	if strings.Contains(hovered.hint, "plane") {
		t.Errorf("hovering a sketch says %q, which is the plane behind it", hovered.hint)
	}
	if sels[0].count != 0 {
		t.Errorf("hovering selected something: %q", sels[0].desc)
	}

	// Clicking selects the sketch itself.
	if sels[1].desc != "Sketch 1" {
		t.Errorf("clicking a sketch selected %q, want Sketch 1", sels[1].desc)
	}
	_ = clicked

	// And from there E extrudes it, with its region already picked.
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
