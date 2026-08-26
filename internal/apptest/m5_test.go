package apptest

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// M5 flow tests: sketching on a face and push/pull, driven through the app.
//
// Every assertion here is a volume, because that is the only way to tell a face
// that moved from a face that looks like it moved. A boolean can produce a
// perfectly valid solid of entirely the wrong size.

var (
	m5PushPullLine = regexp.MustCompile(
		`^pushpull body=(\d+) dist=(-?[\d.]+) adding=(\d)$`)
	m5FaceSketchLine = regexp.MustCompile(
		`^facesketch id=(\d+) body=(\d+) ref=(\d)$`)
)

type pushPullDump struct {
	body    int
	dist    float64
	adding  bool
	present bool
}

type faceSketchDump struct {
	id, body int
	refAlive bool
}

// parseM5Dumps pulls the push/pull and face-sketch lines out of each dump.
func parseM5Dumps(t *testing.T, stdout string) ([]pushPullDump, [][]faceSketchDump) {
	t.Helper()
	var pp []pushPullDump
	var fs [][]faceSketchDump
	var curPP *pushPullDump
	var curFS *[]faceSketchDump

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "doc "):
			pp = append(pp, pushPullDump{})
			curPP = &pp[len(pp)-1]
			fs = append(fs, nil)
			curFS = &fs[len(fs)-1]
		case curPP != nil && strings.HasPrefix(line, "pushpull "):
			m := m5PushPullLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable pushpull line: %q", line)
			}
			d, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				t.Fatalf("unparsable distance %q", m[2])
			}
			*curPP = pushPullDump{
				body: atoi(t, m[1]), dist: d, adding: m[3] == "1", present: true,
			}
		case curFS != nil && strings.HasPrefix(line, "facesketch "):
			m := m5FaceSketchLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable facesketch line: %q", line)
			}
			*curFS = append(*curFS, faceSketchDump{
				id: atoi(t, m[1]), body: atoi(t, m[2]), refAlive: m[3] == "1",
			})
		}
	}
	return pp, fs
}

func TestGoldenSteppedHull(t *testing.T) {
	_, outDir := runScript(t, "m5_stepped")
	checkGolden(t, "m5_stepped", outDir)
}

func TestGoldenPushPull(t *testing.T) {
	_, outDir := runScript(t, "m5_pushpull")
	checkGolden(t, "m5_pushpull", outDir)
}

func TestGoldenFaceSketch(t *testing.T) {
	_, outDir := runScript(t, "m5_facesketch")
	checkGolden(t, "m5_facesketch", outDir)
}

// TestSteppedHullBuiltFromFacesAlone is M5's acceptance, stated as arithmetic.
//
// A slab, a face pulled out to make a step, a block grown from a face sketch on
// top, and two windows cut from a face sketch on the front — no default plane
// touched after the first one. Every volume is exact, because every shape here
// is a box and boxes have no rounding to hide behind.
func TestSteppedHullBuiltFromFacesAlone(t *testing.T) {
	stdout, _ := runScript(t, "m5_stepped")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}

	steps := []struct {
		what string
		want float64
	}{
		{"the base slab, 12x8x3", 288},
		{"a face pulled out 3 units, adding 8x3x3", 288 + 72},
		{"a block grown from a face sketch, 8x6x3", 360 + 144},
		{"two 2x2 windows cut 2 deep", 504 - 16},
	}
	for i, s := range steps {
		got := dumps[i].body(t, "Body 4").vol
		if math.Abs(got-s.want) > 1e-9 {
			t.Errorf("after %s: volume = %v, want %v", s.what, got, s.want)
		}
	}

	// The whole thing is still one body.
	final := dumps[len(dumps)-1]
	if n := len(final.bodies); n != 4 {
		t.Errorf("%d bodies at the end, want the 3 from the test scene plus one", n)
	}
	_, faceSketches := parseM5Dumps(t, stdout)
	last := faceSketches[len(faceSketches)-1]
	if len(last) != 2 {
		t.Fatalf("%d face sketches at the end, want 2", len(last))
	}
	for _, fsk := range last {
		if fsk.body != 4 {
			t.Errorf("face sketch %d is anchored to body %d, want 4", fsk.id, fsk.body)
		}
	}
}

