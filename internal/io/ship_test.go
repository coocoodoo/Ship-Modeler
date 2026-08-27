package io

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// The .ship container (SPEC-DATA §4).
//
// Two properties matter more than the rest. A save of a document that was just
// loaded has to produce the same bytes, or every save shows as a change to
// whatever the user keeps their ships in. And the reader has to survive
// anything: a truncated file, a file from a newer build, a file somebody edited
// by hand. Losing work to a malformed read is the one failure this program is
// not allowed to have.

// shipDoc builds a document with a bit of everything: two bodies, a sketch, a
// camera, and two faces sharing one picture the way a cut leaves them.
func shipDoc(t *testing.T) *model.Document {
	t.Helper()
	doc := model.NewDocument()
	doc.Planes[1].Visible = false
	doc.Seq.Body, doc.Seq.Sketch = 2, 1

	hull := mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -2}, geom.Vec3{X: 4, Y: 1, Z: 2}, 1)
	shared := &mesh.FacePaint{
		Res:   32,
		Texel: 0.25,
		Frame: geom.FrameFromNormal(geom.Vec3{Y: 1}, geom.Vec3{Y: 1}),
		Img:   image.NewRGBA(image.Rect(0, 0, 6, 4)),
		Off:   image.Point{X: -1, Y: -1},
	}
	shared.Img.SetRGBA(2, 1, color.RGBA{R: 242, G: 84, B: 45, A: 255})
	shared.Img.SetRGBA(3, 2, color.RGBA{R: 33, G: 231, B: 231, A: 255})
	hull.Faces[0].Paint = shared
	hull.Faces[1].Paint = shared

	doc.Bodies = []*model.Body{
		{ID: 1, Name: "Hull", Color: color.RGBA{R: 142, G: 163, B: 176, A: 255},
			Visible: true, Mesh: hull, FaceSeq: uint32(len(hull.Faces))},
		{ID: 2, Name: "Pod", Color: color.RGBA{R: 176, G: 142, B: 142, A: 255},
			Visible: false,
			Mesh:    mesh.Box(geom.Vec3{X: 6, Y: -1, Z: -1}, geom.Vec3{X: 8, Y: 1, Z: 1}, 2),
			FaceSeq: 6},
	}
	doc.Sketches = []*model.Sketch{{
		ID: 1, Name: "Sketch 1", Visible: true, Plane: geom.PlaneFront,
		Entities: []model.Entity{
			model.NewRect(geom.Vec2i{X: 0, Y: 0}, geom.Vec2i{X: 512, Y: 256}),
			model.NewCircle(geom.Vec2i{X: 256, Y: 128}, 64, 16),
		},
		Consumed: true,
	}}
	doc.Camera = model.CameraState{
		Target: [3]float64{1, 2, 3}, Azimuth: 45, Elevation: 30,
		Dist: 40, OrthoScale: 24, Perspective: true,
	}
	doc.Features = []model.FeatureRec{{
		Kind: "Extrude",
		Time: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
	}}
	return doc
}

