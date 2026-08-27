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

	// A crash must not take the user's work with it (SPEC-DATA §6). The
	// recovery copy is written first, before anything else is attempted,
	// because everything else can fail too.
	defer func() {
		if p := recover(); p != nil {
			path := a.CrashSave()
			WriteCrashLog(p, path)
			panic(p)
		}
	}()

	// The window's own close is intercepted: unsaved work gets asked about
	// first (SPEC-UX §15). raylib's flag is cleared by reading it, so the loop
	// keeps running until the prompt has an answer.
	rl.SetExitKey(0)
	title := ""
	for {
		if rl.WindowShouldClose() {
			a.RequestClose()
		}
		if a.ShouldClose() {
			break
		}
		dt := float64(rl.GetFrameTime()) * 1000
		if dt <= 0 || dt > 250 {
			dt = 1000.0 / 60.0 // first frame, or after a long stall
		}
		in := PollInput(dt)

		rl.BeginDrawing()
		a.Frame(in)
		rl.EndDrawing()

		// Dialogs run out here. A native one is modal and pumps its own message
		// loop, and doing that between BeginDrawing and EndDrawing would mean
		// running somebody else's loop with a frame half submitted.
		a.RunPendingFile()
		a.stepClose()

		if want := a.WindowTitle(); want != title {
			rl.SetWindowTitle(want)
			title = want
		}
	}
	// Leaving on purpose is not a crash: there is nothing to recover.
	a.clearAutosave()
}

// WindowTitle is what the OS window is called: the document, then the program.
func (a *App) WindowTitle() string {
	return a.DocumentTitle() + " — " + WindowTitle
}
