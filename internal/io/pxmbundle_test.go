package io

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPXMSingleFileCarriesPaintPBRAndPins(t *testing.T) {
	doc := markedDoc(t)
	b := doc.Bodies[0]
	p := b.Mesh.Faces[0].Paint
	p.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}
	for i, kind := range mesh.MaterialChannels[1:] {
		img := image.NewRGBA(image.Rect(0, 0, 6, 4))
		img.SetRGBA(2, 1, color.RGBA{R: uint8(50 + i*20), G: 128, B: 255, A: 255})
		p.Material.Maps[kind] = &mesh.MaterialMap{Bounds: image.Rect(-1, -1, 5, 3), Image: img}
	}
	tri := b.Mesh.FaceTris(0)[0]
	at := b.Mesh.Verts[tri.A].Add(b.Mesh.Verts[tri.B]).Add(b.Mesh.Verts[tri.C]).Mul(1.0 / 3)
	pin, err := model.AnchorNotePin(b, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	pin.ID, pin.Text, pin.Done = 1, "Keep the copper panel and cyan lighting", true
	doc.NotePins, doc.Seq.NotePin = []model.NotePin{pin}, 1
	dir := t.TempDir()
	path := filepath.Join(dir, "complete.pxm")
	if err := SaveShip(path, doc, p.Img); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "complete.pxm" {
		t.Fatalf("save created companion files/folders: %v, %v", entries, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		t.Fatal("PXM is not a ZIP archive")
	}
	members := zipMembers(t, path)
	for _, name := range []string{"document.json", "thumbnail.png", "game/ship.glb", "game/markers.json", paintMemberName(b.Mesh.PaintTable()[0].Owner)} {
		if members[name] == nil {
			t.Fatalf("missing embedded %s", name)
		}
	}
	var packed, raw uint64
	stored := new(bytes.Buffer)
	zw := zip.NewWriter(stored)
	for _, f := range members {
		if f.Method != zip.Deflate {
			t.Fatal("entry is not compressed")
		}
		packed += f.CompressedSize64
		raw += f.UncompressedSize64
		contents, err := readMember(f)
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if packed >= raw {
		t.Fatal("archive payload did not shrink")
	}
	// Only the single archive travels: no source directory or external assets.
	// Also exercise compatibility with the older uncompressed ZIP encoding.
	for name, contents := range map[string][]byte{"relocated.pxm": data, "legacy.pxm": stored.Bytes()} {
		isolated := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(isolated, contents, 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadShip(isolated)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.Warnings) != 0 {
			t.Fatalf("missing embedded data: %v", loaded.Warnings)
		}
		if len(loaded.Doc.Bodies) != 2 || loaded.Doc.Bodies[1].Visible || len(loaded.Doc.Sketches) != 1 {
			t.Fatal("lost hidden bodies or sketches")
		}
		if !reflect.DeepEqual(loaded.Doc.NotePins, doc.NotePins) || !reflect.DeepEqual(loaded.Doc.Markers, doc.Markers) {
			t.Fatal("lost pins or markers")
		}
		if _, ok := loaded.Doc.NotePins[0].Position(loaded.Doc); !ok {
			t.Fatal("pin lost its anchor")
		}
		q := loaded.Doc.Bodies[0].Mesh.Faces[0].Paint
		if q == nil || !bytes.Equal(q.Img.Pix, p.Img.Pix) {
			t.Fatal("lost base-color pixels")
		}
		for _, kind := range mesh.MaterialChannels[1:] {
			if q.Material == nil || q.Material.Maps[kind] == nil || !reflect.DeepEqual(q.Material.Maps[kind], p.Material.Maps[kind]) {
				t.Fatalf("lost %s map", kind)
			}
		}
	}
}
