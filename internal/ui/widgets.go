package ui

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The frozen widget set of SPEC-UX §4. Additions need a DECISIONS entry.

// ButtonStyle selects a button's visual weight.
type ButtonStyle uint8

const (
	// ButtonNormal is the default card-coloured button.
	ButtonNormal ButtonStyle = iota
	// ButtonPrimary is the accent-filled confirm button.
	ButtonPrimary
	// ButtonGhost has no fill until hovered, for low-emphasis actions.
	ButtonGhost
	// ButtonDanger is used for destructive confirmations.
	ButtonDanger
)

// ButtonOpts configures a button.
type ButtonOpts struct {
	Icon     IconFunc
	IconEnd  IconFunc
	Style    ButtonStyle
	Disabled bool
	// Tooltip and Shortcut are shown after the hover delay. A disabled control
	// must say how to enable it, so DisabledWhy replaces Tooltip when set
	// (SPEC-UX §15).
	Tooltip     string
	Shortcut    string
	DisabledWhy string
}

// Button draws a labelled button and reports whether it was clicked.
func (c *Context) Button(id ID, r rl.Rectangle, label string, opts ButtonOpts) bool {
	it := c.interact(id, r, opts.Disabled)
	c.describeControl(id, "button", label)
	r = c.buttonFeedback(id, r, it)
	endVisual := c.buttonTransform(id, r, it)
	defer endVisual()
	hover := c.hoverAmount(id, it.Hovered && !it.Disabled)

	var fill, text color.RGBA
	switch opts.Style {
	case ButtonPrimary:
		fill = c.effectAccent(ColorAccent)
		text = onAccent(fill)
	case ButtonDanger:
		fill, text = ColorError, onAccent(ColorError)
	case ButtonGhost:
		fill, text = color.RGBA{}, ColorText
		fill = Fade(ColorHover, hover)
	default:
		fill, text = blendColor(ColorCard, ColorAccent, 0.06), ColorText
	}
	c.buttonGlow(r, fill, !it.Disabled && (opts.Style == ButtonPrimary || it.Hovered))
	if fill.A > 0 {
		if opts.Style != ButtonGhost {
			fill = blendColor(fill, ColorText, hover*0.07)
		}
		visual := it
		visual.Hovered = false
		fill = stateColor(fill, visual)
		if opts.Style == ButtonPrimary && !it.Disabled {
			c.Shadow(Inset(r, c.Px(2)), CornerRadius, .45)
			c.FillGradientRounded(r, CornerRadius, fill, blendColor(fill, ColorCard, .09))
		} else {
			c.FillRounded(r, CornerRadius, fill)
		}
	}
	switch opts.Style {
	case ButtonNormal:
		c.StrokeRounded(r, CornerRadius, ColorStroke)
	case ButtonPrimary, ButtonDanger:
		// A filled button carries the top-edge light every raised surface
		// gets; disabled ones lie flat.
		if !it.Disabled {
			c.StrokeRounded(r, CornerRadius, Fade(ColorText, 0.14))
		}
	}
	c.drawRipple(id, r, text)
	if opts.Icon == nil {
		labelBox := r
		if opts.IconEnd != nil {
			iconBox, rest := SplitRight(r, c.Px(26))
			labelBox = rest
			ctr := Center(iconBox)
			c.animatedIcon(opts.IconEnd, float64(ctr.X), float64(ctr.Y), IconSize*c.Scale, textColorFor(text, it), false)
		}
		c.TextCentered(labelBox, label, FontSizeUI, textColorFor(text, it))
	} else {
		label = c.Truncate(label, FontSizeUI, r.Width-c.Px(28))
		width := c.TextWidth(label, FontSizeUI) + c.Px(20)
		content := Rect(r.X+(r.Width-width)/2, r.Y, width, r.Height)
		icon, labelBox := SplitLeft(content, c.Px(20))
		ctr := Center(icon)
		c.animatedIcon(opts.Icon, float64(ctr.X), float64(ctr.Y), 15*c.Scale, textColorFor(text, it), !it.Disabled && (it.Hovered || opts.Style == ButtonPrimary))
		c.Text(labelBox, label, FontSizeUI, textColorFor(text, it))
	}

	c.queueTooltip(id, r, it, opts.Tooltip, opts.Shortcut, opts.DisabledWhy)
	return it.Clicked
}

