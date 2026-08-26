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
	flag.Parse()

	if err := run(*headless || *script != "", *script, *out, *size, *bench); err != nil {
		fmt.Fprintln(os.Stderr, "modeler:", err)
		os.Exit(1)
	}
}

func run(headless bool, script, out, size string, bench int) (err error) {
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

	app.OpenWindow(app.DefaultWindowW, app.DefaultWindowH, true, false)
	defer rl.CloseWindow()
	app.Run()
	return nil
}
