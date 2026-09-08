package io

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestModelExportScalePreservesTexturesAndAttachments(t *testing.T) {
	doc := exportDoc(t)
	doc.Markers = []model.Marker{{Kind: model.MarkerAttachment, Slot: "A", At: geom.Vec3{X: 2, Y: 1}, Dir: geom.Vec3{Z: 1}}}
	p := doc.Bodies[0].Mesh.Faces[0].Paint
	p.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{"roughness": {Bounds: p.Img.Bounds(), Image: p.Img}}}
	before, _ := json.Marshal(doc)
	baseJS, baseBin, baseImages, err := buildGLTF(doc, false, "ship")
	if err != nil {
		t.Fatal(err)
	}
	var base gltfJSON
	if err := json.Unmarshal(baseJS, &base); err != nil {
		t.Fatal(err)
	}
	for _, scale := range []float64{.01, .5, 2} {
		js, bin, images, err := buildGLTF(doc, false, "ship", scale)
		if err != nil {
			t.Fatal(err)
		}
		var got gltfJSON
		if err := json.Unmarshal(js, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(images, baseImages) || !reflect.DeepEqual(got.Materials, base.Materials) {
			t.Fatal("scaling altered paint or PBR maps")
		}
		for mi, m := range got.Meshes {
			for pi, pr := range m.Primitives {
				original := base.Meshes[mi].Primitives[pi]
				for _, pair := range [][2]int{{pr.Attributes.Position, original.Attributes.Position}, {pr.Attributes.Normal, original.Attributes.Normal}} {
					a, orig := got.Accessors[pair[0]], base.Accessors[pair[1]]
					off, oldOff := got.BufferViews[a.BufferView].ByteOffset, base.BufferViews[orig.BufferView].ByteOffset
					for k := 0; k < a.Count*3; k++ {
						v := math.Float32frombits(binary.LittleEndian.Uint32(bin[off+k*4:]))
						want := math.Float32frombits(binary.LittleEndian.Uint32(baseBin[oldOff+k*4:]))
						if pair[0] == pr.Attributes.Position {
							want = float32(float64(want) * scale)
						}
						if math.Abs(float64(v-want)) > 1e-6 {
							t.Fatalf("scale %g: vertex/normal %g want %g", scale, v, want)
						}
					}
				}
				if pr.Attributes.TexCoord != nil {
					a, orig := got.Accessors[*pr.Attributes.TexCoord], base.Accessors[*original.Attributes.TexCoord]
					v, ov := got.BufferViews[a.BufferView], base.BufferViews[orig.BufferView]
					if !bytes.Equal(bin[v.ByteOffset:v.ByteOffset+v.ByteLength], baseBin[ov.ByteOffset:ov.ByteOffset+ov.ByteLength]) {
						t.Fatal("scaling shifted UVs")
					}
				}
			}
		}
		point := got.Nodes[len(got.Nodes)-1]
		if point.Translation == nil || *point.Translation != [3]float64{2 * scale, scale, 0} {
			t.Fatal("attachment did not follow scaled geometry")
		}
		after, _ := json.Marshal(doc)
		if !bytes.Equal(before, after) || doc.Bodies[0].Mesh.Faces[0].Paint != p {
			t.Fatal("export modified the project")
		}
	}
}

func TestScaledExportFiles(t *testing.T) {
	doc := exportDoc(t)
	for ext, write := range map[string]func(string, *model.Document, ...float64) error{".obj": ExportOBJ, ".stl": ExportSTL, ".glb": ExportGLTF, ".gltf": ExportGLTF} {
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ship"+ext)
			if err := write(path, doc, .5); err != nil {
				t.Fatal(err)
			}
			if ext == ".obj" || ext == ".stl" {
				tris, err := ReadMeshFile(path)
				if err != nil {
					t.Fatal(err)
				}
				bounds := geom.Empty()
				for _, tr := range tris {
					for _, v := range []geom.Vec3{tr.A, tr.B, tr.C} {
						bounds = bounds.AddPoint(v)
					}
				}
				if bounds.Size() != (geom.Vec3{X: 2, Y: 1, Z: 1}) {
					t.Fatalf("wrong exported size: %v", bounds.Size())
				}
			} else {
				var gl gltfDoc
				var bin []byte
				if ext == ".glb" {
					gl, bin = readGLB(t, path)
				} else {
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(data, &gl); err != nil {
						t.Fatal(err)
					}
					bin, err = os.ReadFile(filepath.Join(filepath.Dir(path), gl.Buffers[0].URI))
					if err != nil {
						t.Fatal(err)
					}
				}
				checkGLTF(t, gl, len(bin))
				bounds := geom.Empty()
				for _, m := range gl.Meshes {
					for _, p := range m.Primitives {
						a := gl.Accessors[p.Attributes["POSITION"]]
						bounds = bounds.AddPoint(geom.Vec3{X: a.Min[0], Y: a.Min[1], Z: a.Min[2]}).AddPoint(geom.Vec3{X: a.Max[0], Y: a.Max[1], Z: a.Max[2]})
					}
				}
				if bounds.Size() != (geom.Vec3{X: 2, Y: 1, Z: 1}) {
					t.Fatalf("wrong accessor bounds: %v", bounds.Size())
				}
			}
			old, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1)} {
				if err := write(path, doc, bad); err == nil {
					t.Fatal("accepted invalid scale")
				}
				now, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(old, now) {
					t.Fatal("invalid scale overwrote export")
				}
			}
		})
	}
}
