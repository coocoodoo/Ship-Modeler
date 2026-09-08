package paint

import (
	"bytes"
	"image"
	"image/color"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"testing"
)

func workflowFixture(t *testing.T) (*model.Bus, mesh.FaceUID, mesh.FaceUID) {
	t.Helper()
	d := model.NewDocument()
	b := &model.Body{ID: 1, Visible: true, Mesh: mesh.Box(geom.Vec3{}, geom.Vec3{X: 2, Y: 2, Z: 2}, 1)}
	d.Bodies = []*model.Body{b}
	p, e := Allocate(b.Mesh, 0, 32)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < len(p.Img.Pix); i += 4 {
		copy(p.Img.Pix[i:i+4], []byte{40, 50, 60, 255})
	}
	b.Mesh.Faces[0].Paint = p
	b.Mesh.Faces[1].Paint = p
	pin, e := model.AnchorNotePin(b, 0, b.Mesh.Verts[b.Mesh.Faces[0].Outer()[0]])
	if e != nil {
		t.Fatal(e)
	}
	pin.ID = 1
	pin.Text = "Paint only this face"
	d.NotePins = []model.NotePin{pin}
	d.Seq.NotePin = 1
	return model.NewBus(d), b.Mesh.Faces[0].ID, b.Mesh.Faces[1].ID
}
func TestPinReviewIsolationRejectAndAtomicUndo(t *testing.T) {
	bus, face, other := workflowFixture(t)
	original := bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	before := append([]byte(nil), original.Img.Pix...)
	if e := bus.BeginReview(1); e != nil {
		t.Fatal(e)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:i+4], []byte{220, 30, 10, 255})
	}
	if e := bus.Run(&SetMaterialMap{Body: 1, Face: other, Kind: "base_color", Image: img}); e == nil {
		t.Fatal("out-of-scope face accepted")
	}
	if e := bus.Run(&model.ClearNotePins{}); e == nil {
		t.Fatal("global edit accepted")
	}
	if e := bus.Run(&SetMaterialMap{Body: 1, Face: face, Kind: "base_color", Image: img}); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(original.Img.Pix, before) || bus.Doc().Bodies[0].Mesh.Faces[1].Paint != original {
		t.Fatal("shared image escaped scope")
	}
	if bus.UndoDepth() != 0 || bus.CanUndo() {
		t.Fatal("draft leaked into undo history")
	}
	bus.RejectReview()
	if bus.Doc().Bodies[0].Mesh.Faces[0].Paint != original {
		t.Fatal("reject did not restore")
	}
	if e := bus.BeginReview(1); e != nil {
		t.Fatal(e)
	}
	if e := bus.Run(&SetMaterialMap{Body: 1, Face: face, Kind: "base_color", Image: img}); e != nil {
		t.Fatal(e)
	}
	if e := bus.Run(&RefreshPBR{Body: 1, Face: face}); e != nil {
		t.Fatal(e)
	}
	if e := bus.ProposeReview("Red armor with matching PBR"); e != nil {
		t.Fatal(e)
	}
	if e := bus.Run(&RefreshPBR{Body: 1, Face: face}); e == nil {
		t.Fatal("editable after proposal")
	}
	if e := bus.AcceptReview(); e != nil {
		t.Fatal(e)
	}
	if bus.UndoDepth() != 1 || !bus.Doc().NotePins[0].Done {
		t.Fatal("not grouped / pin status")
	}
	bus.Undo()
	if bus.Doc().Bodies[0].Mesh.Faces[0].Paint != original || bus.Doc().NotePins[0].Done {
		t.Fatal("whole request undo failed")
	}
	bus.Redo()
	if !bus.Doc().NotePins[0].Done || bus.Doc().Bodies[0].Mesh.Faces[0].Paint.Material == nil {
		t.Fatal("whole request redo failed")
	}
}
func TestLayersMaskHistoryAndFaceIsolation(t *testing.T) {
	bus, face, _ := workflowFixture(t)
	original := bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	run := func(c model.Command) {
		t.Helper()
		if e := bus.Run(c); e != nil {
			t.Fatal(e)
		}
	}
	run(&EditLayer{Body: 1, Face: face, Action: "add", Label: "Markings"})
	p := bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	if len(p.Layers) != 2 || p.Layers[1].Name != "Markings" {
		t.Fatal("layer not created")
	}
	run(&SetMaterialMap{Body: 1, Face: face, Kind: "base_color", Image: solidImage(color.RGBA{250, 0, 0, 255})})
	p = bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	if p.Img.RGBAAt(5, 5).R != 250 || p.Layers[0].Pixels.RGBAAt(5, 5).R != 40 {
		t.Fatal("paint did not stay in active layer")
	}
	run(&EditLayer{Body: 1, Face: face, Action: "mask", Index: 1, On: true})
	run(&SetMaterialMap{Body: 1, Face: face, Kind: "base_color", Image: solidImage(color.RGBA{0, 0, 0, 255})})
	p = bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	if p.Img.RGBAAt(5, 5).R != 40 {
		t.Fatal("black mask did not reveal base")
	}
	bus.Undo()
	p = bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	if p.Img.RGBAAt(5, 5).R != 250 {
		t.Fatal("mask undo failed")
	}
	if bus.Doc().Bodies[0].Mesh.Faces[1].Paint != original {
		t.Fatal("layer edited sibling")
	}
	run(&RefreshPBR{Body: 1, Face: face})
	if bus.Doc().Bodies[0].Mesh.Faces[0].Paint.PBRStale {
		t.Fatal("regeneration left stale PBR")
	}
}
func solidImage(c color.RGBA) *image.RGBA {
	p := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			p.SetRGBA(x, y, c)
		}
	}
	return p
}
func TestMaterialReferenceMatchesOnlyTargetAndResolution(t *testing.T) {
	bus, face, other := workflowFixture(t)
	ref, e := AnalyzeReference(bus.Doc(), 1, face)
	if e != nil {
		t.Fatal(e)
	}
	ref.Resolution = 16
	if e := bus.Run(&MatchReference{Body: 1, Face: other, Reference: ref}); e != nil {
		t.Fatal(e)
	}
	a := bus.Doc().Bodies[0].Mesh.Faces[0].Paint
	b := bus.Doc().Bodies[0].Mesh.Faces[1].Paint
	if a.Res != 32 || b.Res != 16 || b.Material == nil || len(b.Material.Maps) != 5 {
		t.Fatal("reference scale / PBR / isolation")
	}
}
