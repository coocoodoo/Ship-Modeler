package io

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"modeler/internal/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Library assets live outside documents and export scenes. Only inserting a
// copy makes them model geometry. One metadata file per asset avoids lost
// updates when two modeling sessions save parts at the same time.
type LibraryPart struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Revision string `json:"revision,omitempty"`
}

func PartLibraryDir() (string, error) {
	dir, err := SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "parts"), nil
}
func validPartID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && len(id) == 32
}
func ReadPartLibrary(dir string) ([]LibraryPart, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.part.json"))
	if err != nil {
		return nil, err
	}
	var parts []LibraryPart
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var p LibraryPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("read library entry %s: %w", filepath.Base(path), err)
		}
		if !validPartID(p.ID) || (p.Revision != "" && !validPartID(p.Revision)) || filepath.Base(path) != p.ID+".part.json" {
			return nil, fmt.Errorf("invalid library entry %s", filepath.Base(path))
		}
		parts = append(parts, p)
	}
	sort.Slice(parts, func(i, j int) bool {
		a, b := parts[i], parts[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return parts, nil
}
func SaveLibraryPart(dir, name, category string, bodies []*model.Body) (LibraryPart, error) {
	id, err := newPartID()
	if err != nil {
		return LibraryPart{}, err
	}
	return writeLibraryPart(dir, LibraryPart{ID: id, Name: name, Category: category}, bodies)
}

// UpdateLibraryPart publishes the new geometry and metadata together by
// atomically replacing the catalog entry only after its revision is complete.
// Old snapshots remain readable by sessions that already loaded the catalog.
func UpdateLibraryPart(dir string, expected LibraryPart, name, category string, bodies []*model.Body) (LibraryPart, error) {
	if !validPartID(expected.ID) {
		return LibraryPart{}, fmt.Errorf("invalid library part ID")
	}
	data, err := os.ReadFile(filepath.Join(dir, expected.ID+".part.json"))
	if err != nil {
		return LibraryPart{}, fmt.Errorf("read existing library part: %w", err)
	}
	var current LibraryPart
	if err := json.Unmarshal(data, &current); err != nil {
		return LibraryPart{}, err
	}
	if current != expected {
		return LibraryPart{}, fmt.Errorf("this library part changed in another session; refresh it before updating")
	}
	revision, err := newPartID()
	if err != nil {
		return LibraryPart{}, err
	}
	return writeLibraryPart(dir, LibraryPart{ID: expected.ID, Name: name, Category: category, Revision: revision}, bodies)
}

func newPartID() (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	return hex.EncodeToString(id), nil
}

func libraryGeometryPath(dir string, p LibraryPart) string {
	stem := p.ID
	if p.Revision != "" {
		stem += "." + p.Revision
	}
	return filepath.Join(dir, stem+".ship")
}

func writeLibraryPart(dir string, p LibraryPart, bodies []*model.Body) (LibraryPart, error) {
	name, category := p.Name, p.Category
	name, category = strings.TrimSpace(name), strings.TrimSpace(category)
	if name == "" || len([]rune(name)) > 80 {
		return LibraryPart{}, fmt.Errorf("enter a part name (up to 80 characters)")
	}
	if category == "" {
		category = "Uncategorized"
	}
	if len([]rune(category)) > 64 {
		return LibraryPart{}, fmt.Errorf("category names can be up to 64 characters")
	}
	if len(bodies) == 0 {
		return LibraryPart{}, fmt.Errorf("select a body to save")
	}
	doc := model.NewDocument()
	for _, b := range bodies {
		if b == nil || b.Mesh == nil {
			return LibraryPart{}, fmt.Errorf("body has no geometry")
		}
		doc.Bodies = append(doc.Bodies, model.SnapshotBody(b))
		doc.Seq.Body = max(doc.Seq.Body, b.ID)
	}
	p.Name, p.Category = name, category
	if err := os.MkdirAll(dir, 0700); err != nil {
		return LibraryPart{}, err
	}
	geometryPath := libraryGeometryPath(dir, p)
	if err := SaveShip(geometryPath, doc, nil); err != nil {
		return LibraryPart{}, err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return LibraryPart{}, err
	}
	if err := writeFileAtomic(filepath.Join(dir, p.ID+".part.json"), data); err != nil {
		_ = os.Remove(geometryPath)
		return LibraryPart{}, err
	}
	return p, nil
}
func LoadLibraryPart(dir string, part LibraryPart) ([]*model.Body, error) {
	if !validPartID(part.ID) || (part.Revision != "" && !validPartID(part.Revision)) {
		return nil, fmt.Errorf("invalid library part ID")
	}
	loaded, err := LoadShip(libraryGeometryPath(dir, part))
	if err != nil {
		return nil, err
	}
	if len(loaded.Warnings) > 0 {
		return nil, fmt.Errorf("part could not be loaded intact: %s", strings.Join(loaded.Warnings, "; "))
	}
	return loaded.Doc.Bodies, nil
}
