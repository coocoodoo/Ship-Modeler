package model

import (
	"modeler/internal/geom"
	"testing"
)

func TestAttachmentLettersAccumulateAndRenameUndo(t *testing.T) {
	bus := NewBus(NewDocument())
	for letter := 'A'; letter <= 'Z'; letter++ {
		if err := bus.Run(&PlaceMarker{Marker: Marker{Kind: MarkerAttachment, Slot: string(letter), AppendText: " Wing ", At: geom.Vec3{X: float64(letter - 'A')}, Dir: geom.AxisZ}}); err != nil {
			t.Fatal(err)
		}
	}
	doc := bus.Doc()
	if len(doc.Markers) != 26 || doc.MarkerLabel(25) != "Ship Part Z[Wing]" {
		t.Fatalf("lost slots: %+v", doc.Markers)
	}
	before := doc.Markers[0]
	if err := bus.Run(&RenameAttachment{Index: 0, Slot: "b", AppendText: "Engine"}); err != nil {
		t.Fatal(err)
	}
	if doc.MarkerLabel(0) != "Ship Part B[Engine]" || doc.Markers[0].At != before.At || doc.Markers[0].Dir != before.Dir {
		t.Fatal("rename changed position or lost label")
	}
	bus.Undo()
	if doc.Markers[0] != before {
		t.Fatal("rename undo failed")
	}
	bus.Redo()
	if doc.MarkerLabel(0) != "Ship Part B[Engine]" {
		t.Fatal("rename redo failed")
	}
	if err := bus.Run(&PlaceMarker{Marker: before}); err != nil {
		t.Fatal(err)
	}
	if len(doc.Markers) != 27 {
		t.Fatal("same-letter sockets replaced one another")
	}
	bus.Undo()
	if len(doc.Markers) != 26 {
		t.Fatal("placement undo failed")
	}
}

func TestAttachmentRejectsInvalidNameWithoutMutation(t *testing.T) {
	doc := NewDocument()
	for _, slot := range []string{"", "AA", "7", "[", "é"} {
		if err := (&PlaceMarker{Marker: Marker{Kind: MarkerAttachment, Slot: slot, Dir: geom.AxisY}}).Do(doc); err == nil {
			t.Fatalf("accepted slot %q", slot)
		}
	}
	if len(doc.Markers) != 0 {
		t.Fatal("failed placement changed the document")
	}
	if _, _, err := CleanAttachmentName("A", "left\nwing"); err == nil {
		t.Fatal("accepted multiline label")
	}
}
