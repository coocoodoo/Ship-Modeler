package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/io"
	"modeler/internal/render"
)

// Headless mode is the executing agent's eyes (SPEC-RENDER §9): a hidden window
// runs an op script through the same command bus and camera controller the UI
// uses, then writes PNGs it can read back.

// VirtualFrameMillis is the fixed clock step of a headless run, so animations
// resolve deterministically no matter how fast the machine is.
const VirtualFrameMillis = 1000.0 / 60.0

// SettleMaxFrames bounds the settle op so a stuck animation fails loudly rather
// than hanging a test.
const SettleMaxFrames = 600

// ShotSize is the fixed render size for golden shots (SPEC-RENDER §10).
type ShotSize struct{ W, H int }

// DefaultShotSize is 1280x720 (SPEC-RENDER §10).
var DefaultShotSize = ShotSize{W: 1280, H: 720}

// ParseSize reads a "1280x720" size string.
func ParseSize(s string) (ShotSize, error) {
	var out ShotSize
	if _, err := fmt.Sscanf(strings.ToLower(s), "%dx%d", &out.W, &out.H); err != nil {
		return out, fmt.Errorf("bad -size %q, want WxH", s)
	}
	if out.W < 16 || out.H < 16 || out.W > 8192 || out.H > 8192 {
		return out, fmt.Errorf("size %dx%d is out of range", out.W, out.H)
	}
	return out, nil
}

// ScriptRunner executes op scripts against an App and captures shots.
type ScriptRunner struct {
	App    *App
	OutDir string
	Size   ShotSize

	rt      rl.RenderTexture2D
	rtReady bool
	shots   []string
}

// NewScriptRunner prepares a runner writing PNGs into outDir.
func NewScriptRunner(a *App, outDir string, size ShotSize) *ScriptRunner {
	return &ScriptRunner{App: a, OutDir: outDir, Size: size}
}

// Close releases the capture target.
func (r *ScriptRunner) Close() {
	if r.rtReady {
		rl.UnloadRenderTexture(r.rt)
		r.rtReady = false
	}
}

// Shots lists the files written, in order.
func (r *ScriptRunner) Shots() []string { return r.shots }

// Run executes every op in order, stopping at the first failure.
func (r *ScriptRunner) Run(s *io.Script) error {
	for _, op := range s.Ops {
		if err := r.runOp(op); err != nil {
			return err
		}
	}
	return nil
}

func (r *ScriptRunner) runOp(op io.Op) error {
	a := r.App
	vp := a.Viewport(r.Size.W, r.Size.H)

	switch op.Op {
	case "camera.view":
		v, ok := render.ParseStandardView(op.View)
		if !ok {
			return op.Errorf("unknown view %q", op.View)
		}
		a.SetView(v)

	case "camera.frame":
		a.FrameSelection(vp)

	case "camera.orbit":
		a.Anim.Cancel()
		a.Camera.Orbit(op.Degrees/render.OrbitDegPerPixel, 0)

	case "camera.zoom":
		a.Anim.Cancel()
		a.Camera.Zoom(op.Depth)

	case "camera.project":
		a.Anim.Cancel()
		a.Camera.Perspective = op.Kind == "perspective"
		a.Camera.Normalize()

	case "body.visible":
		b := a.FindBody(op.Body)
		if b == nil {
			return op.Errorf("no body named %q", op.Body)
		}
		b.Visible = *op.Visible

	case "settle":
		if err := r.settle(); err != nil {
			return op.Wrap(err)
		}

	case "shot":
		if err := r.shot(op.Name); err != nil {
			return op.Wrap(err)
		}

	case "pick":
		r.pick(op.At[0], op.At[1])

	default:
		return op.Errorf("op is not implemented yet in this build")
	}

	// Every op advances the virtual clock by one frame so animations progress
	// even without an explicit settle.
	r.step()
	return nil
}

// step advances one virtual frame of app logic with no user input.
func (r *ScriptRunner) step() {
	in := NewInputFrame()
	in.WindowW, in.WindowH = r.Size.W, r.Size.H
	in.DeltaMillis = VirtualFrameMillis
	// Park the cursor outside the viewport so hover picking stays off.
	in.MouseX, in.MouseY = -1, -1
	r.App.Update(in)
}

// settle steps the clock until every animation has finished.
func (r *ScriptRunner) settle() error {
	for i := 0; i < SettleMaxFrames; i++ {
		if !r.App.Anim.Active() {
			return nil
		}
		r.step()
	}
	return fmt.Errorf("animations did not settle within %d frames", SettleMaxFrames)
}

