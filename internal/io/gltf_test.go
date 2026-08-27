package io

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"modeler/internal/model"
)

// glTF export (asked for by the user, 2026-08-26).
//
// glTF is the one format here that a game engine will read straight in, which
// makes it the one where "it opened but looks wrong" is the expensive failure.
// The checks are therefore about the things a viewer trusts without asking:
// that every index points somewhere, that the accessor bounds are the real
// bounds, and that the samplers say NEAREST — a pixel-art texture filtered
// smooth is not this program's output.

// gltfDoc is the parsed JSON, kept loose so a missing field reads as a zero
// rather than failing to decode.
type gltfDoc struct {
	Asset struct {
		Version   string `json:"version"`
		Generator string `json:"generator"`
	} `json:"asset"`
	Scene  int                     `json:"scene"`
	Scenes []struct{ Nodes []int } `json:"scenes"`
	Nodes  []struct {
		Mesh *int   `json:"mesh"`
		Name string `json:"name"`
	} `json:"nodes"`
	Meshes []struct {
		Name       string `json:"name"`
		Primitives []struct {
			Attributes map[string]int `json:"attributes"`
			Indices    int            `json:"indices"`
			Material   *int           `json:"material"`
			Mode       *int           `json:"mode"`
		} `json:"primitives"`
	} `json:"meshes"`
	Materials []struct {
		Name string `json:"name"`
		PBR  struct {
			BaseColorFactor  []float64 `json:"baseColorFactor"`
			BaseColorTexture *struct {
				Index    int `json:"index"`
				TexCoord int `json:"texCoord"`
			} `json:"baseColorTexture"`
			MetallicFactor  *float64 `json:"metallicFactor"`
			RoughnessFactor *float64 `json:"roughnessFactor"`
		} `json:"pbrMetallicRoughness"`
	} `json:"materials"`
	Textures []struct {
		Sampler *int `json:"sampler"`
		Source  *int `json:"source"`
	} `json:"textures"`
	Samplers []struct {
		MagFilter int `json:"magFilter"`
		MinFilter int `json:"minFilter"`
	} `json:"samplers"`
	Images []struct {
		URI        string `json:"uri"`
		MimeType   string `json:"mimeType"`
		BufferView *int   `json:"bufferView"`
	} `json:"images"`
	Accessors []struct {
		BufferView    int       `json:"bufferView"`
		ByteOffset    int       `json:"byteOffset"`
		ComponentType int       `json:"componentType"`
		Count         int       `json:"count"`
		Type          string    `json:"type"`
		Min           []float64 `json:"min"`
		Max           []float64 `json:"max"`
	} `json:"accessors"`
	BufferViews []struct {
		Buffer     int  `json:"buffer"`
		ByteOffset int  `json:"byteOffset"`
		ByteLength int  `json:"byteLength"`
		Target     *int `json:"target"`
	} `json:"bufferViews"`
	Buffers []struct {
		ByteLength int    `json:"byteLength"`
		URI        string `json:"uri"`
	} `json:"buffers"`
}

// readGLB splits a .glb into its JSON and BIN chunks, checking the container
// rules on the way (glTF 2.0 §4.4).
func readGLB(t *testing.T, path string) (gltfDoc, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) < 12 {
		t.Fatalf("the file is %d bytes, too short to be a glb", len(data))
	}
	if string(data[0:4]) != "glTF" {
		t.Fatalf("the file does not start with the glTF magic: %q", data[0:4])
	}
	if v := binary.LittleEndian.Uint32(data[4:8]); v != 2 {
		t.Errorf("container version %d, want 2", v)
	}
	if total := binary.LittleEndian.Uint32(data[8:12]); int(total) != len(data) {
		t.Errorf("the header claims %d bytes, the file is %d", total, len(data))
	}

	var doc gltfDoc
	var bin []byte
	off := 12
	seen := 0
	for off+8 <= len(data) {
		length := int(binary.LittleEndian.Uint32(data[off : off+4]))
		kind := string(data[off+4 : off+8])
		if length%4 != 0 {
			t.Errorf("chunk %q is %d bytes, which is not 4-byte aligned", kind, length)
		}
		body := data[off+8 : off+8+length]
		switch kind {
		case "JSON":
			if seen != 0 {
				t.Error("the JSON chunk is not first")
			}
			if err := json.Unmarshal(body, &doc); err != nil {
				t.Fatalf("the JSON chunk does not parse: %v", err)
			}
		case "BIN\x00":
			bin = body
		default:
			t.Errorf("unknown chunk %q", kind)
		}
		seen++
		off += 8 + length
	}
	if off != len(data) {
		t.Errorf("the chunks stop at %d, the file is %d bytes", off, len(data))
	}
	return doc, bin
}

