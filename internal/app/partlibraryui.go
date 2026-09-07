package app

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/ui"
	"strings"
)

func (a *App) buildLibrarySection(rest rl.Rectangle, rowH float32) rl.Rectangle {
	head, rest := ui.SplitTop(rest, rowH)
	if a.UI.TreeSection(ui.MakeID("tree.section.library"), head, "Library", len(a.library.parts), a.library.sectionOpen) {
		a.library.sectionOpen = !a.library.sectionOpen
	}
	if !a.library.sectionOpen {
		return rest
	}
	row, rest := ui.SplitTop(rest, a.px(20))
	a.emptyRow(row, "Shared parts · not exported")
	row, rest = ui.SplitTop(rest, a.px(27))
	if a.UI.Button(ui.MakeID("library.browse"), ui.InsetXY(row, a.px(12), a.px(2)), "Browse library", ui.ButtonOpts{Disabled: a.Mode != ModeIdle, DisabledWhy: "Finish the current tool first"}) {
		a.refreshPartLibrary()
		a.library.open = true
		a.library.page = 0
		a.UI.ClearFocus()
	}
	row, rest = ui.SplitTop(rest, a.px(27))
	if a.UI.Button(ui.MakeID("library.save.selected"), ui.InsetXY(row, a.px(12), a.px(2)), "Save selected to Library", ui.ButtonOpts{Disabled: a.Mode != ModeIdle || len(a.selectedLibrarySources()) == 0, DisabledWhy: "Select a body first"}) {
		a.beginSaveToLibrary(a.selectedLibrarySources())
	}
	if a.libraryEditAvailable() {
		row, rest = ui.SplitTop(rest, a.px(27))
		if a.UI.Button(ui.MakeID("library.update.edit"), ui.InsetXY(row, a.px(12), a.px(2)), "Save library changes", ui.ButtonOpts{
			Disabled: a.Mode != ModeIdle, DisabledWhy: "Finish the current tool first", Tooltip: "Update " + a.library.editPart.Name,
		}) {
			a.beginUpdateLibraryPart(a.library.editPart, a.library.editBodies)
		}
	}
	return rest
}
func (a *App) libraryDialogBox(w, h float64) rl.Rectangle {
	vp := a.layout.Viewport
	width, height := min(a.px(w), vp.Width-a.px(16)), min(a.px(h), vp.Height-a.px(16))
	return ui.Rect(vp.X+(vp.Width-width)/2, vp.Y+(vp.Height-height)/2, width, height)
}
func (a *App) buildLibraryDialogs() {
	if !a.library.open {
		a.dropLibraryPreview()
	}
	// File/close confirmation dialogs have priority over library input.
	if a.UI.ModalOpen() {
		return
	}
	if a.library.saveOpen {
		a.buildSaveLibraryDialog()
	} else if a.library.open {
		a.buildPartLibraryBrowser()
	}
}
func (a *App) buildSaveLibraryDialog() {
	st := &a.library
	if st.categoryMenu {
		a.buildLibraryCategoryMenu()
	}
	box := a.libraryDialogBox(380, 300)
	title, confirm := "Save to Library", "Save"
	if st.updatePart.ID != "" {
		title, confirm = "Update library part", "Update"
	}
	card := a.UI.FloatingCard(ui.MakeID("library.save.card"), box, title, ui.FloatingCardOpts{Footer: true, ConfirmLabel: confirm, CancelLabel: "Cancel", ConfirmDisabled: strings.TrimSpace(st.name) == "", ConfirmWhy: "Enter a part name"})
	body := card.Body
	row := func(h float32) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, h); return r }
	a.UI.Text(row(a.px(20)), "Name", ui.FontSizeSmall, ui.ColorTextDim)
	name := a.UI.TextField(ui.MakeID("library.name"), row(a.px(28)), st.name, ui.TextFieldOpts{SelectAllOnFocus: true, Placeholder: "Part name"})
	st.name = name.Text
	row(a.px(10))
	a.UI.Text(row(a.px(20)), "Category — choose one or type a new one", ui.FontSizeSmall, ui.ColorTextDim)
	catRow := row(a.px(28))
	st.categoryX, st.categoryY, st.categoryW = catRow.X, catRow.Y+catRow.Height, catRow.Width
	field, button := ui.SplitLeft(catRow, catRow.Width-a.px(32))
	category := a.UI.TextField(ui.MakeID("library.category"), field, st.category, ui.TextFieldOpts{SelectAllOnFocus: true, Placeholder: "Uncategorized"})
	st.category = category.Text
	if a.UI.ChevronButton(ui.MakeID("library.category.choose"), button, st.categoryMenu, "Choose an existing category") {
		st.categoryMenu = !st.categoryMenu
		st.categoryPage = 0
		a.UI.ClearFocus()
	}
	row(a.px(10))
	hint := "Saved on this computer for every project. Library parts stay out of exports until inserted."
	if st.updatePart.ID != "" {
		hint = "Replaces " + st.updatePart.Name + " in the library. Existing project copies stay unchanged."
	}
	a.UI.TextWrapped(row(a.px(36)), hint, ui.FontSizeSmall, ui.ColorTextDim)
	if st.err != "" {
		a.UI.TextWrapped(row(a.px(42)), st.err, ui.FontSizeSmall, ui.ColorError)
	}
	a.UI.ClaimPointer(a.layout.Screen)
	if card.Cancelled || a.UI.In.KeyPressed(rl.KeyEscape) {
		st.saveOpen = false
		st.sources = nil
		st.categoryMenu = false
		a.UI.ClearFocus()
		return
	}
	if card.Confirmed {
		a.saveLibraryPart()
	}
}
func (a *App) buildLibraryCategoryMenu() {
	st := &a.library
	cats := a.libraryCategories()
	start := min(st.categoryPage*6, max(0, len(cats)-1))
	end := min(start+6, len(cats))
	items := []ui.MenuItem{{Label: "Uncategorized"}}
	for _, c := range cats[start:end] {
		items = append(items, ui.MenuItem{Label: c, Selected: c == st.category})
	}
	if len(cats) > 6 {
		items = append(items, ui.MenuItem{Label: "More categories…"})
	}
	mr := a.UI.Menu(ui.MakeID("library.categories"), ui.Rect(st.categoryX, st.categoryY, st.categoryW, a.px(float64(8+26*len(items)))), items)
	if mr.Dismissed {
		st.categoryMenu = false
	}
	if mr.Chosen >= 0 {
		if mr.Chosen == len(items)-1 && len(cats) > 6 {
			st.categoryPage++
			if st.categoryPage*6 >= len(cats) {
				st.categoryPage = 0
			}
		} else {
			st.category = items[mr.Chosen].Label
			st.categoryMenu = false
			a.UI.ClearFocus()
		}
	}
}
func (a *App) buildPartLibraryBrowser() {
	st := &a.library
	box := a.libraryDialogBox(480, 486)
	shown := a.filteredLibraryParts()
	valid := false
	for _, p := range shown {
		if p.ID == st.selected {
			valid = true
		}
	}
	card := a.UI.FloatingCard(ui.MakeID("library.browser"), box, "Parts Library", ui.FloatingCardOpts{Footer: true, ConfirmLabel: "Insert copy", CancelLabel: "Close", ConfirmDisabled: !valid, ConfirmWhy: "Choose a part to insert"})
	body := card.Body
	row := func(h float32) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, h); return r }
	a.UI.Text(row(a.px(22)), "Shared across projects · Library originals never export", ui.FontSizeSmall, ui.ColorTextDim)
	search := row(a.px(28))
	field, refresh := ui.SplitLeft(search, search.Width-a.px(72))
	res := a.UI.TextField(ui.MakeID("library.search"), field, st.query, ui.TextFieldOpts{Placeholder: "Search names or categories", LeadingIcon: ui.DrawSearchIcon})
	if res.Text != st.query {
		st.query = res.Text
		st.page = 0
	}
	if a.UI.Button(ui.MakeID("library.refresh"), refresh, "Refresh", ui.ButtonOpts{Icon: ui.DrawRefreshIcon}) {
		a.refreshPartLibrary()
	}
	row(a.px(8))
	// Reserve most of the browser for the selected part's live turntable.
	perPage := max(1, min(3, int((body.Height-a.px(55))*0.3/a.px(42))))
	listBottom := body.Y + float32(perPage)*a.px(42)
	pages := max(1, (len(shown)+perPage-1)/perPage)
	st.page = min(st.page, pages-1)
	start, end := st.page*perPage, min((st.page+1)*perPage, len(shown))
	for _, part := range shown[start:end] {
		r := row(a.px(42))
		title, cat := ui.SplitTop(r, a.px(24))
		rr := a.UI.TreeRow(ui.MakeID("library.part."+part.ID), title, ui.TreeRowSpec{Label: part.Name, Icon: ui.DrawBodyIcon, Selected: st.selected == part.ID})
		cat.X += a.px(24)
		a.UI.Text(cat, part.Category, ui.FontSizeSmall, ui.ColorTextDim)
		if rr.Clicked {
			st.selected = part.ID
		}
		if rr.DoubleClicked {
			st.selected = part.ID
			a.insertLibraryPart(part.ID)
		}
	}
	if len(shown) == 0 {
		a.UI.TextWrapped(row(a.px(48)), "No parts found. Right-click a model body and choose Save to Library.", ui.FontSizeSmall, ui.ColorTextDim)
	}
	nav := ui.Rect(body.X, box.Y+box.Height-a.px(84), body.Width, a.px(25))
	actions := ui.Rect(body.X, nav.Y-a.px(36), body.Width, a.px(28))
	edit, replace := ui.SplitLeft(actions, a.px(130))
	replace = ui.InsetXY(replace, a.px(4), 0)
	if a.UI.Button(ui.MakeID("library.edit"), edit, "Edit part", ui.ButtonOpts{Disabled: !valid, DisabledWhy: "Choose a library part"}) {
		a.editLibraryPart(st.selected)
	}
	if a.UI.Button(ui.MakeID("library.replace"), replace, "Update from selection", ui.ButtonOpts{Disabled: !valid || len(a.selectedLibrarySources()) == 0, DisabledWhy: "Select a model body and a library part"}) {
		for _, part := range st.parts {
			if part.ID == st.selected {
				a.beginUpdateLibraryPart(part, a.selectedLibrarySources())
				break
			}
		}
	}
	previewBottom := actions.Y - a.px(8)
	if st.err != "" {
		previewBottom -= a.px(22)
	}
	a.buildLibraryPreview(ui.Rect(body.X, listBottom+a.px(6), body.Width, max(0, previewBottom-listBottom-a.px(6))), a.filteredLibraryParts())
	prev, rest := ui.SplitLeft(nav, a.px(70))
	next, label := ui.SplitRight(rest, a.px(70))
	if a.UI.Button(ui.MakeID("library.prev"), prev, "Previous", ui.ButtonOpts{Disabled: st.page == 0}) {
		st.page--
	}
	a.UI.Text(label, fmt.Sprintf("  %d / %d", st.page+1, pages), ui.FontSizeSmall, ui.ColorTextDim)
	if a.UI.Button(ui.MakeID("library.next"), next, "Next", ui.ButtonOpts{Disabled: st.page+1 >= pages}) {
		st.page++
	}
	if st.err != "" {
		a.UI.Text(ui.Rect(body.X, nav.Y-a.px(22), body.Width, a.px(20)), st.err, ui.FontSizeSmall, ui.ColorError)
	}
	a.UI.ClaimPointer(a.layout.Screen)
	if card.Cancelled || a.UI.In.KeyPressed(rl.KeyEscape) {
		st.open = false
		a.dropLibraryPreview()
		a.UI.ClearFocus()
		return
	}
	if card.Confirmed {
		a.insertLibraryPart(st.selected)
	}
}
