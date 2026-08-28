package paint

import (
	"image"
	"image/color"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
)

// The stroke command of SPEC-DATA §3.3, written before it exists.
//
// A stroke is the only command in the program that edits pixels rather than
// geometry, and it is the only one whose undo data is big enough to need a
// budget. Both of those are easy to get subtly wrong in ways a screenshot will
// never show: an undo that restores the wrong rectangle looks like paint that
// went missing, and a stroke that copies a shared image looks like paint that
// stopped following a cut.

// painted builds a document with one plate body and returns the bus, the body
// and the UID of its top face.
func painted(t *testing.T) (*model.Bus, *model.Body, mesh.FaceUID, int) {
	t.Helper()
	m := mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -2}, geom.Vec3{X: 4, Y: 1, Z: 2}, 1)
	fi := faceAlong(m, geom.Vec3{X: 0, Y: 1, Z: 0})
	bus := model.NewBus(model.NewDocument())
	add := &model.AddBody{Mesh: m}
	if err := bus.Run(add); err != nil {
		t.Fatalf("AddBody: %v", err)
	}
	b := add.AddedBody()
	return bus, b, m.Faces[fi].ID, fi
}

func stroke(body uint32, face mesh.FaceUID, res int, tool Tool, col color.RGBA, pts ...image.Point) *StrokeFace {
	return &StrokeFace{
		Body: body, Face: face, Res: res,
		Tool: tool, Color: col, Size: 1,
		Points: pts,
	}
}

func at(t *testing.T, b *model.Body, fi int, x, y int) color.RGBA {
	t.Helper()
	return At(b.Mesh.Faces[fi].Paint, image.Point{X: x, Y: y})
}

func TestFirstStrokeAllocatesAtTheChosenResolution(t *testing.T) {
	bus, b, uid, fi := painted(t)
	cmd := stroke(b.ID, uid, 4, ToolPencil, red, image.Point{X: 4, Y: 4})
	if err := bus.Run(cmd); err != nil {
		t.Fatalf("stroke: %v", err)
	}
	p := b.Mesh.Faces[fi].Paint
	if p == nil {
		t.Fatal("the face is still unpainted after a stroke")
	}
	if p.Res != 4 {
		t.Errorf("res = %d, want 4", p.Res)
	}
	if got := at(t, b, fi, 4, 4); got != red {
		t.Errorf("texel (4,4) = %v, want %v", got, red)
	}
	if !cmd.LayoutChanged() {
		t.Error("allocating a texture is a layout change and must say so")
	}
}

func TestUndoOfTheFirstStrokeLeavesTheFaceUnpainted(t *testing.T) {
	bus, b, uid, fi := painted(t)
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, red, image.Point{X: 4, Y: 4})); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	// Not "an image full of transparent texels" — no image at all. A face that
	// was never painted and a face whose paint was undone are the same face.
	if b.Mesh.Faces[fi].Paint != nil {
		t.Error("undoing the first stroke left the texture behind")
	}
}

func TestUndoOfALaterStrokeRestoresOnlyItsOwnRect(t *testing.T) {
	bus, b, uid, fi := painted(t)
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, red, image.Point{X: 2, Y: 2})); err != nil {
		t.Fatal(err)
	}
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, blue, image.Point{X: 9, Y: 9})); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if got := at(t, b, fi, 2, 2); got != red {
		t.Errorf("the earlier stroke was disturbed: (2,2) = %v, want %v", got, red)
	}
	if got := at(t, b, fi, 9, 9); got.A != 0 {
		t.Errorf("the undone stroke is still there: (9,9) = %v", got)
	}
}

func TestRedoRepaintsTheSameTexels(t *testing.T) {
	bus, b, uid, fi := painted(t)
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, red,
		image.Point{X: 3, Y: 3}, image.Point{X: 7, Y: 3})); err != nil {
		t.Fatal(err)
	}
	bus.Undo()
	if _, ok := bus.Redo(); !ok {
		t.Fatal("nothing to redo")
	}
	for x := 3; x <= 7; x++ {
		if got := at(t, b, fi, x, 3); got != red {
			t.Errorf("after redo, texel (%d,3) = %v, want %v", x, got, red)
		}
	}
}

