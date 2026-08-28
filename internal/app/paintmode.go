package app

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/io"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/scene"
	"modeler/internal/ui"
)

// Paint mode (R13, SPEC-UX §13).
//
// The mode is deliberately narrow: hover any face of any visible body and the
// texels under the brush are outlined on the surface; press and they change.
// Everything else — which face, which texel, how big the picture is — is
// arithmetic that already exists in package paint, so this file is routing and
// nothing more.
//
// The widest hit area in the program is the reason for the care taken over what
// is armed. In Idle a click has to choose between a gizmo, an arrow, a body and
// empty space; in paint mode a press on any face means one thing, and the other
// tools must be disarmed rather than merely undrawn (that bug cost M6 a
// session).

// ObliqueWarnDegrees is how far off square-on the view can get before the card
// offers to turn the camera (SPEC-UX §13.2). Past this the texels under the
// cursor are compressed to slivers and aiming becomes guesswork.
const ObliqueWarnDegrees = 70

// PaintPickIntervalMillis throttles the ID pass while merely hovering, matching
// the rest of the app (SPEC-RENDER §6.1). A live stroke does not pick at all —
// it intersects the face it started on, so dragging over a nearer body cannot
// steal the stroke half way through.
const PaintPickIntervalMillis = 33

// FaceViewMargin is the fraction of air left around a face when the camera
// frames it. Without any, the face's edges sit exactly on the viewport border
// and there is nowhere to see the shape it belongs to.
const FaceViewMargin = 0.12

// paintState is the app's half of paint mode.
type paintState struct {
	tool  paint.Tool
	size  int
	res   int
	color color.RGBA
	// colorB is the gradient's far end. It is a second armed colour rather than
	// a gradient setting, because picking one up with the eyedropper and
	// picking the other has to be the same gesture twice.
	colorB color.RGBA
	// slot is which of the two swatches a palette click, an eyedrop or the HSV
	// popover writes to: 0 the near colour, 1 the far one.
	slot int
	// dither is the Bayer mode the soft brush and the gradient spread their
	// coverage through (SPEC-UX §13.4).
	dither paint.Dither
	// fillShape makes a rectangle or an ellipse solid rather than an outline.
	fillShape bool

	// recents and custom are settings rather than document state
	// (SPEC-DATA §3.3): picking a colour is not an undo step.
	recents paint.Recents
	custom  []color.RGBA
	// page is which palette page the panel shows: 0 built-in, 1 imported.
	page int

	// hideTextures is the "Textures" eye: the geometry with the paint
	// suppressed, for checking the shape under the pixels.
	hideTextures bool

	// locked confines every stroke to one face, and lockBody/lockFace name it.
	//
	// It is a paint lock and not a camera lock: navigation works the same in
	// every mode (SPEC-UX §1), and checking your work from an angle is exactly
	// what you want to do while painting. What it stops is the brush wandering
	// onto the neighbouring face when the pointer runs over an edge.
	locked   bool
	lockBody uint32
	lockFace mesh.FaceUID
	// awaitingLock means the Lock button has been pressed and the next click on
	// a face is what chooses it.
	//
	// Arming first and picking second, rather than acting on whatever was under
	// the cursor when the button was pressed: the button is then live whatever
	// the pointer is doing, which is the whole point — a control that depends on
	// where the pointer is cannot be reached by moving the pointer to it. It is
	// the same shape as pressing S with no plane selected (SPEC-UX §8.1).
	awaitingLock bool

	// hover is what the pointer is over this frame.
	hover paintHover
	// sticky is the last face the pointer resolved, and it outlives the trip
	// from the viewport to the panel.
	//
	// The live hover has to go the moment the pointer leaves the viewport —
	// there is no texel under a button, and drawing a cursor for one would be a
	// lie. But every control in the panel that acts on "the face you are
	// pointing at" then disarms itself exactly as you reach for it: the Lock
	// button greys out, the Face view button vanishes, and the resample prompt
	// takes its own buttons with it. Those controls read this instead.
	sticky paintHover

	// prov caches the mapping an unpainted face *would* get, so the texel
	// cursor can show the grid before the first stroke commits to it. It is
	// rebuilt only when the face or the resolution chip changes — an Allocate
	// per frame would mean a megabyte of image per frame at 512 px.
	prov     *mesh.FacePaint
	provBody uint32
	provFace mesh.FaceUID
	provRes  int

	// The live stroke.
	stroking   bool
	strokeBody uint32
	strokeFace mesh.FaceUID
	strokePt   *mesh.FacePaint
	points     []image.Point
	// anchor is where a two-point tool was pressed. Those tools keep exactly
	// two points — the anchor and wherever the pointer is now — so dragging out
	// and back leaves the shape you let go of rather than every shape you
	// passed through.
	anchor image.Point

	// pickCooldown throttles the hover pick.
	pickCooldown float64

	// The edge-line tool's state: which edges are picked, which one is under
	// the pointer, and how thick the band is.
	edges         []edgeRef
	hoverEdge     int
	hoverEdgeBody uint32
	// edgeWidth is the band's thickness in texels, on each face (V-126).
	edgeWidth int
}

