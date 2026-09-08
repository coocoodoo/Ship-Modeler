package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/bridge"
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/ui"
)

type aiConnection struct {
	server *bridge.Server
	dir    string
	stopAt time.Time
}

func (a *App) startAI() error {
	if a.ai != nil {
		return nil
	}
	dir, err := io.SettingsDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "ai")
	s, err := bridge.Start(dir, io.ScriptCatalog())
	if err != nil {
		return err
	}
	a.ai = &aiConnection{server: s, dir: dir}
	a.UI.TrackControls = true
	a.Toast(ui.Toast{Text: "AI connection on · Local access to this Modeler session"})
	return nil
}
func (a *App) stopAI() {
	if a.ai != nil {
		a.ai.server.Close()
		a.ai = nil
		a.UI.TrackControls = false
	}
}

// processAI is only called from the window thread, outside a drawing block.
func (a *App) processAI() {
	if a.aiToggle {
		a.aiToggle = false
		if a.ai != nil {
			a.stopAI()
			a.Toast(ui.Toast{Text: "AI connection off"})
		} else if err := a.startAI(); err != nil {
			a.Toast(ui.Toast{Text: err.Error(), Kind: ui.ToastError})
		}
	}
	if a.ai == nil {
		return
	}
	if !a.ai.stopAt.IsZero() && time.Now().After(a.ai.stopAt) {
		a.stopAI()
		return
	}
	conn := a.ai
	req, ok := conn.server.Next()
	if !ok {
		return
	}
	result, err := a.runAIRequest(req, conn.dir)
	conn.server.Complete(req.ID, result, err)
}

func (a *App) runAIRequest(req bridge.Request, dir string) (result any, err error) {
	// A command failure must be reported to its caller, not crash the app.
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("operation failed: %v", p)
		}
	}()
	switch req.Type {
	case "state":
		return a.aiState(), nil
	case "capture":
		path := filepath.Join(dir, "captures", req.ID+".png")
		if err := a.captureAI(path); err != nil {
			return nil, err
		}
		return map[string]any{"path": path, "width": a.layout.Screen.Width, "height": a.layout.Screen.Height}, nil
	case "disconnect":
		a.ai.stopAt = time.Now().Add(2 * time.Second)
		return map[string]bool{"disconnected": true}, nil
	case "commands":
		script, err := io.ParseScript(req.Ops)
		if err != nil {
			return nil, err
		}
		if len(script.Ops) == 0 || len(script.Ops) > 128 {
			return nil, fmt.Errorf("send between 1 and 128 operations")
		}
		for _, op := range script.Ops {
			if op.Op == "library.testdir" {
				return nil, fmt.Errorf("test-only operation is unavailable in a live session")
			}
			if op.Segs > 600 || len(op.Points) > 10000 || len(op.Pts) > 10000 {
				return nil, fmt.Errorf("operation is too large; split it into smaller steps")
			}
			if op.Op == "shot" && (filepath.Base(op.Name) != op.Name || strings.ContainsAny(op.Name, `\/:`)) {
				return nil, fmt.Errorf("shot name must be a file name")
			}
		}
		checkpoint := filepath.Join(dir, "checkpoints", req.ID+".ship")
		if err := os.MkdirAll(filepath.Dir(checkpoint), 0700); err != nil {
			return nil, err
		}
		if err := io.SaveShip(checkpoint, a.Doc(), nil); err != nil {
			return nil, fmt.Errorf("could not checkpoint the current model: %w", err)
		}
		runner := NewScriptRunner(a, filepath.Join(dir, "captures"), ShotSize{rl.GetRenderWidth(), rl.GetRenderHeight()})
		runner.Live = true
		runner.mouse = [2]float64{a.lastMouseX, a.lastMouseY}
		defer runner.Close()
		completed := 0
		for _, op := range script.Ops {
			if a.Bus.Review() != nil && (op.Op == "click" || op.Op == "ui.key" || op.Op == "drag" || op.Op == "mouse.down" || op.Op == "mouse.up" || op.Op == "drag.release" || op.Op == "wheel") {
				return map[string]any{"completed": completed, "state": a.aiState()}, fmt.Errorf("use face commands during a request; approval and scope expansion are reserved for the user")
			}
			if err := runner.runOp(op); err != nil {
				return map[string]any{"completed": completed, "checkpoint": checkpoint, "state": a.aiState()}, err
			}
			completed++
		}
		return map[string]any{"completed": completed, "checkpoint": checkpoint, "shots": runner.Shots(), "state": a.aiState()}, nil
	}
	return nil, fmt.Errorf("unknown request type")
}

