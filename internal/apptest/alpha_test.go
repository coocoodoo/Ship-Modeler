package apptest

import "testing"

// The brush's alpha (V-158), asked for as "between Palette and Recent, can you
// add an alpha slider? and make alpha colors functional?".
//
// Functional is the hard half. The slider is a number; what makes it real is
// that a stroke at less than full alpha leaves texels you can see the hull
// through, that painting over one of them mixes rather than replaces, and that
// the top of the slider still lays paint down exactly as it did before alpha
// existed — every golden in the repo is a claim about that last part.

func TestGoldenPaintAlpha(t *testing.T) {
	_, outDir := runScript(t, "m7_alpha")
	checkGolden(t, "m7_alpha", outDir)
}

func TestATranslucentStrokeLeavesPaintYouCanSeeThrough(t *testing.T) {
	stdout, _ := runScript(t, "m7_alpha")
	dumps := parseM7Dumps(t, stdout)
	if len(dumps) != 6 {
		t.Fatalf("expected 6 dumps, got %d:\n%s", len(dumps), stdout)
	}
	start, solid, armed, thin, layered := dumps[0], dumps[1], dumps[2], dumps[3], dumps[4]

	if start.alpha != 255 {
		t.Errorf("the brush starts at alpha %d, want it solid", start.alpha)
	}
	if len(start.pictures) != 0 {
		t.Fatal("the face was already painted before the script painted anything")
	}

	// A full-alpha stroke is what it always was: opaque texels, nothing thin.
	if len(solid.pictures) != 1 {
		t.Fatal("the solid stroke painted nothing")
	}
	if solid.pictures[0].thin != 0 {
		t.Errorf("a stroke at full alpha left %d see-through texels, want none",
			solid.pictures[0].thin)
	}
	if solid.pictures[0].opaque == 0 {
		t.Error("the solid stroke left no paint at all")
	}

	// Moving the slider changes the brush and nothing else.
	if armed.alpha != 110 {
		t.Errorf("the slider set alpha to %d, want 110", armed.alpha)
	}
	if armed.pictures[0].sum != solid.pictures[0].sum {
		t.Error("arming a lower alpha changed paint that was already down")
	}

	// The stroke that follows lays paint you can see the hull through.
	laid := thin.pictures[0].thin
	if laid == 0 {
		t.Fatal("a stroke at alpha 110 left no see-through texels: alpha is decorative")
	}
	if laid < 20 {
		t.Errorf("only %d see-through texels for a stroke across a third of the face", laid)
	}

	// And painting over translucent paint mixes with it rather than ignoring
	// it: the picture changes, and it is still not solid.
	if layered.pictures[0].sum == thin.pictures[0].sum {
		t.Error("a second translucent stroke over the first changed nothing")
	}
	if layered.pictures[0].thin == 0 {
		t.Error("painting over a glaze turned the whole thing solid")
	}
}

// Every texel a translucent stroke lays down is see-through — not most of
// them, and not the ones the dabs happened to reach only once.
//
// Freehand paints one segment per pair of pointer samples and consecutive
// segments share their endpoints, so every interior texel is written twice.
// Without a per-stroke record of what has already been laid, those texels
// composite twice and the line comes out blotched at its own joins, with the
// overlaps darker and, at a higher alpha, solid. The evenness itself is
// guarded in internal/paint, where a single texel can be read; here the claim
// is the countable half of it.
func TestEveryTexelOfATranslucentStrokeIsSeeThrough(t *testing.T) {
	stdout, _ := runScript(t, "m7_alpha")
	dumps := parseM7Dumps(t, stdout)
	solid, thin := dumps[1].pictures[0], dumps[3].pictures[0]

	// The two strokes are the same drag at different heights, so the second
	// covers as many texels as the first.
	laid := thin.opaque - solid.opaque
	if laid != solid.opaque {
		t.Fatalf("the translucent stroke covered %d texels against the solid "+
			"stroke's %d, and they are the same drag", laid, solid.opaque)
	}
	if thin.thin != laid {
		t.Errorf("%d of the %d texels the translucent stroke laid came out solid",
			laid-thin.thin, laid)
	}
}

// The bug the user caught in the first cut, reported with a screenshot: "The
// alpha paint is only applying to every other pixel, see the pattern." A
// dither was armed, and the dither was being fed the brush's alpha instead of
// the dab's coverage, so a hard stroke at half alpha landed on half its texels.
func TestADitheredTranslucentStrokeStillCoversItsTexels(t *testing.T) {
	stdout, _ := runScript(t, "m7_alpha")
	dumps := parseM7Dumps(t, stdout)
	plain, dithered := dumps[3].pictures[0], dumps[5].pictures[0]
	beforePlain, beforeDithered := dumps[1].pictures[0], dumps[4].pictures[0]

	// The same drag across the same face, one with a dither armed and one
	// without. A dither decides how a *coverage* is spent, and a hard dab
	// covers its texels completely, so there is nothing for it to spend.
	laidPlain := plain.opaque - beforePlain.opaque
	laidDithered := dithered.opaque - beforeDithered.opaque
	if laidDithered != laidPlain {
		t.Errorf("the dithered stroke laid %d texels against the plain stroke's %d: "+
			"the dither is thinning out the glaze", laidDithered, laidPlain)
	}
	if dithered.thin-beforeDithered.thin != laidDithered {
		t.Error("some of the dithered stroke came out solid")
	}
}
