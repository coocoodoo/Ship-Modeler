package app

import (
	"fmt"
	"image"
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
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/scene"
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
	// held keeps the left button down across ops, so a drag can be paused
	// mid-way and inspected. Every op ends by stepping a frame, and without
	// this that frame would look like a release and finish the drag.
	held bool
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

	case "drag":
		r.mods = mods{shift: op.Shift, ctrl: op.Ctrl, alt: op.Alt}
		// kind "hold" stops with the button still down, so a shot or a dump can
		// catch what a tool looks like mid-drag. Anything a tool only does
		// between press and release is otherwise invisible to a script.
		r.drag(op.From[0], op.From[1], op.To[0], op.To[1], op.Segs, op.Kind == "hold")
		if op.Kind != "hold" {
			r.mods = mods{}
		}

	case "drag.release":
		r.dragRelease()

	case "ui.tree":
		a.tree.collapsed = op.Visible != nil && !*op.Visible

	case "view.ao":
		a.Settings.AO = *op.Strength

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

	case "marker.front", "marker.top", "marker.thruster":
		kind := model.MarkerFront
		switch op.Op {
		case "marker.top":
			kind = model.MarkerTop
		case "marker.thruster":
			kind = model.MarkerThruster
		}
		if op.Dot == nil {
			return op.Errorf("%s needs dot [x,y,z]", op.Op)
		}
		dir := geom.Vec3{Y: 1}
		if op.Normal != nil {
			dir = geom.Vec3{X: op.Normal[0], Y: op.Normal[1], Z: op.Normal[2]}
		}
		if err := a.Bus.Run(&model.PlaceMarker{Marker: model.Marker{
			Kind: kind,
			At:   geom.Vec3{X: op.Dot[0], Y: op.Dot[1], Z: op.Dot[2]},
			Dir:  dir,
		}}); err != nil {
			return op.Errorf("place the marker: %v", err)
		}

	case "marker.move":
		// Moves the selected dots by a delta through the same command the gizmo
		// builds, so a script exercises the real edit rather than a shortcut.
		idx := a.selectedMarkers()
		if len(idx) == 0 {
			return op.Errorf("no dot is selected")
		}
		if err := a.Bus.Run(&model.MoveMarkers{
			Indices: idx,
			Delta:   geom.Vec3{X: op.Delta[0], Y: op.Delta[1], Z: op.Delta[2]},
		}); err != nil {
			return op.Errorf("move the dot: %v", err)
		}

	case "marker.clear":
		for len(a.Doc().Markers) > 0 {
			if err := a.Bus.Run(&model.DeleteMarker{Index: 0}); err != nil {
				return op.Errorf("clear markers: %v", err)
			}
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

	case "sketch.grid":
		if op.Step <= 0 {
			return op.Errorf("sketch.grid needs a positive step")
		}
		a.Settings.GridStep = op.Step

	case "sketch.line":
		if err := r.addEntity(op, model.NewLine(vec(op.From), vec(op.To))); err != nil {
			return err
		}

	case "sketch.point":
		if op.At == nil {
			return op.Errorf("sketch.point needs at [x,y] in sketch units")
		}
		if err := r.addEntity(op, model.NewPoint(vec(op.At))); err != nil {
			return err
		}

	case "sketch.midline":
		// The same arithmetic the tool does: from the midpoint, out to an end.
		if op.C == nil || op.To == nil {
			return op.Errorf("sketch.midline needs c (the midpoint) and to (one end)")
		}
		mid, end := vec(op.C), vec(op.To)
		far := geom.Vec2i{X: 2*mid.X - end.X, Y: 2*mid.Y - end.Y}
		if err := r.addEntity(op, model.NewLine(far, end)); err != nil {
			return err
		}

	case "sketch.centerrect":
		if op.C == nil || op.B == nil {
			return op.Errorf("sketch.centerrect needs c (the centre) and b (a corner)")
		}
		c, corner := vec(op.C), vec(op.B)
		opp := geom.Vec2i{X: 2*c.X - corner.X, Y: 2*c.Y - corner.Y}
		if err := r.addEntity(op, model.NewRect(opp, corner)); err != nil {
			return err
		}

	case "sketch.alignedrect":
		if err := r.alignedRectOp(op); err != nil {
			return err
		}

	case "sketch.circle3":
		if op.A == nil || op.B == nil || op.C == nil {
			return op.Errorf("sketch.circle3 needs three points a, b and c")
		}
		centre, ok := sketch.Circumcentre(vec(op.A), vec(op.B), vec(op.C))
		if !ok {
			return op.Errorf("those three points are in a line")
		}
		segs := op.Segs
		if segs == 0 {
			segs = model.DefaultCircleSegs
		}
		radius := int64(vec(op.A).Sub(centre).Len() + 0.5)
		if err := r.addEntity(op, model.NewCircle(centre, radius, segs)); err != nil {
			return err
		}

	case "sketch.arc":
		if err := r.arcOp(op); err != nil {
			return err
		}

	case "sketch.polygon":
		if op.C == nil || op.A == nil {
			return op.Errorf("sketch.polygon needs c (centre) and a (a corner, or a side midpoint when circumscribed)")
		}
		sides := op.Segs
		if sides == 0 {
			sides = model.DefaultPolygonSides
		}
		var poly model.Entity
		switch op.Kind {
		case "", "inscribed":
			poly = model.NewPolygon(vec(op.C), vec(op.A), sides)
		case "circumscribed":
			poly = model.NewCircumscribedPolygon(vec(op.C), vec(op.A), sides)
		default:
			return op.Errorf("polygon kind %q is not inscribed or circumscribed", op.Kind)
		}
		if err := r.addEntity(op, poly); err != nil {
			return err
		}

	case "sketch.spline":
		if len(op.Pts) < 2 {
			return op.Errorf("sketch.spline needs at least two points in pts")
		}
		segs := op.Segs
		if segs == 0 {
			segs = model.DefaultSplineSegs
		}
		through := make([]geom.Vec2i, len(op.Pts))
		for i := range op.Pts {
			p := op.Pts[i]
			through[i] = vec(&p)
		}
		if err := r.addEntity(op, model.NewSpline(through, op.Closed, segs)); err != nil {
			return err
		}

	case "sketch.bezier":
		if len(op.Pts) != 4 {
			return op.Errorf("sketch.bezier needs exactly four control points in pts")
		}
		segs := op.Segs
		if segs == 0 {
			segs = model.DefaultSplineSegs
		}
		var ctrl [4]geom.Vec2i
		for i := range op.Pts {
			p := op.Pts[i]
			ctrl[i] = vec(&p)
		}
		if err := r.addEntity(op,
			model.NewBezier(ctrl[0], ctrl[1], ctrl[2], ctrl[3], segs)); err != nil {
			return err
		}

	case "sketch.slot":
		if op.A == nil || op.B == nil || op.C == nil {
			return op.Errorf("sketch.slot needs a and b (the two ends) and c (a point across the track)")
		}
		slot, ok := sketch.SlotThrough(vec(op.A), vec(op.B), vec(op.C))
		if !ok {
			return op.Errorf("those points make no slot")
		}
		if err := r.addEntity(op, slot); err != nil {
			return err
		}

	case "sketch.ellipse":
		if op.C == nil || op.A == nil || op.B == nil {
			return op.Errorf("sketch.ellipse needs c (centre), a (long axis) and b (a point across it)")
		}
		segs := op.Segs
		if segs == 0 {
			segs = model.DefaultCircleSegs
		}
		e, ok := sketch.EllipseThrough(vec(op.C), vec(op.A), vec(op.B), segs)
		if !ok {
			return op.Errorf("that makes no ellipse")
		}
		if err := r.addEntity(op, e); err != nil {
			return err
		}

	case "sketch.select":
		// The modify ops all act on a selection, so a script needs a way to
		// make one. Indices into the sketch, which is what the tools use.
		if !a.InSketch() {
			return op.Errorf("no sketch is being edited")
		}
		sk := a.ActiveSketch()
		for _, i := range op.Indices {
			if i < 0 || i >= len(sk.Entities) {
				return op.Errorf("no entity %d in that sketch", i)
			}
		}
		a.sketch.session.Selected = append([]int(nil), op.Indices...)

	case "sketch.fillet":
		if err := r.selectFor(op); err != nil {
			return err
		}
		if !a.FilletSelection(op.R) {
			return op.Errorf("the fillet was refused")
		}

	case "sketch.chamfer":
		if err := r.selectFor(op); err != nil {
			return err
		}
		if !a.ChamferSelection(op.R) {
			return op.Errorf("the chamfer was refused")
		}

	case "sketch.offset":
		if err := r.selectFor(op); err != nil {
			return err
		}
		if !a.OffsetSelection(op.Depth) {
			return op.Errorf("the offset was refused")
		}

	case "sketch.mirror":
		if err := r.selectFor(op); err != nil {
			return err
		}
		axis := MirrorVertical
		switch op.Axis {
		case "", "vertical", "v":
		case "horizontal", "h":
			axis = MirrorHorizontal
		default:
			return op.Errorf("mirror axis %q is not vertical or horizontal", op.Axis)
		}
		if !a.MirrorSelection(axis) {
			return op.Errorf("the mirror was refused")
		}

	case "sketch.pattern":
		if err := r.selectFor(op); err != nil {
			return err
		}
		kind := PatternLinear
		switch op.Kind {
		case "", "linear":
		case "circular":
			kind = PatternCircular
		default:
			return op.Errorf("pattern kind %q is not linear or circular", op.Kind)
		}
		count := op.Segs
		if count == 0 {
			count = 2
		}
		var delta geom.Vec2i
		if op.Delta != nil {
			delta = geom.Vec2i{
				X: geom.ToSubunits(op.Delta[0]),
				Y: geom.ToSubunits(op.Delta[1]),
			}
		}
		sweep := op.Degrees
		if sweep == 0 {
			sweep = 360
		}
		if !a.PatternSelection(kind, count, delta, sweep) {
			return op.Errorf("the pattern was refused")
		}

	case "sketch.construction":
		if err := r.constructionOp(op); err != nil {
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

	case "file.new", "file.save", "file.open", "file.export",
		"file.autosave", "file.recover", "file.discard",
		"file.importmesh", "import.scale", "import.center",
		"import.commit", "import.cancel",
		"export.begin", "export.format", "export.cancel",
		"export.scale", "export.alpha":
		if err := r.fileOp(op); err != nil {
			return err
		}

	case "paint.begin", "paint.exit", "paint.res", "paint.color", "paint.tool",
		"paint.size", "paint.pixel", "paint.stroke", "paint.resample",
		"paint.textures", "paint.faceview", "paint.color2", "paint.swap",
		"paint.dither", "paint.shapefill", "paint.lock", "paint.unlock",
		"paint.edges", "paint.edgewidth", "paint.creases", "paint.pickedge",
		"tile.import", "tile.grid", "tile.select", "tile.orient", "tile.stamp":
		if err := r.paintOp(op); err != nil {
			return err
		}

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
	in.Down[MouseLeft] = r.held
	if r.moved {
		// A nominal delta so hover picking treats this as real motion.
		in.MouseDX, in.MouseDY = 1, 0
		r.moved = false
	}
	return in
}

// drag synthesizes a press, a run of motion and a release, which is the only
// way to exercise a tool that only does anything between the two.
//
// The steps matter. A drag that jumped straight from press to release would
// miss everything that happens per-frame — the hover that has to latch onto a
// handle first, the live preview, the coalescing — and those are exactly the
// places drags go wrong.
func (r *ScriptRunner) drag(x0, y0, x1, y1 float64, steps int, hold bool) {
	if steps <= 0 {
		steps = 8
	}
	// Hover the start first: a handle has to know the pointer is on it before
	// the button goes down, just as it would with a real mouse.
	r.mouse = [2]float64{x0, y0}
	r.moved = true
	r.step()

	down := r.frame()
	down.Pressed[MouseLeft] = true
	down.Down[MouseLeft] = true
	r.runFrame(down)

	prevX, prevY := x0, y0
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + (x1-x0)*t
		y := y0 + (y1-y0)*t
		r.mouse = [2]float64{x, y}
		f := r.frame()
		f.Down[MouseLeft] = true
		f.MouseDX, f.MouseDY = x-prevX, y-prevY
		r.runFrame(f)
		prevX, prevY = x, y
	}

	if hold {
		r.held = true
		return
	}
	up := r.frame()
	up.Released[MouseLeft] = true
	r.runFrame(up)
}

// dragRelease finishes a held drag.
func (r *ScriptRunner) dragRelease() {
	r.held = false
	up := r.frame()
	up.Released[MouseLeft] = true
	r.runFrame(up)
	r.mods = mods{}
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
	case "marker", "dot":
		// Face doubles as the index: a marker has no name of its own, and
		// adding a second index field for one op would be worse.
		if op.Face < 0 || op.Face >= len(a.Doc().Markers) {
			return op.Errorf("no marker %d — the document has %d",
				op.Face, len(a.Doc().Markers))
		}
		a.Sel.Set(model.MarkerRef(op.Face))
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
	// A runner with nowhere to write is building a document rather than
	// capturing one — the in-app sample ship runs the same script.
	if r.OutDir == "" {
		r.step()
		return nil
	}
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
	t.SetThroughAll(op.Through)
	// An unspecified result leaves whatever the tool chose for itself, which is
	// how a face sketch keeps its Add default (SPEC-UX §10).
	switch op.Result {
	case "":
	case "new":
		t.SetResult(tools.ResultNew)
	case "add":
		t.SetResult(tools.ResultAdd)
	case "subtract":
		t.SetResult(tools.ResultSubtract)
	case "intersect":
		t.SetResult(tools.ResultIntersect)
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

// fileOp drives the file layer. The paths are explicit because a headless run
// has nobody to answer a dialog; everything below the dialog is the same code
// the buttons reach.
func (r *ScriptRunner) fileOp(op io.Op) error {
	a := r.App
	switch op.Op {
	case "file.new":
		a.NewDocument()

	case "file.save":
		if !a.saveTo(op.Path) {
			return op.Errorf("the save was refused")
		}

	case "file.open":
		if !a.OpenPath(op.Path) {
			return op.Errorf("the file would not open")
		}

	case "file.export":
		if err := a.ExportTo(op.Path, op.Res, op.Visible != nil && *op.Visible); err != nil {
			return op.Wrap(err)
		}

	case "file.importmesh":
		if op.Path == "" {
			return op.Errorf("file.importmesh needs a path")
		}
		if !a.LoadMeshFile(op.Path) {
			return op.Errorf("the mesh would not read")
		}

	case "import.scale":
		if !a.InImportMesh() {
			return op.Errorf("no mesh is waiting to be imported")
		}
		if op.Scale == nil || *op.Scale <= 0 {
			return op.Errorf("import.scale needs a positive scale")
		}
		a.files.importScale = *op.Scale

	case "import.center":
		if !a.InImportMesh() {
			return op.Errorf("no mesh is waiting to be imported")
		}
		a.files.importCenter = op.Visible == nil || *op.Visible

	case "import.commit":
		if !a.InImportMesh() {
			return op.Errorf("no mesh is waiting to be imported")
		}
		if !a.CommitImport() {
			return op.Errorf("the import was refused")
		}

	case "import.cancel":
		a.CancelImport()

	case "file.autosave":
		a.writeAutosave(op.Kind == "crash")

	case "file.recover":
		a.files.recovery = nil
		if found, err := ioFindRecoverable(); err == nil {
			a.files.recovery = found
		}
		if !a.RecoverNewest() {
			return op.Errorf("there was nothing to recover")
		}

	case "file.discard":
		a.DiscardRecovery()

	case "export.begin":
		a.BeginExport()
		if !a.InExport() {
			return op.Errorf("the export card would not open")
		}

	case "export.cancel":
		a.CancelExport()

	case "export.format":
		found := false
		for i, f := range io.ExportFormats() {
			if strings.EqualFold(f.Extension, "."+op.Kind) {
				a.files.exportFormat, found = i, true
			}
		}
		if !found {
			return op.Errorf("no export format writes .%s", op.Kind)
		}

	case "export.scale":
		a.files.exportScale = op.Res

	case "export.alpha":
		a.files.exportAlpha = *op.Visible
	}
	return nil
}

// ioFindRecoverable is split out so the op reads the same list the app does.
func ioFindRecoverable() ([]io.Autosave, error) { return io.FindRecoverable() }

// paintOp runs the paint ops through the same brush, palette and command the
// pointer drives, so a scripted ship and a painted one are the same code path.
func (r *ScriptRunner) paintOp(op io.Op) error {
	a := r.App
	switch op.Op {
	case "paint.begin":
		if !a.BeginPaint() {
			return op.Errorf("paint mode would not open")
		}

	case "paint.exit":
		if !a.InPaint() {
			return op.Errorf("paint mode is not open")
		}
		a.ExitPaint()

	case "paint.res":
		if !a.SetPaintRes(op.Res) {
			return op.Errorf("%d is not a paint resolution", op.Res)
		}

	case "paint.color", "paint.color2":
		c, ok := paint.ParseColor(op.Hex)
		if !ok {
			return op.Errorf("%q is not an RRGGBB colour", op.Hex)
		}
		// Setting either colour goes through the slot the panel would have to
		// be pointed at first, so a script and a session leave the same recents.
		was := a.paint.slot
		if op.Op == "paint.color2" {
			a.paint.slot = 1
		} else {
			a.paint.slot = 0
		}
		a.setPaintColor(c)
		a.paint.slot = was

	case "paint.swap":
		a.swapPaintColors()

	case "paint.dither":
		d, ok := paint.ParseDither(op.Kind)
		if !ok {
			return op.Errorf("unknown dither mode %q", op.Kind)
		}
		a.paint.dither = d

	case "paint.shapefill":
		a.paint.fillShape = *op.Visible

	case "paint.tool":
		t, ok := paint.ParseTool(op.Kind)
		if !ok {
			return op.Errorf("unknown paint tool %q", op.Kind)
		}
		a.setPaintTool(t)

	case "paint.size":
		if !validBrushSize(op.Size) {
			return op.Errorf("%d is not a brush size", op.Size)
		}
		a.paint.size = op.Size

	case "paint.textures":
		a.paint.hideTextures = !*op.Visible

	case "paint.faceview":
		if !a.FaceView() {
			return op.Errorf("nothing is under the cursor to look at")
		}

	case "paint.edgewidth":
		if op.Size <= 0 {
			return op.Errorf("paint.edgewidth needs a positive width in pixels")
		}
		a.paint.edgeWidth = clampInt(op.Size, MinEdgeWidth, MaxEdgeWidth)

	case "paint.creases":
		// Every sharp edge of the named body, or of everything visible.
		if op.Body != "" {
			b := a.Doc().BodyByName(op.Body)
			if b == nil {
				return op.Errorf("no body named %q", op.Body)
			}
			a.Sel.Set(model.BodyRef(b.ID))
		}
		if !a.SelectBodyCreases() {
			return op.Errorf("no sharp edges to pick")
		}

	case "tile.import":
		if !a.ImportTileset(op.Path) {
			return op.Errorf("the tileset would not load")
		}

	case "tile.grid":
		if !a.SetTileGrid(op.TileW, op.TileH, op.TileMargin, op.TileSpacing) {
			return op.Errorf("the grid was refused")
		}

	case "tile.select":
		if !a.SelectTile(op.Tile) {
			return op.Errorf("no tile %d", op.Tile)
		}

	case "tile.orient":
		a.SetTileOrientation(paint.Orientation{Rot: uint8(op.Rot % 4), FlipX: op.FlipTile})

	case "tile.stamp":
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		if !a.StampTileAt(f.body.ID, f.uid, image.Point{X: op.UV[0], Y: op.UV[1]}, op.Alt) {
			return op.Errorf("the stamp was refused")
		}

	case "paint.pickedge":
		// The click path, minus the pixel hunt: toggling an edge into the
		// tool's selection the same way pickPaintEdge does, chain and all.
		b := a.Doc().BodyByName(op.Body)
		if b == nil {
			return op.Errorf("no body named %q", op.Body)
		}
		a.toggleEdge(b.ID, op.Face)

	case "paint.edges":
		// Indices name edges of the body; with none, whatever is already
		// picked is baked.
		if op.Body != "" && len(op.Indices) > 0 {
			b := a.Doc().BodyByName(op.Body)
			if b == nil {
				return op.Errorf("no body named %q", op.Body)
			}
			a.paint.edges = a.paint.edges[:0]
			for _, e := range op.Indices {
				a.paint.edges = append(a.paint.edges, edgeRef{body: b.ID, edge: e})
			}
		}
		if !a.PaintSelectedEdges() {
			return op.Errorf("the edge paint was refused")
		}

	case "paint.lock":
		// A script names the face rather than arming the pick and clicking it,
		// which is the same entry point with the choice already made. Naming no
		// body arms it instead, so the two-step flow can be driven as well.
		if op.Body == "" {
			a.BeginLockPick()
			break
		}
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		if !a.LockToFace(f.body.ID, f.uid) {
			return op.Errorf("there was no face to lock to")
		}

	case "paint.unlock":
		if !a.paint.locked {
			return op.Errorf("nothing is locked")
		}
		a.UnlockFace()

	case "paint.pixel", "paint.stroke":
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		pts := texelPoints(op)
		if !a.PaintFace(f.body.ID, f.uid, pts) {
			return op.Errorf("the stroke was refused")
		}

	case "paint.resample":
		f, err := r.faceByIndex(op)
		if err != nil {
			return err
		}
		if !a.ResampleFace(f.body.ID, f.uid, op.Res) {
			return op.Errorf("the resample was refused")
		}
	}
	return nil
}

// texelPoints reads a paint op's texel path: one point for a pixel, a run for
// a stroke.
func texelPoints(op io.Op) []image.Point {
	if op.UV != nil {
		return []image.Point{{X: op.UV[0], Y: op.UV[1]}}
	}
	pts := make([]image.Point, 0, len(op.Points))
	for _, p := range op.Points {
		pts = append(pts, image.Point{X: p[0], Y: p[1]})
	}
	return pts
}

func validBrushSize(v int) bool {
	for _, s := range paint.BrushSizes {
		if s == v {
			return true
		}
	}
	return false
}

// vec converts an op's world-unit point into sketch subunits.
func vec(p *[2]float64) geom.Vec2i {
	if p == nil {
		return geom.Vec2i{}
	}
	return geom.Vec2i{X: geom.ToSubunits(p[0]), Y: geom.ToSubunits(p[1])}
}

// parseSketchTool names every tool a script can arm.
//
// It is built from AllTools rather than a hand-kept switch, which is what
// stopped it naming only M2's four while a dozen more shipped past it. The
// names are the tools' own, lowercased with the spaces taken out, so
// "centre rectangle" is "centrerectangle" and adding a tool needs nothing
// here at all.
func parseSketchTool(name string) (sketch.Tool, bool) {
	want := normalizeToolName(name)
	if want == "" {
		return 0, false
	}
	// The short names the older scripts use, kept working.
	switch want {
	case "rect":
		return sketch.ToolRect, true
	case "point":
		return sketch.ToolPoint, true
	}
	for _, t := range sketch.AllTools() {
		if normalizeToolName(t.String()) == want {
			return t, true
		}
	}
	return 0, false
}

// normalizeToolName folds a tool name to its comparable form.
func normalizeToolName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		}
	}
	return string(out)
}

