package io

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// The .ship project container (SPEC-DATA §4):
//
//	manifest.json          what wrote it, and when
//	document.json          the document, geometry and all
//	paint/<faceUID>.png    one per distinct picture, named for the face that
//	                       introduced it
//	thumbnail.png          256x256 of the saved view, for the welcome screen
//
// Two rules govern everything here. The writer never touches the target until
// the new file is whole — a save that fails must leave the last good one
// standing. And the reader never gives up on a file it can partly understand: a
// missing picture costs a face its paint and a warning, not the ship.

// ShipExtension is the project file suffix: .pxm, "pixel model" (the user's
// request, 2026-08-28 — the format their game engine consumes).
const ShipExtension = ".pxm"

// LegacyShipExtension is the suffix the format wore before the rename. Files
// saved under it still open — a rename must never orphan anybody's work — and
// save under the new name from then on.
const LegacyShipExtension = ".ship"

// IsShipFile reports whether a path looks like a project file, either name.
func IsShipFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ShipExtension || ext == LegacyShipExtension
}

// AppName in the manifest, so a file can say what wrote it.
const manifestApp = "modeler"

// Manifest is the small header that identifies a .ship.
type Manifest struct {
	App           string    `json:"app"`
	FormatVersion int       `json:"formatVersion"`
	SavedAt       time.Time `json:"savedAt"`
}

// LoadResult is a document read from disk, plus everything the reader had to
// forgive on the way.
type LoadResult struct {
	Doc *model.Document
	// Thumbnail is the saved view, or nil.
	Thumbnail *image.RGBA
	// ReadOnly is set for a file from a newer build: it opens, and saving over
	// it would throw away whatever this build did not understand.
	ReadOnly bool
	// Warnings are shown to the user as one toast (SPEC-DATA §4).
	Warnings []string
}

// SaveShip writes a document to path, atomically. The thumbnail may be nil.
func SaveShip(shipPath string, doc *model.Document, thumb *image.RGBA) error {
	if doc == nil {
		return fmt.Errorf("there is no document to save")
	}
	data, err := buildShip(doc, thumb)
	if err != nil {
		return err
	}
	return writeFileAtomic(shipPath, data)
}

