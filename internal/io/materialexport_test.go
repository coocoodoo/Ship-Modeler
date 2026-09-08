package io

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
)

func authoringMaterial(t *testing.T) (*model.Document, *model.Body, *mesh.FacePaint) {
	t.Helper()
	b := &model.Body{ID: 1, Name: "Panel", Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1), Color: color.RGBA{80, 100, 120, 255}}
	d := model.NewDocument()
	d.Bodies = []*model.Body{b}
	for _, kind := range []string{"base_color", "roughness", "normal", "height"} {
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				c := color.RGBA{uint8(x * 60), uint8(y * 60), 150, 255}
				if kind == "normal" {
					c = color.RGBA{128, 128, 255, 255}
				}
				if kind == "base_color" && x == 0 {
					c = color.RGBA{}
				}
				img.SetRGBA(x, y, c)
			}
		}
		cmd := &paint.SetMaterialMap{Body: 1, Face: b.Mesh.Faces[0].ID, Kind: kind, Image: img, Res: 8}
		if err := cmd.Do(d); err != nil {
			t.Fatal(err)
		}
	}
	return d, b, b.Mesh.Faces[0].Paint
}

func TestMaterialAuthoringExportRoundTrip(t *testing.T) {
	d, b, p := authoringMaterial(t)
	pixels := append([]byte(nil), p.Img.Pix...)
	bounds := p.MaterialBounds(b.Mesh, 0)
	wantBase := flattenPaint(p.Img, b.Color)
	for _, kind := range mesh.MaterialChannels {
		path := filepath.Join(t.TempDir(), kind+".png")
		if err := ExportMaterialMap(path, b, 0, p, kind); err != nil {
			t.Fatal(err)
		}
		img, err := ReadMaterialImage(path)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Size() != bounds.Size() {
			t.Fatal("export included margins or changed dimensions")
		}
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				tx, ty := x+bounds.Min.X, y+bounds.Min.Y
				want := p.Material.Sample(kind, float64(tx)+.5, float64(ty)+.5)
				if kind == "base_color" {
					want = wantBase.RGBAAt(tx-p.Off.X, ty-p.Off.Y)
				} else if kind != "normal" {
					want.G, want.B = want.R, want.R
				}
				want.A = 255
				if img.RGBAAt(x, y) != want {
					t.Fatalf("%s: wrong authoring pixel at %d,%d", kind, x, y)
				}
			}
		}
		cmd := &paint.SetMaterialMap{Body: 1, Face: b.Mesh.Faces[0].ID, Kind: kind, Image: img}
		if err = cmd.Do(d); err != nil {
			t.Fatal(err)
		}
		round, err := MaterialExportImage(b, 0, b.Mesh.Faces[0].Paint, kind)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(round.Pix, img.Pix) {
			t.Fatalf("%s shifted during export/reimport", kind)
		}
		cmd.Undo(d)
	}
	if b.Mesh.Faces[0].Paint != p || !bytes.Equal(p.Img.Pix, pixels) || d.DirtySinceSave {
		t.Fatal("export changed the document")
	}
}

func TestMaterialTextureSetIncludesAlignedStarterMaps(t *testing.T) {
	_, b, p := authoringMaterial(t)
	path := filepath.Join(t.TempDir(), "panel.zip")
	if err := ExportMaterialSet(path, b, 0, p); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	if len(files) != 8 {
		t.Fatalf("expected six maps, metadata and instructions; got %d files", len(files))
	}
	for _, kind := range mesh.MaterialChannels {
		f := files[kind+".png"]
		if f == nil {
			t.Fatalf("missing %s", kind)
		}
		data, err := readMember(f)
		if err != nil {
			t.Fatal(err)
		}
		img, err := decodePNG(data)
		if err != nil {
			t.Fatal(err)
		}
		if img.Rect.Size() != p.MaterialBounds(b.Mesh, 0).Size() {
			t.Fatal("maps are not aligned")
		}
		if kind == "specular" || kind == "ao" {
			if img.RGBAAt(0, 0) != mesh.MaterialDefault(kind) {
				t.Fatal("invalid starter map")
			}
		}
	}
	data, err := readMember(files["material.json"])
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		NeutralStarterMaps []string `json:"neutralStarterMaps"`
	}
	if err = json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.NeutralStarterMaps) != 2 || meta.NeutralStarterMaps[0] != "specular" || meta.NeutralStarterMaps[1] != "ao" {
		t.Fatalf("starter maps not identified: %s", data)
	}
}

func TestMaterialExportFailurePreservesExistingFile(t *testing.T) {
	_, b, p := authoringMaterial(t)
	path := filepath.Join(t.TempDir(), "keep.png")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExportMaterialMap(path, b, 0, p, "bad"); err == nil {
		t.Fatal("invalid channel accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "existing" {
		t.Fatal("failed export overwrote file")
	}
}
