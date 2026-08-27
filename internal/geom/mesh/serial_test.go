package mesh

import (
	"encoding/json"
	"image"
	"image/color"
	"testing"

	"modeler/internal/geom"
)

// A mesh has to survive a round trip through the .ship document (SPEC-DATA §4),
// and the thing that is easy to lose on the way is not the geometry.
//
// Fragments of a cut face share one FacePaint by pointer (SPEC-GEOMETRY §8.4).
// Marshalling a face at a time would write that picture once per fragment and
// read back one copy each, and from then on painting one fragment would stop
// showing on its siblings — the contract quietly broken by having been saved.

// sharedPaintMesh is a plate whose two top faces read from one picture, which
// is the shape a boolean leaves behind.
func sharedPaintMesh(t *testing.T) *Mesh {
	t.Helper()
	m := Box(geom.Vec3{X: -2, Y: -1, Z: -2}, geom.Vec3{X: 2, Y: 1, Z: 2}, 1)
	shared := &FacePaint{
		Res:   32,
		Texel: 0.125,
		Frame: geom.FrameFromNormal(geom.Vec3{Y: 1}, geom.Vec3{Y: 1}),
		Img:   image.NewRGBA(image.Rect(0, 0, 4, 4)),
		Off:   image.Point{X: -1, Y: -1},
	}
	shared.Img.SetRGBA(1, 1, color.RGBA{R: 200, G: 30, B: 40, A: 255})

	own := &FacePaint{
		Res:   128,
		Texel: 0.03125,
		Frame: geom.FrameFromNormal(geom.Vec3{Z: 1}, geom.Vec3{Z: 1}),
		Img:   image.NewRGBA(image.Rect(0, 0, 2, 2)),
	}
	m.Faces[0].Paint = shared
	m.Faces[1].Paint = shared
	m.Faces[2].Paint = own
	m.Faces[3].SrcFace = m.Faces[0].ID
	m.Faces[4].NonPlanar = true
	return m
}

func TestMeshRoundTripsThroughJSON(t *testing.T) {
	src := sharedPaintMesh(t)
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Mesh
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}

	if len(got.Verts) != len(src.Verts) {
		t.Fatalf("%d verts came back, want %d", len(got.Verts), len(src.Verts))
	}
	for i := range src.Verts {
		if got.Verts[i] != src.Verts[i] {
			t.Errorf("vertex %d is %v, want %v", i, got.Verts[i], src.Verts[i])
		}
	}
	if len(got.Faces) != len(src.Faces) {
		t.Fatalf("%d faces came back, want %d", len(got.Faces), len(src.Faces))
	}
	for i := range src.Faces {
		a, b := &src.Faces[i], &got.Faces[i]
		if a.ID != b.ID || a.SrcFace != b.SrcFace || a.NonPlanar != b.NonPlanar {
			t.Errorf("face %d came back as id=%d src=%d nonplanar=%v, want %d/%d/%v",
				i, b.ID, b.SrcFace, b.NonPlanar, a.ID, a.SrcFace, a.NonPlanar)
		}
		if len(a.Loops) != len(b.Loops) {
			t.Fatalf("face %d has %d loops, want %d", i, len(b.Loops), len(a.Loops))
		}
		for li := range a.Loops {
			if len(a.Loops[li]) != len(b.Loops[li]) {
				t.Fatalf("face %d loop %d has %d indices, want %d",
					i, li, len(b.Loops[li]), len(a.Loops[li]))
			}
			for vi := range a.Loops[li] {
				if a.Loops[li][vi] != b.Loops[li][vi] {
					t.Fatalf("face %d loop %d index %d moved", i, li, vi)
				}
			}
		}
	}
}

func TestSharedPaintIsStillSharedAfterALoad(t *testing.T) {
	src := sharedPaintMesh(t)
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var got Mesh
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.Faces[0].Paint == nil || got.Faces[1].Paint == nil {
		t.Fatal("the painted faces came back unpainted")
	}
	if got.Faces[0].Paint != got.Faces[1].Paint {
		t.Error("two faces that shared one picture came back with a copy each, " +
			"so painting one would no longer show on the other")
	}
	if got.Faces[2].Paint == got.Faces[0].Paint {
		t.Error("two faces with different pictures came back sharing one")
	}
	if got.Faces[3].Paint != nil {
		t.Error("an unpainted face came back painted")
	}

	p := got.Faces[0].Paint
	if p.Res != 32 || p.Texel != 0.125 || p.Off != (image.Point{X: -1, Y: -1}) {
		t.Errorf("the mapping came back as res=%d texel=%v off=%v", p.Res, p.Texel, p.Off)
	}
	if p.Frame != src.Faces[0].Paint.Frame {
		t.Errorf("the paint frame came back as %+v", p.Frame)
	}
	// The pixels travel as PNGs beside the document, not inside it: a texture
	// spelled out as a JSON array of bytes would dwarf the geometry.
	if p.Img != nil {
		t.Error("the image was written into the document instead of beside it")
	}
}

// TestPaintTableIsInFirstUseOrder pins the order the pictures are listed in, so
// the files beside the document can be named after their owning face and land
// in the same place every save (SPEC-DATA §4).
func TestPaintTableIsInFirstUseOrder(t *testing.T) {
	m := sharedPaintMesh(t)
	table := m.PaintTable()
	if len(table) != 2 {
		t.Fatalf("the mesh reports %d pictures, want 2", len(table))
	}
	if table[0].Owner != m.Faces[0].ID || table[0].Paint != m.Faces[0].Paint {
		t.Errorf("the first picture is owned by face %d, want %d",
			table[0].Owner, m.Faces[0].ID)
	}
	if table[1].Owner != m.Faces[2].ID {
		t.Errorf("the second picture is owned by face %d, want %d",
			table[1].Owner, m.Faces[2].ID)
	}
}

// TestMarshalIsStable is the invariant the save path depends on: writing a
// document that was just read must produce the same bytes, or every save would
// show as a change to whatever the user keeps their ships in.
func TestMarshalIsStable(t *testing.T) {
	src := sharedPaintMesh(t)
	first, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Mesh
	if err := json.Unmarshal(first, &loaded); err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(&loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("a save of a loaded mesh differs:\n first: %s\nsecond: %s", first, second)
	}
}

func TestUnmarshalRejectsAPaintIndexThatIsNotThere(t *testing.T) {
	// A file can be truncated, hand-edited or written by something else. A bad
	// index has to be an error rather than a panic (SPEC-DATA §4).
	bad := `{"verts":[[0,0,0]],"faces":[{"id":1,"loops":[[0]],"paint":7}],"paints":[]}`
	var m Mesh
	if err := json.Unmarshal([]byte(bad), &m); err == nil {
		t.Error("a face pointing at a picture that is not in the table loaded cleanly")
	}
}
