package paint

// Ordered (Bayer) dithering, which is how this program gives a two-colour ramp
// a middle without inventing a third colour (SPEC-UX §13.4).
//
// It is the pixel-art answer to a soft edge. A blend would be honest to the
// maths and wrong for the medium — the Crisp pillar says a texel is one colour
// out of the palette — so coverage between nothing and everything is spent on
// *how many* texels are painted rather than on how strongly each one is.
//
// The matrices are indexed by texel position in the face's own grid, never by
// position within the shape being drawn, so two passes over the same area line
// up with each other instead of seaming.

// Dither is which Bayer matrix a tool spreads its coverage through.
type Dither uint8

const (
	// DitherNone blends instead: a gradient ramps through real colours and a
	// soft brush fades into what is under it.
	DitherNone Dither = iota
	Dither2x2
	Dither4x4
	Dither8x8
)

// Dithers lists the modes in the order the panel shows them.
var Dithers = []Dither{DitherNone, Dither2x2, Dither4x4, Dither8x8}

func (d Dither) String() string {
	switch d {
	case Dither2x2:
		return "2x2"
	case Dither4x4:
		return "4x4"
	case Dither8x8:
		return "8x8"
	default:
		return "None"
	}
}

// ParseDither reads a mode name, as the op scripts and the settings spell it.
func ParseDither(s string) (Dither, bool) {
	for _, d := range Dithers {
		if d.String() == s {
			return d, true
		}
	}
	switch s {
	case "none", "":
		return DitherNone, true
	}
	return DitherNone, false
}

// Size is the matrix's edge, or zero for no dithering.
func (d Dither) Size() int {
	switch d {
	case Dither2x2:
		return 2
	case Dither4x4:
		return 4
	case Dither8x8:
		return 8
	default:
		return 0
	}
}

// bayer2 is the seed of the whole family. Each larger order is built from it by
// the standard recurrence, which is what keeps the thresholds spread rather
// than clumped — and is far easier to get right than typing out sixty-four
// numbers.
var bayer2 = [][]int{{0, 2}, {3, 1}}

var (
	bayer4 = growBayer(bayer2)
	bayer8 = growBayer(bayer4)
)

// growBayer doubles a Bayer matrix's order: M(2n) = 4*M(n) + bayer2 tiled over
// the four quadrants.
func growBayer(m [][]int) [][]int {
	n := len(m)
	out := make([][]int, 2*n)
	for y := range out {
		out[y] = make([]int, 2*n)
		for x := range out[y] {
			out[y][x] = 4*m[y%n][x%n] + bayer2[y/n][x/n]
		}
	}
	return out
}

// rank is the texel's position in the matrix's ordering, 0..n²-1.
func (d Dither) rank(x, y int) int {
	n := d.Size()
	if n == 0 {
		return 0
	}
	// Go's % keeps the sign of the dividend and texel indices go negative
	// inside a picture's margin, so the wrap is done by hand.
	x, y = ((x%n)+n)%n, ((y%n)+n)%n
	switch d {
	case Dither2x2:
		return bayer2[y][x]
	case Dither4x4:
		return bayer4[y][x]
	default:
		return bayer8[y][x]
	}
}

// Threshold is the coverage at which this texel turns on, in [0,1).
//
// The half-step offset is what keeps both ends clear: without it the lowest
// threshold would be exactly zero and a texel would paint at no coverage at all.
func (d Dither) Threshold(x, y int) float64 {
	n := d.Size()
	if n == 0 {
		return 0.5
	}
	return (float64(d.rank(x, y)) + 0.5) / float64(n*n)
}

// Covers reports whether a texel is painted at a given coverage.
//
// With no dithering this is a plain cut at half, because a two-colour ramp with
// no pattern to spread through has nothing else it can do.
func (d Dither) Covers(x, y int, coverage float64) bool {
	if coverage >= 1 {
		return true
	}
	if coverage <= 0 {
		return false
	}
	return coverage > d.Threshold(x, y)
}
