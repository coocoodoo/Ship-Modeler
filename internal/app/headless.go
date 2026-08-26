package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	csggeom "modeler/internal/geom/csg"
	extrudegeom "modeler/internal/geom/extrude"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/sketch"
	"modeler/internal/tools"
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
	// dumped records that the script printed at least one dump, which makes it
	// a state probe rather than a capture and exempts it from needing a shot.
	dumped bool

	// mouse is the scripted cursor position, which persists between ops so a
	// hover op can be followed by a click at the same place.
	mouse [2]float64
	// moved marks that the next frame should report pointer motion, which is
	// what wakes the throttled hover pick.
	moved bool
	// mods are the modifier keys the next pointer op holds down.
	mods mods
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
		b := a.Doc().BodyByName(op.Body)
		if b == nil {
			return op.Errorf("no body named %q", op.Body)
		}
		if !a.Run(&model.SetBodyVisible{ID: b.ID, Visible: *op.Visible}) {
			return op.Errorf("could not change the visibility of %q", op.Body)
		}

	case "plane.visible":
		k, ok := geom.ParsePlaneKind(op.Plane)
		if !ok {
			return op.Errorf("unknown plane %q", op.Plane)
		}
		if !a.Run(&model.SetPlaneVisible{Plane: k, Visible: *op.Visible}) {
			return op.Errorf("could not change the visibility of the %s plane", op.Plane)
		}

	case "sketch.visible":
		sk := a.Doc().SketchByName(op.Sketch)
		if sk == nil {
			return op.Errorf("no sketch named %q", op.Sketch)
		}
		if !a.Run(&model.SetSketchVisible{ID: sk.ID, Visible: *op.Visible}) {
			return op.Errorf("could not change the visibility of %q", op.Sketch)
		}

	case "select":
		if err := r.selectOp(op); err != nil {
			return err
		}

	case "deselect":
		a.Sel.Clear()

	case "delete":
		a.deleteSelection()

	case "undo":
		a.Undo()

	case "redo":
		a.Redo()

	case "hover":
		r.mouse = [2]float64{op.At[0], op.At[1]}
		r.mods = mods{shift: op.Shift, ctrl: op.Ctrl, alt: op.Alt}

	case "click":
		r.mods = mods{shift: op.Shift, ctrl: op.Ctrl, alt: op.Alt}
		r.click(op.At[0], op.At[1], op.Kind)
		r.mods = mods{}

	case "ui.tree":
		a.tree.collapsed = op.Visible != nil && !*op.Visible

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

	case "dump":
		r.dumped = true
		r.dump()

	case "sketch.face":
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		if !a.BeginSketchOnFace(f.body.ID, f.uid) {
			return op.Errorf("could not start a sketch on that face")
		}

	case "sketch.project":
		if !a.InSketch() {
			return op.Errorf("no sketch is being edited")
		}
		if !a.ProjectFaceOutline() {
			return op.Errorf("the outline could not be projected")
		}

	case "pushpull":
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		a.Sel.Set(model.FaceRef(f.body.ID, f.uid))
		a.armPushPull()
		t := a.pushPull.tool
		if t == nil {
			return op.Errorf("that face cannot be pushed or pulled")
		}
		t.DistanceUnits = op.Depth
		a.rebuildPushPullPreview()
		if op.Kind == "preview" {
			return nil
		}
		if !a.CommitPushPull() {
			return op.Errorf("the push/pull was refused")
		}

	case "move":
		if err := r.moveOp(op); err != nil {
			return err
		}

	case "rotate":
		if err := r.rotateOp(op); err != nil {
			return err
		}

	case "duplicate":
		a.duplicateSelection()

	case "box.select":
		if err := r.boxSelectOp(op, vp); err != nil {
			return err
		}

	case "sketch.begin":
		k, ok := geom.ParsePlaneKind(op.Plane)
		if !ok {
			return op.Errorf("unknown plane %q", op.Plane)
		}
		if !a.BeginSketchOnPlane(k) {
			return op.Errorf("could not start a sketch on the %s plane", op.Plane)
		}

	case "sketch.line":
		if err := r.addEntity(op, model.NewLine(vec(op.From), vec(op.To))); err != nil {
			return err
		}

	case "sketch.rect":
		if err := r.addEntity(op, model.NewRect(vec(op.A), vec(op.B))); err != nil {
			return err
		}

	case "sketch.circle":
		segs := op.Segs
		if segs == 0 {
			segs = model.DefaultCircleSegs
		}
		if err := r.addEntity(op, model.NewCircle(vec(op.C), geom.ToSubunits(op.R), segs)); err != nil {
			return err
		}

	case "extrude":
		if err := r.extrudeOp(op, true); err != nil {
			return err
		}

	case "extrude.begin":
		if err := r.extrudeOp(op, false); err != nil {
			return err
		}

	case "extrude.commit":
		if !a.InExtrude() {
			return op.Errorf("the extrude tool is not open")
		}
		if !a.CommitExtrude() {
			return op.Errorf("the extrude was refused")
		}

	case "boolean":
		if err := r.booleanOp(op, true); err != nil {
			return err
		}

	case "boolean.begin":
		if err := r.booleanOp(op, false); err != nil {
			return err
		}

	case "boolean.commit":
		if !a.InBoolean() {
			return op.Errorf("the boolean tool is not open")
		}
		if !a.CommitBoolean() {
			return op.Errorf("the boolean was refused")
		}

	case "boolean.cancel":
		if !a.InBoolean() {
			return op.Errorf("the boolean tool is not open")
		}
		a.CancelBoolean()

	case "extrude.cancel":
		if !a.InExtrude() {
			return op.Errorf("the extrude tool is not open")
		}
		a.CancelExtrude()

	case "sketch.finish":
		if !a.InSketch() {
			return op.Errorf("no sketch is being edited")
		}
		a.ExitSketch(true)

	case "sketch.tool":
		if !a.InSketch() {
			return op.Errorf("no sketch is being edited")
		}
		t, ok := parseSketchTool(op.Kind)
		if !ok {
			return op.Errorf("unknown sketch tool %q", op.Kind)
		}
		a.sketch.session.SetTool(t)

	default:
		return op.Errorf("op is not implemented yet in this build")
	}

	// Every op advances the virtual clock by one frame so animations progress
	// even without an explicit settle.
	r.step()
	return nil
}

