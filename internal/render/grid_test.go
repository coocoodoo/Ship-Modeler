package render

import (
	"math"
	"testing"
)

func TestGridLinesStayWorldAnchoredWithinFaceSheet(t *testing.T) {
	for _, origin := range []float64{0, -2.5, 1.25, -35.25, 100.5} {
		for _, step := range []float64{0.25, 0.5, 1, 2} {
			minor := gridLinePositions(origin, 16, step, step*8)
			major := gridLinePositions(origin, 16, step*8, 0)
			seen := map[float64]bool{}
			for _, line := range append(minor, major...) {
				if line < -16 || line > 16 || math.Abs(math.Remainder(line-origin, step)) > 1e-9 {
					t.Fatalf("origin %v step %v: line %v leaves the sheet or lattice", origin, step, line)
				}
				if seen[line] {
					t.Fatalf("major line %v drawn twice", line)
				}
				seen[line] = true
			}
			for line := math.Ceil((-16-origin)/step)*step + origin; line <= 16; line += step {
				if !seen[line] {
					t.Fatalf("origin %v step %v: missing grid line %v", origin, step, line)
				}
			}
		}
	}
}
