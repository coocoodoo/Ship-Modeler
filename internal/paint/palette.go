package paint

import (
	"bufio"
	"fmt"
	"image/color"
	"io"
	"strings"
)

// The palette of SPEC-UX §13.3: a built-in page of 32 colours, a Lospec .hex
// import for bringing your own, and a recents strip that tracks what you have
// actually been using.
//
// None of this is document state (SPEC-DATA §3.3) — it lives in settings, and
// picking a colour is not an undo step.

// PaletteSize is the built-in page, laid out 8x4 in the panel.
const PaletteSize = 32

// RecentsSize is how many recently used colours the strip remembers.
const RecentsSize = 8

// DefaultPalette is the built-in spaceship palette: four ramps of eight, in the
// order the panel draws them.
//
// The rows are chosen for the job rather than for coverage of the colour wheel.
// Two hull ramps — neutral steel and cold blue — because a ship is mostly hull
// and plating reads as plating only when its shades come from one family. A
// warm rust/brass ramp for the parts that are meant to look worn or mechanical,
// which is what stops a grey ship looking like untextured geometry. Then one row
// of saturated accents for the things that are supposed to glow: engines,
// warning stripes, cockpit light. Eight steps per ramp is enough to shade a
// bevel without ever tempting anyone to blend.
func DefaultPalette() []color.RGBA {
	return []color.RGBA{
		// Steel — the default hull, near-black to white with a cold cast.
		rgb(0x0D0F14), rgb(0x1A1E26), rgb(0x2B313D), rgb(0x3F4756),
		rgb(0x5A6473), rgb(0x7C8797), rgb(0xA8B2BE), rgb(0xE4EAF0),
		// Cold hull — navy to ice, for panelling and shadowed plate.
		rgb(0x0F2036), rgb(0x173250), rgb(0x22496D), rgb(0x32648C),
		rgb(0x4A85AB), rgb(0x6BA8C8), rgb(0x95CBE0), rgb(0xC6E7F2),
		// Warm hull — brown through brass to bone, for wear and trim.
		rgb(0x2A1A14), rgb(0x44281C), rgb(0x6B3B22), rgb(0x8F5A2B),
		rgb(0xB4823C), rgb(0xD4A857), rgb(0xE8CB86), rgb(0xF5E6BE),
		// Accents and glow — hazard, thruster, plasma.
		rgb(0x7A1230), rgb(0xC21F3A), rgb(0xF2542D), rgb(0xFFA51E),
		rgb(0xFFE24B), rgb(0x3DDB6E), rgb(0x21E7E7), rgb(0xB45CF0),
	}
}

// rgb turns an 0xRRGGBB literal into an opaque colour.
func rgb(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// Hex formats a colour the way a .hex file spells it, which is also what the
// op scripts and the settings file store.
func Hex(c color.RGBA) string {
	return fmt.Sprintf("%02X%02X%02X", c.R, c.G, c.B)
}

// ParseColor reads one RRGGBB (with or without a leading #). It is the single
// place a colour string turns into a colour, so the palette file, the settings
// and the paint.color op all agree on what counts as one.
func ParseColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if len(s) != 6 {
		return color.RGBA{}, false
	}
	var v uint32
	for _, r := range s {
		d, ok := hexDigit(r)
		if !ok {
			return color.RGBA{}, false
		}
		v = v<<4 | uint32(d)
	}
	return rgb(v), true
}

func hexDigit(r rune) (int, bool) {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0'), true
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10, true
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10, true
	}
	return 0, false
}

// ParseHex reads a Lospec .hex palette: one RRGGBB per line.
//
// Lines starting with # are ambiguous in that format — Lospec writes bare
// RRGGBB, but plenty of files in the wild use #RRGGBB, and plenty carry a
// comment header. So a # line that spells a colour is a colour and a # line
// that does not is a comment, while a line without the # has to be a colour or
// the file is not the thing the user thought they were importing. Failing loudly
// there matters: silently importing three of a hundred colours looks like the
// app is broken, not like the file was.
func ParseHex(r io.Reader) ([]color.RGBA, error) {
	var out []color.RGBA
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		c, ok := ParseColor(text)
		if ok {
			out = append(out, c)
			continue
		}
		if strings.HasPrefix(text, "#") {
			continue // a comment, not a malformed colour
		}
		return nil, fmt.Errorf("line %d: %q is not an RRGGBB colour", line, text)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading palette: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no colours in that file")
	}
	return out, nil
}

// Recents is the most-recently-used strip. The zero value is an empty strip.
type Recents struct {
	list []color.RGBA
}

// Add records a colour as just used. Re-picking a colour already in the strip
// promotes it instead of duplicating it, so the strip stays a set of eight
// distinct colours rather than eight slots of the same one.
func (r *Recents) Add(c color.RGBA) {
	c.A = 255
	for i, have := range r.list {
		if have == c {
			r.list = append(r.list[:i], r.list[i+1:]...)
			break
		}
	}
	r.list = append([]color.RGBA{c}, r.list...)
	if len(r.list) > RecentsSize {
		r.list = r.list[:RecentsSize]
	}
}

// List returns the strip, most recent first.
func (r *Recents) List() []color.RGBA {
	return append([]color.RGBA(nil), r.list...)
}

// Set replaces the whole strip, which is how settings restore it at startup.
func (r *Recents) Set(cs []color.RGBA) {
	r.list = nil
	// Backwards, so the first colour in cs ends up first in the strip.
	for i := len(cs) - 1; i >= 0; i-- {
		r.Add(cs[i])
	}
}