// IconOpts configures an icon button.
type IconOpts struct {
	Disabled    bool
	Active      bool // the tool this button represents is the current one
	Tooltip     string
	Shortcut    string
	DisabledWhy string
	Label       string // optional text under or beside the icon
	// Accent, when set, is this button's own colour instead of the live mode
	// accent: its active wash and underline, and — so a mode button carries
	// its colour even when it is not the mode — its resting icon tint too.
	Accent color.RGBA
}

// IconButton draws a stroke icon in a square hit area with an optional label.
// The active tool gets an accent underline and a tinted icon (SPEC-UX §4).
func (c *Context) IconButton(id ID, r rl.Rectangle, icon IconFunc, opts IconOpts) bool {
	it := c.interact(id, r, opts.Disabled)
	label := opts.Label
	if label == "" {
		label = opts.Tooltip
	}
	c.describeControl(id, "button", label)
	r = c.buttonFeedback(id, r, it)
	endVisual := c.buttonTransform(id, r, it)
	defer endVisual()

	accent := ColorAccent
	if opts.Accent.A > 0 {
		accent = opts.Accent
	}
	if opts.Active {
		accent = c.effectAccent(accent)
	}
	c.buttonGlow(r, accent, !it.Disabled && (opts.Active || it.Hovered))

	// The active tool gets a soft accent wash as well as its underline, so the
	// current mode reads from across the room and not only from two pixels.
	if opts.Active {
		c.FillGradientRounded(r, CornerRadius, blendColor(ColorPanel, accent, .19), blendColor(ColorPanel, accent, .08))
		c.StrokeRounded(r, CornerRadius, Fade(accent, 0.35))
	}
	c.FillRounded(r, CornerRadius, Fade(ColorHover, c.hoverAmount(id, it.Hovered && !it.Disabled)))
	if it.Pressed && !it.Disabled {
		c.FillRounded(r, CornerRadius, Fade(ColorHover, 1.6))
	}
	c.drawRipple(id, r, accent)

	tint := ColorTextDim
	switch {
	case opts.Active:
		tint = accent
	case it.Hovered:
		tint = ColorText
	}
	tint = textColorFor(tint, it)

	iconBox := r
	if opts.Label != "" {
		var labelBox rl.Rectangle
		iconBox, labelBox = SplitLeft(r, c.Px(IconSize)+c.Px(Spacing))
		c.Text(labelBox, opts.Label, FontSizeUI, tint)
	}
	center := Center(iconBox)
	if icon != nil {
		c.animatedIcon(icon, float64(center.X), float64(center.Y), IconSize*c.Scale, tint, !it.Disabled && (opts.Active || it.Hovered))
	}

	c.queueTooltip(id, r, it, opts.Tooltip, opts.Shortcut, opts.DisabledWhy)
	return it.Clicked
}

// Eye draws the visibility toggle used by every tree row. It returns true when
// the user flipped it, and the caller applies the change through the bus.
func (c *Context) Eye(id ID, r rl.Rectangle, visible bool, opts IconOpts) bool {
	it := c.interact(id, r, opts.Disabled)

	if it.Hovered && !it.Disabled {
		c.FillRounded(r, CornerRadius, ColorHover)
	}
	tint := ColorTextDim
	if !visible {
		tint = Fade(ColorTextDim, 0.55)
	}
	if it.Hovered {
		tint = ColorText
	}
	center := Center(r)
	DrawEyeIcon(float64(center.X), float64(center.Y), IconSize*c.Scale, visible, textColorFor(tint, it))

	tip := opts.Tooltip
	if tip == "" {
		tip = "Hide"
		if !visible {
			tip = "Show"
		}
	}
	c.queueTooltip(id, r, it, tip, opts.Shortcut, opts.DisabledWhy)
	return it.Clicked
}

