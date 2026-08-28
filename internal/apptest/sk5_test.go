package apptest

import (
	"math"
	"testing"
)

// SK5: the modify tools (Sketch_func.md §5 SK5) — fillet, chamfer, offset,
// mirror and the patterns, each a ReplaceEntities over a selection.

func TestGoldenModifyTools(t *testing.T) {
	_, outDir := runScript(t, "sk5_modify")
	checkGolden(t, "sk5_modify", outDir)
}

// The acceptance: one porthole, patterned four across and mirrored to the
// other side, cut through a plate.
//
// Eight identical holes is a number the tools have to earn twice over — the
// pattern must make three copies at the right spacing, and the mirror must
// make four more the same size. A pattern that dropped one, or a mirror that
// scaled what it reflected, changes this number.
func TestPatternAndMirrorCutEightIdenticalHoles(t *testing.T) {
	stdout, _ := runScript(t, "sk5_modify")
	dumps := parseM3Dumps(t, stdout)
	if len(dumps) != 2 {
		t.Fatalf("expected 2 dumps, got %d:\n%s", len(dumps), stdout)
	}
	before, after := dumps[0], dumps[1]

	const plateVol = 18.0 * 10.0 * 2.0
	if got := before.body(t, "Body 4").vol; math.Abs(got-plateVol) > 1e-6 {
		t.Fatalf("the plate is %.4f before the cut, want %.4f", got, plateVol)
	}

	// One porthole: a 16-gon of radius 0.75, cut through 2 units of plate.
	const holes = 8
	hole := polygonArea(16, 0.75, 0.75)
	want := plateVol - holes*hole*2

	got := after.body(t, "Body 4").vol
	if math.Abs(got-want) > 0.1 {
		t.Errorf("after the cut the plate is %.4f, want %.4f\n"+
			"  %d holes of %.4f each, through 2 of plate — "+
			"a wrong count or a resized mirror moves this",
			got, want, holes, hole)
	}
}

// The regression that found itself while this milestone was being built: a
// fillet must be the *minor* arc between its tangent points.
//
// Taking the long way round sweeps 270° out across the corner, and that arc
// crosses its own legs — which does not merely look wrong, it closes a region.
// A plain L, which has no inside, would become an extrudable shape. Here the
// sketch holds eight circles and one filleted L: exactly eight regions, and
// the L's two ends still loose.
func TestAFilletDoesNotInventARegion(t *testing.T) {
	stdout, _ := runScript(t, "sk5_modify")
	dumps := parseSketchDumps(t, stdout)
	var cutting sketchDump
	for _, d := range dumps {
		if d.present && d.name == "Sketch 2" {
			cutting = d
		}
	}
	if !cutting.present {
		t.Fatalf("the modify sketch was never dumped:\n%s", stdout)
	}

	// 8 circles + the L's two trimmed legs + the fillet arc.
	if cutting.entities != 11 {
		t.Errorf("the sketch holds %d entities, want 11 "+
			"(8 portholes, 2 trimmed legs, 1 fillet arc)", cutting.entities)
	}
	if cutting.regions != 8 {
		t.Errorf("the region engine found %d regions, want the 8 portholes — "+
			"a 9th means the fillet arc swept the long way and closed one",
			cutting.regions)
	}
	if cutting.openEnds != 2 {
		t.Errorf("the sketch has %d open ends, want the filleted L's 2 — "+
			"a fillet trims a corner, it does not close a shape", cutting.openEnds)
	}
}

// The flyout bug the user reported, 2026-08-27: "dropdown menu is buggy, is
// also clicking behind the drop down menu".
//
// The variant list hangs off the toolbar over both the tree panel and the
// viewport, and everything under it went on answering clicks aimed at it. The
// script clicks the Rectangle group's chevron, then clicks "Centre rectangle"
// — a row that sits exactly over the Top plane's tree row.
//
// Three things have to be true afterwards, and only the first was:
//   - the tool changed to Centre rectangle
//   - the Top plane was NOT selected by the same click
//   - no point was placed, with the Point tool armed to make one if it leaked
func TestClickingTheToolFlyoutDoesNotReachWhatIsBehindIt(t *testing.T) {
	stdout, _ := runScript(t, "sk5_flyout")

	sketches := parseSketchDumps(t, stdout)
	if len(sketches) < 3 {
		t.Fatalf("expected 3 sketch dumps, got %d:\n%s", len(sketches), stdout)
	}
	final := sketches[len(sketches)-1]
	if final.tool != "Centre rectangle" {
		t.Errorf("the flyout row picked %q, want Centre rectangle", final.tool)
	}
	// The Point tool was armed throughout: a click that leaked into the
	// viewport would have committed one.
	if final.entities != 0 {
		t.Errorf("%d entities were drawn — a flyout click reached the viewport",
			final.entities)
	}

	docs := parseDumps(t, stdout)
	if len(docs) == 0 {
		t.Fatalf("no doc dumps:\n%s", stdout)
	}
	last := docs[len(docs)-1]
	if last.sel != 0 {
		t.Errorf("the selection is %q after clicking a flyout row — "+
			"the click reached the tree row underneath it", last.selDesc)
	}
}
