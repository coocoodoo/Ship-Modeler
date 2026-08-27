package app

import (
	"testing"

	"modeler/internal/io"
)

// The welcome card's state machine, which shipped with a loop in it: "New
// ship" replaces an empty document with an empty document, and an empty clean
// document is exactly what the card shows for — so the button appeared to do
// nothing at all. Answering the card, by any means, must put it away.

func TestWelcomeShowsOnAFreshEmptyDocument(t *testing.T) {
	a := bareApp()
	if !a.showWelcome() {
		t.Fatal("a fresh empty document should offer the welcome card")
	}
}

func TestNewShipAnswersTheWelcomeCard(t *testing.T) {
	t.Setenv(io.ConfigDirEnv, t.TempDir())
	a := bareApp()
	a.NewDocument()
	if a.showWelcome() {
		t.Fatal("New ship left the welcome card up — empty replaced empty and the card concluded nothing happened")
	}
	if !a.Doc().IsEmpty() || a.Doc().DirtySinceSave {
		t.Error("New ship should still produce an empty, clean document")
	}
}

func TestEscapeWavesTheWelcomeCardAway(t *testing.T) {
	a := bareApp()
	a.escape()
	if a.showWelcome() {
		t.Fatal("Escape did not dismiss the welcome card")
	}
}

func TestStartingWorkHidesTheWelcomeCard(t *testing.T) {
	a := bareApp()
	// Entering any mode hides it while the mode runs, dismissed or not.
	a.Mode = ModeSketch
	if a.showWelcome() {
		t.Error("the card should never sit over a working mode")
	}
	a.Mode = ModeIdle

	// A document with a home on disk is not a first launch.
	a.files.path = `C:\ships\scout.ship`
	if a.showWelcome() {
		t.Error("the card should not show over an opened document")
	}
}