// addEntity commits a scripted entity through the same command the UI uses.
func (r *ScriptRunner) addEntity(op io.Op, e model.Entity) error {
	s := r.App.ActiveSketch()
	if s == nil {
		return op.Errorf("no sketch is being edited — call sketch.begin first")
	}
	// A script gets the armed construction mode too, so `sketch.construction`
	// followed by a draw op does what the Q key followed by a click does.
	e.Construction = r.App.sketch.construction
	if err := r.App.Bus.Run(&model.AddEntity{Sketch: s.ID, Entity: e}); err != nil {
		return op.Wrap(err)
	}
	return nil
}

// alignedRectOp draws a rectangle at an angle from a base edge and a height
// point, through the same arithmetic the interactive tool uses.
func (r *ScriptRunner) alignedRectOp(op io.Op) error {
	s := r.App.ActiveSketch()
	if s == nil {
		return op.Errorf("no sketch is being edited — call sketch.begin first")
	}
	if op.A == nil || op.B == nil || op.C == nil {
		return op.Errorf("sketch.alignedrect needs a and b (the base edge) and c (a height point)")
	}
	lines, ok := sketch.AlignedRectLines(vec(op.A), vec(op.B), vec(op.C))
	if !ok {
		return op.Errorf("that base edge and height make no rectangle")
	}
	if r.App.sketch.construction {
		for i := range lines {
			lines[i].Construction = true
		}
	}
	if err := r.App.Bus.Run(&model.ReplaceEntities{
		Sketch: s.ID, Add: lines, Label: "Draw aligned rectangle",
	}); err != nil {
		return op.Wrap(err)
	}
	return nil
}

