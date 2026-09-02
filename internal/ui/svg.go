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

// SVG icons (the user's request, 2026-08-29; V-147, rebuilt V-154).
//
// Every icon in the program is authored as a real SVG file under icons/ and
// rasterized here at load: parse the path data, flatten the curves, fill by
// exact-coverage scanline, upload once per size as a white alpha mask, and
// tint at draw time. The Draw*Icon functions the widgets call keep their
// signatures — each prefers its SVG and falls back to its old procedural
// strokes if the asset is missing, so a new icon is a new file, not a new
// function.
//
// The engine reads the subset good icon files are made of: <path> with
// M L H V C S Q T Z (absolute and relative), viewBox, fill, fill-rule,
// stroke, stroke-width, stroke-linecap and stroke-linejoin. No arcs — author
// them as cubics, which is what icon tools export anyway.
//
// Strokes matter more than they look. Every modern icon set is drawn as
// centre-lines with one weight, and without stroking them here an author has
// to hand-build each shape as a filled silhouette — which is exactly how the
// first set came out heavy and uneven. A stroked path gets its consistency
// from the format instead of from the care of whoever typed the numbers.

//go:embed icons/*.svg
var iconFS embed.FS

// svgPt is a point in viewBox space.
type svgPt struct{ x, y float64 }

// svgSubpath is one flattened run of points, and whether Z closed it.
type svgSubpath struct {
	pts    []svgPt
	closed bool
}

// svgPath is one <path>: its geometry and how it is painted.
type svgPath struct {
	subs []svgSubpath

	fill    bool
	evenOdd bool

	stroke  bool
	width   float64
	linecap string // butt | round | square
	// Joins are always round: an icon grid at 18 px cannot tell a mitre from
	// a round join, and round is the one that never spikes.
}

// svgShape is one parsed icon.
type svgShape struct {
	paths []svgPath
	view  [4]float64 // minX, minY, width, height
}

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
		tex = uploadMask(RasterizeIcon(s, px))
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

func uploadMask(img *image.RGBA) rl.Texture2D {
	rimg := rl.NewImageFromImage(img)
	tex := rl.LoadTextureFromImage(rimg)
	rl.UnloadImage(rimg)
	return tex
}

// IconMask rasterizes a named icon to a white alpha mask, or nil if there is
// no such icon. Exported for the contact-sheet test, which is how the set is
// reviewed without a GPU.
func IconMask(name string, px int) *image.RGBA {
	s := svgShapeFor(name)
	if s == nil {
		return nil
	}
	return RasterizeIcon(s, px)
}

