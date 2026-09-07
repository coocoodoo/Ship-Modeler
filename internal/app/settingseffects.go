package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/appearance"
	"modeler/internal/ui"
)

func (a *App) storeEffects() {
	e := a.UI.Effects
	e.Normalize()
	a.UI.Effects = e
	a.Settings.UIEffects = &e
	a.UI.AdvanceAppearance(0)
	a.saveInterfacePreference()
}

func (a *App) buildEffectsSettings(body rl.Rectangle) {
	row := func(h float32) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, h); return r }
	a.UI.Text(row(a.px(24)), "Effects & animation", ui.FontSizeHeader, ui.ColorText)
	a.UI.Text(row(a.px(24)), "Try the button. Changes apply throughout the interface.", ui.FontSizeSmall, ui.ColorTextDim)
	preview := row(a.px(74))
	button, art := ui.SplitLeft(preview, preview.Width*.54)
	button = ui.InsetXY(button, a.px(6), a.px(16))
	if a.UI.Button(ui.MakeID("effects.preview.button"), button, "Try animation", ui.ButtonOpts{Style: ui.ButtonPrimary, Icon: ui.DrawBodyIcon, Tooltip: "Click to preview press feedback and spring motion"}) {
		if a.effectsPreview > .5 {
			a.effectsPreview = .2
		} else {
			a.effectsPreview = .8
		}
	}
	art = ui.InsetXY(art, a.px(8), a.px(6))
	a.UI.MotionSVG(art)
	track := row(a.px(14))
	a.UI.Progress(ui.MakeID("effects.preview.progress"), ui.InsetXY(track, a.px(6), a.px(3)), a.effectsPreview)
	a.UI.Text(row(a.px(24)), "Button & card effects", ui.FontSizeUI, ui.ColorText)
	pair := func(l1 string, b1 *bool, l2 string, b2 *bool) {
		r := row(a.px(32))
		left, right := ui.SplitLeft(r, r.Width/2)
		if a.UI.Toggle(ui.MakeID("effects."+l1), left, l1, *b1, ui.ButtonOpts{}) {
			*b1 = !*b1
			a.storeEffects()
		}
		if a.UI.Toggle(ui.MakeID("effects."+l2), right, l2, *b2, ui.ButtonOpts{}) {
			*b2 = !*b2
			a.storeEffects()
		}
	}
	e := &a.UI.Effects
	pair("Button glow", &e.Glow, "Color cycling", &e.ColorCycle)
	pair("Click ripples", &e.Ripple, "Gradient surfaces", &e.Gradients)
	pair("Hover lift", &e.HoverLift, "Card entrances", &e.Entrances)
	press := row(a.px(36))
	label, control := ui.SplitLeft(press, a.px(150))
	a.UI.Text(label, "Press effect", ui.FontSizeUI, ui.ColorText)
	pressValues := []string{"none", "subtle", "bounce", "rubber", "gelatin"}
	index := 0
	for i, v := range pressValues {
		if e.PressEffect == v {
			index = i
		}
	}
	if chosen, changed := a.UI.Select(ui.MakeID("effects.press"), control, []string{"None", "Subtle", "Bounce", "Rubber band", "Gelatin"}, index); changed {
		e.PressEffect = pressValues[chosen]
		a.storeEffects()
	}
	a.UI.Text(row(a.px(28)), "Icons & SVG", ui.FontSizeUI, ui.ColorText)
	pair("Spin icons", &e.IconSpin, "Pulse icons", &e.IconPulse)
	pair("Animate SVG", &e.SVGAnimation, "SVG path motion", &e.PathMotion)
	a.UI.Text(row(a.px(28)), "Menus & motion", ui.FontSizeUI, ui.ColorText)
	pair("Menu entrances", &e.MenuAnimation, "Tooltips", &e.Tooltips)
	pair("Theme transitions", &e.ThemeFade, "Rotate part preview", &e.PreviewRotation)
	motion := row(a.px(36))
	label, control = ui.SplitLeft(motion, a.px(150))
	a.UI.Text(label, "Motion physics", ui.FontSizeUI, ui.ColorText)
	styles := []string{"ease", "spring", "bouncy"}
	index = 0
	for i, v := range styles {
		if e.MotionStyle == v {
			index = i
		}
	}
	if chosen, changed := a.UI.Select(ui.MakeID("effects.motion"), control, []string{"Smooth easing", "Spring", "Bouncy spring"}, index); changed {
		e.MotionStyle = styles[chosen]
		a.storeEffects()
	}
	a.UI.Text(row(a.px(24)), "Animation speed", ui.FontSizeUI, ui.ColorText)
	index = 0
	if a.Settings.UIMotion == "slow" {
		index = 1
	}
	if a.Settings.UIMotion == "off" {
		index = 2
	}
	if chosen, changed := a.UI.ChipGroup(ui.MakeID("effects.speed"), row(a.px(32)), []string{"Normal", "Slow", "Reduced"}, index, ui.ChipGroupOpts{}); changed {
		a.setUIMotion([]string{"", "slow", "off"}[chosen])
	}
	a.UI.Text(row(a.px(30)), "Reduced freezes motion and keeps your effect choices for later.", ui.FontSizeSmall, ui.ColorTextDim)
	if a.UI.Button(ui.MakeID("effects.reset"), row(a.px(30)), "Reset effects", ui.ButtonOpts{Style: ui.ButtonGhost, Tooltip: "Restore the default effects and normal animation speed"}) {
		a.UI.Effects = appearance.Defaults()
		a.setUIMotion("")
		a.storeEffects()
	}
}
