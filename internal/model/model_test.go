package model

import (
	"errors"
	"image/color"
	"reflect"
	"testing"
	"time"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

func testDoc(t *testing.T) (*Document, *Bus) {
	t.Helper()
	doc := NewDocument()
	bus := NewBus(doc)
	bus.Now = func() time.Time { return time.Unix(0, 0).UTC() }
	return doc, bus
}

func box(id uint32) *mesh.Mesh {
	return mesh.Box(geom.Vec3{}, geom.Vec3{X: 1, Y: 1, Z: 1}, id)
}

func addBody(t *testing.T, bus *Bus) *Body {
	t.Helper()
	cmd := &AddBody{Mesh: box(1)}
	if err := bus.Run(cmd); err != nil {
		t.Fatalf("AddBody: %v", err)
	}
	return cmd.AddedBody()
}

func TestNewDocumentShowsAllPlanes(t *testing.T) {
	doc := NewDocument()
	for i := 0; i < geom.PlaneCount; i++ {
		if !doc.PlaneVisible(geom.PlaneKind(i)) {
			t.Errorf("plane %v starts hidden", geom.PlaneKind(i))
		}
	}
	if !doc.IsEmpty() {
		t.Error("a new document is not empty")
	}
	if doc.FormatVersion != FormatVersion {
		t.Errorf("format version = %d, want %d", doc.FormatVersion, FormatVersion)
	}
}

func TestSequencesNeverReuse(t *testing.T) {
	doc, bus := testDoc(t)
	a := addBody(t, bus)
	b := addBody(t, bus)
	if a.ID == b.ID {
		t.Fatal("two bodies got the same id")
	}
	// Deleting the newest body must not hand its number back out.
	if err := bus.Run(&DeleteBody{ID: b.ID}); err != nil {
		t.Fatal(err)
	}
	c := addBody(t, bus)
	if c.ID == b.ID || c.ID == a.ID {
		t.Errorf("id %d was reused after a delete", c.ID)
	}
	if doc.Seq.Body != 3 {
		t.Errorf("body sequence = %d, want 3", doc.Seq.Body)
	}
}

func TestAutoNamesUseTheSequenceNotTheCount(t *testing.T) {
	_, bus := testDoc(t)
	addBody(t, bus)
	second := addBody(t, bus)
	if err := bus.Run(&DeleteBody{ID: second.ID}); err != nil {
		t.Fatal(err)
	}
	third := addBody(t, bus)
	if third.Name != "Body 3" {
		t.Errorf("third body is named %q, want \"Body 3\"", third.Name)
	}
}

func TestFaceUIDsAreUniqueAndNeverReused(t *testing.T) {
	_, bus := testDoc(t)
	b := addBody(t, bus)
	seen := map[mesh.FaceUID]bool{}
	for i := 0; i < 100; i++ {
		uid := b.NextFaceUID()
		if seen[uid] {
			t.Fatalf("FaceUID %d handed out twice", uid)
		}
		if uid.BodyID() != b.ID {
			t.Fatalf("FaceUID %d belongs to body %d, want %d", uid, uid.BodyID(), b.ID)
		}
		seen[uid] = true
	}
}

func TestVisibilityRoundTripsThroughUndo(t *testing.T) {
	doc, bus := testDoc(t)
	b := addBody(t, bus)
	if !b.Visible {
		t.Fatal("new bodies start hidden")
	}

	if err := bus.Run(&SetBodyVisible{ID: b.ID, Visible: false}); err != nil {
		t.Fatal(err)
	}
	if doc.BodyByID(b.ID).Visible {
		t.Error("hiding a body did nothing")
	}
	name, ok := bus.Undo()
	if !ok || name != "Hide Body 1" {
		t.Errorf("undo returned %q, %v", name, ok)
	}
	if !doc.BodyByID(b.ID).Visible {
		t.Error("undo did not restore visibility")
	}
	if _, ok := bus.Redo(); !ok {
		t.Fatal("redo unavailable")
	}
	if doc.BodyByID(b.ID).Visible {
		t.Error("redo did not re-hide the body")
	}
}

func TestPlaneVisibilityIsUndoable(t *testing.T) {
	doc, bus := testDoc(t)
	cmd := &SetPlaneVisible{Plane: geom.PlaneTop, Visible: false}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "Hide Top plane" {
		t.Errorf("command name = %q", cmd.Name())
	}
	if doc.PlaneVisible(geom.PlaneTop) {
		t.Error("the Top plane is still visible")
	}
	bus.Undo()
	if !doc.PlaneVisible(geom.PlaneTop) {
		t.Error("undo did not restore the plane")
	}
	// The other two planes were never touched.
	if !doc.PlaneVisible(geom.PlaneFront) || !doc.PlaneVisible(geom.PlaneRight) {
		t.Error("hiding one plane affected the others")
	}
}

func TestRenameRejectsBlankNames(t *testing.T) {
	doc, bus := testDoc(t)
	b := addBody(t, bus)
	before := b.Name

	for _, bad := range []string{"", "   ", "\t\n"} {
		err := bus.Run(&RenameBody{ID: b.ID, To: bad})
		if err == nil {
			t.Fatalf("rename to %q was accepted", bad)
		}
		if doc.BodyByID(b.ID).Name != before {
			t.Fatal("a rejected rename still changed the name")
		}
	}
	// A failed command leaves no history entry.
	depth := bus.UndoDepth()
	if err := bus.Run(&RenameBody{ID: b.ID, To: "  Nose cone  "}); err != nil {
		t.Fatal(err)
	}
	if got := doc.BodyByID(b.ID).Name; got != "Nose cone" {
		t.Errorf("name = %q, want trimmed \"Nose cone\"", got)
	}
	if bus.UndoDepth() != depth+1 {
		t.Errorf("undo depth = %d, want %d", bus.UndoDepth(), depth+1)
	}
	bus.Undo()
	if doc.BodyByID(b.ID).Name != before {
		t.Error("undo did not restore the old name")
	}
}

func TestDeleteRestoresPosition(t *testing.T) {
	doc, bus := testDoc(t)
	a := addBody(t, bus)
	b := addBody(t, bus)
	c := addBody(t, bus)

	if err := bus.Run(&DeleteBody{ID: b.ID}); err != nil {
		t.Fatal(err)
	}
	if len(doc.Bodies) != 2 || doc.Bodies[0].ID != a.ID || doc.Bodies[1].ID != c.ID {
		t.Fatalf("after delete the order is %v", ids(doc))
	}
	bus.Undo()
	if len(doc.Bodies) != 3 {
		t.Fatalf("undo restored %d bodies", len(doc.Bodies))
	}
	// The body comes back where it was, not at the end.
	if doc.Bodies[1].ID != b.ID {
		t.Errorf("restored order is %v, want the middle body back in the middle", ids(doc))
	}
}

func ids(doc *Document) []uint32 {
	var out []uint32
	for _, b := range doc.Bodies {
		out = append(out, b.ID)
	}
	return out
}

// TestUndoRedoRestoresDeepEqualDocuments is the canon test of TESTING §2: a
// sequence of commands, fully undone, must leave the document as it started.
func TestUndoRedoRestoresDeepEqualDocuments(t *testing.T) {
	doc, bus := testDoc(t)
	b1 := addBody(t, bus)
	b2 := addBody(t, bus)
	snapshot := snapshotOf(doc)

	steps := []Command{
		&SetBodyVisible{ID: b1.ID, Visible: false},
		&RenameBody{ID: b2.ID, To: "Wing"},
		&SetBodyColor{ID: b2.ID, To: color.RGBA{R: 1, G: 2, B: 3, A: 255}},
		&SetPlaneVisible{Plane: geom.PlaneRight, Visible: false},
		&DeleteBody{ID: b1.ID},
	}
	for _, cmd := range steps {
		if err := bus.Run(cmd); err != nil {
			t.Fatalf("%s: %v", cmd.Name(), err)
		}
	}
	after := snapshotOf(doc)
	if reflect.DeepEqual(snapshot, after) {
		t.Fatal("the commands changed nothing, so the test proves nothing")
	}

	for i := 0; i < len(steps); i++ {
		if _, ok := bus.Undo(); !ok {
			t.Fatalf("undo %d unavailable", i)
		}
	}
	if got := snapshotOf(doc); !reflect.DeepEqual(got, snapshot) {
		t.Errorf("after undoing everything the document differs:\n got %+v\nwant %+v", got, snapshot)
	}

	for i := 0; i < len(steps); i++ {
		if _, ok := bus.Redo(); !ok {
			t.Fatalf("redo %d unavailable", i)
		}
	}
	if got := snapshotOf(doc); !reflect.DeepEqual(got, after) {
		t.Errorf("after redoing everything the document differs:\n got %+v\nwant %+v", got, after)
	}
}

// snapshotOf captures the observable document state, ignoring the feature log
// (which is append-only by design and never rewound).
type docSnapshot struct {
	Planes [geom.PlaneCount]bool
	Bodies []bodySnapshot
	Seq    Sequences
}

type bodySnapshot struct {
	ID      uint32
	Name    string
	Color   color.RGBA
	Visible bool
}

func snapshotOf(d *Document) docSnapshot {
	s := docSnapshot{Seq: d.Seq}
	for i := range d.Planes {
		s.Planes[i] = d.Planes[i].Visible
	}
	for _, b := range d.Bodies {
		s.Bodies = append(s.Bodies, bodySnapshot{b.ID, b.Name, b.Color, b.Visible})
	}
	return s
}

// failingCommand probes the atomicity contract of SPEC-DATA §3.1.
type failingCommand struct{ ran bool }

func (c *failingCommand) Name() string { return "Explode" }
func (c *failingCommand) Do(doc *Document) error {
	c.ran = true
	return errors.New("nope")
}
func (c *failingCommand) Undo(doc *Document) { panic("Undo must never run for a failed command") }

func TestFailedCommandLeavesEverythingAlone(t *testing.T) {
	doc, bus := testDoc(t)
	addBody(t, bus)
	before := snapshotOf(doc)
	depth := bus.UndoDepth()
	features := len(doc.Features)

	cmd := &failingCommand{}
	if err := bus.Run(cmd); err == nil {
		t.Fatal("a failing command reported success")
	}
	if !cmd.ran {
		t.Fatal("the command never ran")
	}
	if got := snapshotOf(doc); !reflect.DeepEqual(got, before) {
		t.Error("a failed command changed the document")
	}
	if bus.UndoDepth() != depth {
		t.Error("a failed command landed on the undo stack")
	}
	if len(doc.Features) != features {
		t.Error("a failed command wrote a feature record")
	}
}

func TestRunClearsRedo(t *testing.T) {
	_, bus := testDoc(t)
	b := addBody(t, bus)
	if err := bus.Run(&SetBodyVisible{ID: b.ID, Visible: false}); err != nil {
		t.Fatal(err)
	}
	bus.Undo()
	if !bus.CanRedo() {
		t.Fatal("nothing to redo after an undo")
	}
	if err := bus.Run(&RenameBody{ID: b.ID, To: "Fin"}); err != nil {
		t.Fatal(err)
	}
	if bus.CanRedo() {
		t.Error("a new command did not clear the redo stack")
	}
}

// TestDragCoalescesToOneHistoryEntry covers SPEC-DATA §3.2: a drag is live
// every frame but lands as a single undo step.
func TestDragCoalescesToOneHistoryEntry(t *testing.T) {
	doc, bus := testDoc(t)
	b := addBody(t, bus)
	depth := bus.UndoDepth()

	if err := bus.BeginDrag(&RenameBody{ID: b.ID, To: "A"}); err != nil {
		t.Fatal(err)
	}
	if !bus.Dragging() {
		t.Fatal("the bus does not report a drag in progress")
	}
	if doc.BodyByID(b.ID).Name != "A" {
		t.Error("the drag is not live in the document")
	}
	if bus.UndoDepth() != depth {
		t.Error("a live drag already pushed history")
	}

	for _, name := range []string{"AB", "ABC", "ABCD"} {
		if err := bus.UpdateDrag(&RenameBody{ID: b.ID, To: name}); err != nil {
			t.Fatal(err)
		}
		if doc.BodyByID(b.ID).Name != name {
			t.Fatalf("drag update did not apply %q", name)
		}
	}

	if _, ok := bus.CommitDrag(); !ok {
		t.Fatal("commit failed")
	}
	if bus.UndoDepth() != depth+1 {
		t.Errorf("the drag produced %d history entries, want 1", bus.UndoDepth()-depth)
	}
	bus.Undo()
	if got := doc.BodyByID(b.ID).Name; got != "Body 1" {
		t.Errorf("undoing the drag left the name %q, want the pre-drag \"Body 1\"", got)
	}
}

func TestCancelDragRevertsWithNoHistory(t *testing.T) {
	doc, bus := testDoc(t)
	b := addBody(t, bus)
	depth := bus.UndoDepth()

	if err := bus.BeginDrag(&RenameBody{ID: b.ID, To: "Scratch"}); err != nil {
		t.Fatal(err)
	}
	bus.UpdateDrag(&RenameBody{ID: b.ID, To: "Scratch 2"})
	bus.CancelDrag()

	if bus.Dragging() {
		t.Error("the drag is still live after cancelling")
	}
	if got := doc.BodyByID(b.ID).Name; got != "Body 1" {
		t.Errorf("cancel left the name %q, want \"Body 1\"", got)
	}
	if bus.UndoDepth() != depth {
		t.Error("a cancelled drag left a history entry")
	}
}

func TestRunIsRefusedDuringADrag(t *testing.T) {
	_, bus := testDoc(t)
	b := addBody(t, bus)
	if err := bus.BeginDrag(&RenameBody{ID: b.ID, To: "X"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Run(&SetBodyVisible{ID: b.ID, Visible: false}); err == nil {
		t.Error("a command ran in the middle of a drag")
	}
	bus.CancelDrag()
}

func TestUndoStackIsCapped(t *testing.T) {
	_, bus := testDoc(t)
	b := addBody(t, bus)
	for i := 0; i < UndoCap+50; i++ {
		if err := bus.Run(&SetBodyVisible{ID: b.ID, Visible: i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	if bus.UndoDepth() != UndoCap {
		t.Errorf("undo depth = %d, want the cap %d", bus.UndoDepth(), UndoCap)
	}
	// The history still works after dropping the oldest entries.
	if _, ok := bus.Undo(); !ok {
		t.Error("undo broke after the cap kicked in")
	}
}

func TestEventsFireForEachChange(t *testing.T) {
	_, bus := testDoc(t)
	var got []Event
	bus.Events.Listen(func(e Event) { got = append(got, e) })

	b := addBody(t, bus)
	bus.Run(&SetBodyVisible{ID: b.ID, Visible: false})
	bus.Run(&SetPlaneVisible{Plane: geom.PlaneFront, Visible: false})

	want := []EventKind{EvBodyAdded, EvBodyChanged, EvPlanesChanged}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, k := range want {
		if got[i].Kind != k {
			t.Errorf("event %d = %v, want %v", i, got[i].Kind, k)
		}
	}
	// Undo notifies too, so caches rebuild.
	before := len(got)
	bus.Undo()
	if len(got) == before {
		t.Error("undo emitted no event")
	}
}

func TestFeatureLogGrowsAndIsNotRewound(t *testing.T) {
	doc, bus := testDoc(t)
	b := addBody(t, bus)
	bus.Run(&SetBodyVisible{ID: b.ID, Visible: false})
	if len(doc.Features) != 2 {
		t.Fatalf("feature log has %d entries, want 2", len(doc.Features))
	}
	if doc.Features[1].Kind != "Hide Body 1" {
		t.Errorf("feature kind = %q", doc.Features[1].Kind)
	}
	bus.Undo()
	if len(doc.Features) != 2 {
		t.Error("undo rewound the append-only feature log")
	}
}

func TestDirtyFlag(t *testing.T) {
	doc, bus := testDoc(t)
	if doc.DirtySinceSave {
		t.Error("a fresh document is already dirty")
	}
	addBody(t, bus)
	if !doc.DirtySinceSave {
		t.Error("adding a body left the document clean")
	}
	doc.DirtySinceSave = false
	bus.Undo()
	if !doc.DirtySinceSave {
		t.Error("undo left the document clean")
	}
}

func TestReplaceClearsHistory(t *testing.T) {
	_, bus := testDoc(t)
	addBody(t, bus)
	replaced := false
	bus.Events.Listen(func(e Event) {
		if e.Kind == EvDocReplaced {
			replaced = true
		}
	})
	bus.Replace(NewDocument())
	if bus.CanUndo() || bus.CanRedo() {
		t.Error("Replace left history behind")
	}
	if !replaced {
		t.Error("Replace did not announce itself")
	}
	if !bus.Doc().IsEmpty() {
		t.Error("Replace did not swap the document")
	}
}

func TestStatsLine(t *testing.T) {
	doc, bus := testDoc(t)
	if got := doc.Stats().String(); got != "0 bodies · 0 tris" {
		t.Errorf("empty stats = %q", got)
	}
	addBody(t, bus)
	if got := doc.Stats().String(); got != "1 body · 12 tris" {
		t.Errorf("one-body stats = %q, want \"1 body · 12 tris\"", got)
	}
	addBody(t, bus)
	if got := doc.Stats().String(); got != "2 bodies · 24 tris" {
		t.Errorf("two-body stats = %q", got)
	}
	if got := commas(1204); got != "1,204" {
		t.Errorf("commas(1204) = %q", got)
	}
	if got := commas(1234567); got != "1,234,567" {
		t.Errorf("commas(1234567) = %q", got)
	}
	if got := commas(-1234); got != "-1,234" {
		t.Errorf("commas(-1234) = %q", got)
	}
}

func TestSelection(t *testing.T) {
	doc, bus := testDoc(t)
	a := addBody(t, bus)
	b := addBody(t, bus)

	var sel Selection
	if !sel.Empty() || sel.Kind() != SelNone {
		t.Fatal("a fresh selection is not empty")
	}

	sel.Set(BodyRef(a.ID))
	if sel.Len() != 1 || !sel.Contains(BodyRef(a.ID)) {
		t.Fatal("Set did not select the body")
	}
	if got := sel.Describe(doc); got != "Body 1" {
		t.Errorf("describe = %q", got)
	}

	sel.Add(BodyRef(b.ID))
	if sel.Len() != 2 || sel.Kind() != SelBody {
		t.Fatalf("Add left %d refs of kind %v", sel.Len(), sel.Kind())
	}
	if got := sel.Describe(doc); got != "2 bodies" {
		t.Errorf("describe = %q, want \"2 bodies\"", got)
	}
	sel.Add(BodyRef(b.ID))
	if sel.Len() != 2 {
		t.Error("Add duplicated an existing reference")
	}

	sel.Toggle(BodyRef(b.ID))
	if sel.Len() != 1 {
		t.Error("Toggle did not remove")
	}
	sel.Toggle(BodyRef(b.ID))
	if sel.Len() != 2 {
		t.Error("Toggle did not re-add")
	}

	// A mixed selection has no single kind.
	sel.Add(PlaneRef(geom.PlaneTop))
	if sel.Kind() != SelNone {
		t.Error("a mixed selection reported a kind")
	}

	primary, ok := sel.Primary()
	if !ok || primary.Body != a.ID {
		t.Error("the primary selection is not the first one added")
	}

	// Deleting a body prunes it out of the selection.
	if err := bus.Run(&DeleteBody{ID: a.ID}); err != nil {
		t.Fatal(err)
	}
	sel.Prune(doc)
	if sel.Contains(BodyRef(a.ID)) {
		t.Error("a deleted body is still selected")
	}
	if !sel.Contains(BodyRef(b.ID)) || !sel.Contains(PlaneRef(geom.PlaneTop)) {
		t.Error("Prune dropped references that are still valid")
	}

	sel.Clear()
	if !sel.Empty() {
		t.Error("Clear left references behind")
	}
}

func TestSelectionDescribesVerticesCorrectly(t *testing.T) {
	doc := NewDocument()
	var sel Selection
	sel.SetAll([]Ref{VertRef(1, 0), VertRef(1, 1), VertRef(1, 2)})
	if got := sel.Describe(doc); got != "3 vertices" {
		t.Errorf("describe = %q, want \"3 vertices\"", got)
	}
}

func TestAutoBodyColorCycles(t *testing.T) {
	first := AutoBodyColor(0)
	if AutoBodyColor(len(autoBodyColors)) != first {
		t.Error("the colour cycle does not wrap")
	}
	if AutoBodyColor(-1) != autoBodyColors[len(autoBodyColors)-1] {
		t.Error("a negative index does not wrap cleanly")
	}
	seen := map[color.RGBA]bool{}
	for i := 0; i < len(autoBodyColors); i++ {
		c := AutoBodyColor(i)
		if seen[c] {
			t.Errorf("colour %v repeats within one cycle", c)
		}
		if c.A != 255 {
			t.Errorf("colour %v is not opaque", c)
		}
		seen[c] = true
	}
}

func TestAddBodyNeedsAMesh(t *testing.T) {
	_, bus := testDoc(t)
	if err := bus.Run(&AddBody{}); err == nil {
		t.Error("a body without a mesh was accepted")
	}
}

func TestSketchCommands(t *testing.T) {
	doc, bus := testDoc(t)
	cmd := &AddSketch{Plane: geom.PlaneFront}
	if err := bus.Run(cmd); err != nil {
		t.Fatal(err)
	}
	s := cmd.AddedSketch()
	if s.Name != "Sketch 1" || !s.Visible {
		t.Fatalf("new sketch = %+v", s)
	}
	if err := bus.Run(&RenameSketch{ID: s.ID, To: "Hull profile"}); err != nil {
		t.Fatal(err)
	}
	if doc.SketchByName("Hull profile") == nil {
		t.Error("the sketch cannot be found by its new name")
	}
	if err := bus.Run(&SetSketchVisible{ID: s.ID, Visible: false}); err != nil {
		t.Fatal(err)
	}
	if doc.SketchByID(s.ID).Visible {
		t.Error("the sketch is still visible")
	}
	if err := bus.Run(&DeleteSketch{ID: s.ID}); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sketches) != 0 {
		t.Error("the sketch was not deleted")
	}
	bus.Undo()
	if len(doc.Sketches) != 1 {
		t.Error("undo did not restore the sketch")
	}
}

func TestCommandsOnMissingObjectsFail(t *testing.T) {
	_, bus := testDoc(t)
	cmds := []Command{
		&SetBodyVisible{ID: 99},
		&SetSketchVisible{ID: 99},
		&RenameBody{ID: 99, To: "x"},
		&RenameSketch{ID: 99, To: "x"},
		&SetBodyColor{ID: 99},
		&DeleteBody{ID: 99},
		&DeleteSketch{ID: 99},
		&SetPlaneVisible{Plane: geom.PlaneKind(42)},
	}
	for _, c := range cmds {
		if err := bus.Run(c); err == nil {
			t.Errorf("%T on a missing object succeeded", c)
		}
	}
	if bus.UndoDepth() != 0 {
		t.Error("failed commands landed on the undo stack")
	}
}
