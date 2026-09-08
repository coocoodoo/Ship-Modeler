package io

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
)

func buildGLTFMaterialMaps(b *gltfBuilder, mat *gltfMaterial, e mesh.PaintEntry, embed bool, external map[string]*image.RGBA) error {
	p := e.Paint
	if p.Material == nil {
		return nil
	}
	for _, kind := range mesh.MaterialChannels {
		if (p.Material.Maps[kind] == nil && !(kind == "normal" && p.Material.Maps["height"] != nil)) || kind == "base_color" {
			continue
		}
		pixels := p.MaterialRaster(kind)
		if kind == "normal" {
			for y := 0; y < pixels.Rect.Dy(); y++ {
				for x := 0; x < pixels.Rect.Dx(); x++ {
					pixels.SetRGBA(x, y, p.Material.SurfaceNormal(float64(x+p.Off.X)+.5, float64(y+p.Off.Y)+.5))
				}
			}
		}
		if kind == "roughness" || kind == "specular" {
			for y := 0; y < pixels.Rect.Dy(); y++ {
				for x := 0; x < pixels.Rect.Dx(); x++ {
					c := pixels.RGBAAt(x, y)
					if kind == "roughness" {
						c = color.RGBA{0, c.R, 0, 255}
					} else {
						c = color.RGBA{0, 0, 0, c.R}
					}
					pixels.SetRGBA(x, y, c)
				}
			}
		}
		img := gltfImage{Name: fmt.Sprintf("paint_%d_%s", e.Owner, kind)}
		if embed {
			buf := new(bytes.Buffer)
			if err := encodePNG(buf, pixels); err != nil {
				return err
			}
			bv := b.view(buf.Bytes(), 0)
			img.BufferView = &bv
			img.MimeType = "image/png"
		} else {
			img.URI = "paint/" + img.Name + ".png"
			external[img.URI] = pixels
		}
		b.doc.Images = append(b.doc.Images, img)
		b.doc.Textures = append(b.doc.Textures, gltfTexture{Sampler: 0, Source: len(b.doc.Images) - 1})
		ref := &gltfTextureRef{Index: len(b.doc.Textures) - 1}
		switch kind {
		case "normal":
			mat.NormalTexture = ref
		case "ao":
			mat.OcclusionTexture = ref
		case "roughness":
			mat.PBR.RoughnessFactor = 1
			mat.PBR.MetallicRoughnessTexture = ref
		case "specular":
			mat.Extensions = map[string]any{"KHR_materials_specular": map[string]any{"specularFactor": 1, "specularTexture": ref}}
			found := false
			for _, s := range b.doc.ExtensionsUsed {
				if s == "KHR_materials_specular" {
					found = true
				}
			}
			if !found {
				b.doc.ExtensionsUsed = append(b.doc.ExtensionsUsed, "KHR_materials_specular")
			}
		case "height":
			mat.Extras = map[string]any{"modelerHeightTexture": ref, "modelerHeightMode": "surface relief; no geometry displacement"}
		}
	}
	return nil
}
