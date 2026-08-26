package io

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
)

// Settings are the app preferences that live outside any document
// (SPEC-DATA §6). They are stored as JSON in the user's roaming app data.
//
// The reader is deliberately forgiving: a missing, truncated or garbage file
// yields defaults rather than an error, because losing preferences must never
// stop the app from starting.

// AppName is the folder name under %APPDATA%.
const AppName = "Modeler"

// Settings defaults.
const (
	DefaultAutosaveSeconds = 120
	DefaultGridStep        = 1.0
	MaxRecentFiles         = 8
)

// WindowRect is the saved window position and size.
type WindowRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"w"`
	Height int `json:"h"`
}

// Valid reports whether a saved rect is usable. A zero rect means "never saved".
func (r WindowRect) Valid() bool { return r.Width > 0 && r.Height > 0 }

// TipCounters track how many times a one-off teaching hint has been shown, so
// the app can stop nagging (SPEC-UX §12.5).
type TipCounters struct {
	PushPull int `json:"pushPull"`
}

// Settings is the whole preferences file.
type Settings struct {
	Window WindowRect `json:"window"`

	// UIScaleOverride forces a UI scale instead of following the display.
	// Zero means "follow the OS".
	UIScaleOverride float64 `json:"uiScaleOverride"`

	// TreePanelWidth and TreeCollapsed remember the left panel's state.
	TreePanelWidth int  `json:"treePanelWidth"`
	TreeCollapsed  bool `json:"treeCollapsed"`

	GridStep        float64      `json:"gridStep"`
	MSAA            bool         `json:"msaa"`
	AutosaveSeconds int          `json:"autosaveSeconds"`
	RecentFiles     []string     `json:"recentFiles"`
	CustomPalette   []color.RGBA `json:"customPalette"`
	RecentColors    []color.RGBA `json:"recentColors"`
	Tips            TipCounters  `json:"tips"`

	// path is where this was loaded from, so Save can write it back.
	path string
}

// DefaultSettings returns the settings a first run uses.
func DefaultSettings() *Settings {
	return &Settings{
		GridStep:        DefaultGridStep,
		MSAA:            true,
		AutosaveSeconds: DefaultAutosaveSeconds,
	}
}

// SettingsDir is %APPDATA%\Modeler, or a sensible equivalent elsewhere.
func SettingsDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the settings directory: %w", err)
	}
	return filepath.Join(base, AppName), nil
}

// SettingsPath is the settings file's full path.
func SettingsPath() (string, error) {
	dir, err := SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// LoadSettings reads the settings file. It never fails on bad content: an
// unreadable or malformed file yields defaults, and the returned error is
// advisory so the caller can surface a toast without blocking startup.
func LoadSettings() (*Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		s := DefaultSettings()
		return s, err
	}
	return LoadSettingsFrom(path)
}

// LoadSettingsFrom reads settings from an explicit path, which is what tests use.
func LoadSettingsFrom(path string) (*Settings, error) {
	s := DefaultSettings()
	s.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // first run
		}
		return s, fmt.Errorf("read settings: %w", err)
	}

	// Decode into a copy so a partial parse cannot leave half-applied values.
	parsed := DefaultSettings()
	if err := json.Unmarshal(data, parsed); err != nil {
		return s, fmt.Errorf("settings file is malformed, using defaults: %w", err)
	}
	parsed.path = path
	parsed.normalize()
	return parsed, nil
}

// normalize clamps loaded values into their legal ranges, so a hand-edited or
// stale file cannot put the app into a broken state.
func (s *Settings) normalize() {
	if s.GridStep <= 0 {
		s.GridStep = DefaultGridStep
	}
	if s.AutosaveSeconds < 10 {
		s.AutosaveSeconds = DefaultAutosaveSeconds
	}
	if s.UIScaleOverride != 0 && (s.UIScaleOverride < 0.5 || s.UIScaleOverride > 4) {
		s.UIScaleOverride = 0
	}
	if s.TreePanelWidth < 0 {
		s.TreePanelWidth = 0
	}
	if len(s.RecentFiles) > MaxRecentFiles {
		s.RecentFiles = s.RecentFiles[:MaxRecentFiles]
	}
}

// Path reports where these settings were loaded from or will be saved to.
func (s *Settings) Path() string { return s.path }

// SetPath overrides the save location, for tests and portable installs.
func (s *Settings) SetPath(p string) { s.path = p }

// Save writes the settings atomically: a temp file in the same directory, then
// a rename, so a crash mid-write cannot leave a truncated file behind
// (SPEC-DATA §4's writer rule, applied here too).
func (s *Settings) Save() error {
	if s.path == "" {
		p, err := SettingsPath()
		if err != nil {
			return err
		}
		s.path = p
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, "settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp settings file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close settings: %w", err)
	}
	// Windows will not rename onto an existing file, so clear the way first.
	os.Remove(s.path)
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace settings: %w", err)
	}
	return nil
}

// AddRecentFile pushes a path to the front of the most-recently-used list,
// removing any earlier entry for the same file and capping the length.
func (s *Settings) AddRecentFile(path string) {
	if path == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	out := make([]string, 0, MaxRecentFiles)
	out = append(out, path)
	for _, p := range s.RecentFiles {
		if strings.EqualFold(p, path) {
			continue
		}
		out = append(out, p)
		if len(out) == MaxRecentFiles {
			break
		}
	}
	s.RecentFiles = out
}

// AddRecentColor pushes a colour onto the paint palette's recents list.
func (s *Settings) AddRecentColor(c color.RGBA) {
	const maxRecentColors = 8
	out := make([]color.RGBA, 0, maxRecentColors)
	out = append(out, c)
	for _, p := range s.RecentColors {
		if p == c {
			continue
		}
		out = append(out, p)
		if len(out) == maxRecentColors {
			break
		}
	}
	s.RecentColors = out
}
