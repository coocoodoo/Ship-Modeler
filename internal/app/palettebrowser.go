package app

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/io"
	"modeler/internal/paint"
	"modeler/internal/ui"
)

// The palette browser (SPEC-UX §13.3, V-152): the shipped library as a list
// you search and click, rather than a file dialog you have to already know
// the answer to.
//
// A few thousand palettes is too many to page through, so the list is one
// search box over one scrolling column, and every row shows its own colours —
// the name of a palette tells you almost nothing, and the strip tells you
// everything.

// Browser metrics in logical pixels.
const (
	paletteCardWidth  = 468
	paletteRowHeight  = 30
	paletteStripCount = 14 // swatches previewed per row
	paletteWheelRows  = 3  // rows moved per wheel notch
	paletteMinRows    = 6
)

// paletteBrowser is the list's own state. None of it is document state: it is
// a way of choosing, and what it chooses lands in the paint panel.
type paletteBrowser struct {
	open  bool
	query string
	// first is the topmost visible row. The list scrolls by whole rows, so
	// every row drawn is a row entirely inside the box — no half rows to clip
	// and no half rows that are visible but not clickable.
	first int
	// shown indexes the library, narrowed by the query. Rebuilt only when the
	// query changes: filtering four thousand names every frame is work done
	// four thousand times for nothing.
	shown    []int
	shownFor string
	// applied is the name of the palette on the custom page, so the list can
	// show which one you are painting with.
	applied string
	// library is the shipped bundle plus the user's own folder, merged once.
	library []paint.Palette
	loaded  bool
}

// InPaletteBrowser reports whether the list is up.
func (a *App) InPaletteBrowser() bool { return a.paint.browser.open }

// PaletteLibrary is every palette on offer: the thousands bundled with the
// program, plus any the user has dropped in their own folder beside the
// settings. Merged once and kept, because the answer cannot change while the
// program is running without the user having gone and edited the folder.
func (a *App) PaletteLibrary() []paint.Palette {
	b := &a.paint.browser
	if b.loaded {
		return b.library
	}
	b.loaded = true
	lib := paint.Library()
	if dir, err := io.SettingsDir(); err == nil {
		if mine := paint.ScanFolder(filepath.Join(dir, PaletteFolderName)); len(mine) > 0 {
			// The user's own come first: a folder you filled yourself is not a
			// needle to find in someone else's haystack.
			lib = append(mine, lib...)
		}
	}
	// Names come from thousands of files by thousands of authors, and the font
	// atlas is a curated codepoint list (D-11) — anything outside it draws as a
	// box. So every name is made renderable before it can reach a row or a
	// toast, rather than trusting four thousand strangers to have typed ASCII.
	b.library = make([]paint.Palette, len(lib))
	for i, p := range lib {
		p.Name = renderableName(p.Name)
		b.library[i] = p
	}
	return b.library
}

// asciiFolds are the punctuation worth keeping as its nearest drawable twin;
// everything else outside the atlas is simply dropped.
var asciiFolds = map[rune]rune{
	'‚': ',', '„': '"', '‛': '\'', '‟': '"',
	'ʼ': '\'', '`': '\'', '´': '\'', '′': '\'', '″': '"',
	'‐': '-', '‑': '-', '−': '-', '﹘': '-', '～': '~',
	'　': ' ', ' ': ' ',
}

// renderableName reduces a palette's name to glyphs the atlas can draw.
func renderableName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if ui.CanRender(r) {
			b.WriteRune(r)
			continue
		}
		if fold, ok := asciiFolds[r]; ok && ui.CanRender(fold) {
			b.WriteRune(fold)
			continue
		}
		// An emoji or a script the atlas has never carried: drop it, and let
		// the space it sat in close up below.
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if out == "" {
		return "Untitled palette"
	}
	return out
}

// PaletteFolderName is where a user's own palettes live, beside the settings.
const PaletteFolderName = "palettes"

// OpenPaletteBrowser shows the list, starting from the palette in use.
func (a *App) OpenPaletteBrowser() {
	a.paint.browser.open = true
	a.PaletteLibrary()
	a.revealAppliedPalette()
}

