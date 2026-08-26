package model

import (
	"math"
	"strings"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/csg"
	"modeler/internal/geom/mesh"
)

// Push/pull at the command level. The app tests drive the drag; these check the
// arithmetic and the refusals, which is where a face-moving tool goes wrong in
// ways a screenshot would not show.

func v3(x, y, z float64) geom.Vec3 { return geom.Vec3{X: x, Y: y, Z: z} }

// boxDoc is a document holding one 4x4x4 box, and the identity of its top face.
func boxDoc(t *testing.T) (*Document, *Body, mesh.FaceUID) {
	t.Helper()
	doc := NewDocument()
	id := doc.Seq.NextBody()
	b := &Body{
		ID: id, Name: "Box", Visible: true,
		Mesh: mesh.Box(v3(0, 0, 0), v3(4, 4, 4), id),
	}
	b.FaceSeq = uint32(len(b.Mesh.Faces))
	doc.Bodies = append(doc.Bodies, b)

	top := mesh.NoFace
	for i := range b.Mesh.Faces {
		if b.Mesh.FaceNormal(i).Dot(geom.AxisY) > 0.99 {
			top = b.Mesh.Faces[i].ID
		}
	}
	if top == mesh.NoFace {
		t.Fatal("the box has no top face")
	}
	return doc, b, top
}

// TestPullingAFaceAddsAPrismOfMaterial is the arithmetic of R11: moving a 4x4
// face out by 2 adds exactly 32 cubic units, no more and no less.
func TestPullingAFaceAddsAPrismOfMaterial(t *testing.T) {
	doc, b, top := boxDoc(t)
	before := mesh.Volume(b.Mesh)

	cmd := &PushPull{Body: b.ID, Face: top, Distance: geom.ToSubunits(2)}
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("PushPull: %v", err)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("the result is invalid: %v", err)
	}
	if got := mesh.Volume(b.Mesh); math.Abs(got-(before+32)) > 1e-9 {
		t.Errorf("volume = %v, want %v", got, before+32)
	}
	// Pulling a whole face of a box out leaves a taller box, not a box with a
	// slab balanced on it: the shared face has to disappear.
	if n := len(b.Mesh.Faces); n != 6 {
		t.Errorf("the taller box has %d faces, want 6", n)
	}

	cmd.Undo(doc)
	if got := mesh.Volume(b.Mesh); math.Abs(got-before) > 1e-9 {
		t.Errorf("after undo, volume = %v, want %v", got, before)
	}
}

// TestPushingAFaceInRemovesMaterial covers the other direction, chosen by the
// sign of the drag rather than by a mode.
func TestPushingAFaceInRemovesMaterial(t *testing.T) {
	doc, b, top := boxDoc(t)
	before := mesh.Volume(b.Mesh)

	cmd := &PushPull{Body: b.ID, Face: top, Distance: -geom.ToSubunits(1.5)}
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("PushPull: %v", err)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("the result is invalid: %v", err)
	}
	if got := mesh.Volume(b.Mesh); math.Abs(got-(before-24)) > 1e-9 {
		t.Errorf("volume = %v, want %v", got, before-24)
	}
	if n := len(b.Mesh.Faces); n != 6 {
		t.Errorf("the shorter box has %d faces, want 6", n)
	}
}

// TestPushingAFaceThroughTheBodyEmptiesItLegally: pushing a face further than
// the body is deep is a legal nothing, not an error, and undo brings it back.
func TestPushingAFaceThroughTheBodyEmptiesItLegally(t *testing.T) {
	doc, b, top := boxDoc(t)

	cmd := &PushPull{Body: b.ID, Face: top, Distance: -geom.ToSubunits(6)}
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("PushPull: %v", err)
	}
	if !cmd.Emptied() {
		t.Fatal("pushing a face right through the body left something behind")
	}
	if len(doc.Bodies) != 0 {
		t.Errorf("%d bodies remain, want none", len(doc.Bodies))
	}

	cmd.Undo(doc)
	if len(doc.Bodies) != 1 {
		t.Fatalf("undo restored %d bodies, want 1", len(doc.Bodies))
	}
	if got := mesh.Volume(doc.Bodies[0].Mesh); math.Abs(got-64) > 1e-9 {
		t.Errorf("the restored box has volume %v, want 64", got)
	}
}

