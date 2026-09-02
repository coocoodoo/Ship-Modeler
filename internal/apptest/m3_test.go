package apptest

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// M3 flow tests. The scripts drive the real extrude tool and the real command,
// so what they measure is what a drag would produce. The volumes are checked
// against the closed-form frustum formula rather than against a recorded
// number, which is the only way a solid-modelling test can catch a builder that
// is wrong in a way the mesh validator would still accept.

// bodyDump is one body line out of a dump block.
type bodyDump struct {
	id      int
	name    string
	visible bool
	tris    int
	vol     float64
}

// extrudeDump is the open tool's line, absent when no extrude is in progress.
type extrudeDump struct {
	depth, draft, achieved float64
	clamped                bool
	dir                    string
	through                bool
	regions                int
	result                 string
	targets                int
	present                bool
}

// m3Dump is a whole dump block, as much of it as M3 cares about.
type m3Dump struct {
	bodies        []bodyDump
	sketchVisible map[string]bool
	extrude       extrudeDump
	toasts        []string
	hint          string
	undo, redo    int
}

var (
	m3BodyLine = regexp.MustCompile(
		`^body id=(\d+) name="([^"]*)" visible=(\d) tris=(\d+) vol=(-?[\d.]+) `)
	m3SketchLine  = regexp.MustCompile(`^sketch id=\d+ name="([^"]*)" visible=(\d)$`)
	m3ExtrudeLine = regexp.MustCompile(
		`^extrude depth=(-?[\d.]+) draft=(-?[\d.]+) achieved=(-?[\d.]+) ` +
			`clamped=(\d) dir="(\w+)" through=(\d) regions=(\d+) ` +
			`result="(\w+)" targets=(\d+) reach=(\d+)`)
	m3DocLine   = regexp.MustCompile(`undo=(\d+) redo=(\d+)`)
	m3ToastLine = regexp.MustCompile(`^toast "(.*)"$`)
)

func parseM3Dumps(t *testing.T, stdout string) []m3Dump {
	t.Helper()
	var out []m3Dump
	var cur *m3Dump
	num := func(s string) float64 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("unparsable number %q: %v", s, err)
		}
		return v
	}

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "doc ") {
			out = append(out, m3Dump{sketchVisible: map[string]bool{}})
			cur = &out[len(out)-1]
			if m := m3DocLine.FindStringSubmatch(line); m != nil {
				cur.undo, cur.redo = atoi(t, m[1]), atoi(t, m[2])
			}
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "body "):
			m := m3BodyLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable body line: %q", line)
			}
			cur.bodies = append(cur.bodies, bodyDump{
				id: atoi(t, m[1]), name: m[2], visible: m[3] == "1",
				tris: atoi(t, m[4]), vol: num(m[5]),
			})
		case strings.HasPrefix(line, "sketch id="):
			m := m3SketchLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable sketch line: %q", line)
			}
			cur.sketchVisible[m[1]] = m[2] == "1"
		case strings.HasPrefix(line, "extrude "):
			m := m3ExtrudeLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable extrude line: %q", line)
			}
			cur.extrude = extrudeDump{
				depth: num(m[1]), draft: num(m[2]), achieved: num(m[3]),
				clamped: m[4] == "1", dir: m[5], through: m[6] == "1",
				regions: atoi(t, m[7]), result: m[8], targets: atoi(t, m[9]),
				present: true,
			}
		case strings.HasPrefix(line, "hint "):
			if m := regexp.MustCompile(`^hint "(.*)"$`).FindStringSubmatch(line); m != nil {
				cur.hint = m[1]
			}
		case strings.HasPrefix(line, "toast "):
			if m := m3ToastLine.FindStringSubmatch(line); m != nil {
				cur.toasts = append(cur.toasts, m[1])
			}
		}
	}
	return out
}

// body finds a body by name, failing the test if it is not there.
func (d m3Dump) body(t *testing.T, name string) bodyDump {
	t.Helper()
	for _, b := range d.bodies {
		if b.name == name {
			return b
		}
	}
	t.Fatalf("no body named %q in %v", name, d.bodies)
	return bodyDump{}
}

func (d m3Dump) hasBody(name string) bool {
	for _, b := range d.bodies {
		if b.name == name {
			return true
		}
	}
	return false
}

// frustumVolume is the closed form for a solid whose cross-section shrinks
// linearly: V = h/3 (A1 + A2 + sqrt(A1*A2)). A straight extrude is the case
// A1 == A2, so one formula covers every test below.
func frustumVolume(h, a1, a2 float64) float64 {
	return h / 3 * (a1 + a2 + math.Sqrt(a1*a2))
}

