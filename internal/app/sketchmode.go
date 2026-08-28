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
	// construction arms the construction flag on everything drawn next, which
	// is what Q toggles with nothing selected (SPEC-UX §8.9).
	construction bool
	// groupPick remembers which variant each toolbar group last used, so the
	// group button re-arms what you chose rather than resetting to the first.
	groupPick map[sketch.ToolGroup]sketch.Tool
	// flyoutOpen is the group whose variant list is showing, if any.
	flyoutOpen sketch.ToolGroup
	flyoutUp   bool
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
	// Exactly one mode is active (SPEC-UX §1), so anything else running steps
	// aside — including a paint stroke, which would otherwise still be holding
	// the bus's pending command when the sketch's first edit arrives.
	a.ExitPaint()
	a.sketch.session = sketch.NewSession(s.ID)
	a.sketch.selectedRegions = map[int]bool{}
	a.sketch.groupPick = map[sketch.ToolGroup]sketch.Tool{}
	a.sketch.construction = false
	a.sketch.flyoutUp = false
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
		// straight on is to put the eye on the outward side and look back down
		// the normal. LookAlong takes the direction the eye sits in, so it is
		// the normal itself — negating it puts the camera inside the body,
		// looking at the back of the face you asked to sketch on.
		to.LookAlong(frame.N)
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

// gridStep is the sketch grid's spacing in units: the setting when it is
// sane, one unit when it is not. Snapping and the drawn grid both read this,
// so what the eye lands on and what the point lands on are always the same
// lines.
func (a *App) gridStep() float64 {
	if s := a.Settings.GridStep; s > 0 {
		return s
	}
	return 1
}

// SketchGridSteps are the spacings the card offers, in units.
var SketchGridSteps = []float64{0.25, 0.5, 1, 2}

// snapConfig builds this frame's snap tolerances.
func (a *App) snapConfig(in InputFrame, vp render.Viewport) sketch.Config {
	c := sketch.DefaultConfig(a.subunitsPerPixel(vp))
	c.GridStep = geom.ToSubunits(a.gridStep())
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
	if s == nil {
		return geom.Vec2i{}, false
	}
	return a.cursorInPlaneOf(s, in.MouseX, in.MouseY, vp)
}