// checkGLTF walks the whole document, checking every index resolves and every
// accessor fits inside the buffer it names.
func checkGLTF(t *testing.T, doc gltfDoc, binLen int) {
	t.Helper()
	if doc.Asset.Version != "2.0" {
		t.Errorf("asset version %q, want 2.0", doc.Asset.Version)
	}
	if doc.Asset.Generator == "" {
		t.Error("the file does not say what wrote it")
	}
	if len(doc.Scenes) == 0 || doc.Scene < 0 || doc.Scene >= len(doc.Scenes) {
		t.Fatalf("scene %d of %d", doc.Scene, len(doc.Scenes))
	}
	for _, n := range doc.Scenes[doc.Scene].Nodes {
		if n < 0 || n >= len(doc.Nodes) {
			t.Fatalf("the scene names node %d of %d", n, len(doc.Nodes))
		}
	}
	for i, bv := range doc.BufferViews {
		if bv.Buffer < 0 || bv.Buffer >= len(doc.Buffers) {
			t.Fatalf("buffer view %d names buffer %d of %d", i, bv.Buffer, len(doc.Buffers))
		}
		if end := bv.ByteOffset + bv.ByteLength; end > doc.Buffers[bv.Buffer].ByteLength {
			t.Fatalf("buffer view %d runs to %d, past the %d-byte buffer",
				i, end, doc.Buffers[bv.Buffer].ByteLength)
		}
	}
	if binLen > 0 && doc.Buffers[0].ByteLength > binLen {
		t.Fatalf("the buffer claims %d bytes, the BIN chunk has %d",
			doc.Buffers[0].ByteLength, binLen)
	}
	for i, a := range doc.Accessors {
		if a.BufferView < 0 || a.BufferView >= len(doc.BufferViews) {
			t.Fatalf("accessor %d names buffer view %d of %d", i, a.BufferView, len(doc.BufferViews))
		}
		if a.Count <= 0 {
			t.Errorf("accessor %d holds nothing", i)
		}
		size := componentSize(t, a.ComponentType) * typeCount(t, a.Type)
		if end := a.ByteOffset + a.Count*size; end > doc.BufferViews[a.BufferView].ByteLength {
			t.Fatalf("accessor %d needs %d bytes of a %d-byte view",
				i, end, doc.BufferViews[a.BufferView].ByteLength)
		}
	}
	for mi, m := range doc.Meshes {
		if len(m.Primitives) == 0 {
			t.Errorf("mesh %d has no primitives", mi)
		}
		for pi, p := range m.Primitives {
			if p.Mode != nil && *p.Mode != 4 {
				t.Errorf("mesh %d primitive %d is mode %d, want triangles", mi, pi, *p.Mode)
			}
			pos, ok := p.Attributes["POSITION"]
			if !ok {
				t.Fatalf("mesh %d primitive %d has no POSITION", mi, pi)
			}
			for name, acc := range p.Attributes {
				if acc < 0 || acc >= len(doc.Accessors) {
					t.Fatalf("attribute %s names accessor %d of %d", name, acc, len(doc.Accessors))
				}
				if doc.Accessors[acc].Count != doc.Accessors[pos].Count {
					t.Errorf("mesh %d primitive %d: %s has %d entries, POSITION has %d",
						mi, pi, name, doc.Accessors[acc].Count, doc.Accessors[pos].Count)
				}
			}
			if p.Indices < 0 || p.Indices >= len(doc.Accessors) {
				t.Fatalf("mesh %d primitive %d names index accessor %d", mi, pi, p.Indices)
			}
			if doc.Accessors[p.Indices].Count%3 != 0 {
				t.Errorf("mesh %d primitive %d has %d indices, not a whole number of triangles",
					mi, pi, doc.Accessors[p.Indices].Count)
			}
			if p.Material != nil && (*p.Material < 0 || *p.Material >= len(doc.Materials)) {
				t.Fatalf("mesh %d primitive %d names material %d of %d",
					mi, pi, *p.Material, len(doc.Materials))
			}
		}
	}
	for i, tex := range doc.Textures {
		if tex.Source == nil || *tex.Source < 0 || *tex.Source >= len(doc.Images) {
			t.Fatalf("texture %d names no valid image", i)
		}
		if tex.Sampler == nil || *tex.Sampler < 0 || *tex.Sampler >= len(doc.Samplers) {
			t.Fatalf("texture %d names no valid sampler", i)
		}
	}
}

