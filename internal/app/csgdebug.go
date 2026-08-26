package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"modeler/internal/geom/mesh"
)

// Repro dumps for a boolean that failed (SPEC-UX §11.3).
//
// A boolean that fails leaves the document untouched, which is the right thing
// to do and also the end of the evidence: the geometry that caused it is gone
// the moment the tool closes. Writing the inputs out as OBJ pairs means the
// failure can be reproduced later from a file instead of from a description.

// CSGDumpDir is where repro geometry lands.
const CSGDumpDir = "debug/csg"

// csgDumpEnv turns automatic dumping on. Off by default: a failure is rare and
// writing files nobody asked for is not a thing a modelling tool should do
// behind your back.
const csgDumpEnv = "MODELER_CSG_DUMP"

// CSGDumpEnabled reports whether failures write themselves out.
func CSGDumpEnabled() bool {
	v := os.Getenv(csgDumpEnv)
	return v != "" && v != "0"
}

// dumpCSGRepro writes the tool solid and every target it was run against, as
// OBJ files under debug/csg. It is called on failure only, and it never
// reports its own errors to the user: a dump that fails is not the problem the
// user is having.
func (a *App) dumpCSGRepro(what string, tool *mesh.Mesh, targets []uint32) {
	if !CSGDumpEnabled() {
		return
	}
	dir := filepath.Join(CSGDumpDir, fmt.Sprintf("%s-%d", what, a.Doc().Seq.Body))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if tool != nil {
		_ = writeOBJ(filepath.Join(dir, "tool.obj"), tool)
	}
	for i, id := range targets {
		b := a.Doc().BodyByID(id)
		if b == nil || b.Mesh == nil {
			continue
		}
		name := fmt.Sprintf("target%d-%s.obj", i, safeName(b.Name))
		_ = writeOBJ(filepath.Join(dir, name), b.Mesh)
	}
}

// writeOBJ writes a mesh as a Wavefront OBJ, which every tool on earth can
// open. Faces keep their polygon form: OBJ allows n-gons, and triangulating on
// the way out would throw away the very structure a repro needs to show.
func writeOBJ(path string, m *mesh.Mesh) error {
	var b strings.Builder
	b.WriteString("# modeler csg repro\n")
	for _, v := range m.Verts {
		fmt.Fprintf(&b, "v %.17g %.17g %.17g\n", v.X, v.Y, v.Z)
	}
	for fi := range m.Faces {
		for _, loop := range m.Faces[fi].Loops {
			b.WriteString("f")
			for _, vi := range loop {
				fmt.Fprintf(&b, " %d", vi+1) // OBJ indices start at one
			}
			b.WriteString("\n")
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// safeName strips a body name down to something a filesystem will accept.
func safeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ' || r == '-' || r == '_':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "body"
	}
	return string(out)
}

// csgFailureToast is the message of SPEC-UX §11.3, which says what happened,
// promises nothing was lost, and gives something to try.
func csgFailureToast(err error) string {
	msg := "Boolean failed — nothing was changed. " +
		"This shape combo hit a solver edge; try nudging one body 1 subunit."
	if CSGDumpEnabled() {
		msg += " Repro written to " + CSGDumpDir + "."
	}
	_ = err
	return msg
}
