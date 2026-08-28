package io

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"
)

// Native file dialogs (D-10, SPEC-DATA §5).
//
// One thin wrapper, for two reasons. Cancelling is not an error the app should
// ever have to think about, so it comes back as a plain false rather than
// something to unwrap. And a dialog blocks the frame loop while it is up, which
// is normal for a modal but means every caller here is a place the window
// stops — worth being able to find them all in one file.

// ErrDialogCancelled is what zenity reports when the user backs out. It is
// never surfaced: the callers get a false.
var errCancelled = zenity.ErrCanceled

// shipFilter is the project file type: .pxm, plus the .ship name it had
// before the rename, so old files stay one dialog away.
func shipFilter() zenity.FileFilters {
	return zenity.FileFilters{{
		Name:     "Pixel model (*.pxm, *.ship)",
		Patterns: []string{"*" + ShipExtension, "*" + LegacyShipExtension},
		CaseFold: true,
	}}
}

// AskOpenShip asks for a project to open. It returns false if the user
// cancelled, which is not a failure and is never reported as one.
func AskOpenShip(startIn string) (string, bool, error) {
	return ask(zenity.SelectFile(
		zenity.Title("Open ship"),
		shipFilter(),
		zenity.Filename(startIn),
	))
}

// AskSaveShip asks where to write a project, defaulting to a name.
func AskSaveShip(suggested string) (string, bool, error) {
	path, ok, err := ask(zenity.SelectFileSave(
		zenity.Title("Save ship as"),
		zenity.ConfirmOverwrite(),
		shipFilter(),
		zenity.Filename(suggested),
	))
	if !ok || err != nil {
		return "", ok, err
	}
	return ensureExtension(path, ShipExtension), true, nil
}

// AskExport asks where to write an export of a given kind. The filter and the
// extension come from the format so the dialog and the file agree.
func AskExport(title, suggested, extension, describe string) (string, bool, error) {
	path, ok, err := ask(zenity.SelectFileSave(
		zenity.Title(title),
		zenity.ConfirmOverwrite(),
		zenity.FileFilters{{
			Name:     describe,
			Patterns: []string{"*" + extension},
			CaseFold: true,
		}},
		zenity.Filename(suggested),
	))
	if !ok || err != nil {
		return "", ok, err
	}
	return ensureExtension(path, extension), true, nil
}

// AskOpenPalette asks for a Lospec .hex palette (SPEC-UX §13.3).
func AskOpenPalette(startIn string) (string, bool, error) {
	return ask(zenity.SelectFile(
		zenity.Title("Import palette"),
		zenity.FileFilters{{
			Name:     "Lospec palette (*.hex)",
			Patterns: []string{"*.hex"},
			CaseFold: true,
		}},
		zenity.Filename(startIn),
	))
}

// ask turns zenity's cancel-as-error into a plain false.
func ask(path string, err error) (string, bool, error) {
	if errors.Is(err, errCancelled) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(path) == "" {
		return "", false, nil
	}
	return path, true, nil
}

// ensureExtension adds the suffix when the dialog handed one back without it,
// which the Windows dialog does whenever the user types a bare name.
func ensureExtension(path, ext string) string {
	if strings.EqualFold(filepath.Ext(path), ext) {
		return path
	}
	return path + ext
}
