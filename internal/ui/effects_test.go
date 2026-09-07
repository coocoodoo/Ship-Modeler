package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
	"testing"
)

func TestSpringRemainsStableAndOvershoots(t *testing.T) {
	for _, dt := range []float64{1.0 / 240, 1.0 / 60, .1, .25} {
		s := hoverTransition{}
		peak := 0.0
		for elapsed := 0.0; elapsed < 5; elapsed += dt {
			s = advanceTween(s, 1, dt, 65, 300, 10)
			peak = max(peak, s.value)
			if math.IsNaN(s.value) || math.Abs(s.value) > 2 {
				t.Fatal("unstable spring", dt, s)
			}
		}
		if peak <= 1.02 || math.Abs(s.value-1) > .001 || math.Abs(s.velocity) > .02 {
			t.Fatalf("spring did not overshoot and settle at dt=%v: peak %v, %+v", dt, peak, s)
		}
	}
}

func TestMenuSkipsDisabledAndSeparatorsAndCloses(t *testing.T) {
	c := NewContext(nil, 1)
	c.Effects.MenuAnimation = false
	items := []MenuItem{{Label: "First"}, {Separator: true}, {Label: "Disabled", Disabled: true}, {Label: "Last", Selected: true}}
	id := MakeID("test.menu")
	r := Rect(10, 10, 240, 120)
	frame := func(key int32, focusLost bool) MenuResult {
		in := Input{DeltaMillis: 16, FocusLost: focusLost}
		if key != 0 {
			in.KeysPressed = []int32{key}
		}
		c.Begin(in)
		return c.Menu(id, r, items)
	}
	frame(rl.KeyHome, false)
	frame(rl.KeyDown, false)
	if got := frame(rl.KeyEnter, false); got.Chosen != 3 {
		t.Fatal("keyboard chose disabled item", got)
	}
	frame(rl.KeyEnd, false)
	frame(rl.KeyUp, false)
	if got := frame(rl.KeySpace, false); got.Chosen != 0 {
		t.Fatal("keyboard did not skip separator", got)
	}
	if !frame(rl.KeyEscape, false).Dismissed || !frame(0, true).Dismissed {
		t.Fatal("menu did not close")
	}
	bounded := menuBounds(Rect(1260, 710, 240, 120), Rect(0, 0, 1280, 720))
	if bounded.X < 0 || bounded.Y < 0 || bounded.X+bounded.Width > 1280 || bounded.Y+bounded.Height > 720 {
		t.Fatal("menu escaped window")
	}
}

func TestTooltipInterruptionsAndToggle(t *testing.T) {
	for _, in := range []Input{{Wheel: 1}, {KeysPressed: []int32{rl.KeyDown}}, {Pressed: [3]bool{true}}, {FocusLost: true}} {
		c := NewContext(nil, 1)
		c.tip = tooltipState{id: 1, text: "tip", waited: 600, seen: true}
		c.In = in
		c.stepTooltip(16)
		c.queueTooltip(1, Rect(0, 0, 100, 30), Interaction{Hovered: true}, "tip", "", "")
		if c.tip.id != NoID {
			t.Fatal("input failed to suppress tooltip", in)
		}
	}
	c := NewContext(nil, 1)
	c.Effects.Tooltips = false
	c.queueTooltip(1, Rect(0, 0, 100, 30), Interaction{Hovered: true}, "tip", "", "")
	if c.tip.id != NoID {
		t.Fatal("disabled tooltip appeared")
	}
}

func TestDropdownFlipsUpAndBlocksUnderlyingControls(t *testing.T) {
	c := NewContext(nil, 1)
	c.Effects.MenuAnimation = false
	c.Screen = Rect(0, 0, 1280, 720)
	id := MakeID("select")
	r := Rect(10, 670, 200, 30)
	c.selectOpen = id
	c.selectSpecs = map[ID]selectSpec{id: {rect: r, labels: []string{"First", "Second", "Third"}}}
	c.Begin(Input{MouseX: 40, MouseY: 620, Pressed: [3]bool{true}, DeltaMillis: 16})
	if len(c.cards) != 1 || c.cards[0].Y+c.cards[0].Height > r.Y {
		t.Fatal("dropdown did not flip up", c.cards)
	}
	if it := c.interact(MakeID("behind"), Rect(10, 600, 200, 50), false); it.Hovered || it.Pressed {
		t.Fatal("dropdown clicked a control behind it")
	}
	c.ClearFocus()
	if c.selectOpen != NoID {
		t.Fatal("closed settings left a dropdown open")
	}
}
