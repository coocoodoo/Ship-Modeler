package io

import (
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// Exports (SPEC-DATA §5).
//
// An export is the one artefact this program produces that nothing in it will
// ever read back, so it cannot be checked by round-tripping. It is checked
// structurally instead: the counts have to add up, every index has to point at
// something, and every material a face names has to exist. A viewer that
// silently drops half a ship is the failure mode, and it looks like a smaller
// ship rather than an error.

func exportDoc(t *testing.T) *model.Document {
	t.Helper()
	doc := model.NewDocument()
	hull := mesh.Box(geom.Vec3{X: -2, Y: -1, Z: -1}, geom.Vec3{X: 2, Y: 1, Z: 1}, 1)
	shared := &mesh.FacePaint{
		Res:   16,
		Texel: 0.25,
		Frame: geom.FrameFromNormal(geom.Vec3{Y: 1}, geom.Vec3{Y: 1}),
		Img:   image.NewRGBA(image.Rect(0, 0, 4, 4)),
	}
	shared.Img.SetRGBA(1, 1, color.RGBA{R: 255, G: 165, B: 30, A: 255})
	hull.Faces[0].Paint = shared
	hull.Faces[1].Paint = shared

	doc.Bodies = []*model.Body{
		{ID: 1, Name: "Hull", Color: color.RGBA{R: 140, G: 160, B: 175, A: 255},
			Visible: true, Mesh: hull, FaceSeq: uint32(len(hull.Faces))},
		{ID: 2, Name: "Hidden pod", Color: color.RGBA{R: 200, G: 100, B: 100, A: 255},
			Visible: false,
			Mesh:    mesh.Box(geom.Vec3{X: 4, Y: 0, Z: 0}, geom.Vec3{X: 5, Y: 1, Z: 1}, 2)},
	}
	return doc
}

func TestOBJIsStructurallySound(t *testing.T) {
	dir := t.TempDir()
	objPath := filepath.Join(dir, "ship.obj")
	if err := ExportOBJ(objPath, exportDoc(t)); err != nil {
		t.Fatalf("export: %v", err)
	}
	obj := readText(t, objPath)

	var verts, uvs, normals, faces int
	materials := map[string]bool{}
	used := map[string]bool{}
	for _, line := range strings.Split(obj, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "v":
			verts++
			if len(f) != 4 {
				t.Fatalf("a vertex line has %d fields: %q", len(f), line)
			}
		case "vt":
			uvs++
		case "vn":
			normals++
		case "usemtl":
			used[f[1]] = true
		case "f":
			faces++
			if len(f) != 4 {
				t.Fatalf("a face is not a triangle: %q", line)
			}
			for _, ref := range f[1:] {
				checkFaceRef(t, ref, verts, uvs, normals)
			}
		case "mtllib":
			if f[1] != "ship.mtl" {
				t.Errorf("the obj points at %q, want ship.mtl", f[1])
			}
		}
	}
	if verts == 0 || faces == 0 {
		t.Fatalf("the export is empty: %d verts, %d faces", verts, faces)
	}
	// Only what is visible: exporting a hidden body would put geometry into the
	// file that the user cannot see and did not ask for.
	if strings.Contains(obj, "Hidden pod") {
		t.Error("a hidden body was exported")
	}
	// A box is six quads, so twelve triangles.
	if faces != 12 {
		t.Errorf("%d triangles, want 12 for one box", faces)
	}
	if normals == 0 {
		t.Error("no normals were written, so every face will shade smooth")
	}
	if !strings.HasPrefix(obj, "#") {
		t.Error("the file does not start with the header comment naming the axes")
	}
	if !strings.Contains(obj, "Y-up") {
		t.Error("the header does not say which way is up")
	}

	// Every material the obj names has to be in the mtl.
	mtl := readText(t, filepath.Join(dir, "ship.mtl"))
	for _, line := range strings.Split(mtl, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "newmtl" {
			materials[f[1]] = true
		}
	}
	if len(used) == 0 {
		t.Fatal("the obj names no materials at all")
	}
	for name := range used {
		if !materials[name] {
			t.Errorf("the obj uses material %q, which the mtl does not define", name)
		}
	}
	// One for the body colour, one for the picture its two top faces share.
	if len(used) != 2 {
		t.Errorf("the obj uses %d materials (%v), want the body colour and one picture",
			len(used), used)
	}
}

func TestOBJWritesAPictureForEachPaintedMaterial(t *testing.T) {
	dir := t.TempDir()
	if err := ExportOBJ(filepath.Join(dir, "ship.obj"), exportDoc(t)); err != nil {
		t.Fatal(err)
	}
	mtl := readText(t, filepath.Join(dir, "ship.mtl"))
	var maps []string
	for _, line := range strings.Split(mtl, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "map_Kd" {
			maps = append(maps, f[1])
		}
	}
	if len(maps) != 1 {
		t.Fatalf("%d textures referenced, want 1: %v", len(maps), maps)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(maps[0]))); err != nil {
		t.Errorf("the mtl points at %q, which is not there: %v", maps[0], err)
	}
	if !strings.Contains(mtl, "nearest") {
		t.Error("the mtl does not tell the reader to sample these textures nearest")
	}
}

