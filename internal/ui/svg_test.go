package ui

import "testing"

func TestOutlineIconsKeepOpenPathsAndConsistentStrokes(t *testing.T) {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := iconFS.ReadFile("icons/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		s, err := parseSVG(string(data))
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if len(s.contours) == 0 && len(s.strokes) == 0 {
			t.Fatalf("empty icon: %s", e.Name())
		}
		if len(s.strokes) > 0 && s.strokeWidth != 1.75 {
			t.Fatalf("inconsistent stroke: %s", e.Name())
		}
	}
	s, err := parseSVG(`<svg fill="none" stroke-width="1.75"><path d="M3 12H21 M12 3V21"/></svg>`)
	if err != nil || len(s.strokes) != 2 || len(s.strokes[0]) != 2 {
		t.Fatal("open line segments disappeared")
	}
}
