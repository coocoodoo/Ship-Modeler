package apptest

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// M7 flow tests: paint mode through the real brush, the real command bus and
// the real renderer.
//
// The measurements are texel coordinates, texel sizes and a hash of each
// picture. A screenshot cannot tell you whether paint is one texel out, whether
// an undo put back the picture or merely something the same size, or whether
// two fragments of a cut face are reading from one image or from two copies of
// it — and every one of those is a way paint mode can be quietly wrong.

// facePaintDump is one painted picture out of a dump block.
type facePaintDump struct {
	body   int
	face   int
	faces  int // how many faces read from this one picture
	res    int
	texel  float64
	rect   [4]int
	opaque int
	sum    string
}

// paintDump is the brush state and, when the pointer is over a face, what it is
// over.
type paintDump struct {
	mode     bool
	tool     string
	size     int
	res      int
	color    string
	color2   string
	dither   string
	fill     bool
	slot     int
	textures bool
	locked   bool
	lockFace int
	// target is the face the panel's controls would act on: the live hover, or
	// the last one, so a button can be reached without the journey disarming it.
	target int
	// awaiting is the Lock button armed and waiting for a face to be clicked.
	awaiting bool

	hovering  bool
	hoverBody int
	hoverFace int
	hoverTex  [2]int
	hoverRes  int
	allocated bool
	oblique   float64

	pictures []facePaintDump
	toasts   []string
	hint     string
}

var (
	m7ModeLine = regexp.MustCompile(
		`^paint mode=(\d) tool="(\w+)" size=(\d+) res=(\d+) color="(\w+)" color2="(\w+)" ` +
			`dither="(\w+)" fill=(\d) slot=(\d) textures=(\d) locked=(\d) lockface=(\d+) ` +
			`target=(\d+) awaitlock=(\d)$`)
	m7HoverLine = regexp.MustCompile(
		`^painthover body=(\d+) face=(\d+) texel=(-?\d+),(-?\d+) res=(\d+) ` +
			`allocated=(\d) oblique=([\d.]+)$`)
	m7PaintLine = regexp.MustCompile(
		`^facepaint body=(\d+) face=(\d+) faces=(\d+) res=(\d+) texel=([\d.]+) ` +
			`rect=(-?\d+),(-?\d+),(-?\d+),(-?\d+) opaque=(\d+) sum=([0-9a-f]+)$`)
	m7HintLine  = regexp.MustCompile(`^hint "(.*)"$`)
	m7ToastLine = regexp.MustCompile(`^toast "(.*)"$`)
)

func parseM7Dumps(t *testing.T, stdout string) []paintDump {
	t.Helper()
	var out []paintDump
	var cur *paintDump

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "doc ") {
			out = append(out, paintDump{})
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "paint mode="):
			m := m7ModeLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable paint line: %q", line)
			}
			cur.mode = m[1] == "1"
			cur.tool = m[2]
			cur.size = atoi(t, m[3])
			cur.res = atoi(t, m[4])
			cur.color = m[5]
			cur.color2 = m[6]
			cur.dither = m[7]
			cur.fill = m[8] == "1"
			cur.slot = atoi(t, m[9])
			cur.textures = m[10] == "1"
			cur.locked = m[11] == "1"
			cur.lockFace = atoi(t, m[12])
			cur.target = atoi(t, m[13])
			cur.awaiting = m[14] == "1"
		case strings.HasPrefix(line, "painthover "):
			m := m7HoverLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable painthover line: %q", line)
			}
			cur.hovering = true
			cur.hoverBody = atoi(t, m[1])
			cur.hoverFace = atoi(t, m[2])
			cur.hoverTex = [2]int{m7Int(t, m[3]), m7Int(t, m[4])}
			cur.hoverRes = atoi(t, m[5])
			cur.allocated = m[6] == "1"
			cur.oblique = m7Num(t, m[7])
		case strings.HasPrefix(line, "facepaint "):
			m := m7PaintLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsable facepaint line: %q", line)
			}
			cur.pictures = append(cur.pictures, facePaintDump{
				body:  atoi(t, m[1]),
				face:  atoi(t, m[2]),
				faces: atoi(t, m[3]),
				res:   atoi(t, m[4]),
				texel: m7Num(t, m[5]),
				rect: [4]int{m7Int(t, m[6]), m7Int(t, m[7]),
					m7Int(t, m[8]), m7Int(t, m[9])},
				opaque: atoi(t, m[10]),
				sum:    m[11],
			})
		case strings.HasPrefix(line, "toast "):
			if m := m7ToastLine.FindStringSubmatch(line); m != nil {
				cur.toasts = append(cur.toasts, m[1])
			}
		case strings.HasPrefix(line, "hint "):
			if m := m7HintLine.FindStringSubmatch(line); m != nil {
				cur.hint = m[1]
			}
		}
	}
	return out
}

