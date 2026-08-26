package apptest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The harness builds modeler.exe once and runs each case as a real headless
// process, exactly the way the executing agent uses it by hand (TESTING §4).

var (
	exePath  string
	repoRoot string
)

func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "apptest:", err)
		os.Exit(1)
	}
	repoRoot = root

	dir, err := os.MkdirTemp("", "modeler-apptest-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "apptest:", err)
		os.Exit(1)
	}
	exePath = filepath.Join(dir, "modeler_test.exe")

	build := exec.Command("go", "build", "-o", exePath, "./cmd/modeler")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "apptest: building modeler failed: %v\n%s\n", err, out)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find the repository root (no go.mod above the test)")
}

// runScript executes one op script headlessly and returns its stdout plus the
// directory the shots landed in.
func runScript(t *testing.T, name string) (stdout, outDir string) {
	t.Helper()
	script := filepath.Join(repoRoot, "testdata", "scripts", name+".json")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("missing script %s: %v", script, err)
	}
	outDir = t.TempDir()
	cmd := exec.Command(exePath, "-headless", "-script", script, "-out", outDir)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running %s failed: %v\n%s", name, err, out)
	}
	return string(out), outDir
}

// checkGolden compares every shot a case produced with its committed baseline.
// Setting GOLDEN_UPDATE=1 rewrites the baselines; review the diffs it leaves in
// the output directory before committing them (TESTING §4).
func checkGolden(t *testing.T, caseName, outDir string) {
	t.Helper()
	goldenDir := filepath.Join(repoRoot, "testdata", "golden", caseName)
	shots, err := filepath.Glob(filepath.Join(outDir, "*.png"))
	if err != nil || len(shots) == 0 {
		t.Fatalf("case %s produced no shots", caseName)
	}
	update := os.Getenv("GOLDEN_UPDATE") == "1"

	for _, shot := range shots {
		base := filepath.Base(shot)
		want := filepath.Join(goldenDir, base)

		if update {
			if err := CopyPNG(want, shot); err != nil {
				t.Fatalf("updating baseline %s: %v", want, err)
			}
			t.Logf("updated baseline %s", want)
			continue
		}

		if _, err := os.Stat(want); err != nil {
			t.Fatalf("no baseline for %s; run with GOLDEN_UPDATE=1 and review it", base)
		}
		gotImg, err := LoadPNG(shot)
		if err != nil {
			t.Fatalf("%v", err)
		}
		wantImg, err := LoadPNG(want)
		if err != nil {
			t.Fatalf("%v", err)
		}
		res, err := Compare(gotImg, wantImg)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		if !res.WithinTolerance() {
			diff := filepath.Join(outDir, "diff_"+base)
			if err := WriteDiff(diff, gotImg, wantImg); err == nil {
				t.Logf("wrote diff image %s", diff)
			}
			t.Errorf("%s differs from its baseline: %s", base, res)
			continue
		}
		t.Logf("%s matches: %s", base, res)
	}
}

func TestGoldenViews(t *testing.T) {
	_, outDir := runScript(t, "m0_views")
	checkGolden(t, "m0_views", outDir)
}

func TestGoldenPickScene(t *testing.T) {
	_, outDir := runScript(t, "m0_pick")
	checkGolden(t, "m0_pick", outDir)
}

// TestRenderIsDeterministic runs the same script twice and requires the two
// renders to agree, which is the check M0 asks for before trusting a baseline.
func TestRenderIsDeterministic(t *testing.T) {
	_, dirA := runScript(t, "m0_views")
	_, dirB := runScript(t, "m0_views")

	shots, _ := filepath.Glob(filepath.Join(dirA, "*.png"))
	if len(shots) == 0 {
		t.Fatal("no shots produced")
	}
	for _, a := range shots {
		base := filepath.Base(a)
		imgA, err := LoadPNG(a)
		if err != nil {
			t.Fatal(err)
		}
		imgB, err := LoadPNG(filepath.Join(dirB, base))
		if err != nil {
			t.Fatal(err)
		}
		res, err := Compare(imgA, imgB)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		// Two runs on the same machine, same driver, same clock: identical.
		if res.MaxDelta != 0 {
			t.Errorf("%s is not reproducible run to run: %s", base, res)
		}
	}
}