// IconNames lists every icon asset, sorted, for tests and tooling.
func IconNames() []string {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".svg") {
			out = append(out, strings.TrimSuffix(n, ".svg"))
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// ---------------------------------------------------------------------------
// Rasterizing
// ---------------------------------------------------------------------------

// subScanlines is how many sample rows each pixel row is measured on.
// Horizontal coverage is exact, so this is the only approximation left, and
// sixteen of them puts the error below what a byte of alpha can hold.
const subScanlines = 16

// rasterEdge is one line segment in pixel space, with its winding direction.
type rasterEdge struct {
	x0, y0, x1, y1 float64
	dir            int // +1 downward, -1 upward
}

// RasterizeIcon fills a parsed icon into a px-square white alpha mask.
//
// Coverage is computed rather than sampled: each sub-scanline contributes the
// exact fraction of every pixel its spans cover, so an edge at any angle lands
// on a smooth ramp instead of the sixteen steps a 4x4 supersample can express.
// At 18 px — where every icon in this program actually lives — that is the
// difference between a crisp glyph and a ragged one.
func RasterizeIcon(s *svgShape, px int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	if px <= 0 || s == nil {
		return img
	}

	// ViewBox space to pixel space, preserving aspect, centred.
	vw := math.Max(s.view[2], 1e-9)
	vh := math.Max(s.view[3], 1e-9)
	scale := float64(px) / math.Max(vw, vh)
	offX := (float64(px) - vw*scale) / 2
	offY := (float64(px) - vh*scale) / 2
	toPx := func(p svgPt) svgPt {
		return svgPt{(p.x-s.view[0])*scale + offX, (p.y-s.view[1])*scale + offY}
	}

	cov := make([]float64, px*px)
	for i := range s.paths {
		p := &s.paths[i]
		if p.fill {
			var edges []rasterEdge
			for _, sub := range p.subs {
				edges = appendContour(edges, sub.pts, toPx, true)
			}
			accumulate(cov, px, edges, p.evenOdd)
		}
		if p.stroke && p.width > 0 {
			// The outline is stamped rather than joined: a quad per segment
			// and a disc per joint and round cap, all wound the same way, so
			// a nonzero fill of the pile is exactly their union. It needs no
			// mitre arithmetic and cannot produce the spike a sharp corner
			// gives a naive offset.
			var edges []rasterEdge
			for _, sub := range p.subs {
				edges = appendStroke(edges, sub, p, toPx, scale)
			}
			accumulate(cov, px, edges, false)
		}
	}

	for i, c := range cov {
		if c <= 0 {
			continue
		}
		if c > 1 {
			c = 1
		}
		a := uint8(math.Round(c * 255))
		if a == 0 {
			continue
		}
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 255, 255, 255, a
	}
	return img
}

// appendContour adds a polyline's edges, closing it when asked.
func appendContour(edges []rasterEdge, pts []svgPt, toPx func(svgPt) svgPt, closed bool) []rasterEdge {
	if len(pts) < 2 {
		return edges
	}
	n := len(pts)
	last := n - 1
	if closed {
		last = n
	}
	for i := 0; i < last; i++ {
		a, b := toPx(pts[i]), toPx(pts[(i+1)%n])
		edges = appendEdge(edges, a, b)
	}
	return edges
}

func appendEdge(edges []rasterEdge, a, b svgPt) []rasterEdge {
	if a.y == b.y {
		return edges // horizontal edges never cross a scanline
	}
	dir := 1
	if a.y > b.y {
		a, b, dir = b, a, -1
	}
	return append(edges, rasterEdge{a.x, a.y, b.x, b.y, dir})
}

// appendPolygon adds a closed polygon wound consistently counter-clockwise.
func appendPolygon(edges []rasterEdge, poly []svgPt) []rasterEdge {
	if len(poly) < 3 {
		return edges
	}
	// Signed area decides the winding; flip so every stamped piece agrees and
	// their nonzero union is the shape rather than a set of holes.
	area := 0.0
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		area += a.x*b.y - b.x*a.y
	}
	if area < 0 {
		for i, j := 0, len(poly)-1; i < j; i, j = i+1, j-1 {
			poly[i], poly[j] = poly[j], poly[i]
		}
	}
	for i := range poly {
		edges = appendEdge(edges, poly[i], poly[(i+1)%len(poly)])
	}
	return edges
}

// discSegments is how many sides a stamped round join or cap gets. Sixteen is
// smooth past any size an icon is drawn at.
const discSegments = 16

// appendStroke stamps one subpath's stroke as quads and discs in pixel space.
func appendStroke(edges []rasterEdge, sub svgSubpath, p *svgPath,
	toPx func(svgPt) svgPt, scale float64) []rasterEdge {

	pts := make([]svgPt, 0, len(sub.pts))
	for _, q := range sub.pts {
		v := toPx(q)
		// Drop repeats: a zero-length segment has no direction to offset.
		if n := len(pts); n > 0 && math.Abs(pts[n-1].x-v.x) < 1e-9 && math.Abs(pts[n-1].y-v.y) < 1e-9 {
			continue
		}
		pts = append(pts, v)
	}
	if sub.closed && len(pts) > 1 {
		a, b := pts[0], pts[len(pts)-1]
		if math.Abs(a.x-b.x) < 1e-9 && math.Abs(a.y-b.y) < 1e-9 {
			pts = pts[:len(pts)-1]
		}
	}
	half := p.width * scale / 2
	if half <= 0 {
		return edges
	}

	disc := func(c svgPt) {
		poly := make([]svgPt, 0, discSegments)
		for i := 0; i < discSegments; i++ {
			t := 2 * math.Pi * float64(i) / discSegments
			poly = append(poly, svgPt{c.x + half*math.Cos(t), c.y + half*math.Sin(t)})
		}
		edges = appendPolygon(edges, poly)
	}

	if len(pts) == 1 {
		// A lone point is a dot when the cap is round, and nothing otherwise.
		if p.linecap == "round" {
			disc(pts[0])
		}
		return edges
	}

	segs := len(pts) - 1
	if sub.closed {
		segs = len(pts)
	}
	for i := 0; i < segs; i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		dx, dy := b.x-a.x, b.y-a.y
		l := math.Hypot(dx, dy)
		if l < 1e-12 {
			continue
		}
		ux, uy := dx/l, dy/l
		if p.linecap == "square" && !sub.closed {
			// Square caps run the end segments half a width further out.
			if i == 0 {
				a = svgPt{a.x - ux*half, a.y - uy*half}
			}
			if i == segs-1 {
				b = svgPt{b.x + ux*half, b.y + uy*half}
			}
		}
		nx, ny := -uy*half, ux*half
		edges = appendPolygon(edges, []svgPt{
			{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny},
			{b.x - nx, b.y - ny}, {a.x - nx, a.y - ny},
		})
	}

	// Joints get a disc so corners are round rather than notched.
	first, last := 1, len(pts)-1
	if sub.closed {
		first, last = 0, len(pts)-1
	}
	for i := first; i <= last; i++ {
		if !sub.closed && (i == 0 || i == len(pts)-1) {
			continue
		}
		disc(pts[i])
	}
	if !sub.closed && p.linecap == "round" {
		disc(pts[0])
		disc(pts[len(pts)-1])
	}
	return edges
}

