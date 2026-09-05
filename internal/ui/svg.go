package ui

import (
	"embed"
	"fmt"
	"image"
	"math"
	"regexp"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// SVG icons (the user's request, 2026-08-29; V-146).
//
// Every icon in the program is authored as a real SVG file under icons/ and
// rasterized here at load: parse the path data, flatten the curves, fill by
// even-odd scanline at 4x supersampling, upload once per size as a white
// alpha mask, and tint at draw time exactly the way the stroke-drawn icons
// tinted. The Draw*Icon functions the widgets call keep their signatures —
// each now prefers its SVG and falls back to its old procedural strokes if
// the asset is missing, so a new icon is a new file, not a new function.
//
// The engine reads the subset good icon files are made of: <path d="..."/>
// with M L H V C S Q T Z (absolute and relative), viewBox, and nothing else.
// No arcs — author arcs as cubics, which is what icon tools export anyway.
// Filled silhouettes and uniformly stroked outline paths are supported. Outline
// paths use round joins/caps; closed outlines repeat their starting point.
// Colour belongs to the theme, so the SVG only supplies shape and weight.

//go:embed icons/*.svg
var iconFS embed.FS

// svgShape is one parsed icon: closed contours in viewBox space.
type svgShape struct {
	contours [][]svgPt
	// Outline icons use open, round-capped paths at one consistent weight.
	strokes     [][]svgPt
	strokeWidth float64
	// view is the viewBox: minX, minY, width, height.
	view [4]float64
}

type svgPt struct{ x, y float64 }

// parsed icons by name ("save" for icons/save.svg), loaded lazily.
var svgShapes = map[string]*svgShape{}

// rasterized textures by name and pixel size.
type svgTexKey struct {
	name string
	px   int
}

var svgTextures = map[svgTexKey]rl.Texture2D{}

// svgShapeFor parses an icon file once. Missing or unparsable files return
// nil, which is the signal to fall back to the procedural drawing.
func svgShapeFor(name string) *svgShape {
	if s, ok := svgShapes[name]; ok {
		return s
	}
	data, err := iconFS.ReadFile("icons/" + name + ".svg")
	if err != nil {
		svgShapes[name] = nil
		return nil
	}
	s, err := parseSVG(string(data))
	if err != nil {
		// A bad asset is a build-time authoring mistake; say so once, loudly
		// enough to find, and keep the program drawing its fallback.
		fmt.Printf("icon %s.svg: %v\n", name, err)
		svgShapes[name] = nil
		return nil
	}
	svgShapes[name] = s
	return s
}

// drawSVGIcon draws a named icon centred at (cx, cy) in a size-px box, tinted.
// It reports false when the asset is missing so the caller can stroke the old
// shape instead.
func drawSVGIcon(name string, cx, cy, size float64, col rl.Color) bool {
	s := svgShapeFor(name)
	if s == nil || size < 2 {
		return s != nil
	}
	px := int(math.Round(size))
	key := svgTexKey{name: name, px: px}
	tex, ok := svgTextures[key]
	if !ok {
		tex = rasterizeSVG(s, px)
		svgTextures[key] = tex
	}
	if tex.ID == 0 {
		return false
	}
	// Integer placement: an alpha mask blitted on half-pixels is a blurry
	// icon, which is the one thing this whole file exists to prevent.
	x := float32(math.Round(cx - size/2))
	y := float32(math.Round(cy - size/2))
	rl.DrawTexture(tex, int32(x), int32(y), col)
	return true
}

// UnloadIcons releases the rasterized textures; the parsed shapes are cheap
// and stay.
func UnloadIcons() {
	for k, t := range svgTextures {
		if t.ID != 0 {
			rl.UnloadTexture(t)
		}
		delete(svgTextures, k)
	}
}

// rasterizeSVG fills the shape into a px-square white alpha mask at 4x4
// supersampling and uploads it.
func rasterizeSVG(s *svgShape, px int) rl.Texture2D {
	const ss = 4
	w := px * ss

	// ViewBox space to supersampled pixel space, preserving aspect, centred.
	scale := float64(w) / math.Max(s.view[2], s.view[3])
	offX := (float64(w) - s.view[2]*scale) / 2
	offY := (float64(w) - s.view[3]*scale) / 2

	type edge struct{ x0, y0, x1, y1 float64 }
	var edges []edge
	for _, c := range s.contours {
		if len(c) < 2 {
			continue
		}
		for i := range c {
			a, b := c[i], c[(i+1)%len(c)]
			edges = append(edges, edge{
				(a.x-s.view[0])*scale + offX, (a.y-s.view[1])*scale + offY,
				(b.x-s.view[0])*scale + offX, (b.y-s.view[1])*scale + offY,
			})
		}
	}

	cover := make([]uint32, px*px)
	var xs []float64
	for sy := 0; sy < w; sy++ {
		y := float64(sy) + 0.5
		xs = xs[:0]
		for _, e := range edges {
			if (e.y0 <= y) == (e.y1 <= y) {
				continue // does not cross this scanline
			}
			t := (y - e.y0) / (e.y1 - e.y0)
			xs = append(xs, e.x0+t*(e.x1-e.x0))
		}
		if len(xs) == 0 {
			continue
		}
		sortFloats(xs)
		row := (sy / ss) * px
		// Even-odd: fill between alternate crossings.
		for i := 0; i+1 < len(xs); i += 2 {
			x0, x1 := xs[i], xs[i+1]
			if x1 <= 0 || x0 >= float64(w) {
				continue
			}
			from := int(math.Max(0, math.Ceil(x0-0.5)))
			to := int(math.Min(float64(w), math.Ceil(x1-0.5)))
			for sx := from; sx < to; sx++ {
				cover[row+sx/ss]++
			}
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, px, px))
	if len(s.strokes) > 0 {
		mask := make([]bool, w*w)
		radius := s.strokeWidth * scale / 2
		for _, path := range s.strokes {
			for i := 1; i < len(path); i++ {
				a, b := path[i-1], path[i]
				ax, ay := (a.x-s.view[0])*scale+offX, (a.y-s.view[1])*scale+offY
				bx, by := (b.x-s.view[0])*scale+offX, (b.y-s.view[1])*scale+offY
				dx, dy := bx-ax, by-ay
				length2 := dx*dx + dy*dy
				for y := max(0, int(math.Floor(math.Min(ay, by)-radius))); y < min(w, int(math.Ceil(math.Max(ay, by)+radius))); y++ {
					for x := max(0, int(math.Floor(math.Min(ax, bx)-radius))); x < min(w, int(math.Ceil(math.Max(ax, bx)+radius))); x++ {
						qx, qy := float64(x)+0.5-ax, float64(y)+0.5-ay
						t := 0.0
						if length2 > 0 {
							t = math.Max(0, math.Min(1, (qx*dx+qy*dy)/length2))
						}
						if (qx-t*dx)*(qx-t*dx)+(qy-t*dy)*(qy-t*dy) <= radius*radius {
							mask[y*w+x] = true
						}
					}
				}
			}
		}
		for y := 0; y < w; y++ {
			for x := 0; x < w; x++ {
				if mask[y*w+x] {
					cover[(y/ss)*px+x/ss]++
				}
			}
		}
	}
	for i, c := range cover {
		c = min(c, ss*ss)
		a := uint8(math.Round(float64(c) * 255 / (ss * ss)))
		if a == 0 {
			continue
		}
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 255, 255, 255, a
	}

	rimg := rl.NewImageFromImage(img)
	tex := rl.LoadTextureFromImage(rimg)
	rl.UnloadImage(rimg)
	return tex
}

func sortFloats(v []float64) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

var (
	svgViewBoxRe     = regexp.MustCompile(`viewBox\s*=\s*"([^"]+)"`)
	svgPathRe        = regexp.MustCompile(`<path[^>]*\sd\s*=\s*"([^"]+)"`)
	svgStrokeWidthRe = regexp.MustCompile(`stroke-width\s*=\s*"([^"]+)"`)
)

func parseSVG(text string) (*svgShape, error) {
	s := &svgShape{view: [4]float64{0, 0, 24, 24}}
	outline := strings.Contains(text, `fill="none"`)
	if outline {
		s.strokeWidth = 1.75
		if m := svgStrokeWidthRe.FindStringSubmatch(text); m != nil {
			v, err := strconv.ParseFloat(m[1], 64)
			if err != nil || v <= 0 {
				return nil, fmt.Errorf("invalid stroke width")
			}
			s.strokeWidth = v
		}
	}
	if m := svgViewBoxRe.FindStringSubmatch(text); m != nil {
		f := strings.Fields(strings.ReplaceAll(m[1], ",", " "))
		if len(f) == 4 {
			for i := range f {
				v, err := strconv.ParseFloat(f[i], 64)
				if err != nil {
					return nil, fmt.Errorf("viewBox: %w", err)
				}
				s.view[i] = v
			}
		}
	}
	paths := svgPathRe.FindAllStringSubmatch(text, -1)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no <path d=...> in the file")
	}
	for _, m := range paths {
		contours, err := parsePathData(m[1])
		if err != nil {
			return nil, err
		}
		if outline {
			s.strokes = append(s.strokes, contours...)
		} else {
			s.contours = append(s.contours, contours...)
		}
	}
	return s, nil
}