// arcOp draws an arc by whichever of the three gestures the script names,
// through the same construction the interactive tools use.
func (r *ScriptRunner) arcOp(op io.Op) error {
	segs := op.Segs
	if segs == 0 {
		segs = model.DefaultCircleSegs
	}
	var (
		e  model.Entity
		ok bool
	)
	switch op.Kind {
	case "", "center":
		if op.C == nil || op.From == nil || op.To == nil {
			return op.Errorf("a centre arc needs c (centre), from (start) and to (a point it sweeps toward)")
		}
		e, ok = sketch.ArcToward(vec(op.C), vec(op.From), vec(op.To), segs)
	case "points":
		if op.From == nil || op.To == nil || op.A == nil {
			return op.Errorf("a 3 point arc needs from, to and a (a point on the way)")
		}
		e, ok = sketch.ArcThrough(vec(op.From), vec(op.A), vec(op.To), segs)
	case "tangent":
		if op.From == nil || op.To == nil {
			return op.Errorf("a tangent arc needs from (a loose endpoint) and to (the far end)")
		}
		s := r.App.ActiveSketch()
		if s == nil {
			return op.Errorf("no sketch is being edited — call sketch.begin first")
		}
		at, dir, found := sketch.EndpointNear(vec(op.From), s.Entities, geom.SubunitsPerUnit)
		if !found {
			return op.Errorf("no loose endpoint near from — a tangent arc needs one")
		}
		e, ok = sketch.ArcTangent(at, dir, vec(op.To), segs)
	default:
		return op.Errorf("arc kind %q is not center, points or tangent", op.Kind)
	}
	if !ok {
		return op.Errorf("those points make no arc")
	}
	return r.addEntity(op, e)
}

