package ui

import (
	"testing"
)

func TestPaletteTransitionsCompleteAndCanBeInterrupted(t *testing.T) {
	defer ResetTheme()
	ApplyPalette("Midnight")
	c := NewContext(nil, 1)
	before := snapshotTheme()
	c.TransitionTheme(func() { ApplyPalette("Ocean") })
	if snapshotTheme() != before {
		t.Fatal("transition jumped before its first frame")
	}
	c.AdvanceAppearance(140)
	mid := snapshotTheme()
	if mid == before {
		t.Fatal("colors did not advance")
	}
	c.TransitionTheme(func() { ApplyPalette("Rose Dark") })
	if snapshotTheme() != mid {
		t.Fatal("interrupted transition jumped")
	}
	c.AdvanceAppearance(280)
	got := snapshotTheme()
	ApplyPalette("Rose Dark")
	if got != snapshotTheme() || c.Animating() {
		t.Fatal("transition failed to reach exact target")
	}
	c.Motion = "slow"
	c.TransitionTheme(func() { ApplyPalette("Indigo") })
	c.AdvanceAppearance(280)
	if !c.Animating() {
		t.Fatal("slow transition finished at normal speed")
	}
	c.Motion = "off"
	c.AdvanceAppearance(0)
	got = snapshotTheme()
	ApplyPalette("Indigo")
	if got != snapshotTheme() || c.Animating() {
		t.Fatal("reduced motion did not finish immediately")
	}
	c.TransitionTheme(func() { ApplyPalette("Carbon") })
	if c.Animating() {
		t.Fatal("reduced motion started an animation")
	}
}

func TestPalettesPreserveModelColorsAndNeutralStage(t *testing.T) {
	defer ResetTheme()
	body, axis := BodyColor(0), ColorAxisX
	for _, dark := range []bool{false, true} {
		palettes := Palettes(dark)
		if len(palettes) != 10 {
			t.Fatalf("got %d palettes", len(palettes))
		}
		mode := "light"
		if dark {
			mode = "dark"
		}
		ApplyAppearance(mode)
		top, bottom := ColorViewportTop, ColorViewportBottom
		for _, p := range palettes {
			if !ApplyPalette(p.Name) || ColorPanel != paletteColor(p.Surface) {
				t.Fatal("palette not applied", p.Name)
			}
			if BodyColor(0) != body || ColorAxisX != axis || ColorViewportTop != top || ColorViewportBottom != bottom {
				t.Fatal("palette altered model or stage", p.Name)
			}
		}
	}
	before := snapshotTheme()
	if ApplyPalette("missing") || before != snapshotTheme() {
		t.Fatal("invalid palette changed theme")
	}
}
