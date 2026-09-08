package io

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
)

// The .pxm game payload (the user's request, 2026-08-28): the file an engine
// consumes without knowing this program exists. These pin the three promises
// the format makes — the payload is there, the archive is compressed and
// self-contained, and the numbers in it are the ones an engine needs.

// markedDoc is shipDoc with the three orientation dots placed: front out the
// +X end, top on the +Y roof, one thruster firing out the -X stern.
func markedDoc(t *testing.T) *model.Document {
	t.Helper()
	doc := shipDoc(t)
	doc.Markers = []model.Marker{
		{Kind: model.MarkerFront, At: geom.Vec3{X: 4}, Dir: geom.Vec3{X: 1}},
		{Kind: model.MarkerTop, At: geom.Vec3{Y: 1}, Dir: geom.Vec3{Y: 1}},
		{Kind: model.MarkerThruster, At: geom.Vec3{X: -4, Y: 0.5}, Dir: geom.Vec3{X: -1}, R: 0.5},
	}
	return doc
}

func zipMembers(t *testing.T, path string) map[string]*zip.File {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { zr.Close() })
	out := map[string]*zip.File{}
	for _, f := range zr.File {
		out[f.Name] = f
	}
	return out
}

func TestPXMCarriesTheGamePayloadCompressed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scout.pxm")
	if err := SaveShip(path, markedDoc(t), nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	members := zipMembers(t, path)

	for _, want := range []string{"game/ship.glb", "game/markers.json"} {
		f, ok := members[want]
		if !ok {
			t.Fatalf("the .pxm has no %s — the payload is the point of the format", want)
		}
		if f.Method != zip.Deflate {
			t.Errorf("%s is not compressed (method %d)", want, f.Method)
		}
	}
	// Every entry, including document data, uses standard ZIP compression.
	for name, f := range members {
		if f.Method != zip.Deflate {
			t.Errorf("%s is not compressed", name)
		}
	}

	// The glb payload is a real glTF binary.
	rc, err := members["game/ship.glb"].Open()
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 4)
	if _, err := rc.Read(head); err != nil || !bytes.Equal(head, []byte("glTF")) {
		t.Errorf("game/ship.glb does not start with the glTF magic: %q", head)
	}
	rc.Close()
}

func TestGameMarkersCarryTheBasisAndThrusters(t *testing.T) {
	m := BuildGameMarkers(markedDoc(t))
	if m.Format != GameMarkersFormat {
		t.Errorf("format = %q", m.Format)
	}
	if m.Forward == nil || m.Up == nil || m.Right == nil {
		t.Fatal("a front dot must yield the full basis")
	}
	f := geom.Vec3{X: m.Forward[0], Y: m.Forward[1], Z: m.Forward[2]}
	u := geom.Vec3{X: m.Up[0], Y: m.Up[1], Z: m.Up[2]}
	r := geom.Vec3{X: m.Right[0], Y: m.Right[1], Z: m.Right[2]}

	// Orthonormal, and right-handed the way the game thinks: right = up x fwd.
	for name, v := range map[string]geom.Vec3{"forward": f, "up": u, "right": r} {
		if math.Abs(v.Len()-1) > 1e-9 {
			t.Errorf("%s is not unit length: %v", name, v.Len())
		}
	}
	if math.Abs(f.Dot(u)) > 1e-9 || math.Abs(f.Dot(r)) > 1e-9 || math.Abs(u.Dot(r)) > 1e-9 {
		t.Error("the basis is not orthogonal")
	}
	if want := u.Cross(f); r.Sub(want).Len() > 1e-9 {
		t.Errorf("right = %v, want up x forward = %v", r, want)
	}
	// The hull spans -4..4 in X with its front dot at +X: forward is +X.
	if f.X < 0.99 {
		t.Errorf("forward = %v, want +X", f)
	}
	if u.Y < 0.99 {
		t.Errorf("up = %v, want +Y", u)
	}

	if len(m.Thrusters) != 1 {
		t.Fatalf("thrusters = %d, want 1", len(m.Thrusters))
	}
	th := m.Thrusters[0]
	if th.Dir[0] > -0.99 {
		t.Errorf("thruster dir = %v, want -X (the way the exhaust points)", th.Dir)
	}
	if th.R != 0.5 {
		t.Errorf("thruster r = %v, want the authored 0.5", th.R)
	}
}

func TestMarkersSurviveTheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scout.pxm")
	doc := markedDoc(t)
	if err := SaveShip(path, doc, nil); err != nil {
		t.Fatal(err)
	}
	res, err := LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Markers) != len(doc.Markers) {
		t.Fatalf("markers = %d after reload, want %d", len(res.Doc.Markers), len(doc.Markers))
	}
	for i := range doc.Markers {
		if res.Doc.Markers[i] != doc.Markers[i] {
			t.Errorf("marker %d = %+v, want %+v", i, res.Doc.Markers[i], doc.Markers[i])
		}
	}
}

// A file saved under the old name still opens: the rename must orphan nobody.
func TestLegacyShipFilesStillOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-work.ship")
	if err := SaveShip(path, shipDoc(t), nil); err != nil {
		t.Fatal(err)
	}
	if !IsShipFile(path) || !IsShipFile("x.pxm") || IsShipFile("x.png") {
		t.Error("IsShipFile does not accept both names and only both names")
	}
	if _, err := LoadShip(path); err != nil {
		t.Fatalf("a .ship saved yesterday no longer opens: %v", err)
	}
}

// An empty document carries no payload: it is not a ship yet, and the file
// says so instead of shipping an unloadable glb.
func TestAnEmptyDocumentHasNoGamePayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.pxm")
	if err := SaveShip(path, model.NewDocument(), nil); err != nil {
		t.Fatal(err)
	}
	members := zipMembers(t, path)
	if _, ok := members["game/ship.glb"]; ok {
		t.Error("an empty document shipped a game model")
	}
	_ = os.Remove(path)
}

// The JSON the engine parses, exactly as written: a serde-shaped smoke test.
func TestGameMarkersJSONShape(t *testing.T) {
	data, err := marshalGameMarkers(markedDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"format", "center", "halfExtents", "forward", "up", "right", "nose", "thrusters"} {
		if _, ok := back[key]; !ok {
			t.Errorf("markers.json lacks %q", key)
		}
	}
}
