package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/ui"
)

// WindowTitle is the OS window caption.
const WindowTitle = "Modeler"

// Default window size (SPEC-UX §2).
const (
	DefaultWindowW = 1600
	DefaultWindowH = 900
)

// OpenWindow creates the application window. msaa and hidden control the two
// modes we need: the interactive app wants 4x MSAA, headless capture wants a
// hidden window and no MSAA so golden shots stay comparable (SPEC-RENDER §10).
func OpenWindow(w, h int, msaa, hidden bool) {
	flags := uint32(rl.FlagWindowResizable)
	if msaa {
		flags |= rl.FlagMsaa4xHint
	}
	if hidden {
		flags |= rl.FlagWindowHidden
	} else {
		flags |= rl.FlagVsyncHint
	}
	rl.SetConfigFlags(flags)
	rl.SetTraceLogLevel(rl.LogWarning)
	rl.InitWindow(int32(w), int32(h), WindowTitle)
	rl.SetWindowMinSize(ui.MinWindowW, ui.MinWindowH)
	rl.SetExitKey(0) // Esc is the app's back-one-level key, not quit
}

// Run drives the interactive frame loop until the window closes.
func Run() {
	a := New(false)
	defer a.Close()
	a.box.init()
	a.LoadTestScene()
	a.layout = a.Layout(rl.GetRenderWidth(), rl.GetRenderHeight())
	a.FrameSelection(a.layout.RenderViewport())

	for !rl.WindowShouldClose() {
		dt := float64(rl.GetFrameTime()) * 1000
		if dt <= 0 || dt > 250 {
			dt = 1000.0 / 60.0 // first frame, or after a long stall
		}
		in := PollInput(dt)

		rl.BeginDrawing()
		a.Frame(in)
		rl.EndDrawing()
	}
}