// m7Int parses a signed integer. The shared helper takes unsigned ones only,
// and a picture's rectangle starts at -1: the margin the first allocation
// leaves around the face so a stroke along its edge has somewhere to land.
func m7Int(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("unparsable integer %q: %v", s, err)
	}
	return v
}

func m7Num(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("unparsable number %q: %v", s, err)
	}
	return v
}

// picture finds the picture a body's face reads from.
func (d paintDump) picture(t *testing.T, body, face int) facePaintDump {
	t.Helper()
	for _, p := range d.pictures {
		if p.body == body && p.face == face {
			return p
		}
	}
	t.Fatalf("no picture on body %d face %d; pictures were %+v", body, face, d.pictures)
	return facePaintDump{}
}

func TestGoldenPaintedShip(t *testing.T) {
	_, outDir := runScript(t, "m7_painted")
	checkGolden(t, "m7_painted", outDir)
}

// TestPaintingAShipAtThreeDensities is M7's acceptance as arithmetic.
//
// A resolution chip means "this face is N texels across at its widest"
// (SPEC-GEOMETRY §8.2), so the texel size it produces is the face's longest
// side over N — and it is fixed at that from then on. Three faces painted at
// two chips is the case where getting that wrong shows up: a shared density
// would make the same chip mean different things on different faces, and a
// recomputed one would make a face's pixels resize when the face did.
func TestPaintingAShipAtTwoChips(t *testing.T) {
	stdout, _ := runScript(t, "m7_painted")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, plated, painted := dumps[0], dumps[1], dumps[2]

	if before.mode {
		t.Error("paint mode was already on before the script asked for it")
	}
	if len(before.pictures) != 0 {
		t.Errorf("the scene starts with paint on it: %+v", before.pictures)
	}
	if !plated.mode {
		t.Error("paint mode did not open")
	}

	// The hull's front is 12 u across and the cockpit block's top is 5 u, and
	// both were painted at 4 px/u — so both have quarter-unit texels. That
	// equality is the whole point of the chip being a density (V-128): before
	// it, the same chip gave the front 0.375 u pixels and the cockpit 0.156,
	// and nothing on screen admitted it.
	front := painted.picture(t, 1, 7)
	if front.res != 4 || math.Abs(front.texel-1.0/4) > 1e-9 {
		t.Errorf("the front face is %d px/u at %v u/texel, want 4 at %v",
			front.res, front.texel, 1.0/4)
	}
	top := painted.picture(t, 1, 17)
	if top.res != 4 || math.Abs(top.texel-1.0/4) > 1e-9 {
		t.Errorf("the cockpit face is %d px/u at %v u/texel, want 4 at %v",
			top.res, top.texel, 1.0/4)
	}
	if front.texel != top.texel {
		t.Errorf("two faces painted at the same chip have %v and %v u texels — "+
			"a pixel has to be the same size everywhere", front.texel, top.texel)
	}
	// A different chip is still a different density: the end cap was painted at
	// 16 px/u, so its texels are a sixteenth of a unit.
	end := painted.picture(t, 1, 9)
	if end.res != 16 || math.Abs(end.texel-1.0/16) > 1e-9 {
		t.Errorf("the end face is %d px/u at %v u/texel, want 16 at %v",
			end.res, end.texel, 1.0/16)
	}

	// Painting one face leaves the others alone: the plating shot has one
	// picture on it, and it is the same picture at the end of the run.
	if len(plated.pictures) != 1 {
		t.Errorf("plating the front painted %d faces, want 1", len(plated.pictures))
	}
	if plated.pictures[0].sum != front.sum {
		t.Error("painting other faces changed the front's picture")
	}
	if front.opaque == 0 || top.opaque == 0 || end.opaque == 0 {
		t.Errorf("a face came out with no painted texels: %+v", painted.pictures)
	}
}

