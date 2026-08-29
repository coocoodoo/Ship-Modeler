package ui

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The overlay widgets: tooltip, toast, floating card, hint bar, modal and the
// shortcut sheet (SPEC-UX §4). All of them draw above the panels, so they are
// queued to the deferred layer rather than painted inline.

// TooltipDelayMillis is how long the pointer must rest before a tip appears.
const TooltipDelayMillis = 600

type tooltipState struct {
	id       ID
	waited   float64
	text     string
	shortcut string
	anchor   rl.Rectangle
	// seen marks that the hovered widget re-registered this frame; a tip whose
	// widget vanished is dropped rather than left hanging.
	seen bool
}

// queueTooltip records that a widget wants a tip while hovered. A disabled
// control shows its reason instead, because every disabled thing must say how
// to enable it (SPEC-UX §15).
func (c *Context) queueTooltip(id ID, r rl.Rectangle, it Interaction, text, shortcut, disabledWhy string) {
	if !it.Hovered {
		return
	}
	if it.Disabled {
		if disabledWhy == "" {
			return
		}
		text, shortcut = disabledWhy, ""
	}
	if text == "" {
		return
	}
	if c.tip.id != id {
		c.tip = tooltipState{id: id}
	}
	c.tip.text, c.tip.shortcut, c.tip.anchor, c.tip.seen = text, shortcut, r, true
}

func (c *Context) stepTooltip(dtMillis float64) {
	if !c.tip.seen {
		c.tip = tooltipState{}
		return
	}
	c.tip.waited += dtMillis
	c.tip.seen = false
}

func (c *Context) drawTooltip() {
	if c.tip.id == NoID || c.tip.waited < TooltipDelayMillis || c.tip.text == "" {
		return
	}
	pad := c.Px(Spacing / 2)
	label := c.tip.text
	w := c.TextWidth(label, FontSizeSmall)
	var shortcutW float32
	if c.tip.shortcut != "" {
		shortcutW = c.TextWidth(c.tip.shortcut, FontSizeSmall) + c.Px(10)
	}
	h := c.Fonts.LineHeight(FontSizeSmall) + pad*2
	box := Rect(c.tip.anchor.X, c.tip.anchor.Y+c.tip.anchor.Height+c.Px(4), w+shortcutW+pad*2, h)

	// Keep the tip on screen.
	if maxX := float32(rl.GetRenderWidth()) - c.Px(Spacing); box.X+box.Width > maxX {
		box.X = maxX - box.Width
	}
	if maxY := float32(rl.GetRenderHeight()) - c.Px(Spacing); box.Y+box.Height > maxY {
		box.Y = c.tip.anchor.Y - box.Height - c.Px(4)
	}

	c.Shadow(box, CornerRadius, 1)
	c.FillRounded(box, CornerRadius, ColorCard)
	c.StrokeRounded(box, CornerRadius, ColorStroke)
	textBox := InsetXY(box, pad, 0)
	c.Text(textBox, label, FontSizeSmall, ColorText)
	if c.tip.shortcut != "" {
		sb, _ := SplitRight(textBox, shortcutW)
		c.Text(sb, c.tip.shortcut, FontSizeSmall, ColorTextDim)
	}
}

// Toast is a transient message at the bottom of the viewport (SPEC-UX §4):
// three seconds, at most three stacked, optionally carrying one action.
type Toast struct {
	Text string
	// Action labels the single button, e.g. "Undo". Empty means no button.
	Action string
	// Kind tints the accent stripe.
	Kind ToastKind
	// OnAction runs when the button is pressed.
	OnAction func()

	remaining float64
	id        ID
}

// ToastKind selects a toast's accent colour.
type ToastKind uint8

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastWarn
	ToastError
)

// Toast timing and stacking limits.
const (
	ToastMillis   = 3000
	MaxToasts     = 3
	toastWidth    = 340
	toastHeight   = 34
	toastFadeMs   = 250
	toastStackGap = 6
)

// ShowToast queues a message. Older toasts beyond the cap are dropped so the
// stack never grows past three.
func (c *Context) ShowToast(t Toast) {
	t.remaining = ToastMillis
	t.id = MakeID("toast" + itoa(len(c.toasts)) + t.Text)
	c.toasts = append(c.toasts, t)
	if len(c.toasts) > MaxToasts {
		c.toasts = c.toasts[len(c.toasts)-MaxToasts:]
	}
}