func TestStrokeInterpolatesBetweenItsSamples(t *testing.T) {
	bus, b, uid, fi := painted(t)
	// Two samples eight texels apart, as a fast drag would deliver.
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, red,
		image.Point{X: 2, Y: 2}, image.Point{X: 10, Y: 2})); err != nil {
		t.Fatal(err)
	}
	for x := 2; x <= 10; x++ {
		if got := at(t, b, fi, x, 2); got != red {
			t.Fatalf("the stroke is dotted: texel (%d,2) = %v", x, got)
		}
	}
}

func TestAStrokeOnOneFragmentShowsOnItsSiblings(t *testing.T) {
	// SPEC-GEOMETRY §8.4: fragments of a cut face share one FacePaint by
	// pointer, and v1 keeps it shared. Painting one must therefore write into
	// the picture the others are reading — copying it here would be the bug,
	// not the fix.
	bus, b, uid, fi := painted(t)
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, red, image.Point{X: 4, Y: 4})); err != nil {
		t.Fatal(err)
	}
	shared := b.Mesh.Faces[fi].Paint

	// Give a second face the same paint object, as a boolean would.
	other := faceAlong(b.Mesh, geom.Vec3{X: 0, Y: -1, Z: 0})
	b.Mesh.Faces[other].Paint = shared

	if err := bus.Run(stroke(b.ID, b.Mesh.Faces[fi].ID, 4, ToolPencil, blue,
		image.Point{X: 6, Y: 6})); err != nil {
		t.Fatal(err)
	}
	if b.Mesh.Faces[other].Paint != shared {
		t.Fatal("the stroke replaced the shared paint with a copy")
	}
	if got := At(b.Mesh.Faces[other].Paint, image.Point{X: 6, Y: 6}); got != blue {
		t.Errorf("the sibling face did not see the stroke: %v", got)
	}
}

func TestFillStopsAtTheEdgeOfTheFace(t *testing.T) {
	bus, b, uid, fi := painted(t)
	if err := bus.Run(&StrokeFace{
		Body: b.ID, Face: uid, Res: 4, Tool: ToolFill, Color: red, Size: 1,
		Points: []image.Point{{X: 5, Y: 5}},
	}); err != nil {
		t.Fatalf("fill: %v", err)
	}
	p := b.Mesh.Faces[fi].Paint
	// The face is 32x16 texels; the image has a margin ring outside it that the
	// fill must not have touched, or paint would exist where no surface does.
	face := FaceRect(b.Mesh, fi, p)
	if got := At(p, image.Point{X: face.Min.X, Y: face.Min.Y}); got != red {
		t.Errorf("the fill did not reach the face corner: %v", got)
	}
	if got := At(p, image.Point{X: face.Min.X - 1, Y: face.Min.Y}); got.A != 0 {
		t.Errorf("the fill leaked into the margin: %v", got)
	}
}

