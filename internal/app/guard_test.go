package app

import (
	"testing"

	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/ui"
)

// The unsaved-work guard and the close prompt's answer routing, tested without
// a GPU: everything here is state machinery, and it guards against exactly one
// disaster — an action or a keystroke quietly discarding work.

// bareApp is the minimum App these flows touch. No renderer, no fonts, no
// window: ShowModal, the file queue and the settings are plain fields.
func bareApp() *App {
	return &App{
		Bus:      model.NewBus(model.NewDocument()),
		UI:       ui.NewContext(nil, 1),
		Settings: io.DefaultSettings(),
	}
}

func TestDestructiveActionsStopAtTheGuardWhenDirty(t *testing.T) {
	a := bareApp()
	a.Doc().DirtySinceSave = true

	a.RequestFile(fileNew)
	if a.files.pending != fileNone {
		t.Error("a dirty document let Ctrl+N straight through")
	}
	if a.files.confirm != fileNew {
		t.Errorf("the guard parked %v, want fileNew", a.files.confirm)
	}
	if !a.UI.ModalOpen() {
		t.Error("the guard did not put its question up")
	}
}

func TestCleanDocumentsSkipTheGuard(t *testing.T) {
	a := bareApp()
	a.RequestFile(fileNew)
	if a.files.pending != fileNew {
		t.Error("a clean document was asked about discarding nothing")
	}
	if a.UI.ModalOpen() {
		t.Error("a modal opened with nothing to lose")
	}
}

func TestNonDestructiveActionsNeverAsk(t *testing.T) {
	a := bareApp()
	a.Doc().DirtySinceSave = true
	for _, act := range []fileAction{fileSave, fileSaveAs, fileExport, fileImportPalette} {
		a.RequestFile(act)
		if a.files.pending != act || a.UI.ModalOpen() {
			t.Errorf("action %v was guarded; saving is how work stops being at risk", act)
		}
		a.files.pending = fileNone
	}
}

func TestGuardedOpenKeepsItsPath(t *testing.T) {
	a := bareApp()
	a.Doc().DirtySinceSave = true
	a.RequestOpenPath(`C:\ships\scout.ship`)
	if a.files.confirm != fileOpenPath || a.files.confirmPath != `C:\ships\scout.ship` {
		t.Fatalf("guard parked %v %q", a.files.confirm, a.files.confirmPath)
	}

	// Confirming releases exactly the parked action into the queue.
	a.routeModalAnswer(ui.ModalResult{Confirmed: true})
	if a.files.pending != fileOpenPath || a.files.pendingPath != `C:\ships\scout.ship` {
		t.Errorf("confirm queued %v %q", a.files.pending, a.files.pendingPath)
	}
	if a.files.confirm != fileNone {
		t.Error("the parked action was not cleared")
	}
}

func TestDecliningTheGuardDropsTheAction(t *testing.T) {
	for _, res := range []ui.ModalResult{{Cancelled: true}, {Dismissed: true}} {
		a := bareApp()
		a.Doc().DirtySinceSave = true
		a.RequestFile(fileSample)
		a.routeModalAnswer(res)
		if a.files.pending != fileNone || a.files.confirm != fileNone {
			t.Errorf("%+v left pending=%v confirm=%v; declining must do nothing at all",
				res, a.files.pending, a.files.confirm)
		}
	}
}

// The close prompt: Confirm saves, the labelled button discards, and Escape —
// the reflex key — keeps working. The discard path must demand the words.
func TestClosePromptAnswers(t *testing.T) {
	cases := []struct {
		res  ui.ModalResult
		want closeAnswerKind
	}{
		{ui.ModalResult{Confirmed: true}, closeAnswerSave},
		{ui.ModalResult{Cancelled: true}, closeAnswerDiscard},
		{ui.ModalResult{Dismissed: true}, closeAnswerNone},
	}
	for _, tc := range cases {
		a := bareApp()
		a.Doc().DirtySinceSave = true
		a.RequestClose()
		if a.closing != closeAsking || !a.UI.ModalOpen() {
			t.Fatal("RequestClose on a dirty document did not ask")
		}
		a.closeAnswer = closeAnswerKind(99) // a sentinel no case sets
		a.routeModalAnswer(tc.res)
		if a.closeAnswer != tc.want {
			t.Errorf("%+v routed to %v, want %v", tc.res, a.closeAnswer, tc.want)
		}
	}
}

// Escape's dismissal must land the state machine back at "not asked": the
// window stays open and a later close asks again.
func TestDismissedClosePromptKeepsWorking(t *testing.T) {
	a := bareApp()
	a.Doc().DirtySinceSave = true
	a.RequestClose()
	a.UI.CloseModal() // DrawModal closes itself on any outcome
	a.routeModalAnswer(ui.ModalResult{Dismissed: true})
	a.stepClose()
	if a.ShouldClose() {
		t.Fatal("Escape on the close prompt closed the program")
	}
	if a.closing != closeNotAsked {
		t.Errorf("closing = %v, want closeNotAsked so the next close asks again", a.closing)
	}
}

func TestCleanCloseNeverAsks(t *testing.T) {
	a := bareApp()
	a.RequestClose()
	if !a.ShouldClose() || a.UI.ModalOpen() {
		t.Error("a clean document should close without a question")
	}
}