// parsePathData turns a d attribute into flattened closed contours.
func parsePathData(d string) ([][]svgPt, error) {
	toks, err := tokenizePath(d)
	if err != nil {
		return nil, err
	}
	var (
		out       [][]svgPt
		cur       []svgPt
		pos       svgPt
		start     svgPt
		lastCtrl  svgPt
		lastWasCS bool // the previous command was C/S (for S reflection)
		lastWasQT bool // the previous command was Q/T (for T reflection)
	)
	const flat = 16 // segments per curve: plenty at icon sizes

	closeContour := func() {
		if len(cur) >= 2 {
			out = append(out, cur)
		}
		cur = nil
	}
	moveTo := func(p svgPt) {
		closeContour()
		pos, start = p, p
		cur = []svgPt{p}
	}
	lineTo := func(p svgPt) {
		pos = p
		cur = append(cur, p)
	}
	cubicTo := func(c1, c2, p svgPt) {
		p0 := pos
		for i := 1; i <= flat; i++ {
			t := float64(i) / flat
			u := 1 - t
			x := u*u*u*p0.x + 3*u*u*t*c1.x + 3*u*t*t*c2.x + t*t*t*p.x
			y := u*u*u*p0.y + 3*u*u*t*c1.y + 3*u*t*t*c2.y + t*t*t*p.y
			cur = append(cur, svgPt{x, y})
		}
		pos, lastCtrl = p, c2
	}
	quadTo := func(c, p svgPt) {
		p0 := pos
		for i := 1; i <= flat; i++ {
			t := float64(i) / flat
			u := 1 - t
			x := u*u*p0.x + 2*u*t*c.x + t*t*p.x
			y := u*u*p0.y + 2*u*t*c.y + t*t*p.y
			cur = append(cur, svgPt{x, y})
		}
		pos, lastCtrl = p, c
	}

	i := 0
	need := func(n int) ([]float64, error) {
		if i+n > len(toks.nums) {
			return nil, fmt.Errorf("path data ends mid-command")
		}
		v := toks.nums[i : i+n]
		i += n
		return v, nil
	}
	rel := func(cmd byte) bool { return cmd >= 'a' }
	pt := func(cmd byte, x, y float64) svgPt {
		if rel(cmd) {
			return svgPt{pos.x + x, pos.y + y}
		}
		return svgPt{x, y}
	}

	for i < len(toks.nums) {
		cmd := toks.numCmd[i]
		switch cmd {
		case 'M', 'm':
			v, err := need(2)
			if err != nil {
				return nil, err
			}
			moveTo(pt(cmd, v[0], v[1]))
			lastWasCS, lastWasQT = false, false
			// Extra pairs after a move are implicit line-tos; the tokenizer
			// marks them as L/l already.
		case 'L', 'l':
			v, err := need(2)
			if err != nil {
				return nil, err
			}
			lineTo(pt(cmd, v[0], v[1]))
			lastWasCS, lastWasQT = false, false
		case 'H', 'h':
			v, err := need(1)
			if err != nil {
				return nil, err
			}
			x := v[0]
			if rel(cmd) {
				x += pos.x
			}
			lineTo(svgPt{x, pos.y})
			lastWasCS, lastWasQT = false, false
		case 'V', 'v':
			v, err := need(1)
			if err != nil {
				return nil, err
			}
			y := v[0]
			if rel(cmd) {
				y += pos.y
			}
			lineTo(svgPt{pos.x, y})
			lastWasCS, lastWasQT = false, false
		case 'C', 'c':
			v, err := need(6)
			if err != nil {
				return nil, err
			}
			cubicTo(pt(cmd, v[0], v[1]), pt(cmd, v[2], v[3]), pt(cmd, v[4], v[5]))
			lastWasCS, lastWasQT = true, false
		case 'S', 's':
			v, err := need(4)
			if err != nil {
				return nil, err
			}
			c1 := pos
			if lastWasCS {
				c1 = svgPt{2*pos.x - lastCtrl.x, 2*pos.y - lastCtrl.y}
			}
			cubicTo(c1, pt(cmd, v[0], v[1]), pt(cmd, v[2], v[3]))
			lastWasCS, lastWasQT = true, false
		case 'Q', 'q':
			v, err := need(4)
			if err != nil {
				return nil, err
			}
			quadTo(pt(cmd, v[0], v[1]), pt(cmd, v[2], v[3]))
			lastWasCS, lastWasQT = false, true
		case 'T', 't':
			v, err := need(2)
			if err != nil {
				return nil, err
			}
			c := pos
			if lastWasQT {
				c = svgPt{2*pos.x - lastCtrl.x, 2*pos.y - lastCtrl.y}
			}
			quadTo(c, pt(cmd, v[0], v[1]))
			lastWasCS, lastWasQT = false, true
		default:
			return nil, fmt.Errorf("path command %q is outside the icon subset", string(cmd))
		}
	}
	_ = start
	closeContour()
	return out, nil
}