// ClosePaletteBrowser hides it, keeping whatever was applied.
func (a *App) ClosePaletteBrowser() { a.paint.browser.open = false }

// paletteShown is the filtered index list for the current query.
func (a *App) paletteShown() []int {
	b := &a.paint.browser
	if b.shown != nil && b.shownFor == b.query {
		return b.shown
	}
	lib := a.PaletteLibrary()
	b.shown = b.shown[:0]
	for i := range lib {
		if lib[i].Matches(b.query) {
			b.shown = append(b.shown, i)
		}
	}
	if b.shown == nil {
		b.shown = []int{}
	}
	b.shownFor = b.query
	b.first = 0
	return b.shown
}

// revealAppliedPalette scrolls the applied palette into view, so opening the
// list starts where you left off rather than back at the letter A.
func (a *App) revealAppliedPalette() {
	b := &a.paint.browser
	if b.applied == "" {
		return
	}
	lib := a.PaletteLibrary()
	for row, idx := range a.paletteShown() {
		if lib[idx].Name == b.applied {
			b.first = row - 3
			if b.first < 0 {
				b.first = 0
			}
			return
		}
	}
}

// ApplyPaletteNamed puts a library palette on the custom page by name, which
// is what a script and the list both do.
func (a *App) ApplyPaletteNamed(name string) bool {
	lib := a.PaletteLibrary()
	for i := range lib {
		if strings.EqualFold(lib[i].Name, name) {
			a.applyPalette(lib[i])
			return true
		}
	}
	a.Toast(ui.Toast{Text: "No palette called " + name, Kind: ui.ToastWarn})
	return false
}

// applyPalette swaps the custom page to a library palette. It never touches
// the built-in page, for the reason the file import does not: whatever you
// try, the colours you started with stay one click away.
func (a *App) applyPalette(p paint.Palette) {
	a.paint.custom = append([]color.RGBA(nil), p.Colors...)
	a.paint.page = 1
	a.paint.browser.applied = p.Name
	a.Settings.PaletteName = p.Name
	a.Toast(ui.Toast{
		Text: fmt.Sprintf("%s — %s", p.Name, plural(len(p.Colors), "colour", "colours")),
	})
}

