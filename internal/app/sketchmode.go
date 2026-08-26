package app

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/sketch"
	"modeler/internal/ui"
)

// Sketch mode (SPEC-UX §8): entering on a plane, the drawing tools, the live
// region fill, and leaving with the work kept.

// SketchDimFactor is how far the rest of the model fades while a sketch is
// being edited (SPEC-UX §8.1).
const SketchDimFactor = 0.3

// sketchState is the app's half of sketch mode: the session, plus what the
// pointer is over this frame.
type sketchState struct {
	session *sketch.Session
	// snap is this frame's resolved cursor.
	snap    sketch.Snap
	hasSnap bool
	// hoverRegion is the region under the cursor, or -1.
	hoverRegion int
	// selectedRegions are the regions picked for extrude (M3 consumes these).
	selectedRegions map[int]bool
	// returnCamera is where the camera was before entering, so leaving can put
	// it back (SPEC-UX §8.7).
	returnCamera render.Camera
	// pendingPlane is set when S was pressed and the app is waiting for a plane
	// click (SPEC-UX §8.1).
	awaitingPlane bool
}

// InSketch reports whether sketch mode is active.
func (a *App) InSketch() bool { return a.Mode == ModeSketch && a.sketch.session != nil }

// ActiveSketch returns the sketch being edited, or nil.
func (a *App) ActiveSketch() *model.Sketch {
	if a.sketch.session == nil {
		return nil
	}
	return a.Doc().SketchByID(a.sketch.session.SketchID)
}

// BeginSketchOnPlane starts a new sketch on a default plane and animates the
// camera to look squarely at it (SPEC-UX §8.1).
func (a *App) BeginSketchOnPlane(p geom.PlaneKind) bool {
	cmd := &model.AddSketch{Plane: p}
	if !a.Run(cmd) {
		return false
	}
	a.enterSketch(cmd.AddedSketch())
	a.Toast(ui.Toast{Text: "Started " + cmd.AddedSketch().Name + " on the " + p.String() + " plane"})
	return true
}

// EditSketch re-enters an existing sketch (SPEC-UX §8.8).
func (a *App) EditSketch(s *model.Sketch) {
	if s == nil {
		return
	}
	a.enterSketch(s)
}

func (a *App) enterSketch(s *model.Sketch) {
	a.sketch.session = sketch.NewSession(s.ID)
	a.sketch.selectedRegions = map[int]bool{}
	a.sketch.hoverRegion = -1
	a.sketch.returnCamera = a.targetCamera()
	a.sketch.awaitingPlane = false
	a.Mode = ModeSketch
	a.Sel.Clear()

	// Look squarely at the plane, framing a comfortable working area.
	frame := s.Frame()
	to := a.targetCamera()
	if s.OnFace {
		// A face has no entry in the named-view table; the way to look at it
		// straight on is to look down its own normal (SPEC-UX §10).
		to.LookAlong(frame.N.Neg())
	} else {
		to.Azimuth, to.Elevation = render.PlaneView(s.Plane)
	}
	to.Target = frame.O
	to.OrthoScale = 24
	a.Anim.Start(a.Camera, to)
}

// ExitSketch leaves sketch mode, keeping whatever was drawn: a sketch is never
// lost by leaving (SPEC-UX §8.7).
func (a *App) ExitSketch(restoreCamera bool) {
	if a.sketch.session == nil {
		return
	}
	s := a.ActiveSketch()
	a.sketch.session = nil
	a.sketch.awaitingPlane = false
	a.Mode = ModeIdle
	if restoreCamera {
		a.Anim.Start(a.Camera, a.sketch.returnCamera)
	}
	if s != nil {
		a.Sel.Set(model.SketchRef(s.ID))
	}
}

// subunitsPerPixel is the sketch-space distance one screen pixel spans, which
// is what makes the snap radii feel identical at every zoom.
func (a *App) subunitsPerPixel(vp render.Viewport) float64 {
	px := a.Camera.PixelsPerWorldUnit(float64(vp.H))
	if px <= 0 {
		return geom.Unit
	}
	return geom.Unit / px
}