// buildShip assembles the whole archive in memory.
//
// In memory on purpose: the document has to be complete and valid before the
// file on disk is touched at all, and a ship is a few megabytes at the extreme.
func buildShip(doc *model.Document, thumb *image.RGBA) ([]byte, error) {
	docJSON, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("write the document: %w", err)
	}
	manifest, err := json.MarshalIndent(Manifest{
		App:           manifestApp,
		FormatVersion: model.FormatVersion,
		SavedAt:       time.Now().UTC().Truncate(time.Second),
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("write the manifest: %w", err)
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	add := func(name string, data []byte) error {
		// Every member is STORED, not deflated. The game payload is read by
		// engines outside this program — Iron Drift's loader is ~100 lines of
		// dependency-free Rust because it never has to inflate anything — and
		// what deflate would save is noise: the PNGs and the .glb's textures
		// are already compressed, and the JSON is small.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return fmt.Errorf("add %s: %w", name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		return nil
	}

	if err := add("manifest.json", manifest); err != nil {
		return nil, err
	}
	if err := add("document.json", docJSON); err != nil {
		return nil, err
	}
	for _, b := range doc.Bodies {
		if b == nil {
			return nil, fmt.Errorf("the document holds a body that is not there")
		}
		if b.Mesh == nil {
			continue
		}
		for _, e := range b.Mesh.PaintTable() {
			if e.Paint == nil || e.Paint.Img == nil {
				continue
			}
			png := new(bytes.Buffer)
			if err := encodePNG(png, e.Paint.Img); err != nil {
				return nil, fmt.Errorf("write the paint on face %d: %w", e.Owner, err)
			}
			if err := add(paintMemberName(e.Owner), png.Bytes()); err != nil {
				return nil, err
			}
		}
	}
	if thumb != nil {
		png := new(bytes.Buffer)
		if err := encodePNG(png, thumb); err != nil {
			return nil, fmt.Errorf("write the thumbnail: %w", err)
		}
		if err := add("thumbnail.png", png.Bytes()); err != nil {
			return nil, err
		}
	}

	// The game payload: a render-ready .glb and the orientation markers, under
	// game/ so an engine can take just that folder's two members and ignore
	// everything this program keeps for itself. Only when there is something
	// visible to render — an empty document is not a ship yet, and its file
	// says so by carrying no payload.
	if len(visibleBodies(doc)) > 0 {
		glb, err := BuildGLB(doc)
		if err != nil {
			return nil, fmt.Errorf("build the game model: %w", err)
		}
		if err := add("game/ship.glb", glb); err != nil {
			return nil, err
		}
		markers, err := marshalGameMarkers(doc)
		if err != nil {
			return nil, fmt.Errorf("write the game markers: %w", err)
		}
		if err := add("game/markers.json", markers); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finish the archive: %w", err)
	}
	return buf.Bytes(), nil
}

// LoadShip reads a project file.
func LoadShip(shipPath string) (*LoadResult, error) {
	zr, err := zip.OpenReader(shipPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(shipPath), err)
	}
	defer zr.Close()

	members := map[string]*zip.File{}
	for _, f := range zr.File {
		members[f.Name] = f
	}

	out := &LoadResult{}
	written := 0
	if f := members["manifest.json"]; f != nil {
		var man Manifest
		if data, err := readMember(f); err == nil {
			// A manifest we cannot parse is not worth refusing the file over —
			// the document is the part that matters, and it carries its own
			// version.
			_ = json.Unmarshal(data, &man)
		}
		written = man.FormatVersion
	}

	f := members["document.json"]
	if f == nil {
		return nil, fmt.Errorf("%s has no document in it", filepath.Base(shipPath))
	}
	data, err := readMember(f)
	if err != nil {
		return nil, fmt.Errorf("read the document: %w", err)
	}
	doc := model.NewDocument()
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("read the document: %w", err)
	}
	// Whichever of the two says the higher number is the one to believe: a
	// build that could write a field this one does not know about could have
	// stamped it in either place.
	if doc.FormatVersion > written {
		written = doc.FormatVersion
	}
	if written > model.FormatVersion {
		out.ReadOnly = true
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"This ship was saved by a newer version (format %d, this build reads %d) — "+
				"opened read-only so nothing in it is lost", written, model.FormatVersion))
	}
	if doc.FormatVersion <= 0 {
		doc.FormatVersion = model.FormatVersion
	}
	// A file that predates a field, or one somebody trimmed, must still open.
	repairDocument(doc)

	out.Warnings = append(out.Warnings, attachPaint(doc, members)...)
	if t := members["thumbnail.png"]; t != nil {
		if data, err := readMember(t); err == nil {
			if img, err := decodePNG(data); err == nil {
				out.Thumbnail = img
			}
		}
	}

	doc.DirtySinceSave = false
	out.Doc = doc
	return out, nil
}

