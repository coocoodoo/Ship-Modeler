package app

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"math"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/ui"
	"strings"
)

// Workshop chrome stays fixed; only the tab's contents scroll. All measurements
// are logical UI units so the panel also fits large UI scales and small windows.
func (a *App) buildWorkflow() {
	st := &a.workflow
	if !st.open || a.UI.ModalOpen() {
		return
	}
	height := 650.0
	if st.tab == 2 {
		height = 690
	}
	if st.tab == 0 && a.Bus.Review() == nil {
		height = 470
	}
	if st.tab == 1 {
		height = 740
	}
	rl.DrawRectangleRec(a.layout.Screen, ui.Fade(ui.ColorPanel, .42))
	card := a.UI.FloatingCard(ui.MakeID("workflow"), a.libraryDialogBox(760, height), "Model Workshop", ui.FloatingCardOpts{})
	body := ui.InsetXY(card.Body, a.px(6), 0)
	subtitle, body := ui.SplitTop(body, a.px(27))
	a.UI.Text(subtitle, "Review requests, refine materials, and inspect your model.", ui.FontSizeSmall, ui.ColorTextDim)
	tabs, body := ui.SplitTop(body, a.px(39))
	labels := []string{"AI Review", "Layers & Materials", "Material Health"}
	if body.Width < a.px(560) {
		labels = []string{"Review", "Layers", "Health"}
	}
	icons := []ui.IconFunc{ui.DrawCheckIcon, ui.DrawTileIcon, ui.DrawSearchIcon}
	for i, label := range labels {
		r := a.workColumn(tabs, i, 3, 8)
		style := ui.ButtonGhost
		if st.tab == i {
			style = ui.ButtonPrimary
		}
		if a.workButton(fmt.Sprintf("tab.%d", i), r, label, style, icons[i], false, "") {
			a.finishWorkshopOpacity()
			st.tab = i
			st.scroll = 0
			st.contentHeight = 0
			a.UI.ClearFocus()
			if i == 2 {
				st.issues = model.MaterialHealth(a.Doc())
				st.issuePage = 0
			}
		}
	}
	body.Y += a.px(16)
	body.Height -= a.px(16)
	footer, viewport := ui.SplitBottom(body, a.px(49))
	viewport.Height -= a.px(10)
	a.UI.HairlineH(footer.X, footer.Y, footer.Width, ui.ColorStroke)
	footer.Y += a.px(12)
	footer.Height = a.px(34)
	close, foot := ui.SplitRight(footer, a.px(102))
	inspect, message := ui.SplitLeft(foot, a.px(210))
	message = ui.InsetXY(message, a.px(12), 0)
	if ts := a.UI.Toasts(); len(ts) > 0 {
		t := ts[len(ts)-1]
		if t.Kind == ui.ToastWarn || t.Kind == ui.ToastError {
			a.UI.TextWrapped(message, t.Text, ui.FontSizeSmall, ui.ColorWarn)
		}
	}
	if a.workButton("inspect", inspect, "Capture inspection views", ui.ButtonGhost, ui.DrawExportIcon, false, "Save six standard views and close-ups of edited faces") {
		a.finishWorkshopOpacity()
		st.inspect = true
		st.open = false
	}
	if a.workButton("close", close, "Close", ui.ButtonNormal, ui.DrawCrossIcon, false, "Escape") || a.UI.In.KeyPressed(rl.KeyEscape) {
		a.finishWorkshopOpacity()
		st.open = false
		a.UI.ClearFocus()
	}
	maxScroll := max(float32(0), st.contentHeight-viewport.Height)
	st.scroll = max(0, min(st.scroll-float32(a.UI.ScrollWheel(viewport))*a.px(38), maxScroll))
	content := viewport
	content.Width -= a.px(9)
	content.Y -= st.scroll
	start := content.Y
	a.UI.Clip(viewport, func() {
		switch st.tab {
		case 0:
			a.workReview(&content)
		case 1:
			a.workLayers(&content)
		case 2:
			a.workHealth(&content)
		}
	})
	st.contentHeight = content.Y - start
	if st.contentHeight > viewport.Height {
		track := ui.Rect(viewport.X+viewport.Width-a.px(3), viewport.Y, a.px(3), viewport.Height)
		a.UI.FillRounded(track, 2, ui.ColorStroke)
		thumb := track
		thumb.Height = max(a.px(24), track.Height*viewport.Height/st.contentHeight)
		thumb.Y += (track.Height - thumb.Height) * st.scroll / max(a.px(1), st.contentHeight-viewport.Height)
		a.UI.FillRounded(thumb, 2, ui.ColorTextDim)
	}
	if st.opacityDragging && !a.UI.In.Down[ui.MouseLeft] {
		a.finishWorkshopOpacity()
	}
	a.UI.ClaimPointer(a.layout.Screen)
}
func (a *App) finishWorkshopOpacity() {
	if a.workflow.opacityDragging {
		a.Bus.CommitDrag()
		a.workflow.opacityDragging = false
	}
}
func (a *App) workRow(body *rl.Rectangle, h float64) rl.Rectangle {
	r := ui.Rect(body.X, body.Y, body.Width, a.px(h))
	body.Y += r.Height
	return r
}
func (a *App) workGap(body *rl.Rectangle, h float64) { body.Y += a.px(h) }
func (a *App) workColumn(r rl.Rectangle, i, n int, gap float64) rl.Rectangle {
	w := (r.Width - a.px(gap)*float32(n-1)) / float32(n)
	return ui.Rect(r.X+float32(i)*(w+a.px(gap)), r.Y, w, r.Height)
}
func (a *App) workButton(id string, r rl.Rectangle, label string, style ui.ButtonStyle, icon ui.IconFunc, disabled bool, why string) bool {
	return a.UI.Button(ui.MakeID("work."+id), r, label, ui.ButtonOpts{Style: style, Icon: icon, Disabled: disabled, Tooltip: why, DisabledWhy: why})
}
func (a *App) workPanel(r rl.Rectangle) rl.Rectangle {
	a.UI.FillRounded(r, 8, ui.ColorPanel)
	a.UI.StrokeRounded(r, 8, ui.ColorStroke)
	return ui.Inset(r, a.px(14))
}
func (a *App) workHeading(body *rl.Rectangle, title, detail string) {
	a.UI.Text(a.workRow(body, 25), title, ui.FontSizeHeader, ui.ColorText)
	if detail != "" {
		a.UI.TextWrapped(a.workRow(body, 37), detail, ui.FontSizeSmall, ui.ColorTextDim)
	}
	a.workGap(body, 8)
}
func (a *App) workBadge(r rl.Rectangle, label string, c color.RGBA) {
	a.UI.FillRounded(r, 5, ui.Fade(c, .13))
	a.UI.TextCentered(r, label, ui.FontSizeSmall, c)
}

