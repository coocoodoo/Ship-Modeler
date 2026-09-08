package io

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// MaterialExportImage produces an unlit, face-fitted authoring image. All
// channels share the base paint's pixel grid, independent of viewport settings.
// Normals intentionally exclude height relief so reimport never applies it twice.
func MaterialExportImage(b *model.Body, fi int, p *mesh.FacePaint, kind string) (*image.RGBA, error) {
	if !mesh.ValidMaterialChannel(kind) {
		return nil, fmt.Errorf("unknown texture type %q", kind)
	}
	if b == nil || b.Mesh == nil || p == nil || p.Img == nil {
		return nil, fmt.Errorf("choose a model face first")
	}
	bounds := p.MaterialBounds(b.Mesh, fi)
	if bounds.Empty() || bounds.Dx() > 4096 || bounds.Dy() > 4096 {
		return nil, fmt.Errorf("export needs a face texture between 1 and 4096 pixels per side; try a lower paint resolution")
	}
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	var base *image.RGBA
	if kind == "base_color" {
		base = flattenPaint(p.Img, b.Color)
	}
	for y := 0; y < out.Rect.Dy(); y++ {
		for x := 0; x < out.Rect.Dx(); x++ {
			tx, ty := x+bounds.Min.X, y+bounds.Min.Y
			var c color.RGBA
			if kind == "base_color" {
				at := image.Pt(tx-p.Off.X, ty-p.Off.Y)
				c = b.Color
				c.A = 255
				if at.In(base.Bounds()) {
					c = base.RGBAAt(at.X, at.Y)
				}
			} else {
				c = p.Material.Sample(kind, float64(tx)+.5, float64(ty)+.5)
				if kind != "normal" {
					c.G, c.B = c.R, c.R
				}
				c.A = 255
			}
			out.SetRGBA(x, y, c)
		}
	}
	return out, nil
}

func ExportMaterialMap(path string, b *model.Body, fi int, p *mesh.FacePaint, kind string) error {
	img, err := MaterialExportImage(b, fi, p, kind)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	if err = encodePNG(buf, img); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes())
}

// ExportMaterialSet writes one portable authoring package atomically. Missing
// maps are explicitly marked neutral starter maps, not inferred from color.
func ExportMaterialSet(path string, b *model.Body, fi int, p *mesh.FacePaint) error {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	add := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	files := map[string]string{}
	var starters []string
	var size image.Point
	for _, kind := range mesh.MaterialChannels {
		img, err := MaterialExportImage(b, fi, p, kind)
		if err != nil {
			return err
		}
		size = img.Rect.Size()
		png := new(bytes.Buffer)
		if err = encodePNG(png, img); err != nil {
			return err
		}
		name := kind + ".png"
		files[kind] = name
		if err = add(name, png.Bytes()); err != nil {
			return err
		}
		if kind != "base_color" && (p.Material == nil || p.Material.Maps[kind] == nil || p.Material.Maps[kind].Image == nil) {
			starters = append(starters, kind)
		}
	}
	meta := map[string]any{"version": 1, "body": b.Name, "faceIndex": fi, "faceID": fmt.Sprint(b.Mesh.Faces[fi].ID), "width": size.X, "height": size.Y, "texelSize": p.Texel, "frame": p.Frame, "texelBounds": p.MaterialBounds(b.Mesh, fi), "files": files, "neutralStarterMaps": starters, "normalConvention": "OpenGL +Y; height is separate"}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err = add("material.json", data); err != nil {
		return err
	}
	if err = add("README.txt", []byte("Use base_color.png as the visual reference for creating matching PBR maps.\nAll six PNGs share the same pixel grid and orientation, with paint margins removed.\nEdit the images without cropping, rotating, or resizing their canvas, then import each into its matching PBR channel on the original face.\nBase color includes the body color underneath paint; viewport lighting is excluded.\nScalar maps are ordinary grayscale PNGs. Normals use OpenGL (+Y) and do not include height relief.\nUnassigned channels are neutral starter images listed in material.json, not maps generated from base color.\n")); err != nil {
		return err
	}
	if err = zw.Close(); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes())
}
