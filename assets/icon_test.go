package assets

import (
	"bytes"
	"image"
	_ "image/png"
	"testing"
)

// The window icon has to be a PNG raylib can actually decode at startup, and
// the same drawing as the executable's resource. A silent failure there is a
// program that runs with a blank mark.
func TestWindowIconDecodes(t *testing.T) {
	if len(WindowIcon) == 0 {
		t.Fatal("no window icon embedded")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(WindowIcon))
	if err != nil {
		t.Fatalf("the embedded icon does not decode: %v", err)
	}
	if format != "png" {
		t.Errorf("icon is %s; raylib is asked to load it as .png", format)
	}
	if cfg.Width != cfg.Height {
		t.Errorf("icon is %dx%d, want a square", cfg.Width, cfg.Height)
	}
	if cfg.Width < 32 {
		t.Errorf("icon is only %d px; too small for a taskbar", cfg.Width)
	}
}