// paintHover is the face and texel under the pointer.
type paintHover struct {
	ok    bool
	body  uint32
	face  mesh.FaceUID
	index int
	// paint is the face's texture, or the provisional mapping for a face that
	// has not been painted yet.
	paint *mesh.FacePaint
	// allocated distinguishes those two, which is what the resolution prompt
	// and the res chip both need to know.
	allocated bool
	texel     image.Point
	// obliqueDeg is how far the view is from square-on to the face.
	obliqueDeg float64
}

// InPaint reports whether paint mode is active.
func (a *App) InPaint() bool { return a.Mode == ModePaint }

// initPaint sets the defaults a fresh session starts on and restores the parts
// of the palette that live in settings.
func (a *App) initPaint() {
	a.paint.tool = paint.ToolPencil
	a.paint.size = 1
	a.paint.res = paint.DefaultRes
	// A first stroke should be unmistakable against a grey hull, so the default
	// is the palette's warm accent rather than another grey.
	a.paint.color = paint.DefaultPalette()[27]
	// The far end of a ramp defaults to the palette's near-black, so a gradient
	// straight out of the box fades into shadow rather than into nothing.
	a.paint.colorB = paint.DefaultPalette()[0]
	a.paint.edgeWidth = paint.DefaultEdgeWidth
	a.paint.hoverEdge = -1
	a.paint.custom = append([]color.RGBA(nil), a.Settings.CustomPalette...)
	a.paint.recents.Set(a.Settings.RecentColors)
	if len(a.paint.custom) == 0 {
		a.paint.page = 0
	}
}

// BeginPaint enters paint mode (SPEC-UX §13.1).
func (a *App) BeginPaint() bool {
	if a.InPaint() {
		return true
	}
	if len(a.Doc().Bodies) == 0 {
		a.Toast(ui.Toast{
			Text: "There is nothing to paint yet — sketch and extrude a body first",
			Kind: ui.ToastWarn,
		})
		return false
	}
	// A face already selected is almost certainly the one about to be painted,
	// so the camera is left alone and the selection is dropped: a highlighted
	// face under a texel cursor is two cursors.
	a.Sel.Clear()
	a.dropPushPull()
	a.transform.tool = nil
	a.Mode = ModePaint
	a.clearPaintHover(true)
	return true
}

// ExitPaint leaves paint mode, keeping every stroke.
func (a *App) ExitPaint() {
	if !a.InPaint() {
		return
	}
	a.finishStroke()
	a.Mode = ModeIdle
	a.clearPaintHover(true)
	a.paint.prov = nil
	a.paint.locked = false
	a.paint.awaitingLock = false
	a.paint.edges = a.paint.edges[:0]
	a.paint.hoverEdge = -1
}

// paintPanelReachPx is how far in from the viewport's right edge the palette
// panel reaches, in device pixels.
func (a *App) paintPanelReachPx() float64 {
	if !a.InPaint() {
		return 0
	}
	return float64(a.px(paintPanelWidth + ui.Spacing*3))
}

// BeginLockPick arms the lock: the next click on a face chooses it.
func (a *App) BeginLockPick() {
	a.finishStroke()
	a.paint.awaitingLock = true
}

// CancelLockPick disarms it, and reports whether there was anything to disarm.
func (a *App) CancelLockPick() bool {
	if !a.paint.awaitingLock {
		return false
	}
	a.paint.awaitingLock = false
	return true
}

// LockToFace confines painting to a face and turns the camera square-on to it
// (the user's request, 2026-08-26; SPEC-UX §13.5).
//
// Fixing what can be painted is the point; the camera move is what makes it
// worth doing, because a face you are locked to is a face you want to be
// looking at. Both are one action rather than two, so there is one thing to
// press and one thing to undo.
func (a *App) LockToFace(bodyID uint32, uid mesh.FaceUID) bool {
	f, ok := a.resolveFace(bodyID, uid)
	if !ok {
		a.Toast(ui.Toast{Text: "That face is no longer there", Kind: ui.ToastWarn})
		return false
	}
	a.finishStroke()
	a.paint.awaitingLock = false
	a.paint.locked = true
	a.paint.lockBody, a.paint.lockFace = f.body.ID, f.uid
	a.FaceView()
	a.Toast(ui.Toast{
		Text:     fmt.Sprintf("Locked to face %d of %s", f.uid.Seq(), f.body.Name),
		Action:   "Unlock",
		OnAction: func() { a.UnlockFace() },
	})
	return true
}

