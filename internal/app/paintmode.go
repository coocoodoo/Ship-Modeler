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

// paintState is the app's half of paint mode.
type paintState struct {
	tool  paint.Tool
	size  int
	res   int
	color color.RGBA

	// recents and custom are settings rather than document state
	// (SPEC-DATA §3.3): picking a colour is not an undo step.
	recents paint.Recents
	custom  []color.RGBA
	// page is which palette page the panel shows: 0 built-in, 1 imported.
	page int

	// hideTextures is the "Textures" eye: the geometry with the paint
	// suppressed, for checking the shape under the pixels.
	hideTextures bool

	// hover is what the pointer is over this frame.
	hover paintHover

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

	// pickCooldown throttles the hover pick.
	pickCooldown float64
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
	a.paint.hover = paintHover{}
	return true
}

// ExitPaint leaves paint mode, keeping every stroke.
func (a *App) ExitPaint() {
	if !a.InPaint() {
		return
	}
	a.finishStroke()
	a.Mode = ModeIdle
	a.paint.hover = paintHover{}
	a.paint.prov = nil
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
func (a *App) paintChromeFrame(in InputFrame) {
	a.paint.hover = paintHover{}
	if a.paint.stroking && !in.Down[MouseLeft] {
		a.finishStroke()
	}
}

// refreshPaintHover resolves the face and texel under the pointer.
func (a *App) refreshPaintHover(in InputFrame, vp render.Viewport) {
	if !vp.Contains(int(in.MouseX), int(in.MouseY)) ||
		a.Cube.Contains(in.MouseX, in.MouseY) || a.orbiting || a.panning || a.cubeDrag {
		a.paint.hover = paintHover{}
		return
	}
	a.pickPaintFace(in, vp)

	h := a.paint.hover
	if !h.ok {
		return
	}
	f, ok := a.resolveFace(h.body, h.face)
	if !ok {
		a.paint.hover = paintHover{}
		return
	}
	h.index = f.face
	h.paint, h.allocated = a.mappingFor(f)
	if h.paint == nil {
		a.paint.hover = paintHover{}
		return
	}
	p, hit := a.pointOnFace(in.MouseX, in.MouseY, vp, h.paint.Frame)
	if !hit {
		a.paint.hover = paintHover{}
		return
	}
	h.texel = paint.Texel(h.paint, p)
	h.obliqueDeg = obliqueDegrees(a.Camera, f.body.Mesh.FaceNormal(f.face))
	a.paint.hover = h
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
		a.paint.hover = paintHover{}
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

// setPaintColor arms a colour and records it as recently used.
func (a *App) setPaintColor(c color.RGBA) {
	c.A = 255
	a.paint.color = c
	a.paint.recents.Add(c)
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
		Body:  a.paint.strokeBody,
		Face:  a.paint.strokeFace,
		Tool:  a.paint.tool,
		Color: a.paint.color,
		Size:  a.paint.size,
		Res:   a.paint.res,
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
		Tool: a.paint.tool, Color: a.paint.color, Size: a.paint.size,
		Res: a.paint.res, Points: pts,
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
	h := a.paint.hover
	f, ok := a.resolveFace(h.body, h.face)
	if !ok {
		return false
	}
	to := a.targetCamera()
	to.LookAlong(f.body.Mesh.FaceNormal(f.face))
	to.Target = f.body.Mesh.FaceCentroid(f.face)
	a.Anim.Start(a.Camera, to)
	return true
}

// paintCursorOverlay is the texel cursor for this frame, or nil.
func (a *App) paintCursorOverlay() *render.Overlay {
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

// handlePaintKeys is the paint-mode keyboard map: the four tools, and Escape
// stepping back out (SPEC-UX §1, §13).
func (a *App) handlePaintKeys(in InputFrame) {
	if in.Ctrl {
		return
	}
	switch {
	case in.KeyPressed(rl.KeyD):
		a.paint.tool = paint.ToolPencil
	case in.KeyPressed(rl.KeyE):
		a.paint.tool = paint.ToolEraser
	case in.KeyPressed(rl.KeyG):
		a.paint.tool = paint.ToolFill
	case in.KeyPressed(rl.KeyI):
		a.paint.tool = paint.ToolPick
	}
	if in.KeyPressed(rl.KeyP) {
		a.ExitPaint()
	}
	if in.KeyPressed(rl.KeyEscape) {
		if !a.CancelStroke() {
			a.ExitPaint()
		}
	}
}

// paintHint is what the hint bar says in paint mode.
func (a *App) paintHint() string {
	st := &a.paint
	if st.stroking {
		return st.tool.String() + " · release to finish the stroke"
	}
	h := st.hover
	if !h.ok {
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

// handleDroppedFiles is the import path that works today: drop a .hex palette
// on the window and it lands on the custom page.
//
// The file dialogs arrive with M8 (SPEC-DATA §5), and until they do this is the
// difference between an import that exists and one that is only specced.
func (a *App) handleDroppedFiles(in InputFrame) {
	for _, path := range in.Dropped {
		if strings.EqualFold(filepath.Ext(path), ".hex") {
			if a.ImportPalette(path) && !a.InPaint() {
				a.BeginPaint()
			}
			continue
		}
		a.Toast(ui.Toast{
			Text: "Only .hex palettes can be dropped for now — opening projects arrives with M8",
			Kind: ui.ToastWarn,
		})
	}
}
