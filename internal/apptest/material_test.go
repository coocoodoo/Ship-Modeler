package apptest

import (
	"archive/zip"
	"encoding/json"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPBRInspectionClicksAndScope(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "testdata/scripts/pbr_materials.json"))
	if err != nil {
		t.Fatal(err)
	}
	var original []map[string]any
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	var ops []map[string]any
	for _, op := range original {
		switch op["op"] {
		case "wheel", "shot", "material.export", "material.export_set":
			continue
		}
		ops = append(ops, op)
	}
	var checks []map[string]any
	if err := json.Unmarshal([]byte(`[
	 {"op":"paint.tool","kind":"fill"}, {"op":"dump"},
	 {"op":"click","at":[780,440]}, {"op":"hover","at":[1100,60]}, {"op":"dump"},
	 {"op":"shot","name":"picked"},
	 {"op":"click","at":[1200,205]}, {"op":"shot","name":"whole"},
	 {"op":"click","at":[1200,178]}, {"op":"shot","name":"face"}, {"op":"dump"}
	]`), &checks); err != nil {
		t.Fatal(err)
	}
	ops = append(ops, checks...)
	dir := t.TempDir()
	data, err = json.Marshal(ops)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "inspection.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exePath, "-headless", "-script", path, "-out", dir)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "MODELER_CONFIG_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspection: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"material body=1 face=2 faceOnly=0 view=0", "material body=1 face=4 faceOnly=0 view=0", "material body=1 face=4 faceOnly=1 view=6"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s\n%s", want, text)
		}
	}
	paints := regexp.MustCompile(`(?m)^facepaint .*`).FindAllString(text, -1)
	if len(paints) != 18 {
		t.Fatalf("expected three snapshots of six textures: %s", text)
	}
	for i := 6; i < len(paints); i++ {
		if paints[i] != paints[i%6] {
			t.Fatal("PBR selection or preview changed paint pixels")
		}
	}
	undo := regexp.MustCompile(`undo=(\d+)`).FindAllString(text, -1)
	if len(undo) != 3 || undo[0] != undo[1] || undo[1] != undo[2] {
		t.Fatal("inspection changed undo history")
	}
	read := func(name string) image.Image {
		f, err := os.Open(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	picked, whole, face := read("picked"), read("whole"), read("face")
	pixel := func(img image.Image, x, y int) color.RGBA { return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA) }
	if pixel(face, 720, 280) != pixel(picked, 720, 280) || pixel(whole, 720, 280) == pixel(picked, 720, 280) {
		t.Fatal("whole/face preview scope did not isolate the map to the selected face")
	}
	if pixel(face, 780, 440) != pixel(whole, 780, 440) || pixel(face, 780, 440) == pixel(picked, 780, 440) {
		t.Fatal("selected face did not show its normal map")
	}
}

func TestPBRMaterialPreviews(t *testing.T) {
	_, dir := runScript(t, "pbr_materials")
	read := func(name string) image.Image {
		t.Helper()
		f, err := os.Open(filepath.Join(dir, "material_"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	expected := map[string]color.RGBA{"base_color": {55, 90, 120, 255}, "specular": {60, 60, 60, 255}, "ao": {60, 60, 60, 255}, "height": {220, 220, 220, 255}, "roughness": {30, 30, 30, 255}, "normal": {64, 128, 238, 255}}
	for kind, want := range expected {
		if got := color.RGBAModel.Convert(read(kind).At(720, 280)).(color.RGBA); got != want {
			t.Fatalf("%s preview: got %v want %v", kind, got, want)
		}
	}
	shaded := read("shaded")
	if color.RGBAModel.Convert(shaded.At(720, 280)).(color.RGBA) == expected["base_color"] {
		t.Fatal("PBR shading did not affect base color")
	}
	read("panel")
	read("export_controls")
	z, err := zip.OpenReader(filepath.Join(repoRoot, "testdata/tmp/pbr-authoring/textures.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 8 {
		t.Fatal("AI texture-set export omitted files")
	}
	f, err := os.Open(filepath.Join(repoRoot, "testdata/tmp/pbr-authoring/base_color.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatalf("base export has wrong dimensions: %v", img.Bounds())
	}
}
