package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"math"
)

// Motion controls visual feedback only. Input, geometry and undo apply immediately.
func (c *Context) MotionFactor() float64 {
	switch c.Motion {
	case "off":
		return 0
	case "slow":
		return .5
	default:
		return 1
	}
}

func (c *Context) easeAmount(id ID, target, millis float64, initialize bool) float64 {
	if c.hoverTransitions == nil {
		c.hoverTransitions = make(map[ID]hoverTransition)
	}
	s, found := c.hoverTransitions[id]
	if (!found && initialize) || c.MotionFactor() == 0 {
		s.value = target
		s.velocity = 0
	} else {
		k, d := 0.0, 0.0
		switch c.Effects.MotionStyle {
		case "spring":
			k, d = 200, 18
		case "bouncy":
			k, d = 300, 10
		}
		if custom, ok := c.springs[id]; ok {
			k, d = custom[0], custom[1]
		}
		s = advanceTween(s, target, max(0, c.In.DeltaMillis)*c.MotionFactor()/1000, millis, k, d)
	}
	s.frame = c.frameNumber
	s.target = target
	c.hoverTransitions[id] = s
	return s.value
}

type appearanceTransition struct {
	from, to themeSnapshot
	elapsed  float64
}

func (c *Context) Animating() bool {
	if c.appearance != nil || len(c.ripples) > 0 {
		return true
	}
	for _, s := range c.hoverTransitions {
		if s.finite && s.frame+1 >= c.frameNumber && (math.Abs(s.value-s.target) > .0001 || math.Abs(s.velocity) > .01) {
			return true
		}
	}
	return false
}

// TransitionTheme starts from the currently displayed colors, including when
// the user chooses another palette before the previous transition finishes.
func (c *Context) TransitionTheme(apply func()) {
	from := snapshotTheme()
	apply()
	to := snapshotTheme()
	if c.MotionFactor() == 0 || !c.Effects.ThemeFade {
		c.appearance = nil
		return
	}
	c.appearance = &appearanceTransition{from: from, to: to}
	from.restore()
}

func (c *Context) AdvanceAppearance(delta float64) {
	s := c.appearance
	if s == nil {
		return
	}
	s.elapsed += math.Max(0, delta) * c.MotionFactor()
	t := math.Min(1, s.elapsed/280)
	if c.MotionFactor() == 0 || !c.Effects.ThemeFade {
		t = 1
	}
	if t == 1 {
		s.to.restore()
		c.appearance = nil
		return
	}
	t = t * t * (3 - 2*t)
	f, to := s.from, s.to
	m := func(a, b color.RGBA) color.RGBA { return blendColor(a, b, t) }
	themeSnapshot{
		bg: m(f.bg, to.bg), panel: m(f.panel, to.panel), card: m(f.card, to.card), stroke: m(f.stroke, to.stroke),
		text: m(f.text, to.text), textDim: m(f.textDim, to.textDim), warn: m(f.warn, to.warn), errorC: m(f.errorC, to.errorC), success: m(f.success, to.success),
		viewportTop: m(f.viewportTop, to.viewportTop), viewportBottom: m(f.viewportBottom, to.viewportBottom),
		model: m(f.model, to.model), sketch: m(f.sketch, to.sketch), extrude: m(f.extrude, to.extrude), boolean: m(f.boolean, to.boolean), paint: m(f.paint, to.paint), marker: m(f.marker, to.marker),
		axisX: m(f.axisX, to.axisX), axisY: m(f.axisY, to.axisY), axisZ: m(f.axisZ, to.axisZ), accent: m(f.accent, to.accent), accentSoft: m(f.accentSoft, to.accentSoft),
		gridMinor: m(f.gridMinor, to.gridMinor), gridMajor: m(f.gridMajor, to.gridMajor), hover: m(f.hover, to.hover), bevel: m(f.bevel, to.bevel), shadow: m(f.shadow, to.shadow),
	}.restore()
}

type clickRipple struct{ x, y, age float64 }

func (c *Context) stepRipples(delta float64) {
	for id, r := range c.ripples {
		r.age += math.Max(0, delta) * c.MotionFactor()
		if r.age >= 320 || c.MotionFactor() == 0 || !c.Effects.Ripple {
			delete(c.ripples, id)
		} else {
			c.ripples[id] = r
		}
	}
}

func (c *Context) buttonFeedback(id ID, r rl.Rectangle, it Interaction) rl.Rectangle {
	target := 0.0
	if it.Pressed && !it.Disabled {
		target = 1
	}
	press := c.easeAmount(id.Child("press"), target, 40, false)
	if c.MotionFactor() > 0 && c.Effects.PressEffect == "subtle" {
		// Content moves with the surface; the original hit area stays fixed.
		r = InsetXY(r, c.Px(1)*float32(press), c.Px(.5)*float32(press))
		r.Y += c.Px(.6) * float32(press)
	}
	if it.Clicked && c.MotionFactor() > 0 && c.Effects.Ripple {
		if c.ripples == nil {
			c.ripples = make(map[ID]clickRipple)
		}
		c.ripples[id] = clickRipple{x: c.In.MouseX, y: c.In.MouseY}
	}
	return r
}