// UnlockFace releases the lock, leaving the camera where it is.
func (a *App) UnlockFace() {
	if !a.paint.locked {
		return
	}
	a.finishStroke()
	a.paint.locked = false
	a.paint.hover = paintHover{}
	a.Toast(ui.Toast{Text: "Unlocked — every face can be painted again"})
}

// lockedFace resolves the locked face, dropping the lock if it has been cut
// away since.
func (a *App) lockedFace() (faceRef, bool) {
	if !a.paint.locked {
		return faceRef{}, false
	}
	f, ok := a.resolveFace(a.paint.lockBody, a.paint.lockFace)
	if !ok {
		a.paint.locked = false
		a.clearPaintHover(true)
		a.Toast(ui.Toast{
			Text: "The face you were locked to is gone — unlocked",
			Kind: ui.ToastWarn,
		})
		return faceRef{}, false
	}
	return f, true
}

// hoverLockedFace resolves the cursor against the locked face's own plane
// rather than against the ID pass.
//
// This is what makes the lock a lock. The pick pass answers "what is in front
// here", which is the wrong question once you have chosen a face: a body
// drifting in front of it, or an edge along its border, would take the stroke.
// Intersecting the plane cannot be stolen — and it costs no readback at all, so
// a locked session is cheaper than a free one.
func (a *App) hoverLockedFace(in InputFrame, vp render.Viewport) {
	f, ok := a.lockedFace()
	if !ok {
		return
	}
	h := paintHover{ok: true, body: f.body.ID, face: f.uid, index: f.face}
	h.paint, h.allocated = a.mappingFor(f)
	if h.paint == nil {
		a.clearPaintHover(false)
		return
	}
	p, hit := a.pointOnFace(in.MouseX, in.MouseY, vp, h.paint.Frame)
	if !hit {
		a.clearPaintHover(false)
		return
	}
	h.texel = paint.Texel(h.paint, p)
	// The plane runs on past the face, and paint out there would be paint on
	// nothing. Off the face is the same as off the model: no cursor, no stroke.
	//
	// The sticky face stays put through all of that: while a lock is on, the
	// face the panel acts on is the locked one whether the pointer is on it,
	// past its edge, or over the Unlock button.
	a.paint.sticky = h
	if !h.texel.In(paint.FaceRect(f.body.Mesh, f.face, h.paint)) {
		a.clearPaintHover(false)
		return
	}
	h.obliqueDeg = obliqueDegrees(a.Camera, f.body.Mesh.FaceNormal(f.face))
	a.paint.hover = h
}

// canPaint reports whether the tool can run, and why not if it cannot.
func (a *App) canPaint() (bool, string) {
	for _, b := range a.Doc().Bodies {
		if b.Visible && b.Mesh != nil {
			return true, ""
		}
	}
	if len(a.Doc().Bodies) == 0 {
		return false, "Paint needs a body — sketch a profile and extrude it first"
	}
	return false, "Every body is hidden — switch one back on to paint it"
}

// updatePaint runs one frame of paint mode with the pointer in the viewport.
func (a *App) updatePaint(in InputFrame, vp render.Viewport) {
	// The edge tool picks edges rather than painting texels, so it takes the
	// pointer before any of the brush machinery runs.
	if a.paint.tool == paint.ToolEdge {
		a.clearPaintHover(false)
		a.pickPaintEdge(in, vp)
		return
	}

	// The pick pass is the only thing that knows which face is in front at the
	// cursor, but a stroke already knows: it stays on the face it started on.
	if a.paint.stroking {
		a.trackStroke(in, vp)
		return
	}
	a.refreshPaintHover(in, vp)

	if !in.Pressed[MouseLeft] || !a.paint.hover.ok {
		return
	}
	// A click while the lock is armed chooses the face and paints nothing. The
	// click is spent on the choice.
	if a.paint.awaitingLock {
		a.LockToFace(a.paint.hover.body, a.paint.hover.face)
		return
	}
	// Alt is the eyedropper everywhere, held or armed (SPEC-UX §16).
	if in.Alt || a.paint.tool == paint.ToolPick {
		a.eyedrop()
		return
	}
	a.beginStroke()
}