// selectFor points the session's selection at the entities an op names, so a
// modify op can be one line in a script instead of two.
func (r *ScriptRunner) selectFor(op io.Op) error {
	if !r.App.InSketch() {
		return op.Errorf("no sketch is being edited — call sketch.begin first")
	}
	if len(op.Indices) == 0 {
		return nil // act on whatever is already selected
	}
	s := r.App.ActiveSketch()
	for _, i := range op.Indices {
		if i < 0 || i >= len(s.Entities) {
			return op.Errorf("no entity %d in that sketch", i)
		}
	}
	r.App.sketch.session.Selected = append([]int(nil), op.Indices...)
	return nil
}

// constructionOp arms the construction mode, or converts named entities.
func (r *ScriptRunner) constructionOp(op io.Op) error {
	s := r.App.ActiveSketch()
	if s == nil {
		return op.Errorf("no sketch is being edited — call sketch.begin first")
	}
	on := true
	if op.On != nil {
		on = *op.On
	}
	if len(op.Indices) == 0 {
		// No indices: arm the mode for whatever is drawn next, which is what Q
		// with nothing selected does.
		r.App.sketch.construction = on
		return nil
	}
	if err := r.App.Bus.Run(&model.SetConstruction{
		Sketch: s.ID, Indices: op.Indices, On: on,
	}); err != nil {
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

	// The camera's forward direction, which is the only way to assert that a
	// view faces what it was asked to face rather than the back of it.
	// How many default planes the scene is actually drawing, which is how a
	// test says "they got out of the way" without comparing pixels.
	fmt.Printf("planes drawn=%d\n", len(a.BuildScene().Planes))

	f := a.targetCamera().Forward()
	fmt.Printf("camera forward=%.4f,%.4f,%.4f ortho=%d\n",
		f.X, f.Y, f.Z, boolBit(!a.Camera.Perspective))

	for _, b := range doc.Bodies {
		// faces says what folding did to the topology, which tris cannot: a
		// quad split into two triangles draws the same two triangles it
		// always did. valid was on this line once, and the whole-corpus
		// invariant test quietly skipped every script while it was gone —
		// its regex found nothing to check.
		fmt.Printf("body id=%d name=%q visible=%d tris=%d vol=%.4f color=%02X%02X%02X faces=%d edges=%d valid=%d\n",
			b.ID, b.Name, boolBit(b.Visible), b.TriangleCount(), bodyVolume(b),
			b.Color.R, b.Color.G, b.Color.B, len(b.Mesh.Faces),
			len(b.Mesh.DrawnEdges()), boolBit(bodyValid(b)))
	}
	for _, s := range doc.Sketches {
		fmt.Printf("sketch id=%d name=%q visible=%d\n", s.ID, s.Name, boolBit(s.Visible))
	}
	if sk := a.ActiveSketch(); sk != nil {
		arr := sk.Arrangement()
		cx := 0
		for i := range sk.Entities {
			if sk.Entities[i].Construction {
				cx++
			}
		}
		fmt.Printf("sketch active=%q entities=%d construction=%d regions=%d openends=%d tool=%q\n",
			sk.Name, len(sk.Entities), cx, len(arr.Regions), len(arr.OpenEnds),
			a.sketch.session.Tool.String())
	}
	if t := a.extrude.tool; t != nil {
		fmt.Printf("extrude depth=%.4f draft=%.2f achieved=%.2f clamped=%d dir=%q "+
			"through=%d regions=%d result=%q targets=%d reach=%d\n",
			t.EffectiveDepth(), t.Draft, t.AchievedDraft, boolBit(t.Clamped),
			t.Dir.String(), boolBit(t.ThroughAll), len(t.Regions),
			t.Result.String(), len(a.extrudeTargets(t.Result)),
			// reach is what the Result chips are enabled from: the bodies the
			// pending solid runs into, whatever result is armed. targets is
			// narrower — it is empty for New, which says nothing about reach.
			len(a.extrude.targets))
	}
	if t := a.boolean.tool; t != nil {
		fmt.Printf("boolean op=%q target=%d tools=%d keep=%d\n",
			t.Op.String(), t.Target, len(t.Tools), boolBit(t.KeepTools))
	}
	if t := a.transform.tool; t != nil {
		fmt.Printf("gizmo mode=%q pivot=%.4f,%.4f,%.4f\n",
			t.Mode.String(), t.Pivot.X, t.Pivot.Y, t.Pivot.Z)
	}
	// The box filter is app state in its own right, not part of the gizmo. It
	// was reported on the gizmo's line until a face selection stopped arming
	// one and took the filter with it.
	fmt.Printf("boxselect filter=%q\n", BoxFilterLabel(a.box.Filter))
	if a.InExport() {
		f := io.ExportFormats()[a.files.exportFormat]
		fmt.Printf("export format=%q scale=%d alpha=%d\n",
			f.Extension, a.files.exportScale, boolBit(a.files.exportAlpha))
	}
	fmt.Printf("file name=%q saved=%d readonly=%d recovery=%d recents=%d\n",
		a.DocumentName(), boolBit(a.files.path != ""), boolBit(a.files.readOnly),
		len(a.files.recovery), len(a.Settings.RecentFiles))
	if t := a.pushPull.tool; t != nil {
		// The arrow's screen ends are reported so a scripted drag can aim at the
		// same pixels a person would. Without them a drag test has to guess,
		// and a test that guesses at coordinates is a test that passes by
		// accident.
		vp := a.Viewport(r.Size.W, r.Size.H)
		ax, ay, bx, by, _ := scene.ArrowScreenEnds(
			a.Camera, vp, t.Origin, t.ArrowDirection(), a.arrowLength(vp))
		fmt.Printf("pushpull body=%d dist=%.4f adding=%d arrow=%.0f,%.0f,%.0f,%.0f\n",
			t.Body, t.DistanceUnits, boolBit(t.Adding()), ax, ay, bx, by)
	}
	for _, sk := range doc.Sketches {
		if sk.OnFace {
			fmt.Printf("facesketch id=%d body=%d ref=%d\n",
				sk.ID, sk.Body, boolBit(a.faceRefAlive(sk)))
		}
	}
	r.dumpPaint()
	// Where every dot is, and whether it is selected or hovered. A dot is a
	// few pixels in a shot, so a golden cannot tell a moved one from a still
	// one; these numbers can.
	for i, m := range doc.Markers {
		// The dot's screen pixel comes along, for the same reason the push/pull
		// arrow's does: a scripted click has to aim where a person would, and a
		// test that guesses at coordinates passes by accident.
		vp := a.Viewport(r.Size.W, r.Size.H)
		px, _ := a.Camera.WorldToViewport(m.At, float64(vp.W), float64(vp.H))
		fmt.Printf("marker %d kind=%q at=%.4f,%.4f,%.4f dir=%.4f,%.4f,%.4f sel=%d hover=%d px=%.0f,%.0f\n",
			i, m.Kind.String(), m.At.X, m.At.Y, m.At.Z, m.Dir.X, m.Dir.Y, m.Dir.Z,
			boolBit(a.Sel.Contains(model.MarkerRef(i))), boolBit(a.markers.hover == i),
			px.X+float64(vp.X), px.Y+float64(vp.Y))
	}
	fmt.Printf("sel count=%d desc=%q\n", a.Sel.Len(), a.Sel.Describe(doc))
	fmt.Printf("hint %q\n", a.HintText())
	for _, t := range a.UI.Toasts() {
		fmt.Printf("toast %q\n", t.Text)
	}
}

// stickyTargetSeq is the face the panel's controls would act on right now, or
// zero when they would be disabled. It is dumped because the bug it guards
// against is invisible in a screenshot: a button that is enabled while you look
// at it and disabled by the time the pointer arrives.
func stickyTargetSeq(a *App) uint32 {
	if h, ok := a.stickyFace(); ok {
		return h.face.Seq()
	}
	return 0
}

// dumpPaint reports the brush and every painted face, which is how a flow test
// asserts that paint went where it was aimed and stayed there.
//
// Texels are counted rather than compared as pixels: a picture that survived a
// boolean with the right number of opaque texels in the right world positions
// is the claim SPEC-GEOMETRY §8.4 actually makes, and it is one a screenshot
// cannot make for you.
func (r *ScriptRunner) dumpPaint() {
	a := r.App
	st := &a.paint
	fmt.Printf("paint mode=%d tool=%q size=%d res=%d color=%q color2=%q "+
		"dither=%q fill=%d slot=%d textures=%d locked=%d lockface=%d target=%d "+
		"awaitlock=%d\n",
		boolBit(a.InPaint()), st.tool.String(), st.size, st.res,
		paint.Hex(st.color), paint.Hex(st.colorB), st.dither.String(),
		boolBit(st.fillShape), st.slot, boolBit(!st.hideTextures),
		boolBit(st.locked), st.lockFace.Seq(), stickyTargetSeq(a),
		boolBit(st.awaitingLock))

	if on, res := a.paintResMismatch(); on {
		fmt.Printf("resprompt offer=%d armed=%d\n", res, st.res)
	}

	if ts := st.tiles.set; ts != nil {
		fmt.Printf("tileset sheet=%dx%d grid=%dx%d+%d+%d tiles=%d sel=%d rot=%d flip=%d\n",
			ts.Img.Bounds().Dx(), ts.Img.Bounds().Dy(),
			ts.TileW, ts.TileH, ts.Margin, ts.Spacing,
			ts.Count(), st.tiles.sel, st.tiles.orient.Rot, boolBit(st.tiles.orient.FlipX))
	}

	if h := st.hover; h.ok && h.paint != nil {
		fmt.Printf("painthover body=%d face=%d texel=%d,%d res=%d allocated=%d oblique=%.1f\n",
			h.body, h.face.Seq(), h.texel.X, h.texel.Y, h.paint.Res,
			boolBit(h.allocated), h.obliqueDeg)
	}

	for _, b := range a.Doc().Bodies {
		if b.Mesh == nil {
			continue
		}
		// One line per distinct picture, not per face: fragments of a cut face
		// share theirs, and that sharing is the contract under test.
		seen := map[*mesh.FacePaint]bool{}
		for fi := range b.Mesh.Faces {
			p := b.Mesh.Faces[fi].Paint
			if p == nil || seen[p] {
				continue
			}
			seen[p] = true
			bounds := p.TexelBounds()
			fmt.Printf("facepaint body=%d face=%d faces=%d res=%d texel=%.8f "+
				"rect=%d,%d,%d,%d opaque=%d sum=%08x\n",
				b.ID, b.Mesh.Faces[fi].ID.Seq(), facesSharing(b.Mesh, p), p.Res, p.Texel,
				bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y,
				opaqueTexels(p), pictureSum(p))
		}
	}
}

// facesSharing counts how many of a body's faces read from one picture.
func facesSharing(m *mesh.Mesh, p *mesh.FacePaint) int {
	n := 0
	for fi := range m.Faces {
		if m.Faces[fi].Paint == p {
			n++
		}
	}
	return n
}

// pictureSum is a hash of a picture's pixels, so a test can say "this is the
// same picture as before" without shipping the picture.
//
// A count of painted texels cannot answer that: a stroke that recolours texels
// that were already painted leaves the count exactly where it was, which is
// most of what an undo has to put back.
func pictureSum(p *mesh.FacePaint) uint32 {
	if p == nil || p.Img == nil {
		return 0
	}
	// FNV-1a over the pixels, plus the origin, because a picture that grew is
	// not the same picture even if its bytes match.
	const offset, prime = uint32(2166136261), uint32(16777619)
	sum := offset
	eat := func(b byte) { sum = (sum ^ uint32(b)) * prime }
	for _, v := range [2]int{p.Off.X, p.Off.Y} {
		for shift := 0; shift < 32; shift += 8 {
			eat(byte(uint32(v) >> shift))
		}
	}
	for _, b := range p.Img.Pix {
		eat(b)
	}
	return sum
}

// opaqueTexels is how many texels of a picture have been painted.
func opaqueTexels(p *mesh.FacePaint) int {
	if p == nil || p.Img == nil {
		return 0
	}
	n := 0
	for i := 3; i < len(p.Img.Pix); i += 4 {
		if p.Img.Pix[i] != 0 {
			n++
		}
	}
	return n
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
