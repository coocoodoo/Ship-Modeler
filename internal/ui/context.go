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
	return rl.CheckCollisionPointRec(c.MousePos(), r)
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
