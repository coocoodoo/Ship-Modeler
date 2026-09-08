package main

import (
	"path/filepath"
	"testing"
)

func TestProjectStartupModes(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		args                                      []string
		view, edit, headless, wantViewer, wantErr bool
	}{
		{name: "normal editor"},
		{name: "double click", args: []string{"models/my ship.pxm"}, wantViewer: true},
		{name: "explicit viewer", args: []string{"ship.pxm"}, view: true, wantViewer: true},
		{name: "explicit editor", args: []string{"ship.pxm"}, edit: true},
		{name: "missing path", view: true, wantErr: true},
		{name: "conflicting modes", args: []string{"ship.pxm"}, view: true, edit: true, wantErr: true},
		{name: "multiple files", args: []string{"a.pxm", "b.pxm"}, wantErr: true},
		{name: "headless conflict", args: []string{"a.pxm"}, headless: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := startupOptions(tc.args, tc.view, tc.edit, tc.headless)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil {
				return
			}
			if got.Viewer != tc.wantViewer {
				t.Fatalf("viewer=%v", got.Viewer)
			}
			if len(tc.args) == 1 {
				want, _ := filepath.Abs(tc.args[0])
				if got.Path != want {
					t.Fatalf("path=%q, want %q", got.Path, want)
				}
			}
		})
	}
}
