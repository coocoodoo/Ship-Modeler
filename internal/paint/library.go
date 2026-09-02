package paint

import (
	"bufio"
	_ "embed"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// The palette library (SPEC-UX §13.3): a few thousand named palettes shipped
// with the program, plus whatever the user drops in their own folder.
//
// They live in one bundled file rather than a few thousand small ones. The
// collection is read start to finish or not at all, a directory of that many
// entries is slow to walk on Windows, and one file is one thing to diff.
// Format, per line: Name<TAB>rrggbb rrggbb ... — see palettes/library.pal.

//go:embed palettes/library.pal
var libraryBundle string

// Palette is a named list of colours.
type Palette struct {
	Name   string
	Colors []color.RGBA
}

var (
	libraryOnce sync.Once
	library     []Palette
)

// Library is the palettes shipped with the program, sorted by name. Parsed
// once on first use: it costs nothing until something asks for it, and the
// program can run a whole session without ever opening the browser.
func Library() []Palette {
	libraryOnce.Do(func() { library = parseBundle(libraryBundle) })
	return library
}

func parseBundle(s string) []Palette {
	var out []Palette
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, body, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var cols []color.RGBA
		for _, tok := range strings.Fields(body) {
			if c, ok := ParseColor(tok); ok {
				cols = append(cols, c)
			}
		}
		if len(cols) > 0 {
			out = append(out, Palette{Name: name, Colors: cols})
		}
	}
	sortPalettes(out)
	return out
}

// ScanFolder reads every .hex and .txt palette in a folder, named by its
// file. A folder that is not there is not an error — it is the ordinary case
// of a user who has not put anything in one.
func ScanFolder(dir string) []Palette {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Palette
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".hex" && ext != ".txt" {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		cols, err := ParseHex(bufio.NewReader(f))
		f.Close()
		if err != nil || len(cols) == 0 {
			continue // a stray text file in the folder is not a failure
		}
		out = append(out, Palette{
			Name:   strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
			Colors: cols,
		})
	}
	sortPalettes(out)
	return out
}

// sortPalettes orders by name, case-insensitively, so the list reads the way
// a person would file it rather than putting every capital letter first.
func sortPalettes(p []Palette) {
	sort.SliceStable(p, func(i, j int) bool {
		a, b := strings.ToLower(p[i].Name), strings.ToLower(p[j].Name)
		if a == b {
			return p[i].Name < p[j].Name
		}
		return a < b
	})
}

// Matches reports whether a palette answers a search box's text: every
// space-separated word must appear somewhere in the name, in any order, so
// "gb pocket" finds "Pocket GB" as readily as "GB Pocket".
func (p Palette) Matches(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	name := strings.ToLower(p.Name)
	for _, word := range strings.Fields(q) {
		if !strings.Contains(name, word) {
			return false
		}
	}
	return true
}