// pickLine parses one line of the pick op's machine-readable output.
var pickLine = regexp.MustCompile(
	`^pick at=(\d+),(\d+) kind=(\w+)(?: body=(\d+) face=(\d+) edge=(\d+) vert=(\d+) plane=(\w+) dist=([\d.]+))?$`)

type pickOut struct {
	x, y string
	kind string
	body string
	face string
	edge string
	vert string
	dist string
}

func parsePicks(t *testing.T, stdout string) []pickOut {
	t.Helper()
	var out []pickOut
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "pick ") {
			continue
		}
		m := pickLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("unparsable pick line: %q", line)
		}
		out = append(out, pickOut{
			x: m[1], y: m[2], kind: m[3],
			body: m[4], face: m[5], edge: m[6], vert: m[7], dist: m[9],
		})
	}
	return out
}

// TestPickPassResolvesGeometry is the M0 acceptance check for picking: the ID
// pass must name the element actually under the cursor, and must honour the
// forgiveness order vertices > edges > faces (SPEC-UX §12.1).
func TestPickPassResolvesGeometry(t *testing.T) {
	stdout, _ := runScript(t, "m0_pick")
	picks := parsePicks(t, stdout)
	if len(picks) != 7 {
		t.Fatalf("expected 7 pick lines, got %d:\n%s", len(picks), stdout)
	}

	// The script front-frames the test scene, so the geometry under each probe
	// is known: see testdata/scripts/m0_pick.json for the coordinates. They are
	// window pixels, so they move whenever the chrome layout changes; re-derive
	// them with a sweep of pick ops rather than nudging them by hand.
	cases := []struct {
		desc string
		kind string
		body string
	}{
		{"centre of the hull's front face", "face", "1"},
		{"the hull's top edge", "edge", "1"},
		{"a side face of the 16-gon prism", "face", "2"},
		{"a crease on the rotated box", "edge", "3"},
		{"empty space below everything", "none", ""},
		{"the hull's bottom edge", "edge", "1"},
		{"the hull's bottom-left corner", "vert", "1"},
	}
	for i, c := range cases {
		got := picks[i]
		if got.kind != c.kind {
			t.Errorf("probe %d (%s) at %s,%s: kind = %s, want %s",
				i, c.desc, got.x, got.y, got.kind, c.kind)
			continue
		}
		if c.body != "" && got.body != c.body {
			t.Errorf("probe %d (%s): body = %s, want %s", i, c.desc, got.body, c.body)
		}
	}

	// The corner probe must resolve to a vertex even though an edge and a face
	// also cover that pixel: that is the whole point of the priority order.
	if picks[6].kind != "vert" {
		t.Errorf("corner probe resolved to %s, so vertices are not winning over edges", picks[6].kind)
	}
	// Its neighbour a face-width away resolves to the edge, not the vertex.
	if picks[5].kind != "edge" {
		t.Errorf("mid-edge probe resolved to %s, want edge", picks[5].kind)
	}
}

// TestScriptErrorsExitNonZero keeps the golden workflow honest: a broken script
// must fail loudly rather than silently producing no shots (SPEC-RENDER §9).
func TestScriptErrorsExitNonZero(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"op":"camera.view","view":"sideways"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exePath, "-headless", "-script", bad, "-out", dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("a script with an unknown view exited 0:\n%s", out)
	}
	if !strings.Contains(string(out), "op 0") {
		t.Errorf("error does not name the failing op index:\n%s", out)
	}
	if !strings.Contains(string(out), "sideways") {
		t.Errorf("error does not name the offending value:\n%s", out)
	}
}

func TestUnknownOpIsRejected(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"op":"teleport"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exePath, "-headless", "-script", bad, "-out", dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("an unknown op exited 0:\n%s", out)
	}
	if !strings.Contains(string(out), "unknown op") {
		t.Errorf("error does not explain the problem:\n%s", out)
	}
}
