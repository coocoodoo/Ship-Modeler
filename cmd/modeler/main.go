// Command modeler is the Modeler application: a pixel-art spaceship modeller.
//
// Run with no flags for the interactive app. The headless flags render an op
// script to PNGs without showing a window, which is how the build verifies
// itself (SPEC-RENDER §9).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/app"
)

func init() {
	// GL and GLFW are only legal from the thread that created the context.
	runtime.LockOSThread()
}

func main() {
	headless := flag.Bool("headless", false, "run an op script with a hidden window and exit")
	script := flag.String("script", "", "op script to run (implies -headless)")
	out := flag.String("out", "shots", "directory to write shot PNGs into")
	size := flag.String("size", "1280x720", "render size for headless shots, WxH")
	bench := flag.Int("bench", 0, "after the script, render N frames and report frame cost")
	ai := flag.Bool("ai", false, "enable the local AI connection for this session")
	hidden := flag.Bool("hidden", false, "hide the live window (requires -ai; for connection tests)")
	view := flag.Bool("view", false, "open the project in viewer mode")
	edit := flag.Bool("edit", false, "open the project directly in the editor")
	flag.Parse()

	startup, err := startupOptions(flag.Args(), *view, *edit, *headless || *script != "")
	if err == nil {
		err = run(*headless || *script != "", *script, *out, *size, *bench, *ai, *hidden, startup)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "modeler:", err)
		os.Exit(1)
	}
}

func startupOptions(args []string, view, edit, headless bool) (app.StartupOptions, error) {
	var options app.StartupOptions
	if view && edit {
		return options, fmt.Errorf("choose either -view or -edit")
	}
	if len(args) > 1 {
		return options, fmt.Errorf("open one project at a time")
	}
	if (view || edit) && len(args) == 0 {
		return options, fmt.Errorf("-view and -edit need a project filename")
	}
	if headless && (len(args) != 0 || view || edit) {
		return options, fmt.Errorf("project filenames cannot be combined with a headless script")
	}
	if len(args) == 1 {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return options, err
		}
		options.Path, options.Viewer = path, !edit
	}
	return options, nil
}

func run(headless bool, script, out, size string, bench int, ai, hidden bool, startup app.StartupOptions) (err error) {
	// A crash must never take the user's work with it silently: recover, report
	// and exit non-zero. M8 adds the crash-save and the log file (SPEC-DATA §6).
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("crashed: %v\n%s", p, debug.Stack())
		}
	}()

	if headless {
		if script == "" {
			return fmt.Errorf("-headless needs -script <file>")
		}
		sz, err := app.ParseSize(size)
		if err != nil {
			return err
		}
		return app.RunHeadless(script, out, sz, bench)
	}

	if hidden && !ai {
		return fmt.Errorf("-hidden requires -ai")
	}
	app.OpenWindow(app.DefaultWindowW, app.DefaultWindowH, true, hidden)
	defer rl.CloseWindow()
	return app.RunStartup(ai, startup)
}