// TestUndoPutsThePictureBack is SPEC-DATA §3.3 through the app.
//
// The hash is the point. Two strokes over already-painted texels leave the
// count of painted texels exactly where it was, so an undo that restored the
// wrong rectangle — or nothing at all — would pass any test that counted.
func TestUndoPutsThePictureBack(t *testing.T) {
	stdout, _ := runScript(t, "m7_painted")
	dumps := parseM7Dumps(t, stdout)
	painted, undone, redone := dumps[2], dumps[3], dumps[4]

	before := painted.picture(t, 1, 9)
	after := undone.picture(t, 1, 9)
	if after.sum == before.sum {
		t.Fatal("two undos changed nothing — the strokes were never separate steps")
	}
	if after.opaque != before.opaque {
		t.Errorf("undoing strokes over painted texels changed the painted count "+
			"from %d to %d, so this test would have passed on the count alone",
			before.opaque, after.opaque)
	}
	if got := redone.picture(t, 1, 9); got.sum != before.sum {
		t.Errorf("redo produced a different picture: %s, want %s", got.sum, before.sum)
	}
	// Undo is per stroke, not per face: the other two faces sit still.
	for _, face := range []int{7, 17} {
		if undone.picture(t, 1, face).sum != painted.picture(t, 1, face).sum {
			t.Errorf("undoing a stroke on the end face disturbed face %d", face)
		}
	}
}

func TestGoldenPaintCursor(t *testing.T) {
	_, outDir := runScript(t, "m7_cursor")
	checkGolden(t, "m7_cursor", outDir)
}

// TestTheCursorShowsTheGridBeforeYouCommitToIt covers the provisional mapping
// of SPEC-UX §13.1: the resolution chips have to mean something on an unpainted
// face, or choosing one is guesswork.
func TestTheCursorShowsTheGridBeforeYouCommitToIt(t *testing.T) {
	stdout, _ := runScript(t, "m7_cursor")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 6 {
		t.Fatalf("expected 6 dumps, got %d:\n%s", len(dumps), stdout)
	}
	at32, at16 := dumps[0], dumps[3]

	for _, d := range []paintDump{at32, at16} {
		if !d.hovering {
			t.Fatal("the pointer resolved no face to paint")
		}
		if d.allocated {
			t.Error("the face already had a texture; this case is about the one it would get")
		}
		if len(d.pictures) != 0 {
			t.Error("hovering a face painted it")
		}
	}
	// Same pixel, half the density: the texel under the cursor is half the
	// index it was, because each texel is twice as wide.
	if at32.hoverTex != [2]int{17, 8} {
		t.Errorf("at 4 px/u the cursor is on texel %v, want 17,8", at32.hoverTex)
	}
	if at16.hoverTex != [2]int{8, 4} {
		t.Errorf("at 2 px/u the same pixel is on texel %v, want 8,4", at16.hoverTex)
	}
	if at16.hoverRes != 2 || at32.hoverRes != 4 {
		t.Errorf("the chip did not reach the mapping: %d and %d",
			at32.hoverRes, at16.hoverRes)
	}
}

// TestTheObliqueChipTurnsTheCamera is SPEC-UX §13.2: past 70° off square-on,
// the card offers a view you can actually aim in, and taking it works.
func TestTheObliqueChipTurnsTheCamera(t *testing.T) {
	stdout, _ := runScript(t, "m7_cursor")
	dumps := parseM7Dumps(t, stdout)
	oblique, turned := dumps[4], dumps[5]

	if oblique.oblique <= 70 {
		t.Fatalf("the grazing view came out at %v°, which would not offer the chip",
			oblique.oblique)
	}
	if turned.oblique > 1 {
		t.Errorf("after Face view the same face is still %v° off square-on",
			turned.oblique)
	}
	if turned.hoverFace != oblique.hoverFace {
		t.Errorf("Face view turned to a different face: %d, was %d",
			turned.hoverFace, oblique.hoverFace)
	}
}

func TestGoldenResample(t *testing.T) {
	_, outDir := runScript(t, "m7_resample")
	checkGolden(t, "m7_resample", outDir)
}