func componentSize(t *testing.T, code int) int {
	t.Helper()
	switch code {
	case 5121:
		return 1
	case 5123:
		return 2
	case 5125, 5126:
		return 4
	}
	t.Fatalf("unknown component type %d", code)
	return 0
}

func typeCount(t *testing.T, name string) int {
	t.Helper()
	switch name {
	case "SCALAR":
		return 1
	case "VEC2":
		return 2
	case "VEC3":
		return 3
	case "VEC4":
		return 4
	}
	t.Fatalf("unknown accessor type %q", name)
	return 0
}

func TestGLBIsAValidContainer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship.glb")
	if err := ExportGLTF(path, exportDoc(t)); err != nil {
		t.Fatalf("export: %v", err)
	}
	doc, bin := readGLB(t, path)
	checkGLTF(t, doc, len(bin))

	if len(bin) == 0 {
		t.Fatal("a glb with no binary chunk has no geometry in it")
	}
	// One node per visible body, and the hidden one is not there.
	if len(doc.Nodes) != 1 {
		t.Errorf("%d nodes, want 1 for the single visible body", len(doc.Nodes))
	}
	if doc.Nodes[0].Name != "Hull" {
		t.Errorf("the node is named %q, want Hull", doc.Nodes[0].Name)
	}
	// The images ride inside the container rather than beside it.
	for i, img := range doc.Images {
		if img.BufferView == nil {
			t.Errorf("image %d is external in a glb: %q", i, img.URI)
		}
		if img.MimeType != "image/png" {
			t.Errorf("image %d is %q", i, img.MimeType)
		}
	}
	if len(doc.Buffers) != 1 || doc.Buffers[0].URI != "" {
		t.Errorf("a glb must hold its buffer inside it, got %+v", doc.Buffers)
	}
}

// TestGLTFSamplersAreNearest is the whole point of exporting from a pixel-art
// tool: a viewer that filters these textures smooth has thrown the art away.
func TestGLTFSamplersAreNearest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship.glb")
	if err := ExportGLTF(path, exportDoc(t)); err != nil {
		t.Fatal(err)
	}
	doc, _ := readGLB(t, path)
	if len(doc.Samplers) == 0 {
		t.Fatal("no samplers were written, so the viewer picks the filtering")
	}
	const nearest = 9728
	for i, s := range doc.Samplers {
		if s.MagFilter != nearest || s.MinFilter != nearest {
			t.Errorf("sampler %d filters %d/%d, want %d for both",
				i, s.MagFilter, s.MinFilter, nearest)
		}
	}
}

