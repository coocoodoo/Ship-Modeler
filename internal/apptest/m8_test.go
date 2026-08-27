package apptest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// M8 flow tests: save, load, export, autosave and recovery through the real
// app (SPEC-DATA §4–§6).
//
// The one that matters most is the last: work that existed when the process
// died is still there when it starts again. It is tested with two processes,
// because one process deliberately ignores its own recovery files — a test that
// recovered from itself would prove nothing about the case it exists for.

var m8FileLine = regexp.MustCompile(
	`^file name="([^"]*)" saved=(\d) readonly=(\d) recovery=(\d) recents=(\d+)$`)

type fileDump struct {
	name     string
	saved    bool
	readOnly bool
	recovery int
	recents  int
}

func parseFileDumps(t *testing.T, stdout string) []fileDump {
	t.Helper()
	var out []fileDump
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		if !strings.HasPrefix(line, "file ") {
			continue
		}
		m := m8FileLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("unparsable file line: %q", line)
		}
		out = append(out, fileDump{
			name: m[1], saved: m[2] == "1", readOnly: m[3] == "1",
			recovery: atoi(t, m[4]), recents: atoi(t, m[5]),
		})
	}
	return out
}

// runScriptIn runs a generated script with its own config directory, so an
// autosave test cannot reach the profile of whoever is running it.
func runScriptIn(t *testing.T, configDir, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "case.json")
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exePath, "-headless", "-script", script, "-out", dir)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "MODELER_CONFIG_DIR="+configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running the script failed: %v\n%s", err, out)
	}
	return string(out)
}

// jsonPath escapes a path for embedding in a script.
func jsonPath(p string) string {
	return strings.ReplaceAll(filepath.ToSlash(p), `"`, `\"`)
}

func TestSaveAndReopenKeepsTheShip(t *testing.T) {
	dir := t.TempDir()
	ship := filepath.Join(dir, "hull.ship")
	stdout := runScriptIn(t, dir, `[
	  {"op":"paint.begin"},
	  {"op":"paint.res","res":32},
	  {"op":"paint.color","hex":"F2542D"},
	  {"op":"paint.tool","kind":"fill"},
	  {"op":"paint.pixel","body":"Hull","axis":"+z","uv":[8,4]},
	  {"op":"paint.exit"},
	  {"op":"camera.view","view":"front"},
	  {"op":"settle"},
	  {"op":"dump"},
	  {"op":"file.save","path":"`+jsonPath(ship)+`"},
	  {"op":"dump"},
	  {"op":"file.new"},
	  {"op":"dump"},
	  {"op":"file.open","path":"`+jsonPath(ship)+`"},
	  {"op":"settle"},
	  {"op":"dump"}
	]`)

	files := parseFileDumps(t, stdout)
	paints := parseM7Dumps(t, stdout)
	docs := parseM3Dumps(t, stdout)
	if len(files) != 4 {
		t.Fatalf("expected 4 dumps, got %d:\n%s", len(files), stdout)
	}
	before, saved, blank, loaded := 0, 1, 2, 3

	if files[before].saved {
		t.Error("the document claimed a home before it had one")
	}
	if !files[saved].saved || files[saved].name != "hull" {
		t.Errorf("after saving the document is %+v", files[saved])
	}
	if files[saved].recents != 1 {
		t.Errorf("%d recent files after a save, want 1", files[saved].recents)
	}

	// New really empties it, so the reload is proving something.
	if len(docs[blank].bodies) != 0 {
		t.Fatalf("New left %d bodies behind", len(docs[blank].bodies))
	}

	// And the reload brings back the geometry and the paint.
	if len(docs[loaded].bodies) != len(docs[before].bodies) {
		t.Errorf("%d bodies came back, want %d",
			len(docs[loaded].bodies), len(docs[before].bodies))
	}
	for _, want := range docs[before].bodies {
		got := docs[loaded].body(t, want.name)
		if got.tris != want.tris {
			t.Errorf("%s came back with %d triangles, want %d", want.name, got.tris, want.tris)
		}
		if diff := got.vol - want.vol; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s came back at volume %v, want %v", want.name, got.vol, want.vol)
		}
	}
	if len(paints[loaded].pictures) != 1 {
		t.Fatalf("%d pictures came back, want 1", len(paints[loaded].pictures))
	}
	if got, want := paints[loaded].pictures[0], paints[saved].pictures[0]; got.sum != want.sum {
		t.Errorf("the paint came back as %s, want %s", got.sum, want.sum)
	}
}