func (a *App) captureAI(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	runner := NewScriptRunner(a, "", ShotSize{rl.GetRenderWidth(), rl.GetRenderHeight()})
	defer runner.Close()
	runner.ensureTarget()
	in := runner.frame()
	in.MouseX, in.MouseY = a.lastMouseX, a.lastMouseY
	for i := range in.Down {
		in.Down[i] = a.UI.In.Down[i]
	}
	rl.BeginDrawing()
	rl.BeginTextureMode(runner.rt)
	a.draw(in)
	rl.EndTextureMode()
	rl.EndDrawing()
	img := rl.LoadImageFromTexture(runner.rt.Texture)
	if img == nil {
		return fmt.Errorf("could not capture the model view")
	}
	defer rl.UnloadImage(img)
	rl.ImageFlipVertical(img)
	if !rl.ExportImage(*img, path) {
		return fmt.Errorf("could not write screenshot")
	}
	return nil
}

func (a *App) aiState() map[string]any {
	doc := a.Doc()
	toasts := make([]string, 0)
	for _, toast := range a.UI.Toasts() {
		toasts = append(toasts, toast.Text)
	}
	bodies := make([]any, 0, len(doc.Bodies))
	for _, b := range doc.Bodies {
		entry := map[string]any{"id": b.ID, "name": b.Name, "visible": b.Visible, "color": b.Color}
		if b.Mesh != nil {
			entry["vertices"] = b.Mesh.Verts
			entry["edges"] = b.Mesh.Topo().Edges
			entry["drawnEdges"] = b.Mesh.DrawnEdges()
			entry["bounds"] = geom.AABBOf(b.Mesh.Verts)
			faces := make([]any, 0, len(b.Mesh.Faces))
			for fi, f := range b.Mesh.Faces {
				face := map[string]any{"index": fi, "id": fmt.Sprint(f.ID), "normal": b.Mesh.FaceNormal(fi), "loops": f.Loops}
				if f.Paint != nil {
					p := f.Paint
					face["layers"] = layerMetadata(p)
					face["paint"] = map[string]any{"resolution": p.Res, "texelSize": p.Texel, "frame": p.Frame, "offset": p.Off, "bounds": p.TexelBounds()}
					if p.Material != nil {
						face["material"] = p.Material
					}
				}
				faces = append(faces, face)
			}
			entry["faces"] = faces
		}
		bodies = append(bodies, entry)
	}
	return map[string]any{"document": a.DocumentName(), "path": a.files.path, "dirty": doc.DirtySinceSave, "mode": a.modeName(), "viewer": a.Viewer,
		"review": a.Bus.Review(), "materialReference": a.workflow.reference, "materialHealth": a.workflow.issues, "inspectionViews": a.workflow.inspectionPaths,
		"notePins": a.aiNotePins(), "bodies": bodies, "sketches": doc.Sketches, "markers": doc.Markers, "planes": doc.Planes, "selection": a.Sel.Refs(),
		"camera": a.Camera, "viewport": a.layout.Viewport, "window": a.layout.Screen, "uiScale": a.Scale, "controls": a.UI.Controls,
		"undo": a.Bus.UndoDepth(), "canUndo": a.Bus.CanUndo(), "canRedo": a.Bus.CanRedo(), "modalOpen": a.UI.ModalOpen(),
		"hint": a.HintText(), "pick": a.Hover, "library": a.library.parts, "toasts": toasts, "paint": map[string]any{"tool": a.paint.tool.String(), "color": a.paint.color, "size": a.paint.size, "resolution": a.paint.res}}
}

func automationKey(name string) (int32, bool) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if len(name) == 1 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= '0' && name[0] <= '9')) {
		return int32(name[0]), true
	}
	keys := map[string]int32{"ESCAPE": rl.KeyEscape, "ESC": rl.KeyEscape, "ENTER": rl.KeyEnter, "TAB": rl.KeyTab, "SPACE": rl.KeySpace, "BACKSPACE": rl.KeyBackspace, "DELETE": rl.KeyDelete, "LEFT": rl.KeyLeft, "RIGHT": rl.KeyRight, "UP": rl.KeyUp, "DOWN": rl.KeyDown, "HOME": rl.KeyHome, "END": rl.KeyEnd, "PAGEUP": rl.KeyPageUp, "PAGEDOWN": rl.KeyPageDown, "SLASH": rl.KeySlash, "COMMA": rl.KeyComma, "PERIOD": rl.KeyPeriod, "MINUS": rl.KeyMinus, "EQUAL": rl.KeyEqual}
	if key, ok := keys[name]; ok {
		return key, true
	}
	for i := 0; i < 12; i++ {
		if name == fmt.Sprintf("F%d", i+1) {
			return rl.KeyF1 + int32(i), true
		}
	}
	return 0, false
}

func layerMetadata(p *mesh.FacePaint) any {
	layers := []any{}
	for i, l := range p.Layers {
		layers = append(layers, map[string]any{"index": i, "name": l.Name, "visible": l.Visible, "opacity": l.Opacity, "hasMask": l.Mask != nil})
	}
	return map[string]any{"active": p.ActiveLayer, "paintMask": p.PaintMask, "items": layers, "pbrStale": p.PBRStale}
}
