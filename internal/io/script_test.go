package io

import (
	"errors"
	"strings"
	"testing"
)

func TestParseScriptHappyPath(t *testing.T) {
	src := `[
	  {"op":"sketch.begin","plane":"Front"},
	  {"op":"sketch.rect","a":[0,0],"b":[8,3]},
	  {"op":"sketch.circle","c":[4,1],"r":1,"segs":16},
	  {"op":"extrude","sketch":"Sketch 1","regions":[0],"depth":6,"draft":5,"dir":"normal","result":"new"},
	  {"op":"camera.view","view":"iso"},
	  {"op":"settle"},
	  {"op":"shot","name":"m3_crate"}
	]`
	s, err := ParseScript([]byte(src))
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	if len(s.Ops) != 7 {
		t.Fatalf("parsed %d ops, want 7", len(s.Ops))
	}
	for i, op := range s.Ops {
		if op.Index != i {
			t.Errorf("op %d has index %d", i, op.Index)
		}
	}
	if s.Ops[1].A == nil || (*s.Ops[1].A)[0] != 0 || (*s.Ops[1].A)[1] != 0 {
		t.Errorf("rect corner a = %v", s.Ops[1].A)
	}
	if s.Ops[2].Segs != 16 || s.Ops[2].R != 1 {
		t.Errorf("circle parsed as segs=%d r=%v", s.Ops[2].Segs, s.Ops[2].R)
	}
	if s.Ops[3].Depth != 6 || s.Ops[3].Draft != 5 || s.Ops[3].Result != "new" {
		t.Errorf("extrude parsed as %+v", s.Ops[3])
	}
}

func TestParseScriptRejectsUnknownOp(t *testing.T) {
	_, err := ParseScript([]byte(`[{"op":"teleport"}]`))
	if err == nil {
		t.Fatal("unknown op accepted")
	}
	if !strings.Contains(err.Error(), "unknown op") {
		t.Errorf("error does not explain the problem: %v", err)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("error is not an OpError: %v", err)
	}
	if opErr.Index != 0 || opErr.Op != "teleport" {
		t.Errorf("OpError = %+v, want index 0 op teleport", opErr)
	}
}

func TestParseScriptNamesTheFailingOpIndex(t *testing.T) {
	src := `[
	  {"op":"settle"},
	  {"op":"settle"},
	  {"op":"shot"}
	]`
	_, err := ParseScript([]byte(src))
	if err == nil {
		t.Fatal("a shot without a name was accepted")
	}
	if !strings.Contains(err.Error(), "op 2") {
		t.Errorf("error does not name op 2: %v", err)
	}
}

func TestParseScriptRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"sketch.begin without a plane", `[{"op":"sketch.begin"}]`, "plane"},
		{"sketch.line without endpoints", `[{"op":"sketch.line","from":[0,0]}]`, "from and to"},
		{"sketch.rect without corners", `[{"op":"sketch.rect","a":[0,0]}]`, "a and b"},
		{"sketch.circle without a radius", `[{"op":"sketch.circle","c":[0,0]}]`, "positive r"},
		{"camera.view without a name", `[{"op":"camera.view"}]`, "view name"},
		{"camera.project with a bad kind", `[{"op":"camera.project","kind":"fisheye"}]`, "ortho or perspective"},
		{"body.visible without a body", `[{"op":"body.visible","visible":true}]`, "body and visible"},
		{"pick without a point", `[{"op":"pick"}]`, "at [x,y]"},
		{"an op with no name", `[{"plane":"Front"}]`, "missing op name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseScript([]byte(c.src))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

// TestParseScriptRejectsStrayFields keeps typos loud: a misspelled key would
// otherwise be silently dropped and the script would quietly do the wrong thing.
func TestParseScriptRejectsStrayFields(t *testing.T) {
	_, err := ParseScript([]byte(`[{"op":"shot","nmae":"typo"}]`))
	if err == nil {
		t.Fatal("a misspelled field was accepted")
	}
}

func TestParseScriptRejectsGarbage(t *testing.T) {
	for _, src := range []string{``, `not json`, `{"op":"settle"}`, `[`} {
		if _, err := ParseScript([]byte(src)); err == nil {
			t.Errorf("accepted garbage %q", src)
		}
	}
}

// TestParseScriptTolerantOfBOM covers files saved by a Windows editor.
func TestParseScriptTolerantOfBOM(t *testing.T) {
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`[{"op":"settle"}]`)...)
	if _, err := ParseScript(src); err != nil {
		t.Fatalf("a script with a byte order mark failed to parse: %v", err)
	}
}

func TestOpErrorWrapping(t *testing.T) {
	inner := errors.New("boom")
	op := Op{Op: "shot", Index: 4}
	err := op.Wrap(inner)
	if !errors.Is(err, inner) {
		t.Error("Wrap lost the underlying error")
	}
	if !strings.Contains(err.Error(), "op 4 (shot)") {
		t.Errorf("error text = %q", err)
	}
	if op.Wrap(nil) != nil {
		t.Error("Wrap(nil) should be nil")
	}
}

func TestLoadScriptMissingFile(t *testing.T) {
	if _, err := LoadScript("no-such-script.json"); err == nil {
		t.Fatal("loading a missing script succeeded")
	}
}