// buildPaletteBrowser lays out and runs the list.
func (a *App) buildPaletteBrowser(viewport rl.Rectangle) {
	b := &a.paint.browser
	lib := a.PaletteLibrary()
	shown := a.paletteShown()

	pad := a.px(ui.Spacing + 2)
	rowH := a.px(paletteRowHeight)
	head := a.UI.Fonts.LineHeight(ui.FontSizeHeader) + a.px(4)
	fieldH := a.px(28)
	footH := a.px(30)

	// As tall as the viewport allows, in whole rows.
	chrome := pad*2 + head + a.px(10) + fieldH + a.px(10) + footH + a.px(10)
	avail := viewport.Height - a.px(ui.Spacing*4) - chrome
	rows := int(avail / rowH)
	if rows < paletteMinRows {
		rows = paletteMinRows
	}
	if rows > len(shown) {
		rows = len(shown)
	}
	if rows < 1 {
		rows = 1
	}
	listH := float32(rows) * rowH

	w := a.px(paletteCardWidth)
	if w > viewport.Width-a.px(ui.Spacing*4) {
		w = viewport.Width - a.px(ui.Spacing*4)
	}
	h := chrome + listH
	box := ui.Rect(
		viewport.X+(viewport.Width-w)/2,
		viewport.Y+(viewport.Height-h)/2,
		w, h)

	card := a.UI.FloatingCard(ui.MakeID("palette.card"), box, "Palettes", ui.FloatingCardOpts{})
	body := card.Body

	// ---- search -----------------------------------------------------------
	var searchBox rl.Rectangle
	searchBox, body = ui.SplitTop(body, fieldH)
	countW := a.UI.TextWidth("0000 of 0000", ui.FontSizeSmall) + a.px(ui.Spacing)
	field, countBox := ui.SplitLeft(searchBox, searchBox.Width-countW)
	res := a.UI.TextField(ui.MakeID("palette.search"), field, b.query, ui.TextFieldOpts{
		Placeholder: "Search palettes…",
		Tooltip:     "Every word has to appear in the name, in any order",
	})
	if res.Text != b.query {
		b.query = res.Text
	}
	countBox.X += a.px(ui.Spacing)
	a.UI.Text(countBox, fmt.Sprintf("%d of %d", len(shown), len(lib)),
		ui.FontSizeSmall, ui.ColorTextDim)
	body.Y += a.px(10)
	body.Height -= a.px(10)

	// ---- the list ---------------------------------------------------------
	var list rl.Rectangle
	list, body = ui.SplitTop(body, listH)

	maxFirst := len(shown) - rows
	if maxFirst < 0 {
		maxFirst = 0
	}
	if a.UI.In.Wheel != 0 && rl.CheckCollisionPointRec(a.UI.MousePos(), list) {
		b.first -= int(a.UI.In.Wheel) * paletteWheelRows
	}
	if b.first > maxFirst {
		b.first = maxFirst
	}
	if b.first < 0 {
		b.first = 0
	}

	barW := a.px(4)
	rowsBox, scrollBox := ui.SplitLeft(list, list.Width-barW-a.px(4))
	a.UI.FillRounded(list, 4, ui.WithAlpha(ui.ColorBG, 0x66))

	if len(shown) == 0 {
		a.UI.Text(ui.InsetXY(rowsBox, a.px(ui.Spacing), a.px(6)),
			"Nothing matches that", ui.FontSizeUI, ui.ColorTextDim)
	}
	for i := 0; i < rows && b.first+i < len(shown); i++ {
		p := lib[shown[b.first+i]]
		row := ui.Rect(rowsBox.X, rowsBox.Y+float32(i)*rowH, rowsBox.Width, rowH)
		if a.paletteRow(row, p, p.Name == b.applied) {
			a.applyPalette(p)
		}
	}

	// The scrollbar is a readout, not a control: with the wheel and a search
	// box there is nothing left for a draggable thumb to do that is not
	// already easier another way.
	if len(shown) > rows {
		a.UI.FillRounded(scrollBox, 2, ui.WithAlpha(ui.ColorStroke, 0x80))
		frac := float32(rows) / float32(len(shown))
		thumbH := listH * frac
		if min := a.px(18); thumbH < min {
			thumbH = min
		}
		t := float32(b.first) / float32(maxFirst)
		thumb := ui.Rect(scrollBox.X, scrollBox.Y+t*(listH-thumbH), scrollBox.Width, thumbH)
		a.UI.FillRounded(thumb, 2, ui.ColorAccent)
	}
	body.Y += a.px(10)
	body.Height -= a.px(10)

	// ---- footer -----------------------------------------------------------
	foot, _ := ui.SplitTop(body, footH)
	importW := a.UI.TextWidth("Import .hex…", ui.FontSizeUI) + a.px(ui.Spacing*3)
	importBox, rest := ui.SplitLeft(foot, importW)
	if a.UI.Button(ui.MakeID("palette.import"), importBox, "Import .hex…", ui.ButtonOpts{
		Tooltip: "Load a palette file from disk onto the custom page",
	}) {
		a.RequestFile(fileImportPalette)
	}
	hintBox, closeBox := ui.SplitLeft(rest, rest.Width-a.px(88))
	hintBox.X += a.px(ui.Spacing)
	a.UI.Text(hintBox, "or drop files in your palettes folder",
		ui.FontSizeSmall, ui.Fade(ui.ColorTextDim, 0.8))
	if a.UI.Button(ui.MakeID("palette.close"), closeBox, "Close", ui.ButtonOpts{
		Style: ui.ButtonPrimary, Shortcut: "Esc",
	}) {
		a.ClosePaletteBrowser()
	}
}

// paletteRow draws one row: the name, then its colours. It reports a click.
func (a *App) paletteRow(r rl.Rectangle, p paint.Palette, applied bool) bool {
	return a.UI.SwatchRow(ui.MakeID("palette.row."+p.Name), r, ui.SwatchRowSpec{
		Label:    p.Name,
		Swatches: p.Colors,
		Columns:  paletteStripCount,
		Selected: applied,
		Tooltip: fmt.Sprintf("%s · %s", p.Name,
			plural(len(p.Colors), "colour", "colours")),
	})
}
