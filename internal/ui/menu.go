package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// The tool flyout of Sketch_func.md §4.1: a chevron beside a button, and the
// short list it opens.
//
// An addition to the frozen widget set of SPEC-UX §4, logged in DECISIONS. It
// earns its place because the sketch toolbar grew from four tools to a dozen
// and more are coming: without grouping, either the toolbar overflows or every
// variant fights for its own letter. The list is deliberately not a general
// menu system — no submenus, no separators, no keyboard traversal. It shows a
// handful of siblings and closes.

// ChevronButton is the little disclosure arrow that opens a flyout. It is
// drawn as part of its group's button rather than as a button in its own
// right, so the pair reads as one control with two hit areas.
func (c *Context) ChevronButton(id ID, r rl.Rectangle, open bool, tooltip string) bool {
	it := c.interact(id, r, false)
	if it.Hovered || open {
		c.FillRounded(r, CornerRadius, ColorHover)
	}
	tint := ColorTextDim
	if it.Hovered || open {
		tint = ColorText
	}
	dir := 1 // down
	if open {
		dir = 3 // up
	}
	ctr := Center(r)
	DrawChevron(float64(ctr.X), float64(ctr.Y), IconSize*c.Scale*0.8, dir, tint)
	c.queueTooltip(id, r, it, tooltip, "", "")
	return it.Clicked
}

// MenuItem is one row of a flyout.
type MenuItem struct {
	Label    string
	Icon     IconFunc
	Shortcut string
	Selected bool
}

// MenuResult reports what the user did with an open flyout.
type MenuResult struct {
	// Chosen is the index picked this frame, or -1.
	Chosen int
	// Dismissed is set when the pointer went somewhere else, which closes the
	// list without choosing.
	Dismissed bool
}

// Menu draws an open flyout and reports the choice.
//
// It draws to the overlay layer so it is never clipped by the toolbar it hangs
// from, and it registers as a card so the viewport underneath does not also
// act on the click — the same two rules every floating thing in this kit obeys.
func (c *Context) Menu(id ID, r rl.Rectangle, items []MenuItem) MenuResult {
	out := MenuResult{Chosen: -1}
	if len(items) == 0 {
		return out
	}
	c.registerCard(r)

	// A press anywhere else closes the list. Checked before the rows so a
	// click that lands on one is a choice rather than a dismissal.
	inside := rl.CheckCollisionPointRec(c.MousePos(), r)
	if c.In.Pressed[MouseLeft] && !inside {
		out.Dismissed = true
	}
	if c.In.KeyPressed(rl.KeyEscape) {
		out.Dismissed = true
	}

	pad := c.Px(4)
	rowH := (r.Height - pad*2) / float32(len(items))
	// Hit-test now, draw later: the rows have to answer clicks in the same
	// frame the list is drawn, and the drawing is deferred above everything.
	for i := range items {
		row := Rect(r.X+pad, r.Y+pad+float32(i)*rowH, r.Width-pad*2, rowH)
		if c.interact(id.Child(items[i].Label), row, false).Clicked {
			out.Chosen = i
		}
	}

	c.Defer(func() {
		c.Shadow(r, CardRadius, 1)
		c.FillRounded(r, CardRadius, ColorCard)
		c.StrokeRounded(r, CardRadius, ColorStroke)
		c.Bevel(r, CardRadius)

		for i, item := range items {
			row := Rect(r.X+pad, r.Y+pad+float32(i)*rowH, r.Width-pad*2, rowH)
			hovered := rl.CheckCollisionPointRec(c.MousePos(), row)
			if hovered {
				c.FillRounded(row, CornerRadius, ColorHover)
			}
			tint := ColorTextDim
			if item.Selected {
				tint = ColorAccent
			} else if hovered {
				tint = ColorText
			}

			iconBox, labelBox := SplitLeft(row, c.Px(IconSize+Spacing))
			if item.Icon != nil {
				ctr := Center(iconBox)
				item.Icon(float64(ctr.X), float64(ctr.Y), IconSize*c.Scale, tint)
			}
			if item.Shortcut != "" {
				var keyBox rl.Rectangle
				keyBox, labelBox = SplitRight(labelBox, c.Px(24))
				c.Text(keyBox, item.Shortcut, FontSizeSmall, Fade(ColorTextDim, 0.8))
			}
			textCol := ColorText
			if item.Selected {
				textCol = ColorAccent
			}
			c.Text(labelBox, item.Label, FontSizeUI, textCol)
		}
	})
	return out
}
