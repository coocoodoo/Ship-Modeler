package ui

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
)

type Control struct {
	ID      string       `json:"id"`
	Kind    string       `json:"kind"`
	Label   string       `json:"label,omitempty"`
	Rect    rl.Rectangle `json:"rect"`
	Enabled bool         `json:"enabled"`
}

func (c *Context) recordControl(id ID, r rl.Rectangle, disabled bool) {
	if !c.TrackControls || c.blocked || r.Width <= 0 || r.Height <= 0 {
		return
	}
	if c.clip != nil {
		x, y := max(r.X, c.clip.X), max(r.Y, c.clip.Y)
		r = Rect(x, y, min(r.X+r.Width, c.clip.X+c.clip.Width)-x, min(r.Y+r.Height, c.clip.Y+c.clip.Height)-y)
		if r.Width <= 0 || r.Height <= 0 {
			return
		}
	}
	c.Controls = append(c.Controls, Control{ID: fmt.Sprint(id), Kind: "control", Rect: r, Enabled: !disabled})
}
func (c *Context) describeControl(id ID, kind, label string) {
	if !c.TrackControls {
		return
	}
	key := fmt.Sprint(id)
	for i := len(c.Controls) - 1; i >= 0; i-- {
		if c.Controls[i].ID == key {
			c.Controls[i].Kind = kind
			c.Controls[i].Label = label
			return
		}
	}
}
