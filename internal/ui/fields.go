package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The two costly widgets of SPEC-UX §4: the drag-number field (drag to scrub
// with snapping, click to type, Enter commits, Escape reverts) and the text
// field used for renaming.

// editState is the live buffer of whichever field holds keyboard focus.
type editState struct {
	text  []rune
	caret int
	// sel is the other end of the selection, or -1 when nothing is selected.
	sel int
	// original is what the field held when editing began, so Escape can revert.
	original string
	// blink accumulates so the caret pulses.
	blink   float64
	scrollY float32
}

func (e *editState) String() string { return string(e.text) }

func (e *editState) selRange() (int, int, bool) {
	if e.sel < 0 || e.sel == e.caret {
		return 0, 0, false
	}
	a, b := e.caret, e.sel
	if a > b {
		a, b = b, a
	}
	return a, b, true
}

func (e *editState) deleteSelection() bool {
	a, b, ok := e.selRange()
	if !ok {
		return false
	}
	e.text = append(e.text[:a], e.text[b:]...)
	e.caret, e.sel = a, -1
	return true
}

func (e *editState) insert(rs []rune) {
	e.deleteSelection()
	rest := append([]rune(nil), e.text[e.caret:]...)
	e.text = append(e.text[:e.caret], rs...)
	e.text = append(e.text, rest...)
	e.caret += len(rs)
	e.sel = -1
}

func (e *editState) selectAll() {
	e.sel = 0
	e.caret = len(e.text)
}

// FieldResult is what a text or number field reports for a frame.
type FieldResult struct {
	// Text is the live buffer while editing, or the incoming value otherwise.
	Text string
	// Editing is true while the field holds keyboard focus.
	Editing bool
	// Committed is set on the frame Enter is pressed or focus leaves cleanly.
	Committed bool
	// Cancelled is set on the frame Escape reverts the edit.
	Cancelled bool
	// Changed is set when a drag or an accepted edit produced a new value.
	Changed bool
}

// TextFieldOpts configures a text field.
type TextFieldOpts struct {
	LeadingIcon IconFunc
	// Live reports the editing buffer on every keystroke, for previews.
	Live        bool
	Disabled    bool
	Placeholder string
	// SelectAllOnFocus starts an edit with the whole value selected, which is
	// what a rename wants.
	SelectAllOnFocus bool
	Tooltip          string
}

// TextField edits a string. It returns the live text plus the commit and
// cancel edges; the caller only writes to the document on Committed.
func (c *Context) TextField(id ID, r rl.Rectangle, text string, opts TextFieldOpts) FieldResult {
	it := c.interact(id, r, opts.Disabled)
	controlText := text
	if c.focus == id {
		controlText = c.edit.String()
	}
	c.describeControl(id, "text", controlText+" "+opts.Placeholder)
	res := FieldResult{Text: text}

	if it.Clicked && c.focus != id {
		c.beginEdit(id, text, opts.SelectAllOnFocus)
	}
	// Clicking anywhere else commits the edit rather than silently dropping it.
	if c.focus == id && c.In.Pressed[MouseLeft] && !it.Hovered {
		res.Text, res.Committed, res.Changed = c.edit.String(), true, c.edit.String() != c.edit.original
		c.ClearFocus()
	}

	editing := c.focus == id
	res.Editing = editing

	fill := ColorCard
	border := ColorStroke
	if editing {
		border = ColorAccent
		c.StrokeRounded(Inset(r, -c.Px(2)), CornerRadius+2, Fade(ColorAccent, 0.35))
	}
	c.FillRounded(r, CornerRadius, stateColor(fill, it))
	c.StrokeRounded(r, CornerRadius, border)

	inner := InsetXY(r, c.Px(Spacing/2), 0)
	if opts.LeadingIcon != nil {
		icon, rest := SplitLeft(inner, c.Px(24))
		ctr := Center(icon)
		opts.LeadingIcon(float64(ctr.X), float64(ctr.Y), 16*c.Scale, ColorTextDim)
		inner = rest
	}

	if editing {
		c.edit.blink += c.In.DeltaMillis
		if r := c.handleTextKeys(); r.Committed || r.Cancelled {
			res = r
			c.ClearFocus()
		}
		if res.Cancelled {
			res.Text = c.textOrOriginal(text)
		}
		c.drawEditable(inner)
		if opts.Live && c.focus == id {
			res.Text = c.edit.String()
		}
	} else {
		display, col := text, ColorText
		if display == "" {
			display, col = opts.Placeholder, ColorTextDim
		}
		c.Text(inner, display, FontSizeUI, textColorFor(col, it))
	}

	c.queueTooltip(id, r, it, opts.Tooltip, "", "")
	return res
}

func (c *Context) textOrOriginal(fallback string) string {
	if c.edit.original != "" {
		return c.edit.original
	}
	return fallback
}

func (c *Context) beginEdit(id ID, text string, selectAll bool) {
	c.SetFocus(id, text)
	c.edit.original = text
	if selectAll {
		c.edit.selectAll()
	}
	c.wantKeyboard = true
}

// handleTextKeys applies this frame's typing to the focused editor.
func (c *Context) handleTextKeys() FieldResult {
	e := &c.edit
	c.wantKeyboard = true

	for _, ch := range c.In.Chars {
		if ch >= 32 && ch != 127 {
			e.insert([]rune{ch})
		}
	}
	for _, k := range c.In.KeysPressed {
		switch k {
		case rl.KeyBackspace:
			if !e.deleteSelection() && e.caret > 0 {
				e.text = append(e.text[:e.caret-1], e.text[e.caret:]...)
				e.caret--
			}
		case rl.KeyDelete:
			if !e.deleteSelection() && e.caret < len(e.text) {
				e.text = append(e.text[:e.caret], e.text[e.caret+1:]...)
			}
		case rl.KeyLeft:
			if e.caret > 0 {
				e.caret--
			}
			e.sel = -1
		case rl.KeyRight:
			if e.caret < len(e.text) {
				e.caret++
			}
			e.sel = -1
		case rl.KeyHome:
			e.caret, e.sel = 0, -1
		case rl.KeyEnd:
			e.caret, e.sel = len(e.text), -1
		case rl.KeyA:
			if c.In.Ctrl {
				e.selectAll()
			}
		case rl.KeyEnter, rl.KeyKpEnter:
			s := e.String()
			return FieldResult{Text: s, Committed: true, Changed: s != e.original}
		case rl.KeyEscape:
			return FieldResult{Text: e.original, Cancelled: true}
		}
	}
	return FieldResult{Text: e.String(), Editing: true}
}

// drawEditable renders the live buffer with its selection and caret.
func (c *Context) drawEditable(inner rl.Rectangle) {
	e := &c.edit
	s := e.String()
	font := c.fontFor(FontSizeUI)
	_, th := c.Fonts.Measure(font, "Ag", FontSizeUI)
	y := inner.Y + (inner.Height-th)/2

	if a, b, ok := e.selRange(); ok {
		x0 := inner.X + c.TextWidth(string(e.text[:a]), FontSizeUI)
		x1 := inner.X + c.TextWidth(string(e.text[:b]), FontSizeUI)
		FillRect(Rect(x0, y, x1-x0, th), Fade(ColorAccent, 0.45))
	}
	c.Fonts.Draw(font, s, inner.X, y, FontSizeUI, ColorText)

	// The caret blinks at roughly 1 Hz and is always solid right after a
	// keystroke, so typing never looks like it was dropped.
	if c.MotionFactor() == 0 || math.Mod(e.blink*c.MotionFactor(), 1000) < 600 || len(c.In.Chars) > 0 {
		cx := inner.X + c.TextWidth(string(e.text[:e.caret]), FontSizeUI)
		FillRect(Rect(cx, y, c.hairline(), th), ColorAccent)
	}
}

// dragState tracks a drag-number scrub in progress.
type dragState struct {
	active ID
	// startValue is the value when the drag began, so the scrub is absolute
	// rather than accumulating rounding error.
	startValue float64
	startX     float64
	moved      bool
}

// NumberOpts configures a drag-number field.
type NumberOpts struct {
	Disabled bool
	// Unit is appended to the display, e.g. "u", "°" or "px".
	Unit string
	// Step is the snap applied while dragging; FineStep is used with Ctrl held.
	Step     float64
	FineStep float64
	// PixelsPerStep is how far the pointer travels to advance one step.
	PixelsPerStep float64
	Min, Max      float64
	// Decimals controls the display precision.
	Decimals int
	// Warn draws the field in the warning colour, which is how a clamped draft
	// angle announces itself (SPEC-UX §9.3).
	Warn        bool
	WarnTooltip string
	Tooltip     string
	DisabledWhy string
}

func (o *NumberOpts) withDefaults() {
	if o.Step == 0 {
		o.Step = 1
	}
	if o.FineStep == 0 {
		o.FineStep = o.Step / 4
	}
	if o.PixelsPerStep == 0 {
		o.PixelsPerStep = 6
	}
	if o.Min == 0 && o.Max == 0 {
		o.Min, o.Max = math.Inf(-1), math.Inf(1)
	}
}

// DragNumber is the scrub-or-type numeric field. Dragging horizontally changes
// the value in snapped steps; clicking without moving switches to typing.
func (c *Context) DragNumber(id ID, r rl.Rectangle, value float64, opts NumberOpts) (float64, FieldResult) {
	opts.withDefaults()
	editing := c.focus == id

	if editing {
		res := c.numberEditor(id, r, value, opts)
		return res.value, res.result
	}

	it := c.interact(id, r, opts.Disabled)

	// Press begins a potential scrub; a release with no movement means "type".
	if it.Pressed && c.drag.active != id && c.In.Pressed[MouseLeft] {
		c.drag = dragState{active: id, startValue: value, startX: c.In.MouseX}
	}
	c.describeControl(id, "number", formatNumber(value, opts.Decimals))
	res := FieldResult{Text: formatNumber(value, opts.Decimals)}
	if c.drag.active == id {
		dx := c.In.MouseX - c.drag.startX
		if math.Abs(dx) > 2 {
			c.drag.moved = true
		}
		if c.drag.moved {
			step := opts.Step
			if c.In.Ctrl {
				step = opts.FineStep
			}
			steps := math.Round(dx / opts.PixelsPerStep)
			nv := clampRange(c.drag.startValue+steps*step, opts.Min, opts.Max)
			if nv != value {
				value = nv
				res.Changed = true
			}
		}
		if c.In.Released[MouseLeft] {
			if !c.drag.moved {
				c.beginEdit(id, formatNumber(value, opts.Decimals), true)
			} else {
				res.Committed = true
			}
			c.drag.active = NoID
		}
	}

	c.drawNumberBox(r, it, value, opts, false)
	tip := opts.Tooltip
	if opts.Warn && opts.WarnTooltip != "" {
		tip = opts.WarnTooltip
	}
	c.queueTooltip(id, r, it, tip, "", opts.DisabledWhy)
	res.Text = formatNumber(value, opts.Decimals)
	return value, res
}

type numberEditResult struct {
	value  float64
	result FieldResult
}

// numberEditor runs the typing half of a drag-number field.
func (c *Context) numberEditor(id ID, r rl.Rectangle, value float64, opts NumberOpts) numberEditResult {
	it := c.interact(id, r, opts.Disabled)
	c.describeControl(id, "number", c.edit.String())
	c.edit.blink += c.In.DeltaMillis

	out := numberEditResult{value: value, result: FieldResult{Editing: true}}

	commit := func(text string) {
		if v, err := parseNumber(text, opts.Unit); err == nil {
			nv := clampRange(v, opts.Min, opts.Max)
			out.value = nv
			out.result.Changed = nv != value
		}
		out.result.Committed = true
		out.result.Editing = false
	}

	if c.In.Pressed[MouseLeft] && !it.Hovered {
		commit(c.edit.String())
		c.ClearFocus()
	} else if kr := c.handleTextKeys(); kr.Committed {
		commit(kr.Text)
		c.ClearFocus()
	} else if kr.Cancelled {
		out.result.Cancelled = true
		out.result.Editing = false
		c.ClearFocus()
	}

	c.drawNumberBox(r, it, value, opts, out.result.Editing)
	if out.result.Editing {
		c.drawEditable(InsetXY(r, c.Px(Spacing/2), 0))
	}
	out.result.Text = formatNumber(out.value, opts.Decimals)
	return out
}

func (c *Context) drawNumberBox(r rl.Rectangle, it Interaction, value float64, opts NumberOpts, editing bool) {
	border := ColorStroke
	text := ColorText
	switch {
	case editing:
		border = ColorAccent
	case opts.Warn:
		border, text = ColorWarn, ColorWarn
	}
	c.FillRounded(r, CornerRadius, stateColor(ColorCard, it))
	c.StrokeRounded(r, CornerRadius, border)
	if editing {
		return
	}
	inner := InsetXY(r, c.Px(Spacing/2), 0)
	label := formatNumber(value, opts.Decimals)
	if opts.Unit != "" {
		label += " " + opts.Unit
	}
	c.Text(inner, label, FontSizeUI, textColorFor(text, it))
}

// formatNumber renders a value with the given precision, trimming trailing
// zeros so "6" never shows as "6.00".
func formatNumber(v float64, decimals int) string {
	if decimals <= 0 {
		// Values that are whole print without a point; others get two places.
		if v == math.Trunc(v) {
			return strconv.FormatFloat(v, 'f', 0, 64)
		}
		decimals = 2
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" {
		s = "0"
	}
	return s
}

// parseNumber accepts a typed value with or without its unit suffix.
func parseNumber(s, unit string) (float64, error) {
	t := strings.TrimSpace(s)
	if unit != "" {
		t = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(t, unit)), " ")
	}
	t = strings.TrimSpace(t)
	if t == "" {
		return 0, fmt.Errorf("empty")
	}
	return strconv.ParseFloat(t, 64)
}

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
