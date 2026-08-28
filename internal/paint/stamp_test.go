package paint

import (
	"image"
	"image/color"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// The stamp write and its command (Tile_paint.md TP1). A stamp is a batch of
// texel writes at an offset; what these tests pin is the contract around it —
// the alpha threshold, the clip, the exact dirty rect, and the undo putting
// back precisely what was there.

// gradTile is a 4x4 tile with a known pattern: opaque red on the left half,
// fully transparent on the right, and one half-alpha pixel at (2,0) that must
// NOT paint (threshold 128).
func gradTile() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	img.SetRGBA(2, 0, color.RGBA{R: 200, G: 30, B: 30, A: 100})
	return img
}

func TestStampRectHonoursAlphaAndClip(t *testing.T) {
	m, fi := plate() // 8x4 top face
	p, err := Allocate(m, fi, 8)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[fi].Paint = p
	// Something already there, to prove transparency does not erase it.
	underneath := color.RGBA{R: 20, G: 20, B: 200, A: 255}
	Set(p, image.Point{X: 3, Y: 1}, underneath)

	clip := FaceRect(m, fi, p)
	wrote := StampRect(p, gradTile(), image.Point{X: 0, Y: 0}, clip)
	if wrote.Empty() {
		t.Fatal("the stamp wrote nothing")
	}
	// Opaque half painted.
	if got := At(p, image.Point{X: 0, Y: 0}); got.R != 200 {
		t.Errorf("opaque tile pixel did not paint: %v", got)
	}
	if got := At(p, image.Point{X: 1, Y: 3}); got.R != 200 {
		t.Errorf("opaque tile pixel (1,3) did not paint: %v", got)
	}
	// Half-alpha pixel skipped.
	if got := At(p, image.Point{X: 2, Y: 0}); got.A != 0 {
		t.Errorf("a 100-alpha tile pixel painted: %v", got)
	}
	// Transparent pixels leave what was there.
	if got := At(p, image.Point{X: 3, Y: 1}); got != underneath {
		t.Errorf("a transparent tile pixel disturbed the surface: %v", got)
	}
	// The dirty rect covers exactly the written pixels: the opaque 2x4 block.
	if want := image.Rect(0, 0, 2, 4); wrote != want {
		t.Errorf("dirty rect = %v, want %v", wrote, want)
	}
}

func TestStampRectClipsToTheRectItIsGiven(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 8)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[fi].Paint = p
	face := FaceRect(m, fi, p) // 64x32 texels

	// Stamp hanging off the face's max corner: only the on-rect part lands.
	at := image.Point{X: face.Max.X - 1, Y: face.Max.Y - 1}
	wrote := StampRect(p, gradTile(), at, face)
	if want := image.Rect(at.X, at.Y, at.X+1, at.Y+1); wrote != want {
		t.Errorf("clipped stamp wrote %v, want just %v", wrote, want)
	}
	if got := At(p, image.Point{X: face.Max.X, Y: face.Max.Y}); got.A != 0 {
		t.Error("the stamp escaped its clip rect")
	}
}

func stampDoc(t *testing.T) (*model.Bus, *model.Body, mesh.FaceUID) {
	t.Helper()
	bus := model.NewBus(model.NewDocument())
	cmd := &model.AddBody{Mesh: mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -2}, geom.Vec3{X: 4, Y: 1, Z: 2}, 1), Label: "Plate"}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	b := cmd.AddedBody()
	b.FaceSeq = uint32(len(b.Mesh.Faces))
	top := -1
	for i := range b.Mesh.Faces {
		if b.Mesh.FaceNormal(i).Y > 0.9 {
			top = i
		}
	}
	if top < 0 {
		t.Fatal("no top face")
	}
	return bus, b, b.Mesh.Faces[top].ID
}