func (a *App) workReview(body *rl.Rectangle) {
	st := &a.workflow
	r := a.Bus.Review()
	if r == nil {
		hero := a.workPanel(a.workRow(body, 124))
		icon, text := ui.SplitLeft(hero, a.px(64))
		drawNotePinIcon(float64(icon.X+icon.Width/2), float64(icon.Y)+35*a.Scale, 28*a.Scale, ui.ColorAccent)
		a.UI.Text(a.workRow(&text, 31), "Ready for your next request", ui.FontSizeHeader, ui.ColorText)
		a.UI.TextWrapped(a.workRow(&text, 54), "Place a note on the face you want changed. Start work from that pin, then review the result here before accepting it.", ui.FontSizeUI, ui.ColorTextDim)
		a.workGap(body, 12)
		steps := a.workRow(body, 67)
		for i, label := range []string{"Place a pin", "Edit its face", "Review & accept"} {
			c := a.workPanel(a.workColumn(steps, i, 3, 10))
			num, txt := ui.SplitLeft(c, a.px(30))
			a.UI.TextCentered(num, fmt.Sprint(i+1), ui.FontSizeHeader, ui.ColorAccent)
			a.UI.TextWrapped(txt, label, ui.FontSizeSmall, ui.ColorText)
		}
		a.workGap(body, 16)
		actions := a.workRow(body, 36)
		if a.workButton("pins", a.workColumn(actions, 0, 2, 10), "Open model pins", ui.ButtonPrimary, drawNotePinIcon, false, "") {
			st.open = false
			a.notePins.open = true
			a.notePins.editing = false
		}
		if a.workButton("undoRequest", a.workColumn(actions, 1, 2, 10), "Undo last request", ui.ButtonGhost, ui.DrawUndoIcon, !isRequestUndo(a.Bus.UndoName()), "Available when an accepted request is the latest undo step") {
			a.Undo()
		}
		return
	}
	header := a.workRow(body, 32)
	badge, left := ui.SplitRight(header, a.px(126))
	a.UI.Text(left, fmt.Sprintf("Pin %d  /  Review changes", r.Pin), ui.FontSizeHeader, ui.ColorText)
	a.workBadge(badge, r.Status, ui.ColorAccent)
	a.UI.Text(a.workRow(body, 25), fmt.Sprintf("%d face(s) in scope  ·  %d changed  ·  Other faces are protected", len(r.Scope), len(r.Changed)), ui.FontSizeSmall, ui.ColorTextDim)
	a.workGap(body, 10)
	previews := a.workRow(body, 185)
	for i, label := range []string{"Original", "Proposed"} {
		pane := a.workPanel(a.workColumn(previews, i, 2, 12))
		data := r.BeforeImage
		if i == 1 {
			data = r.AfterImage
		}
		a.reviewImage(i, data, pane, label)
	}
	a.workGap(body, 14)
	a.UI.Text(a.workRow(body, 23), "Change summary", ui.FontSizeUI, ui.ColorText)
	if r.Status == "Needs review" {
		p := a.workPanel(a.workRow(body, 68))
		a.UI.TextWrapped(p, r.Description, ui.FontSizeUI, ui.ColorText)
	} else {
		field := a.UI.TextArea(ui.MakeID("work.description"), a.workRow(body, 68), st.description, "Describe what changed on this face…")
		st.description = field.Text
	}
	a.workGap(body, 12)
	tools := a.workRow(body, 34)
	label := "View original model"
	if st.beforeView {
		label = "View proposed model"
	}
	if a.workButton("before", a.workColumn(tools, 0, 2, 10), label, ui.ButtonNormal, ui.DrawSwapIcon, false, "Compare the original and proposed model in the viewport") {
		a.toggleReviewBefore(!st.beforeView)
	}
	if a.workButton("expand", a.workColumn(tools, 1, 2, 10), "Expand to selected faces", ui.ButtonGhost, ui.DrawPixelSelectIcon, r.Status != "In progress", "Select additional faces in the viewport before expanding scope") {
		count := 0
		for _, ref := range a.Sel.Refs() {
			if ref.Kind == model.SelFace {
				a.workflowError(a.Bus.ExpandReview(model.FaceScope{Body: ref.Body, Face: ref.Face}))
				count++
			}
		}
		if count == 0 {
			a.Toast(ui.Toast{Text: "Select the additional faces, then reopen Review to expand scope"})
		}
	}
	a.workGap(body, 18)
	actions := a.workRow(body, 38)
	if a.workButton("reject", a.workColumn(actions, 0, 2, 10), "Reject request", ui.ButtonDanger, ui.DrawCrossIcon, false, "Restore the original model and discard this request") {
		a.toggleReviewBefore(false)
		a.Bus.RejectReview()
		st.open = false
	}
	if r.Status == "Needs review" {
		if a.workButton("accept", a.workColumn(actions, 1, 2, 10), "Accept changes", ui.ButtonPrimary, ui.DrawCheckIcon, false, "Commit the complete request as one undo step") {
			a.toggleReviewBefore(false)
			if e := a.Bus.AcceptReview(); e != nil {
				a.workflowError(e)
			} else {
				st.open = false
			}
		}
	} else {
		if a.workButton("propose", a.workColumn(actions, 1, 2, 10), "Submit for review", ui.ButtonPrimary, ui.DrawCheckIcon, strings.TrimSpace(st.description) == "", "Add a summary before submitting") {
			a.workflowError(a.Bus.ProposeReview(st.description))
		}
	}
}