// step advances one virtual frame with no button activity.
func (r *ScriptRunner) step() { r.runFrame(r.frame()) }

// runFrame drives one whole app frame into the capture target. Headless renders
// every scripted frame, because the widget kit handles its input during the
// draw pass: a scripted click is only seen if a frame actually runs.
func (r *ScriptRunner) runFrame(in InputFrame) {
	r.ensureTarget()
	rl.BeginDrawing()
	rl.BeginTextureMode(r.rt)
	r.App.Frame(in)
	rl.EndTextureMode()
	rl.EndDrawing()
}

// ensureTarget lazily creates the offscreen render target.
func (r *ScriptRunner) ensureTarget() {
	if !r.rtReady {
		r.rt = rl.LoadRenderTexture(int32(r.Size.W), int32(r.Size.H))
		r.rtReady = true
	}
}

// frame builds an input frame at the scripted cursor position.
func (r *ScriptRunner) frame() InputFrame {
	in := NewInputFrame()
	in.WindowW, in.WindowH = r.Size.W, r.Size.H
	in.DeltaMillis = VirtualFrameMillis
	in.MouseX, in.MouseY = r.mouse[0], r.mouse[1]
	in.Shift, in.Ctrl, in.Alt = r.mods.shift, r.mods.ctrl, r.mods.alt
	if r.moved {
		// A nominal delta so hover picking treats this as real motion.
		in.MouseDX, in.MouseDY = 1, 0
		r.moved = false
	}
	return in
}

// mods is the modifier state a scripted pointer op runs under.
type mods struct{ shift, ctrl, alt bool }

// click synthesizes a full press and release at a point, driving the same
// widget code a real click would. kind selects the button: "right" and
// "middle" are accepted, anything else is the left button.
func (r *ScriptRunner) click(x, y float64, kind string) {
	button := MouseLeft
	switch kind {
	case "right":
		button = MouseRight
	case "middle":
		button = MouseMiddle
	}
	r.mouse = [2]float64{x, y}
	r.moved = true
	// Move first, so hover state settles before the button goes down.
	r.step()

	down := r.frame()
	down.Pressed[button] = true
	down.Down[button] = true
	r.runFrame(down)

	up := r.frame()
	up.Released[button] = true
	r.runFrame(up)
}