// attachPaint hangs the PNGs beside the document back onto their faces.
//
// A picture that is not in the file costs its face the paint rather than
// leaving a FacePaint with no pixels in it — that would be a texture every
// brush stroke, every upload and every export would have to special-case.
func attachPaint(doc *model.Document, members map[string]*zip.File) []string {
	var missing []string
	for _, b := range doc.Bodies {
		if b == nil || b.Mesh == nil {
			continue
		}
		gone := map[*mesh.FacePaint]bool{}
		for _, e := range b.Mesh.PaintTable() {
			f := members[paintMemberName(e.Owner)]
			if f == nil {
				gone[e.Paint] = true
				missing = append(missing, fmt.Sprintf("%s face %d", b.Name, e.Owner.Seq()))
				continue
			}
			data, err := readMember(f)
			if err == nil {
				var img *image.RGBA
				if img, err = decodePNG(data); err == nil {
					e.Paint.Img = img
					continue
				}
			}
			gone[e.Paint] = true
			missing = append(missing, fmt.Sprintf("%s face %d", b.Name, e.Owner.Seq()))
		}
		if len(gone) == 0 {
			continue
		}
		for i := range b.Mesh.Faces {
			if gone[b.Mesh.Faces[i].Paint] {
				b.Mesh.Faces[i].Paint = nil
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []string{fmt.Sprintf("Paint is missing for %s — %s left in the body colour",
		plural(len(missing), "face", "faces"), strings.Join(missing, ", "))}
}

// repairDocument fills in what a tolerant read may have left empty, so the rest
// of the program never has to ask whether a loaded document is well formed.
func repairDocument(doc *model.Document) {
	for _, b := range doc.Bodies {
		if b == nil {
			continue
		}
		if b.Name == "" {
			b.Name = fmt.Sprintf("Body %d", b.ID)
		}
		if b.Color.A == 0 {
			b.Color.A = 255
		}
		// Face identities are the anchor for paint and provenance, so the
		// counter behind them must be at least past every face that exists.
		if b.Mesh != nil {
			for i := range b.Mesh.Faces {
				if seq := b.Mesh.Faces[i].ID.Seq(); seq > b.FaceSeq {
					b.FaceSeq = seq
				}
			}
		}
		if b.ID > doc.Seq.Body {
			doc.Seq.Body = b.ID
		}
	}
	for _, s := range doc.Sketches {
		if s == nil {
			continue
		}
		if s.Name == "" {
			s.Name = fmt.Sprintf("Sketch %d", s.ID)
		}
		if s.ID > doc.Seq.Sketch {
			doc.Seq.Sketch = s.ID
		}
	}
	// Drop the holes rather than carry them: a nil body is not something any
	// caller should have to test for.
	doc.Bodies = compactBodies(doc.Bodies)
	doc.Sketches = compactSketches(doc.Sketches)
}

func compactBodies(in []*model.Body) []*model.Body {
	out := in[:0]
	for _, b := range in {
		if b != nil {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compactSketches(in []*model.Sketch) []*model.Sketch {
	out := in[:0]
	for _, s := range in {
		if s != nil {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// paintMemberName is where a picture lives inside the archive.
func paintMemberName(owner mesh.FaceUID) string {
	return path.Join("paint", fmt.Sprintf("%d", uint64(owner))+".png")
}

func readMember(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(rc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodePNG(w *bytes.Buffer, img *image.RGBA) error {
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	return enc.Encode(w, img)
}

func decodePNG(data []byte) (*image.RGBA, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok && rgba.Bounds().Min == (image.Point{}) {
		return rgba, nil
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out, nil
}

// writeFileAtomic writes beside the target and renames over it, so the file
// that is already there survives anything that goes wrong on the way.
func writeFileAtomic(dst string, data []byte) error {
	if dir := filepath.Dir(dst); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create a temporary file: %w", err)
	}
	name := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(name)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", filepath.Base(dst), err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("flush %s: %w", filepath.Base(dst), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("close %s: %w", filepath.Base(dst), err)
	}
	// Windows will not rename onto an existing file, so the old one steps aside
	// first — and steps back if the rename fails.
	backup := ""
	if _, err := os.Stat(dst); err == nil {
		backup = name + ".prev"
		if err := os.Rename(dst, backup); err != nil {
			os.Remove(name)
			return fmt.Errorf("replace %s: %w", filepath.Base(dst), err)
		}
	}
	if err := os.Rename(name, dst); err != nil {
		if backup != "" {
			os.Rename(backup, dst)
		}
		os.Remove(name)
		return fmt.Errorf("replace %s: %w", filepath.Base(dst), err)
	}
	if backup != "" {
		os.Remove(backup)
	}
	return nil
}

// plural is the small English helper the warnings need.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// marshalJSON and unmarshalJSON are the one place this package encodes small
// records, so the indentation and the tolerance are decided once.
func marshalJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return append(data, '\n'), nil
}

func unmarshalJSON(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// WriteTextFile writes a text file atomically, for the crash log.
func WriteTextFile(path, text string) error {
	return writeFileAtomic(path, []byte(text))
}