// paintChromeFrame runs the parts of paint mode that must keep running while
// the pointer is over the chrome: a stroke that ends off the viewport still
// ends, rather than staying open until the pointer wanders back.
// This is also where the panel gets its own pointer: the live hover goes, and
// the sticky one stays, so a control that acts on the face you were pointing at
// is still armed when you reach it.
func (a *App) paintChromeFrame(in InputFrame) {
	a.clearPaintHover(false)
	if a.paint.stroking && !in.Down[MouseLeft] {
		a.finishStroke()
	}
}

// clearPaintHover drops the live hover. It keeps the sticky one unless the
// pointer is genuinely resting on nothing paintable inside the viewport, which
// is the only case where the panel should forget what it was pointed at.
func (a *App) clearPaintHover(alsoSticky bool) {
	a.paint.hover = paintHover{}
	if alsoSticky {
		a.paint.sticky = paintHover{}
	}
}

// stickyFace is the face the panel's controls act on: the one under the
// pointer, or the last one that was.
//
// It is re-resolved rather than trusted, because a boolean can take the face
// away between the hover and the click.
func (a *App) stickyFace() (paintHover, bool) {
	h := a.paint.hover
	if !h.ok {
		h = a.paint.sticky
	}
	if !h.ok {
		return paintHover{}, false
	}
	if _, ok := a.resolveFace(h.body, h.face); !ok {
		return paintHover{}, false
	}
	return h, true
}

// refreshPaintHover resolves the face and texel under the pointer.
func (a *App) refreshPaintHover(in InputFrame, vp render.Viewport) {
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) ||
		a.Cube.Contains(in.MouseX, in.MouseY) || a.orbiting || a.panning || a.cubeDrag {
		// Off the viewport or navigating: no cursor, but the panel still knows
		// which face you were on.
		a.clearPaintHover(false)
		return
	}
	if a.paint.locked {
		a.hoverLockedFace(in, vp)
		return
	}
	a.pickPaintFace(in, vp)

	h := a.paint.hover
	if !h.ok {
		return
	}
	f, ok := a.resolveFace(h.body, h.face)
	if !ok {
		a.clearPaintHover(true)
		return
	}
	h.index = f.face
	h.paint, h.allocated = a.mappingFor(f)
	if h.paint == nil {
		a.clearPaintHover(true)
		return
	}
	p, hit := a.pointOnFace(in.MouseX, in.MouseY, vp, h.paint.Frame)
	if !hit {
		a.clearPaintHover(true)
		return
	}
	h.texel = paint.Texel(h.paint, p)
	h.obliqueDeg = obliqueDegrees(a.Camera, f.body.Mesh.FaceNormal(f.face))
	a.paint.hover, a.paint.sticky = h, h
}

// pickPaintFace refreshes which face is under the cursor, throttled to the
// 30 Hz of SPEC-RENDER §6.1.
//
// Unlike Idle's hover it does not wait for pointer motion. What is under a
// stationary cursor changes on its own here — an undo can take a face away, a
// resample can rebuild one — and a press must always pick fresh rather than
// trust a result that is up to a frame old (SPEC-RENDER §6.1).
func (a *App) pickPaintFace(in InputFrame, vp render.Viewport) {
	a.paint.pickCooldown -= in.DeltaMillis
	if a.paint.pickCooldown > 0 && !in.Pressed[MouseLeft] {
		return
	}
	a.paint.pickCooldown = PaintPickIntervalMillis

	s := a.BuildScene()
	a.Renderer.SetFramebuffer(in.WindowW, in.WindowH)
	hit := a.Renderer.Pick(&s, vp, in.MouseX, in.MouseY)
	a.Hover = hit
	if !hit.Hit || hit.Kind != render.PickFace {
		a.clearPaintHover(true)
		return
	}
	a.paint.hover = paintHover{ok: true, body: hit.BodyID, face: hit.FaceUID}
}

// mappingFor is the face's texture, or the one a first stroke would create.
//
// Showing the provisional grid is what makes the resolution chips mean
// anything before you commit: you can see 16 px against 128 px on the face
// itself rather than guessing from a number.
func (a *App) mappingFor(f faceRef) (*mesh.FacePaint, bool) {
	if p := f.body.Mesh.Faces[f.face].Paint; p != nil {
		return p, true
	}
	st := &a.paint
	if st.prov != nil && st.provBody == f.body.ID && st.provFace == f.uid && st.provRes == st.res {
		return st.prov, false
	}
	p, err := paint.Allocate(f.body.Mesh, f.face, st.res)
	if err != nil {
		st.prov, st.provFace = nil, 0
		return nil, false
	}
	st.prov, st.provBody, st.provFace, st.provRes = p, f.body.ID, f.uid, st.res
	return p, false
}

