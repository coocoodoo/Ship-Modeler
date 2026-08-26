// Package app is the top of the stack: the mode state machine, input routing
// and the frame loop. Everything below it is driven from here.
package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
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

	WindowW, WindowH int
	DeltaMillis      float64
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
	rl.KeyZ, rl.KeyY, rl.KeyD, rl.KeyN,
}

// PollInput reads raylib's state into an InputFrame.
func PollInput(dtMillis float64) InputFrame {
	f := NewInputFrame()
	pos := rl.GetMousePosition()
	d := rl.GetMouseDelta()
	f.MouseX, f.MouseY = float64(pos.X), float64(pos.Y)
	f.MouseDX, f.MouseDY = float64(d.X), float64(d.Y)
	f.Wheel = float64(rl.GetMouseWheelMove())

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

	f.Shift = f.KeysDown[rl.KeyLeftShift] || f.KeysDown[rl.KeyRightShift]
	f.Ctrl = f.KeysDown[rl.KeyLeftControl] || f.KeysDown[rl.KeyRightControl]
	f.Alt = f.KeysDown[rl.KeyLeftAlt] || f.KeysDown[rl.KeyRightAlt]

	f.WindowW, f.WindowH = rl.GetRenderWidth(), rl.GetRenderHeight()
	f.DeltaMillis = dtMillis
	return f
}
