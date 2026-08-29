package app

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/ui"
)

// Importing a mesh file (the user's request, 2026-08-28).
//
// The read and the assembly are done elsewhere; what lives here is the one
// decision the file cannot answer. STL and OBJ carry no units — a part
// exported from CAD is usually in millimetres, and a ship here is a couple of
// dozen units long — so bringing one in at face value is as likely to produce
// something the size of a continent as something you can see. The card asks,
// shows the answer in units before anything is committed, and defaults to the
// scale that makes the thing fit on screen.

// ImportFitSize is what "Fit" scales the longest side to, in units. It is the
// rough size of a hull section in this program, which is what someone
// importing a part is usually about to work against.
const ImportFitSize = 16.0

// importScales are the chips beside Fit.
var importScales = []float64{0.01, 0.1, 0.5, 1, 2, 10}

// InImportMesh reports whether the import card is up.
func (a *App) InImportMesh() bool { return a.files.importOpen }

// importMeshWithDialog picks a file, reads it, and opens the card.
func (a *App) importMeshWithDialog() {
	path, ok, err := io.AskImportMesh(a.Settings.LastDir)
	if err != nil {
		a.Toast(ui.Toast{Text: "The file dialog could not open", Kind: ui.ToastError})
		return
	}
	if !ok {
		return
	}
	a.LoadMeshFile(path)
}

// LoadMeshFile reads a mesh file and opens the import card on it. Split from
// the dialog so a script can drive it.
func (a *App) LoadMeshFile(path string) bool {
	tris, err := io.ReadMeshFile(path)
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	a.Settings.LastDir = filepath.Dir(path)

	lo := geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi := geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, t := range tris {
		for _, p := range [3]geom.Vec3{t.A, t.B, t.C} {
			lo = geom.Vec3{X: math.Min(lo.X, p.X), Y: math.Min(lo.Y, p.Y), Z: math.Min(lo.Z, p.Z)}
			hi = geom.Vec3{X: math.Max(hi.X, p.X), Y: math.Max(hi.Y, p.Y), Z: math.Max(hi.Z, p.Z)}
		}
	}
	a.files.importTris = tris
	a.files.importName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	a.files.importLo, a.files.importHi = lo, hi
	a.files.importScale = fitScale(lo, hi)
	a.files.importCenter = true
	a.files.importOpen = true
	return true
}

// fitScale is what brings the longest side to ImportFitSize, rounded to
// something a person would have chosen.
func fitScale(lo, hi geom.Vec3) float64 {
	longest := math.Max(hi.X-lo.X, math.Max(hi.Y-lo.Y, hi.Z-lo.Z))
	if longest <= 0 {
		return 1
	}
	s := ImportFitSize / longest
	// Round to one significant figure so the number reads as a decision rather
	// than as a measurement: 0.0642 becomes 0.06.
	mag := math.Pow(10, math.Floor(math.Log10(s)))
	return math.Max(math.Round(s/mag)*mag, 1e-6)
}

// CancelImport drops the read triangles and closes the card.
func (a *App) CancelImport() {
	a.files.importOpen = false
	a.files.importTris = nil
}

// CommitImport assembles the held triangles and adds the body.
func (a *App) CommitImport() bool {
	tris := a.files.importTris
	if len(tris) == 0 {
		return false
	}
	m, st, err := mesh.Assemble(tris, mesh.AssembleOpts{
		Scale:  a.files.importScale,
		Center: a.files.importCenter,
	})
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	name := a.files.importName
	a.CancelImport()
	if !a.Run(&model.AddBody{Mesh: m, Label: name}) {
		return false
	}

	// What it did, in the terms the user cares about: how much of the soup
	// became real faces, and whether the result is a solid the booleans can
	// take. An import that quietly produced an open shell would fail later,
	// somewhere that could not explain why.
	text := fmt.Sprintf("Imported %s: %s from %s",
		name, plural(st.Faces, "face", "faces"), plural(st.InputTris, "triangle", "triangles"))
	kind := ui.ToastInfo
	if st.OpenEdges > 0 || st.NonManifold > 0 {
		text += " — not a closed solid, so booleans will refuse it"
		kind = ui.ToastWarn
	}
	a.Toast(ui.Toast{Text: text, Kind: kind, Action: "Undo", OnAction: func() { a.Undo() }})
	return true
}

// importCardWidth matches the export card, because they are the same kind of
// thing and sitting in the same place.
const importCardWidth = exportCardWidth

// buildImportCard asks the one question the file cannot answer.
func (a *App) buildImportCard(viewport rl.Rectangle) {
	line := a.UI.Fonts.LineHeight(ui.FontSizeUI) + a.px(2)
	h := a.px(38) + line*2 + a.px(24+8) + a.px(24+8) + line + a.px(8+28)
	box := ui.Rect(
		viewport.X+viewport.Width-a.px(importCardWidth)-a.px(ui.Spacing*2),
		viewport.Y+a.px(ui.ViewCubeSize+ui.ViewCubeMargin*2+34),
		a.px(importCardWidth), h)

	card := a.UI.FloatingCard(ui.MakeID("import.card"), box, "Import mesh", ui.FloatingCardOpts{
		Footer:       true,
		ConfirmLabel: "Import",
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

	a.UI.Text(row(line), a.files.importName+" · "+
		plural(len(a.files.importTris), "triangle", "triangles"),
		ui.FontSizeSmall, ui.ColorTextDim)
	space(4)

	// Scale, with Fit first because it is the answer nearly every time.
	a.UI.Text(row(line), "Scale", ui.FontSizeSmall, ui.ColorTextDim)
	fit := fitScale(a.files.importLo, a.files.importHi)
	labels := []string{"Fit"}
	values := []float64{fit}
	for _, v := range importScales {
		labels = append(labels, trimZeros(v))
		values = append(values, v)
	}
	sel := -1
	for i, v := range values {
		if math.Abs(v-a.files.importScale) < 1e-12 {
			sel = i
			break
		}
	}
	if pick, changed := a.UI.ChipGroup(ui.MakeID("import.scale"), row(a.px(24)),
		labels, sel, ui.ChipGroupOpts{
			Tooltip: "Mesh files carry no units — this is what one of theirs is worth here",
		}); changed {
		a.files.importScale = values[pick]
	}
	space(8)

	if a.UI.Toggle(ui.MakeID("import.center"), row(a.px(24)), "Centre on the origin",
		a.files.importCenter, ui.ButtonOpts{
			Tooltip: "Bring it in around the origin rather than wherever the file put it",
		}) {
		a.files.importCenter = !a.files.importCenter
	}
	space(8)

	// The size it will actually be, before it is committed. This is the whole
	// reason the card exists.
	d := a.files.importHi.Sub(a.files.importLo).Mul(a.files.importScale)
	a.UI.Text(row(line), fmt.Sprintf("Result: %.3g × %.3g × %.3g u", d.X, d.Y, d.Z),
		ui.FontSizeSmall, ui.ColorText)

	if card.Confirmed {
		a.CommitImport()
	}
	if card.Cancelled {
		a.CancelImport()
	}
}

// trimZeros writes a scale the way someone would say it.
func trimZeros(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	return s
}