// Toasts returns the live messages, for tests and diagnostics.
func (c *Context) Toasts() []Toast { return c.toasts }

// ClearToasts removes every message immediately.
func (c *Context) ClearToasts() { c.toasts = c.toasts[:0] }

func (c *Context) stepToasts(dtMillis float64) {
	kept := c.toasts[:0]
	for _, t := range c.toasts {
		t.remaining -= dtMillis
		if t.remaining > 0 {
			kept = append(kept, t)
		}
	}
	c.toasts = kept
}

// DrawToasts paints the stack bottom-centre inside the given area.
func (c *Context) DrawToasts(area rl.Rectangle) {
	if len(c.toasts) == 0 {
		return
	}
	w := c.Px(toastWidth)
	h := c.Px(toastHeight)
	gap := c.Px(toastStackGap)
	baseY := area.Y + area.Height - h - c.Px(Spacing*2)

	boxAt := func(i int) rl.Rectangle {
		idx := len(c.toasts) - 1 - i
		return Rect(area.X+(area.Width-w)/2, baseY-float32(idx)*(h+gap), w, h)
	}
	alphaOf := func(t *Toast) float64 {
		if t.remaining < toastFadeMs {
			return t.remaining / toastFadeMs
		}
		return 1
	}

	// Every shadow before any body: a stack sits closer together than a
	// shadow reaches, and a shadow painted over the neighbouring toast reads
	// as a grey halo around it rather than as depth under it.
	for i := range c.toasts {
		c.Shadow(boxAt(i), CornerRadius, alphaOf(&c.toasts[i]))
	}

	for i := len(c.toasts) - 1; i >= 0; i-- {
		t := &c.toasts[i]
		box := boxAt(i)
		alpha := alphaOf(t)

		c.FillRounded(box, CornerRadius, Fade(ColorCard, alpha))
		c.StrokeRounded(box, CornerRadius, Fade(ColorStroke, alpha))
		stripe := Rect(box.X, box.Y+c.Px(4), c.Px(3), box.Height-c.Px(8))
		FillRect(stripe, Fade(toastColor(t.Kind), alpha))

		inner := InsetXY(box, c.Px(Spacing), 0)
		if t.Action != "" {
			btnW := c.TextWidth(t.Action, FontSizeSmall) + c.Px(Spacing*2)
			btn, rest := SplitRight(inner, btnW)
			if c.Button(t.id.Child("action"), InsetXY(btn, 0, c.Px(6)), t.Action,
				ButtonOpts{Style: ButtonGhost}) {
				if t.OnAction != nil {
					t.OnAction()
				}
				t.remaining = 0
			}
			inner = rest
		}
		c.Text(inner, t.Text, FontSizeSmall, Fade(ColorText, alpha))
	}
}

func toastColor(k ToastKind) color.RGBA {
	switch k {
	case ToastSuccess:
		return ColorSuccess
	case ToastWarn:
		return ColorWarn
	case ToastError:
		return ColorError
	default:
		return ColorAccent
	}
}

// CardResult reports a floating card's footer buttons.
type CardResult struct {
	Confirmed bool
	Cancelled bool
	// Body is the rectangle the caller fills with the card's contents.
	Body rl.Rectangle
}

// FloatingCardOpts configures a card.
type FloatingCardOpts struct {
	// Footer adds the confirm and cancel buttons.
	Footer bool
	// ConfirmLabel and CancelLabel default to a tick and a cross.
	ConfirmLabel string
	CancelLabel  string
	// ConfirmDisabled greys out the confirm button.
	ConfirmDisabled bool
	ConfirmWhy      string
}