func saveTo(t *testing.T, doc *model.Document) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ship.ship")
	if err := SaveShip(path, doc, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

func TestShipRoundTripsTheWholeDocument(t *testing.T) {
	src := shipDoc(t)
	res, err := LoadShip(saveTo(t, src))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("a clean file loaded with warnings: %q", res.Warnings)
	}
	got := res.Doc

	if got.FormatVersion != model.FormatVersion {
		t.Errorf("format version %d, want %d", got.FormatVersion, model.FormatVersion)
	}
	if got.Seq != src.Seq {
		t.Errorf("sequences came back %+v, want %+v", got.Seq, src.Seq)
	}
	if got.Planes != src.Planes {
		t.Errorf("plane visibility came back %+v, want %+v", got.Planes, src.Planes)
	}
	if got.Camera != src.Camera {
		t.Errorf("camera came back %+v, want %+v", got.Camera, src.Camera)
	}
	if len(got.Bodies) != 2 || len(got.Sketches) != 1 {
		t.Fatalf("%d bodies and %d sketches came back", len(got.Bodies), len(got.Sketches))
	}
	for i, b := range got.Bodies {
		want := src.Bodies[i]
		if b.ID != want.ID || b.Name != want.Name || b.Color != want.Color ||
			b.Visible != want.Visible || b.FaceSeq != want.FaceSeq {
			t.Errorf("body %d came back as %+v", i, b)
		}
		if b.Mesh == nil || len(b.Mesh.Verts) != len(want.Mesh.Verts) {
			t.Errorf("body %d lost its mesh", i)
		}
	}
	sk := got.Sketches[0]
	if sk.Name != "Sketch 1" || !sk.Consumed || len(sk.Entities) != 2 {
		t.Errorf("the sketch came back as %+v", sk)
	}
	if sk.Entities[1].R != 64 || sk.Entities[1].Segs != 16 {
		t.Errorf("the circle came back as %+v", sk.Entities[1])
	}
	if len(got.Features) != 1 || got.Features[0].Kind != "Extrude" {
		t.Errorf("the feature log came back as %+v", got.Features)
	}
	if got.DirtySinceSave {
		t.Error("a freshly loaded document is not unsaved work")
	}
}

// TestSavingALoadedShipIsByteIdentical is the invariant of SPEC-DATA §4.
func TestSavingALoadedShipIsByteIdentical(t *testing.T) {
	first := saveTo(t, shipDoc(t))
	res, err := LoadShip(first)
	if err != nil {
		t.Fatal(err)
	}
	second := saveTo(t, res.Doc)

	a, b := zipMember(t, first, "document.json"), zipMember(t, second, "document.json")
	if !bytes.Equal(a, b) {
		t.Errorf("document.json changed across a load and save:\n first: %s\nsecond: %s", a, b)
	}
}

func TestPaintSurvivesTheRoundTripAndStaysShared(t *testing.T) {
	src := shipDoc(t)
	res, err := LoadShip(saveTo(t, src))
	if err != nil {
		t.Fatal(err)
	}
	m := res.Doc.Bodies[0].Mesh
	p := m.Faces[0].Paint
	if p == nil {
		t.Fatal("the painted face came back unpainted")
	}
	if p != m.Faces[1].Paint {
		t.Error("faces that shared a picture came back with a copy each")
	}
	if p.Img == nil {
		t.Fatal("the picture came back without its pixels")
	}
	if got := p.Img.Bounds(); got != image.Rect(0, 0, 6, 4) {
		t.Errorf("the image came back %v, want 6x4", got)
	}
	if got := p.Img.RGBAAt(2, 1); got != (color.RGBA{R: 242, G: 84, B: 45, A: 255}) {
		t.Errorf("a painted texel came back as %v", got)
	}
	if got := p.Img.RGBAAt(0, 0); got.A != 0 {
		t.Errorf("an unpainted texel came back as %v, want transparent", got)
	}
	if p.Res != 32 || p.Texel != 0.25 || p.Off != (image.Point{X: -1, Y: -1}) {
		t.Errorf("the mapping came back as res=%d texel=%v off=%v", p.Res, p.Texel, p.Off)
	}
}