func (c *Context) drawRipple(id ID, r rl.Rectangle, col color.RGBA) {
	s, ok := c.ripples[id]
	if !ok || c.MotionFactor() == 0 || !c.Effects.Ripple {
		return
	}
	t := s.age / 320
	radius := math.Hypot(float64(r.Width), float64(r.Height)) * (1 - math.Pow(1-t, 3))
	polygon := make([]rl.Vector2, 48)
	for i := range polygon {
		a := float64(i) * 2 * math.Pi / float64(len(polygon))
		polygon[i] = rl.Vector2{X: float32(s.x + radius*math.Cos(a)), Y: float32(s.y + radius*math.Sin(a))}
	}
	boundary := roundedBoundary(r, min(c.Px(CornerRadius), r.Width/2, r.Height/2))
	polygon = clipConvex(polygon, boundary[:])
	if len(polygon) < 3 {
		return
	}
	ink := Fade(col, .12*(1-t))
	rl.SetTexture(0)
	rl.Begin(rl.Triangles)
	rl.Color4ub(ink.R, ink.G, ink.B, ink.A)
	for i := 1; i+1 < len(polygon); i++ {
		rl.Vertex2f(polygon[0].X, polygon[0].Y)
		rl.Vertex2f(polygon[i+1].X, polygon[i+1].Y)
		rl.Vertex2f(polygon[i].X, polygon[i].Y)
	}
	rl.End()
}

func roundedBoundary(r rl.Rectangle, rad float32) [36]rl.Vector2 {
	centers := [4]rl.Vector2{{X: r.X + rad, Y: r.Y + rad}, {X: r.X + r.Width - rad, Y: r.Y + rad}, {X: r.X + r.Width - rad, Y: r.Y + r.Height - rad}, {X: r.X + rad, Y: r.Y + r.Height - rad}}
	var points [36]rl.Vector2
	for corner, center := range centers {
		for segment := 0; segment <= 8; segment++ {
			a := math.Pi + float64(corner)*math.Pi/2 + float64(segment)*math.Pi/16
			points[corner*9+segment] = rl.Vector2{X: center.X + rad*float32(math.Cos(a)), Y: center.Y + rad*float32(math.Sin(a))}
		}
	}
	return points
}

func clipConvex(poly, boundary []rl.Vector2) []rl.Vector2 {
	for i, a := range boundary {
		if len(poly) == 0 {
			break
		}
		b := boundary[(i+1)%len(boundary)]
		distance := func(p rl.Vector2) float32 { return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X) }
		out := make([]rl.Vector2, 0, len(poly)+2)
		previous := poly[len(poly)-1]
		pd := distance(previous)
		for _, p := range poly {
			d := distance(p)
			if (d >= 0) != (pd >= 0) {
				t := pd / (pd - d)
				out = append(out, rl.Vector2{X: previous.X + (p.X-previous.X)*t, Y: previous.Y + (p.Y-previous.Y)*t})
			}
			if d >= 0 {
				out = append(out, p)
			}
			previous, pd = p, d
		}
		poly = out
	}
	return poly
}
func intersection(a, b rl.Rectangle) rl.Rectangle {
	x, y := max(a.X, b.X), max(a.Y, b.Y)
	return Rect(x, y, min(a.X+a.Width, b.X+b.Width)-x, min(a.Y+a.Height, b.Y+b.Height)-y)
}

// FillGradientRounded draws a colored triangle fan, so gradients have the same
// antialiased silhouette as other controls without an offscreen texture.
func (c *Context) FillGradientRounded(r rl.Rectangle, radius float64, from, to color.RGBA) {
	if !c.Effects.Gradients {
		c.FillRounded(r, radius, from)
		return
	}
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	rad := min(c.Px(radius), r.Height/2, r.Width/2)
	centers := [4]rl.Vector2{{X: r.X + rad, Y: r.Y + rad}, {X: r.X + r.Width - rad, Y: r.Y + rad}, {X: r.X + r.Width - rad, Y: r.Y + r.Height - rad}, {X: r.X + rad, Y: r.Y + r.Height - rad}}
	var points [36]rl.Vector2
	for corner, center := range centers {
		for segment := 0; segment <= 8; segment++ {
			angle := math.Pi + float64(corner)*math.Pi/2 + float64(segment)*math.Pi/16
			points[corner*9+segment] = rl.Vector2{X: center.X + rad*float32(math.Cos(angle)), Y: center.Y + rad*float32(math.Sin(angle))}
		}
	}
	vertex := func(p rl.Vector2) {
		ink := blendColor(from, to, clamp01(float64((p.X-r.X)/r.Width)))
		rl.Color4ub(ink.R, ink.G, ink.B, ink.A)
		rl.Vertex2f(p.X, p.Y)
	}
	rl.SetTexture(0)
	rl.Begin(rl.Triangles)
	for i, p := range points {
		vertex(Center(r))
		vertex(points[(i+1)%len(points)])
		vertex(p)
	}
	rl.End()
}