// FloatingCard draws the contextual panel that hosts a tool's options
// (SPEC-UX §2). It returns the body rectangle for the caller to fill.
func (c *Context) FloatingCard(id ID, r rl.Rectangle, title string, opts FloatingCardOpts) CardResult {
	// The pointer over a card belongs to the card, gaps included, and the
	// viewport has to be able to find that out before the widgets run.
	c.registerCard(r)
	c.Card(r)
	var out CardResult

	pad := c.Px(Spacing + 2)
	inner := Inset(r, pad)

	titleBox, rest := SplitTop(inner, c.Fonts.LineHeight(FontSizeHeader))
	c.Text(titleBox, title, FontSizeHeader, ColorText)
	c.HairlineH(r.X, titleBox.Y+titleBox.Height+c.Px(4), r.Width, ColorStroke)
	rest.Y += c.Px(8)
	rest.Height -= c.Px(8)

	if opts.Footer {
		footer, body := SplitBottom(rest, c.Px(28))
		out.Body = body

		confirm := opts.ConfirmLabel
		cancel := opts.CancelLabel
		btnW := c.Px(76)
		cb, remaining := SplitRight(footer, btnW)
		if confirm == "" {
			if c.iconFooterButton(id.Child("ok"), cb, DrawCheckIcon, ColorAccent, opts.ConfirmDisabled, opts.ConfirmWhy) {
				out.Confirmed = true
			}
		} else if c.Button(id.Child("ok"), cb, confirm, ButtonOpts{
			Style: ButtonPrimary, Disabled: opts.ConfirmDisabled, DisabledWhy: opts.ConfirmWhy,
		}) {
			out.Confirmed = true
		}

		xb, _ := SplitRight(remaining, btnW+c.Px(6))
		xb.Width -= c.Px(6)
		if cancel == "" {
			if c.iconFooterButton(id.Child("cancel"), xb, DrawCrossIcon, ColorTextDim, false, "") {
				out.Cancelled = true
			}
		} else if c.Button(id.Child("cancel"), xb, cancel, ButtonOpts{Style: ButtonGhost}) {
			out.Cancelled = true
		}
	} else {
		out.Body = rest
	}
	return out
}

// iconFooterButton is the tick/cross pair a card's footer uses by default.
func (c *Context) iconFooterButton(id ID, r rl.Rectangle, icon IconFunc, tint color.RGBA, disabled bool, why string) bool {
	it := c.interact(id, r, disabled)
	fill := ColorCard
	if it.Hovered && !disabled {
		fill = ColorHover
	}
	c.FillRounded(r, CornerRadius, fill)
	c.StrokeRounded(r, CornerRadius, ColorStroke)
	ctr := Center(r)
	icon(float64(ctr.X), float64(ctr.Y), IconSize*c.Scale, textColorFor(tint, it))
	c.queueTooltip(id, r, it, "", "", why)
	return it.Clicked
}

// HintBar draws the always-present strip that says what to do next
// (SPEC-UX §2). It is never empty: silence is never the answer.
func (c *Context) HintBar(r rl.Rectangle, text, version string) {
	c.Panel(r)
	c.HairlineH(r.X, r.Y, r.Width, ColorStroke)
	inner := InsetXY(r, c.Px(Spacing), 0)

	if version != "" {
		vb, rest := SplitRight(inner, c.TextWidth(version, FontSizeSmall)+c.Px(Spacing))
		c.Text(vb, version, FontSizeSmall, Fade(ColorTextDim, 0.7))
		inner = rest
	}
	c.Text(inner, text, FontSizeSmall, ColorTextDim)
}

// ModalState is an open confirmation dialog. Modals are rare by design: only
// crash recovery and export overwrite use them (SPEC-UX §4).
type ModalState struct {
	Title       string
	Body        string
	ConfirmText string
	// AltText is an optional third answer, drawn between confirm and cancel.
	//
	// It exists for the one question that genuinely has three answers: save,
	// don't save, don't go. Folding those into two buttons makes one of them a
	// lie — either "cancel" means "discard", or the user has to leave the
	// dialog, save by hand, and come back (V-143).
	AltText    string
	CancelText string
	Danger     bool
}

// ShowModal opens a confirmation dialog.
func (c *Context) ShowModal(m ModalState) { c.modal = &m }

// ModalOpen reports whether a dialog is showing.
func (c *Context) ModalOpen() bool { return c.modal != nil }

// CloseModal dismisses the dialog.
func (c *Context) CloseModal() { c.modal = nil }

// ModalResult reports the dialog's outcome.
//
// Dismissed is Escape, and it is its own answer. The cancel button carries
// words — "Close without saving" — and words can be read; Escape is a reflex,
// and a reflex must never be the one that throws work away. Dismissing a
// question leaves things exactly as they were.
type ModalResult struct {
	Confirmed bool
	// Alt is the third button, when the dialog offers one.
	Alt       bool
	Cancelled bool
	Dismissed bool
}

// ModalContents reports the open dialog's text, for tests and for callers that
// need to know what was asked.
func (c *Context) ModalContents() (ModalState, bool) {
	if c.modal == nil {
		return ModalState{}, false
	}
	return *c.modal, true
}

