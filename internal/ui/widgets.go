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

	var fill, text color.RGBA
	switch opts.Style {
	case ButtonPrimary:
		fill, text = ColorAccent, ColorBG
	case ButtonDanger:
		fill, text = ColorError, ColorBG
	case ButtonGhost:
		fill, text = color.RGBA{}, ColorText
		if it.Hovered && !it.Disabled {
			fill = ColorHover
		}
	default:
		fill, text = ColorCard, ColorText
	}
	if fill.A > 0 {
		c.FillRounded(r, CornerRadius, stateColor(fill, it))
	}
	if opts.Style == ButtonNormal {
		c.StrokeRounded(r, CornerRadius, ColorStroke)
	}
	c.TextCentered(r, label, FontSizeUI, textColorFor(text, it))

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
}

// IconButton draws a stroke icon in a square hit area with an optional label.
// The active tool gets an accent underline and a tinted icon (SPEC-UX §4).
func (c *Context) IconButton(id ID, r rl.Rectangle, icon IconFunc, opts IconOpts) bool {
	it := c.interact(id, r, opts.Disabled)

	if it.Hovered && !it.Disabled {
		c.FillRounded(r, CornerRadius, ColorHover)
	}
	if it.Pressed && !it.Disabled {
		c.FillRounded(r, CornerRadius, Fade(ColorHover, 1.6))
	}

	tint := ColorTextDim
	if opts.Active {
		tint = ColorAccent
	} else if it.Hovered {
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
		icon(float64(center.X), float64(center.Y), IconSize*c.Scale, tint)
	}

	if opts.Active {
		// Accent underline, inset a little so it reads as a tab indicator.
		u := Rect(r.X+c.Px(4), r.Y+r.Height-c.Px(2), r.Width-c.Px(8), c.Px(2))
		FillRect(u, ColorAccent)
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

	knobBox, labelBox := SplitLeft(r, c.Px(30))
	track := InsetXY(knobBox, c.Px(2), (knobBox.Height-c.Px(14))/2)

	trackCol := ColorStroke
	if on {
		trackCol = ColorAccent
	}
	c.FillRounded(track, 7, stateColor(trackCol, it))

	knobD := track.Height - c.Px(4)
	knobX := track.X + c.Px(2)
	if on {
		knobX = track.X + track.Width - knobD - c.Px(2)
	}
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
	filled := track
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

		fill := ColorCard
		text := ColorTextDim
		if i == selected {
			fill, text = ColorAccent, ColorBG
		}
		c.FillRounded(chip, CornerRadius, stateColor(fill, it))
		if i != selected {
			c.StrokeRounded(chip, CornerRadius, ColorStroke)
		}
		c.TextCentered(chip, label, FontSizeSmall, textColorFor(text, it))

		c.queueTooltip(cid, chip, it, opts.Tooltip, "", why)
		if it.Clicked && i != selected {
			selected, changed = i, true
		}
	}
	return selected, changed
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
