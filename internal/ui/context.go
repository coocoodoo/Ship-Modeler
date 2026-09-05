package ui

import (
	"hash/fnv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The immediate-mode core (D-02). Widgets are called every frame; the Context
// carries the little state that must persist between frames — which widget is
// hovered, which is being dragged, where the caret is, how long a tooltip has
// been waiting — and nothing else.

// ID identifies a widget across frames. Callers pass a stable string; nested
// or repeated widgets scope it with a parent id.
type ID uint64

// NoID is the zero identity, meaning "no widget".
const NoID ID = 0

// MakeID hashes a string into a widget identity.
func MakeID(s string) ID {
	h := fnv.New64a()
	h.Write([]byte(s))
	id := ID(h.Sum64())
	if id == NoID {
		id = 1
	}
	return id
}

// Child derives a scoped identity, so a row's eye toggle in one row never
// collides with the next row's.
func (id ID) Child(s string) ID {
	h := fnv.New64a()
	var buf [8]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(id >> (8 * i))
	}
	h.Write(buf[:])
	h.Write([]byte(s))
	out := ID(h.Sum64())
	if out == NoID {
		out = 1
	}
	return out
}

// MouseButton indexes the button arrays.
const (
	MouseLeft = iota
	MouseRight
	MouseMiddle
)

// Input is one frame of user input, handed to the widget kit by the app. The
// kit never polls the platform itself, which is what lets scripted tests drive
// the whole UI (PLAN §4).
type Input struct {
	MouseX, MouseY   float64
	MouseDX, MouseDY float64
	Wheel            float64

	Down     [3]bool
	Pressed  [3]bool
	Released [3]bool

	Chars       []rune
	KeysPressed []int32
	KeysDown    map[int32]bool

	Shift, Ctrl, Alt bool

	DeltaMillis float64
}

// KeyPressed reports whether a key went down this frame.
func (in *Input) KeyPressed(k int32) bool {
	for _, p := range in.KeysPressed {
		if p == k {
			return true
		}
	}
	return false
}

// Context is the widget kit's per-frame state.
type Context struct {
	Fonts *Fonts
	Scale float64
	In    Input

	// hot is the widget under the cursor; active is the one being pressed or
	// dragged. Only one of each exists at a time, which is what makes an
	// immediate-mode kit behave under overlapping widgets.
	hot     ID
	active  ID
	nextHot ID

	// focus is the widget receiving typed characters.
	focus ID

	// edit holds the live text of whichever field has focus.
	edit editState

	// drag tracks a scrubbing drag-number field.
	drag dragState

	// tip is the hover-delay machinery of SPEC-UX §4.
	tip tooltipState

	// clicks remembers the last click so a row can recognise a double click.
	clicks clickTracker

	// popover is the open colour picker, drawn above everything, and
	// cards are the floating panels drawn last frame, so hit-testing can see
	// them. A card is not chrome — it floats inside the viewport — but the
	// pointer over one belongs to it and not to the model behind it, and the
	// app has to be able to decide that before the widgets run.
	//
	// Asking whether a widget is hovered is not enough: the gaps between a
	// card's controls are still the card, and a viewport tool that treated them
	// as open air would act through the panel.
	cards     []rl.Rectangle
	lastCards []rl.Rectangle

	// claims are rectangles that swallow the pointer for the rest of *this*
	// frame, so a panel drawn over another one is not merely on top of it but
	// actually in front of it.
	//
	// lastCards cannot do this job: it is a frame old, so a widget drawn under
	// a panel that appeared this frame would still answer the click. That was
	// the flyout landing a sketch point and selecting a tree row through
	// itself.
	claims []rl.Rectangle

	// wheelClaims are rectangles that took the mouse wheel last frame: a list
	// that scrolls under the pointer rather than the camera behind it.
	//
	// It is asked separately from the cards because the answers differ. A card
	// deliberately lets navigation through — orbiting from wherever the pointer
	// happens to be is how this program moves (SPEC-UX §1) — but a panel with a
	// scrolling list cannot, or one wheel notch both scrolls the list and zooms
	// the model behind it.
	wheelClaims     []rl.Rectangle
	lastWheelClaims []rl.Rectangle

	// popoverBox is where it landed last frame so hit-testing can see it.
	popover    popoverState
	popoverBox rl.Rectangle

	// overlays are drawn after every panel, so tooltips and popovers are never
	// clipped by the widget that opened them.
	overlays []func()

	// toasts are the transient messages at the bottom of the viewport.
	toasts []Toast

	// modal, when set, blocks input to everything beneath it.
	modal *ModalState

	// wantMouse records that a widget consumed the pointer this frame, so the
	// viewport knows not to also treat the click as a selection.
	wantMouse bool
	// wantKeyboard records that a text field is swallowing key presses.
	wantKeyboard bool

	// blocked is set while drawing beneath an open modal or popover.
	blocked bool
}

