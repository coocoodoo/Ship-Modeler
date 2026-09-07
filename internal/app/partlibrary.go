package app

import (
	"fmt"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/ui"
	"os"
	"sort"
	"strings"
)

type partLibraryState struct {
	dir                             string
	parts                           []io.LibraryPart
	err                             string
	open, sectionOpen               bool
	query, selected                 string
	page                            int
	saveOpen                        bool
	name, category                  string
	sources                         []*model.Body
	sourceIDs                       []uint32
	updatePart, editPart            io.LibraryPart
	editDoc                         *model.Document
	editBodies                      []uint32
	categoryMenu                    bool
	categoryPage                    int
	categoryX, categoryY, categoryW float32
	menu                            bodyMenuState
	preview                         partLibraryPreview
}

func (a *App) initPartLibrary() {
	a.library.sectionOpen = true
	// Tests never read or write the user's library. A supplied configuration
	// directory makes a headless session's library explicitly isolated.
	if a.Headless && os.Getenv(io.ConfigDirEnv) == "" {
		return
	}
	dir, err := io.PartLibraryDir()
	if err != nil {
		a.library.err = err.Error()
		return
	}
	a.library.dir = dir
	a.refreshPartLibrary()
}
func (a *App) refreshPartLibrary() {
	a.dropLibraryPreview()
	if a.library.dir == "" {
		return
	}
	parts, err := io.ReadPartLibrary(a.library.dir)
	if err != nil {
		a.library.err = err.Error()
		return
	}
	a.library.parts = parts
	a.library.err = ""
}
func (a *App) libraryCategories() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range a.library.parts {
		if p.Category == "Uncategorized" {
			continue
		}
		if !seen[p.Category] {
			seen[p.Category] = true
			out = append(out, p.Category)
		}
	}
	sort.Strings(out)
	return out
}
func (a *App) beginSaveToLibrary(ids []uint32) {
	if a.Mode != ModeIdle {
		return
	}
	var sources []*model.Body
	seen := map[uint32]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if b := a.Doc().BodyByID(id); b != nil && b.Mesh != nil {
			sources = append(sources, model.SnapshotBody(b))
		}
	}
	if len(sources) == 0 {
		a.Toast(ui.Toast{Text: "Select a body first", Kind: ui.ToastWarn})
		return
	}
	a.refreshPartLibrary()
	a.library.sources = sources
	a.library.sourceIDs = nil
	for _, b := range sources {
		a.library.sourceIDs = append(a.library.sourceIDs, b.ID)
	}
	a.library.updatePart = io.LibraryPart{}
	a.library.name = sources[0].Name
	if len(sources) > 1 {
		a.library.name = "Modular assembly"
	}
	a.library.category = ""
	a.library.categoryMenu = false
	a.library.saveOpen = true
	a.library.open = false
	a.library.menu = bodyMenuState{}
	a.UI.ClearFocus()
}
func (a *App) selectedLibrarySources() []uint32 {
	var ids []uint32
	for _, r := range a.Sel.Refs() {
		if r.Body != 0 {
			ids = append(ids, r.Body)
		}
	}
	return ids
}
func (a *App) saveLibraryPart() bool {
	if a.library.dir == "" {
		a.library.err = "Library storage is unavailable"
		return false
	}
	st := &a.library
	catalog, err := io.ReadPartLibrary(st.dir)
	if err != nil {
		st.err = err.Error()
		return false
	}
	category := strings.TrimSpace(st.category)
	if category == "" {
		category = "Uncategorized"
	}
	for _, p := range catalog {
		if st.updatePart.ID == "" && strings.EqualFold(p.Name, strings.TrimSpace(st.name)) && strings.EqualFold(p.Category, category) {
			st.err = "That part already exists. Choose it in Library and use Update from selection."
			return false
		}
	}
	var part io.LibraryPart
	updating := st.updatePart.ID != ""
	if updating {
		part, err = io.UpdateLibraryPart(st.dir, st.updatePart, st.name, st.category, st.sources)
	} else {
		part, err = io.SaveLibraryPart(st.dir, st.name, st.category, st.sources)
	}
	if err != nil {
		a.library.err = err.Error()
		return false
	}
	a.library.saveOpen = false
	a.library.sources = nil
	a.UI.ClearFocus()
	a.refreshPartLibrary()
	a.library.selected = part.ID
	if updating {
		st.editPart, st.editDoc = part, a.Doc()
		st.editBodies = append([]uint32(nil), st.sourceIDs...)
		a.Toast(ui.Toast{Text: "Updated " + part.Name + " in Library · No duplicate created"})
	} else {
		a.Toast(ui.Toast{Text: "Saved " + part.Name + " to Library · Available in every project"})
	}
	return true
}

func (a *App) libraryEditAvailable() bool {
	st := &a.library
	if st.editPart.ID == "" || st.editDoc != a.Doc() || len(st.editBodies) == 0 {
		return false
	}
	for _, id := range st.editBodies {
		if b := a.Doc().BodyByID(id); b == nil || b.Mesh == nil {
			return false
		}
	}
	return true
}

func (a *App) beginUpdateLibraryPart(part io.LibraryPart, ids []uint32) {
	if a.Mode != ModeIdle || part.ID == "" || len(ids) == 0 {
		return
	}
	a.beginSaveToLibrary(ids)
	if !a.library.saveOpen {
		return
	}
	a.library.updatePart = part
	a.library.name, a.library.category = part.Name, part.Category
}

func (a *App) editLibraryPart(id string) bool {
	if a.Mode != ModeIdle {
		return false
	}
	st := &a.library
	if st.editPart.ID == id && a.libraryEditAvailable() {
		a.Sel.Clear()
		for _, body := range st.editBodies {
			a.Sel.Add(model.BodyRef(body))
		}
		st.open = false
		a.armTransform()
		return true
	}
	for _, part := range st.parts {
		if part.ID == id {
			if !a.insertLibraryPart(id) {
				return false
			}
			st.editPart, st.editDoc = part, a.Doc()
			st.editBodies = a.selectedLibrarySources()
			a.Toast(ui.Toast{Text: "Edit " + part.Name + ", then choose Save library changes in the sidebar"})
			return true
		}
	}
	return false
}
func (a *App) insertLibraryPart(id string) bool {
	if a.Mode != ModeIdle {
		return false
	}
	for _, part := range a.library.parts {
		if part.ID == id {
			bodies, err := io.LoadLibraryPart(a.library.dir, part)
			if err != nil {
				a.library.err = err.Error()
				return false
			}
			if len(bodies) == 1 {
				bodies[0].Name = part.Name
			}
			cmd := &model.PasteBodies{Sources: bodies}
			if !a.Run(cmd) {
				return false
			}
			a.Sel.Clear()
			for _, b := range cmd.Copies() {
				a.Sel.Add(model.BodyRef(b.ID))
			}
			a.armTransform()
			a.library.open = false
			a.UI.ClearFocus()
			a.Toast(ui.Toast{Text: fmt.Sprintf("Inserted %s · Drag the gizmo to position it", part.Name)})
			return true
		}
	}
	a.library.err = "Choose a library part first"
	return false
}
func (a *App) filteredLibraryParts() []io.LibraryPart {
	query := strings.ToLower(strings.TrimSpace(a.library.query))
	var out []io.LibraryPart
	for _, p := range a.library.parts {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Category), query) {
			out = append(out, p)
		}
	}
	return out
}
func (a *App) libraryOwnsInput() bool {
	return a.library.menu.open || a.library.saveOpen || a.library.open
}
