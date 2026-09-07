package io

import (
	"bytes"
	"image/color"
	"modeler/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedLibraryPersistsPaintAndDoesNotEnterExports(t *testing.T) {
	dir := t.TempDir()
	doc := shipDoc(t)
	before, err := BuildGLB(doc)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := SaveLibraryPart(dir, "Wing module", "Wings", doc.Bodies[:1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveLibraryPart(dir, "Wing module", "Wings", doc.Bodies[:1]); err != nil {
		t.Fatal(err)
	}
	// A freshly read catalog and freshly loaded asset model a later session.
	catalog, err := ReadPartLibrary(dir)
	if err != nil || len(catalog) != 2 {
		t.Fatal(catalog, err)
	}
	if catalog[0].ID == catalog[1].ID {
		t.Fatal("same-name save overwrote an existing part")
	}
	bodies, err := LoadLibraryPart(dir, asset)
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Bodies[0].Mesh.Faces[0].Paint
	if len(bodies) != 1 || bodies[0].Mesh.Faces[0].Paint.Img.RGBAAt(2, 1) != p.Img.RGBAAt(2, 1) {
		t.Fatal("library lost paint")
	}
	after, err := BuildGLB(doc)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("saving to library changed export", err)
	}
	for _, ext := range []string{"obj", "stl"} {
		path := filepath.Join(dir, "model."+ext)
		export := ExportOBJ
		if ext == "stl" {
			export = ExportSTL
		}
		if err := export(path, doc); err != nil {
			t.Fatal(err)
		}
		first, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := SaveLibraryPart(dir, "Another part", "Engines", doc.Bodies); err != nil {
			t.Fatal(err)
		}
		if err := export(path, doc); err != nil {
			t.Fatal(err)
		}
		second, _ := os.ReadFile(path)
		if !bytes.Equal(first, second) {
			t.Fatalf("library entered %s export", ext)
		}
	}
	fresh := model.NewDocument()
	cmd := &model.PasteBodies{Sources: bodies}
	if err := cmd.Do(fresh); err != nil {
		t.Fatal(err)
	}
	if len(visibleBodies(fresh)) != 1 {
		t.Fatal("inserted copy was not exportable")
	}
	if _, err := LoadLibraryPart(dir, LibraryPart{ID: "../outside"}); err == nil {
		t.Fatal("unsafe asset ID accepted")
	}
}

func TestUpdateLibraryPartKeepsIdentityAndPublishesCompleteRevision(t *testing.T) {
	dir := t.TempDir()
	doc := shipDoc(t)
	original, err := SaveLibraryPart(dir, "Wing", "Parts", doc.Bodies[:1])
	if err != nil {
		t.Fatal(err)
	}
	edited := model.SnapshotBody(doc.Bodies[0])
	ink := color.RGBA{R: 12, G: 34, B: 56, A: 255}
	edited.Mesh.Faces[0].Paint.Img.SetRGBA(2, 1, ink)
	edited.Color = ink
	updated, err := UpdateLibraryPart(dir, original, "Wing revised", "Wings", []*model.Body{edited})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadPartLibrary(dir)
	if err != nil || len(catalog) != 1 || catalog[0] != updated || updated.ID != original.ID || updated.Revision == "" {
		t.Fatalf("update duplicated or lost catalog entry: %+v, %v", catalog, err)
	}
	fresh, err := LoadLibraryPart(dir, catalog[0])
	if err != nil {
		t.Fatal(err)
	}
	if fresh[0].Color != ink || fresh[0].Mesh.Faces[0].Paint.Img.RGBAAt(2, 1) != ink {
		t.Fatal("revision lost edited paint/colors")
	}
	old, err := LoadLibraryPart(dir, original)
	if err != nil || old[0].Color == ink {
		t.Fatal("previous reader's snapshot was changed")
	}
	if _, err := UpdateLibraryPart(dir, original, "Stale", "Parts", doc.Bodies); err == nil {
		t.Fatal("stale update accepted")
	}
	if _, err := UpdateLibraryPart(dir, updated, "Broken", "Parts", []*model.Body{nil}); err == nil {
		t.Fatal("invalid geometry accepted")
	}
	after, err := ReadPartLibrary(dir)
	if err != nil || len(after) != 1 || after[0] != updated {
		t.Fatal("failed update changed catalog")
	}
	if _, err := LoadLibraryPart(dir, LibraryPart{ID: original.ID, Revision: "../bad"}); err == nil {
		t.Fatal("invalid revision accepted")
	}
}