// TestOBJUVsLandInsideTheTexture is what makes the pixels line up in a viewer:
// a texel that maps outside 0..1 is a texel that samples the wrong thing, or
// nothing.
func TestOBJUVsLandInsideTheTexture(t *testing.T) {
	dir := t.TempDir()
	if err := ExportOBJ(filepath.Join(dir, "ship.obj"), exportDoc(t)); err != nil {
		t.Fatal(err)
	}
	obj := readText(t, filepath.Join(dir, "ship.obj"))
	seen := 0
	for _, line := range strings.Split(obj, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != "vt" {
			continue
		}
		seen++
		for _, s := range f[1:] {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				t.Fatalf("unparsable uv %q", s)
			}
			if v < -0.001 || v > 1.001 {
				t.Errorf("uv %v is outside the texture", v)
			}
		}
	}
	if seen == 0 {
		t.Error("no uvs were written, so the paint cannot be placed")
	}
}

func TestSTLIsBinaryAndComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship.stl")
	if err := ExportSTL(path, exportDoc(t)); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 84 {
		t.Fatalf("the file is %d bytes, too short to be an STL", len(data))
	}
	count := binary.LittleEndian.Uint32(data[80:84])
	if count != 12 {
		t.Errorf("the header claims %d triangles, want 12", count)
	}
	if want := 84 + int(count)*50; len(data) != want {
		t.Errorf("the file is %d bytes, want %d for %d triangles", len(data), want, count)
	}
	// A binary STL that opens with "solid" is read as an ASCII one by half the
	// viewers in existence.
	if strings.HasPrefix(strings.ToLower(string(data[:5])), "solid") {
		t.Error("the header starts with \"solid\", which makes readers guess ASCII")
	}

	for i := 0; i < int(count); i++ {
		off := 84 + i*50
		var n [3]float64
		for k := 0; k < 3; k++ {
			bits := binary.LittleEndian.Uint32(data[off+k*4 : off+k*4+4])
			n[k] = float64(math.Float32frombits(bits))
		}
		length := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		if math.Abs(length-1) > 1e-3 {
			t.Fatalf("triangle %d has a normal of length %v", i, length)
		}
	}
}

func TestScaledPNGUpscalesByWholeTexels(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	src.SetRGBA(1, 1, color.RGBA{B: 255, A: 255})

	path := filepath.Join(t.TempDir(), "shot.png")
	if err := WritePNG(path, src, 3); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePNG(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds(); got != image.Rect(0, 0, 6, 6) {
		t.Fatalf("the shot came out %v, want 6x6", got)
	}
	// Nearest, not smooth: a 3x upscale is nine identical texels, or the pixel
	// art has been blurred on the way out (SPEC-DATA §5).
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			if got := img.RGBAAt(x, y); got != (color.RGBA{R: 255, A: 255}) {
				t.Fatalf("(%d,%d) is %v, want the top-left texel unblended", x, y, got)
			}
		}
	}
	if got := img.RGBAAt(5, 5); got != (color.RGBA{B: 255, A: 255}) {
		t.Errorf("the bottom-right texel is %v", got)
	}
}

func TestExportRefusesAnEmptyDocument(t *testing.T) {
	dir := t.TempDir()
	empty := model.NewDocument()
	if err := ExportOBJ(filepath.Join(dir, "a.obj"), empty); err == nil {
		t.Error("exporting nothing reported success")
	}
	if err := ExportSTL(filepath.Join(dir, "a.stl"), empty); err == nil {
		t.Error("exporting nothing reported success")
	}
	// And it does not leave a stub behind for the user to wonder about.
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a refused export left %d file(s) behind", len(entries))
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// checkFaceRef validates one "v/vt/vn" reference of a face line.
func checkFaceRef(t *testing.T, ref string, verts, uvs, normals int) {
	t.Helper()
	parts := strings.Split(ref, "/")
	if len(parts) != 3 {
		t.Fatalf("face reference %q is not v/vt/vn", ref)
	}
	limits := []int{verts, uvs, normals}
	for i, p := range parts {
		if p == "" {
			continue // a face with no uv is allowed to leave the slot empty
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("face reference %q has an unparsable index", ref)
		}
		// OBJ indices are 1-based, and must already have been written.
		if n < 1 || n > limits[i] {
			t.Fatalf("face reference %q points at index %d, and only %d exist so far",
				ref, n, limits[i])
		}
	}
}
