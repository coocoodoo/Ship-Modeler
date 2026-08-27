package io

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Autosave, crash-save and recovery (SPEC-DATA §6).
//
// The promise is narrow and absolute: work that existed when the process died
// is still there when it starts again. Everything here serves that and nothing
// else, which is why an autosave is a whole .ship rather than a journal — the
// recovery path is then the ordinary load path, already tested, rather than a
// second reader that only runs on the worst day somebody has.

// AutosaveDirName is the folder under %APPDATA%\Modeler holding them.
const AutosaveDirName = "autosave"

// AutosaveDir is where autosaves and crash saves are written.
func AutosaveDir() (string, error) {
	dir, err := SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AutosaveDirName), nil
}

// Autosave describes one recoverable file found on disk.
type Autosave struct {
	Path string
	// Origin is the document it was made from, or empty for work never saved.
	Origin string
	// SavedAt is when the autosave was written.
	SavedAt time.Time
	// Crash marks a save written by the panic handler rather than the timer.
	Crash bool
}

// Describe is what the recovery prompt says this file is.
func (a Autosave) Describe() string {
	what := "Unsaved work"
	if a.Origin != "" {
		what = filepath.Base(a.Origin)
	}
	when := a.SavedAt.Local().Format("15:04 on 2 Jan")
	if a.Crash {
		return fmt.Sprintf("%s — recovered from a crash at %s", what, when)
	}
	return fmt.Sprintf("%s — autosaved at %s", what, when)
}

// autosaveName is the file an autosave for a given document goes to.
//
// The process id is in the name so two copies of the program running at once
// cannot overwrite each other's recovery file, which is exactly the moment you
// would need both.
func autosaveName(origin string, pid int, crash bool) string {
	stem := "untitled"
	if origin != "" {
		stem = strings.TrimSuffix(filepath.Base(origin), filepath.Ext(origin))
	}
	kind := "autosave"
	if crash {
		kind = "crash"
	}
	return fmt.Sprintf("%s-%d-%s%s", sanitizeStem(stem), pid, kind, ShipExtension)
}

// sanitizeStem keeps a document's name usable as part of a filename.
func sanitizeStem(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "untitled"
	}
	out := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// AutosavePath is where the running process writes its recovery file.
func AutosavePath(origin string, crash bool) (string, error) {
	dir, err := AutosaveDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, autosaveName(origin, os.Getpid(), crash)), nil
}

// sidecar is the small record written beside an autosave so the recovery prompt
// can say what it is without opening the whole thing.
type sidecar struct {
	Origin  string    `json:"origin"`
	SavedAt time.Time `json:"savedAt"`
	Crash   bool      `json:"crash"`
	PID     int       `json:"pid"`
}

// WriteSidecar records what an autosave was made from.
func WriteSidecar(shipPath, origin string, crash bool) error {
	return writeJSONAtomic(sidecarPath(shipPath), sidecar{
		Origin:  origin,
		SavedAt: time.Now().UTC(),
		Crash:   crash,
		PID:     os.Getpid(),
	})
}

func sidecarPath(shipPath string) string {
	return strings.TrimSuffix(shipPath, ShipExtension) + ".json"
}

// FindRecoverable lists autosaves worth offering back, newest first.
//
// An autosave belonging to this very process is skipped: it is not recovery,
// it is the file we are in the middle of writing.
func FindRecoverable() ([]Autosave, error) {
	dir, err := AutosaveDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Autosave
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ShipExtension) {
			continue
		}
		full := filepath.Join(dir, e.Name())
		rec := Autosave{Path: full, Crash: strings.Contains(e.Name(), "-crash")}
		if info, err := e.Info(); err == nil {
			rec.SavedAt = info.ModTime()
		}
		if side, err := readSidecar(sidecarPath(full)); err == nil {
			rec.Origin = side.Origin
			if !side.SavedAt.IsZero() {
				rec.SavedAt = side.SavedAt
			}
			if side.PID == os.Getpid() {
				continue
			}
			rec.Crash = side.Crash
		}
		// Work that was already saved somewhere newer is not worth recovering:
		// the document on disk has everything this file does.
		if rec.Origin != "" {
			if info, err := os.Stat(rec.Origin); err == nil && info.ModTime().After(rec.SavedAt) {
				continue
			}
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	return out, nil
}

// DiscardAutosave removes a recovery file and its record.
func DiscardAutosave(shipPath string) {
	os.Remove(shipPath)
	os.Remove(sidecarPath(shipPath))
}

// ClearOwnAutosaves removes the recovery files this process wrote, which is
// what a clean save or a clean exit does: there is nothing left to recover.
func ClearOwnAutosaves(origin string) {
	dir, err := AutosaveDir()
	if err != nil {
		return
	}
	for _, crash := range []bool{false, true} {
		p := filepath.Join(dir, autosaveName(origin, os.Getpid(), crash))
		DiscardAutosave(p)
	}
}

func readSidecar(path string) (sidecar, error) {
	var out sidecar
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	return out, unmarshalJSON(data, &out)
}

// writeJSONAtomic writes a small record the same way everything else here
// writes: beside the target, then renamed over it.
func writeJSONAtomic(path string, v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}