func TestGLTFPaintsWhatIsPaintedAndColoursTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship.glb")
	if err := ExportGLTF(path, exportDoc(t)); err != nil {
		t.Fatal(err)
	}
	doc, _ := readGLB(t, path)

	textured, flat := 0, 0
	for _, m := range doc.Materials {
		if m.PBR.BaseColorTexture != nil {
			textured++
			continue
		}
		flat++
		if len(m.PBR.BaseColorFactor) != 4 {
			t.Errorf("material %q has no base colour", m.Name)
		}
	}
	// The two top faces share one picture, so one textured material; the rest
	// of the box takes the body's colour.
	if textured != 1 {
		t.Errorf("%d textured materials, want 1", textured)
	}
	if flat != 1 {
		t.Errorf("%d flat materials, want 1", flat)
	}
	// A ship is not a mirror: metallic has to be off or every viewer renders it
	// as chrome.
	for _, m := range doc.Materials {
		if m.PBR.MetallicFactor == nil || *m.PBR.MetallicFactor != 0 {
			t.Errorf("material %q is metallic", m.Name)
		}
	}
	// And the primitives that use the textured material must carry UVs.
	for _, mesh := range doc.Meshes {
		for pi, p := range mesh.Primitives {
			if p.Material == nil {
				continue
			}
			needsUV := doc.Materials[*p.Material].PBR.BaseColorTexture != nil
			_, hasUV := p.Attributes["TEXCOORD_0"]
			if needsUV && !hasUV {
				t.Errorf("primitive %d uses a texture but has no uvs", pi)
			}
		}
	}
}

// TestGLTFPositionBoundsAreTheRealBounds matters because viewers frame the
// camera from them: a wrong min/max puts the model off screen on open.
func TestGLTFPositionBoundsAreTheRealBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship.glb")
	if err := ExportGLTF(path, exportDoc(t)); err != nil {
		t.Fatal(err)
	}
	doc, bin := readGLB(t, path)

	for _, m := range doc.Meshes {
		for _, p := range m.Primitives {
			a := doc.Accessors[p.Attributes["POSITION"]]
			if len(a.Min) != 3 || len(a.Max) != 3 {
				t.Fatal("a POSITION accessor has no bounds, which glTF requires")
			}
			bv := doc.BufferViews[a.BufferView]
			base := bv.ByteOffset + a.ByteOffset
			min := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
			max := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
			for i := 0; i < a.Count; i++ {
				for k := 0; k < 3; k++ {
					off := base + i*12 + k*4
					v := float64(math.Float32frombits(
						binary.LittleEndian.Uint32(bin[off : off+4])))
					min[k] = math.Min(min[k], v)
					max[k] = math.Max(max[k], v)
				}
			}
			for k := 0; k < 3; k++ {
				if math.Abs(a.Min[k]-min[k]) > 1e-6 || math.Abs(a.Max[k]-max[k]) > 1e-6 {
					t.Errorf("axis %d: the file says %v..%v, the data is %v..%v",
						k, a.Min[k], a.Max[k], min[k], max[k])
				}
			}
		}
	}
}

// TestGLTFWritesTheSeparateFormTooCovers the .gltf spelling: JSON, a .bin
// beside it, and the textures as their own PNGs.
func TestGLTFWritesTheSeparateFormToo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ship.gltf")
	if err := ExportGLTF(path, exportDoc(t)); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc gltfDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("the .gltf does not parse: %v", err)
	}

	if len(doc.Buffers) != 1 || doc.Buffers[0].URI != "ship.bin" {
		t.Fatalf("the buffer uri is %+v, want ship.bin", doc.Buffers)
	}
	bin, err := os.ReadFile(filepath.Join(dir, "ship.bin"))
	if err != nil {
		t.Fatalf("the buffer file is missing: %v", err)
	}
	if len(bin) != doc.Buffers[0].ByteLength {
		t.Errorf("the buffer file is %d bytes, the document says %d",
			len(bin), doc.Buffers[0].ByteLength)
	}
	checkGLTF(t, doc, len(bin))

	for i, img := range doc.Images {
		if img.BufferView != nil {
			t.Errorf("image %d is embedded in a .gltf, want a file beside it", i)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(img.URI))); err != nil {
			t.Errorf("image %d points at %q, which is not there", i, img.URI)
		}
	}
}

func TestGLTFRefusesAnEmptyDocument(t *testing.T) {
	dir := t.TempDir()
	if err := ExportGLTF(filepath.Join(dir, "a.glb"), model.NewDocument()); err == nil {
		t.Error("exporting nothing reported success")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a refused export left %d file(s) behind", len(entries))
	}
}