// TestResampleRebuildsTheFaceAtTheNewChip covers the second half of the
// mismatch prompt (SPEC-UX §13.2), including the part that matters most: a
// resample is an ordinary undoable command, not a quiet rebuild.
func TestResampleRebuildsTheFaceAtTheNewChip(t *testing.T) {
	stdout, _ := runScript(t, "m7_resample")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 4 {
		t.Fatalf("expected 4 dumps, got %d:\n%s", len(dumps), stdout)
	}
	small, mismatch, big, undone := dumps[0], dumps[1], dumps[2], dumps[3]

	before := small.picture(t, 1, 7)
	if before.res != 2 || math.Abs(before.texel-1.0/2) > 1e-9 {
		t.Errorf("the face came out %d px/u at %v u/texel, want 2 at 0.5",
			before.res, before.texel)
	}

	// The prompt is an offer, not a gate: the brush is at 16 and the face is
	// still 2, and the app says so rather than silently doing either.
	if mismatch.res != 16 || mismatch.hoverRes != 2 {
		t.Errorf("brush at %d px/u over a %d px/u face, want 16 over 2",
			mismatch.res, mismatch.hoverRes)
	}
	if !strings.Contains(mismatch.hint, "2 px") {
		t.Errorf("the hint bar said %q, which does not mention the face's own density",
			mismatch.hint)
	}
	if mismatch.picture(t, 1, 7).texel != before.texel {
		t.Error("arming a different chip resized an existing face's texels")
	}

	after := big.picture(t, 1, 7)
	if after.res != 16 || math.Abs(after.texel-1.0/16) > 1e-9 {
		t.Errorf("the resampled face is %d px/u at %v u/texel, want 16 at %v",
			after.res, after.texel, 1.0/16)
	}
	// Eight times the density in each direction, so the picture covers eight
	// times the texels along each axis and stays in the same place.
	if got, want := after.rect[2]-after.rect[0], (before.rect[2]-before.rect[0]-2)*8+2; got != want {
		t.Errorf("the resampled picture is %d texels wide, want %d", got, want)
	}
	if after.opaque <= before.opaque {
		t.Errorf("the resample kept %d painted texels, fewer than the %d it started with",
			after.opaque, before.opaque)
	}
	if got := undone.picture(t, 1, 7); got.sum != before.sum || got.res != 2 {
		t.Errorf("undoing the resample left a %d px/u picture with sum %s, want 2 px/u %s",
			got.res, got.sum, before.sum)
	}
}

func TestGoldenPaintSurvivesACut(t *testing.T) {
	_, outDir := runScript(t, "m7_persist")
	checkGolden(t, "m7_persist", outDir)
}

// TestPaintSurvivesACutInTheApp is SPEC-GEOMETRY §8.4 end to end.
//
// The kernel's own test proves the fragments share the source picture. This
// proves the rest of the program agrees: the same screen pixel resolves to the
// same texel of the same picture after a boolean has rebuilt the mesh
// underneath it, and the eyedropper reads back the colour that was painted
// there rather than the body's own.
func TestPaintSurvivesACutInTheApp(t *testing.T) {
	stdout, _ := runScript(t, "m7_persist")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, after := dumps[0], dumps[1]

	whole := before.pictures[0]
	if whole.faces != 1 {
		t.Fatalf("the face was already in %d pieces before the cut", whole.faces)
	}
	if len(after.pictures) != 1 {
		t.Fatalf("the cut left %d pictures, want the one they share", len(after.pictures))
	}
	frag := after.pictures[0]

	if frag.faces != 2 {
		t.Errorf("the cut left %d faces reading from the picture, want the 2 fragments",
			frag.faces)
	}
	if frag.sum != whole.sum {
		t.Errorf("the picture changed across the cut: %s, was %s", frag.sum, whole.sum)
	}
	if frag.texel != whole.texel || frag.rect != whole.rect {
		t.Errorf("the mapping moved: %v %v, was %v %v",
			frag.texel, frag.rect, whole.texel, whole.rect)
	}
	// The identity changed — these are new faces — which is exactly why the
	// picture has to be followed by lineage rather than by face number.
	if frag.face == whole.face {
		t.Error("the cut produced the same face identity, so this proves nothing")
	}

	// The same pixel of the window, before and after.
	if before.hoverTex != after.hoverTex {
		t.Errorf("the same pixel now lands on texel %v, was %v",
			after.hoverTex, before.hoverTex)
	}
	// And the eyedropper reads the paint, not the body colour under it.
	const painted = "Picked #21E7E7"
	if !hasToast(before.toasts, painted) {
		t.Fatalf("the eyedropper did not read the paint before the cut: %q", before.toasts)
	}
	if !hasToast(after.toasts, painted) {
		t.Errorf("the eyedropper reads %q after the cut, want %q", after.toasts, painted)
	}
}

