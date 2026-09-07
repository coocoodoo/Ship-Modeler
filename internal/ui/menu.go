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

// MenuItem supports labels, icons, shortcuts, checked items and separators.
type MenuItem struct {
	Label     string
	Icon      IconFunc
	Shortcut  string
	Selected  bool
	Disabled  bool
	Separator bool
}
type MenuResult struct {
	Chosen    int
	Dismissed bool
}
type menuState struct {
	index int
	frame uint64
}

func (c *Context) MenuWasOpen() bool {
	return c.lastMenuFrame > 0 && c.lastMenuFrame+1 >= c.frameNumber
}

func menuIndex(items []MenuItem, start, step int) int {
	for n := 0; n < len(items); n++ {
		start = (start + step + len(items)) % len(items)
		if !items[start].Disabled && !items[start].Separator {
			return start
		}
	}
	return -1
}

func menuBounds(r, screen rl.Rectangle) rl.Rectangle {
	r.Width = min(r.Width, screen.Width-8)
	r.Height = min(r.Height, screen.Height-8)
	r.X = max(screen.X+4, min(r.X, screen.X+screen.Width-r.Width-4))
	r.Y = max(screen.Y+4, min(r.Y, screen.Y+screen.Height-r.Height-4))
	return r
}

func (c *Context) Menu(id ID, r rl.Rectangle, items []MenuItem) MenuResult {
	out := MenuResult{Chosen: -1}
	if len(items) == 0 {
		return out
	}
	screen := c.Screen
	if screen.Width <= 0 {
		screen = Rect(0, 0, 1280, 720)
	}
	r = menuBounds(r, screen)
	c.registerCard(r)
	c.wantKeyboard = true
	c.lastMenuFrame = c.frameNumber
	if c.menuStates == nil {
		c.menuStates = make(map[ID]menuState)
	}
	s, exists := c.menuStates[id]
	if !exists || s.frame+1 < c.frameNumber {
		s.index = menuIndex(items, -1, 1)
		for i, item := range items {
			if item.Selected && !item.Disabled && !item.Separator {
				s.index = i
				break
			}
		}
	}
	if s.index >= len(items) || (s.index >= 0 && (items[s.index].Disabled || items[s.index].Separator)) {
		s.index = menuIndex(items, -1, 1)
	}
	inside := rl.CheckCollisionPointRec(c.MousePos(), r)
	if (c.In.Pressed[MouseLeft] && !inside) || c.In.KeyPressed(rl.KeyEscape) || c.In.FocusLost {
		out.Dismissed = true
	}
	if c.In.KeyPressed(rl.KeyDown) {
		s.index = menuIndex(items, s.index, 1)
	}
	if c.In.KeyPressed(rl.KeyUp) {
		s.index = menuIndex(items, s.index, -1)
	}
	if c.In.KeyPressed(rl.KeyHome) {
		s.index = menuIndex(items, -1, 1)
	}
	if c.In.KeyPressed(rl.KeyEnd) {
		s.index = menuIndex(items, 0, -1)
	}
	if !out.Dismissed && s.index >= 0 && (c.In.KeyPressed(rl.KeyEnter) || c.In.KeyPressed(rl.KeySpace)) {
		out.Chosen = s.index
	}
	pad := c.Px(4)
	units := float32(0)
	for _, item := range items {
		if item.Separator {
			units += .3
		} else {
			units++
		}
	}
	rowH := (r.Height - 2*pad) / max(1, units)
	rows := make([]rl.Rectangle, len(items))
	y := r.Y + pad
	previousClip := c.clip
	c.clip = nil
	for i, item := range items {
		h := rowH
		if item.Separator {
			h *= .3
		}
		row := Rect(r.X+pad, y, r.Width-pad*2, h)
		rows[i] = row
		y += h
		if item.Separator {
			continue
		}
		cid := id.Child(itoa(i) + item.Label)
		it := c.interact(cid, row, item.Disabled)
		c.describeControl(cid, "menu item", item.Label)
		if it.Hovered && !item.Disabled && (c.In.MouseDX != 0 || c.In.MouseDY != 0) {
			s.index = i
		}
		if it.Clicked && !out.Dismissed {
			out.Chosen = i
		}
	}
	c.clip = previousClip
	s.frame = c.frameNumber
	c.menuStates[id] = s
	c.ClaimPointer(r)
	amount := c.entrance(id, c.Effects.MenuAnimation)
	offset := c.Px(6) * float32(1-amount)
	c.Defer(func() {
		drawBox := r
		drawBox.Y += offset
		c.Shadow(drawBox, CardRadius, 1)
		c.FillRounded(drawBox, CardRadius, ColorCard)
		c.StrokeRounded(drawBox, CardRadius, ColorStroke)
		for i, item := range items {
			row := rows[i]
			row.Y += offset
			if item.Separator {
				c.HairlineH(row.X+c.Px(6), row.Y+row.Height/2, row.Width-c.Px(12), ColorStroke)
				continue
			}
			if i == s.index && !item.Disabled {
				c.FillRounded(row, CornerRadius, Fade(ColorAccent, .15))
			}
			tint := ColorText
			if item.Disabled {
				tint = Fade(ColorTextDim, .45)
			}
			iconBox, labelBox := SplitLeft(row, c.Px(IconSize+Spacing))
			ctr := Center(iconBox)
			if item.Selected {
				DrawCheckIcon(float64(ctr.X), float64(ctr.Y), IconSize*c.Scale, ColorAccent)
			} else if item.Icon != nil {
				item.Icon(float64(ctr.X), float64(ctr.Y), IconSize*c.Scale, tint)
			}
			if item.Shortcut != "" {
				var keyBox rl.Rectangle
				keyBox, labelBox = SplitRight(labelBox, min(labelBox.Width*.4, c.TextWidth(item.Shortcut, FontSizeSmall)+c.Px(8)))
				c.Text(keyBox, item.Shortcut, FontSizeSmall, ColorTextDim)
			}
			c.Text(labelBox, item.Label, FontSizeUI, tint)
		}
	})
	return out
}