// draftedSide is the side of a square profile after a run of h at the given
// draft angle, with the taper snapped to the subunit lattice the way every
// authored length is (SPEC-GEOMETRY §1.2). The snap is why this is not simply
// side - 2*h*tan(draft): the builder cannot store an off-lattice offset, so
// neither can the number this test compares against.
func draftedSide(side, h, draftDeg float64) float64 {
	const subunits = 256.0
	delta := math.Round(h*math.Tan(draftDeg*math.Pi/180)*subunits) / subunits
	return side - 2*delta
}

func closeTo(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.6f, want %.6f (+/-%g)", what, got, want, tol)
	}
}

func TestGoldenExtrudeStraight(t *testing.T) {
	_, outDir := runScript(t, "m3_straight")
	checkGolden(t, "m3_straight", outDir)
}

func TestGoldenExtrudeDrafted(t *testing.T) {
	_, outDir := runScript(t, "m3_draft")
	checkGolden(t, "m3_draft", outDir)
}

func TestGoldenExtrudeSymmetric(t *testing.T) {
	_, outDir := runScript(t, "m3_symmetric")
	checkGolden(t, "m3_symmetric", outDir)
}

func TestGoldenExtrudeGizmo(t *testing.T) {
	_, outDir := runScript(t, "m3_gizmo")
	checkGolden(t, "m3_gizmo", outDir)
}

// TestStraightExtrudeIsExact is the simplest case, and the one that must be
// exact rather than approximate: a 6x6 square pulled 6 units is a cube, and 216
// is not a number a tolerance should be needed to reach.
func TestStraightExtrudeIsExact(t *testing.T) {
	stdout, _ := runScript(t, "m3_straight")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, after := dumps[0], dumps[1]

	if before.hasBody("Body 4") {
		t.Error("a body existed before the extrude ran")
	}
	b := after.body(t, "Body 4")
	if b.vol != 216 {
		t.Errorf("cube volume = %v, want exactly 216", b.vol)
	}
	if b.tris != 12 {
		t.Errorf("cube has %d triangles, want 12", b.tris)
	}
	if !b.visible {
		t.Error("the new body is not visible")
	}
}

// TestSquareExtrudesToADraftedCrate is M3's acceptance, checked as geometry:
// the solid is the frustum the draft angle asks for, not merely something the
// validator was willing to accept.
func TestSquareExtrudesToADraftedCrate(t *testing.T) {
	stdout, _ := runScript(t, "m3_draft")
	dumps := parseM3Dumps(t, stdout)
	b := dumps[len(dumps)-1].body(t, "Body 4")

	const side, depth, draft = 6.0, 6.0, 12.0
	top := draftedSide(side, depth, draft)
	want := frustumVolume(depth, side*side, top*top)
	closeTo(t, "drafted crate volume", b.vol, want, 1e-3)

	if b.vol >= side*side*depth {
		t.Errorf("volume %v is not less than the undrafted %v", b.vol, side*side*depth)
	}
	if b.tris != 12 {
		t.Errorf("a drafted crate has %d triangles, want 12", b.tris)
	}
}

// TestSymmetricStraddlesThePlane covers the symmetric-with-draft reading: two
// frusta meeting at the sketch plane, widest there and tapering to both ends,
// so the solid carries a mid-belt of extra vertices and twice the side faces.
func TestSymmetricStraddlesThePlane(t *testing.T) {
	stdout, _ := runScript(t, "m3_symmetric")
	dumps := parseM3Dumps(t, stdout)
	b := dumps[len(dumps)-1].body(t, "Body 4")

	const side, depth, draft = 6.0, 6.0, 10.0
	end := draftedSide(side, depth/2, draft)
	want := 2 * frustumVolume(depth/2, side*side, end*end)
	closeTo(t, "symmetric volume", b.vol, want, 1e-3)

	// Four side quads per half plus two caps: (4*2)*2 + 2*2.
	if b.tris != 20 {
		t.Errorf("symmetric solid has %d triangles, want 20 for a two-ring taper", b.tris)
	}
}

