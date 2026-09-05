package app

import (
	"path/filepath"
	"testing"

	"modeler/internal/io"
)

func TestAOTogglePersistsAndWorksFromFlatView(t *testing.T) {
	a := bareApp()
	path := filepath.Join(t.TempDir(), "settings.json")
	var err error
	a.Settings, err = io.LoadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	a.ToggleAO()
	if a.Settings.AO != 0 {
		t.Fatal("toggle did not disable AO")
	}
	if err := a.Settings.Save(); err != nil {
		t.Fatal(err)
	}
	a.Settings, err = io.LoadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if a.Settings.AO != 0 {
		t.Fatal("AO off was not remembered")
	}
	a.Settings.FlatShading = true
	a.ToggleAO()
	if a.Settings.AO <= 0 || a.Settings.FlatShading {
		t.Fatal("enabling AO from flat view did not restore visible shading")
	}
}