// pointOnFace intersects the cursor ray with a face's plane.
func (a *App) pointOnFace(mouseX, mouseY float64, vp render.Viewport, frame geom.Frame) (geom.Vec3, bool) {
	origin, dir := a.Camera.Ray(vp.Local(mouseX, mouseY), float64(vp.W), float64(vp.H))
	denom := dir.Dot(frame.N)
	if denom > -geom.NormalEps && denom < geom.NormalEps {
		return geom.Vec3{}, false
	}
	t := frame.O.Sub(origin).Dot(frame.N) / denom
	if t < 0 && a.Camera.Perspective {
		return geom.Vec3{}, false
	}
	return origin.Add(dir.Mul(t)), true
}

// obliqueDegrees is the angle between the view direction and the face's
// normal: zero is square-on, ninety is edge-on.
func obliqueDegrees(cam render.Camera, normal geom.Vec3) float64 {
	d := -cam.Forward().Dot(normal)
	if d > 1 {
		d = 1
	}
	if d < -1 {
		d = -1
	}
	return math.Acos(d) * 180 / math.Pi
}

// eyedrop samples the colour under the cursor into the brush (SPEC-UX §13.2).
func (a *App) eyedrop() {
	h := a.paint.hover
	b := a.Doc().BodyByID(h.body)
	if b == nil {
		return
	}
	var p *mesh.FacePaint
	if h.allocated {
		p = h.paint
	}
	c := paint.Sample(p, h.texel, b.Color)
	a.setPaintColor(c)
	a.Toast(ui.Toast{Text: "Picked #" + paint.Hex(c)})
}

// setPaintColor arms a colour in the active slot and records it as recently
// used.
func (a *App) setPaintColor(c color.RGBA) {
	c.A = 255
	if a.paint.slot == 1 {
		a.paint.colorB = c
	} else {
		a.paint.color = c
	}
	a.paint.recents.Add(c)
}

// activeColor is the colour the palette, the eyedropper and the picker are
// currently pointed at.
func (a *App) activeColor() color.RGBA {
	if a.paint.slot == 1 {
		return a.paint.colorB
	}
	return a.paint.color
}

// swapPaintColors exchanges the two armed colours, which is how a ramp gets
// reversed without picking both again.
func (a *App) swapPaintColors() {
	a.paint.color, a.paint.colorB = a.paint.colorB, a.paint.color
}

// beginStroke starts a drag's worth of painting.
func (a *App) beginStroke() {
	h := a.paint.hover
	if a.paint.tool == paint.ToolEraser && !h.allocated {
		a.Toast(ui.Toast{Text: "There is no paint on that face to rub out", Kind: ui.ToastWarn})
		return
	}
	a.paint.stroking = true
	a.paint.strokeBody = h.body
	a.paint.strokeFace = h.face
	a.paint.strokePt = h.paint
	a.paint.anchor = h.texel
	a.paint.points = append(a.paint.points[:0], h.texel)
	a.applyStroke()
}

// trackStroke follows the pointer while the button is down, then commits.
//
// The texels come from the face's own plane rather than from the pick pass:
// once a stroke has started, the face it started on is the only face it can
// paint, and a body that passes in front of the cursor mid-drag must not
// capture it.
func (a *App) trackStroke(in InputFrame, vp render.Viewport) {
	if !in.Down[MouseLeft] {
		a.finishStroke()
		return
	}
	if a.paint.strokePt == nil {
		return
	}
	p, ok := a.pointOnFace(in.MouseX, in.MouseY, vp, a.paint.strokePt.Frame)
	if !ok {
		return
	}
	t := paint.Texel(a.paint.strokePt, p)
	if a.paint.tool.TwoPoint() {
		// Two points, always: the anchor and where the pointer is now. Shift
		// constrains what "now" is allowed to mean.
		t = constrainEnd(a.paint.anchor, t, a.paint.tool, in.Shift)
		if len(a.paint.points) == 2 && a.paint.points[1] == t {
			return
		}
		a.paint.points = append(a.paint.points[:1], t)
		a.applyStroke()
		return
	}
	if n := len(a.paint.points); n > 0 && a.paint.points[n-1] == t {
		return // same texel: nothing new to say
	}
	a.paint.points = append(a.paint.points, t)
	a.applyStroke()
}

