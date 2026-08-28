package io

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempSettings(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "settings.json")
}

func TestSettingsRoundTrip(t *testing.T) {
	path := tempSettings(t)
	s := DefaultSettings()
	s.SetPath(path)
	s.Window = WindowRect{X: 100, Y: 50, Width: 1600, Height: 900}
	s.TreePanelWidth = 260
	s.TreeCollapsed = true
	s.GridStep = 0.25
	s.MSAA = false
	s.AutosaveSeconds = 300
	s.UIScaleOverride = 1.5
	s.Tips.PushPull = 2
	s.AddRecentFile(filepath.Join(t.TempDir(), "ship.ship"))
	s.AddRecentColor(color.RGBA{R: 1, G: 2, B: 3, A: 255})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadSettingsFrom(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Window != s.Window {
		t.Errorf("window = %+v, want %+v", got.Window, s.Window)
	}
	if got.TreePanelWidth != 260 || !got.TreeCollapsed {
		t.Errorf("tree panel state = %d/%v", got.TreePanelWidth, got.TreeCollapsed)
	}
	if got.GridStep != 0.25 || got.MSAA || got.AutosaveSeconds != 300 {
		t.Errorf("scalars = %v/%v/%v", got.GridStep, got.MSAA, got.AutosaveSeconds)
	}
	if got.UIScaleOverride != 1.5 {
		t.Errorf("ui scale override = %v", got.UIScaleOverride)
	}
	if got.Tips.PushPull != 2 {
		t.Errorf("tip counter = %d", got.Tips.PushPull)
	}
	if len(got.RecentFiles) != 1 || len(got.RecentColors) != 1 {
		t.Errorf("recents = %v / %v", got.RecentFiles, got.RecentColors)
	}
}

func TestSettingsFirstRunReturnsDefaults(t *testing.T) {
	s, err := LoadSettingsFrom(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("a missing settings file is not an error: %v", err)
	}
	if s.GridStep != DefaultGridStep || !s.MSAA || s.AutosaveSeconds != DefaultAutosaveSeconds {
		t.Errorf("defaults = %+v", s)
	}
	if s.Window.Valid() {
		t.Error("a first run should have no saved window rect")
	}
}

// TestSettingsToleratesGarbage is the promise of SPEC-DATA §6: preferences
// never stop the app from starting.
func TestSettingsToleratesGarbage(t *testing.T) {
	cases := []string{
		"",
		"{",
		"not json at all",
		`{"gridStep": "banana"}`,
		`[1,2,3]`,
	}
	for _, content := range cases {
		path := tempSettings(t)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := LoadSettingsFrom(path)
		if err == nil {
			t.Errorf("%q parsed without complaint", content)
		}
		if s == nil {
			t.Fatalf("%q returned no settings at all", content)
		}
		if s.GridStep != DefaultGridStep || s.AutosaveSeconds != DefaultAutosaveSeconds {
			t.Errorf("%q did not fall back to defaults: %+v", content, s)
		}
	}
}

// TestSettingsNormalizesOutOfRangeValues covers a hand-edited or stale file.
func TestSettingsNormalizesOutOfRangeValues(t *testing.T) {
	path := tempSettings(t)
	body := `{
	  "gridStep": -5,
	  "autosaveSeconds": 1,
	  "uiScaleOverride": 99,
	  "treePanelWidth": -20,
	  "recentFiles": ["a","b","c","d","e","f","g","h","i","j"]
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.GridStep != DefaultGridStep {
		t.Errorf("grid step = %v", s.GridStep)
	}
	if s.AutosaveSeconds != DefaultAutosaveSeconds {
		t.Errorf("autosave = %v", s.AutosaveSeconds)
	}
	if s.UIScaleOverride != 0 {
		t.Errorf("an absurd ui scale should fall back to following the OS, got %v", s.UIScaleOverride)
	}
	if s.TreePanelWidth != 0 {
		t.Errorf("tree width = %v", s.TreePanelWidth)
	}
	if len(s.RecentFiles) != MaxRecentFiles {
		t.Errorf("recent files = %d, want capped at %d", len(s.RecentFiles), MaxRecentFiles)
	}
}

func TestRecentFilesAreMostRecentFirstAndDeduplicated(t *testing.T) {
	s := DefaultSettings()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.ship")
	b := filepath.Join(dir, "b.ship")

	s.AddRecentFile(a)
	s.AddRecentFile(b)
	if len(s.RecentFiles) != 2 || s.RecentFiles[0] != b {
		t.Fatalf("recents = %v, want b first", s.RecentFiles)
	}
	// Re-opening an older file moves it to the front rather than duplicating it.
	s.AddRecentFile(a)
	if len(s.RecentFiles) != 2 || s.RecentFiles[0] != a {
		t.Fatalf("recents = %v, want a first with no duplicate", s.RecentFiles)
	}
	// The list is capped.
	for i := 0; i < MaxRecentFiles*2; i++ {
		s.AddRecentFile(filepath.Join(dir, string(rune('a'+i))+".ship"))
	}
	if len(s.RecentFiles) != MaxRecentFiles {
		t.Errorf("recents grew to %d", len(s.RecentFiles))
	}
	// An empty path is ignored.
	before := len(s.RecentFiles)
	s.AddRecentFile("")
	if len(s.RecentFiles) != before {
		t.Error("an empty path was added to the recents list")
	}
}

func TestRecentColorsDeduplicate(t *testing.T) {
	s := DefaultSettings()
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	s.AddRecentColor(red)
	s.AddRecentColor(blue)
	s.AddRecentColor(red)
	if len(s.RecentColors) != 2 || s.RecentColors[0] != red {
		t.Fatalf("recent colors = %v", s.RecentColors)
	}
	for i := 0; i < 20; i++ {
		s.AddRecentColor(color.RGBA{R: uint8(i), A: 255})
	}
	if len(s.RecentColors) > 8 {
		t.Errorf("recent colors grew to %d", len(s.RecentColors))
	}
}

// TestSaveIsAtomic checks that saving over an existing file replaces it whole
// and leaves no temp files behind.
func TestSaveIsAtomic(t *testing.T) {
	path := tempSettings(t)
	s := DefaultSettings()
	s.SetPath(path)
	s.GridStep = 1
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s.GridStep = 0.25
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.GridStep != 0.25 {
		t.Errorf("second save did not take effect: %v", got.GridStep)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a temp file was left behind: %s", e.Name())
		}
	}
}

func TestSaveCreatesMissingDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "settings.json")
	s := DefaultSettings()
	s.SetPath(path)
	if err := s.Save(); err != nil {
		t.Fatalf("Save into a missing directory: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not written: %v", err)
	}
}

func TestSettingsPathIsUnderTheAppName(t *testing.T) {
	p, err := SettingsPath()
	if err != nil {
		t.Skipf("no user config dir on this machine: %v", err)
	}
	if !strings.Contains(p, AppName) {
		t.Errorf("settings path %q does not mention %q", p, AppName)
	}
	if filepath.Base(p) != "settings.json" {
		t.Errorf("settings file is named %q", filepath.Base(p))
	}
}

func TestTileSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ConfigDirEnv, dir)
	s := DefaultSettings()
	s.Tiles = TileSettings{
		Path: "tilesets/hull.png", TileW: 16, TileH: 8,
		Margin: 1, Spacing: 2, Selected: 5, Rot: 3, FlipX: true,
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if back.Tiles != s.Tiles {
		t.Errorf("tiles came back %+v, want %+v", back.Tiles, s.Tiles)
	}
}