// TestExtrudeAutoHidesItsSketchAndUndoesInOneStep is SPEC-UX §9.5: committing
// is a single undoable step that takes the sketch's visibility with it, so one
// Ctrl+Z puts the document back exactly as it was.
func TestExtrudeAutoHidesItsSketchAndUndoesInOneStep(t *testing.T) {
	stdout, _ := runScript(t, "m3_undo")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	done, undone, redone := dumps[0], dumps[1], dumps[2]

	if !done.hasBody("Body 4") {
		t.Fatal("the extrude produced no body")
	}
	if done.sketchVisible["Sketch 1"] {
		t.Error("the consumed sketch is still visible")
	}

	if undone.hasBody("Body 4") {
		t.Error("undo left the extruded body behind")
	}
	if !undone.sketchVisible["Sketch 1"] {
		t.Error("undo did not bring the sketch back")
	}
	if undone.undo != done.undo-1 || undone.redo != 1 {
		t.Errorf("after undo: undo=%d redo=%d, want %d and 1",
			undone.undo, undone.redo, done.undo-1)
	}

	if !redone.hasBody("Body 4") {
		t.Error("redo did not rebuild the body")
	}
	if redone.body(t, "Body 4").vol != done.body(t, "Body 4").vol {
		t.Error("redo rebuilt a different solid")
	}
	if redone.sketchVisible["Sketch 1"] {
		t.Error("redo did not re-hide the sketch")
	}
}

// TestCancellingAnExtrudeLeavesNoTrace: Esc must be free. Nothing is added,
// nothing is hidden, and the undo stack does not grow.
func TestCancellingAnExtrudeLeavesNoTrace(t *testing.T) {
	stdout, _ := runScript(t, "m3_gizmo")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	open, cancelled := dumps[0], dumps[1]

	if !open.extrude.present {
		t.Fatal("the extrude tool was not open at the first dump")
	}
	if cancelled.extrude.present {
		t.Error("the tool is still open after a cancel")
	}
	if len(cancelled.bodies) != len(open.bodies) {
		t.Errorf("cancelling changed the body count from %d to %d",
			len(open.bodies), len(cancelled.bodies))
	}
	if cancelled.undo != open.undo {
		t.Errorf("cancelling pushed %d step(s) onto the undo stack",
			cancelled.undo-open.undo)
	}
	if !cancelled.sketchVisible["Sketch 1"] {
		t.Error("a cancelled extrude hid the sketch anyway")
	}
}

// TestOpenToolReportsItsOptions keeps the options card honest: what it shows is
// read straight off the tool, so the dump line is the card's contents.
func TestOpenToolReportsItsOptions(t *testing.T) {
	stdout, _ := runScript(t, "m3_gizmo")
	e := parseM3Dumps(t, stdout)[0].extrude

	if e.depth != 5 || e.draft != 10 {
		t.Errorf("tool reports depth=%v draft=%v, want 5 and 10", e.depth, e.draft)
	}
	if e.achieved != e.draft || e.clamped {
		t.Errorf("a 10 degree draft on a 6-unit square was clamped to %v", e.achieved)
	}
	if e.dir != "Normal" || e.regions != 1 {
		t.Errorf("tool reports dir=%q regions=%d, want Normal and 1", e.dir, e.regions)
	}
}

func TestGoldenExtrudeRefusesTouchingRegions(t *testing.T) {
	_, outDir := runScript(t, "m3_multiregion")
	checkGolden(t, "m3_multiregion", outDir)
}

// TestShiftAddsRegionsToTheSelection drives the region picking of SPEC-UX §9.1
// through real clicks: one click takes a region, Shift-click adds the next.
//
// The pair it builds is also the one an extrude cannot honour — a disc filling
// the ring around it, whose shells would meet wall to wall — so the same script
// checks that the refusal is a sentence about regions rather than the
// validator's opinion of a vertex.
func TestShiftAddsRegionsToTheSelection(t *testing.T) {
	stdout, _ := runScript(t, "m3_multiregion")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	open := dumps[2]

	if !open.extrude.present {
		t.Fatal("the extrude tool did not open")
	}
	if open.extrude.regions != 2 {
		t.Errorf("regions = %d, want 2 after a click and a Shift-click",
			open.extrude.regions)
	}

	hint := lastHint(t, stdout)
	if !strings.Contains(hint, "touch") || !strings.Contains(hint, "one at a time") {
		t.Errorf("the refusal %q does not say what happened or what to do", hint)
	}
	for _, jargon := range []string{"vertex", "invalid mesh", "manifold", "Euler"} {
		if strings.Contains(hint, jargon) {
			t.Errorf("the refusal %q leaks mesh internals (%q)", hint, jargon)
		}
	}
	// Nothing was built and nothing was hidden: a refusal costs the user nothing.
	if open.hasBody("Body 4") {
		t.Error("a body was created despite the refusal")
	}
	if !open.sketchVisible["Sketch 1"] {
		t.Error("the sketch was hidden despite the refusal")
	}
}

