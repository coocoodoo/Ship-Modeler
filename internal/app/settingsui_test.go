package app

import (
	"os"
	"path/filepath"
	"testing"

	"modeler/internal/io"
	"modeler/internal/ui"
)

func TestAppearancePersistsAndOverridesLegacyTheme(t *testing.T) {
	defer ui.ResetTheme()
	dir := t.TempDir()
	t.Setenv(io.ConfigDirEnv, dir)
	custom := []byte(`{"panel":"#010203"}`)
	path := filepath.Join(dir, ThemeFileName)
	if err := os.WriteFile(path, custom, 0600); err != nil {
		t.Fatal(err)
	}
	if warning := loadAppearance(""); warning != "" || ui.ColorPanel.R != 1 {
		t.Fatal("legacy custom theme was not preserved")
	}
	a := bareApp()
	for _, name := range []string{"light", "dark", "light"} {
		a.setAppearance(name)
		wantPanel, wantGrid := ui.ColorPanel, ui.ColorGridMinor
		settings, err := io.LoadSettings()
		if err != nil || settings.Theme != name {
			t.Fatalf("saved theme = %q, %v", settings.Theme, err)
		}
		ui.ResetTheme()
		if warning := loadAppearance(settings.Theme); warning != "" {
			t.Fatal(warning)
		}
		if ui.ColorPanel != wantPanel || ui.ColorGridMinor != wantGrid {
			t.Fatal("reopened appearance differs")
		}
		if (ui.ColorPanel.R > 128) != (name == "light") {
			t.Fatal("wrong palette applied")
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(custom) {
		t.Fatal("switching themes changed custom theme.json")
	}
}

func TestUISizeChoicePersists(t *testing.T) {
	t.Setenv(io.ConfigDirEnv, t.TempDir())
	a := bareApp()
	for _, size := range []float64{1.25, 1.5, 2, 1, 0} {
		a.pendingUISize = false
		a.setUISize(size)
		if !a.pendingUISize {
			t.Fatal("size was not queued for the next frame")
		}
		saved, err := io.LoadSettings()
		if err != nil {
			t.Fatal(err)
		}
		if saved.UIScaleOverride != size {
			t.Fatalf("saved size = %v, want %v", saved.UIScaleOverride, size)
		}
	}
	a.pendingUISize = false
	a.setUISize(8)
	if a.pendingUISize || a.Settings.UIScaleOverride != 0 {
		t.Fatal("invalid size changed the preference")
	}
}

func TestPaletteAndMotionPreferencesPersist(t *testing.T) {
	defer ui.ResetTheme()
	t.Setenv(io.ConfigDirEnv, t.TempDir())
	a := bareApp()
	a.setAppearancePalette("ocean dark")
	a.setUIMotion("off")
	s, err := io.LoadSettings()
	if err != nil || s.AppearancePalette != "Ocean Dark" || s.Theme != "dark" || s.UIMotion != "off" {
		t.Fatalf("preferences not saved: %+v %v", s, err)
	}
	a.setAppearancePalette("Sand")
	a.setUIMotion("slow")
	s, err = io.LoadSettings()
	if err != nil || s.AppearancePalette != "Sand" || s.Theme != "light" || s.UIMotion != "slow" {
		t.Fatal("light palette / slow motion not saved")
	}
	a.setAppearancePalette("invalid")
	if a.Settings.AppearancePalette != "Sand" {
		t.Fatal("invalid palette changed preference")
	}
	a.setAppearance("dark")
	s, err = io.LoadSettings()
	if err != nil || s.AppearancePalette != "" || s.Theme != "dark" {
		t.Fatal("mode selection did not clear named palette")
	}
}