// Toggle draws a labelled on/off switch.
func (c *Context) Toggle(id ID, r rl.Rectangle, label string, on bool, opts ButtonOpts) bool {
	it := c.interact(id, r, opts.Disabled)
	c.describeControl(id, "toggle", label)
	target := 0.0
	if on {
		target = 1
	}
	amount := clamp01(c.easeAmount(id.Child("value"), target, 65, true))

	knobBox, labelBox := SplitLeft(r, c.Px(30))
	track := InsetXY(knobBox, c.Px(2), (knobBox.Height-c.Px(14))/2)

	trackCol := blendColor(ColorStroke, ColorAccent, amount)
	c.FillRounded(track, 7, stateColor(trackCol, it))

	knobD := track.Height - c.Px(4)
	knobX := track.X + c.Px(2)
	knobX += (track.Width - knobD - c.Px(4)) * float32(amount)
	knob := Rect(knobX, track.Y+c.Px(2), knobD, knobD)
	c.FillRounded(knob, float64(knobD), textColorFor(ColorText, it))

	if label != "" {
		lb := labelBox
		lb.X += c.Px(Spacing / 2)
		c.Text(lb, label, FontSizeUI, textColorFor(ColorText, it))
	}
	c.queueTooltip(id, r, it, opts.Tooltip, opts.Shortcut, opts.DisabledWhy)
	return it.Clicked
}

// Slider drags a value between min and max. It returns the new value and
// whether it changed this frame.
func (c *Context) Slider(id ID, r rl.Rectangle, value, min, max float64, opts ButtonOpts) (float64, bool) {
	it := c.interact(id, r, opts.Disabled)
	if max <= min {
		return value, false
	}

	trackH := c.Px(4)
	track := Rect(r.X, r.Y+(r.Height-trackH)/2, r.Width, trackH)
	c.FillRounded(track, 2, ColorStroke)

	t := (value - min) / (max - min)
	t = clamp01(t)
	if !it.Pressed {
		t = c.easeAmount(id.Child("value"), t, 75, true)
	} else {
		if c.hoverTransitions == nil {
			c.hoverTransitions = make(map[ID]hoverTransition)
		}
		c.hoverTransitions[id.Child("value")] = hoverTransition{value: t, frame: c.frameNumber}
	}
	filled := track
	// Springs may overshoot a target inside the track, but never its bounds.
	t = clamp01(t)
	filled.Width = float32(t) * track.Width
	c.FillRounded(filled, 2, stateColor(ColorAccent, it))

	knobD := c.Px(12)
	knob := Rect(track.X+filled.Width-knobD/2, r.Y+(r.Height-knobD)/2, knobD, knobD)
	c.FillRounded(knob, float64(knobD), textColorFor(ColorText, it))

	changed := false
	if it.Pressed && !opts.Disabled {
		nt := clamp01(float64((float32(c.In.MouseX) - track.X) / track.Width))
		nv := min + nt*(max-min)
		if nv != value {
			value, changed = nv, true
		}
	}
	c.queueTooltip(id, r, it, opts.Tooltip, opts.Shortcut, opts.DisabledWhy)
	return value, changed
}

// ChipGroup is an exclusive row of small buttons, e.g. the paint resolutions
// 16/32/128/256/512 or the extrude direction (SPEC-UX §4).
type ChipGroupOpts struct {
	Disabled    bool
	DisabledWhy string
	// PerChipDisabled disables individual chips, each with its own reason.
	PerChipDisabled []bool
	PerChipWhy      []string
	Tooltip         string
}

