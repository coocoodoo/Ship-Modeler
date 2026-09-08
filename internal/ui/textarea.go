package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
	"strings"
)

// TextArea is a wrapped, clipped note editor. Enter inserts a line break;
// clicking outside commits. It shares the toolkit's selection and undo-free
// editing buffer, so document history only changes when the dialog saves.
func (c *Context) TextArea(id ID, r rl.Rectangle, text, placeholder string) FieldResult {
	it := c.interact(id, r, false)
	c.describeControl(id, "text", text+" "+placeholder)
	if it.Clicked && c.focus != id {
		c.beginEdit(id, text, false)
	}
	res := FieldResult{Text: text}
	if c.focus == id && c.In.Pressed[MouseLeft] && !it.Hovered {
		res.Text = c.edit.String()
		res.Committed = true
		c.ClearFocus()
	}
	editing := c.focus == id
	border := ColorStroke
	if editing {
		border = ColorAccent
	}
	c.FillRounded(r, CornerRadius, ColorCard)
	c.StrokeRounded(r, CornerRadius, border)
	inner := Inset(r, c.Px(8))
	lineH := c.Fonts.LineHeight(FontSizeUI)
	if lineH <= 0 || inner.Width <= 0 {
		return res
	}
	changed := false
	if editing {
		originalKeys := c.In.KeysPressed
		filtered := make([]int32, 0, len(originalKeys))
		for _, key := range originalKeys {
			switch {
			case key == rl.KeyEnter || key == rl.KeyKpEnter:
				c.edit.insert([]rune{'\n'})
				changed = true
			case key == rl.KeyV && c.In.Ctrl:
				c.edit.insert([]rune(strings.ReplaceAll(rl.GetClipboardText(), "\r\n", "\n")))
				changed = true
			default:
				filtered = append(filtered, key)
			}
		}
		c.In.KeysPressed = filtered
		res = c.handleTextKeys()
		c.In.KeysPressed = originalKeys
		if res.Cancelled {
			c.ClearFocus()
			return res
		}
		text = c.edit.String()
		res.Text = text
		res.Editing = true
		c.edit.blink += c.In.DeltaMillis
		changed = changed || len(originalKeys) > 0 || len(c.In.Chars) > 0
	}
	chars := []rune(text)
	positions := make([]rl.Vector2, len(chars)+1)
	x, y := float32(0), float32(0)
	for i, ch := range chars {
		w := c.TextWidth(string(ch), FontSizeUI)
		if ch != '\n' && x > 0 && x+w > inner.Width {
			x = 0
			y += lineH
		}
		positions[i] = rl.Vector2{X: x, Y: y}
		if ch == '\n' {
			x = 0
			y += lineH
		} else {
			x += w
		}
	}
	positions[len(chars)] = rl.Vector2{X: x, Y: y}
	scroll := float32(0)
	if editing {
		if it.Hovered && c.In.Wheel != 0 {
			c.edit.scrollY -= float32(c.In.Wheel) * lineH * 3
		}
		if it.Clicked {
			best := float64(math.Inf(1))
			caret := 0
			for i, p := range positions {
				dx := float64(inner.X+p.X) - c.In.MouseX
				dy := float64(inner.Y+p.Y+lineH/2-c.edit.scrollY) - c.In.MouseY
				d := dx*dx + dy*dy*16
				if d < best {
					best = d
					caret = i
				}
			}
			c.edit.caret = caret
			c.edit.sel = -1
		}
		if changed {
			cy := positions[c.edit.caret].Y
			if cy < c.edit.scrollY {
				c.edit.scrollY = cy
			}
			if cy+lineH > c.edit.scrollY+inner.Height {
				c.edit.scrollY = cy + lineH - inner.Height
			}
		}
		c.edit.scrollY = max(0, min(c.edit.scrollY, y+lineH-inner.Height))
		scroll = c.edit.scrollY
	}
	c.Clip(inner, func() {
		if len(chars) == 0 {
			c.Text(inner, placeholder, FontSizeUI, ColorTextDim)
		}
		start, end, selected := c.edit.selRange()
		font := c.fontFor(FontSizeUI)
		for i, ch := range chars {
			p := positions[i]
			px, py := inner.X+p.X, inner.Y+p.Y-scroll
			if py+lineH < inner.Y || py > inner.Y+inner.Height {
				continue
			}
			w := c.TextWidth(string(ch), FontSizeUI)
			if editing && selected && i >= start && i < end {
				FillRect(Rect(px, py, max(w, c.Px(3)), lineH), Fade(ColorAccent, .4))
			}
			if ch != '\n' {
				c.Fonts.Draw(font, string(ch), px, py, FontSizeUI, ColorText)
			}
		}
		if editing && (c.MotionFactor() == 0 || math.Mod(c.edit.blink*c.MotionFactor(), 1000) < 600 || changed) {
			p := positions[c.edit.caret]
			FillRect(Rect(inner.X+p.X, inner.Y+p.Y-scroll, c.hairline(), lineH), ColorAccent)
		}
	})
	return res
}