func hasToast(toasts []string, want string) bool {
	for _, t := range toasts {
		if t == want {
			return true
		}
	}
	return false
}

func TestGoldenPaintStroke(t *testing.T) {
	_, outDir := runScript(t, "m7_stroke")
	checkGolden(t, "m7_stroke", outDir)
}

// TestADraggedStrokeIsLiveAndIsOneStep drives paint mode the way a hand does:
// a press, a run of motion and a release.
//
// Every scripted paint op elsewhere calls the command directly, which proves
// the brush and skips the part that goes wrong. M6's last bug was exactly this
// gap — no drag in the program had ever been tested through a real press — so
// this one goes through the pointer: the picture has to be live while the
// button is down, and the whole drag has to land as one entry in the history
// (SPEC-DATA §3.2).
func TestADraggedStrokeIsLiveAndIsOneStep(t *testing.T) {
	stdout, _ := runScript(t, "m7_stroke")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, holding, released, undone, erased := dumps[0], dumps[1], dumps[2], dumps[3], dumps[4]
	depth := undoDepth(t, stdout)

	if len(before.pictures) != 0 {
		t.Fatal("the face was already painted before the drag")
	}
	// Live: the picture exists and is painted while the button is still down.
	if len(holding.pictures) != 1 {
		t.Fatal("nothing was painted mid-drag — the stroke only applies on release")
	}
	live := holding.pictures[0]
	if live.opaque < 20 {
		t.Errorf("mid-drag the stroke has only %d texels; a drag across a third of "+
			"the face that dabs only at its samples would look like this", live.opaque)
	}
	if depth[1] != depth[0] {
		t.Errorf("the history grew to %d mid-drag, want it untouched at %d",
			depth[1], depth[0])
	}

	// One step for the whole drag, and releasing does not repaint.
	if got := depth[2] - depth[0]; got != 1 {
		t.Errorf("the drag left %d history entries, want 1", got)
	}
	if released.pictures[0].sum != live.sum {
		t.Error("releasing the button changed the picture the drag had already drawn")
	}
	if undone.hovering && undone.allocated {
		t.Error("undoing the first stroke on a face left its texture behind")
	}
	if len(undone.pictures) != 0 {
		t.Errorf("undo left %d pictures behind", len(undone.pictures))
	}

	// The eraser goes back to the body's own colour rather than painting over
	// it in something that looks like it (SPEC-UX §13.2).
	rubbed := erased.pictures[0]
	if rubbed.opaque >= released.pictures[0].opaque {
		t.Errorf("erasing left %d painted texels, up from %d",
			rubbed.opaque, released.pictures[0].opaque)
	}
}

var m7UndoLine = regexp.MustCompile(`undo=(\d+) redo=(\d+)`)

// undoDepth reads the history depth out of every dump block, which is how the
// coalescing rule is checked without a second parser.
func undoDepth(t *testing.T, stdout string) []int {
	t.Helper()
	var out []int
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		if !strings.HasPrefix(line, "doc ") {
			continue
		}
		m := m7UndoLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("unparsable doc line: %q", line)
		}
		out = append(out, atoi(t, m[1]))
	}
	return out
}

// --- Shapes, the soft brush, gradients and the face lock (SPEC-UX §13.4–13.5) ---

func TestGoldenPaintShapes(t *testing.T) {
	_, outDir := runScript(t, "m7_shapes")
	checkGolden(t, "m7_shapes", outDir)
}