func TestAMissingPaintFileLeavesTheFaceBare(t *testing.T) {
	path := saveTo(t, shipDoc(t))
	stripMember(t, path, func(name string) bool { return strings.HasPrefix(name, "paint/") })

	res, err := LoadShip(path)
	if err != nil {
		t.Fatalf("a file with a missing picture must still open: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("the missing picture was not reported")
	}
	// Bare rather than half-loaded: a FacePaint with no pixels is a texture
	// every brush stroke would have to special-case.
	if p := res.Doc.Bodies[0].Mesh.Faces[0].Paint; p != nil {
		t.Errorf("the face kept a picture with no pixels in it: %+v", p)
	}
}

func TestANewerFileOpensReadOnly(t *testing.T) {
	doc := shipDoc(t)
	doc.FormatVersion = model.FormatVersion + 5
	res, err := LoadShip(saveTo(t, doc))
	if err != nil {
		t.Fatalf("a newer file must still open: %v", err)
	}
	if !res.ReadOnly {
		t.Error("a file from a newer build opened writable")
	}
	if len(res.Warnings) == 0 {
		t.Error("a file from a newer build opened without saying so")
	}
}

// TestTheReaderSurvivesRubbish is the promise of SPEC-DATA §4: never crash on
// malformed input.
func TestTheReaderSurvivesRubbish(t *testing.T) {
	good, err := os.ReadFile(saveTo(t, shipDoc(t)))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cases := map[string][]byte{
		"empty":     {},
		"garbage":   []byte("this is not a zip, it is a sentence"),
		"truncated": good[:len(good)/2],
		"headOnly":  good[:4],
	}
	for name, data := range cases {
		path := filepath.Join(dir, name+".ship")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadShip(path); err == nil {
			t.Errorf("%s: loaded cleanly, want an error", name)
		}
	}
	// A valid zip with a broken document inside it is the same promise.
	path := filepath.Join(dir, "baddoc.ship")
	writeZip(t, path, map[string][]byte{
		"manifest.json": []byte(`{"app":"modeler","formatVersion":1}`),
		"document.json": []byte(`{"bodies": [ this is not json`),
	})
	if _, err := LoadShip(path); err == nil {
		t.Error("a broken document.json loaded cleanly")
	}
}

func TestUnknownFieldsAreIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.ship")
	writeZip(t, path, map[string][]byte{
		"manifest.json": []byte(`{"app":"modeler","formatVersion":1,"whatIsThis":true}`),
		"document.json": []byte(`{"formatVersion":1,"planes":[{"visible":true},` +
			`{"visible":true},{"visible":true}],"mirrorPlane":"X"}`),
	})
	res, err := LoadShip(path)
	if err != nil {
		t.Fatalf("a document with a field we do not know must still load: %v", err)
	}
	if !res.Doc.PlaneVisible(geom.PlaneTop) {
		t.Error("the fields we do know were not read")
	}
}

// TestAFailedSaveLeavesTheOldFileAlone is the atomicity of SPEC-DATA §4: the
// target is never clobbered until the new file is whole.
func TestAFailedSaveLeavesTheOldFileAlone(t *testing.T) {
	path := saveTo(t, shipDoc(t))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A document whose mesh cannot be written: a nil body is enough.
	broken := shipDoc(t)
	broken.Bodies = append(broken.Bodies, nil)
	if err := SaveShip(path, broken, nil); err == nil {
		t.Fatal("saving a broken document reported success")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the existing file is gone: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a failed save damaged the file that was already there")
	}
	if leftovers := tempLeftovers(t, filepath.Dir(path)); len(leftovers) > 0 {
		t.Errorf("a failed save left %v behind", leftovers)
	}
}

func TestThumbnailIsWrittenWhenGiven(t *testing.T) {
	thumb := image.NewRGBA(image.Rect(0, 0, 8, 8))
	thumb.SetRGBA(1, 1, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	path := filepath.Join(t.TempDir(), "thumb.ship")
	if err := SaveShip(path, shipDoc(t), thumb); err != nil {
		t.Fatal(err)
	}
	if data := zipMember(t, path, "thumbnail.png"); len(data) == 0 {
		t.Error("no thumbnail was written")
	}
	res, err := LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Thumbnail == nil {
		t.Fatal("the thumbnail did not come back")
	}
	if got := res.Thumbnail.Bounds(); got != image.Rect(0, 0, 8, 8) {
		t.Errorf("the thumbnail came back %v", got)
	}
}

// --- helpers ---

func zipMember(t *testing.T, path, name string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	return nil
}

// stripMember rewrites a .ship without the members a predicate selects.
func stripMember(t *testing.T, path string, drop func(string) bool) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string][]byte{}
	for _, f := range zr.File {
		if drop(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		kept[f.Name] = buf.Bytes()
	}
	zr.Close()
	writeZip(t, path, kept)
}

func writeZip(t *testing.T, path string, members map[string][]byte) {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, data := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func tempLeftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") || strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}