// NewContext creates a kit bound to a font set.
func NewContext(fonts *Fonts, scale float64) *Context {
	return &Context{Fonts: fonts, Scale: scale}
}

// Begin starts a frame.
func (c *Context) Begin(in Input) {
	c.In = in
	c.hot = c.nextHot
	c.nextHot = NoID
	c.wantMouse = false
	c.lastCards, c.cards = c.cards, c.cards[:0]
	c.lastWheelClaims, c.wheelClaims = c.wheelClaims, c.wheelClaims[:0]
	c.claims = c.claims[:0]
	c.wantKeyboard = c.focus != NoID
	c.overlays = c.overlays[:0]
	c.blocked = false

	c.clicks.since += in.DeltaMillis
	if c.clicks.since > doubleClickMillis {
		c.clicks = clickTracker{}
	}
	c.stepToasts(in.DeltaMillis)
	c.stepTooltip(in.DeltaMillis)
}

// End finishes a frame: it draws the deferred overlay layer and releases the
// active widget.
//
// Clearing the active widget belongs here, not in Begin. The frame that carries
// the button release is the frame a widget turns into a click, so dropping the
// active widget before the widgets run would mean no click ever fires.
func (c *Context) End() {
	for _, fn := range c.overlays {
		fn()
	}
	c.overlays = c.overlays[:0]
	c.drawTooltip()

	if !c.In.Down[MouseLeft] {
		c.active = NoID
		c.drag.active = NoID
	}
}

// Defer queues drawing for the overlay layer.
func (c *Context) Defer(fn func()) { c.overlays = append(c.overlays, fn) }

// WantMouse reports whether a widget consumed the pointer this frame.
func (c *Context) WantMouse() bool { return c.wantMouse }

// WantKeyboard reports whether a text field is capturing keys.
func (c *Context) WantKeyboard() bool { return c.wantKeyboard }

// ConsumeMouse marks the pointer as handled by the UI.
func (c *Context) ConsumeMouse() { c.wantMouse = true }

// Px scales a logical size to device pixels.
func (c *Context) Px(v float64) float32 { return float32(v * c.Scale) }

// MousePos is the cursor in device pixels.
func (c *Context) MousePos() rl.Vector2 {
	return rl.Vector2{X: float32(c.In.MouseX), Y: float32(c.In.MouseY)}
}

// hovering reports whether the cursor is inside a rectangle and not blocked by
// an overlay above it.
func (c *Context) hovering(r rl.Rectangle) bool {
	if c.blocked {
		return false
	}
	p := c.MousePos()
	if !rl.CheckCollisionPointRec(p, r) {
		return false
	}
	// Something drawn earlier this frame has already claimed this pixel.
	for _, claim := range c.claims {
		if rl.CheckCollisionPointRec(p, claim) {
			return false
		}
	}
	return true
}

// ClaimPointer marks a rectangle as owning the pointer for the rest of the
// frame. Widgets that run after it stop responding underneath it.
//
// Call it after the claiming panel has hit-tested its own contents, or it will
// block itself.
func (c *Context) ClaimPointer(r rl.Rectangle) {
	c.claims = append(c.claims, r)
	if rl.CheckCollisionPointRec(c.MousePos(), r) {
		c.wantMouse = true
	}
}

// Interaction is the outcome of one widget's input handling. Every interactive
// widget reports the same shape, which keeps hover, press and disabled states
// consistent across the kit (SPEC-UX §15's first checklist item).
type Interaction struct {
	Hovered  bool
	Pressed  bool // the button is currently held on this widget
	Clicked  bool // released over the widget after pressing on it
	Disabled bool
}

