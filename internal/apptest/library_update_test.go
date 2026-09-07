package apptest

import (
	"modeler/internal/io"
	"path/filepath"
	"testing"
)

func TestLibraryUpdateUI(t *testing.T) {
	_, dir := runScript(t, "library_update")
	parts, err := io.ReadPartLibrary(filepath.Join(dir, "parts"))
	if err != nil || len(parts) != 1 {
		t.Fatalf("update duplicated the library entry: %v, %v", parts, err)
	}
	if parts[0].Name != "Refined hull" || parts[0].Revision == "" {
		t.Fatalf("update was not saved: %+v", parts[0])
	}
	bodies, err := io.LoadLibraryPart(filepath.Join(dir, "parts"), parts[0])
	if err != nil || len(bodies) != 1 {
		t.Fatalf("updated body did not reload: %v", err)
	}
	checkGolden(t, "library_update", dir)
}
