package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/assets"
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
	setWindowIcon()
	rl.SetWindowMinSize(ui.MinWindowW, ui.MinWindowH)
	rl.SetExitKey(0) // Esc is the app's back-one-level key, not quit
}

// setWindowIcon puts the program's mark on the window and the taskbar.
//
// A failure here is cosmetic and silent: an icon that will not load is the
// last reason a modelling program should refuse to start.
func setWindowIcon() {
	img := rl.LoadImageFromMemory(".png", assets.WindowIcon, int32(len(assets.WindowIcon)))
	if img == nil {
		return
	}
	defer rl.UnloadImage(img)
	rl.SetWindowIcon(*img)
}

// Run drives the interactive frame loop until the window closes.
func Run() {
	RunWithAI(false)
}

func RunWithAI(enableAI bool) {
	a := New(false)
	defer a.Close()
	a.box.init()
	// The document starts empty, which is what makes the welcome card appear
	// (SPEC-UX §14). The test scene is for headless scripts only: shipping it
	// here meant every launch opened on three debug boxes and the welcome card
	// — New, Open, the sample ship, the recents — could never show at all.
	a.applySavedWindow()
	a.layout = a.Layout(rl.GetRenderWidth(), rl.GetRenderHeight())
	a.FrameSelection(a.layout.RenderViewport())
	if enableAI {
		if err := a.startAI(); err != nil {
			a.Toast(ui.Toast{Text: err.Error(), Kind: ui.ToastError})
		}
	}

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
	wasFocused := rl.IsWindowFocused()
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
		focused := rl.IsWindowFocused()
		in.FocusLost, wasFocused = wasFocused && !focused, focused

		rl.BeginDrawing()
		a.Frame(in)
		rl.EndDrawing()

		// Dialogs run out here. A native one is modal and pumps its own message
		// loop, and doing that between BeginDrawing and EndDrawing would mean
		// running somebody else's loop with a frame half submitted. The
		// autosave writes here for the same family of reason: zipping the
		// document mid-frame was a hitch the next stroke could feel.
		a.RunPendingFile()
		a.writeDueAutosave()
		a.stepClose()
		a.processAI()

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

// applySavedWindow puts the window back where the last session left it. The
// settings have remembered the rect since M8; nothing ever read it back, so
// the program forgot its size every time it was closed.
//
// The size is taken whenever it is usable. The position is taken only if that
// corner still lands on a monitor — a rect saved on an unplugged second screen
// must not put the title bar somewhere nothing can grab it.
func (a *App) applySavedWindow() {
	w := a.Settings.Window
	if w.Width >= ui.MinWindowW && w.Height >= ui.MinWindowH {
		rl.SetWindowSize(w.Width, w.Height)
	}
	for m := 0; m < rl.GetMonitorCount(); m++ {
		p := rl.GetMonitorPosition(m)
		mw, mh := float32(rl.GetMonitorWidth(m)), float32(rl.GetMonitorHeight(m))
		const grab = 64 // pixels of title bar that must stay reachable
		if float32(w.X) >= p.X-grab && float32(w.X) <= p.X+mw-grab &&
			float32(w.Y) >= p.Y && float32(w.Y) <= p.Y+mh-grab {
			rl.SetWindowPosition(w.X, w.Y)
			return
		}
	}
}