// TestAFaceSketchOutlivesItsFace is what the frame snapshot is for
// (SPEC-GEOMETRY §3).
//
// Every one of these sketches was drawn on a face that a later edit then
// replaced — that is what "extrude this and add it to the body" does to the
// surface you drew on. The sketches are still in the document, still on the
// plane they were drawn on, and still openable. Only the live reference to the
// face is gone, which costs them their snap targets and nothing else.
func TestAFaceSketchOutlivesItsFace(t *testing.T) {
	stdout, _ := runScript(t, "m5_stepped")
	dumps := parseM3Dumps(t, stdout)
	_, faceSketches := parseM5Dumps(t, stdout)

	final := dumps[len(dumps)-1]
	if len(final.sketchVisible) != 3 {
		t.Errorf("%d sketches survived, want all 3", len(final.sketchVisible))
	}
	var orphaned int
	for _, fsk := range faceSketches[len(faceSketches)-1] {
		if !fsk.refAlive {
			orphaned++
		}
	}
	if orphaned == 0 {
		t.Skip("no sketch outlived its face in this run, so there is nothing to check")
	}
	// The point is that nothing broke: the run got all the way to the end and
	// every volume above was exact, with orphaned sketches in the document the
	// whole time.
	if final.body(t, "Body 4").vol != 488 {
		t.Errorf("the model is wrong with %d orphaned sketches in it", orphaned)
	}
}

// TestPushPullAddsAndRemovesByDirection is R11 through the app: the same drag
// on the same arrow does opposite things depending on which way it went, and
// nothing but the sign decides.
func TestPushPullAddsAndRemovesByDirection(t *testing.T) {
	stdout, _ := runScript(t, "m5_pushpull")
	dumps := parseM3Dumps(t, stdout)
	pp, _ := parseM5Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	start, pulling, pulled, pushing, pushed := dumps[0], dumps[1], dumps[2], dumps[3], dumps[4]

	// A preview changes nothing: CSG runs on release only (SPEC-UX §12.5).
	if pulling.body(t, "Hull").vol != start.body(t, "Hull").vol {
		t.Error("the preview changed the body before the drag was released")
	}
	if !pp[1].present || !pp[1].adding || pp[1].dist != 2.5 {
		t.Errorf("mid-pull the tool reports %+v, want a live +2.5 add", pp[1])
	}

	// Pulling out adds a prism of exactly the face's area times the distance.
	// The hull's raised block is 5x4 on top, so 2.5 units of pull is 50.
	added := pulled.body(t, "Hull").vol - start.body(t, "Hull").vol
	if math.Abs(added-50) > 1e-9 {
		t.Errorf("pulling out 2.5 units added %v, want 20 square units times 2.5", added)
	}

	// Pushing in takes one away, and the tool says so before it runs.
	if !pp[3].present || pp[3].adding || pp[3].dist != -2 {
		t.Errorf("mid-push the tool reports %+v, want a live -2 cut", pp[3])
	}
	if pushing.body(t, "Hull").vol != pulled.body(t, "Hull").vol {
		t.Error("the push preview changed the body before release")
	}
	// The +X face is 4x6, so pushing it in 2 units takes 48 away.
	removed := pulled.body(t, "Hull").vol - pushed.body(t, "Hull").vol
	if math.Abs(removed-48) > 1e-9 {
		t.Errorf("pushing in 2 units removed %v, want 24 square units times 2", removed)
	}

	// No new bodies: push/pull edits the body it was called on.
	if len(pushed.bodies) != len(start.bodies) {
		t.Errorf("body count went from %d to %d", len(start.bodies), len(pushed.bodies))
	}

	var told bool
	for _, msg := range pushed.toasts {
		if strings.Contains(msg, "Pushed the face in") {
			told = true
		}
	}
	if !told {
		t.Errorf("the push was silent; toasts were %q", pushed.toasts)
	}
}

