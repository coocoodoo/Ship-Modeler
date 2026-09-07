package io

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"strings"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// glTF 2.0 export, in both spellings: `.glb` packs everything into one file,
// `.gltf` writes the JSON with a `.bin` and the PNGs beside it.
//
// This is the format a game engine reads straight in, which makes it the one
// where "it opened but looks wrong" costs the most. Two decisions carry that
// weight. Every sampler is NEAREST — a pixel-art texture filtered smooth is not
// this program's output, and glTF is the only format here that can say so in
// the file rather than in a README. And every vertex is emitted per face rather
// than shared, because these are flat-shaded solids: sharing a vertex between
// two faces means sharing its normal, and the crease between them rounds off.

// glTF constants, spelled out rather than left as magic numbers.
const (
	gltfFloat          = 5126
	gltfUnsignedInt    = 5125
	gltfArrayBuffer    = 34962
	gltfElementBuffer  = 34963
	gltfTriangles      = 4
	gltfNearest        = 9728
	gltfRepeat         = 10497
	gltfGeneratorName  = "Modeler"
	gltfChunkAlignment = 4
)

type gltfAsset struct {
	Version   string `json:"version"`
	Generator string `json:"generator"`
}

type gltfScene struct {
	Nodes []int `json:"nodes"`
}

type gltfNode struct {
	Mesh        *int            `json:"mesh,omitempty"`
	Name        string          `json:"name,omitempty"`
	Translation *[3]float64     `json:"translation,omitempty"`
	Rotation    *[4]float64     `json:"rotation,omitempty"`
	Extras      *GameAttachment `json:"extras,omitempty"`
}

type gltfAttributes struct {
	Position int  `json:"POSITION"`
	Normal   int  `json:"NORMAL"`
	TexCoord *int `json:"TEXCOORD_0,omitempty"`
}

type gltfPrimitive struct {
	Attributes gltfAttributes `json:"attributes"`
	Indices    int            `json:"indices"`
	Material   *int           `json:"material,omitempty"`
	Mode       int            `json:"mode"`
}

type gltfMesh struct {
	Name       string          `json:"name,omitempty"`
	Primitives []gltfPrimitive `json:"primitives"`
}

type gltfTextureRef struct {
	Index    int `json:"index"`
	TexCoord int `json:"texCoord"`
}

type gltfPBR struct {
	BaseColorFactor  []float64       `json:"baseColorFactor,omitempty"`
	BaseColorTexture *gltfTextureRef `json:"baseColorTexture,omitempty"`
	MetallicFactor   float64         `json:"metallicFactor"`
	RoughnessFactor  float64         `json:"roughnessFactor"`
}

type gltfMaterial struct {
	Name string  `json:"name,omitempty"`
	PBR  gltfPBR `json:"pbrMetallicRoughness"`
}

type gltfTexture struct {
	Sampler int `json:"sampler"`
	Source  int `json:"source"`
}

type gltfSampler struct {
	MagFilter int `json:"magFilter"`
	MinFilter int `json:"minFilter"`
	WrapS     int `json:"wrapS"`
	WrapT     int `json:"wrapT"`
}