// shot renders one frame into the capture target and writes it as a PNG.
func (r *ScriptRunner) shot(name string) error {
	if err := os.MkdirAll(r.OutDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if !r.rtReady {
		r.rt = rl.LoadRenderTexture(int32(r.Size.W), int32(r.Size.H))
		r.rtReady = true
	}

	rl.BeginDrawing()
	rl.BeginTextureMode(r.rt)
	r.App.Draw(r.Size.W, r.Size.H)
	rl.EndTextureMode()
	rl.EndDrawing()

	img := rl.LoadImageFromTexture(r.rt.Texture)
	if img == nil {
		return fmt.Errorf("could not read the render target back")
	}
	defer rl.UnloadImage(img)
	// Render textures are stored bottom-up; flip so the PNG is the right way up.
	rl.ImageFlipVertical(img)

	path := filepath.Join(r.OutDir, name+".png")
	if !rl.ExportImage(*img, path) {
		return fmt.Errorf("could not write %s", path)
	}
	r.shots = append(r.shots, path)
	return nil
}

// pick runs the ID pass at a window pixel and prints a machine-readable line.
// It is how the flow tests prove that picking resolves the element the cursor
// is actually over (SPEC-RENDER §6).
func (r *ScriptRunner) pick(x, y float64) {
	a := r.App
	vp := a.Viewport(r.Size.W, r.Size.H)
	a.Renderer.SetFramebuffer(r.Size.W, r.Size.H)

	rl.BeginDrawing()
	s := a.BuildScene()
	res := a.Renderer.Pick(&s, vp, x, y)
	rl.EndDrawing()

	a.Hover = res
	if !res.Hit {
		fmt.Printf("pick at=%.0f,%.0f kind=none\n", x, y)
		return
	}
	fmt.Printf("pick at=%.0f,%.0f kind=%s body=%d face=%d edge=%d vert=%d plane=%s dist=%.2f\n",
		x, y, res.Kind, res.BodyID, res.FaceUID.Seq(), res.Edge, res.Vert, res.Plane, res.DistancePx)
}

// Bench renders the current scene repeatedly and reports the frame cost, which
// is how the 60 fps budget of SPEC-RENDER §8 is checked without a human
// watching the window. VSync is off in headless mode, so this measures the real
// render cost rather than the swap interval.
func (r *ScriptRunner) Bench(frames int) {
	if frames <= 0 {
		return
	}
	if !r.rtReady {
		r.rt = rl.LoadRenderTexture(int32(r.Size.W), int32(r.Size.H))
		r.rtReady = true
	}
	samples := make([]float64, 0, frames)
	for i := 0; i < frames; i++ {
		start := time.Now()
		rl.BeginDrawing()
		rl.BeginTextureMode(r.rt)
		r.App.Draw(r.Size.W, r.Size.H)
		rl.EndTextureMode()
		rl.EndDrawing()
		// A readback forces the GPU to finish, so the number is honest rather
		// than the time it took to queue the commands.
		if img := rl.LoadImageFromTexture(r.rt.Texture); img != nil {
			rl.UnloadImage(img)
		}
		samples = append(samples, float64(time.Since(start).Microseconds())/1000)
	}
	sort.Float64s(samples)
	var sum float64
	for _, v := range samples {
		sum += v
	}
	pct := func(p float64) float64 {
		i := int(p * float64(len(samples)-1))
		return samples[i]
	}
	fmt.Printf("bench frames=%d size=%dx%d mean=%.2fms p50=%.2fms p95=%.2fms p99=%.2fms max=%.2fms budget=16.60ms\n",
		len(samples), r.Size.W, r.Size.H,
		sum/float64(len(samples)), pct(0.5), pct(0.95), pct(0.99), samples[len(samples)-1])
}

// RunHeadless opens a hidden window, runs a script and writes its shots.
func RunHeadless(scriptPath, outDir string, size ShotSize, benchFrames int) error {
	script, err := io.LoadScript(scriptPath)
	if err != nil {
		return err
	}
	OpenWindow(size.W, size.H, false, true)
	defer rl.CloseWindow()

	a := New(true)
	defer a.Close()
	a.LoadTestScene()

	runner := NewScriptRunner(a, outDir, size)
	defer runner.Close()

	// One frame of warm-up so the first shot has uploaded meshes and a laid-out
	// cube, exactly like the interactive app's steady state.
	runner.step()

	if err := runner.Run(script); err != nil {
		return err
	}
	runner.Bench(benchFrames)
	if len(runner.Shots()) == 0 && benchFrames == 0 {
		return fmt.Errorf("script produced no shots; add a {\"op\":\"shot\"} step")
	}
	for _, s := range runner.Shots() {
		fmt.Println("wrote", s)
	}
	return nil
}
