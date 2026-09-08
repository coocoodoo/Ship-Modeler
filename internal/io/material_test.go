package io

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
	"os"
	"testing"
)

func TestMaterialPersistenceAndExport(t *testing.T) {
	doc := shipDoc(t)
	p := doc.Bodies[0].Mesh.Faces[0].Paint
	p.Material = &mesh.Material{Maps: map[string]*mesh.MaterialMap{}}
	for _, kind := range mesh.MaterialChannels[1:] {
		img := image.NewRGBA(image.Rect(0, 0, 3, 2))
		img.SetRGBA(1, 1, color.RGBA{80, 100, 200, 255})
		p.Material.Maps[kind] = &mesh.MaterialMap{Bounds: image.Rect(-1, -1, 5, 3), Image: img}
	}
	path := saveTo(t, doc)
	result, err := LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	q := result.Doc.Bodies[0].Mesh.Faces[0].Paint
	for _, kind := range mesh.MaterialChannels[1:] {
		if got := q.Material.Maps[kind]; got == nil || got.Bounds != p.Material.Maps[kind].Bounds || !bytes.Equal(got.Image.Pix, p.Material.Maps[kind].Image.Pix) {
			t.Fatalf("lost %s map", kind)
		}
	}
	if result.Doc.Bodies[0].Mesh.Faces[1].Paint != q {
		t.Fatal("lost shared material")
	}
	again := saveTo(t, result.Doc)
	one, _ := os.ReadFile(path)
	two, _ := os.ReadFile(again)
	if !bytes.Equal(one, two) {
		t.Fatal("material save is not deterministic")
	}
	js, _, external, err := buildGLTF(doc, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	var gl gltfJSON
	if err = json.Unmarshal(js, &gl); err != nil {
		t.Fatal(err)
	}
	var mat *gltfMaterial
	for i := range gl.Materials {
		if gl.Materials[i].NormalTexture != nil {
			mat = &gl.Materials[i]
		}
	}
	if mat == nil || mat.OcclusionTexture == nil || mat.PBR.MetallicRoughnessTexture == nil || mat.Extensions["KHR_materials_specular"] == nil || mat.Extras["modelerHeightTexture"] == nil {
		t.Fatal("export omitted PBR channels")
	}
	ref := mat.PBR.MetallicRoughnessTexture
	img := external[gl.Images[gl.Textures[ref.Index].Source].URI]
	src := p.MaterialRaster("roughness")
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			c := img.RGBAAt(x, y)
			if c.G != src.RGBAAt(x, y).R || c.B != 0 {
				t.Fatal("roughness/metallic packing is wrong")
			}
		}
	}
}
