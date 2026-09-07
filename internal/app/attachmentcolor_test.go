package app

import (
	"modeler/internal/model"
	"testing"
)

func TestAttachmentPairColorFollowsAppendAcrossLetters(t *testing.T) {
	a := model.Marker{Kind: model.MarkerAttachment, Slot: "A", AppendText: "Wing"}
	b := model.Marker{Kind: model.MarkerAttachment, Slot: "Z", AppendText: " wing "}
	if placedMarkerColor(a) != placedMarkerColor(b) {
		t.Fatal("matching append text has different colors across letters")
	}
	b.AppendText = "Engine"
	if placedMarkerColor(a) == placedMarkerColor(b) {
		t.Fatal("wing and engine pairs have the same color")
	}
	if attachmentPairColor("") != markerColor(model.MarkerAttachment) {
		t.Fatal("unnamed attachments lost their default color")
	}
	for _, kind := range []model.MarkerKind{model.MarkerFront, model.MarkerTop, model.MarkerThruster} {
		if placedMarkerColor(model.Marker{Kind: kind, AppendText: "Wing"}) != markerColor(kind) {
			t.Fatal("pair colors changed another marker kind")
		}
	}
}