// snapConfig builds this frame's snap tolerances.
func (a *App) snapConfig(in InputFrame, vp render.Viewport) sketch.Config {
	c := sketch.DefaultConfig(a.subunitsPerPixel(vp))
	c.GridStep = geom.ToSubunits(a.Settings.GridStep)
	if c.GridStep <= 0 {
		c.GridStep = geom.SubunitsPerUnit
	}
	if in.Ctrl {
		c.GridStep = geom.SubunitsFine
	}
	c.Suppressed = in.Alt
	// The face a face-sketch sits on brings its own corners and edge midpoints
	// to snap to (SPEC-UX §10).
	c.Reference = a.faceOutline2D(a.ActiveSketch())
	return c
}

// cursorInSketchPlane projects the pointer onto the sketch plane and returns it
// in sketch coordinates.
func (a *App) cursorInSketchPlane(in InputFrame, vp render.Viewport) (geom.Vec2i, bool) {
	s := a.ActiveSketch()
	if s == nil || !vp.Contains(int(in.MouseX), int(in.MouseY)) {
		return geom.Vec2i{}, false
	}
	frame := s.Frame()
	origin, dir := a.Camera.Ray(vp.Local(in.MouseX, in.MouseY), float64(vp.W), float64(vp.H))

	denom := dir.Dot(frame.N)
	if denom > -geom.NormalEps && denom < geom.NormalEps {
		return geom.Vec2i{}, false // looking along the plane: no intersection
	}
	t := frame.O.Sub(origin).Dot(frame.N) / denom
	if t < 0 {
		return geom.Vec2i{}, false // the plane is behind the camera
	}
	local := frame.ToLocal(origin.Add(dir.Mul(t)))
	return geom.Vec2iFromUnits(local), true
}

// updateSketch runs one frame of sketch-mode input.
func (a *App) updateSketch(in InputFrame, vp render.Viewport) {
	s := a.ActiveSketch()
	if s == nil {
		a.ExitSketch(false)
		return
	}
	sess := a.sketch.session

	raw, ok := a.cursorInSketchPlane(in, vp)
	a.sketch.hasSnap = ok
	if ok {
		a.sketch.snap = sketch.Resolve(raw, s.Entities, sess.RubberFrom(), a.snapConfig(in, vp))
		a.sketch.hoverRegion = scene.RegionAt(s.Arrangement(), a.sketch.snap.Point)
	} else {
		a.sketch.hoverRegion = -1
	}

	if in.Pressed[MouseLeft] && ok {
		a.handleSketchClick(in, s, sess)
	}
}

// handleSketchKeys implements the sketch-mode shortcuts of SPEC-UX §8.2.
func (a *App) handleSketchKeys(in InputFrame) {
	sess := a.sketch.session
	if in.Ctrl {
		return
	}
	switch {
	case in.KeyPressed(rl.KeyV):
		sess.SetTool(sketch.ToolSelect)
	case in.KeyPressed(rl.KeyL):
		sess.SetTool(sketch.ToolLine)
	case in.KeyPressed(rl.KeyR):
		sess.SetTool(sketch.ToolRect)
	case in.KeyPressed(rl.KeyC):
		sess.SetTool(sketch.ToolCircle)
	}
	if in.KeyPressed(rl.KeyE) {
		// E from a sketch with a region selected goes straight into extrude,
		// which is the golden path of SPEC-UX §8.7.
		a.BeginExtrude()
		return
	}
	if in.KeyPressed(rl.KeyDelete) {
		a.deleteSketchSelection()
	}
	if in.KeyPressed(rl.KeyEscape) {
		switch sess.Escape() {
		case sketch.EscapeExitMode:
			a.ExitSketch(true)
		case sketch.EscapeCancelledDraw:
			a.SetHint("Cancelled")
		}
	}
}

// handleSketchClick routes a click to the active tool.
func (a *App) handleSketchClick(in InputFrame, s *model.Sketch, sess *sketch.Session) {
	p := a.sketch.snap.Point

	if sess.Tool == sketch.ToolSelect {
		a.handleSketchSelectClick(in, s, sess, p)
		return
	}

	res := sess.Click(p)
	if res.Rejected != "" {
		a.Toast(ui.Toast{Text: res.Rejected, Kind: ui.ToastWarn})
		return
	}
	if !res.Commit {
		return
	}
	if !a.Run(&model.AddEntity{Sketch: s.ID, Entity: res.Entity}) {
		sess.CancelDraw()
		return
	}
	if res.ClosedChain {
		a.SetHint("Profile closed")
	}
}

