package io

import (
	"modeler/internal/appearance"
	"testing"
)

func TestEffectsPersistIncludingExplicitFalse(t *testing.T) {
	s := DefaultSettings()
	s.SetPath(tempSettings(t))
	e := appearance.Defaults()
	e.Glow = false
	e.Ripple = false
	e.Gradients = false
	e.Tooltips = false
	e.PathMotion = false
	e.ColorCycle = true
	e.PressEffect = "gelatin"
	e.MotionStyle = "bouncy"
	s.UIEffects = &e
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettingsFrom(s.Path())
	if err != nil || got.UIEffects == nil || *got.UIEffects != e {
		t.Fatalf("effect choices were not preserved: %+v %v", got, err)
	}
	e.PressEffect = "unknown"
	e.MotionStyle = "unknown"
	s.Save()
	got, err = LoadSettingsFrom(s.Path())
	if err != nil || got.UIEffects.PressEffect != "subtle" || got.UIEffects.MotionStyle != "ease" {
		t.Fatal("invalid effects not normalized")
	}
}