// applyStroke pushes the stroke so far into the document as a live drag, so
// what you see is the real command's output and the history gets one entry per
// mouse-down (SPEC-DATA §3.2).
func (a *App) applyStroke() {
	cmd := &paint.StrokeFace{
		Body:   a.paint.strokeBody,
		Face:   a.paint.strokeFace,
		Tool:   a.paint.tool,
		Color:  a.paint.color,
		ColorB: a.paint.colorB,
		Size:   a.paint.size,
		Res:    a.paint.res,
		Dither: a.paint.dither,
		Fill:   a.paint.fillShape,
		// The command reruns the whole stroke from its points each frame, so it
		// needs its own copy: the slice keeps growing under it otherwise.
		Points: append([]image.Point(nil), a.paint.points...),
	}
	// A stroke that has not changed anything yet is not an error worth saying
	// out loud — the very first press on a texel that is already the brush
	// colour is the common case. It simply does not open a drag, and the next
	// texel the pointer reaches will.
	if a.Bus.Dragging() {
		_ = a.Bus.UpdateDrag(cmd)
		return
	}
	_ = a.Bus.BeginDrag(cmd)
}

// finishStroke ends the drag, recording it as one step.
func (a *App) finishStroke() {
	if !a.paint.stroking {
		return
	}
	a.paint.stroking = false
	a.paint.points = a.paint.points[:0]
	a.paint.strokePt = nil
	if !a.Bus.Dragging() {
		return
	}
	if _, ok := a.Bus.CommitDrag(); ok {
		a.paint.recents.Add(a.paint.color)
	}
}

// CancelStroke reverts a live stroke, which is what Escape mid-drag does.
func (a *App) CancelStroke() bool {
	if !a.paint.stroking {
		return false
	}
	a.paint.stroking = false
	a.paint.points = a.paint.points[:0]
	a.paint.strokePt = nil
	a.Bus.CancelDrag()
	return true
}

// PaintFace paints one texel run on a named face, which is what the script ops
// drive. It goes through the same command the pointer does.
func (a *App) PaintFace(bodyID uint32, uid mesh.FaceUID, pts []image.Point) bool {
	return a.Run(&paint.StrokeFace{
		Body: bodyID, Face: uid,
		Tool: a.paint.tool, Color: a.paint.color, ColorB: a.paint.colorB,
		Size: a.paint.size, Res: a.paint.res,
		Dither: a.paint.dither, Fill: a.paint.fillShape,
		Points: pts,
	})
}

// ResampleHoveredFace rebuilds a face's texture at the armed resolution, which
// is the second half of the mismatch prompt (SPEC-UX §13.2).
func (a *App) ResampleFace(bodyID uint32, uid mesh.FaceUID, res int) bool {
	if !a.Run(&paint.ResampleFace{Body: bodyID, Face: uid, Res: res}) {
		return false
	}
	a.Toast(ui.Toast{
		Text:     fmt.Sprintf("Resampled the face to %d px", res),
		Action:   "Undo",
		OnAction: func() { a.Undo() },
	})
	return true
}

// SetPaintRes arms a resolution chip. It never touches an existing texture:
// texel density is fixed when a face is first painted and only an explicit
// resample changes it (SPEC-GEOMETRY §8.2).
func (a *App) SetPaintRes(res int) bool {
	if !paint.ValidRes(res) {
		a.Toast(ui.Toast{Text: fmt.Sprintf("%d is not a paint resolution", res), Kind: ui.ToastWarn})
		return false
	}
	a.paint.res = res
	return true
}