// lastHint is the hint bar's text at the final dump.
func lastHint(t *testing.T, stdout string) string {
	t.Helper()
	line := regexp.MustCompile(`^hint "(.*)"$`)
	var last string
	for _, raw := range strings.Split(stdout, "\n") {
		if m := line.FindStringSubmatch(strings.TrimRight(raw, "\r")); m != nil {
			last = m[1]
		}
	}
	if last == "" {
		t.Fatalf("no hint line in:\n%s", stdout)
	}
	return last
}

// TestExtrudeStartsFromASketchPickedInTheTree is the second entry point of
// SPEC-UX §9.1: from Idle, a sketch selected in the tree is enough — it is
// re-entered and every one of its regions comes preselected.
func TestExtrudeStartsFromASketchPickedInTheTree(t *testing.T) {
	stdout, _ := runScript(t, "m3_fromtree")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(dumps), stdout)
	}
	idle, open, done := dumps[0], dumps[1], dumps[2]

	if idle.extrude.present {
		t.Fatal("the tool was already open before the extrude began")
	}
	if !open.extrude.present {
		t.Fatal("selecting the sketch in the tree did not start an extrude")
	}
	if open.extrude.regions != 2 {
		t.Errorf("regions = %d, want both of the sketch's regions preselected",
			open.extrude.regions)
	}

	// Two disjoint squares, 4x4 and 4x2, pulled 3 units: two shells, and a
	// volume that needs no tolerance at all.
	b := done.body(t, "Body 4")
	if b.vol != 72 {
		t.Errorf("volume = %v, want exactly (16+8)*3", b.vol)
	}
	if b.tris != 24 {
		t.Errorf("triangles = %d, want 24 for two boxes", b.tris)
	}
}

func TestGoldenExtrudeThroughAll(t *testing.T) {
	_, outDir := runScript(t, "m3_through")
	checkGolden(t, "m3_through", outDir)
}

// TestThroughAllClearsTheWholeScene is the toggle of SPEC-UX §9.3 measured
// rather than eyeballed: the solid must be exactly as long as the tool says,
// and long enough to have come out the far side of everything already there.
func TestThroughAllClearsTheWholeScene(t *testing.T) {
	stdout, _ := runScript(t, "m3_through")
	dumps := parseM3Dumps(t, stdout)
	open, done := dumps[0], dumps[1]

	if !open.extrude.through {
		t.Fatal("the tool does not report through-all as on")
	}
	depth := open.extrude.depth
	if depth <= 1 {
		t.Fatalf("through-all depth = %v, which is no larger than the drag's 1 unit", depth)
	}

	// A 2x2 profile run straight through: the volume pins the length exactly.
	b := done.body(t, "Body 4")
	closeTo(t, "through-all volume", b.vol, 4*depth, 1e-6)

	// The scene it has to clear is the test document's three bodies. Half the
	// depth reaches from the sketch plane to one end, so that half alone must
	// already be past the furthest of them, margin included.
	const sceneReachAlongZ = 3.0
	if depth/2 <= sceneReachAlongZ {
		t.Errorf("half-depth %v stops short of the scene's %v reach",
			depth/2, sceneReachAlongZ)
	}
}

// TestTooTightAProfileClampsTheDraftAndSaysSo is SPEC-UX §9.3: the extrude is
// never refused for an over-steep draft. It is clamped to the most the profile
// can take, and a toast reports the angle actually used.
func TestTooTightAProfileClampsTheDraftAndSaysSo(t *testing.T) {
	stdout, _ := runScript(t, "m3_clamp")
	dumps := parseM3Dumps(t, stdout)
	open, done := dumps[0], dumps[1]

	if !open.extrude.clamped {
		t.Fatal("40 degrees of draft on a 2-unit square was not clamped")
	}
	if open.extrude.achieved >= open.extrude.draft || open.extrude.achieved <= 0 {
		t.Errorf("achieved draft = %v, want something between 0 and 40",
			open.extrude.achieved)
	}

	b := done.body(t, "Body 4")
	if b.vol <= 0 {
		t.Errorf("the clamped solid has volume %v", b.vol)
	}
	// The clamp stops just short of collapsing the profile, so the solid is a
	// near-spike: far less than the undrafted 24 cubic units.
	if b.vol >= 24 {
		t.Errorf("volume %v shows the draft was not applied at all", b.vol)
	}

	var warned bool
	for _, msg := range done.toasts {
		if strings.Contains(msg, "clamped") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("the clamp was silent; toasts were %q", done.toasts)
	}
}