func (a *App) workLayers(body *rl.Rectangle) {
	st := &a.workflow
	f, ok := a.resolveFace(st.face.Body, st.face.Face)
	if !ok {
		a.workHeading(body, "Choose a face to begin", "Right-click a model face and choose Layers / Match. Its texture and editable layers will appear here.")
		return
	}
	header := a.workRow(body, 30)
	badge, left := ui.SplitRight(header, a.px(86))
	a.UI.Text(left, a.UI.Truncate(f.body.Name, ui.FontSizeHeader, left.Width-a.px(8)), ui.FontSizeHeader, ui.ColorText)
	a.workBadge(badge, fmt.Sprintf("Face %d", f.face), ui.ColorAccent)
	a.workGap(body, 10)
	p := f.body.Mesh.Faces[f.face].Paint
	compact := body.Width < a.px(560)
	refHeight := 145.0
	if compact {
		refHeight = 225
	}
	refBox := a.workPanel(a.workRow(body, refHeight))
	a.UI.Text(a.workRow(&refBox, 25), "Material reference", ui.FontSizeUI, ui.ColorText)
	details := "Choose this face as your reference, then match it on another face."
	if st.reference != nil {
		details = fmt.Sprintf("%d px/u  ·  %d palette colors  ·  Matched surface response", st.reference.Resolution, len(st.reference.Palette))
	}
	a.UI.TextWrapped(a.workRow(&refBox, 31), details, ui.FontSizeSmall, ui.ColorTextDim)
	swatches := a.workRow(&refBox, 18)
	if st.reference != nil {
		for i, c := range st.reference.Palette[:min(16, len(st.reference.Palette))] {
			box := ui.Rect(swatches.X+float32(i)*a.px(21), swatches.Y, a.px(17), a.px(14))
			a.UI.FillRounded(box, 3, c)
			a.UI.StrokeRounded(box, 3, ui.ColorStroke)
		}
	}
	a.workGap(&refBox, 6)
	r := a.workRow(&refBox, 32)
	refButton := func(i int) rl.Rectangle {
		if compact {
			if i > 0 {
				a.workGap(&refBox, 7)
				return a.workRow(&refBox, 32)
			}
			return r
		}
		return a.workColumn(r, i, 3, 8)
	}
	if a.workButton("reference", refButton(0), "Use as reference", ui.ButtonNormal, ui.DrawDropperIcon, p == nil, "Sample this face's palette and material response") {
		ref, e := paint.AnalyzeReference(a.Doc(), st.face.Body, st.face.Face)
		if e == nil {
			st.reference = &ref
		}
		a.workflowError(e)
	}
	if a.workButton("match", refButton(1), "Match reference", ui.ButtonNormal, ui.DrawSwapIcon, st.reference == nil || p == nil, "Choose a painted reference and a painted target face") {
		a.Run(&paint.MatchReference{Body: st.face.Body, Face: st.face.Face, Reference: *st.reference})
	}
	if a.workButton("pbr", refButton(2), "Refresh PBR", ui.ButtonNormal, ui.DrawRefreshIcon, p == nil, "Generate matching maps from this face's base color") {
		a.Run(&paint.RefreshPBR{Body: st.face.Body, Face: st.face.Face})
	}
	a.workGap(body, 18)
	a.UI.Text(a.workRow(body, 25), "Paint layers", ui.FontSizeHeader, ui.ColorText)
	r = a.workRow(body, 31)
	columns := 5
	if compact {
		columns = 3
	}
	for i, name := range []string{"Armor", "Markings", "Scratches", "Vents", "Decals"} {
		if i == columns {
			a.workGap(body, 5)
			r = a.workRow(body, 31)
		}
		if a.workButton("add."+name, a.workColumn(r, i%columns, columns, 6), "+ "+name, ui.ButtonGhost, nil, false, "Add a separate editable layer") {
			a.Run(&paint.EditLayer{Body: st.face.Body, Face: st.face.Face, Action: "add", Label: name})
		}
	}
	a.workGap(body, 9)
	if p == nil || len(p.Layers) == 0 {
		empty := a.workPanel(a.workRow(body, 84))
		a.UI.TextWrapped(empty, "Add your first layer above. Existing artwork is preserved in Base; new details can be changed independently.", ui.FontSizeUI, ui.ColorTextDim)
		return
	}
	edit := func(action string, index int, on bool, value float64) {
		a.Run(&paint.EditLayer{Body: st.face.Body, Face: st.face.Face, Action: action, Index: index, On: on, Value: value})
	}
	perPage := 4
	pages := (len(p.Layers) + perPage - 1) / perPage
	st.layerPage = max(0, min(st.layerPage, pages-1))
	for i := st.layerPage * perPage; i < min(len(p.Layers), (st.layerPage+1)*perPage); i++ {
		l := p.Layers[i]
		row := a.workRow(body, 47)
		row.Height -= a.px(5)
		fill := ui.ColorPanel
		if i == p.ActiveLayer {
			fill = ui.Fade(ui.ColorAccent, .15)
		}
		a.UI.FillRounded(row, 6, fill)
		if i == p.ActiveLayer {
			a.UI.FillRounded(ui.Rect(row.X, row.Y+a.px(7), a.px(3), row.Height-a.px(14)), 2, ui.ColorAccent)
		}
		inner := ui.InsetXY(row, a.px(8), a.px(3))
		eye, inner := ui.SplitLeft(inner, a.px(35))
		status, name := ui.SplitRight(inner, a.px(124))
		icon := func(x, y, s float64, c color.RGBA) { ui.DrawEyeIcon(x, y, s, l.Visible, c) }
		if a.workButton(fmt.Sprintf("eye.%d", i), eye, "", ui.ButtonGhost, icon, false, "Show or hide "+l.Name) {
			edit("visible", i, !l.Visible, 0)
		}
		if a.workButton(fmt.Sprintf("layer.%d", i), name, l.Name, ui.ButtonGhost, ui.DrawTileIcon, false, "Select this layer for painting") {
			edit("select", i, false, 0)
		}
		statusLabel := fmt.Sprintf("%d%%", int(l.Opacity*100))
		if l.Mask != nil {
			statusLabel += "  ·  Mask"
		}
		a.UI.TextCentered(status, statusLabel, ui.FontSizeSmall, ui.ColorTextDim)
	}
	if pages > 1 {
		a.workPager(body, "layers", &st.layerPage, pages)
	}
	a.workGap(body, 9)
	active := max(0, min(p.ActiveLayer, len(p.Layers)-1))
	controls := a.workPanel(a.workRow(body, 123))
	label := a.workRow(&controls, 24)
	value, title := ui.SplitRight(label, a.px(52))
	a.UI.Text(title, p.Layers[active].Name+" / Opacity", ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.TextCentered(value, fmt.Sprintf("%d%%", int(p.Layers[active].Opacity*100)), ui.FontSizeSmall, ui.ColorText)
	if v, changed := a.UI.Slider(ui.MakeID("work.opacity"), a.workRow(&controls, 25), p.Layers[active].Opacity, 0, 1, ui.ButtonOpts{Tooltip: "Layer opacity"}); changed {
		v = math.Round(v*100) / 100
		c := &paint.EditLayer{Body: st.face.Body, Face: st.face.Face, Action: "opacity", Index: active, Value: v}
		var e error
		if st.opacityDragging {
			e = a.Bus.UpdateDrag(c)
		} else {
			e = a.Bus.BeginDrag(c)
			st.opacityDragging = e == nil
		}
		a.workflowError(e)
	}
	a.workGap(&controls, 8)
	r = a.workRow(&controls, 32)
	for i, action := range []string{"down", "up", "delete"} {
		labels := []string{"Move down", "Move up", "Delete layer"}
		disabled := action == "down" && active == 0 || action == "up" && active == len(p.Layers)-1 || action == "delete" && len(p.Layers) == 1
		style := ui.ButtonGhost
		if action == "delete" {
			style = ui.ButtonGhost
		}
		if a.workButton(action, a.workColumn(r, i, 3, 8), labels[i], style, nil, disabled, "Reorder or remove the active layer") {
			edit(action, active, false, 0)
		}
	}
	a.workGap(body, 12)
	r = a.workRow(body, 36)
	if a.workButton("mask", a.workColumn(r, 0, 2, 10), map[bool]string{false: "Paint layer mask", true: "Return to layer pixels"}[p.PaintMask], ui.ButtonNormal, ui.DrawPixelSelectIcon, false, "White reveals the layer; black hides it") {
		edit("mask", active, !p.PaintMask, 0)
		a.BeginPaint()
		a.paint.res = 32
		st.open = false
	}
	if a.workButton("paintLayer", a.workColumn(r, 1, 2, 10), "Paint selected layer", ui.ButtonPrimary, ui.DrawPaintIcon, false, "") {
		if p.PaintMask {
			edit("mask", active, false, 0)
		}
		a.BeginPaint()
		a.paint.res = 32
		st.open = false
	}
}

func (a *App) workPager(body *rl.Rectangle, id string, page *int, pages int) {
	r := a.workRow(body, 31)
	prev, rest := ui.SplitLeft(r, a.px(90))
	next, mid := ui.SplitRight(rest, a.px(90))
	if a.workButton(id+".prev", prev, "Previous", ui.ButtonGhost, nil, *page == 0, "") {
		*page--
	}
	a.UI.TextCentered(mid, fmt.Sprintf("%d / %d", *page+1, pages), ui.FontSizeSmall, ui.ColorTextDim)
	if a.workButton(id+".next", next, "Next", ui.ButtonGhost, nil, *page+1 == pages, "") {
		*page++
	}
}
func (a *App) workHealth(body *rl.Rectangle) {
	st := &a.workflow
	header := a.workRow(body, 34)
	scan, text := ui.SplitRight(header, a.px(144))
	a.UI.Text(text, "Material readiness", ui.FontSizeHeader, ui.ColorText)
	if a.workButton("check", scan, "Run check", ui.ButtonNormal, ui.DrawRefreshIcon, false, "Scan all faces for material issues") {
		st.issues = model.MaterialHealth(a.Doc())
		st.issuePage = 0
	}
	a.workGap(body, 12)
	faces := map[model.FaceScope]bool{}
	missing := 0
	for _, v := range st.issues {
		faces[model.FaceScope{Body: v.Body, Face: v.Face}] = true
		if v.Kind == "missing_pbr" {
			missing++
		}
	}
	summary := a.workRow(body, 79)
	for i, metric := range []struct{ value, label string }{{fmt.Sprint(len(st.issues)), "Issues found"}, {fmt.Sprint(len(faces)), "Affected faces"}, {fmt.Sprint(missing), "Missing PBR maps"}} {
		r := a.workPanel(a.workColumn(summary, i, 3, 10))
		col := ui.ColorText
		if i == 0 {
			col = ui.ColorWarn
			if len(st.issues) == 0 {
				col = ui.ColorSuccess
			}
		}
		a.UI.Text(a.workRow(&r, 26), metric.value, ui.FontSizeHeader, col)
		a.UI.TextWrapped(a.workRow(&r, 25), metric.label, ui.FontSizeSmall, ui.ColorTextDim)
	}
	a.workGap(body, 14)
	if len(st.issues) == 0 {
		r := a.workPanel(a.workRow(body, 100))
		a.UI.Text(a.workRow(&r, 30), "Materials are ready", ui.FontSizeHeader, ui.ColorSuccess)
		a.UI.TextWrapped(r, "No missing maps, pixel-scale mismatches, or unpainted surfaces were found.", ui.FontSizeUI, ui.ColorTextDim)
		return
	}
	a.UI.Text(a.workRow(body, 28), "Select an issue to highlight and frame its face.", ui.FontSizeSmall, ui.ColorTextDim)
	pages := (len(st.issues) + 4) / 5
	st.issuePage = max(0, min(st.issuePage, pages-1))
	for i := st.issuePage * 5; i < min(len(st.issues), (st.issuePage+1)*5); i++ {
		v := st.issues[i]
		r := a.workRow(body, 59)
		r.Height -= a.px(7)
		a.UI.FillRounded(r, 6, ui.ColorPanel)
		inner := ui.Inset(r, a.px(9))
		badge, text := ui.SplitLeft(inner, a.px(98))
		badge.Height = a.px(25)
		kind := map[string]string{"missing_pbr": "Missing map", "stale_pbr": "Refresh PBR", "pixel_resolution": "Pixel scale", "dimensions": "Dimensions", "unpainted": "Unpainted", "coverage": "Coverage"}[v.Kind]
		text.X += a.px(10)
		text.Width -= a.px(10)
		if a.workButton(fmt.Sprintf("issue.%d", i), r, "", ui.ButtonGhost, nil, false, v.Message) {
			a.Sel.Set(model.FaceRef(v.Body, v.Face))
			st.face = model.FaceScope{Body: v.Body, Face: v.Face}
			a.LookAtSelection(a.layout.RenderViewport())
			st.open = false
		}
		a.workBadge(badge, kind, ui.ColorWarn)
		a.UI.TextWrapped(text, v.Message, ui.FontSizeSmall, ui.ColorText)
	}
	a.workGap(body, 3)
	a.workPager(body, "issues", &st.issuePage, pages)
}
