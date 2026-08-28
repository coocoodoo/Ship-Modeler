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
