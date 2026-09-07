package apptest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"modeler/internal/appearance"
	"modeler/internal/bridge"
	"modeler/internal/io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLiveAIConnectionModelsPaintsAndSaves(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(exePath, "-ai", "-hidden")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), io.ConfigDirEnv+"="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var session bridge.Session
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		path := filepath.Join(dir, "ai", fmt.Sprintf("session-%d.json", cmd.Process.Pid))
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &session) == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if session.Token == "" {
		t.Fatal("live connection did not start")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	request := func(method, path string, body any) *http.Response {
		t.Helper()
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, session.URL+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+session.Token)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	count := 0
	job := func(kind string, ops any) bridge.Job {
		t.Helper()
		count++
		id := fmt.Sprintf("test-%d", count)
		res := request("POST", "/v1/jobs", map[string]any{"id": id, "type": kind, "ops": ops})
		res.Body.Close()
		if res.StatusCode != 202 {
			t.Fatalf("submit returned %d", res.StatusCode)
		}
		until := time.Now().Add(30 * time.Second)
		for time.Now().Before(until) {
			res = request("GET", "/v1/jobs/"+id, nil)
			var j bridge.Job
			err := json.NewDecoder(res.Body).Decode(&j)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if j.Status == "failed" {
				t.Fatalf("%s failed: %s", kind, j.Error)
			}
			if j.Status == "done" {
				return j
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("job did not complete")
		return bridge.Job{}
	}
	decodeState := func(j bridge.Job, nested bool) map[string]json.RawMessage {
		t.Helper()
		var state map[string]json.RawMessage
		if err := json.Unmarshal(j.Result, &state); err != nil {
			t.Fatal(err)
		}
		if nested {
			if err := json.Unmarshal(state["state"], &state); err != nil {
				t.Fatal(err)
			}
		}
		return state
	}
	initial := decodeState(job("state", nil), false)
	if string(initial["bodies"]) != "[]" {
		t.Fatal("test inherited an existing model")
	}
	commands := []map[string]any{{"op": "sketch.begin", "plane": "Top"}, {"op": "sketch.rect", "a": []int{-2, -1}, "b": []int{2, 1}}, {"op": "extrude", "depth": 2, "result": "new"}, {"op": "camera.view", "view": "iso"}, {"op": "camera.frame"}, {"op": "settle"}}
	created := job("commands", commands)
	state := decodeState(created, true)
	var bodies []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(state["bodies"], &bodies); err != nil || len(bodies) != 1 {
		t.Fatal("modeling command did not create one body", err)
	}
	name := bodies[0].Name
	path := filepath.Join(dir, "painted.ship")
	job("commands", []map[string]any{{"op": "paint.begin"}, {"op": "paint.res", "res": 4}, {"op": "paint.color", "hex": "21E7E7"}, {"op": "paint.pixel", "body": name, "axis": "+y", "uv": []int{0, 0}}, {"op": "paint.exit"}, {"op": "file.save", "path": path}})
	loaded, err := io.LoadShip(path)
	if err != nil {
		t.Fatal(err)
	}
	painted := false
	for _, f := range loaded.Doc.Bodies[0].Mesh.Faces {
		if f.Paint != nil && f.Paint.Img != nil {
			p := f.Paint.Img
			for y := p.Rect.Min.Y; y < p.Rect.Max.Y; y++ {
				for x := p.Rect.Min.X; x < p.Rect.Max.X; x++ {
					if p.RGBAAt(x, y) == (color.RGBA{R: 33, G: 231, B: 231, A: 255}) {
						painted = true
					}
				}
			}
		}
	}
	if !painted {
		t.Fatal("paint command did not save the requested pixels")
	}
	capture := job("capture", nil)
	var captured struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(capture.Result, &captured); err != nil {
		t.Fatal(err)
	}
	img, err := LoadPNG(captured.Path)
	if err != nil || img.Bounds().Dx() < 1280 {
		t.Fatal("screenshot not captured", err)
	}
	// An idempotent retry of the original modeling job must not add another body.
	res := request("POST", "/v1/jobs", map[string]any{"id": created.Request.ID, "type": "commands", "ops": commands})
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("retry was not recognized")
	}
	job("commands", []map[string]any{{"op": "select", "kind": "body", "body": name}, {"op": "delete"}, {"op": "undo"}})
	state = decodeState(job("state", nil), false)
	if err := json.Unmarshal(state["bodies"], &bodies); err != nil || len(bodies) != 1 {
		t.Fatal("retry or undo changed body count", err)
	}
	if string(state["controls"]) == "[]" {
		t.Fatal("visible controls were not exposed")
	}
	// Focus and text entry must survive separate HTTP requests, as they do
	// when an assistant inspects the UI between actions.
	clickControl := func(match func(string, string) bool) {
		t.Helper()
		state := decodeState(job("state", nil), false)
		var controls []struct {
			Label, Kind string
			Enabled     bool
			Rect        struct{ X, Y, Width, Height float64 }
		}
		if err := json.Unmarshal(state["controls"], &controls); err != nil {
			t.Fatal(err)
		}
		for _, c := range controls {
			if c.Enabled && match(c.Kind, c.Label) {
				job("commands", []map[string]any{{"op": "click", "at": []float64{c.Rect.X + c.Rect.Width/2, c.Rect.Y + c.Rect.Height/2}}})
				return
			}
		}
		t.Fatal("requested live control not found")
	}
	job("commands", []map[string]any{{"op": "select", "kind": "body", "body": name}})
	clickControl(func(kind, label string) bool { return label == "Save selected to Library" })
	clickControl(func(kind, label string) bool { return kind == "text" && strings.Contains(label, "Part name") })
	job("commands", []map[string]any{{"op": "ui.type", "name": "AI field entry"}})
	state = decodeState(job("state", nil), false)
	if !strings.Contains(string(state["controls"]), "AI field entry") {
		t.Fatal("text focus was lost between requests")
	}
	job("commands", []map[string]any{{"op": "ui.key", "name": "Escape"}})
	// Exercise the actual palette and motion controls, including preferences
	// written by a live window rather than a headless settings fixture.
	clickControl(func(kind, label string) bool { return label == "Settings" })
	clickControl(func(kind, label string) bool { return kind == "palette" && label == "Ocean Dark" })
	clickControl(func(kind, label string) bool { return label == "Light" })
	clickControl(func(kind, label string) bool { return kind == "palette" && label == "Sand" })
	clickControl(func(kind, label string) bool { return label == "Slow" })
	clickControl(func(kind, label string) bool { return label == "Reduced" })
	preferences, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var savedSettings struct{ Theme, AppearancePalette, UIMotion string }
	if err := json.Unmarshal(preferences, &savedSettings); err != nil || savedSettings.Theme != "light" || savedSettings.AppearancePalette != "Sand" || savedSettings.UIMotion != "off" {
		t.Fatalf("live appearance preferences were not saved: %+v, %v", savedSettings, err)
	}
	clickControl(func(kind, label string) bool { return label == "Effects" })
	clickControl(func(kind, label string) bool { return label == "Button glow" })
	clickControl(func(kind, label string) bool { return label == "Color cycling" })
	clickControl(func(kind, label string) bool { return label == "Bounce" })
	job("commands", []map[string]any{{"op": "ui.key", "name": "End"}, {"op": "ui.key", "name": "Enter"}})
	clickControl(func(kind, label string) bool { return label == "SVG path motion" })
	job("commands", []map[string]any{{"op": "wheel", "at": []float64{850, 500}, "degrees": -8}})
	clickControl(func(kind, label string) bool { return label == "Smooth easing" })
	job("commands", []map[string]any{{"op": "ui.key", "name": "End"}, {"op": "ui.key", "name": "Enter"}})
	clickControl(func(kind, label string) bool { return label == "Normal" })
	preferences, err = os.ReadFile(filepath.Join(dir, "settings.json"))
	var effectsSettings struct{ UIEffects appearance.Effects }
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(preferences, &effectsSettings); err != nil {
		t.Fatal(err)
	}
	if e := effectsSettings.UIEffects; e.Glow || !e.ColorCycle || e.PathMotion || e.PressEffect != "gelatin" || e.MotionStyle != "bouncy" {
		t.Fatalf("live effect controls did not persist: %+v", e)
	}
	clickControl(func(kind, label string) bool { return label == "Done" })
	if runtime.GOOS == "windows" {
		output := filepath.Join(dir, "helper-state.json")
		helper := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(repoRoot, "tools", "modeler.ps1"), "-Action", "State", "-ProcessId", fmt.Sprint(cmd.Process.Pid), "-ConfigDir", dir, "-Output", output)
		if out, err := helper.CombinedOutput(); err != nil {
			t.Fatalf("local helper failed: %v\n%s", err, out)
		}
		data, err := os.ReadFile(output)
		if err != nil || !bytes.Contains(data, []byte(name)) {
			t.Fatal("local helper did not inspect the model", err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "ai", "checkpoints", "*.ship"))
	if len(files) < 3 {
		t.Fatal("pre-edit checkpoints missing")
	}
	job("disconnect", nil)
}