// TestEachDitherModeIsItsOwnPicture is the point of having four of them.
//
// The same drag between the same two colours has to come out differently under
// each Bayer order, and the smooth mode has to differ from all three. A mode
// chip that quietly did nothing would still produce a gradient, and a
// screenshot of one ramp looks much like a screenshot of another.
func TestEachDitherModeIsItsOwnPicture(t *testing.T) {
	stdout, _ := runScript(t, "m7_shapes")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 6 {
		t.Fatalf("expected 6 dumps, got %d:\n%s", len(dumps), stdout)
	}

	want := []string{"None", "2x2", "4x4", "8x8"}
	seen := map[string]string{}
	for i, mode := range want {
		if got := dumps[i].dither; got != mode {
			t.Fatalf("dump %d was taken in %q mode, want %q", i, got, mode)
		}
		sum := dumps[i].picture(t, 1, 7).sum
		for other, otherSum := range seen {
			if sum == otherSum {
				t.Errorf("%s and %s produced the same picture (%s)", mode, other, sum)
			}
		}
		seen[mode] = sum
	}

	// Every ramp covers the same texels — they differ in colour, not in reach.
	base := dumps[0].picture(t, 1, 7).opaque
	for i := 1; i < len(want); i++ {
		if got := dumps[i].picture(t, 1, 7).opaque; got != base {
			t.Errorf("%s painted %d texels, want the same %d as the smooth ramp",
				want[i], got, base)
		}
	}
}

// TestTheShapeToolsEachLeaveTheirOwnMark keeps the four two-point tools honest
// about being four tools: each adds to the picture, and the fill toggle and the
// brush size reach them.
func TestTheShapeToolsEachLeaveTheirOwnMark(t *testing.T) {
	stdout, _ := runScript(t, "m7_shapes")
	dumps := parseM7Dumps(t, stdout)
	ramp, shapes, soft := dumps[3], dumps[4], dumps[5]

	if shapes.picture(t, 1, 7).sum == ramp.picture(t, 1, 7).sum {
		t.Error("the circle, rectangle and line left the gradient untouched")
	}
	// The ramp already filled the face, so the shapes recolour rather than add:
	// the count stays put and only the picture changes. That is exactly the
	// case a texel count cannot see.
	if got, want := shapes.picture(t, 1, 7).opaque, ramp.picture(t, 1, 7).opaque; got != want {
		t.Errorf("the shapes changed the painted count from %d to %d", want, got)
	}
	if b := dumps[4]; b.tool != "Line" || b.fill != true {
		t.Errorf("the shapes dump was taken with tool=%q fill=%v", b.tool, b.fill)
	}

	// The soft brush lands on a different face, at a size the chips only gained
	// for it.
	if b := dumps[5]; b.tool != "Brush" || b.size != 16 {
		t.Errorf("the soft brush dump has tool=%q size=%d, want Brush at 16", b.tool, b.size)
	}
	if got := soft.picture(t, 1, 17).opaque; got == 0 {
		t.Error("the soft brush painted nothing on the cockpit face")
	}
}

func TestGoldenFaceLock(t *testing.T) {
	_, outDir := runScript(t, "m7_lock")
	checkGolden(t, "m7_lock", outDir)
}

// TestTheLockKeepsPaintOnOneFace is the user's request, as arithmetic: the same
// window pixel that resolves a second face when nothing is locked must resolve
// nothing at all while the first one is.
func TestTheLockKeepsPaintOnOneFace(t *testing.T) {
	stdout, _ := runScript(t, "m7_lock")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, locked, painted, offFace, unlocked := dumps[0], dumps[1], dumps[2], dumps[3], dumps[4]

	if before.locked {
		t.Fatal("something was locked before the script asked for it")
	}
	if !locked.locked || locked.lockFace != before.hoverFace {
		t.Errorf("locked to face %d, want the hovered face %d",
			locked.lockFace, before.hoverFace)
	}

	// Locking turns the camera square-on: that is half of what the button is for.
	if before.oblique < 20 {
		t.Fatalf("the starting view was already %v° on, so this proves nothing",
			before.oblique)
	}
	if locked.oblique > 1 {
		t.Errorf("after locking the face is still %v° off square-on", locked.oblique)
	}

	// A drag paints the locked face.
	if len(painted.pictures) != 1 || painted.pictures[0].face != locked.lockFace {
		t.Fatalf("the drag did not paint the locked face; pictures were %+v",
			painted.pictures)
	}

	// The pointer over a different face, while locked: nothing to paint.
	if offFace.hovering {
		t.Errorf("while locked, the pointer over another face resolved face %d",
			offFace.hoverFace)
	}
	if !strings.Contains(offFace.hint, "Locked") {
		t.Errorf("the hint bar said %q, which does not explain why nothing is armed",
			offFace.hint)
	}
	// Unlock, same pixel: now it resolves — and it is a different face, which is
	// what the lock was stopping.
	if !unlocked.hovering {
		t.Fatal("unlocking left the pointer resolving nothing")
	}
	if unlocked.hoverFace == locked.lockFace {
		t.Fatal("the test pixel is over the locked face, so the lock proved nothing")
	}
	if unlocked.locked {
		t.Error("the lock survived Unlock")
	}
	// And the paint that was made under the lock is untouched by any of it.
	if got, want := unlocked.pictures[0].sum, painted.pictures[0].sum; got != want {
		t.Errorf("unlocking changed the picture: %s, was %s", got, want)
	}
}