func TestEraserClearsBackToTheBody(t *testing.T) {
	bus, b, uid, fi := painted(t)
	if err := bus.Run(&StrokeFace{
		Body: b.ID, Face: uid, Res: 4, Tool: ToolFill, Color: red, Size: 1,
		Points: []image.Point{{X: 5, Y: 5}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Run(stroke(b.ID, uid, 4, ToolEraser, color.RGBA{},
		image.Point{X: 5, Y: 5})); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if got := at(t, b, fi, 5, 5); got.A != 0 {
		t.Errorf("the eraser left %v behind, want an unpainted texel", got)
	}
}

func TestAStrokeThatChangesNothingIsRefused(t *testing.T) {
	bus, b, uid, _ := painted(t)
	// An eraser on a face that was never painted has nothing to rub out. It
	// must not allocate a texture, and it must not land in the history — an
	// undo step that undoes nothing is worse than no step at all.
	err := bus.Run(stroke(b.ID, uid, 4, ToolEraser, color.RGBA{}, image.Point{X: 4, Y: 4}))
	if err == nil {
		t.Fatal("erasing bare geometry was accepted")
	}
	if bus.UndoDepth() != 1 { // just the AddBody
		t.Errorf("undo depth = %d, want 1: the refused stroke was recorded", bus.UndoDepth())
	}
}

func TestADragCoalescesIntoOneHistoryStep(t *testing.T) {
	bus, b, uid, fi := painted(t)
	depth := bus.UndoDepth()

	// What the app does per frame: replace the pending command with a longer
	// version of the same stroke.
	pts := []image.Point{{X: 2, Y: 2}}
	if err := bus.BeginDrag(stroke(b.ID, uid, 4, ToolPencil, red, pts...)); err != nil {
		t.Fatal(err)
	}
	for x := 3; x <= 8; x++ {
		pts = append(pts, image.Point{X: x, Y: 2})
		if err := bus.UpdateDrag(stroke(b.ID, uid, 4, ToolPencil, red, pts...)); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := bus.CommitDrag(); !ok {
		t.Fatal("the drag did not commit")
	}
	if got := bus.UndoDepth() - depth; got != 1 {
		t.Errorf("the drag left %d history entries, want 1", got)
	}
	for x := 2; x <= 8; x++ {
		if got := at(t, b, fi, x, 2); got != red {
			t.Errorf("texel (%d,2) = %v after the drag", x, got)
		}
	}
	bus.Undo()
	if b.Mesh.Faces[fi].Paint != nil {
		t.Error("undoing the whole drag left paint behind")
	}
}

func TestResampleKeepsPixelsWhereTheyAreInTheWorld(t *testing.T) {
	bus, b, uid, fi := painted(t)
	if err := bus.Run(&StrokeFace{
		Body: b.ID, Face: uid, Res: 4, Tool: ToolFill, Color: red, Size: 1,
		Points: []image.Point{{X: 5, Y: 5}},
	}); err != nil {
		t.Fatal(err)
	}
	p := b.Mesh.Faces[fi].Paint
	probes := []geom.Vec3{
		{X: -3, Y: 1, Z: -1}, {X: 0, Y: 1, Z: 0}, {X: 3.5, Y: 1, Z: 1.5},
	}
	before := make([]color.RGBA, len(probes))
	for i, q := range probes {
		before[i] = At(p, Texel(p, q))
	}

	if err := bus.Run(&ResampleFace{Body: b.ID, Face: uid, Res: 16}); err != nil {
		t.Fatalf("resample: %v", err)
	}
	q := b.Mesh.Faces[fi].Paint
	if q.Res != 16 {
		t.Fatalf("res = %d after resample, want 16", q.Res)
	}
	if q.Texel >= p.Texel {
		t.Errorf("texel size did not shrink: %v then %v", p.Texel, q.Texel)
	}
	for i, w := range probes {
		if got := At(q, Texel(q, w)); got != before[i] {
			t.Errorf("probe %v changed from %v to %v", w, before[i], got)
		}
	}

	// And it is undoable back to the original picture, pointer and all.
	bus.Undo()
	if b.Mesh.Faces[fi].Paint != p {
		t.Error("undoing a resample did not restore the original texture")
	}
}

func TestStrokeReportsWhatTheRendererHasToReupload(t *testing.T) {
	bus, b, uid, _ := painted(t)
	cmd := stroke(b.ID, uid, 4, ToolPencil, red,
		image.Point{X: 4, Y: 4}, image.Point{X: 6, Y: 4})
	cmd.Size = 2
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	want := image.Rect(4, 4, 8, 6) // two 2x2 dabs from (4,4) to (6,4)
	if got := cmd.DirtyRect(); got != want {
		t.Errorf("dirty rect = %v, want %v", got, want)
	}
	if cmd.UndoBytes() <= 0 {
		t.Error("a stroke that painted something reports no undo cost")
	}
}

// TestAShapeRubberBandsRatherThanAccumulating is the property that makes a
// two-point tool usable at all (SPEC-UX §13.4).
//
// The drag replaces its pending command with a longer version of itself each
// frame, exactly as a freehand stroke does. For a path that is right: the
// stroke really is everything you have drawn. For a shape it would be a
// disaster — dragging a rectangle out and pulling it back in would leave every
// size it passed through stamped on the face. The command has to read only the
// two ends, and the bus has to undo the last version before applying the next.
func TestAShapeRubberBandsRatherThanAccumulating(t *testing.T) {
	bus, b, uid, fi := painted(t)
	shape := func(pts ...image.Point) *StrokeFace {
		return &StrokeFace{
			Body: b.ID, Face: uid, Res: 4,
			Tool: ToolRect, Color: red, Size: 1, Points: pts,
		}
	}
	start := image.Point{X: 4, Y: 4}
	if err := bus.BeginDrag(shape(start, image.Point{X: 20, Y: 16})); err != nil {
		t.Fatal(err)
	}
	// Out to a big rectangle, then back to a small one.
	for _, end := range []image.Point{{X: 24, Y: 20}, {X: 12, Y: 10}, {X: 8, Y: 7}} {
		if err := bus.UpdateDrag(shape(start, end)); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := bus.CommitDrag(); !ok {
		t.Fatal("the drag did not commit")
	}

	// What is left is the last rectangle and nothing else.
	for x := 4; x <= 8; x++ {
		if got := at(t, b, fi, x, 4); got != red {
			t.Errorf("(%d,4) is %v, want the final rectangle's top wall", x, got)
		}
	}
	for _, ghost := range []image.Point{{X: 20, Y: 16}, {X: 24, Y: 20}, {X: 12, Y: 10}} {
		if got := at(t, b, fi, ghost.X, ghost.Y); got.A != 0 {
			t.Errorf("a rectangle the drag passed through is still on the face at %v", ghost)
		}
	}
	if got := at(t, b, fi, 6, 6); got.A != 0 {
		t.Error("the rectangle came out filled, not outlined")
	}
}

func TestAFilledShapeIsOneUndoStep(t *testing.T) {
	bus, b, uid, fi := painted(t)
	depth := bus.UndoDepth()
	err := bus.Run(&StrokeFace{
		Body: b.ID, Face: uid, Res: 4, Tool: ToolCircle,
		Color: blue, Size: 1, Fill: true,
		Points: []image.Point{{X: 4, Y: 4}, {X: 20, Y: 18}},
	})
	if err != nil {
		t.Fatalf("circle: %v", err)
	}
	if got := bus.UndoDepth() - depth; got != 1 {
		t.Errorf("a filled circle left %d history entries, want 1", got)
	}
	if got := at(t, b, fi, 12, 11); got != blue {
		t.Errorf("the middle of a filled circle is %v, want %v", got, blue)
	}
	bus.Undo()
	if b.Mesh.Faces[fi].Paint != nil {
		t.Error("undoing the first shape on a face left its texture behind")
	}
}

// TestAGradientBlendsIntoTheBodyColourOnlyWhereItIsAsked keeps the two ways a
// tool can reach for the body's own colour apart: a soft brush blends into it,
// a gradient does not — it ramps between the two colours it was given.
func TestAGradientRunsBetweenTheTwoArmedColours(t *testing.T) {
	bus, b, uid, fi := painted(t)
	err := bus.Run(&StrokeFace{
		Body: b.ID, Face: uid, Res: 4, Tool: ToolGradient,
		Color: red, ColorB: blue, Size: 1,
		Points: []image.Point{{X: 0, Y: 8}, {X: 31, Y: 8}},
	})
	if err != nil {
		t.Fatalf("gradient: %v", err)
	}
	if got := at(t, b, fi, 0, 8); got != red {
		t.Errorf("the near end is %v, want %v", got, red)
	}
	if got := at(t, b, fi, 31, 8); got != blue {
		t.Errorf("the far end is %v, want %v", got, blue)
	}
	// A gradient covers the face, not just the line the drag drew.
	if got := at(t, b, fi, 15, 0); got.A == 0 {
		t.Error("the gradient left the top of the face unpainted")
	}
}

// TestTheSoftBrushBlendsIntoTheBodyColour proves the command hands the body's
// own colour down to the brush. Without it a soft edge fades into black, which
// looks like a shadow nobody asked for.
func TestTheSoftBrushBlendsIntoTheBodyColour(t *testing.T) {
	bus, body, uid, fi := painted(t)
	err := bus.Run(&StrokeFace{
		Body: body.ID, Face: uid, Res: 16, Tool: ToolBrush,
		Color: red, Size: 16,
		Points: []image.Point{{X: 40, Y: 40}},
	})
	if err != nil {
		t.Fatalf("soft brush: %v", err)
	}
	// Walk out from the centre until the colour stops being pure red; whatever
	// it fades toward has to be the body's colour, not the zero value.
	var edge color.RGBA
	for d := 1; d < 12; d++ {
		c := at(t, body, fi, 40+d, 48)
		if c.A != 0 && c != red {
			edge = c
			break
		}
	}
	if edge.A == 0 {
		t.Fatal("the soft brush has no faded edge at all")
	}
	toward := func(v, from, to uint8) bool {
		return (v > from) == (to > from) || v == to
	}
	if !toward(edge.R, red.R, body.Color.R) || !toward(edge.G, red.G, body.Color.G) {
		t.Errorf("the soft edge %v is not fading toward the body colour %v",
			edge, body.Color)
	}
}

// TestRubberBandReplaceKeepsAMirrorTrue is the user's "dragging a line up and
// down leaves artifacts", 2026-08-27.
//
// A two-point tool rubber-bands by drag replace: every UpdateDrag undoes the
// previous frame's shape and draws the new one. The document came out right —
// the artifacts lived in anything mirroring it from events, because the bus
// announced only the new command's dirty rect and said nothing about the
// pixels the undo had just changed. The renderer's texture cache is such a
// mirror; this test builds a minimal one out of the same events and demands it
// end up identical to the document, texel for texel.
func TestRubberBandReplaceKeepsAMirrorTrue(t *testing.T) {
	bus, b, uid, fi := painted(t)

	// The face must already carry paint: a drag that allocates emits a full
	// rebuild every frame, which hides exactly the bug this is pinning.
	if err := bus.Run(stroke(b.ID, uid, 4, ToolPencil, red, image.Point{X: 0, Y: 0})); err != nil {
		t.Fatalf("first dab: %v", err)
	}

	mirror := map[image.Point]color.RGBA{}
	refreshAll := func() {
		p := b.Mesh.Faces[fi].Paint
		if p == nil {
			mirror = map[image.Point]color.RGBA{}
			return
		}
		r := FaceRect(b.Mesh, fi, p)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				pt := image.Point{X: x, Y: y}
				mirror[pt] = At(p, pt)
			}
		}
	}
	bus.Events.Listen(func(ev model.Event) {
		switch ev.Kind {
		case model.EvBodyChanged:
			refreshAll()
		case model.EvBodyPainted:
			for y := ev.Rect.Min.Y; y < ev.Rect.Max.Y; y++ {
				for x := ev.Rect.Min.X; x < ev.Rect.Max.X; x++ {
					pt := image.Point{X: x, Y: y}
					mirror[pt] = At(ev.Paint, pt)
				}
			}
		}
	})
	refreshAll()

	// A vertical line, then the drag swings it horizontal: the two overlap in
	// one texel and nowhere else, so nearly all of the first line has to be
	// erased — and announced.
	a := image.Point{X: 2, Y: 2}
	if err := bus.BeginDrag(stroke(b.ID, uid, 4, ToolLine, red, a, image.Point{X: 2, Y: 12})); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := bus.UpdateDrag(stroke(b.ID, uid, 4, ToolLine, red, a, image.Point{X: 12, Y: 2})); err != nil {
		t.Fatalf("update: %v", err)
	}

	// The document must hold only the final line...
	if got := at(t, b, fi, 2, 8); got == red {
		t.Error("the old line's texels are still painted in the document")
	}
	if got := at(t, b, fi, 8, 2); got != red {
		t.Error("the new line is missing from the document")
	}

	// ...and the mirror must agree with it everywhere.
	p := b.Mesh.Faces[fi].Paint
	r := FaceRect(b.Mesh, fi, p)
	stale := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pt := image.Point{X: x, Y: y}
			if mirror[pt] != At(p, pt) {
				stale++
			}
		}
	}
	if stale > 0 {
		t.Errorf("%d texels on screen no longer match the document — the undone shape was never announced", stale)
	}
}
