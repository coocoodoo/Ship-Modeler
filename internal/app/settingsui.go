package app

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/ui"
)

var uiSizeChoices = []struct {
	label string
	value float64
}{{"Auto", 0}, {"100%", 1}, {"125%", 1.25}, {"150%", 1.5}, {"200%", 2}}

func (a *App) setUISize(value float64) {
	valid := false
	for _, choice := range uiSizeChoices {
		valid = valid || value == choice.value
	}
	if !valid {
		return
	}
	a.Settings.UIScaleOverride = value
	a.pendingUISize = true
	if !a.Headless {
		if err := a.Settings.Save(); err != nil {
			a.Toast(ui.Toast{Text: "UI size changed, but the preference couldn't be saved", Kind: ui.ToastWarn})
		}
	}
}

// Apply between frames so no queued drawing still references the old fonts.
func (a *App) applyPendingUISize() {
	if !a.pendingUISize {
		return
	}
	a.pendingUISize = false
	scale := a.Settings.UIScaleOverride
	if scale == 0 {
		scale = 1
		if !a.Headless {
			scale = float64(rl.GetWindowScaleDPI().X)
		}
	}
	scale = ui.Scale(scale)
	if scale == a.Scale {
		return
	}
	fonts := ui.LoadFonts(scale)
	rl.DrawRenderBatchActive()
	a.Fonts.Unload()
	a.Fonts, a.Scale = fonts, scale
	a.UI.SetScale(fonts, scale)
}

// Explicit appearance choices take precedence over a legacy theme.json while
// leaving that file intact for users who customized it.
func loadAppearance(name string) string {
	ui.ApplyAppearance(name)
	if name == "" {
		return loadThemeFile()
	}
	return ""
}

func (a *App) setAppearance(name string) {
	if name != "dark" && name != "light" {
		return
	}
	a.Settings.Theme = name
	a.Settings.AppearancePalette = ""
	a.transitionAppearance(func() { ui.ApplyAppearance(name) })
	if !a.Headless {
		if err := a.Settings.Save(); err != nil {
			a.Toast(ui.Toast{Text: "Theme changed, but the preference couldn't be saved", Kind: ui.ToastWarn})
		}
	}
}

func (a *App) transitionAppearance(apply func()) {
	if a.UI != nil && a.UI.Fonts != nil {
		a.UI.TransitionTheme(apply)
	} else {
		apply()
	}
	ui.SetAccent(a.modeAccent())
}

func (a *App) setAppearancePalette(name string) {
	p, ok := ui.FindPalette(name)
	if !ok {
		return
	}
	a.Settings.AppearancePalette = p.Name
	a.Settings.Theme = "light"
	if p.Dark {
		a.Settings.Theme = "dark"
	}
	a.transitionAppearance(func() { ui.ApplyPalette(p.Name) })
	a.saveInterfacePreference()
}

func (a *App) saveInterfacePreference() {
	if !a.Headless {
		if err := a.Settings.Save(); err != nil {
			a.Toast(ui.Toast{Text: "Preference changed, but couldn't be saved", Kind: ui.ToastWarn})
		}
	}
}

func (a *App) setUIMotion(value string) {
	if value != "" && value != "slow" && value != "off" {
		return
	}
	a.Settings.UIMotion, a.UI.Motion = value, value
	a.UI.AdvanceAppearance(0)
	a.saveInterfacePreference()
}

