package io

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// Exports (SPEC-DATA §5). None of these is ever read back by this program, so
// none of them can be checked by round-tripping — they are checked structurally
// instead, because the way an export fails is a viewer quietly showing a
// smaller ship rather than an error.
//
// Only visible bodies go out. Exporting something the user has hidden would put
// geometry in the file that they cannot see and did not ask for.

// ExportOBJ writes an OBJ, its MTL, and a PNG for every painted material.
//
// The textures land in a `paint/` folder beside the OBJ and are referenced
// relatively, so the three move together.
// flattenPaint composites a face's picture over the body's own colour and
// returns an opaque copy of it.
//
// A face's texture only carries the texels that were painted; everywhere else
// it is transparent, and the viewport shows the body's colour through that
// (SPEC-GEOMETRY §8.2). An exported material has no such shader. It names one
// base colour texture and nothing to blend it with, so a transparent texel
// under an opaque material is black in a conforming renderer — which meant a
// single painted pixel used to blacken the rest of its face on export.
//
// Flattening resolves it in the one place that knows both halves: the picture
// leaves with the hull already behind it, so what the file shows is what the
// viewport showed. It is also what makes translucent paint survive the trip at
// all, since a glaze is only a glaze against something (V-158).
func flattenPaint(img *image.RGBA, under color.RGBA) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i, o := img.PixOffset(x, y), out.PixOffset(x, y)
			a := float64(img.Pix[i+3]) / 255
			mix := func(src, dst uint8) uint8 {
				return uint8(float64(src)*a + float64(dst)*(1-a) + 0.5)
			}
			out.Pix[o+0] = mix(img.Pix[i+0], under.R)
			out.Pix[o+1] = mix(img.Pix[i+1], under.G)
			out.Pix[o+2] = mix(img.Pix[i+2], under.B)
			out.Pix[o+3] = 255
		}
	}
	return out
}