func TestStampCommandAllocatesPaintsAndUndoesExactly(t *testing.T) {
	bus, b, uid := stampDoc(t)
	cmd := &StampFace{
		Body: b.ID, Face: uid, Res: 8,
		Tile:  gradTile(),
		Cells: []image.Point{{X: 0, Y: 0}, {X: 4, Y: 0}},
	}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	p := cmd.Painted()
	if p == nil {
		t.Fatal("no paint after the stamp")
	}
	if !cmd.LayoutChanged() {
		t.Error("a first stamp allocated the texture and did not say so")
	}
	// Two cells, 8 opaque pixels each.
	painted := 0
	r := p.TexelBounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if At(p, image.Point{X: x, Y: y}).A != 0 {
				painted++
			}
		}
	}
	if painted != 16 {
		t.Errorf("stamped %d texels, want 16 (two cells of the 8-opaque tile)", painted)
	}
	if cmd.UndoBytes() == 0 {
		t.Error("the stamp reports no undo cost")
	}

	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	fi := -1
	for i := range b.Mesh.Faces {
		if b.Mesh.Faces[i].ID == uid {
			fi = i
		}
	}
	if b.Mesh.Faces[fi].Paint != nil {
		t.Error("undo of an allocating stamp left the texture behind")
	}
	if _, ok := bus.Redo(); !ok {
		t.Fatal("nothing to redo")
	}
	if b.Mesh.Faces[fi].Paint == nil {
		t.Error("redo did not restore the texture")
	}
}

func TestStampCommandOverPaintRestoresIt(t *testing.T) {
	bus, b, uid := stampDoc(t)
	// Underpaint the face first, as its own step.
	if err := bus.Run(&StrokeFace{
		Body: b.ID, Face: uid, Res: 8, Tool: ToolPencil,
		Color: color.RGBA{G: 200, A: 255}, Size: 4,
		Points: []image.Point{{X: 0, Y: 0}},
	}); err != nil {
		t.Fatal(err)
	}
	cmd := &StampFace{
		Body: b.ID, Face: uid,
		Tile:  gradTile(),
		Cells: []image.Point{{X: 0, Y: 0}},
	}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	p := cmd.Painted()
	if got := At(p, image.Point{X: 0, Y: 0}); got.R != 200 {
		t.Fatalf("the stamp did not overwrite the underpaint: %v", got)
	}
	// The transparent tile half left the green underpaint alone.
	if got := At(p, image.Point{X: 3, Y: 0}); got.G != 200 {
		t.Errorf("a transparent tile pixel took the underpaint: %v", got)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if got := At(p, image.Point{X: 0, Y: 0}); got.G != 200 || got.R == 200 {
		t.Errorf("undo did not restore the underpaint: %v", got)
	}
}

func TestStampCommandCoalescesADragIntoOneStep(t *testing.T) {
	bus, b, uid := stampDoc(t)
	depth := bus.UndoDepth()
	cells := []image.Point{{X: 0, Y: 0}}
	mk := func() *StampFace {
		return &StampFace{Body: b.ID, Face: uid, Res: 8, Tile: gradTile(),
			Cells: append([]image.Point(nil), cells...)}
	}
	if err := bus.BeginDrag(mk()); err != nil {
		t.Fatal(err)
	}
	for _, c := range []image.Point{{X: 4, Y: 0}, {X: 8, Y: 0}} {
		cells = append(cells, c)
		if err := bus.UpdateDrag(mk()); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := bus.CommitDrag(); !ok {
		t.Fatal("commit failed")
	}
	if got := bus.UndoDepth(); got != depth+1 {
		t.Errorf("a stamp trail cost %d undo entries, want 1", got-depth)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
}

func TestAStampOfNothingIsRefused(t *testing.T) {
	bus, b, uid := stampDoc(t)
	blank := image.NewRGBA(image.Rect(0, 0, 4, 4)) // fully transparent
	err := bus.Run(&StampFace{Body: b.ID, Face: uid, Res: 8, Tile: blank,
		Cells: []image.Point{{X: 0, Y: 0}}})
	if err == nil {
		t.Fatal("a fully transparent stamp went into the history")
	}
}