func TestGoldenPanelReach(t *testing.T) {
	_, outDir := runScript(t, "m7_panelreach")
	checkGolden(t, "m7_panelreach", outDir)
}

// TestLockIsArmedFirstAndPickedSecond is the flow the user asked for after the
// first attempt got it backwards: press Lock, then click the face.
//
// The first version acted on whatever was under the pointer when the button was
// pressed, which cannot work — reaching for the button is exactly what takes the
// pointer off the face, so it greyed out on the way there. Arming first makes
// the button live whatever the pointer is doing, and the click that follows is
// spent on the choice rather than on paint. It is the same shape as pressing S
// with no plane selected (SPEC-UX §8.1).
func TestLockIsArmedFirstAndPickedSecond(t *testing.T) {
	stdout, _ := runScript(t, "m7_panelreach")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 5 {
		t.Fatalf("expected 5 dumps, got %d:\n%s", len(dumps), stdout)
	}
	idle, armed, locked := dumps[0], dumps[1], dumps[2]

	// Nothing is hovered when the button is pressed. That is the case the old
	// version could not handle at all.
	if idle.hovering || idle.target != 0 {
		t.Fatal("the pointer was already on a face, so this proves nothing")
	}
	if !armed.awaiting {
		t.Fatalf("clicking Lock did not arm the pick; toasts were %q", armed.toasts)
	}
	if armed.locked {
		t.Error("clicking Lock locked something before a face had been chosen")
	}
	if !strings.Contains(armed.hint, "Click the face") {
		t.Errorf("the hint bar said %q, which does not say what to do next", armed.hint)
	}

	// The click that follows chooses the face, turns the camera to it, and
	// paints nothing.
	if !locked.locked || locked.awaiting {
		t.Fatalf("clicking a face did not finish the lock; toasts were %q", locked.toasts)
	}
	if locked.lockFace == 0 {
		t.Error("locked to no face in particular")
	}
	if locked.oblique > 1 {
		t.Errorf("the camera is %v° off square-on to the face it locked to", locked.oblique)
	}
	if len(locked.pictures) != 0 {
		t.Errorf("the click that chose the face also painted it: %+v", locked.pictures)
	}
}

// TestThePanelStillKnowsWhatItWasPointedAt covers the other half of the same
// report, which the arming flow does not remove: the Face view button and the
// resolution mismatch prompt still act on the face under the pointer, and the
// pointer stops being on one the moment it leaves the viewport for the panel.
func TestThePanelStillKnowsWhatItWasPointedAt(t *testing.T) {
	stdout, _ := runScript(t, "m7_panelreach")
	dumps := parseM7Dumps(t, stdout)
	onFace, onPanel := dumps[3], dumps[4]

	if !onFace.hovering || onFace.target == 0 {
		t.Fatal("the pointer never resolved a face to begin with")
	}
	// Over the panel there is no live cursor — there is no texel under a
	// button — but the controls still know what they would act on.
	if onPanel.hovering {
		t.Error("the pointer over the panel still reports a texel under it")
	}
	if onPanel.target != onFace.target {
		t.Errorf("moving onto the panel changed the target from face %d to %d, "+
			"which is what made the buttons impossible to click",
			onFace.target, onPanel.target)
	}
}

// TestClickingThePanelDoesNotPaintThroughIt is the other half of the same bug.
//
// A floating card sits inside the viewport, so the viewport's own hit-testing
// used to run behind it: every press on a chip resolved whatever face was
// behind the panel and left a dab on it. Cards now own the pointer over them.
func TestClickingThePanelDoesNotPaintThroughIt(t *testing.T) {
	stdout, _ := runScript(t, "m7_panelreach")
	dumps := parseM7Dumps(t, stdout)
	armed := dumps[1]

	if len(armed.pictures) != 0 {
		t.Errorf("clicking a panel button painted %d face(s) behind it: %+v",
			len(armed.pictures), armed.pictures)
	}
}
