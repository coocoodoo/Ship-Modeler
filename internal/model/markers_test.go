package model

import (
	"testing"

	"modeler/internal/geom"
)

// The orientation markers' command behaviour (V-131): front and top are
// singletons that move rather than multiply, thrusters accumulate, and undo
// puts back exactly what was displaced.

func TestFrontDotMovesRatherThanMultiplies(t *testing.T) {
	bus := NewBus(NewDocument())
	first := Marker{Kind: MarkerFront, At: geom.Vec3{X: 1}, Dir: geom.Vec3{X: 1}}
	if err := bus.Run(&PlaceMarker{Marker: first}); err != nil {
		t.Fatal(err)
	}
	second := Marker{Kind: MarkerFront, At: geom.Vec3{X: 5}, Dir: geom.Vec3{X: 1}}
	if err := bus.Run(&PlaceMarker{Marker: second}); err != nil {
		t.Fatal(err)
	}
	doc := bus.Doc()
	if len(doc.Markers) != 1 {
		t.Fatalf("two front dots exist — a ship with two fronts obeys nobody")
	}
	if got, _ := doc.FrontMarker(); got.At != second.At {
		t.Errorf("front = %+v, want the newer placement", got)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if got, _ := doc.FrontMarker(); got.At != first.At {
		t.Errorf("undo gave %+v, want the first placement back", got)
	}
}

func TestThrustersAccumulateAndDeleteByIndex(t *testing.T) {
	bus := NewBus(NewDocument())
	for i := 0; i < 3; i++ {
		m := Marker{Kind: MarkerThruster, At: geom.Vec3{X: float64(i)}, Dir: geom.Vec3{Z: 1}}
		if err := bus.Run(&PlaceMarker{Marker: m}); err != nil {
			t.Fatal(err)
		}
	}
	doc := bus.Doc()
	if len(doc.Thrusters()) != 3 {
		t.Fatalf("thrusters = %d, want 3", len(doc.Thrusters()))
	}
	// The default radius arrives on its own.
	if doc.Markers[0].R != DefaultThrusterRadius {
		t.Errorf("r = %v, want the default %v", doc.Markers[0].R, DefaultThrusterRadius)
	}
	if err := bus.Run(&DeleteMarker{Index: 1}); err != nil {
		t.Fatal(err)
	}
	if len(doc.Markers) != 2 || doc.Markers[1].At.X != 2 {
		t.Fatalf("delete removed the wrong marker: %+v", doc.Markers)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if len(doc.Markers) != 3 || doc.Markers[1].At.X != 1 {
		t.Errorf("undo did not restore the marker in place: %+v", doc.Markers)
	}
}

func TestAMarkerNeedsADirection(t *testing.T) {
	bus := NewBus(NewDocument())
	err := bus.Run(&PlaceMarker{Marker: Marker{Kind: MarkerFront, At: geom.Vec3{X: 1}}})
	if err == nil {
		t.Fatal("a direction-less marker was accepted; the engine reads Dir")
	}
}

// Dragging a placed dot (the user's request, 2026-08-28). The gizmo moves a
// marker the same way it moves vertices: live through the bus, coalesced into
// one undo step, and exact on the way back.

func TestMoveMarkersShiftsOneDotAndUndoesExactly(t *testing.T) {
	bus := NewBus(NewDocument())
	place := func(k MarkerKind, x float64) {
		m := Marker{Kind: k, At: geom.Vec3{X: x}, Dir: geom.Vec3{Y: 1}}
		if err := bus.Run(&PlaceMarker{Marker: m}); err != nil {
			t.Fatal(err)
		}
	}
	place(MarkerThruster, 0)
	place(MarkerThruster, 10)
	doc := bus.Doc()

	before := doc.Markers[1]
	delta := geom.Vec3{X: 2, Y: -3, Z: 0.5}
	if err := bus.Run(NewMoveMarker(1, delta)); err != nil {
		t.Fatal(err)
	}
	if got, want := doc.Markers[1].At, before.At.Add(delta); got != want {
		t.Errorf("moved dot is at %v, want %v", got, want)
	}
	// Its neighbour is untouched: a gizmo on one dot moves one dot.
	if doc.Markers[0].At.X != 0 {
		t.Errorf("the other dot moved to %v", doc.Markers[0].At)
	}
	// The face normal is authored, not derived from position: sliding a dot
	// along a hull must not silently repoint a thruster's exhaust.
	if doc.Markers[1].Dir != before.Dir {
		t.Errorf("direction changed to %v, want %v", doc.Markers[1].Dir, before.Dir)
	}
	if doc.Markers[1].R != before.R {
		t.Errorf("radius changed to %v, want %v", doc.Markers[1].R, before.R)
	}

	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if doc.Markers[1].At != before.At {
		t.Errorf("undo left the dot at %v, want %v", doc.Markers[1].At, before.At)
	}
}

func TestMoveMarkersCoalescesADragIntoOneStep(t *testing.T) {
	bus := NewBus(NewDocument())
	if err := bus.Run(&PlaceMarker{Marker: Marker{
		Kind: MarkerFront, At: geom.Vec3{}, Dir: geom.Vec3{Z: 1},
	}}); err != nil {
		t.Fatal(err)
	}
	doc := bus.Doc()
	depth := bus.UndoDepth()

	// A drag is many frames of the same gesture. Each frame replaces the last,
	// so the total is the final delta and not the sum of every frame's.
	if err := bus.BeginDrag(NewMoveMarker(0, geom.Vec3{X: 1})); err != nil {
		t.Fatal(err)
	}
	for _, x := range []float64{2, 3, 4} {
		if err := bus.UpdateDrag(NewMoveMarker(0, geom.Vec3{X: x})); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := bus.CommitDrag(); !ok {
		t.Fatal("the drag did not commit")
	}
	if got := doc.Markers[0].At.X; got != 4 {
		t.Errorf("dot ended at x=%v, want 4 — the frames were summed", got)
	}
	if got := bus.UndoDepth(); got != depth+1 {
		t.Errorf("the drag left %d undo entries, want 1", got-depth)
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if got := doc.Markers[0].At.X; got != 0 {
		t.Errorf("one undo left the dot at x=%v, want 0", got)
	}
}

func TestMoveMarkersRefusesAnIndexThatIsNotThere(t *testing.T) {
	bus := NewBus(NewDocument())
	if err := bus.Run(NewMoveMarker(0, geom.Vec3{X: 1})); err == nil {
		t.Error("moving a dot that does not exist should fail, not no-op")
	}
}

// A marker selection has to survive the same pruning every other reference
// does: deleting a dot must not leave the gizmo pointing at a stale index.
func TestSelectionPrunesDeletedMarkers(t *testing.T) {
	doc := NewDocument()
	doc.Markers = []Marker{
		{Kind: MarkerThruster, At: geom.Vec3{X: 1}, Dir: geom.Vec3{Y: 1}},
		{Kind: MarkerThruster, At: geom.Vec3{X: 2}, Dir: geom.Vec3{Y: 1}},
	}
	var sel Selection
	sel.Set(MarkerRef(1))
	sel.Prune(doc)
	if sel.Len() != 1 {
		t.Fatalf("a live marker was pruned away")
	}
	doc.Markers = doc.Markers[:1]
	sel.Prune(doc)
	if sel.Len() != 0 {
		t.Errorf("a reference to a deleted marker survived: %+v", sel.Refs())
	}
}

func TestMarkerSelectionDescribesItself(t *testing.T) {
	doc := NewDocument()
	doc.Markers = []Marker{
		{Kind: MarkerFront, At: geom.Vec3{}, Dir: geom.Vec3{Z: 1}},
		{Kind: MarkerThruster, At: geom.Vec3{}, Dir: geom.Vec3{Z: 1}},
		{Kind: MarkerThruster, At: geom.Vec3{}, Dir: geom.Vec3{Z: 1}},
	}
	var sel Selection
	sel.Set(MarkerRef(0))
	if got := sel.Describe(doc); got != "Front dot" {
		t.Errorf("describe = %q, want %q", got, "Front dot")
	}
	// Thrusters are numbered as the tree numbers them, so the hint bar and the
	// row agree about which one is selected.
	sel.Set(MarkerRef(2))
	if got := sel.Describe(doc); got != "Thruster 2" {
		t.Errorf("describe = %q, want %q", got, "Thruster 2")
	}
}

func TestMarkerPivotIsTheDotItself(t *testing.T) {
	doc := NewDocument()
	at := geom.Vec3{X: 3, Y: 4, Z: 5}
	doc.Markers = []Marker{{Kind: MarkerTop, At: at, Dir: geom.Vec3{Y: 1}}}
	var sel Selection
	sel.Set(MarkerRef(0))
	p, ok := sel.Pivot(doc)
	if !ok {
		t.Fatal("no pivot for a selected dot — the gizmo would never arm")
	}
	if p != at {
		t.Errorf("pivot = %v, want the dot's own position %v", p, at)
	}
}

func TestMoveMarkersMovesEveryDotItWasGiven(t *testing.T) {
	bus := NewBus(NewDocument())
	for i := 0; i < 3; i++ {
		if err := bus.Run(&PlaceMarker{Marker: Marker{
			Kind: MarkerThruster, At: geom.Vec3{X: float64(i)}, Dir: geom.Vec3{Z: 1},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	doc := bus.Doc()
	delta := geom.Vec3{Y: 2}
	if err := bus.Run(&MoveMarkers{Indices: []int{0, 2}, Delta: delta}); err != nil {
		t.Fatal(err)
	}
	if doc.Markers[0].At.Y != 2 || doc.Markers[2].At.Y != 2 {
		t.Errorf("the selected dots did not both move: %+v", doc.Markers)
	}
	if doc.Markers[1].At.Y != 0 {
		t.Errorf("an unselected dot moved: %+v", doc.Markers[1])
	}
	if _, ok := bus.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	for i, m := range doc.Markers {
		if m.At.Y != 0 {
			t.Errorf("dot %d is at y=%v after undo, want 0", i, m.At.Y)
		}
	}
}

// A set with one bad index must change nothing at all — the atomicity contract
// of SPEC-DATA §3.1, which is what makes a failed command safe to ignore.
func TestMoveMarkersIsAllOrNothing(t *testing.T) {
	bus := NewBus(NewDocument())
	if err := bus.Run(&PlaceMarker{Marker: Marker{
		Kind: MarkerFront, At: geom.Vec3{}, Dir: geom.Vec3{Z: 1},
	}}); err != nil {
		t.Fatal(err)
	}
	doc := bus.Doc()
	if err := bus.Run(&MoveMarkers{Indices: []int{0, 7}, Delta: geom.Vec3{X: 5}}); err == nil {
		t.Fatal("a set naming a dot that is not there should fail")
	}
	if doc.Markers[0].At.X != 0 {
		t.Errorf("the good dot moved anyway, to %v", doc.Markers[0].At)
	}
}