// FaceView turns the camera square-on to the face under the cursor
// (SPEC-UX §13.2).
func (a *App) FaceView() bool {
	f, ok := a.lockedFace()
	if !ok {
		h, hok := a.stickyFace()
		if f, ok = a.resolveFace(h.body, h.face); !hok || !ok {
			return false
		}
	}
	m := f.body.Mesh
	to := a.targetCamera()
	to.LookAlong(m.FaceNormal(f.face))
	to.Target = m.FaceCentroid(f.face)
	// Fill the viewport with the face and a little air, so the texels are as
	// big as they can be without the edges touching the frame. Measured along
	// the screen axes, not around a sphere: a hull side is wide and flat, and
	// its sphere is mostly empty space.
	var pts []geom.Vec3
	for _, loop := range m.Faces[f.face].Loops {
		for _, vi := range loop {
			pts = append(pts, m.Verts[vi])
		}
	}
	// The palette panel floats over the right of the viewport, so the space a
	// face can actually be worked in is narrower than the viewport is. Framing
	// against the whole thing would put a quarter of the face under the panel,
	// which on the one camera move whose entire job is "let me see this face"
	// is the wrong answer.
	vp := a.layout.RenderViewport()
	clear := float64(vp.W) - a.paintPanelReachPx()
	if clear < float64(vp.W)/3 {
		clear = float64(vp.W) / 3
	}
	aspect := clear / float64(vp.H)
	to.FrameTightly(pts, aspect, FaceViewMargin)

	// FrameTightly centres on the viewport; slide the target so the face lands
	// centred in the clear part of it instead.
	if vp.H > 0 {
		worldPerPx := to.OrthoScale / float64(vp.H)
		// Moving the target along the camera's right slides the geometry left on
		// screen, which is the direction the face has to go to clear the panel.
		to.Target = to.Target.Add(to.Right().Mul(a.paintPanelReachPx() / 2 * worldPerPx))
	}
	a.Anim.Start(a.Camera, to)
	return true
}

// paintCursorOverlay is the texel cursor for this frame, or nil.
func (a *App) paintCursorOverlay() *render.Overlay {
	if a.paint.awaitingLock {
		// Choosing a face is not painting one. The face under the cursor already
		// pre-highlights; a texel cursor on top of it would say the next click
		// puts paint down, and it does not.
		return nil
	}
	h := a.paint.hover
	// Mid-stroke the cursor follows the stroke rather than the hover, which
	// stopped being refreshed the moment the button went down.
	if a.paint.stroking && a.paint.strokePt != nil && len(a.paint.points) > 0 {
		h = paintHover{
			ok:    true,
			paint: a.paint.strokePt,
			texel: a.paint.points[len(a.paint.points)-1],
		}
	}
	if !h.ok || h.paint == nil {
		return nil
	}
	v := scene.PaintCursorView{
		Paint:   h.paint,
		Texel:   h.texel,
		Size:    a.paint.size,
		Color:   a.paint.color,
		Erasing: a.paint.tool == paint.ToolEraser,
		Filling: a.paint.tool == paint.ToolFill,
	}
	if v.Filling {
		if f, ok := a.resolveFace(h.body, h.face); ok {
			v.FaceRect = paint.FaceRect(f.body.Mesh, f.face, h.paint)
		}
	}
	if a.paint.tool == paint.ToolPick {
		// The dropper takes a colour rather than leaving one, so it shows the
		// texel it would read and nothing about the brush.
		v.Erasing, v.Size = true, 1
	}
	return scene.BuildPaintCursor(v)
}

// paintToolKeys maps the paint-mode keyboard to the tools. The letters are the
// ones paint.Tool.Shortcut already pins, and they are mode-local the way sketch
// mode's V/L/R/C are (DECISIONS V-50).
var paintToolKeys = []struct {
	key  int32
	tool paint.Tool
}{
	{rl.KeyD, paint.ToolPencil},
	{rl.KeyB, paint.ToolBrush},
	{rl.KeyE, paint.ToolEraser},
	{rl.KeyG, paint.ToolFill},
	{rl.KeyI, paint.ToolPick},
	{rl.KeyL, paint.ToolLine},
	{rl.KeyR, paint.ToolRect},
	{rl.KeyC, paint.ToolCircle},
	{rl.KeyN, paint.ToolGradient},
	{rl.KeyK, paint.ToolEdge},
}

// handlePaintKeys is the paint-mode keyboard map: the tools, the colour swap,
// and Escape stepping back out (SPEC-UX §1, §13).
func (a *App) handlePaintKeys(in InputFrame) {
	if in.Ctrl {
		return
	}
	for _, k := range paintToolKeys {
		if in.KeyPressed(k.key) {
			a.paint.tool = k.tool
			break
		}
	}
	if in.KeyPressed(rl.KeyX) {
		a.swapPaintColors()
	}
	if in.KeyPressed(rl.KeyP) {
		a.ExitPaint()
	}
	if in.KeyPressed(rl.KeyEscape) {
		// One level per press (SPEC-UX §1): a live stroke, then the lock, then
		// the mode.
		switch {
		case a.CancelStroke():
		case a.ClearEdgeSelection():
		case a.CancelLockPick():
		case a.paint.locked:
			a.UnlockFace()
		default:
			a.ExitPaint()
		}
	}
}

