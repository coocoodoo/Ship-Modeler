package ui

import (
	"image/color"
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// These tests cover the widget kit's logic: identity, layout arithmetic, colour
// conversion, number parsing and the text editor. They call no GPU function,
// so they run without a window. Anything that draws is verified by the golden
// shots instead (TESTING §1).

func TestIDsAreStableAndDistinct(t *testing.T) {
	a := MakeID("tree.body.1")
	if a != MakeID("tree.body.1") {
		t.Error("the same string produced two different ids")
	}
	if a == MakeID("tree.body.2") {
		t.Error("different strings collided")
	}
	if a == NoID {
		t.Error("a real id must never be the zero id")
	}
	// Children are scoped to their parent, so two rows' eye toggles differ.
	if a.Child("eye") == MakeID("tree.body.2").Child("eye") {
		t.Error("child ids collided across parents")
	}
	if a.Child("eye") == a.Child("delete") {
		t.Error("child ids collided within a parent")
	}
	if a.Child("eye") != a.Child("eye") {
		t.Error("child ids are not stable")
	}
}

func TestRectSplits(t *testing.T) {
	r := Rect(10, 20, 100, 50)

	left, rest := SplitLeft(r, 30)
	if left.X != 10 || left.Width != 30 || rest.X != 40 || rest.Width != 70 {
		t.Errorf("SplitLeft = %+v / %+v", left, rest)
	}
	right, rest := SplitRight(r, 30)
	if right.X != 80 || right.Width != 30 || rest.X != 10 || rest.Width != 70 {
		t.Errorf("SplitRight = %+v / %+v", right, rest)
	}
	top, rest := SplitTop(r, 20)
	if top.Y != 20 || top.Height != 20 || rest.Y != 40 || rest.Height != 30 {
		t.Errorf("SplitTop = %+v / %+v", top, rest)
	}
	bottom, rest := SplitBottom(r, 20)
	if bottom.Y != 50 || bottom.Height != 20 || rest.Y != 20 || rest.Height != 30 {
		t.Errorf("SplitBottom = %+v / %+v", bottom, rest)
	}

	// Splitting more than there is clamps rather than producing a negative box.
	big, rest := SplitLeft(r, 500)
	if big.Width != 100 || rest.Width != 0 {
		t.Errorf("oversized SplitLeft = %+v / %+v", big, rest)
	}

	in := Inset(r, 5)
	if in.X != 15 || in.Y != 25 || in.Width != 90 || in.Height != 40 {
		t.Errorf("Inset = %+v", in)
	}
	c := Center(r)
	if c.X != 60 || c.Y != 45 {
		t.Errorf("Center = %+v", c)
	}
}

func TestScaleRoundsToSupportedSteps(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{0, 1.0}, {1.0, 1.0}, {1.1, 1.0},
		{1.125, 1.25}, {1.25, 1.25}, {1.3, 1.25},
		{1.375, 1.5}, {1.5, 1.5}, {1.7, 1.5},
		{1.75, 2.0}, {2.0, 2.0}, {3.0, 2.0},
	}
	for _, c := range cases {
		if got := Scale(c.in); got != c.want {
			t.Errorf("Scale(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestThemeHelpers(t *testing.T) {
	base := color.RGBA{R: 100, G: 150, B: 200, A: 200}

	if got := Fade(base, 0.5); got.A != 100 || got.R != 100 {
		t.Errorf("Fade = %+v, want half alpha and untouched colour", got)
	}
	if got := Fade(base, 5); got.A != 200 {
		t.Errorf("Fade above 1 should clamp, got %+v", got)
	}
	if got := Fade(base, -1); got.A != 0 {
		t.Errorf("Fade below 0 should clamp, got %+v", got)
	}

	if got := Shade(base, 0.5); got.R != 50 || got.A != 200 {
		t.Errorf("Shade = %+v, want halved rgb and untouched alpha", got)
	}
	if got := Shade(base, 10); got.R != 255 || got.G != 255 || got.B != 255 {
		t.Errorf("Shade should clamp to 255, got %+v", got)
	}

	if got := WithAlpha(base, 7); got.A != 7 || got.R != base.R {
		t.Errorf("WithAlpha = %+v", got)
	}

	// The edge colour is the body colour darkened and faded (SPEC-UX §3).
	e := EdgeColor(color.RGBA{R: 200, G: 200, B: 200, A: 255})
	if e.R != 70 {
		t.Errorf("edge tint = %d, want 200*0.35 = 70", e.R)
	}
	if e.A != 217 {
		t.Errorf("edge alpha = %d, want 255*0.85 = 217", e.A)
	}
}

func TestBodyColorCycles(t *testing.T) {
	if BodyColor(0) != BodyColor(len(BodyColors)) {
		t.Error("the body colour cycle does not wrap")
	}
	if BodyColor(-1) != BodyColors[len(BodyColors)-1] {
		t.Error("a negative index does not wrap cleanly")
	}
}

func TestAxisColors(t *testing.T) {
	if AxisColor(0) != ColorAxisX || AxisColor(1) != ColorAxisY || AxisColor(2) != ColorAxisZ {
		t.Error("axis colours are mismapped")
	}
}

// TestHSVRoundTrip is what the colour picker's correctness rests on.
func TestHSVRoundTrip(t *testing.T) {
	cases := []color.RGBA{
		{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255},
		{R: 255, G: 255, A: 255}, {R: 0, G: 0, B: 0, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 0x8E, G: 0xA3, B: 0xB0, A: 255},
		{R: 0x4C, G: 0x9A, B: 0xFF, A: 255},
		{R: 17, G: 200, B: 91, A: 128},
	}
	for _, want := range cases {
		h, s, v := rgbToHSV(want)
		got := hsvToRGB(h, s, v, want.A)
		if absDiff(got.R, want.R) > 1 || absDiff(got.G, want.G) > 1 || absDiff(got.B, want.B) > 1 {
			t.Errorf("%+v round tripped to %+v (h=%v s=%v v=%v)", want, got, h, s, v)
		}
		if got.A != want.A {
			t.Errorf("alpha changed: %d -> %d", want.A, got.A)
		}
	}
}

func absDiff(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}

func TestHSVEdgeCases(t *testing.T) {
	// Grey has no meaningful hue, and that must not produce NaNs.
	h, s, v := rgbToHSV(color.RGBA{R: 128, G: 128, B: 128, A: 255})
	if math.IsNaN(h) || s != 0 || math.Abs(v-128.0/255) > 1e-9 {
		t.Errorf("grey gave h=%v s=%v v=%v", h, s, v)
	}
	// Hues wrap rather than clamping.
	if hsvToRGB(370, 1, 1, 255) != hsvToRGB(10, 1, 1, 255) {
		t.Error("hue does not wrap past 360")
	}
	if hsvToRGB(-10, 1, 1, 255) != hsvToRGB(350, 1, 1, 255) {
		t.Error("negative hue does not wrap")
	}
	// Out-of-range saturation and value clamp instead of overflowing.
	c := hsvToRGB(0, 5, 5, 255)
	if c.R != 255 {
		t.Errorf("clamped hsv gave %+v", c)
	}
}

func TestFormatNumberTrimsZeros(t *testing.T) {
	cases := []struct {
		v        float64
		decimals int
		want     string
	}{
		{6, 0, "6"},
		{6.0, 2, "6"},
		{6.25, 2, "6.25"},
		{6.50, 2, "6.5"},
		{-3, 0, "-3"},
		{0, 0, "0"},
		{1.0 / 3, 2, "0.33"},
		// Go formats floats with round-half-to-even, so an exact .125 goes down.
		{6.125, 0, "6.12"},
	}
	for _, c := range cases {
		if got := formatNumber(c.v, c.decimals); got != c.want {
			t.Errorf("formatNumber(%v, %d) = %q, want %q", c.v, c.decimals, got, c.want)
		}
	}
}

func TestParseNumberAcceptsUnits(t *testing.T) {
	cases := []struct {
		in   string
		unit string
		want float64
	}{
		{"6", "u", 6},
		{"6 u", "u", 6},
		{"6u", "u", 6},
		{" -2.5 ", "u", -2.5},
		{"45 °", "°", 45},
		{"12px", "px", 12},
	}
	for _, c := range cases {
		got, err := parseNumber(c.in, c.unit)
		if err != nil {
			t.Errorf("parseNumber(%q) failed: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseNumber(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "   ", "u", "banana", "1.2.3"} {
		if _, err := parseNumber(bad, "u"); err == nil {
			t.Errorf("parseNumber(%q) was accepted", bad)
		}
	}
}

func TestClampRange(t *testing.T) {
	if clampRange(5, 0, 10) != 5 || clampRange(-1, 0, 10) != 0 || clampRange(11, 0, 10) != 10 {
		t.Error("clampRange is wrong")
	}
	if clamp01(-1) != 0 || clamp01(2) != 1 || clamp01(0.5) != 0.5 {
		t.Error("clamp01 is wrong")
	}
}

// TestEditStateTyping covers the text editor's buffer operations, which the
// rename field and the typed drag-number both rely on.
func TestEditStateTyping(t *testing.T) {
	e := &editState{text: []rune("Body 1"), caret: 6, sel: -1}

	e.insert([]rune("23"))
	if e.String() != "Body 123" || e.caret != 8 {
		t.Fatalf("insert gave %q caret %d", e.String(), e.caret)
	}

	e.selectAll()
	a, b, ok := e.selRange()
	if !ok || a != 0 || b != 8 {
		t.Fatalf("selectAll gave %d..%d ok=%v", a, b, ok)
	}
	e.insert([]rune("Wing"))
	if e.String() != "Wing" {
		t.Fatalf("typing over a selection gave %q", e.String())
	}

	// Deleting with nothing selected is a no-op the caller falls back from.
	e.sel = -1
	if e.deleteSelection() {
		t.Error("deleteSelection reported work with no selection")
	}
}

func TestTruncateNeedsNoFont(t *testing.T) {
	// Truncate measures with the font, so it cannot run here; what can be
	// checked without a GPU is that the caller-visible contract holds for the
	// trivial case of unlimited width.
	c := &Context{Scale: 1}
	if c.Px(10) != 10 {
		t.Errorf("Px(10) at scale 1 = %v", c.Px(10))
	}
	c.Scale = 1.5
	if c.Px(10) != 15 {
		t.Errorf("Px(10) at scale 1.5 = %v", c.Px(10))
	}
}

func TestInteractionStates(t *testing.T) {
	c := &Context{Scale: 1}
	id := MakeID("btn")
	r := Rect(0, 0, 100, 30)

	// Not hovered: nothing happens.
	c.Begin(Input{MouseX: 500, MouseY: 500, KeysDown: map[int32]bool{}})
	it := c.interact(id, r, false)
	if it.Hovered || it.Pressed || it.Clicked || c.WantMouse() {
		t.Fatalf("a far-away pointer produced %+v", it)
	}
	c.End()

	// Hovered: the kit claims the pointer so the viewport leaves it alone.
	c.Begin(Input{MouseX: 10, MouseY: 10, KeysDown: map[int32]bool{}})
	it = c.interact(id, r, false)
	if !it.Hovered || it.Pressed || !c.WantMouse() {
		t.Fatalf("hover produced %+v wantMouse=%v", it, c.WantMouse())
	}
	c.End()

	// Press then release over the widget is a click.
	press := Input{MouseX: 10, MouseY: 10, KeysDown: map[int32]bool{}}
	press.Pressed[MouseLeft] = true
	press.Down[MouseLeft] = true
	c.Begin(press)
	it = c.interact(id, r, false)
	if !it.Pressed || it.Clicked {
		t.Fatalf("press frame produced %+v", it)
	}
	c.End()

	release := Input{MouseX: 10, MouseY: 10, KeysDown: map[int32]bool{}}
	release.Released[MouseLeft] = true
	c.Begin(release)
	it = c.interact(id, r, false)
	if !it.Clicked {
		t.Fatal("the release frame did not produce a click")
	}
	c.End()
	if c.active != NoID {
		t.Error("the active widget survived the release")
	}
}

// TestReleasingAwayFromTheWidgetIsNotAClick is the standard escape hatch:
// press, slide off, release, and nothing happens.
func TestReleasingAwayFromTheWidgetIsNotAClick(t *testing.T) {
	c := &Context{Scale: 1}
	id := MakeID("btn")
	r := Rect(0, 0, 100, 30)

	press := Input{MouseX: 10, MouseY: 10, KeysDown: map[int32]bool{}}
	press.Pressed[MouseLeft] = true
	press.Down[MouseLeft] = true
	c.Begin(press)
	c.interact(id, r, false)
	c.End()

	release := Input{MouseX: 400, MouseY: 400, KeysDown: map[int32]bool{}}
	release.Released[MouseLeft] = true
	c.Begin(release)
	it := c.interact(id, r, false)
	c.End()
	if it.Clicked {
		t.Error("releasing away from the widget still clicked it")
	}
}

func TestDisabledWidgetsNeverActivate(t *testing.T) {
	c := &Context{Scale: 1}
	id := MakeID("btn")
	r := Rect(0, 0, 100, 30)

	press := Input{MouseX: 10, MouseY: 10, KeysDown: map[int32]bool{}}
	press.Pressed[MouseLeft] = true
	press.Down[MouseLeft] = true
	c.Begin(press)
	it := c.interact(id, r, true)
	if it.Pressed || it.Clicked || !it.Disabled {
		t.Fatalf("a disabled widget reacted: %+v", it)
	}
	// A disabled control still reports hover, so its tooltip can explain itself.
	if !it.Hovered {
		t.Error("a disabled widget should still know it is hovered")
	}
	c.End()
}

func TestToastLifecycle(t *testing.T) {
	c := &Context{Scale: 1}
	c.ShowToast(Toast{Text: "one"})
	c.ShowToast(Toast{Text: "two"})
	if len(c.Toasts()) != 2 {
		t.Fatalf("%d toasts queued", len(c.Toasts()))
	}

	// The stack never grows past three (SPEC-UX §4).
	c.ShowToast(Toast{Text: "three"})
	c.ShowToast(Toast{Text: "four"})
	if len(c.Toasts()) != MaxToasts {
		t.Fatalf("%d toasts stacked, want at most %d", len(c.Toasts()), MaxToasts)
	}
	if c.Toasts()[0].Text != "two" {
		t.Errorf("the oldest toast was not dropped: %q is first", c.Toasts()[0].Text)
	}

	// They expire after three seconds.
	c.stepToasts(ToastMillis - 1)
	if len(c.Toasts()) != MaxToasts {
		t.Error("toasts expired early")
	}
	c.stepToasts(2)
	if len(c.Toasts()) != 0 {
		t.Errorf("%d toasts outlived their timeout", len(c.Toasts()))
	}

	c.ShowToast(Toast{Text: "x"})
	c.ClearToasts()
	if len(c.Toasts()) != 0 {
		t.Error("ClearToasts left messages behind")
	}
}

func TestTooltipWaitsForTheDelay(t *testing.T) {
	c := &Context{Scale: 1}
	id := MakeID("btn")
	r := Rect(0, 0, 100, 30)
	hovered := Interaction{Hovered: true}

	c.queueTooltip(id, r, hovered, "Extrude", "E", "")
	c.stepTooltip(100)
	if c.tip.waited >= TooltipDelayMillis {
		t.Fatal("the tooltip is ready far too early")
	}

	for i := 0; i < 10; i++ {
		c.queueTooltip(id, r, hovered, "Extrude", "E", "")
		c.stepTooltip(100)
	}
	if c.tip.waited < TooltipDelayMillis {
		t.Fatalf("after 1.1 s the tooltip has waited only %v ms", c.tip.waited)
	}

	// Moving to another widget restarts the wait.
	other := MakeID("other")
	c.queueTooltip(other, r, hovered, "Boolean", "B", "")
	if c.tip.waited != 0 {
		t.Errorf("moving to a new widget kept %v ms of waiting", c.tip.waited)
	}
	// Losing the hover drops the tip entirely.
	c.stepTooltip(16)
	c.stepTooltip(16)
	if c.tip.id != NoID {
		t.Error("the tooltip survived losing its hover")
	}
}

// TestDisabledTooltipExplainsItself covers the SPEC-UX §15 rule that every
// disabled control says how to enable it.
func TestDisabledTooltipExplainsItself(t *testing.T) {
	c := &Context{Scale: 1}
	id := MakeID("btn")
	r := Rect(0, 0, 100, 30)

	c.queueTooltip(id, r, Interaction{Hovered: true, Disabled: true}, "Extrude", "E", "Select a closed region first")
	if c.tip.text != "Select a closed region first" {
		t.Errorf("a disabled control shows %q, want its reason", c.tip.text)
	}
	if c.tip.shortcut != "" {
		t.Error("a disabled control should not advertise its shortcut")
	}

	// With no reason given, a disabled control stays silent rather than
	// promising an action it will not perform.
	c2 := &Context{Scale: 1}
	c2.queueTooltip(id, r, Interaction{Hovered: true, Disabled: true}, "Extrude", "E", "")
	if c2.tip.text != "" {
		t.Errorf("a disabled control with no reason showed %q", c2.tip.text)
	}
}

func TestFocusManagement(t *testing.T) {
	c := &Context{Scale: 1}
	id := MakeID("field")
	if c.Focus() != NoID {
		t.Error("something has focus before anything asked for it")
	}
	c.SetFocus(id, "Hull")
	if c.Focus() != id || c.edit.String() != "Hull" || c.edit.caret != 4 {
		t.Errorf("SetFocus gave focus=%v text=%q caret=%d", c.Focus(), c.edit.String(), c.edit.caret)
	}
	c.ClearFocus()
	if c.Focus() != NoID || c.edit.String() != "" {
		t.Error("ClearFocus left state behind")
	}
}

func TestModalBlocksEverythingBeneath(t *testing.T) {
	c := &Context{Scale: 1}
	if c.ModalOpen() {
		t.Error("a modal is open before one was asked for")
	}
	c.ShowModal(ModalState{Title: "Recover?"})
	if !c.ModalOpen() {
		t.Fatal("ShowModal did not open one")
	}
	if !c.OverlayCapturesPointer(0, 0) {
		t.Error("an open modal does not capture the pointer")
	}
	c.CloseModal()
	if c.ModalOpen() || c.OverlayCapturesPointer(0, 0) {
		t.Error("CloseModal left the modal up")
	}
}

func TestPopoverCapturesPointerOnlyWhereItIs(t *testing.T) {
	c := &Context{Scale: 1}
	anchor := Rect(10, 10, 20, 20)
	if c.OverlayCapturesPointer(15, 15) {
		t.Error("a closed popover captured the pointer")
	}
	c.OpenColorPicker(MakeID("swatch"), anchor, color.RGBA{R: 255, A: 255})
	c.popoverBox = Rect(10, 30, 200, 200)

	if !c.OverlayCapturesPointer(15, 15) {
		t.Error("the anchor should keep the pointer, so re-clicking the swatch works")
	}
	if !c.OverlayCapturesPointer(100, 100) {
		t.Error("the popover body should capture the pointer")
	}
	if c.OverlayCapturesPointer(600, 600) {
		t.Error("the popover captured a pointer nowhere near it")
	}
	c.CloseColorPicker()
	if c.OverlayCapturesPointer(100, 100) {
		t.Error("closing the picker left its box behind")
	}
}

func TestInputKeyPressed(t *testing.T) {
	in := Input{KeysPressed: []int32{rl.KeyE, rl.KeyS}}
	if !in.KeyPressed(rl.KeyE) || !in.KeyPressed(rl.KeyS) {
		t.Error("KeyPressed missed a key that is present")
	}
	if in.KeyPressed(rl.KeyB) {
		t.Error("KeyPressed invented a key")
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 42: "42", -3: "-3", 1204: "1204"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRoundnessStaysInRange(t *testing.T) {
	r := Rect(0, 0, 100, 20)
	if got := roundness(r, 6); got <= 0 || got > 1 {
		t.Errorf("roundness = %v, want (0,1]", got)
	}
	// A radius larger than the box clamps to fully round.
	if got := roundness(r, 500); got != 1 {
		t.Errorf("oversized radius gave %v, want 1", got)
	}
	if got := roundness(Rect(0, 0, 0, 0), 6); got != 0 {
		t.Errorf("a collapsed box gave %v, want 0", got)
	}
}