type selectSpec struct {
	rect     rl.Rectangle
	labels   []string
	selected int
}

// Prepare dropdowns before other widgets so an upward-opening list cannot
// click controls already drawn beneath it.
func (c *Context) prepareDropdown() {
	c.selectResultID = NoID
	if c.selectOpen == NoID {
		return
	}
	id := c.selectOpen
	spec, ok := c.selectSpecs[id]
	if !ok || c.In.FocusLost || c.In.Wheel != 0 {
		c.selectOpen = NoID
		return
	}
	items := make([]MenuItem, len(spec.labels))
	for i, label := range spec.labels {
		items[i] = MenuItem{Label: label, Selected: i == spec.selected}
	}
	r := spec.rect
	menu := Rect(r.X, r.Y+r.Height+c.Px(4), r.Width, c.Px(float64(8+28*len(items))))
	screen := c.Screen
	if screen.Width <= 0 {
		screen = Rect(0, 0, 1280, 720)
	}
	if menu.Y+menu.Height > screen.Y+screen.Height-c.Px(4) {
		menu.Y = r.Y - menu.Height - c.Px(4)
	}
	result := c.Menu(id.Child("menu"), menu, items)
	if result.Dismissed || result.Chosen >= 0 {
		c.selectOpen = NoID
	}
	if result.Dismissed && c.In.Pressed[MouseLeft] && rl.CheckCollisionPointRec(c.MousePos(), r) {
		c.selectSuppress = id
	}
	if result.Chosen >= 0 {
		c.selectResultID = id
		c.selectResult = result.Chosen
	}
}

// Select is a keyboard-accessible dropdown with a trailing chevron.
func (c *Context) Select(id ID, r rl.Rectangle, labels []string, selected int) (int, bool) {
	if len(labels) == 0 {
		return selected, false
	}
	selected = max(0, min(selected, len(labels)-1))
	if c.selectSpecs == nil {
		c.selectSpecs = make(map[ID]selectSpec)
	}
	c.selectSpecs[id] = selectSpec{r, labels, selected}
	changed := false
	if c.selectResultID == id {
		selected = c.selectResult
		changed = true
		c.selectResultID = NoID
	}
	chevron := func(x, y, size float64, col rl.Color) { DrawChevron(x, y, size*.65, 1, col) }
	if c.Button(id, r, labels[selected], ButtonOpts{IconEnd: chevron}) {
		if c.selectOpen == id || c.selectSuppress == id {
			c.selectOpen = NoID
			c.selectSuppress = NoID
		} else {
			c.selectOpen = id
		}
	}
	return selected, changed
}