func ExportOBJ(objPath string, doc *model.Document, scales ...float64) error {
	scale, err := modelExportScale(scales)
	if err != nil {
		return err
	}
	bodies := visibleBodies(doc)
	if len(bodies) == 0 {
		return fmt.Errorf("there is nothing visible to export")
	}
	stem := strings.TrimSuffix(filepath.Base(objPath), filepath.Ext(objPath))
	dir := filepath.Dir(objPath)

	obj := new(bytes.Buffer)
	mtl := new(bytes.Buffer)
	textures := map[string]*image.RGBA{}

	fmt.Fprintf(obj, "# %s\n", stem)
	fmt.Fprintf(obj, "# Written by Modeler. Y-up, -Z forward, 1 unit = 1 ship pixel.\n")
	fmt.Fprintf(obj, "# Textures are pixel art: set your renderer to nearest filtering.\n")
	fmt.Fprintf(obj, "mtllib %s.mtl\n", stem)

	fmt.Fprintf(mtl, "# Materials for %s. Sample every map_Kd nearest, not linear:\n", stem)
	fmt.Fprintf(mtl, "# these are pixel-art textures and smoothing them destroys them.\n")

	// OBJ indices are 1-based and run across the whole file, so they are
	// counted here rather than per body.
	var vBase, vtBase, vnBase int
	for _, b := range bodies {
		bodyMat := fmt.Sprintf("body_%d", b.ID)
		fmt.Fprintf(mtl, "\nnewmtl %s\n", bodyMat)
		writeKd(mtl, b.Color)

		paintMat := map[*mesh.FacePaint]string{}
		for _, e := range b.Mesh.PaintTable() {
			if e.Paint == nil || e.Paint.Img == nil {
				continue
			}
			name := fmt.Sprintf("paint_%d", uint64(e.Owner))
			rel := "paint/" + name + ".png"
			paintMat[e.Paint] = name
			textures[rel] = flattenPaint(e.Paint.Img, b.Color)
			fmt.Fprintf(mtl, "\nnewmtl %s\n", name)
			// White base so the texture's own colours come through unchanged.
			fmt.Fprintf(mtl, "Kd 1.000000 1.000000 1.000000\n")
			fmt.Fprintf(mtl, "d 1.000000\nillum 1\n")
			fmt.Fprintf(mtl, "map_Kd %s\n", rel)
		}

		fmt.Fprintf(obj, "\no %s\n", objName(b.Name))
		for _, v := range b.Mesh.Verts {
			v = v.Mul(scale)
			fmt.Fprintf(obj, "v %.6f %.6f %.6f\n", v.X, v.Y, v.Z)
		}

		// One normal per face: these are flat-shaded solids, and a normal per
		// vertex would round the creases off in every viewer that reads them.
		for fi := range b.Mesh.Faces {
			n := b.Mesh.FaceNormal(fi)
			fmt.Fprintf(obj, "vn %.6f %.6f %.6f\n", n.X, n.Y, n.Z)
		}

		// UVs, one per (face, vertex) pair on a painted face.
		uvIndex := map[[2]int]int{}
		uvCount := 0
		for fi := range b.Mesh.Faces {
			p := b.Mesh.Faces[fi].Paint
			if p == nil || p.Img == nil {
				continue
			}
			for _, loop := range b.Mesh.Faces[fi].Loops {
				for _, vi := range loop {
					key := [2]int{fi, vi}
					if _, ok := uvIndex[key]; ok {
						continue
					}
					u, v := textureUV(p, b.Mesh.Verts[vi])
					fmt.Fprintf(obj, "vt %.6f %.6f\n", u, v)
					uvCount++
					uvIndex[key] = vtBase + uvCount
				}
			}
		}

		current := ""
		for _, tri := range b.Mesh.Triangulate() {
			want := bodyMat
			if p := b.Mesh.Faces[tri.Face].Paint; p != nil {
				if name, ok := paintMat[p]; ok {
					want = name
				}
			}
			if want != current {
				fmt.Fprintf(obj, "usemtl %s\n", want)
				current = want
			}
			fmt.Fprintf(obj, "f")
			for _, vi := range [3]int{tri.A, tri.B, tri.C} {
				vt := ""
				if n, ok := uvIndex[[2]int{tri.Face, vi}]; ok {
					vt = fmt.Sprintf("%d", n)
				}
				fmt.Fprintf(obj, " %d/%s/%d", vBase+vi+1, vt, vnBase+tri.Face+1)
			}
			fmt.Fprintf(obj, "\n")
		}

		vBase += len(b.Mesh.Verts)
		vnBase += len(b.Mesh.Faces)
		vtBase += uvCount
	}

	// Everything is built before anything is written, so a failure part way
	// through cannot leave half an export on disk.
	if err := writeFileAtomic(objPath, obj.Bytes()); err != nil {
		return err
	}
	mtlPath := filepath.Join(dir, stem+".mtl")
	if err := writeFileAtomic(mtlPath, mtl.Bytes()); err != nil {
		return err
	}
	for rel, img := range textures {
		buf := new(bytes.Buffer)
		if err := encodePNG(buf, img); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
		if err := writeFileAtomic(filepath.Join(dir, filepath.FromSlash(rel)), buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

// textureUV maps a world point into 0..1 across a face's picture, with V
// flipped: OBJ counts texture rows from the bottom and an image counts them
// from the top.
func textureUV(p *mesh.FacePaint, world geom.Vec3) (float64, float64) {
	b := p.Img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	t := p.UV(world)
	u := (t.X - float64(p.Off.X)) / w
	v := (t.Y - float64(p.Off.Y)) / h
	return clamp01(u), clamp01(1 - v)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func writeKd(w *bytes.Buffer, c interface {
	RGBA() (uint32, uint32, uint32, uint32)
}) {
	r, g, b, _ := c.RGBA()
	fmt.Fprintf(w, "Kd %.6f %.6f %.6f\n", float64(r)/65535, float64(g)/65535, float64(b)/65535)
	fmt.Fprintf(w, "d 1.000000\nillum 1\n")
}

// ExportSTL writes a binary STL. Geometry only — STL carries no colour, which
// the export dialog says out loud (SPEC-DATA §5).
func ExportSTL(path string, doc *model.Document, scales ...float64) error {
	scale, err := modelExportScale(scales)
	if err != nil {
		return err
	}
	bodies := visibleBodies(doc)
	if len(bodies) == 0 {
		return fmt.Errorf("there is nothing visible to export")
	}
	type stlTri struct {
		n       geom.Vec3
		a, b, c geom.Vec3
	}
	var tris []stlTri
	for _, body := range bodies {
		m := body.Mesh
		for _, t := range m.Triangulate() {
			tris = append(tris, stlTri{
				n: m.FaceNormal(t.Face),
				a: m.Verts[t.A], b: m.Verts[t.B], c: m.Verts[t.C],
			})
		}
	}
	if len(tris) == 0 {
		return fmt.Errorf("there is nothing visible to export")
	}

	buf := new(bytes.Buffer)
	// An 80-byte header that must not begin with "solid": half the readers in
	// existence take that as a promise of ASCII and then fail to parse binary.
	var header [80]byte
	copy(header[:], "Modeler binary STL")
	buf.Write(header[:])
	binary.Write(buf, binary.LittleEndian, uint32(len(tris)))

	put := func(v geom.Vec3) {
		binary.Write(buf, binary.LittleEndian, float32(v.X))
		binary.Write(buf, binary.LittleEndian, float32(v.Y))
		binary.Write(buf, binary.LittleEndian, float32(v.Z))
	}
	for _, t := range tris {
		n := t.n
		if l := math.Sqrt(n.X*n.X + n.Y*n.Y + n.Z*n.Z); l > 0 {
			n = geom.Vec3{X: n.X / l, Y: n.Y / l, Z: n.Z / l}
		} else {
			n = geom.Vec3{Z: 1}
		}
		put(n)
		put(t.a.Mul(scale))
		put(t.b.Mul(scale))
		put(t.c.Mul(scale))
		binary.Write(buf, binary.LittleEndian, uint16(0))
	}
	return writeFileAtomic(path, buf.Bytes())
}

// WritePNG writes an image, upscaled by a whole factor with nearest sampling so
// the pixel look survives the export (SPEC-DATA §5).
func WritePNG(path string, img *image.RGBA, scale int) error {
	if img == nil {
		return fmt.Errorf("there is nothing to write")
	}
	if scale < 1 {
		scale = 1
	}
	out := img
	if scale > 1 {
		b := img.Bounds()
		out = image.NewRGBA(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
		for y := 0; y < b.Dy()*scale; y++ {
			for x := 0; x < b.Dx()*scale; x++ {
				out.SetRGBA(x, y, img.RGBAAt(b.Min.X+x/scale, b.Min.Y+y/scale))
			}
		}
	}
	buf := new(bytes.Buffer)
	if err := encodePNG(buf, out); err != nil {
		return fmt.Errorf("write the image: %w", err)
	}
	return writeFileAtomic(path, buf.Bytes())
}

// visibleBodies is what an export contains.
func visibleBodies(doc *model.Document) []*model.Body {
	var out []*model.Body
	if doc == nil {
		return nil
	}
	for _, b := range doc.Bodies {
		if b != nil && b.Visible && b.Mesh != nil && len(b.Mesh.Faces) > 0 {
			out = append(out, b)
		}
	}
	return out
}

// objName makes a body's name safe to use as an OBJ object name, which cannot
// hold spaces.
func objName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "body"
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return '_'
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, name)
}

// ExportFormat names a way out of the program, for the export dialog.
type ExportFormat struct {
	Name      string
	Extension string
	// Note is the caveat the dialog shows, or empty.
	Note string
}

// ExportFormats are the formats offered (SPEC-DATA §5).
func ExportFormats() []ExportFormat {
	return []ExportFormat{
		{Name: "glTF binary", Extension: ".glb",
			Note: "One file with geometry, paint and PBR materials. Keeps pixel textures crisp."},
		{Name: "Modeler compressed project", Extension: ".pxm",
			Note: "One compressed file: complete project, textures, PBR, pins and attachments. No separate folder."},
		{Name: "glTF", Extension: ".gltf",
			Note: "Geometry, paint and PBR materials. Keep the .gltf, .bin and paint folder together."},
		{Name: "Wavefront OBJ", Extension: ".obj",
			Note: "Geometry, colours and paint. Set your renderer to nearest filtering."},
		{Name: "Binary STL", Extension: ".stl",
			Note: "Geometry only — STL carries no colours."},
		{Name: "PNG image", Extension: ".png",
			Note: "The current view, at whole-number scale so the pixels stay square."},
	}
}

// RemoveIfEmpty is used by callers that create a directory for an export that
// then failed, so a refusal leaves nothing behind.
func RemoveIfEmpty(dir string) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		os.Remove(dir)
	}
}