// interact runs the shared hover/press bookkeeping for a rectangle.
func (c *Context) interact(id ID, r rl.Rectangle, disabled bool) Interaction {
	if disabled {
		return Interaction{Disabled: true, Hovered: c.hovering(r)}
	}
	var out Interaction
	out.Hovered = c.hovering(r)
	if out.Hovered {
		c.nextHot = id
		c.wantMouse = true
		if c.In.Pressed[MouseLeft] {
			c.active = id
		}
	}
	if c.active == id {
		out.Pressed = true
		c.wantMouse = true
		if c.In.Released[MouseLeft] {
			if out.Hovered {
				out.Clicked = true
			}
			c.active = NoID
		}
	}
	return out
}

// stateColor picks the fill for a widget in its current interaction state,
// which is how every control gets distinct hover, pressed and disabled looks
// without each one reinventing them.
func stateColor(base rl.Color, it Interaction) rl.Color {
	switch {
	case it.Disabled:
		return Fade(base, 0.45)
	case it.Pressed:
		return Shade(base, 0.82)
	case it.Hovered:
		return Shade(base, 1.18)
	default:
		return base
	}
}

// textColorFor dims label text on a disabled control.
func textColorFor(base rl.Color, it Interaction) rl.Color {
	if it.Disabled {
		return Fade(base, 0.4)
	}
	return base
}

// Focus returns the widget currently capturing keystrokes.
func (c *Context) Focus() ID { return c.focus }

// SetFocus moves keyboard focus, seeding the editor with the given text.
func (c *Context) SetFocus(id ID, text string) {
	c.focus = id
	c.edit = editState{text: []rune(text), caret: len([]rune(text)), sel: -1}
}

// ClearFocus drops keyboard focus.
func (c *Context) ClearFocus() {
	c.focus = NoID
	c.edit = editState{}
}

// Dragging reports whether a widget is currently capturing a drag, which is
// what keeps the viewport out of the way while a slider or picker is scrubbed.
func (c *Context) Dragging() bool { return c.active != NoID && c.In.Down[MouseLeft] }

// OverlayCapturesPointer reports whether an open popover or modal sits under a
// window pixel. The app asks this geometrically, before the widgets run, so it
// can decide who owns the pointer without running the kit twice.
func (c *Context) OverlayCapturesPointer(x, y float64) bool {
	if c.modal != nil {
		return true
	}
	if c.popover.id == NoID {
		return false
	}
	p := rl.Vector2{X: float32(x), Y: float32(y)}
	return rl.CheckCollisionPointRec(p, c.popoverBox) ||
		rl.CheckCollisionPointRec(p, c.popover.anchor)
}

// CardCapturesPointer reports whether a floating card drawn last frame sits
// under a window pixel.
//
// A card is not chrome — it floats inside the viewport — but the pointer over
// one belongs to it. It is asked separately from OverlayCapturesPointer because
// the answer is used differently: a card takes the left button and leaves
// navigation alone, while a modal takes everything.
func (c *Context) CardCapturesPointer(x, y float64) bool {
	p := rl.Vector2{X: float32(x), Y: float32(y)}
	for _, card := range c.lastCards {
		if rl.CheckCollisionPointRec(p, card) {
			return true
		}
	}
	return false
}

// registerCard records a floating panel's rectangle for the next frame's
// hit-testing.
func (c *Context) registerCard(r rl.Rectangle) { c.cards = append(c.cards, r) }

// ClaimWheel marks a rectangle as owning the mouse wheel, for a panel that
// scrolls. Call it every frame the panel is up; it is read the frame after,
// the same way cards are.
func (c *Context) ClaimWheel(r rl.Rectangle) { c.wheelClaims = append(c.wheelClaims, r) }

// WheelCaptured reports whether something drawn last frame took the wheel at
// a window pixel, so the camera can leave it alone.
func (c *Context) WheelCaptured(x, y float64) bool {
	p := rl.Vector2{X: float32(x), Y: float32(y)}
	for _, r := range c.lastWheelClaims {
		if rl.CheckCollisionPointRec(p, r) {
			return true
		}
	}
	return false
}
