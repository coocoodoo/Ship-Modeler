package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"math"
)

// Patina's semi-implicit spring integration, with bounded 4 ms substeps.
func advanceTween(s hoverTransition, target, dt, millis, k, d float64) hoverTransition {
	dt = min(.25, max(0, dt))
	if k > 0 {
		steps := max(1, int(math.Ceil(dt/.004)))
		h := dt / float64(steps)
		for i := 0; i < steps; i++ {
			s.velocity += (-k*(s.value-target) - d*s.velocity) * h
			s.value += s.velocity * h
		}
	} else {
		s.velocity = 0
		s.value += (target - s.value) * (1 - math.Exp(-dt*1000/millis))
	}
	if math.Abs(s.value-target) < .001 && math.Abs(s.velocity) < .02 {
		s.value = target
		s.velocity = 0
	}
	return s
}

// Spring overrides the global motion style for a widget's value animation.
func (c *Context) Spring(id ID, stiffness, damping float64) {
	if c.springs == nil {
		c.springs = make(map[ID][2]float64)
	}
	c.springs[id.Child("value")] = [2]float64{max(20, min(600, stiffness)), max(4, min(60, damping))}
}

func (c *Context) effectAccent(base color.RGBA) color.RGBA {
	if !c.Effects.ColorCycle || c.MotionFactor() == 0 {
		return base
	}
	return rl.ColorFromHSV(float32(math.Mod(c.visualClock/20, 360)), .55, .95)
}

func (c *Context) buttonGlow(r rl.Rectangle, accent color.RGBA, enabled bool) {
	if !enabled || !c.Effects.Glow {
		return
	}
	pulse := 1.0
	if c.MotionFactor() > 0 {
		pulse = .7 + .3*math.Sin(c.visualClock/650)
	}
	for i := 4; i >= 1; i-- {
		c.StrokeRounded(Inset(r, -c.Px(float64(i))), CornerRadius+float64(i), Fade(accent, pulse*.20/float64(i)))
	}
}

func (c *Context) buttonTransform(id ID, r rl.Rectangle, it Interaction) func() {
	if c.MotionFactor() == 0 || it.Disabled {
		return func() {}
	}
	p := c.hoverTransitions[id.Child("press")].value
	kickID := id.Child("release")
	kick, exists := c.hoverTransitions[kickID]
	if it.Clicked {
		kick = hoverTransition{value: c.visualClock}
		exists = true
	}
	age := c.visualClock - kick.value
	kick.frame = c.frameNumber
	if exists && age < 600 {
		c.hoverTransitions[kickID] = kick
	} else {
		delete(c.hoverTransitions, kickID)
	}
	wave := 0.0
	if exists && age < 600 {
		wave = math.Exp(-age/110) * math.Sin(age/32)
	}
	sx, sy, angle := 1.0, 1.0, 0.0
	switch c.Effects.PressEffect {
	case "bounce":
		sx = 1 - .055*p + .045*wave
		sy = sx
	case "rubber":
		sx = 1 + .055*p - .05*wave
		sy = 1 - .13*p + .13*wave
	case "gelatin":
		sx = 1 + .04*p + .04*wave
		sy = 1 - .09*p - .07*wave
		angle = 3*wave - 1.5*p
	}
	lift := 0.0
	if c.Effects.HoverLift {
		lift = -float64(c.Px(1.5)) * clamp01(c.hoverTransitions[id].value)
	}
	ctr := Center(r)
	rl.PushMatrix()
	rl.Translatef(ctr.X, ctr.Y+float32(lift), 0)
	rl.Rotatef(float32(angle), 0, 0, 1)
	rl.Scalef(float32(sx), float32(sy), 1)
	rl.Translatef(-ctr.X, -ctr.Y, 0)
	return rl.PopMatrix
}

func (c *Context) animatedIcon(icon IconFunc, cx, cy, size float64, col color.RGBA, animate bool) {
	if icon == nil {
		return
	}
	angle, k := 0.0, 1.0
	if animate && c.MotionFactor() > 0 {
		if c.Effects.IconSpin {
			angle = math.Mod(c.visualClock*.045, 360)
		}
		if c.Effects.IconPulse {
			k = 1 + .055*math.Sin(c.visualClock/300)
		}
	}
	rl.PushMatrix()
	rl.Translatef(float32(cx), float32(cy), 0)
	rl.Rotatef(float32(angle), 0, 0, 1)
	rl.Scalef(float32(k), float32(k), 1)
	rl.Translatef(-float32(cx), -float32(cy), 0)
	icon(cx, cy, size, col)
	rl.PopMatrix()
}

func (c *Context) entrance(id ID, enabled bool) float64 {
	if !enabled || c.MotionFactor() == 0 {
		return 1
	}
	key := id.Child("entrance")
	if s, ok := c.hoverTransitions[key]; ok && s.frame+1 < c.frameNumber {
		delete(c.hoverTransitions, key)
	}
	value := c.easeAmount(key, 1, 65, false)
	s := c.hoverTransitions[key]
	s.finite = true
	c.hoverTransitions[key] = s
	return value
}

func (c *Context) Progress(id ID, r rl.Rectangle, value float64) {
	c.FillRounded(r, CornerRadius, ColorStroke)
	t := c.easeAmount(id.Child("value"), clamp01(value), 75, true)
	fill := r
	fill.Width *= float32(clamp01(t))
	c.FillGradientRounded(fill, CornerRadius, ColorAccent, blendColor(ColorAccent, ColorCard, .15))
}

// The frames are baked by Patina's actual SMIL interpreter, including mpath
// and automatic tangent rotation, and share Modeler's clock and texture cache.
func (c *Context) MotionSVG(r rl.Rectangle) {
	name := "beacon"
	if c.Effects.PathMotion {
		name = "orbit"
	}
	key := svgTexKey{name: "motion:" + name}
	tex, ok := svgTextures[key]
	if !ok {
		data, err := patinaIcons.ReadFile("patina/" + name + "-motion.png")
		if err != nil {
			return
		}
		img := rl.LoadImageFromMemory(".png", data, int32(len(data)))
		if img == nil {
			return
		}
		tex = rl.LoadTextureFromImage(img)
		rl.UnloadImage(img)
		rl.SetTextureFilter(tex, rl.FilterBilinear)
		svgTextures[key] = tex
	}
	frame := 0
	if c.Effects.SVGAnimation && c.MotionFactor() > 0 {
		frame = int(c.visualClock*.03) % 120
	}
	src := Rect(float32(frame%8)*192, float32(frame/8)*64, 192, 64)
	rl.DrawTexturePro(tex, src, r, rl.Vector2{}, 0, rl.White)
}