// selectOp implements the select op for whole objects. Sub-element selection
// arrives with M6.
func (r *ScriptRunner) selectOp(op io.Op) error {
	a := r.App
	switch op.Kind {
	case "", "body":
		b := a.Doc().BodyByName(op.Body)
		if b == nil {
			return op.Errorf("no body named %q", op.Body)
		}
		a.Sel.Set(model.BodyRef(b.ID))
	case "plane":
		k, ok := geom.ParsePlaneKind(op.Plane)
		if !ok {
			return op.Errorf("unknown plane %q", op.Plane)
		}
		a.Sel.Set(model.PlaneRef(k))
	case "face":
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		a.Sel.Set(model.FaceRef(f.body.ID, f.uid))
	case "vert":
		b := a.Doc().BodyByName(op.Body)
		if b == nil || b.Mesh == nil {
			return op.Errorf("no body named %q", op.Body)
		}
		if op.Vert < 0 || op.Vert >= len(b.Mesh.Verts) {
			return op.Errorf("body %q has no vertex %d", op.Body, op.Vert)
		}
		a.Sel.Set(model.VertRef(b.ID, op.Vert))
	case "edge":
		b := a.Doc().BodyByName(op.Body)
		if b == nil || b.Mesh == nil {
			return op.Errorf("no body named %q", op.Body)
		}
		a.Sel.Set(model.EdgeRef(b.ID, op.Edge))
	case "sketch":
		sk := a.Doc().SketchByName(op.Sketch)
		if sk == nil {
			return op.Errorf("no sketch named %q", op.Sketch)
		}
		a.Sel.Set(model.SketchRef(sk.ID))
	default:
		return op.Errorf("cannot select %q yet", op.Kind)
	}
	return nil
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
	r.step() // one fresh frame so the shot shows the current state

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
	r.mouse = [2]float64{x, y}
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
	r.ensureTarget()
	samples := make([]float64, 0, frames)
	for i := 0; i < frames; i++ {
		start := time.Now()
		r.step()
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

// extrudeOp runs a scripted extrude through the same tool and command the UI
// uses, so a script and a drag are the same code path.
func (r *ScriptRunner) extrudeOp(op io.Op, commit bool) error {
	a := r.App
	s := a.ActiveSketch()
	if s == nil && op.Sketch != "" {
		if found := a.Doc().SketchByName(op.Sketch); found != nil {
			a.EditSketch(found)
			s = found
		}
	}
	if s == nil && op.Sketch != "" {
		return op.Errorf("no sketch named %q was found", op.Sketch)
	}
	// With no sketch open and none named, the op is deliberately testing the
	// entry point that starts from a sketch picked in the tree, so leave the
	// selection alone and let BeginExtrude find it.
	// Naming regions overrides whatever is selected; naming none leaves a
	// selection built by earlier click ops alone, and falls back to the first
	// region only when there is nothing to preserve.
	if s != nil && (len(op.Regions) > 0 || len(a.sketch.selectedRegions) == 0) {
		regions := op.Regions
		if len(regions) == 0 {
			regions = []int{0}
		}
		a.sketch.selectedRegions = map[int]bool{}
		for _, i := range regions {
			a.sketch.selectedRegions[i] = true
		}
	}
	if !a.BeginExtrude() {
		return op.Errorf("could not open the extrude tool")
	}

	t := a.extrude.tool
	if op.Depth != 0 {
		t.SetDepth(op.Depth)
	}
	t.SetDraft(op.Draft)
	switch op.Dir {
	case "", "normal":
		t.Dir = extrudegeom.Normal
	case "reverse":
		t.Dir = extrudegeom.Reverse
	case "symmetric":
		t.Dir = extrudegeom.Symmetric
	default:
		return op.Errorf("unknown direction %q", op.Dir)
	}
	t.ThroughAll = op.Through
	// An unspecified result leaves whatever the tool chose for itself, which is
	// how a face sketch keeps its Add default (SPEC-UX §10).
	switch op.Result {
	case "":
	case "new":
		t.Result = tools.ResultNew
	case "add":
		t.Result = tools.ResultAdd
	case "subtract":
		t.Result = tools.ResultSubtract
	case "intersect":
		t.Result = tools.ResultIntersect
	default:
		return op.Errorf("unknown result %q", op.Result)
	}
	a.rebuildExtrudePreview()

	if !commit {
		return nil
	}
	if !a.CommitExtrude() {
		return op.Errorf("the extrude was refused")
	}
	return nil
}

// moveOp drags the transform gizmo by a fixed amount, through the same command
// and the same coalescing a real drag uses.
func (r *ScriptRunner) moveOp(op io.Op) error {
	a := r.App
	a.armTransform()
	t := a.transform.tool
	if t == nil {
		return op.Errorf("nothing is selected to move")
	}
	t.Mode = tools.GizmoMove
	t.Begin(tools.PartScreen, geom.Vec3{}, 0)
	t.SetDelta(geom.Vec3{X: op.Delta[0], Y: op.Delta[1], Z: op.Delta[2]})
	a.applyTransformLive()
	t.End()
	a.commitTransform()
	return nil
}

// rotateOp turns the selection about a world axis through its pivot.
func (r *ScriptRunner) rotateOp(op io.Op) error {
	a := r.App
	axis, ok := parseAxis(op.Axis)
	if !ok {
		return op.Errorf("unknown axis %q", op.Axis)
	}
	a.armTransform()
	t := a.transform.tool
	if t == nil {
		return op.Errorf("nothing is selected to rotate")
	}
	t.Mode = tools.GizmoRotate
	switch axis {
	case geom.AxisX:
		t.Begin(tools.PartRingX, geom.Vec3{}, 0)
	case geom.AxisY:
		t.Begin(tools.PartRingY, geom.Vec3{}, 0)
	default:
		t.Begin(tools.PartRingZ, geom.Vec3{}, 0)
	}
	t.UpdateRotate(op.Degrees, tools.DetentFree)
	a.applyTransformLive()
	t.End()
	a.commitTransform()
	return nil
}

// boxSelectOp runs a box select over a rectangle in window pixels.
func (r *ScriptRunner) boxSelectOp(op io.Op, vp render.Viewport) error {
	a := r.App
	switch op.Kind {
	case "", "vert", "verts":
		a.box.Filter = render.PickVert
	case "edge", "edges":
		a.box.Filter = render.PickEdge
	case "face", "faces":
		a.box.Filter = render.PickFace
	default:
		return op.Errorf("unknown box filter %q", op.Kind)
	}
	a.box.rect = render.BoxRect{
		X0: op.Rect[0], Y0: op.Rect[1], X1: op.Rect[2], Y1: op.Rect[3],
	}
	in := r.frame()
	in.Shift, in.Ctrl = op.Shift, op.Ctrl
	a.applyBoxSelect(in, vp)
	a.armTransform()
	return nil
}

// faceByIndex resolves a script's face reference.
//
// Identities are minted at run time, so a script cannot name one. It can name a
// position — `face: 3` — but positions move every time a boolean rebuilds the
// body, which makes a script that edits twice unwritable. So it can also name a
// direction — `axis: "+y"` — which picks the outermost face pointing that way
// and keeps meaning the same thing after the shape underneath it changes.
func (r *ScriptRunner) faceByIndex(op io.Op) (faceRef, error) {
	b := r.App.Doc().BodyByName(op.Body)
	if b == nil || b.Mesh == nil {
		return faceRef{}, op.Errorf("no body named %q", op.Body)
	}
	if op.Axis != "" {
		return faceByAxis(b, op)
	}
	if op.Face < 0 || op.Face >= len(b.Mesh.Faces) {
		return faceRef{}, op.Errorf("body %q has %d faces, not one at index %d",
			op.Body, len(b.Mesh.Faces), op.Face)
	}
	return faceRef{body: b, face: op.Face, uid: b.Mesh.Faces[op.Face].ID}, nil
}

// faceByAxis picks the outermost face pointing along a named world axis.
func faceByAxis(b *model.Body, op io.Op) (faceRef, error) {
	dir, ok := parseAxis(op.Axis)
	if !ok {
		return faceRef{}, op.Errorf("unknown axis %q, want one of +x -x +y -y +z -z", op.Axis)
	}
	best, bestReach := -1, 0.0
	for i := range b.Mesh.Faces {
		if b.Mesh.FaceNormal(i).Dot(dir) < 0.999 {
			continue
		}
		// Among parallel faces, the one furthest out along the axis is the one
		// a person would have clicked.
		reach := b.Mesh.FaceCentroid(i).Dot(dir)
		if best < 0 || reach > bestReach {
			best, bestReach = i, reach
		}
	}
	if best < 0 {
		return faceRef{}, op.Errorf("body %q has no face pointing %s", op.Body, op.Axis)
	}
	return faceRef{body: b, face: best, uid: b.Mesh.Faces[best].ID}, nil
}

func parseAxis(s string) (geom.Vec3, bool) {
	switch s {
	case "+x":
		return geom.AxisX, true
	case "-x":
		return geom.AxisX.Neg(), true
	case "+y":
		return geom.AxisY, true
	case "-y":
		return geom.AxisY.Neg(), true
	case "+z":
		return geom.AxisZ, true
	case "-z":
		return geom.AxisZ.Neg(), true
	}
	return geom.Vec3{}, false
}

// booleanOp runs a scripted boolean through the same tool and command the UI
// uses, so a script and a session of clicking are the same code path.
func (r *ScriptRunner) booleanOp(op io.Op, commit bool) error {
	a := r.App
	if !a.InBoolean() && !a.BeginBoolean() {
		return op.Errorf("could not open the boolean tool")
	}
	t := a.boolean.tool
	t.Clear()

	switch op.Kind {
	case "", "union", "add":
		t.Op = csggeom.Union
	case "subtract":
		t.Op = csggeom.Subtract
	case "intersect":
		t.Op = csggeom.Intersect
	default:
		return op.Errorf("unknown boolean %q", op.Kind)
	}
	t.KeepTools = op.Visible != nil && *op.Visible

	target := a.Doc().BodyByName(op.Target)
	if target == nil {
		return op.Errorf("no body named %q to keep", op.Target)
	}
	t.Pick(target.ID)
	for _, name := range op.Tools {
		b := a.Doc().BodyByName(name)
		if b == nil {
			return op.Errorf("no body named %q to combine", name)
		}
		t.Pick(b.ID)
	}
	if !commit {
		return nil
	}
	if !a.CommitBoolean() {
		return op.Errorf("the boolean was refused")
	}
	return nil
}

// vec converts an op's world-unit point into sketch subunits.
func vec(p *[2]float64) geom.Vec2i {
	if p == nil {
		return geom.Vec2i{}
	}
	return geom.Vec2i{X: geom.ToSubunits(p[0]), Y: geom.ToSubunits(p[1])}
}

func parseSketchTool(s string) (sketch.Tool, bool) {
	switch s {
	case "select":
		return sketch.ToolSelect, true
	case "line":
		return sketch.ToolLine, true
	case "rect":
		return sketch.ToolRect, true
	case "circle":
		return sketch.ToolCircle, true
	}
	return 0, false
}

// addEntity commits a scripted entity through the same command the UI uses.
func (r *ScriptRunner) addEntity(op io.Op, e model.Entity) error {
	s := r.App.ActiveSketch()
	if s == nil {
		return op.Errorf("no sketch is being edited — call sketch.begin first")
	}
	if err := r.App.Bus.Run(&model.AddEntity{Sketch: s.ID, Entity: e}); err != nil {
		return op.Wrap(err)
	}
	return nil
}

// dump prints the document and selection state as machine-readable lines. Flow
// tests assert on these rather than on pixels, so a behavioural regression is
// reported as a behaviour, not as a picture that changed.
func (r *ScriptRunner) dump() {
	a := r.App
	doc := a.Doc()

	planes := make([]string, 0, geom.PlaneCount)
	for i := 0; i < geom.PlaneCount; i++ {
		k := geom.PlaneKind(i)
		planes = append(planes, fmt.Sprintf("%s:%d", k, boolBit(doc.PlaneVisible(k))))
	}
	fmt.Printf("doc planes=%s bodies=%d sketches=%d undo=%d redo=%d dirty=%d\n",
		strings.Join(planes, ","), len(doc.Bodies), len(doc.Sketches),
		a.Bus.UndoDepth(), a.Bus.RedoDepth(), boolBit(doc.DirtySinceSave))

	for _, b := range doc.Bodies {
		fmt.Printf("body id=%d name=%q visible=%d tris=%d vol=%.4f color=%02X%02X%02X\n",
			b.ID, b.Name, boolBit(b.Visible), b.TriangleCount(), bodyVolume(b),
			b.Color.R, b.Color.G, b.Color.B)
	}
	for _, s := range doc.Sketches {
		fmt.Printf("sketch id=%d name=%q visible=%d\n", s.ID, s.Name, boolBit(s.Visible))
	}
	if sk := a.ActiveSketch(); sk != nil {
		arr := sk.Arrangement()
		fmt.Printf("sketch active=%q entities=%d regions=%d openends=%d tool=%q\n",
			sk.Name, len(sk.Entities), len(arr.Regions), len(arr.OpenEnds),
			a.sketch.session.Tool.String())
	}
	if t := a.extrude.tool; t != nil {
		fmt.Printf("extrude depth=%.4f draft=%.2f achieved=%.2f clamped=%d dir=%q "+
			"through=%d regions=%d result=%q targets=%d\n",
			t.EffectiveDepth(), t.Draft, t.AchievedDraft, boolBit(t.Clamped),
			t.Dir.String(), boolBit(t.ThroughAll), len(t.Regions),
			t.Result.String(), len(a.extrudeTargets(t.Result)))
	}
	if t := a.boolean.tool; t != nil {
		fmt.Printf("boolean op=%q target=%d tools=%d keep=%d\n",
			t.Op.String(), t.Target, len(t.Tools), boolBit(t.KeepTools))
	}
	if t := a.transform.tool; t != nil {
		fmt.Printf("gizmo mode=%q pivot=%.4f,%.4f,%.4f boxfilter=%q\n",
			t.Mode.String(), t.Pivot.X, t.Pivot.Y, t.Pivot.Z,
			BoxFilterLabel(a.box.Filter))
	}
	if t := a.pushPull.tool; t != nil {
		fmt.Printf("pushpull body=%d dist=%.4f adding=%d\n",
			t.Body, t.DistanceUnits, boolBit(t.Adding()))
	}
	for _, sk := range doc.Sketches {
		if sk.OnFace {
			fmt.Printf("facesketch id=%d body=%d ref=%d\n",
				sk.ID, sk.Body, boolBit(a.faceRefAlive(sk)))
		}
	}
	fmt.Printf("sel count=%d desc=%q\n", a.Sel.Len(), a.Sel.Describe(doc))
	fmt.Printf("hint %q\n", a.HintText())
	for _, t := range a.UI.Toasts() {
		fmt.Printf("toast %q\n", t.Text)
	}
}

// bodyValid reports whether a body is a solid the rest of the program can
// reason about. Every dump carries it so no script can quietly produce, or
// start from, geometry that only looks right.
func bodyValid(b *model.Body) bool {
	return b.Mesh != nil && mesh.Validate(b.Mesh) == nil
}

// bodyVolume is a body's solid volume, or zero when it has no mesh.
func bodyVolume(b *model.Body) float64 {
	if b.Mesh == nil {
		return 0
	}
	return mesh.Volume(b.Mesh)
}

func boolBit(b bool) int {
	if b {
		return 1
	}
	return 0
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
	a.box.init()
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
	// A script has to observe something, or running it proved nothing. A shot
	// and a dump are both observations; a bench run is its own.
	if len(runner.Shots()) == 0 && !runner.dumped && benchFrames == 0 {
		return fmt.Errorf("script observed nothing; add a {\"op\":\"shot\"} or {\"op\":\"dump\"} step")
	}
	for _, s := range runner.Shots() {
		fmt.Println("wrote", s)
	}
	return nil
}