func (a *App) buildSettingsDialog() {
	if !a.showSettings || a.UI.ModalOpen() {
		return
	}
	screen := a.layout.Screen
	w, h := min(a.px(568), screen.Width-16), min(a.px(530), screen.Height-16)
	box := ui.Rect(screen.X+(screen.Width-w)/2, screen.Y+(screen.Height-h)/2, w, h)
	card := a.UI.FloatingCard(ui.MakeID("settings.dialog"), box, "Settings", ui.FloatingCardOpts{})
	footer, content := ui.SplitBottom(card.Body, a.px(34))
	content.Height -= a.px(12)
	content.Width -= a.px(8)
	total := a.px(406)
	if a.settingsEffects {
		total = a.px(664)
	}
	maxScroll := max(0, float64((total-content.Height)/a.px(1)))
	a.settingsScroll = min(maxScroll, max(0, a.settingsScroll-a.UI.ScrollWheel(content)*32))
	a.UI.Clip(content, func() {
		body := ui.Rect(content.X, content.Y-a.px(a.settingsScroll), content.Width, total)
		if a.settingsEffects {
			a.buildEffectsSettings(body)
			return
		}
		row := func(h float32) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, h); return r }
		a.UI.Text(row(a.px(22)), "Your workspace, your style.", ui.FontSizeSmall, ui.ColorTextDim)
		row(a.px(8))
		a.UI.Text(row(a.px(20)), "Appearance", ui.FontSizeUI, ui.ColorText)
		choices := row(a.px(32))
		dark := a.Settings.Theme != "light"
		for i, name := range []string{"dark", "light"} {
			label := "Dark"
			if name == "light" {
				label = "Light"
			}
			style := ui.ButtonNormal
			if (i == 0) == dark {
				style = ui.ButtonPrimary
			}
			rect := ui.Rect(choices.X+float32(i)*(choices.Width+a.px(8))/2, choices.Y, (choices.Width-a.px(8))/2, choices.Height)
			if a.UI.Button(ui.MakeID("settings.theme."+name), rect, label, ui.ButtonOpts{Style: style}) {
				a.setAppearance(name)
			}
		}
		row(a.px(12))
		paletteLabel := "Color palette"
		if a.Settings.AppearancePalette != "" {
			paletteLabel += "  /  " + a.Settings.AppearancePalette
		}
		a.UI.Text(row(a.px(24)), paletteLabel, ui.FontSizeSmall, ui.ColorTextDim)
		paletteBox := row(a.px(106))
		palettes := ui.Palettes(dark)
		tileW := (paletteBox.Width - a.px(24)) / 5
		for i, p := range palettes {
			rect := ui.Rect(paletteBox.X+float32(i%5)*(tileW+a.px(6)), paletteBox.Y+float32(i/5)*a.px(56), tileW, a.px(50))
			if a.UI.PaletteTile(ui.MakeID("settings.palette."+p.Name), rect, p, p.Name == a.Settings.AppearancePalette) {
				a.setAppearancePalette(p.Name)
			}
		}
		row(a.px(12))
		a.UI.Text(row(a.px(24)), "UI size", ui.FontSizeUI, ui.ColorText)
		sizes := row(a.px(32))
		for i, choice := range uiSizeChoices {
			style := ui.ButtonNormal
			if choice.value == a.Settings.UIScaleOverride {
				style = ui.ButtonPrimary
			}
			width := (sizes.Width - a.px(4)*float32(len(uiSizeChoices)-1)) / float32(len(uiSizeChoices))
			rect := ui.Rect(sizes.X+float32(i)*(width+a.px(4)), sizes.Y, width, sizes.Height)
			if a.UI.Button(ui.MakeID("settings.size."+choice.label), rect, choice.label, ui.ButtonOpts{Style: style}) {
				a.setUISize(choice.value)
			}
		}
		a.UI.Text(row(a.px(24)), fmt.Sprintf("Auto follows your display. Current size: %.0f%%", a.Scale*100), ui.FontSizeSmall, ui.ColorTextDim)
		row(a.px(8))
		a.UI.Text(row(a.px(24)), "Interface motion", ui.FontSizeUI, ui.ColorText)
		motion := row(a.px(32))
		selected := 0
		if a.Settings.UIMotion == "slow" {
			selected = 1
		}
		if a.Settings.UIMotion == "off" {
			selected = 2
		}
		if value, changed := a.UI.ChipGroup(ui.MakeID("settings.motion"), motion, []string{"Normal", "Slow", "Reduced"}, selected, ui.ChipGroupOpts{}); changed {
			a.setUIMotion([]string{"", "slow", "off"}[value])
		}
		a.UI.Text(row(a.px(26)), "Reduced motion turns off interface animation and preview rotation.", ui.FontSizeSmall, ui.ColorTextDim)
	})
	if maxScroll > 0 {
		track := ui.Rect(content.X+content.Width+a.px(3), content.Y, a.px(3), content.Height)
		a.UI.FillRounded(track, 2, ui.ColorStroke)
		thumbH := max(a.px(24), content.Height*content.Height/total)
		thumb := ui.Rect(track.X, track.Y+float32(a.settingsScroll/maxScroll)*(track.Height-thumbH), track.Width, thumbH)
		a.UI.FillRounded(thumb, 2, ui.ColorTextDim)
	}
	a.UI.HairlineH(footer.X, footer.Y-a.px(6), footer.Width, ui.ColorStroke)
	done, connection := ui.SplitRight(footer, a.px(86))
	connection.Width -= a.px(8)
	page, connection := ui.SplitLeft(connection, a.px(112))
	connection.X += a.px(8)
	connection.Width -= a.px(8)
	pageLabel := "Effects"
	if a.settingsEffects {
		pageLabel = "Appearance"
	}
	if a.UI.Button(ui.MakeID("settings.page"), page, pageLabel, ui.ButtonOpts{Icon: ui.DrawSettingsIcon}) {
		a.settingsEffects = !a.settingsEffects
		a.settingsScroll = 0
		a.UI.ClearFocus()
	}
	label := "AI connection: Off"
	if a.ai != nil {
		label = "AI connection: On"
	}
	if a.UI.Button(ui.MakeID("settings.ai"), connection, label, ui.ButtonOpts{Tooltip: "Allow local AI tools to inspect, model and paint in this session"}) {
		a.aiToggle = true
	}
	close := a.UI.Button(ui.MakeID("settings.done"), done, "Done", ui.ButtonOpts{Style: ui.ButtonPrimary, Icon: ui.DrawCheckIcon})
	a.UI.ClaimPointer(a.layout.Screen)
	if close || (a.UI.In.KeyPressed(rl.KeyEscape) && !a.UI.MenuWasOpen()) {
		a.showSettings = false
		a.UI.ClearFocus()
	}
}
