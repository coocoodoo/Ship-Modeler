package app

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
	"modeler/internal/geom/mesh"
	"modeler/internal/model"
	"modeler/internal/paint"
	"modeler/internal/render"
	"modeler/internal/ui"
)

type moveTextureState struct {
	open              bool
	body              uint32
	face              mesh.FaceUID
	dx, dy            int
	original, preview *mesh.FacePaint
	gpu               *render.BodyGPU
	held              int32
	repeat            float64
}

func (a *App) beginMoveTexture(body uint32, face mesh.FaceUID) {
	f, ok := a.resolveFace(body, face)
	if !ok {
		return
	}
	p := f.body.Mesh.Faces[f.face].Paint
	if p == nil || p.Img == nil {
		a.Toast(ui.Toast{Text: "This face has no texture to move", Kind: ui.ToastWarn})
		return
	}
	a.closeMoveTexture()
	a.library.menu = bodyMenuState{}
	a.CancelMarkerPick()
	a.dropPushPull()
	a.transform.tool = nil
	a.Sel.Clear()
	a.UI.ClearFocus()
	a.moveTexture = moveTextureState{open: true, body: body, face: face, original: p}
}
func (a *App) closeMoveTexture() {
	if a.moveTexture.gpu != nil {
		a.moveTexture.gpu.Unload()
	}
	a.moveTexture = moveTextureState{}
}
func (a *App) finishMoveTexture(apply bool) {
	st := a.moveTexture
	a.closeMoveTexture()
	a.UI.ClearFocus()
	if apply && (st.dx != 0 || st.dy != 0) {
		a.Run(&paint.MoveTexture{Body: st.body, Face: st.face, DX: st.dx, DY: st.dy})
	}
}
func (a *App) nudgeMoveTexture(dx, dy int) {
	st := &a.moveTexture
	f, ok := a.resolveFace(st.body, st.face)
	if !ok {
		a.closeMoveTexture()
		return
	}
	st.dx += dx
	st.dy += dy
	p, e := paint.OffsetTexture(f.body.Mesh, f.face, st.original, st.dx, st.dy)
	if e != nil {
		return
	}
	st.preview = p
	if st.gpu != nil {
		st.gpu.Unload()
		st.gpu = nil
	}
}

// Choose the texture axis that most closely follows the screen arrow.
func (a *App) moveTextureArrow(x, y int) {
	st := &a.moveTexture
	p := st.original
	vp := a.layout.RenderViewport()
	o, ok := a.Camera.WorldToViewport(p.Frame.O, float64(vp.W), float64(vp.H))
	if !ok {
		return
	}
	u, uo := a.Camera.WorldToViewport(p.Frame.O.Add(p.Frame.U), float64(vp.W), float64(vp.H))
	v, vo := a.Camera.WorldToViewport(p.Frame.O.Add(p.Frame.V), float64(vp.W), float64(vp.H))
	if !uo || !vo {
		return
	}
	score := func(dx, dy float64) float64 {
		l := math.Hypot(dx, dy)
		if l < 1e-8 {
			return 0
		}
		return (dx*float64(x) + dy*float64(y)) / l
	}
	us, vs := score(u.X-o.X, u.Y-o.Y), score(v.X-o.X, v.Y-o.Y)
	step := max(absInt(x), absInt(y))
	sgn := func(v float64) int {
		if v < 0 {
			return -step
		}
		return step
	}
	if math.Abs(us) >= math.Abs(vs) {
		a.nudgeMoveTexture(sgn(us), 0)
	} else {
		a.nudgeMoveTexture(0, sgn(vs))
	}
}
func (a *App) updateMoveTexture(in InputFrame) {
	if a.UI.ModalOpen() {
		return
	}
	if in.KeyPressed(rl.KeyEscape) {
		a.finishMoveTexture(false)
		return
	}
	if in.KeyPressed(rl.KeyEnter) {
		a.finishMoveTexture(true)
		return
	}
	st := &a.moveTexture
	k := int32(0)
	for _, candidate := range []int32{rl.KeyLeft, rl.KeyRight, rl.KeyUp, rl.KeyDown} {
		if in.KeyDown(candidate) || in.KeyPressed(candidate) {
			k = candidate
			break
		}
	}
	if k == 0 || in.FocusLost {
		st.held = 0
		return
	}
	fire := in.KeyPressed(k) || st.held != k
	if fire {
		st.held = k
		st.repeat = 320
	} else {
		st.repeat -= in.DeltaMillis
		if st.repeat <= 0 {
			fire = true
			st.repeat = 65
		}
	}
	if !fire {
		return
	}
	step := 1
	if in.Shift {
		step = 8
	}
	switch k {
	case rl.KeyLeft:
		a.moveTextureArrow(-step, 0)
	case rl.KeyRight:
		a.moveTextureArrow(step, 0)
	case rl.KeyUp:
		a.moveTextureArrow(0, -step)
	case rl.KeyDown:
		a.moveTextureArrow(0, step)
	}
}
func (a *App) moveTextureGPU(b *model.Body) *render.BodyGPU {
	st := &a.moveTexture
	if st.gpu != nil {
		return st.gpu
	}
	if st.preview == nil {
		return nil
	}
	m := *b.Mesh
	m.Faces = append([]mesh.Face(nil), b.Mesh.Faces...)
	for i := range m.Faces {
		if m.Faces[i].ID == st.face {
			m.Faces[i].Paint = st.preview
		}
	}
	st.gpu = render.BuildBodyGPU(&m)
	st.gpu.Upload()
	return st.gpu
}
func (a *App) buildMoveTexture() {
	st := &a.moveTexture
	if !st.open || a.UI.ModalOpen() {
		return
	}
	// Keep the face visible by placing the controls at the viewport's lower left.
	box := ui.Rect(a.layout.Viewport.X+a.px(14), a.layout.Viewport.Y+a.layout.Viewport.Height-a.px(230), a.px(350), a.px(216))
	card := a.UI.FloatingCard(ui.MakeID("texture.move"), box, "Move Texture", ui.FloatingCardOpts{Footer: true, ConfirmLabel: "Apply", CancelLabel: "Cancel"})
	body := card.Body
	r, body := ui.SplitTop(body, a.px(28))
	a.UI.Text(r, "Arrow keys move · Shift = 8 pixels", ui.FontSizeSmall, ui.ColorTextDim)
	r, body = ui.SplitTop(body, a.px(25))
	a.UI.Text(r, "Only this face · Paint and PBR move together", ui.FontSizeSmall, ui.ColorTextDim)
	r, body = ui.SplitTop(body, a.px(34))
	w := r.Width / 4
	for i, label := range []string{"Left", "Up", "Down", "Right"} {
		if a.UI.Button(ui.MakeID("texture.move."+label), ui.Rect(r.X+float32(i)*w, r.Y, w-a.px(3), r.Height), label, ui.ButtonOpts{}) {
			switch i {
			case 0:
				a.moveTextureArrow(-1, 0)
			case 1:
				a.moveTextureArrow(0, -1)
			case 2:
				a.moveTextureArrow(0, 1)
			case 3:
				a.moveTextureArrow(1, 0)
			}
		}
	}
	r, _ = ui.SplitTop(body, a.px(28))
	a.UI.Text(r, fmt.Sprintf("Offset: %d, %d px · wraps at edges", st.dx, st.dy), ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.ClaimPointer(a.layout.Screen)
	if card.Cancelled {
		a.finishMoveTexture(false)
	} else if card.Confirmed {
		a.finishMoveTexture(true)
	}
}