// ChipGroup draws the chips left to right across r and returns the selected
// index and whether it changed.
func (c *Context) ChipGroup(id ID, r rl.Rectangle, labels []string, selected int, opts ChipGroupOpts) (int, bool) {
	if len(labels) == 0 {
		return selected, false
	}
	gap := c.Px(4)
	w := (r.Width - gap*float32(len(labels)-1)) / float32(len(labels))
	changed := false

	for i, label := range labels {
		chip := Rect(r.X+float32(i)*(w+gap), r.Y, w, r.Height)
		disabled := opts.Disabled
		why := opts.DisabledWhy
		if i < len(opts.PerChipDisabled) && opts.PerChipDisabled[i] {
			disabled = true
			if i < len(opts.PerChipWhy) {
				why = opts.PerChipWhy[i]
			}
		}
		cid := id.Child(label)
		it := c.interact(cid, chip, disabled)
		c.describeControl(cid, "choice", label)

		fill := ColorCard
		text := ColorTextDim
		if i == selected {
			fill, text = blendColor(ColorCard, ColorAccent, 0.20), ColorText
		}
		c.FillRounded(chip, CornerRadius, stateColor(fill, it))
		if i == selected {
			c.StrokeRounded(chip, CornerRadius, Fade(ColorAccent, 0.42))
		}
		c.TextCentered(chip, label, FontSizeSmall, textColorFor(text, it))

		c.queueTooltip(cid, chip, it, opts.Tooltip, "", why)
		if it.Clicked && i != selected {
			selected, changed = i, true
		}
	}
	return selected, changed
}

// SwatchRowSpec describes one row of a list that names a set of colours.
type SwatchRowSpec struct {
	Label string
	// Swatches are drawn right-aligned, as many as fit in Columns.
	Swatches []color.RGBA
	// Columns is how many chips the strip has room for. A set longer than
	// that draws Columns-1 chips and a "+", because showing the first
	// fourteen of a hundred without saying so is a lie about the set.
	Columns int
	// Selected marks the row as the one in use: accent wash and edge bar.
	Selected bool
	Tooltip  string
}

// SwatchRow draws a name beside a strip of its colours and reports a click.
//
// Added for the palette browser (V-152). A list of colour sets cannot be read
// by name — "Sunset 12" and "Dusk" tell you nothing — so the row is mostly
// the colours themselves, and the name is the label on them.
func (c *Context) SwatchRow(id ID, r rl.Rectangle, spec SwatchRowSpec) bool {
	it := c.interact(id, r, false)

	switch {
	case spec.Selected:
		c.FillRounded(r, 4, Fade(ColorAccent, 0.20))
	case it.Hovered:
		c.FillRounded(r, 4, ColorHover)
	}
	if spec.Selected {
		FillRect(Rect(r.X, r.Y+c.Px(4), c.Px(2), r.Height-c.Px(8)), ColorAccent)
	}

	inner := InsetXY(r, c.Px(Spacing), 0)
	cols := spec.Columns
	if cols < 1 {
		cols = 1
	}
	chip := c.Px(13)
	gap := c.Px(2)
	stripW := float32(cols)*(chip+gap) - gap
	nameBox, stripBox := SplitLeft(inner, inner.Width-stripW-c.Px(Spacing))

	label := ColorText
	if spec.Selected {
		label = ColorAccent
	}
	c.Text(nameBox, spec.Label, FontSizeUI, textColorFor(label, it))

	n := len(spec.Swatches)
	show := n
	if show > cols {
		show = cols - 1
	}
	y := stripBox.Y + (stripBox.Height-chip)/2
	for i := 0; i < show; i++ {
		box := Rect(stripBox.X+float32(i)*(chip+gap), y, chip, chip)
		c.FillRounded(box, 2, spec.Swatches[i])
	}
	if show < n {
		box := Rect(stripBox.X+float32(show)*(chip+gap), y, chip, chip)
		c.TextCentered(box, "+", FontSizeSmall, ColorTextDim)
	}

	c.queueTooltip(id, r, it, spec.Tooltip, "", "")
	return it.Clicked
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
