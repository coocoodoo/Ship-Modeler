package apptest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"modeler/internal/ui"
)

// TestEveryMessageIsRenderable runs every op script in the repository and
// checks that no toast, hint or status line contains a character the font atlas
// cannot draw.
//
// The atlas is a curated list of codepoints (D-11), because the typeface does
// not carry every symbol a sentence might reach for and a missing one draws as
// an empty box. That is invisible in a unit test, easy to miss in a screenshot,
// and it happened: a boolean summary reached for a true minus sign and shipped
// a question mark to the user. This is the check that would have caught it, so
// it runs over every message every script can produce.
func TestEveryMessageIsRenderable(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join(repoRoot, "testdata", "scripts", "*.json"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no scripts found: %v", err)
	}
	line := regexp.MustCompile(`^(toast|hint) "(.*)"$`)

	for _, path := range scripts {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stdout, _ := runScript(t, name)
			for _, raw := range strings.Split(stdout, "\n") {
				m := line.FindStringSubmatch(strings.TrimRight(raw, "\r"))
				if m == nil {
					continue
				}
				if r, bad := ui.FirstUnrenderable(m[2]); bad {
					t.Errorf("%s message %q contains %q (U+%04X), which the font atlas cannot draw",
						m[1], m[2], r, r)
				}
			}
		})
	}
}

// TestScriptsAndGoldensLineUp keeps the two directories honest with each other:
// a script that takes shots must have a baseline directory, and a baseline
// directory must have a script that still produces it.
func TestScriptsAndGoldensLineUp(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join(repoRoot, "testdata", "golden", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goldens {
		script := filepath.Join(repoRoot, "testdata", "scripts", filepath.Base(dir)+".json")
		if _, err := os.Stat(script); err != nil {
			t.Errorf("baseline directory %s has no script that produces it",
				filepath.Base(dir))
		}
	}
}