// cursorInPlaneOf maps a window position into one sketch's own coordinates.
func (a *App) cursorInPlaneOf(s *model.Sketch, mouseX, mouseY float64, vp render.Viewport) (geom.Vec2i, bool) {
	if s == nil || !vp.Contains(int(mouseX), int(mouseY)) {
		return geom.Vec2i{}, false
	}
	frame := s.Frame()
	origin, dir := a.Camera.Ray(vp.Local(mouseX, mouseY), float64(vp.W), float64(vp.H))

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

// sketchAt finds the visible sketch under a window position, if any.
//
// A finished sketch is drawn but is not in the pick pass — it is an overlay,
// not geometry — so clicking one used to select whatever was behind it, which
// on the plane it was drawn on is that plane. Since selecting a sketch is how
// you get to extrude it without going to the tree, that made a closed profile
// look unselectable.
//
// The test is done in the sketch's own plane rather than by rendering ids:
// exact, cheap, and the same point-in-region code the sketch-mode hover uses.
// The nearest sketch to the camera wins, so two overlapping sketches resolve
// the way they look.
func (a *App) sketchAt(mouseX, mouseY float64, vp render.Viewport) *model.Sketch {
	var best *model.Sketch
	var bestDist float64
	eye := a.Camera.Eye()

	for _, s := range a.Doc().Sketches {
		if !s.Visible || len(s.Entities) == 0 {
			continue
		}
		p, ok := a.cursorInPlaneOf(s, mouseX, mouseY, vp)
		if !ok {
			continue
		}
		arr := s.Arrangement()
		if scene.RegionAt(arr, p) < 0 && !a.nearSketchStroke(s, p, vp) {
			continue
		}
		d := s.Frame().ToWorld(geom.Vec2{
			X: float64(p.X) / geom.Unit, Y: float64(p.Y) / geom.Unit,
		}).Sub(eye).LenSq()
		if best == nil || d < bestDist {
			best, bestDist = s, d
		}
	}
	return best
}

// nearSketchStroke reports whether a point is within grabbing distance of one
// of a sketch's lines, so an open profile — which has no region to click
// inside — is still selectable.
func (a *App) nearSketchStroke(s *model.Sketch, p geom.Vec2i, vp render.Viewport) bool {
	// The same forgiveness the Select tool gives inside sketch mode.
	radius := int64(SketchStrokePickPx * a.subunitsPerPixel(vp))
	if radius < geom.SubunitsPerUnit/16 {
		radius = geom.SubunitsPerUnit / 16
	}
	return sketch.EntityAt(p, s.Entities, radius) >= 0
}

// updateSketch runs one frame of sketch-mode input.
func (a *App) updateSketch(in InputFrame, vp render.Viewport) {
	s := a.ActiveSketch()
	if s == nil {
		a.ExitSketch(false)
		return
	}
	sess := a.sketch.session

	// The view cube is not the sketch plane. Without this, clicking a zone to
	// turn the camera also put a point down through it.
	if a.cubeOwnsPointer(in) {
		a.sketch.hasSnap = false
		a.sketch.hoverRegion = -1
		return
	}

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
		a.cycleToolGroup(sketch.GroupLine)
	case in.KeyPressed(rl.KeyR):
		a.cycleToolGroup(sketch.GroupRect)
	case in.KeyPressed(rl.KeyC):
		a.cycleToolGroup(sketch.GroupCircle)
	case in.KeyPressed(rl.KeyPeriod):
		a.cycleToolGroup(sketch.GroupPoint)
	case in.KeyPressed(rl.KeyQ):
		a.toggleConstruction()
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

// cycleToolGroup is what a group's key does: arm the group, and press it again
// to step to the next tool in it.
//
// One key per group rather than per tool, because the groups already exist in
// the toolbar and a user who wants the midpoint line reaches for "the line
// key" — the variants are a refinement of one idea, not eight separate ones
// competing for letters (Sketch_func.md §4.1).
func (a *App) cycleToolGroup(g sketch.ToolGroup) {
	sess := a.sketch.session
	if sess == nil {
		return
	}
	members := g.Tools()
	if len(members) == 0 {
		return
	}
	next := members[0]
	for i, t := range members {
		if t == sess.Tool {
			next = members[(i+1)%len(members)]
			break
		}
	}
	sess.SetTool(next)
	a.sketch.groupPick[g] = next
	if len(members) > 1 {
		a.SetHint(next.String() + " · " + g.Key() + " again for the next one")
	}
}
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
	ents := res.Committed()
	if len(ents) == 0 {
		return
	}
	if !a.commitDrawn(s, ents) {
		sess.CancelDraw()
		return
	}
	if res.ClosedChain {
		a.SetHint("Profile closed")
	}
}

// commitDrawn runs a gesture's entities into the sketch as one undo step.
//
// One click, one history entry, however many entities the gesture produced:
// undoing an aligned rectangle has to take the whole rectangle, not a quarter
// of it. A single entity still goes through AddEntity so its undo name stays
// "Draw rectangle" rather than a generic label.
func (a *App) commitDrawn(s *model.Sketch, ents []model.Entity) bool {
	if a.sketch.construction {
		ents = append([]model.Entity(nil), ents...)
		for i := range ents {
			ents[i].Construction = true
		}
	}
	if len(ents) == 1 {
		return a.Run(&model.AddEntity{Sketch: s.ID, Entity: ents[0]})
	}
	return a.Run(&model.ReplaceEntities{
		Sketch: s.ID,
		Add:    ents,
		Label:  "Draw " + a.sketch.session.Tool.String(),
	})
}

// armedConstruction reports whether new entities are being drawn as guides.
func (a *App) armedConstruction() bool { return a.sketch.construction }

// toggleConstruction is what Q does: with a selection it converts those
// entities, and with none it arms the mode for whatever is drawn next.
//
// Two jobs on one key because they are the same intent — "this is a guide, not
// the shape" — and which one applies is never ambiguous: either you have
// something selected or you do not (SPEC-UX §8.9).
func (a *App) toggleConstruction() {
	s := a.ActiveSketch()
	sess := a.sketch.session
	if s == nil || sess == nil {
		return
	}
	if len(sess.Selected) > 0 {
		// The selection decides the direction: if any of it is still real
		// geometry, convert the lot to construction, else convert it back.
		on := false
		for _, i := range sess.Selected {
			if i >= 0 && i < len(s.Entities) && !s.Entities[i].Construction {
				on = true
				break
			}
		}
		idx := append([]int(nil), sess.Selected...)
		if a.Run(&model.SetConstruction{Sketch: s.ID, Indices: idx, On: on}) {
			word := "construction"
			if !on {
				word = "ordinary geometry"
			}
			a.Toast(ui.Toast{Text: fmt.Sprintf("%s is now %s",
				plural(len(idx), "entity", "entities"), word)})
		}
		return
	}
	a.sketch.construction = !a.sketch.construction
	if a.sketch.construction {
		a.Toast(ui.Toast{Text: "Drawing construction geometry — guides that close no region"})
	} else {
		a.Toast(ui.Toast{Text: "Drawing ordinary geometry again"})
	}
}

// SketchStrokePickPx is how close the pointer must come to a sketch line to
// count as being on it.
const SketchStrokePickPx = 3

// handleSketchSelectClick picks entities and regions (SPEC-UX §8.5, §8.6).
func (a *App) handleSketchSelectClick(in InputFrame, s *model.Sketch, sess *sketch.Session, p geom.Vec2i) {
	radius := int64(SketchStrokePickPx * a.subunitsPerPixel(a.layout.RenderViewport()))
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
		view := scene.SketchView{
			Sketch:   s,
			Selected: a.Sel.Contains(model.SketchRef(s.ID)) || a.hoverSketch == s,
		}
		if d := scene.BuildSketchDraw(view); d != nil && !d.Empty() {
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
