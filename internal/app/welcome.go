package app

import (
	"fmt"
	"path/filepath"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/ui"
)

// The welcome and empty states of SPEC-UX §14.
//
// The rule the section is really about: an empty viewport must never be a blank
// stare. A first launch offers somewhere to start, a document with nothing in it
// says what to do first, and a session that ended badly offers the work back
// before anything else happens.

// WelcomeWidth is the card's width in logical pixels.
const (
	welcomeWidth   = 380
	welcomeRow     = 30
	welcomeGap     = 8
	recentRowCount = 5
)

// showWelcome reports whether the centre card should be up: an untouched, empty
// document that has never been saved — and that the user has not yet answered.
//
// The dismissed flag is the part that makes "New ship" work. A new document is
// an empty, clean, unnamed one, which is exactly the state this card shows
// for: without the flag the button replaced the empty document with another
// empty document and the card concluded it should still be up. Any answer —
// a button, Escape, or starting work in the viewport — puts it away for the
// session; from there the hint bar carries the same guidance.
func (a *App) showWelcome() bool {
	if a.files.welcomeDismissed || a.files.path != "" || a.Mode != ModeIdle {
		return false
	}
	return a.Doc().IsEmpty() && !a.Doc().DirtySinceSave
}

// dismissWelcome puts the card away for the rest of the session.
func (a *App) dismissWelcome() { a.files.welcomeDismissed = true }

// buildWelcome draws the centre card, or the recovery offer that outranks it.
func (a *App) buildWelcome(viewport rl.Rectangle) {
	if a.HasRecovery() {
		a.buildRecoveryCard(viewport)
		return
	}
	if !a.showWelcome() {
		return
	}

	recents := a.recentEntries()
	h := a.px(46 + welcomeRow*3 + welcomeGap*4)
	if len(recents) > 0 {
		h += a.px(20) + float32(len(recents))*a.px(welcomeRow-4)
	}
	box := a.centreCard(viewport, a.px(welcomeWidth), h)

	card := a.UI.FloatingCard(ui.MakeID("welcome"), box, "Modeler", ui.FloatingCardOpts{})
	body := card.Body
	row := func(height float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, height)
		return r
	}
	gap := func() {
		body.Y += a.px(welcomeGap)
		body.Height -= a.px(welcomeGap)
	}

	if a.UI.Button(ui.MakeID("welcome.new"), row(a.px(welcomeRow)), "New ship",
		ui.ButtonOpts{Style: ui.ButtonPrimary, Shortcut: "Ctrl+N",
			Tooltip: "Start from nothing — click a plane, then sketch"}) {
		a.RequestFile(fileNew)
	}
	gap()
	if a.UI.Button(ui.MakeID("welcome.open"), row(a.px(welcomeRow)), "Open…",
		ui.ButtonOpts{Shortcut: "Ctrl+O", Tooltip: "Open a ship you saved earlier"}) {
		a.RequestFile(fileOpen)
	}
	gap()
	if a.UI.Button(ui.MakeID("welcome.sample"), row(a.px(welcomeRow)), "Sample ship",
		ui.ButtonOpts{
			Tooltip: "A ship to take apart and see how it was made",
		}) {
		a.RequestFile(fileSample)
	}

	if len(recents) == 0 {
		return
	}
	gap()
	a.UI.Text(row(a.px(20)), "Recent", ui.FontSizeSmall, ui.ColorTextDim)
	for i, entry := range recents {
		r := row(a.px(welcomeRow - 4))
		if a.UI.Button(ui.MakeID("welcome.recent"+itoa(i)), r, entry.label,
			ui.ButtonOpts{Style: ui.ButtonGhost, Tooltip: entry.path}) {
			a.RequestOpenPath(entry.path)
		}
	}
}

// buildRecoveryCard offers back work from a session that did not end cleanly.
//
// It comes up before anything else and takes the pointer, because the one thing
// that must not happen is the user starting to work and burying the offer.
func (a *App) buildRecoveryCard(viewport rl.Rectangle) {
	rec := a.files.recovery[0]
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	h := a.px(46) + line*3 + a.px(welcomeRow+welcomeGap*2)
	box := a.centreCard(viewport, a.px(welcomeWidth), h)

	card := a.UI.FloatingCard(ui.MakeID("recover"), box, "Unsaved work found",
		ui.FloatingCardOpts{})
	body := card.Body
	row := func(height float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, height)
		return r
	}

	a.UI.Text(row(line), rec.Describe(), ui.FontSizeUI, ui.ColorText)
	a.UI.Text(row(line), "Modeler stopped before this was saved.",
		ui.FontSizeSmall, ui.ColorTextDim)
	if n := len(a.files.recovery) - 1; n > 0 {
		a.UI.Text(row(line), fmt.Sprintf("%s older behind it.",
			plural(n, "one", "more")), ui.FontSizeSmall, ui.ColorTextDim)
	} else {
		row(line)
	}

	body.Y += a.px(welcomeGap)
	body.Height -= a.px(welcomeGap)
	buttons := row(a.px(welcomeRow))
	left, right := ui.SplitLeft(buttons, buttons.Width*0.5)
	right.X += a.px(6)
	right.Width -= a.px(6)

	if a.UI.Button(ui.MakeID("recover.discard"), left, "Discard", ui.ButtonOpts{
		Tooltip: "Throw it away and start fresh",
	}) {
		a.DiscardRecovery()
	}
	if a.UI.Button(ui.MakeID("recover.restore"), right, "Restore", ui.ButtonOpts{
		Style:   ui.ButtonPrimary,
		Tooltip: "Open it — it comes back unsaved, so save it somewhere",
	}) {
		a.RecoverNewest()
	}
}

// centreCard is a box in the middle of the viewport.
func (a *App) centreCard(viewport rl.Rectangle, w, h float32) rl.Rectangle {
	return ui.Rect(
		viewport.X+(viewport.Width-w)/2,
		viewport.Y+(viewport.Height-h)/2,
		w, h)
}

// recentEntry is one row of the recent list.
type recentEntry struct {
	path  string
	label string
}

// recentEntries is the most-recently-used list, shortened for display.
func (a *App) recentEntries() []recentEntry {
	var out []recentEntry
	for _, p := range a.Settings.RecentFiles {
		if len(out) >= recentRowCount {
			break
		}
		name := strings.TrimSuffix(filepath.Base(p), ".pxm")
		name = strings.TrimSuffix(name, ".ship")
		if dir := filepath.Base(filepath.Dir(p)); dir != "." && dir != "" {
			name = name + "  ·  " + dir
		}
		out = append(out, recentEntry{path: p, label: name})
	}
	return out
}
