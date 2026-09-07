package apptest

import (
	"strings"
	"testing"
)

func TestAttachmentMarkerDialog(t *testing.T) {
	out, dir := runScript(t, "attachment_markers")
	for _, label := range []string{`name="Ship Part A[Left wing]"`, `name="Ship Part Z[Engine]"`, `name="Ship Part B[Dock]"`} {
		if !strings.Contains(out, label) {
			t.Fatalf("missing attachment %s:\n%s", label, out)
		}
	}
	if strings.Contains(out, "attachment index=2") {
		t.Fatal("Cancel placed an attachment")
	}
	checkGolden(t, "attachment_markers", dir)
}

func TestAttachmentMarkerPages(t *testing.T) {
	out, dir := runScript(t, "attachment_many")
	if !strings.Contains(out, `attachment index=25 name="Ship Part Z[Socket]"`) {
		t.Fatal("did not retain the full A-Z set")
	}
	if !strings.Contains(out, "marker-page=6") {
		t.Fatal("could not reach the last page and navigate back")
	}
	checkGolden(t, "attachment_many", dir)
}

func TestAttachmentPairColors(t *testing.T) {
	_, dir := runScript(t, "attachment_pairs")
	checkGolden(t, "attachment_pairs", dir)
}