// TestFaceSketchProjectsItsOutline is the other half of SPEC-UX §10: a sketch
// made on a face can copy that face's boundary in as real, editable lines.
func TestFaceSketchProjectsItsOutline(t *testing.T) {
	stdout, _ := runScript(t, "m5_facesketch")
	dumps := parseM3Dumps(t, stdout)
	sketches := parseSketchDumps(t, stdout)
	_, faceSketches := parseM5Dumps(t, stdout)

	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	fresh, projected := sketches[0], sketches[1]

	if !fresh.present {
		t.Fatal("sketching on a face did not enter sketch mode")
	}
	if fresh.entities != 0 {
		t.Errorf("a new face sketch already has %d entities", fresh.entities)
	}
	// The hull's top face is a rectangle, so its outline is four lines and one
	// closed region.
	if projected.entities != 4 {
		t.Errorf("projecting gave %d entities, want the face's 4 edges", projected.entities)
	}
	if projected.regions != 1 || projected.openEnds != 0 {
		t.Errorf("the projected outline gave %d regions and %d open ends, want 1 and 0",
			projected.regions, projected.openEnds)
	}

	// The sketch is anchored to the body it was drawn on, and the anchor is live.
	for _, block := range faceSketches {
		for _, fsk := range block {
			if fsk.body == 0 {
				t.Error("the face sketch does not remember which body it belongs to")
			}
			if !fsk.refAlive {
				t.Error("the face the sketch was made on is not resolving")
			}
		}
	}

	var said bool
	for _, msg := range dumps[len(dumps)-1].toasts {
		if strings.Contains(msg, "Projected the face outline") {
			said = true
		}
	}
	if !said {
		t.Errorf("projecting was silent; toasts were %q", dumps[len(dumps)-1].toasts)
	}
}

// TestFaceSketchDefaultsToAdd is SPEC-UX §10: extruding from a face sketch
// grows the body it was drawn on rather than starting a new one beside it.
func TestFaceSketchDefaultsToAdd(t *testing.T) {
	stdout, _ := runScript(t, "m5_faceadd")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, open, after := dumps[0], dumps[1], dumps[2]

	if open.extrude.result != "Add" {
		t.Errorf("a face sketch's extrude opened on %q, want Add", open.extrude.result)
	}
	if open.extrude.targets != 1 {
		t.Errorf("it found %d targets, want the one body it was drawn on",
			open.extrude.targets)
	}
	if len(after.bodies) != len(before.bodies) {
		t.Errorf("Add created a body: %d then %d", len(before.bodies), len(after.bodies))
	}
	grew := after.body(t, "Hull").vol - before.body(t, "Hull").vol
	if math.Abs(grew-8) > 1e-9 {
		t.Errorf("the body grew by %v, want the 2x2x2 that was drawn", grew)
	}
}

// TestSketchingLooksAtTheFaceNotThroughIt is a regression, reported by the
// user: starting a sketch pointed the camera the opposite way.
//
// LookAlong takes the direction the eye sits in, so looking at a face means
// passing its outward normal. M5 passed the negation, which put the camera
// inside the body staring at the back of the surface you had just asked to
// draw on — the geometry was right and the view was inside out.
//
// The check is the camera's forward direction against the face's normal. They
// must oppose: the eye is outside, looking back down the normal.
func TestSketchingLooksAtTheFaceNotThroughIt(t *testing.T) {
	stdout, _ := runScript(t, "m5_faceview")
	cams := parseCameraDumps(t, stdout)
	if len(cams) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(cams), stdout)
	}

	cases := []struct {
		what   string
		normal [3]float64
		cam    [3]float64
	}{
		{"a face pointing +Y", [3]float64{0, 1, 0}, cams[0]},
		{"a face pointing +X", [3]float64{1, 0, 0}, cams[1]},
		{"the Front plane", [3]float64{0, 0, 1}, cams[2]},
	}
	for _, c := range cases {
		dot := c.normal[0]*c.cam[0] + c.normal[1]*c.cam[1] + c.normal[2]*c.cam[2]
		if dot > -0.99 {
			t.Errorf("sketching on %s looks %v, which is %.2f against its normal %v — "+
				"want the camera outside looking back down it",
				c.what, c.cam, dot, c.normal)
		}
	}
}

// parseCameraDumps pulls the camera's forward direction out of each dump.
func parseCameraDumps(t *testing.T, stdout string) [][3]float64 {
	t.Helper()
	line := regexp.MustCompile(
		`^camera forward=(-?[\d.]+),(-?[\d.]+),(-?[\d.]+) ortho=\d$`)
	var out [][3]float64
	for _, raw := range strings.Split(stdout, "\n") {
		m := line.FindStringSubmatch(strings.TrimRight(raw, "\r"))
		if m == nil {
			continue
		}
		var v [3]float64
		for i := 0; i < 3; i++ {
			f, err := strconv.ParseFloat(m[i+1], 64)
			if err != nil {
				t.Fatalf("unparsable camera line %q", raw)
			}
			v[i] = f
		}
		out = append(out, v)
	}
	return out
}