// TestAutosaveIsRecoveredByTheNextRun is M8's acceptance clause: kill the
// process mid-edit, start again, and the work is offered back.
//
// Two runs, because a process ignores its own recovery files — the whole point
// of the feature is the process that is no longer there.
func TestAutosaveIsRecoveredByTheNextRun(t *testing.T) {
	config := t.TempDir()

	// The first run paints something, never saves it, and dies.
	first := runScriptIn(t, config, `[
	  {"op":"paint.begin"},
	  {"op":"paint.res","res":32},
	  {"op":"paint.color","hex":"21E7E7"},
	  {"op":"paint.tool","kind":"fill"},
	  {"op":"paint.pixel","body":"Hull","axis":"+z","uv":[8,4]},
	  {"op":"paint.exit"},
	  {"op":"dump"},
	  {"op":"file.autosave","kind":"crash"},
	  {"op":"dump"}
	]`)
	crashed := parseM7Dumps(t, first)
	if len(crashed) == 0 || len(crashed[0].pictures) != 1 {
		t.Fatalf("the first run painted nothing:\n%s", first)
	}
	want := crashed[0].pictures[0].sum

	// The second run finds it and takes it back.
	second := runScriptIn(t, config, `[
	  {"op":"dump"},
	  {"op":"file.recover"},
	  {"op":"settle"},
	  {"op":"dump"}
	]`)
	files := parseFileDumps(t, second)
	paints := parseM7Dumps(t, second)
	if len(files) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(files), second)
	}
	if files[0].recovery == 0 {
		t.Fatalf("the second run found nothing to recover:\n%s", second)
	}
	if len(paints[1].pictures) != 1 {
		t.Fatalf("the recovered document has %d pictures:\n%s", len(paints[1].pictures), second)
	}
	if got := paints[1].pictures[0].sum; got != want {
		t.Errorf("the recovered paint is %s, want the %s that was lost", got, want)
	}
	// Recovered work is unsaved work: it has no home and it is dirty, so the
	// next Ctrl+S asks where to put it rather than quietly writing to the
	// autosave folder.
	if files[1].saved {
		t.Error("the recovered document claims to live in the autosave folder")
	}

	// And a third run has nothing left to offer, because taking it back
	// consumed it.
	third := runScriptIn(t, config, `[{"op":"dump"}]`)
	if f := parseFileDumps(t, third); len(f) != 1 || f[0].recovery != 0 {
		t.Errorf("the recovery file survived being recovered: %+v", f)
	}
}

func TestDiscardingRecoveryThrowsItAway(t *testing.T) {
	config := t.TempDir()
	runScriptIn(t, config, `[
	  {"op":"paint.begin"},
	  {"op":"paint.color","hex":"FFE24B"},
	  {"op":"paint.pixel","body":"Hull","axis":"+z","uv":[4,4]},
	  {"op":"file.autosave","kind":"crash"},
	  {"op":"dump"}
	]`)

	second := runScriptIn(t, config, `[
	  {"op":"dump"},
	  {"op":"file.discard"},
	  {"op":"dump"}
	]`)
	files := parseFileDumps(t, second)
	if len(files) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(files), second)
	}
	if files[0].recovery == 0 {
		t.Fatal("there was nothing to discard")
	}
	if files[1].recovery != 0 {
		t.Error("discarding left the offer standing")
	}
	third := runScriptIn(t, config, `[{"op":"dump"}]`)
	if f := parseFileDumps(t, third); f[0].recovery != 0 {
		t.Error("a discarded recovery file came back")
	}
}

