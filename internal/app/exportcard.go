package app

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/ui"
)

// The export card of SPEC-DATA §5: what to write, and how, before the file
// dialog asks where.
//
// The options belong here rather than in the dialog because a native file
// dialog has nowhere sensible to put them, and because the choice changes what
// the dialog should offer: picking PNG has to change the filter it opens with.
// So the card decides the format, then the dialog decides the path.

const exportCardWidth = 320

// InExport reports whether the export options are open.
func (a *App) InExport() bool { return a.files.exportOpen }

// BeginExport opens the card.
func (a *App) BeginExport() {
	if a.Bus.Review() != nil {
		a.openWorkflow(a.workflow.face, 0)
		return
	}
	a.workflow.issues = model.MaterialHealth(a.Doc())
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

func (a *App) modelExportScale() float64 {
	if a.files.exportModelScale == 0 {
		return 1
	}
	return a.files.exportModelScale
}

func (a *App) SetModelExportScale(scale float64) error {
	if err := io.ValidateModelExportScale(scale); err != nil {
		return err
	}
	a.files.exportModelScale = scale
	return nil
}

// buildExportCard runs the options panel.
func (a *App) buildExportCard(viewport rl.Rectangle) {
	formats := io.ExportFormats()
	if a.files.exportFormat < 0 || a.files.exportFormat >= len(formats) {
		a.files.exportFormat = 0
	}
	f := formats[a.files.exportFormat]
	isPNG := f.Extension == ".png"
	isProject := io.IsShipFile(f.Extension)

	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	h := a.px(72) + line + a.px(24+8) + line*3 + a.px(8+28)
	if isPNG {
		h += line + a.px(24+6) + a.px(24+6)
	} else if !isProject {
		h += line*3 + a.px(34)
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

	if a.UI.Button(ui.MakeID("export.health"), row(a.px(30)), fmt.Sprintf("Material health · %d issues", len(a.workflow.issues)), ui.ButtonOpts{}) {
		a.openWorkflow(a.workflow.face, 2)
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
	} else if !isProject {
		a.UI.Text(row(line), "Ship scale", ui.FontSizeSmall, ui.ColorTextDim)
		field, reset := ui.SplitLeft(row(a.px(28)), a.px(236))
		scale, result := a.UI.DragNumber(ui.MakeID("export.model_scale"), field, a.modelExportScale(), ui.NumberOpts{
			Unit: "×", Step: .1, FineStep: .01, Min: io.MinModelExportScale, Max: io.MaxModelExportScale, Decimals: 6,
			Tooltip: "Click to type. 0.5 = half size; 2 = double size. Scales around the model origin.",
		})
		if result.Changed {
			_ = a.SetModelExportScale(scale)
		}
		if a.UI.Button(ui.MakeID("export.model_scale.reset"), reset, "Reset", ui.ButtonOpts{}) {
			_ = a.SetModelExportScale(1)
		}
		space(6)
		bounds := geom.Empty()
		for _, b := range a.Doc().Bodies {
			if b.Visible && b.Mesh != nil {
				bounds = bounds.Union(b.Mesh.AABB())
			}
		}
		size := bounds.Size().Mul(a.modelExportScale())
		a.UI.Text(row(line), fmt.Sprintf("Size: X %.4g · Y %.4g · Z %.4g u", size.X, size.Y, size.Z), ui.FontSizeSmall, ui.ColorTextDim)
		a.UI.Text(row(line), "Export only · project stays the same", ui.FontSizeSmall, ui.ColorTextDim)
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