// DrawModal paints the open dialog over the whole window and blocks everything
// beneath it.
func (c *Context) DrawModal(screen rl.Rectangle) ModalResult {
	if c.modal == nil {
		return ModalResult{}
	}
	m := c.modal
	var out ModalResult

	c.Overlay(screen)
	w := c.Px(420)
	h := c.Px(160)
	box := Rect(screen.X+(screen.Width-w)/2, screen.Y+(screen.Height-h)/2, w, h)
	c.Card(box)

	inner := Inset(box, c.Px(Spacing*2))
	titleBox, rest := SplitTop(inner, c.Fonts.LineHeight(FontSizeHeader))
	c.Text(titleBox, m.Title, FontSizeHeader, ColorText)
	bodyBox, _ := SplitTop(rest, c.Fonts.LineHeight(FontSizeUI)*2)
	bodyBox.Y += c.Px(6)
	c.Text(bodyBox, m.Body, FontSizeUI, ColorTextDim)

	footer, _ := SplitBottom(inner, c.Px(30))
	btnW := c.Px(110)
	confirmBox, remaining := SplitRight(footer, btnW)
	style := ButtonPrimary
	if m.Danger {
		style = ButtonDanger
	}
	if c.Button(MakeID("modal.confirm"), confirmBox, orDefault(m.ConfirmText, "OK"), ButtonOpts{Style: style}) {
		out.Confirmed = true
	}
	cancelBox, remaining := SplitRight(remaining, btnW+c.Px(8))
	cancelBox.Width -= c.Px(8)
	if c.Button(MakeID("modal.cancel"), cancelBox, orDefault(m.CancelText, "Cancel"), ButtonOpts{}) {
		out.Cancelled = true
	}
	if m.AltText != "" {
		altBox, _ := SplitRight(remaining, btnW+c.Px(8))
		altBox.Width -= c.Px(8)
		if c.Button(MakeID("modal.alt"), altBox, m.AltText, ButtonOpts{Style: ButtonDanger}) {
			out.Alt = true
		}
	}

	if c.In.KeyPressed(rl.KeyEscape) {
		out.Dismissed = true
	}
	if c.In.KeyPressed(rl.KeyEnter) {
		out.Confirmed = true
	}
	if out.Confirmed || out.Alt || out.Cancelled || out.Dismissed {
		c.CloseModal()
	}
	// Everything under the dialog is inert while it is up.
	c.wantMouse = true
	c.blocked = true
	return out
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Shortcut is one row of the shortcut sheet.
type Shortcut struct {
	Keys        string
	Description string
	// Section starts a new group when non-empty.
	Section string
}

// DrawShortcutOverlay paints the full keyboard map that `?` opens
// (SPEC-UX §16).
func (c *Context) DrawShortcutOverlay(screen rl.Rectangle, shortcuts []Shortcut) {
	c.Overlay(screen)

	w := c.Px(560)
	h := screen.Height * 0.82
	box := Rect(screen.X+(screen.Width-w)/2, screen.Y+(screen.Height-h)/2, w, h)
	c.Card(box)

	inner := Inset(box, c.Px(Spacing*2))
	titleBox, rest := SplitTop(inner, c.Fonts.LineHeight(FontSizeHeader)+c.Px(6))
	c.Text(titleBox, "Keyboard & mouse", FontSizeHeader, ColorText)
	hint, body := SplitBottom(rest, c.Fonts.LineHeight(FontSizeSmall)+c.Px(4))
	c.Text(hint, "Press ? or Esc to close", FontSizeSmall, ColorTextDim)

	// Two columns so the whole map fits without scrolling.
	colW := body.Width / 2
	rowH := c.Fonts.LineHeight(FontSizeUI) + c.Px(3)
	perCol := int(body.Height / rowH)
	if perCol < 1 {
		perCol = 1
	}

	for i, s := range shortcuts {
		col := i / perCol
		row := i % perCol
		if col > 1 {
			break
		}
		r := Rect(body.X+float32(col)*colW, body.Y+float32(row)*rowH, colW-c.Px(Spacing), rowH)
		if s.Section != "" {
			c.Text(r, s.Section, FontSizeSmall, Fade(ColorAccent, 0.9))
			continue
		}
		keyW := c.Px(120)
		keyBox, descBox := SplitLeft(r, keyW)
		c.Text(keyBox, s.Keys, FontSizeUI, ColorText)
		c.Text(descBox, s.Description, FontSizeUI, ColorTextDim)
	}
	c.wantMouse = true
}