// pathTokens carries the numbers of a path with the command each belongs to.
// Z closes are handled during tokenizing by... they carry no numbers, so the
// tokenizer records them as contour breaks via the M that follows; a Z that
// ends the data needs no action because contours close implicitly.
type pathTokens struct {
	nums   []float64
	numCmd []byte
}

func tokenizePath(d string) (*pathTokens, error) {
	out := &pathTokens{}
	cmd := byte(0)
	i := 0
	pairIndex := 0
	for i < len(d) {
		ch := d[i]
		switch {
		case ch == ' ' || ch == ',' || ch == '\n' || ch == '\t' || ch == '\r':
			i++
		case (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z'):
			if ch == 'Z' || ch == 'z' {
				cmd = 0 // a following number without a command is an error
				i++
				continue
			}
			cmd = ch
			pairIndex = 0
			i++
		case ch == '-' || ch == '+' || ch == '.' || (ch >= '0' && ch <= '9'):
			j := i
			if d[j] == '-' || d[j] == '+' {
				j++
			}
			dot := false
			for j < len(d) {
				c := d[j]
				if c >= '0' && c <= '9' {
					j++
					continue
				}
				if c == '.' && !dot {
					dot = true
					j++
					continue
				}
				if c == 'e' || c == 'E' {
					j++
					if j < len(d) && (d[j] == '-' || d[j] == '+') {
						j++
					}
					continue
				}
				break
			}
			v, err := strconv.ParseFloat(d[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("path number %q: %w", d[i:j], err)
			}
			if cmd == 0 {
				return nil, fmt.Errorf("a number with no command before it")
			}
			// After a moveto's first pair, further pairs are implicit linetos.
			use := cmd
			if (cmd == 'M' || cmd == 'm') && pairIndex >= 2 {
				if cmd == 'M' {
					use = 'L'
				} else {
					use = 'l'
				}
			}
			out.nums = append(out.nums, v)
			out.numCmd = append(out.numCmd, use)
			pairIndex++
			i = j
		default:
			return nil, fmt.Errorf("unexpected %q in path data", string(ch))
		}
	}
	return out, nil
}