// paintHint is what the hint bar says in paint mode.
func (a *App) paintHint() string {
	st := &a.paint
	if st.awaitingLock {
		if st.hover.ok {
			return "Click this face to lock to it · Esc to cancel"
		}
		return "Click the face you want to lock to · Esc to cancel"
	}
	if st.tool == paint.ToolEdge {
		if n := len(st.edges); n > 0 {
			return fmt.Sprintf("%s picked · press Paint to bake the line · Esc clears", 
				plural(n, "edge", "edges"))
		}
		return "Click edges to draw a line along · All corners picks them for you"
	}
	if st.stroking {
		return st.tool.String() + " · release to finish the stroke"
	}
	h := st.hover
	if !h.ok {
		if st.locked {
			return "Locked to one face — the pointer is off it · Esc unlocks"
		}
		return "Hover a face to paint it · Esc leaves paint mode"
	}
	if a.UI.In.Alt || st.tool == paint.ToolPick {
		return "Click to pick up the colour under the cursor"
	}
	if h.allocated && h.paint.Res != st.res {
		return fmt.Sprintf("This face is %d px — painting it stays %d px", h.paint.Res, h.paint.Res)
	}
	if !h.allocated {
		return fmt.Sprintf("Paint this face at %d px · texel %d,%d", st.res, h.texel.X, h.texel.Y)
	}
	if st.tool.TwoPoint() {
		return fmt.Sprintf("%s · drag from here · Shift constrains it",
			st.tool.String())
	}
	return fmt.Sprintf("%s · texel %d,%d", st.tool.String(), h.texel.X, h.texel.Y)
}

// paintPalette is the page of swatches the panel shows.
func (a *App) paintPalette() []color.RGBA {
	if a.paint.page == 1 && len(a.paint.custom) > 0 {
		return a.paint.custom
	}
	return paint.DefaultPalette()
}

// ImportPalette reads a Lospec .hex file into the custom page (SPEC-UX §13.3).
// It never touches the built-in page: a bad import must always be one click
// away from the colours you started with.
func (a *App) ImportPalette(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		a.Toast(ui.Toast{Text: "Could not read that palette file", Kind: ui.ToastError})
		return false
	}
	defer f.Close()

	cols, err := paint.ParseHex(f)
	if err != nil {
		a.Toast(ui.Toast{Text: capitalize(err.Error()), Kind: ui.ToastError})
		return false
	}
	a.paint.custom = cols
	a.paint.page = 1
	a.Toast(ui.Toast{
		Text: fmt.Sprintf("Imported %s — %s", filepath.Base(path),
			plural(len(cols), "colour", "colours")),
	})
	return true
}

// handleDroppedFiles routes files dragged onto the window: a .ship opens, a
// .hex palette lands on the custom page.
func (a *App) handleDroppedFiles(in InputFrame) {
	for _, path := range in.Dropped {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".hex":
			if a.ImportPalette(path) && !a.InPaint() {
				a.BeginPaint()
			}
		case io.ShipExtension:
			// Through the request queue, not straight to OpenPath: the unsaved
			// work guard and the after-frame dialog rules apply to a drop the
			// same as to Ctrl+O.
			a.RequestOpenPath(path)
		default:
			a.Toast(ui.Toast{
				Text: "Drop a .ship to open it, or a .hex palette for colours",
				Kind: ui.ToastWarn,
			})
		}
	}
}

// constrainEnd applies Shift to a two-point tool's far end (SPEC-UX §13.4).
//
// What "constrained" means depends on the tool, and both meanings are the one a
// hand expects: a box wants to be square, a line wants to be straight or at
// forty-five degrees.
func constrainEnd(anchor, end image.Point, tool paint.Tool, shift bool) image.Point {
	if !shift {
		return end
	}
	dx, dy := end.X-anchor.X, end.Y-anchor.Y
	if tool.Shape() {
		// Equal sides, taking the longer drag so the shape follows the pointer
		// rather than shrinking to the shorter axis.
		n := absInt(dx)
		if m := absInt(dy); m > n {
			n = m
		}
		return image.Point{X: anchor.X + n*signInt(dx), Y: anchor.Y + n*signInt(dy)}
	}
	// Horizontal, vertical, or the diagonal between them — whichever the drag
	// is already closest to.
	ax, ay := absInt(dx), absInt(dy)
	switch {
	case ax > 2*ay:
		return image.Point{X: end.X, Y: anchor.Y}
	case ay > 2*ax:
		return image.Point{X: anchor.X, Y: end.Y}
	}
	n := ax
	if ay > n {
		n = ay
	}
	return image.Point{X: anchor.X + n*signInt(dx), Y: anchor.Y + n*signInt(dy)}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func signInt(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
