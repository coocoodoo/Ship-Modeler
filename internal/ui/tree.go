package ui

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The tree row of SPEC-UX §7:
//
//	[eye] [type icon] [name] [color swatch (bodies)] [...hover: rename, delete]
//
// Rows for the default planes deliberately have no rename or delete affordance
// at all, because those planes can never be removed (R1) — the absence of the
// buttons is the explanation.

// TreeRowSpec describes one row.
type TreeRowSpec struct {
	Label   string
	Icon    IconFunc
	Visible bool
	// HasEye is false for rows that cannot be hidden.
	HasEye bool
	// Swatch, when non-nil, draws a colour chip for bodies.
	Swatch *color.RGBA
	// CanRename and CanDelete control which hover actions appear. Both are
	// false for the default planes.
	CanRename bool
	CanDelete bool

	Selected bool
	Hovered  bool
	// Indent is the nesting depth in levels.
	Indent int
	// Editing puts the row's name into an inline text field.
	Editing bool
	// Dim draws the row muted, which is how a hidden object reads.
	Dim bool
}

// TreeRowResult reports what the user did to a row this frame.
type TreeRowResult struct {
	Hovered        bool
	Clicked        bool
	DoubleClicked  bool
	ToggledVisible bool
	ClickedSwatch  bool
	ClickedRename  bool
	ClickedDelete  bool
	// Name carries the inline rename's outcome when Editing was set.
	Name FieldResult
}

// TreeRowHeight is the row height in logical pixels.
const TreeRowHeight = 24

// doubleClickMillis is the window two clicks must fall inside.
const doubleClickMillis = 350

// TreeRow draws one row and reports the interactions on it.
func (c *Context) TreeRow(id ID, r rl.Rectangle, spec TreeRowSpec) TreeRowResult {
	var out TreeRowResult
	rowIt := c.interact(id, r, false)
	out.Hovered = rowIt.Hovered

	// Background: selection wins over hover.
	switch {
	case spec.Selected:
		FillRect(r, Fade(ColorAccent, 0.22))
	case rowIt.Hovered || spec.Hovered:
		FillRect(r, ColorHover)
	}
	if spec.Selected {
		FillRect(Rect(r.X, r.Y, c.Px(2), r.Height), ColorAccent)
	}

	text := ColorText
	if spec.Dim {
		text = ColorTextDim
	}

	body := r
	body.X += c.Px(float64(spec.Indent) * 12)
	body.Width -= c.Px(float64(spec.Indent) * 12)

	// Eye toggle on the left.
	eyeBox, rest := SplitLeft(body, c.Px(TreeRowHeight))
	if spec.HasEye {
		if c.Eye(id.Child("eye"), Inset(eyeBox, c.Px(3)), spec.Visible, IconOpts{}) {
			out.ToggledVisible = true
		}
	}

	// Hover actions on the right, so they never shift the label's position.
	actions := rowIt.Hovered && !spec.Editing
	if spec.CanDelete {
		btn, remaining := SplitRight(rest, c.Px(TreeRowHeight))
		rest = remaining
		if actions {
			if c.IconButton(id.Child("delete"), Inset(btn, c.Px(4)), DrawTrashIcon,
				IconOpts{Tooltip: "Delete", Shortcut: "Del"}) {
				out.ClickedDelete = true
			}
		}
	}
	if spec.CanRename {
		btn, remaining := SplitRight(rest, c.Px(TreeRowHeight))
		rest = remaining
		if actions {
			if c.IconButton(id.Child("rename"), Inset(btn, c.Px(4)), DrawPencilIcon,
				IconOpts{Tooltip: "Rename"}) {
				out.ClickedRename = true
			}
		}
	}
	if spec.Swatch != nil {
		sw, remaining := SplitRight(rest, c.Px(TreeRowHeight))
		rest = remaining
		if c.ColorSwatch(id.Child("swatch"), Inset(sw, c.Px(5)), *spec.Swatch) {
			out.ClickedSwatch = true
		}
	}

	// Type icon then label fill the middle.
	iconBox, labelBox := SplitLeft(rest, c.Px(18))
	if spec.Icon != nil {
		ctr := Center(iconBox)
		tint := ColorTextDim
		if spec.Selected {
			tint = ColorAccent
		}
		if spec.Dim {
			tint = Fade(tint, 0.6)
		}
		spec.Icon(float64(ctr.X), float64(ctr.Y), 14*c.Scale, tint)
	}
	labelBox.X += c.Px(2)
	labelBox.Width -= c.Px(2)

	if spec.Editing {
		out.Name = c.TextField(id.Child("name"), InsetXY(labelBox, 0, c.Px(2)), spec.Label,
			TextFieldOpts{SelectAllOnFocus: true})
	} else {
		c.Text(labelBox, spec.Label, FontSizeUI, text)
	}

	if rowIt.Clicked && !spec.Editing {
		out.Clicked = true
		out.DoubleClicked = c.registerClick(id)
	}
	return out
}

// clickTracker remembers the last click so rows can detect double clicks.
type clickTracker struct {
	id    ID
	since float64
}

// registerClick reports whether this click completes a double click.
func (c *Context) registerClick(id ID) bool {
	if c.clicks.id == id && c.clicks.since <= doubleClickMillis {
		c.clicks = clickTracker{}
		return true
	}
	c.clicks = clickTracker{id: id, since: 0}
	return false
}

// TreeSection draws a collapsible section header with its item count.
func (c *Context) TreeSection(id ID, r rl.Rectangle, label string, count int, expanded bool) bool {
	it := c.interact(id, r, false)
	if it.Hovered {
		FillRect(r, ColorHover)
	}

	chev, rest := SplitLeft(r, c.Px(18))
	ctr := Center(chev)
	dir := 0 // pointing right when collapsed
	if expanded {
		dir = 1
	}
	DrawChevron(float64(ctr.X), float64(ctr.Y), 12*c.Scale, dir, ColorTextDim)

	// The count is always shown, including zero: "Sketches · 0" says the
	// section is empty, where a bare heading leaves the reader guessing.
	title := label + " · " + itoa(count)
	c.Text(rest, title, FontSizeHeader, ColorTextDim)
	return it.Clicked
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