// TestPushPullRefusesWhatItCannotDo keeps the refusals honest: each says what
// is wrong in words, and none of them touches the document.
func TestPushPullRefusesWhatItCannotDo(t *testing.T) {
	doc, b, top := boxDoc(t)
	before := mesh.Volume(b.Mesh)

	cases := []struct {
		name string
		cmd  *PushPull
		says string
	}{
		{"no body", &PushPull{Body: 999, Face: top, Distance: 256}, "body"},
		{"no face", &PushPull{Body: b.ID, Face: mesh.MakeFaceUID(b.ID, 999), Distance: 256}, "face"},
		{"no distance", &PushPull{Body: b.ID, Face: top, Distance: 0}, "drag"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cmd.Do(doc)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal %q does not mention %q", err, c.says)
			}
			if got := mesh.Volume(b.Mesh); got != before {
				t.Errorf("the document changed anyway: volume %v, was %v", got, before)
			}
		})
	}
}

// TestPushPullRefusesANonFlatFace is the rule of SPEC-UX §10: a face that has
// been bent by a vertex edit has no single normal to move along, so there is no
// honest answer to give.
func TestPushPullRefusesANonFlatFace(t *testing.T) {
	doc, b, top := boxDoc(t)
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].ID == top {
			b.Mesh.Faces[i].NonPlanar = true
		}
	}

	err := (&PushPull{Body: b.ID, Face: top, Distance: geom.ToSubunits(2)}).Do(doc)
	if err == nil {
		t.Fatal("a bent face was pushed anyway")
	}
	if !strings.Contains(err.Error(), "flat") {
		t.Errorf("the refusal %q does not say the face is not flat", err)
	}
}

// TestFaceRegionKeepsHoles: a face with a hole in it pushes as a ring, not as a
// filled slab. Getting this wrong would quietly plug every window in the ship.
func TestFaceRegionKeepsHoles(t *testing.T) {
	doc := NewDocument()
	id := doc.Seq.NextBody()
	plate := mesh.Box(v3(-5, -5, 0), v3(5, 5, 2), id)
	b := &Body{ID: id, Name: "Plate", Visible: true, Mesh: plate}
	b.FaceSeq = uint32(len(plate.Faces))
	doc.Bodies = append(doc.Bodies, b)

	// Bore a hole so the top face has a hole loop.
	toolID := doc.Seq.NextBody()
	hole := mesh.Box(v3(-1, -1, -1), v3(1, 1, 3), toolID)
	res, err := csg.Boolean(csg.Subtract, plate, hole)
	if err != nil {
		t.Fatalf("boring the hole: %v", err)
	}
	b.Mesh, b.FaceSeq = res.Mesh, res.NextFaceSeq

	top := -1
	for i := range b.Mesh.Faces {
		if b.Mesh.FaceNormal(i).Dot(geom.AxisZ) > 0.99 && len(b.Mesh.Faces[i].Loops) > 1 {
			top = i
		}
	}
	if top < 0 {
		t.Fatal("the bored plate has no top face with a hole in it")
	}

	region, err := FaceRegion(b.Mesh, top, b.Mesh.FaceFrame(top))
	if err != nil {
		t.Fatalf("FaceRegion: %v", err)
	}
	if len(region.Holes) != 1 {
		t.Fatalf("the region has %d holes, want 1", len(region.Holes))
	}

	// Pulling that face out adds a ring, so the hole stays open: 10x10 minus
	// 2x2, three units tall.
	before := mesh.Volume(b.Mesh)
	cmd := &PushPull{Body: b.ID, Face: b.Mesh.Faces[top].ID, Distance: geom.ToSubunits(3)}
	if err := cmd.Do(doc); err != nil {
		t.Fatalf("PushPull: %v", err)
	}
	if err := mesh.Validate(b.Mesh); err != nil {
		t.Fatalf("the result is invalid: %v", err)
	}
	want := before + (100-4)*3
	if got := mesh.Volume(b.Mesh); math.Abs(got-want) > 1e-9 {
		t.Errorf("volume = %v, want %v — the hole was filled in", got, want)
	}
}