type gltfImage struct {
	URI        string `json:"uri,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	BufferView *int   `json:"bufferView,omitempty"`
	Name       string `json:"name,omitempty"`
}

type gltfAccessor struct {
	BufferView    int       `json:"bufferView"`
	ByteOffset    int       `json:"byteOffset,omitempty"`
	ComponentType int       `json:"componentType"`
	Count         int       `json:"count"`
	Type          string    `json:"type"`
	Min           []float64 `json:"min,omitempty"`
	Max           []float64 `json:"max,omitempty"`
}

type gltfBufferView struct {
	Buffer     int  `json:"buffer"`
	ByteOffset int  `json:"byteOffset"`
	ByteLength int  `json:"byteLength"`
	Target     *int `json:"target,omitempty"`
}

type gltfBuffer struct {
	ByteLength int    `json:"byteLength"`
	URI        string `json:"uri,omitempty"`
}

type gltfJSON struct {
	Asset       gltfAsset        `json:"asset"`
	Scene       int              `json:"scene"`
	Scenes      []gltfScene      `json:"scenes"`
	Nodes       []gltfNode       `json:"nodes"`
	Meshes      []gltfMesh       `json:"meshes"`
	Materials   []gltfMaterial   `json:"materials,omitempty"`
	Textures    []gltfTexture    `json:"textures,omitempty"`
	Samplers    []gltfSampler    `json:"samplers,omitempty"`
	Images      []gltfImage      `json:"images,omitempty"`
	Accessors   []gltfAccessor   `json:"accessors"`
	BufferViews []gltfBufferView `json:"bufferViews"`
	Buffers     []gltfBuffer     `json:"buffers"`
}

// gltfBuilder accumulates the binary buffer and the views into it.
type gltfBuilder struct {
	doc gltfJSON
	bin bytes.Buffer
}

// view appends bytes to the buffer and returns the index of a view over them.
//
// Everything is padded to four bytes: accessors have alignment rules, and the
// GLB container has its own, and one rule covering both is easier to keep than
// two that nearly agree.
func (b *gltfBuilder) view(data []byte, target int) int {
	b.pad()
	off := b.bin.Len()
	b.bin.Write(data)
	idx := len(b.doc.BufferViews)
	v := gltfBufferView{Buffer: 0, ByteOffset: off, ByteLength: len(data)}
	if target != 0 {
		v.Target = &target
	}
	b.doc.BufferViews = append(b.doc.BufferViews, v)
	return idx
}

func (b *gltfBuilder) pad() {
	for b.bin.Len()%gltfChunkAlignment != 0 {
		b.bin.WriteByte(0)
	}
}

func (b *gltfBuilder) accessor(a gltfAccessor) int {
	b.doc.Accessors = append(b.doc.Accessors, a)
	return len(b.doc.Accessors) - 1
}

// ExportGLTF writes the scene as glTF 2.0. The extension decides the spelling:
// `.glb` is one self-contained file, anything else writes JSON with a `.bin`
// and the textures beside it.
func ExportGLTF(outPath string, doc *model.Document) error {
	binary := strings.EqualFold(filepath.Ext(outPath), ".glb")
	stem := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	jsonData, bin, external, err := buildGLTF(doc, binary, stem)
	if err != nil {
		return err
	}
	dir := filepath.Dir(outPath)
	if binary {
		return writeFileAtomic(outPath, packGLB(jsonData, bin))
	}
	if err := writeFileAtomic(outPath, jsonData); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, stem+".bin"), bin); err != nil {
		return err
	}
	for rel, img := range external {
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

// BuildGLB renders the document's visible bodies into a single in-memory
// .glb — textures embedded, nothing on disk. It is what a .pxm carries as its
// game payload: the engine loads exactly the bytes the glTF export would have
// produced, and the two can never drift apart because they are one builder.
func BuildGLB(doc *model.Document) ([]byte, error) {
	jsonData, bin, _, err := buildGLTF(doc, true, "ship")
	if err != nil {
		return nil, err
	}
	return packGLB(jsonData, bin), nil
}

// buildGLTF assembles the document into glTF parts: the JSON, the binary
// buffer, and — for the text flavour only — the external images to write
// beside it.
func buildGLTF(doc *model.Document, binary bool, stem string) ([]byte, []byte, map[string]*image.RGBA, error) {
	bodies := visibleBodies(doc)
	if len(bodies) == 0 {
		return nil, nil, nil, fmt.Errorf("there is nothing visible to export")
	}

	b := &gltfBuilder{}
	b.doc.Asset = gltfAsset{Version: "2.0", Generator: gltfGeneratorName}

	// One sampler for everything, and it is NEAREST. There is no case in this
	// program where smoothing a texture is the right answer.
	b.doc.Samplers = []gltfSampler{{
		MagFilter: gltfNearest, MinFilter: gltfNearest,
		WrapS: gltfRepeat, WrapT: gltfRepeat,
	}}

	// External images are written after the document is known to be sound.
	external := map[string]*image.RGBA{}

	for _, body := range bodies {
		node, err := buildGLTFBody(b, body, binary, external)
		if err != nil {
			return nil, nil, nil, err
		}
		b.doc.Nodes = append(b.doc.Nodes, node)
	}

	for _, attachment := range BuildGameMarkers(doc).Attachments {
		point := attachment
		rotation := attachmentRotation(point.Dir)
		b.doc.Nodes = append(b.doc.Nodes, gltfNode{
			Name: point.Name, Translation: &point.At, Rotation: &rotation, Extras: &point,
		})
	}
	nodes := make([]int, len(b.doc.Nodes))
	for i := range nodes {
		nodes[i] = i
	}
	b.doc.Scenes = []gltfScene{{Nodes: nodes}}
	b.doc.Scene = 0
	b.pad()
	b.doc.Buffers = []gltfBuffer{{ByteLength: b.bin.Len()}}
	if !binary {
		b.doc.Buffers[0].URI = stem + ".bin"
	}

	jsonData, err := json.Marshal(b.doc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write the glTF document: %w", err)
	}
	return jsonData, b.bin.Bytes(), external, nil
}

// buildGLTFBody turns one body into a mesh, its materials and a node.
func buildGLTFBody(b *gltfBuilder, body *model.Body, embed bool,
	external map[string]*image.RGBA) (gltfNode, error) {

	m := body.Mesh

	// The body's own colour, used by every face with no picture on it.
	bodyMat := len(b.doc.Materials)
	b.doc.Materials = append(b.doc.Materials, gltfMaterial{
		Name: fmt.Sprintf("%s colour", body.Name),
		PBR: gltfPBR{
			BaseColorFactor: rgbaFactor(body.Color),
			MetallicFactor:  0,
			RoughnessFactor: 0.9,
		},
	})

	// One material per distinct picture, so fragments that share a picture
	// share a material too and the file has one copy of the texture.
	paintMat := map[*mesh.FacePaint]int{}
	for _, e := range m.PaintTable() {
		if e.Paint == nil || e.Paint.Img == nil {
			continue
		}
		img := gltfImage{Name: fmt.Sprintf("paint_%d", uint64(e.Owner))}
		// The hull goes into the picture rather than under it: the material
		// below is opaque and has nothing to blend an unpainted texel with.
		flat := flattenPaint(e.Paint.Img, body.Color)
		if embed {
			buf := new(bytes.Buffer)
			if err := encodePNG(buf, flat); err != nil {
				return gltfNode{}, fmt.Errorf("write the paint on face %d: %w", e.Owner, err)
			}
			bv := b.view(buf.Bytes(), 0)
			img.BufferView = &bv
			img.MimeType = "image/png"
		} else {
			rel := "paint/" + img.Name + ".png"
			img.URI = rel
			external[rel] = flat
		}
		b.doc.Images = append(b.doc.Images, img)
		b.doc.Textures = append(b.doc.Textures, gltfTexture{
			Sampler: 0, Source: len(b.doc.Images) - 1,
		})
		ref := gltfTextureRef{Index: len(b.doc.Textures) - 1}
		b.doc.Materials = append(b.doc.Materials, gltfMaterial{
			Name: img.Name,
			PBR: gltfPBR{
				// White base so the picture's own colours come through.
				BaseColorFactor:  []float64{1, 1, 1, 1},
				BaseColorTexture: &ref,
				MetallicFactor:   0,
				RoughnessFactor:  0.9,
			},
		})
		paintMat[e.Paint] = len(b.doc.Materials) - 1
	}

	// Group the triangles by the material they will use: a primitive is one
	// material, so the faces have to be sorted into buckets before anything is
	// written. The order is first appearance, so the file is the same every
	// time it is written from the same document.
	type bucket struct {
		material int
		paint    *mesh.FacePaint
		tris     []mesh.Tri
	}
	var buckets []*bucket
	byMaterial := map[int]*bucket{}
	for _, tri := range m.Triangulate() {
		mat := bodyMat
		p := m.Faces[tri.Face].Paint
		if idx, ok := paintMat[p]; ok {
			mat = idx
		} else {
			p = nil
		}
		bk := byMaterial[mat]
		if bk == nil {
			bk = &bucket{material: mat, paint: p}
			byMaterial[mat] = bk
			buckets = append(buckets, bk)
		}
		bk.tris = append(bk.tris, tri)
	}

	var prims []gltfPrimitive
	for _, bk := range buckets {
		prim, err := buildGLTFPrimitive(b, m, bk.tris, bk.material, bk.paint)
		if err != nil {
			return gltfNode{}, err
		}
		prims = append(prims, prim)
	}
	b.doc.Meshes = append(b.doc.Meshes, gltfMesh{Name: body.Name, Primitives: prims})
	meshIndex := len(b.doc.Meshes) - 1
	return gltfNode{Mesh: &meshIndex, Name: body.Name}, nil
}

// Attachment nodes face outward along local +Z. The normal supplies the
// shortest rotation from +Z; authored metadata also exposes that normal.
func attachmentRotation(dir [3]float64) [4]float64 {
	if dir[2] < -1+1e-12 {
		return [4]float64{1, 0, 0, 0}
	}
	q := [4]float64{-dir[1], dir[0], 0, 1 + dir[2]}
	n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[3]*q[3])
	if n == 0 {
		return [4]float64{0, 0, 0, 1}
	}
	for i := range q {
		q[i] /= n
	}
	return q
}

// buildGLTFPrimitive writes one material's triangles.
//
// Vertices are emitted per triangle corner rather than shared. These are
// flat-shaded solids: a shared vertex shares its normal, and every crease in
// the ship would round off in the viewer.
func buildGLTFPrimitive(b *gltfBuilder, m *mesh.Mesh, tris []mesh.Tri,
	material int, paint *mesh.FacePaint) (gltfPrimitive, error) {

	count := len(tris) * 3
	pos := new(bytes.Buffer)
	nrm := new(bytes.Buffer)
	uvs := new(bytes.Buffer)
	idx := new(bytes.Buffer)

	min := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	max := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}

	next := uint32(0)
	for _, t := range tris {
		n := m.FaceNormal(t.Face)
		for _, vi := range [3]int{t.A, t.B, t.C} {
			v := m.Verts[vi]
			putVec3(pos, v)
			putVec3(nrm, n)
			if paint != nil {
				u, w := gltfUV(paint, v)
				putFloat(uvs, u)
				putFloat(uvs, w)
			}
			for k, c := range [3]float64{v.X, v.Y, v.Z} {
				min[k] = math.Min(min[k], float64(float32(c)))
				max[k] = math.Max(max[k], float64(float32(c)))
			}
			binary.Write(idx, binary.LittleEndian, next)
			next++
		}
	}

	posAcc := b.accessor(gltfAccessor{
		BufferView:    b.view(pos.Bytes(), gltfArrayBuffer),
		ComponentType: gltfFloat, Count: count, Type: "VEC3",
		Min: min[:], Max: max[:],
	})
	nrmAcc := b.accessor(gltfAccessor{
		BufferView:    b.view(nrm.Bytes(), gltfArrayBuffer),
		ComponentType: gltfFloat, Count: count, Type: "VEC3",
	})
	attrs := gltfAttributes{Position: posAcc, Normal: nrmAcc}
	if paint != nil {
		uvAcc := b.accessor(gltfAccessor{
			BufferView:    b.view(uvs.Bytes(), gltfArrayBuffer),
			ComponentType: gltfFloat, Count: count, Type: "VEC2",
		})
		attrs.TexCoord = &uvAcc
	}
	idxAcc := b.accessor(gltfAccessor{
		BufferView:    b.view(idx.Bytes(), gltfElementBuffer),
		ComponentType: gltfUnsignedInt, Count: count, Type: "SCALAR",
	})

	mat := material
	return gltfPrimitive{
		Attributes: attrs,
		Indices:    idxAcc,
		Material:   &mat,
		Mode:       gltfTriangles,
	}, nil
}

// gltfUV maps a world point into the picture. glTF counts texture rows from the
// top, the same way an image does, so unlike OBJ there is nothing to flip.
func gltfUV(p *mesh.FacePaint, world geom.Vec3) (float64, float64) {
	b := p.Img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	t := p.UV(world)
	return clamp01((t.X - float64(p.Off.X)) / w), clamp01((t.Y - float64(p.Off.Y)) / h)
}

func putVec3(w *bytes.Buffer, v geom.Vec3) {
	putFloat(w, v.X)
	putFloat(w, v.Y)
	putFloat(w, v.Z)
}

func putFloat(w *bytes.Buffer, v float64) {
	binary.Write(w, binary.LittleEndian, float32(v))
}

// rgbaFactor turns a colour into glTF's linear 0..1 base colour.
func rgbaFactor(c interface {
	RGBA() (uint32, uint32, uint32, uint32)
}) []float64 {
	r, g, b, a := c.RGBA()
	return []float64{
		srgbToLinear(float64(r) / 65535),
		srgbToLinear(float64(g) / 65535),
		srgbToLinear(float64(b) / 65535),
		float64(a) / 65535,
	}
}

// srgbToLinear converts a colour the way glTF expects base colour factors,
// which are linear while the palette and the textures are sRGB.
func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// packGLB wraps the JSON and the buffer in the GLB container (glTF 2.0 §4.4).
func packGLB(jsonData, binData []byte) []byte {
	// Both chunks are padded to four bytes — the JSON with spaces so it still
	// parses, the buffer with zeros.
	for len(jsonData)%gltfChunkAlignment != 0 {
		jsonData = append(jsonData, ' ')
	}
	for len(binData)%gltfChunkAlignment != 0 {
		binData = append(binData, 0)
	}
	total := 12 + 8 + len(jsonData)
	if len(binData) > 0 {
		total += 8 + len(binData)
	}

	out := new(bytes.Buffer)
	out.WriteString("glTF")
	binary.Write(out, binary.LittleEndian, uint32(2))
	binary.Write(out, binary.LittleEndian, uint32(total))

	binary.Write(out, binary.LittleEndian, uint32(len(jsonData)))
	out.WriteString("JSON")
	out.Write(jsonData)

	if len(binData) > 0 {
		binary.Write(out, binary.LittleEndian, uint32(len(binData)))
		out.WriteString("BIN\x00")
		out.Write(binData)
	}
	return out.Bytes()
}
