package sketch2d

import (
	"fmt"
	"math"
	"testing"

	"modeler/internal/geom"
)

// The risk Sketch_func.md §6 names first: "region engine chokes on many short
// segments (splines)". Measured before the spline UI was built, so the answer
// decides how many subdivisions a spline is allowed to spend.
//
// The shape is the worst realistic case: one closed loop of n short segments,
// which is what a closed spline through a dozen points becomes.
func benchLoop(n int) []Seg {
	segs := make([]Seg, 0, n)
	const r = 8 * geom.SubunitsPerUnit
	pt := func(i int) geom.Vec2i {
		a := 2 * math.Pi * float64(i%n) / float64(n)
		return geom.Vec2i{
			X: int64(math.Round(r * math.Cos(a))),
			Y: int64(math.Round(r * math.Sin(a))),
		}
	}
	for i := 0; i < n; i++ {
		segs = append(segs, Seg{A: pt(i), B: pt(i + 1), Src: Source{Entity: 0, Seg: i}})
	}
	return segs
}

func BenchmarkBuildLoop(b *testing.B) {
	for _, n := range []int{64, 128, 256, 512, 1024} {
		b.Run(fmt.Sprintf("segments=%d", n), func(b *testing.B) {
			segs := benchLoop(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if a := Build(segs); len(a.Regions) != 1 {
					b.Fatalf("built %d regions, want 1", len(a.Regions))
				}
			}
		})
	}
}
