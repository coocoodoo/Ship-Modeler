// Package app is the top of the stack: the mode state machine, input routing
// and the frame loop. Everything below it is driven from here.
package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/ui"
)

// InputFrame is one frame of user input. The app consumes only this struct, so
// the same code path runs from real raylib polling and from scripted feeders in
// tests (PLAN §4).
type InputFrame struct {
	MouseX, MouseY   float64
	MouseDX, MouseDY float64
	Wheel            float64

	Down     [3]bool // left, right, middle
	Pressed  [3]bool
	Released [3]bool

	KeysDown    map[int32]bool
	KeysPressed []int32
	Chars       []rune

	Shift, Ctrl, Alt bool

	// Dropped are files dragged onto the window this frame. It is how a .hex
	// palette gets in before the file dialogs of M8 (SPEC-UX §13.3).
	Dropped []string

	WindowW, WindowH int
	DeltaMillis      float64
}

// ToUI converts a frame into the widget kit's input form. The kit deliberately
// takes its own struct so it stays independent of how the app polls.
func (f *InputFrame) ToUI() ui.Input {
	return ui.Input{
		MouseX: f.MouseX, MouseY: f.MouseY,
		MouseDX: f.MouseDX, MouseDY: f.MouseDY,
		Wheel:       f.Wheel,
		Down:        f.Down,
		Pressed:     f.Pressed,
		Released:    f.Released,
		Chars:       f.Chars,
		KeysPressed: f.KeysPressed,
		KeysDown:    f.KeysDown,
		Shift:       f.Shift, Ctrl: f.Ctrl, Alt: f.Alt,
		DeltaMillis: f.DeltaMillis,
	}
}

// Mouse buttons, indexed into the InputFrame arrays.
const (
	MouseLeft = iota
	MouseRight
	MouseMiddle
)

// NewInputFrame builds an empty frame with its maps ready.
func NewInputFrame() InputFrame {
	return InputFrame{KeysDown: map[int32]bool{}}
}

// KeyDown reports whether a key is held.
func (f *InputFrame) KeyDown(k int32) bool { return f.KeysDown[k] }

// KeyPressed reports whether a key went down this frame.
func (f *InputFrame) KeyPressed(k int32) bool {
	for _, p := range f.KeysPressed {
		if p == k {
			return true
		}
	}
	return false
}

// watchedKeys are polled every frame for held state. Modifier handling checks
// both the left and right variants, per SPEC-UX §16.
var watchedKeys = []int32{
	rl.KeyS, rl.KeyE, rl.KeyB, rl.KeyM, rl.KeyP,
	rl.KeyV, rl.KeyL, rl.KeyR, rl.KeyC,
	rl.KeyO, rl.KeyF, rl.KeyH,
	rl.KeyEscape, rl.KeyEnter, rl.KeyDelete, rl.KeyTab,
	rl.KeyLeftShift, rl.KeyRightShift,
	rl.KeyLeftControl, rl.KeyRightControl,
	rl.KeyLeftAlt, rl.KeyRightAlt,
	rl.KeyZ, rl.KeyY, rl.KeyD, rl.KeyN, rl.KeyA, rl.KeyG, rl.KeyI,
	rl.KeyBackspace, rl.KeyLeft, rl.KeyRight, rl.KeyHome, rl.KeyEnd,
	rl.KeySlash,
}

// PollInput reads raylib's state into an InputFrame.
func PollInput(dtMillis float64) InputFrame {
	f := NewInputFrame()
	pos := rl.GetMousePosition()
	d := rl.GetMouseDelta()
	f.MouseX, f.MouseY = float64(pos.X), float64(pos.Y)
	f.MouseDX, f.MouseDY = float64(d.X), float64(d.Y)
	f.Wheel = verticalWheelNotches(rl.GetMouseWheelMoveV())

	buttons := [3]rl.MouseButton{rl.MouseLeftButton, rl.MouseRightButton, rl.MouseMiddleButton}
	for i, b := range buttons {
		f.Down[i] = rl.IsMouseButtonDown(b)
		f.Pressed[i] = rl.IsMouseButtonPressed(b)
		f.Released[i] = rl.IsMouseButtonReleased(b)
	}

	for _, k := range watchedKeys {
		if rl.IsKeyDown(k) {
			f.KeysDown[k] = true
		}
	}
	for {
		k := rl.GetKeyPressed()
		if k == 0 {
			break
		}
		f.KeysPressed = append(f.KeysPressed, k)
	}
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		f.Chars = append(f.Chars, rune(c))
	}

	if rl.IsFileDropped() {
		f.Dropped = rl.LoadDroppedFiles()
		rl.UnloadDroppedFiles()
	}

	f.Shift = f.KeysDown[rl.KeyLeftShift] || f.KeysDown[rl.KeyRightShift]
	f.Ctrl = f.KeysDown[rl.KeyLeftControl] || f.KeysDown[rl.KeyRightControl]
	f.Alt = f.KeysDown[rl.KeyLeftAlt] || f.KeysDown[rl.KeyRightAlt]

	f.WindowW, f.WindowH = rl.GetRenderWidth(), rl.GetRenderHeight()
	f.DeltaMillis = dtMillis
	return f
}

// The scalar raylib wheel helper selects the larger of X and Y. A tilt
// wheel/trackpad can therefore reverse an upward scroll when X is negative.
// Navigation and paste rotation both follow vertical movement exclusively.
func verticalWheelNotches(move rl.Vector2) float64 { return float64(move.Y) }
