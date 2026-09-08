package app

import (
	"bytes"
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image"
	"image/png"
	"modeler/internal/geom/mesh"
	mio "modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
	"path/filepath"
	"time"
)

type workflowState struct {
	scroll          float32
	contentHeight   float32
	opacityDragging bool
	open            bool
	tab             int
	face            model.FaceScope
	reference       *paint.MaterialReference
	issues          []model.MaterialIssue
	issuePage       int
	layerPage       int
	description     string
	beforeView      bool
	images          [2][]byte
	thumbnails      [2]rl.Texture2D
	inspect         bool
	inspectionPaths []string
}

func (a *App) openWorkflow(s model.FaceScope, tab int) {
	a.finishWorkshopOpacity()
	a.workflow.contentHeight = 0
	a.workflow.scroll = 0
	a.workflow.face = s
	a.workflow.tab = tab
	a.workflow.open = true
	a.notePins.open = false
	a.library.menu = bodyMenuState{}
	a.UI.ClearFocus()
	if tab == 2 {
		a.workflow.issues = model.MaterialHealth(a.Doc())
	}
}
func (a *App) startPinWork(id uint32) error {
	a.finishStroke()
	if e := a.Bus.BeginReview(id); e != nil {
		return e
	}
	a.workflow.description = ""
	a.openWorkflow(a.Bus.Review().Scope[0], 0)
	return nil
}
func (a *App) workflowError(e error) {
	if e != nil {
		a.Toast(ui.Toast{Text: e.Error(), Kind: ui.ToastWarn})
	}
}
func (a *App) toggleReviewBefore(on bool) {
	a.workflow.beforeView = on
	a.Bus.Events.Emit(model.Event{Kind: model.EvDocReplaced})
}
func isRequestUndo(s string) bool { return len(s) >= 10 && s[:10] == "AI request" }
func (a *App) reviewImage(i int, b []byte, r rl.Rectangle, label string) {
	st := &a.workflow
	a.UI.Text(ui.Rect(r.X, r.Y, r.Width, a.px(20)), label, ui.FontSizeSmall, ui.ColorTextDim)
	r.Y += a.px(22)
	r.Height -= a.px(22)
	if !bytes.Equal(st.images[i], b) {
		if st.thumbnails[i].ID != 0 {
			rl.UnloadTexture(st.thumbnails[i])
		}
		st.thumbnails[i] = rl.Texture2D{}
		st.images[i] = append([]byte(nil), b...)
		if img, e := png.Decode(bytes.NewReader(b)); e == nil {
			rgba := mesh.CopyImage(toRGBAImage(img))
			raw := rl.GenImageColor(rgba.Rect.Dx(), rgba.Rect.Dy(), rl.Blank)
			st.thumbnails[i] = rl.LoadTextureFromImage(raw)
			rl.UnloadImage(raw)
			rl.UpdateTexture(st.thumbnails[i], rgba)
			rl.SetTextureFilter(st.thumbnails[i], rl.FilterPoint)
		}
	}
	t := st.thumbnails[i]
	if t.ID != 0 {
		size := min(r.Width, r.Height)
		rl.DrawTexturePro(t, ui.Rect(0, 0, float32(t.Width), float32(t.Height)), ui.Rect(r.X+(r.Width-size)/2, r.Y, size, size), rl.Vector2{}, 0, rl.White)
	} else {
		a.UI.TextCentered(r, "No preview yet", ui.FontSizeSmall, ui.ColorTextDim)
	}
}
func toRGBAImage(img image.Image) *image.RGBA {
	p := image.NewRGBA(img.Bounds())
	for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
		for x := p.Rect.Min.X; x < p.Rect.Max.X; x++ {
			p.Set(x, y, img.At(x, y))
		}
	}
	return p
}
func (a *App) processInspection() {
	if !a.workflow.inspect {
		return
	}
	a.workflow.inspect = false
	dir, e := mio.SettingsDir()
	if e != nil {
		a.workflowError(e)
		return
	}
	dir = filepath.Join(dir, "inspections", time.Now().Format("20060102-150405"))
	r := NewScriptRunner(a, dir, ShotSize{rl.GetRenderWidth(), rl.GetRenderHeight()})
	defer r.Close()
	e = r.inspection()
	a.workflow.inspectionPaths = r.Shots()
	a.workflowError(e)
	if e == nil {
		a.Toast(ui.Toast{Text: "Inspection views saved to " + dir})
	}
}
func (r *ScriptRunner) inspection() error {
	a := r.App
	camera := a.Camera
	sel := append([]model.Ref(nil), a.Sel.Refs()...)
	open := a.workflow.open
	pins := a.notePins.hidden
	a.workflow.open = false
	a.notePins.hidden = true
	defer func() {
		a.Anim.Cancel()
		a.Camera = camera
		a.Sel.Clear()
		for _, s := range sel {
			a.Sel.Add(s)
		}
		a.workflow.open = open
		a.notePins.hidden = pins
	}()
	for _, name := range []string{"front", "back", "left", "right", "top", "bottom"} {
		v, _ := render.ParseStandardView(name)
		a.Sel.Clear()
		a.SetView(v)
		a.FrameSelection(a.Viewport(r.Size.W, r.Size.H))
		if e := r.settle(); e != nil {
			return e
		}
		if e := r.shot("inspection-" + name); e != nil {
			return e
		}
	}
	faces := []model.FaceScope{}
	if review := a.Bus.Review(); review != nil {
		faces = review.Changed
	} else {
		for _, s := range sel {
			if s.Kind == model.SelFace {
				faces = append(faces, model.FaceScope{Body: s.Body, Face: s.Face})
			}
		}
	}
	for i, s := range faces {
		a.Sel.Set(model.FaceRef(s.Body, s.Face))
		a.LookAtSelection(a.Viewport(r.Size.W, r.Size.H))
		if e := r.settle(); e != nil {
			return e
		}
		a.Sel.Clear()
		if e := r.shot(fmt.Sprintf("inspection-face-%d", i+1)); e != nil {
			return e
		}
	}
	return nil
}