// accumulate adds one edge set's coverage into cov.
func accumulate(cov []float64, px int, edges []rasterEdge, evenOdd bool) {
	if len(edges) == 0 {
		return
	}
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, e := range edges {
		minY = math.Min(minY, e.y0)
		maxY = math.Max(maxY, e.y1)
	}
	y0 := int(math.Max(0, math.Floor(minY)))
	y1 := int(math.Min(float64(px), math.Ceil(maxY)))
	weight := 1.0 / subScanlines

	type crossing struct {
		x   float64
		dir int
	}
	var xs []crossing

	for row := y0; row < y1; row++ {
		base := row * px
		for s := 0; s < subScanlines; s++ {
			y := float64(row) + (float64(s)+0.5)/subScanlines
			xs = xs[:0]
			for _, e := range edges {
				if y < e.y0 || y >= e.y1 {
					continue
				}
				t := (y - e.y0) / (e.y1 - e.y0)
				xs = append(xs, crossing{e.x0 + t*(e.x1-e.x0), e.dir})
			}
			if len(xs) < 2 {
				continue
			}
			for i := 1; i < len(xs); i++ {
				for j := i; j > 0 && xs[j].x < xs[j-1].x; j-- {
					xs[j], xs[j-1] = xs[j-1], xs[j]
				}
			}
			if evenOdd {
				for i := 0; i+1 < len(xs); i += 2 {
					span(cov, base, px, xs[i].x, xs[i+1].x, weight)
				}
				continue
			}
			wind := 0
			for i := 0; i+1 < len(xs); i++ {
				wind += xs[i].dir
				if wind != 0 {
					span(cov, base, px, xs[i].x, xs[i+1].x, weight)
				}
			}
		}
	}
}

