package io

import (
	"encoding/json"
	"math"
	"modeler/internal/geom"
	"modeler/internal/model"
	"path/filepath"
	"testing"
)

func TestAttachmentsRoundTripAndGameExports(t *testing.T) {
	doc := markedDoc(t)
	marker := model.Marker{Kind: model.MarkerAttachment, Slot: "Z", AppendText: "Right wing", At: geom.Vec3{X: 4, Y: 1, Z: 2}, Dir: geom.Vec3{X: 2}}
	doc.Markers = append(doc.Markers, marker)
	for _, ext := range []string{".ship", ".pxm"} {
		path := filepath.Join(t.TempDir(), "modular"+ext)
		if err := SaveShip(path, doc, nil); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadShip(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.Doc.Markers) != 4 || loaded.Doc.Markers[3] != marker {
			t.Fatal("saved file lost attachment metadata")
		}
		members := zipMembers(t, path)
		var game GameMarkers
		r, err := members["game/markers.json"].Open()
		if err != nil {
			t.Fatal(err)
		}
		err = json.NewDecoder(r).Decode(&game)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(game.Attachments) != 1 || game.Attachments[0].Name != "Ship Part Z[Right wing]" || game.Attachments[0].At != [3]float64{4, 1, 2} || game.Attachments[0].Dir != [3]float64{1, 0, 0} {
			t.Fatalf("bad attachment payload: %+v", game.Attachments)
		}
	}
	for _, binary := range []bool{false, true} {
		data, _, _, err := buildGLTF(doc, binary, "modular")
		if err != nil {
			t.Fatal(err)
		}
		var out gltfJSON
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		n := out.Nodes[len(out.Nodes)-1]
		if n.Mesh != nil || n.Name != "Ship Part Z[Right wing]" || n.Translation == nil || *n.Translation != [3]float64{4, 1, 2} || n.Extras == nil || n.Extras.Slot != "Z" || n.Rotation == nil {
			t.Fatalf("bad glTF attachment node: %+v", n)
		}
		if len(out.Scenes[0].Nodes) != len(out.Nodes) {
			t.Fatal("attachment node is not in the scene")
		}
	}
}

func TestAttachmentNodeFacesItsNormal(t *testing.T) {
	for _, v := range []geom.Vec3{geom.AxisX, geom.AxisY, geom.AxisZ, geom.AxisZ.Mul(-1), {X: 1, Y: 2, Z: 3}} {
		d := v.Normalize()
		q := attachmentRotation(vec3Arr(d))
		// Rotate +Z with the exported quaternion.
		facing := geom.Vec3{X: 2 * (q[0]*q[2] + q[3]*q[1]), Y: 2 * (q[1]*q[2] - q[3]*q[0]), Z: 1 - 2*(q[0]*q[0]+q[1]*q[1])}
		if facing.Sub(d).Len() > 1e-9 {
			t.Fatalf("node faces %v, want %v", facing, d)
		}
		norm := 0.0
		for _, x := range q {
			norm += x * x
		}
		if math.Abs(norm-1) > 1e-9 {
			t.Fatal("non-unit node rotation")
		}
	}
}