// TestASavedDocumentIsNotOfferedBack keeps the recovery prompt from crying
// wolf: work that reached its own file has nothing left to recover.
func TestASavedDocumentIsNotOfferedBack(t *testing.T) {
	config := t.TempDir()
	ship := filepath.Join(t.TempDir(), "kept.ship")
	runScriptIn(t, config, `[
	  {"op":"paint.begin"},
	  {"op":"paint.color","hex":"FFE24B"},
	  {"op":"paint.pixel","body":"Hull","axis":"+z","uv":[4,4]},
	  {"op":"file.autosave"},
	  {"op":"file.save","path":"`+jsonPath(ship)+`"},
	  {"op":"dump"}
	]`)
	second := runScriptIn(t, config, `[{"op":"dump"}]`)
	if f := parseFileDumps(t, second); f[0].recovery != 0 {
		t.Errorf("%d recovery offers after a clean save, want none", f[0].recovery)
	}
}

func TestEveryExportFormatWritesSomething(t *testing.T) {
	dir := t.TempDir()
	names := []string{"ship.glb", "ship.gltf", "ship.obj", "ship.stl", "ship.png"}
	var ops []string
	ops = append(ops, `{"op":"paint.begin"}`,
		`{"op":"paint.color","hex":"F2542D"}`,
		`{"op":"paint.tool","kind":"fill"}`,
		`{"op":"paint.pixel","body":"Hull","axis":"+z","uv":[8,4]}`,
		`{"op":"paint.exit"}`)
	for _, n := range names {
		ops = append(ops, `{"op":"file.export","path":"`+jsonPath(filepath.Join(dir, n))+`"}`)
	}
	ops = append(ops, `{"op":"dump"}`)
	runScriptIn(t, dir, "["+strings.Join(ops, ",\n")+"]")

	for _, n := range names {
		info, err := os.Stat(filepath.Join(dir, n))
		if err != nil {
			t.Errorf("%s was not written: %v", n, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", n)
		}
	}
	// The companions each format needs to be readable at all.
	for _, n := range []string{"ship.mtl", "ship.bin"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s was not written beside its document: %v", n, err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "paint")); err != nil || len(entries) == 0 {
		t.Errorf("the exported textures are missing: %v", err)
	}
}

var m8ExportLine = regexp.MustCompile(`^export format="([^"]*)" scale=(\d+) alpha=(\d)$`)

type exportDump struct {
	format string
	scale  int
	alpha  bool
}

func parseExportDumps(t *testing.T, stdout string) []exportDump {
	t.Helper()
	var out []exportDump
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		m := m8ExportLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("unparsable export line: %q", line)
		}
		out = append(out, exportDump{
			format: m[1], scale: atoi(t, m[2]), alpha: m[3] == "1",
		})
	}
	return out
}

func TestGoldenExportCard(t *testing.T) {
	_, outDir := runScript(t, "m8_export")
	checkGolden(t, "m8_export", outDir)
}

// TestTheExportCardOffersWhatEachFormatNeeds covers SPEC-DATA §5: the options
// belong in the app, and they change with the format.
//
// A native file dialog has nowhere sensible to put a scale chip, and the choice
// of format changes what the dialog should even filter for — so the card
// decides the format and the dialog decides only the path.
func TestTheExportCardOffersWhatEachFormatNeeds(t *testing.T) {
	stdout, _ := runScript(t, "m8_export")
	exports := parseExportDumps(t, stdout)
	files := parseFileDumps(t, stdout)
	if len(files) != 3 {
		t.Fatalf("expected 3 dumps, got %d:\n%s", len(files), stdout)
	}
	// The card is open for two of the three dumps, and closed for the last.
	if len(exports) != 2 {
		t.Fatalf("the card reported itself %d times, want 2:\n%s", len(exports), stdout)
	}
	if exports[0].format != ".glb" {
		t.Errorf("the card opens on %q, want the self-contained glTF", exports[0].format)
	}
	if exports[1].format != ".png" {
		t.Errorf("choosing png left the format at %q", exports[1].format)
	}
	if exports[1].scale != 1 {
		t.Errorf("the scale is %d×, want 1× until it is changed", exports[1].scale)
	}
}