// span adds a horizontal run's exact per-pixel coverage to one row.
func span(cov []float64, base, px int, x0, x1, weight float64) {
	if x1 <= x0 {
		return
	}
	if x0 < 0 {
		x0 = 0
	}
	if x1 > float64(px) {
		x1 = float64(px)
	}
	if x1 <= x0 {
		return
	}
	i0 := int(x0)
	i1 := int(x1)
	if i1 >= px {
		i1 = px - 1
	}
	if i0 == i1 {
		cov[base+i0] += (x1 - x0) * weight
		return
	}
	cov[base+i0] += (float64(i0+1) - x0) * weight
	for i := i0 + 1; i < i1; i++ {
		cov[base+i] += weight
	}
	cov[base+i1] += (x1 - float64(i1)) * weight
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

var (
	svgViewBoxRe = regexp.MustCompile(`viewBox\s*=\s*"([^"]+)"`)
	svgPathRe    = regexp.MustCompile(`<path\b[^>]*/?>`)
	svgAttrRe    = regexp.MustCompile(`([a-zA-Z-]+)\s*=\s*"([^"]*)"`)
)

func parseSVG(text string) (*svgShape, error) {
	s := &svgShape{view: [4]float64{0, 0, 24, 24}}
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
	// Attributes on <svg> itself are inherited by every path, which is how a
	// stroked icon says its weight once instead of on every line.
	rootAttrs := map[string]string{}
	if i := strings.Index(text, ">"); i > 0 {
		for _, m := range svgAttrRe.FindAllStringSubmatch(text[:i], -1) {
			rootAttrs[m[1]] = m[2]
		}
	}

	tags := svgPathRe.FindAllString(text, -1)
	if len(tags) == 0 {
		return nil, fmt.Errorf("no <path> in the file")
	}
	for _, tag := range tags {
		attrs := map[string]string{}
		for k, v := range rootAttrs {
			attrs[k] = v
		}
		for _, m := range svgAttrRe.FindAllStringSubmatch(tag, -1) {
			attrs[m[1]] = m[2]
		}
		d := attrs["d"]
		if strings.TrimSpace(d) == "" {
			continue
		}
		subs, err := parsePathData(d)
		if err != nil {
			return nil, err
		}
		p := svgPath{subs: subs}

		fill := strings.TrimSpace(attrs["fill"])
		p.fill = fill != "none"
		p.evenOdd = strings.TrimSpace(attrs["fill-rule"]) == "evenodd"

		stroke := strings.TrimSpace(attrs["stroke"])
		p.stroke = stroke != "" && stroke != "none"
		p.width = 1
		if w, err := strconv.ParseFloat(strings.TrimSpace(attrs["stroke-width"]), 64); err == nil {
			p.width = w
		}
		p.linecap = strings.TrimSpace(attrs["stroke-linecap"])
		if p.linecap == "" {
			p.linecap = "butt"
		}
		if !p.fill && !p.stroke {
			continue
		}
		s.paths = append(s.paths, p)
	}
	if len(s.paths) == 0 {
		return nil, fmt.Errorf("no drawable <path> in the file")
	}
	return s, nil
}

// parsePathData turns a d attribute into flattened subpaths.
func parsePathData(d string) ([]svgSubpath, error) {
	toks, err := tokenizePath(d)
	if err != nil {
		return nil, err
	}
	var (
		out       []svgSubpath
		cur       []svgPt
		curClosed bool
		pos       svgPt
		start     svgPt
		lastCtrl  svgPt
		lastWasCS bool
		lastWasQT bool
	)
	const flat = 24 // segments per curve: smooth past any icon size

	flush := func() {
		if len(cur) >= 2 || (len(cur) == 1 && !curClosed) {
			out = append(out, svgSubpath{pts: cur, closed: curClosed})
		}
		cur, curClosed = nil, false
	}
	moveTo := func(p svgPt) {
		flush()
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
		case 'Z', 'z':
			i++
			curClosed = true
			flush()
			pos = start
			cur = []svgPt{start}
			lastWasCS, lastWasQT = false, false
		default:
			return nil, fmt.Errorf("unsupported path command %q", string(cmd))
		}
	}
	flush()
	return out, nil
}

// pathTokens is a path's numbers, each tagged with the command it belongs to.
type pathTokens struct {
	nums   []float64
	numCmd []byte
}

func tokenizePath(d string) (pathTokens, error) {
	var out pathTokens
	cmd := byte(0)
	pairIndex := 0
	i := 0
	for i < len(d) {
		c := d[i]
		switch {
		case c == ',' || c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			cmd = c
			pairIndex = 0
			if c == 'Z' || c == 'z' {
				out.nums = append(out.nums, 0)
				out.numCmd = append(out.numCmd, c)
			}
			i++
		default:
			j := i
			if d[j] == '+' || d[j] == '-' {
				j++
			}
			for j < len(d) && (d[j] >= '0' && d[j] <= '9' || d[j] == '.') {
				j++
			}
			if j < len(d) && (d[j] == 'e' || d[j] == 'E') {
				j++
				if j < len(d) && (d[j] == '+' || d[j] == '-') {
					j++
				}
				for j < len(d) && d[j] >= '0' && d[j] <= '9' {
					j++
				}
			}
			if j == i {
				return out, fmt.Errorf("unexpected %q in path data", string(d[i]))
			}
			v, err := strconv.ParseFloat(d[i:j], 64)
			if err != nil {
				return out, fmt.Errorf("bad number %q", d[i:j])
			}
			if cmd == 0 {
				return out, fmt.Errorf("path data starts with a number")
			}
			eff := cmd
			// Repeated coordinate pairs after a moveto are implicit linetos,
			// which is how every exporter writes a polygon.
			if (cmd == 'M' || cmd == 'm') && pairIndex >= 2 {
				if cmd == 'M' {
					eff = 'L'
				} else {
					eff = 'l'
				}
			}
			out.nums = append(out.nums, v)
			out.numCmd = append(out.numCmd, eff)
			pairIndex++
			i = j
		}
	}
	return out, nil
}
