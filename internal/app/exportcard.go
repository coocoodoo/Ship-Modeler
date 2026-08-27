package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/io"
	"modeler/internal/ui"
)

// The export card of SPEC-DATA §5: what to write, and how, before the file
// dialog asks where.
//
// The options belong here rather than in the dialog because a native file
// dialog has nowhere sensible to put them, and because the choice changes what
// the dialog should offer: picking PNG has to change the filter it opens with.
// So the card decides the format, then the dialog decides the path.

const exportCardWidth = 260

// InExport reports whether the export options are open.
func (a *App) InExport() bool { return a.files.exportOpen }

// BeginExport opens the card.
func (a *App) BeginExport() {
	if len(a.Doc().Bodies) == 0 {
		a.Toast(ui.Toast{
			Text: "There is nothing to export yet — sketch and extrude a body first",
			Kind: ui.ToastWarn,
		})
		return
	}
	a.files.exportOpen = true
}

// CancelExport closes it without writing anything.
func (a *App) CancelExport() { a.files.exportOpen = false }

// buildExportCard runs the options panel.
func (a *App) buildExportCard(viewport rl.Rectangle) {
	formats := io.ExportFormats()
	if a.files.exportFormat < 0 || a.files.exportFormat >= len(formats) {
		a.files.exportFormat = 0
	}
	f := formats[a.files.exportFormat]
	isPNG := f.Extension == ".png"

	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	h := a.px(38) + line + a.px(24+8) + line*3 + a.px(8+28)
	if isPNG {
		h += line + a.px(24+6) + a.px(24+6)
	}
	box := ui.Rect(
		viewport.X+viewport.Width-a.px(exportCardWidth)-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		a.px(exportCardWidth), h)

	card := a.UI.FloatingCard(ui.MakeID("export.card"), box, "Export", ui.FloatingCardOpts{
		Footer:       true,
		ConfirmLabel: "Choose file…",
		CancelLabel:  "Cancel",
	})
	body := card.Body
	row := func(height float32) rl.Rectangle {
		var r rl.Rectangle
		r, body = ui.SplitTop(body, height)
		return r
	}
	space := func(v float64) {
		body.Y += a.px(v)
		body.Height -= a.px(v)
	}

	a.UI.Text(row(line), "Format", ui.FontSizeSmall, ui.ColorTextDim)
	labels := make([]string, len(formats))
	for i, fm := range formats {
		labels[i] = fm.Extension[1:]
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("export.format"), row(a.px(24)),
		labels, a.files.exportFormat, ui.ChipGroupOpts{}); changed {
		a.files.exportFormat = pick
	}
	space(8)

	if isPNG {
		a.UI.Text(row(line), "Scale", ui.FontSizeSmall, ui.ColorTextDim)
		scaleLabels := make([]string, len(PNGExportScales))
		sel := 0
		for i, s := range PNGExportScales {
			scaleLabels[i] = itoa(s) + "×"
			if s == a.files.exportScale {
				sel = i
			}
		}
		if pick, changed := a.UI.ChipGroup(ui.MakeID("export.scale"), row(a.px(24)),
			scaleLabels, sel, ui.ChipGroupOpts{
				Tooltip: "Whole numbers only, so the pixels stay square",
			}); changed {
			a.files.exportScale = PNGExportScales[pick]
		}
		space(6)
		if a.UI.Toggle(ui.MakeID("export.alpha"), row(a.px(24)), "Transparent background",
			a.files.exportAlpha, ui.ButtonOpts{
				Tooltip: "Leave the backdrop out, so the ship can be dropped onto something else",
			}) {
			a.files.exportAlpha = !a.files.exportAlpha
		}
		space(6)
	}

	// The caveat, in the card rather than in a README nobody opens.
	a.UI.TextWrapped(row(line*3), f.Note, ui.FontSizeSmall, ui.ColorTextDim)

	if card.Confirmed {
		a.files.exportOpen = false
		a.RequestFile(fileExport)
	}
	if card.Cancelled {
		a.CancelExport()
	}
}
