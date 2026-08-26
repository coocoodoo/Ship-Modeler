package apptest

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Invariants that must hold at every dump of every script in the repository.
//
// These are not about any one feature. They are the things that, if they ever
// stop being true, mean something is wrong somewhere nobody was looking — and
// the cheapest place to notice that is over the whole corpus at once.

var invariantBodyLine = regexp.MustCompile(
	`^body id=(\d+) name="([^"]*)" .* valid=(\d)$`)

// TestEveryBodyIsAlwaysAValidSolid checks that no script ever produces, or
// starts from, a body that fails the mesh validator.
//
// Worth stating what this does not catch, since M5 found it the hard way. The
// test scene used to open with a hull built by merging two boxes that met face
// to face. Every shell was closed, manifold, positively oriented and correctly
// counted, so the validator passed it — and it was still two solids pressed
// together rather than one, which Manifold answered by refusing to union onto
// it and changing nothing. Telling that apart from a legal edge-to-edge touch
// needs a check we tried and could not make precise enough to trust; see
// mesh.Merge's comment and DECISIONS V-33.
func TestEveryBodyIsAlwaysAValidSolid(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join(repoRoot, "testdata", "scripts", "*.json"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no scripts found: %v", err)
	}
	for _, path := range scripts {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stdout, _ := runScript(t, name)
			var seen int
			for _, raw := range strings.Split(stdout, "\n") {
				m := invariantBodyLine.FindStringSubmatch(strings.TrimRight(raw, "\r"))
				if m == nil {
					continue
				}
				seen++
				if m[3] != "1" {
					t.Errorf("body %s (%q) is not a valid solid", m[1], m[2])
				}
			}
			if seen == 0 {
				t.Skip("this script dumps no bodies")
			}
		})
	}
}