// handleSketchSelectClick picks entities and regions (SPEC-UX §8.5, §8.6).
func (a *App) handleSketchSelectClick(in InputFrame, s *model.Sketch, sess *sketch.Session, p geom.Vec2i) {
	radius := int64(3 * a.subunitsPerPixel(a.layout.RenderViewport()))
	if radius < geom.SubunitsPerUnit/16 {
		radius = geom.SubunitsPerUnit / 16
	}

	if i := sketch.EntityAt(p, s.Entities, radius); i >= 0 {
		switch {
		case in.Shift:
			sess.Selected = appendUnique(sess.Selected, i)
		case in.Ctrl:
			sess.Selected = toggleInt(sess.Selected, i)
		default:
			sess.Selected = []int{i}
		}
		return
	}

	// Clicking a filled region selects it for extrude.
	if r := a.sketch.hoverRegion; r >= 0 {
		if !in.Shift {
			a.sketch.selectedRegions = map[int]bool{}
		}
		a.sketch.selectedRegions[r] = !a.sketch.selectedRegions[r]
		if !a.sketch.selectedRegions[r] {
			delete(a.sketch.selectedRegions, r)
		}
		return
	}

	if !in.Shift && !in.Ctrl {
		sess.ClearSelection()
		a.sketch.selectedRegions = map[int]bool{}
	}
}

func (a *App) deleteSketchSelection() {
	sess := a.sketch.session
	s := a.ActiveSketch()
	if s == nil || len(sess.Selected) == 0 {
		return
	}
	idx := append([]int(nil), sess.Selected...)
	if a.Run(&model.DeleteEntities{Sketch: s.ID, Indices: idx}) {
		sess.ClearSelection()
		a.sketch.selectedRegions = map[int]bool{}
	}
}

func appendUnique(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func toggleInt(s []int, v int) []int {
	for i, x := range s {
		if x == v {
			return append(s[:i], s[i+1:]...)
		}
	}
	return append(s, v)
}

// buildSketchDraws assembles an overlay for every visible sketch.
//
// A sketch does not stop existing when you finish drawing it: it stays in the
// document, its tree row keeps an eye toggle, and it has to keep showing in the
// viewport or that toggle controls nothing. The one being edited is built last
// so it lands on top of the rest.
func (a *App) buildSketchDraws() []*render.Overlay {
	active := a.ActiveSketch()
	var out []*render.Overlay

	for _, s := range a.Doc().Sketches {
		if !s.Visible || s == active {
			continue
		}
		if d := scene.BuildSketchDraw(scene.SketchView{Sketch: s}); d != nil && !d.Empty() {
			out = append(out, d)
		}
	}

	if active != nil {
		out = append(out, scene.BuildSketchDraw(scene.SketchView{
			Sketch:          active,
			Editing:         true,
			Session:         a.sketch.session,
			Snap:            a.sketch.snap,
			HasSnap:         a.sketch.hasSnap,
			Cursor:          a.sketch.snap.Point,
			HoverRegion:     a.sketch.hoverRegion,
			SelectedRegions: a.sketch.selectedRegions,
			Reference:       a.faceOutline2D(active),
		}))
	}
	return out
}

// sketchHint is what the hint bar says in sketch mode.
func (a *App) sketchHint() string {
	sess := a.sketch.session
	s := a.ActiveSketch()
	if sess == nil || s == nil {
		return ""
	}
	if n := s.OpenEndCount(); n > 0 && sess.Tool == sketch.ToolSelect {
		return fmt.Sprintf("%s · close the %s to make a region you can extrude",
			sess.Tool.Hint(), plural(n, "red endpoint", "red endpoints"))
	}
	return sess.Tool.Hint()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// SelectedRegionCount is how many regions are picked, which M3's extrude gate
// reads.
func (a *App) SelectedRegionCount() int { return len(a.sketch.selectedRegions) }
