// Package apptest is the end-to-end harness: it builds the real executable once
// and drives it through op scripts, comparing the PNGs it produces against
// committed baselines (TESTING §4).
//
// Nothing here is imported by the application; it exists for tests.
package apptest

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// Golden comparison policy (SPEC-RENDER §10). GPU output can shift slightly
// across driver updates, so baselines are same-machine and compared with a
// small tolerance plus an outlier allowance rather than byte for byte.
const (
	// MaxChannelDelta is the largest per-channel difference an ordinary pixel
	// may show.
	MaxChannelDelta = 3
	// MinPixelsWithinTolerance is the fraction of pixels that must stay inside
	// MaxChannelDelta.
	MinPixelsWithinTolerance = 0.997
	// MaxMeanDelta bounds the average per-channel difference over the image.
	MaxMeanDelta = 0.5
)

// CompareResult reports how two renders differ.
type CompareResult struct {
	Width, Height int
	Pixels        int
	Outliers      int
	MeanDelta     float64
	MaxDelta      int
	WorstAt       image.Point
}

// WithinTolerance reports whether the difference passes the golden policy.
func (r CompareResult) WithinTolerance() bool {
	if r.Pixels == 0 {
		return false
	}
	within := 1 - float64(r.Outliers)/float64(r.Pixels)
	return within >= MinPixelsWithinTolerance && r.MeanDelta <= MaxMeanDelta
}

func (r CompareResult) String() string {
	if r.Pixels == 0 {
		return "no pixels compared"
	}
	within := 100 * (1 - float64(r.Outliers)/float64(r.Pixels))
	return fmt.Sprintf(
		"%dx%d: %.4f%% of pixels within %d/channel (need %.2f%%), mean delta %.4f (max %.2f), worst delta %d at %v",
		r.Width, r.Height, within, MaxChannelDelta, 100*MinPixelsWithinTolerance,
		r.MeanDelta, MaxMeanDelta, r.MaxDelta, r.WorstAt)
}

// LoadPNG reads a PNG into an RGBA image.
func LoadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out, nil
}

// Compare measures the difference between two renders.
func Compare(got, want *image.RGBA) (CompareResult, error) {
	gb, wb := got.Bounds(), want.Bounds()
	if gb.Dx() != wb.Dx() || gb.Dy() != wb.Dy() {
		return CompareResult{}, fmt.Errorf("size mismatch: got %dx%d, want %dx%d",
			gb.Dx(), gb.Dy(), wb.Dx(), wb.Dy())
	}
	res := CompareResult{Width: gb.Dx(), Height: gb.Dy(), Pixels: gb.Dx() * gb.Dy()}
	var sum float64
	for y := 0; y < gb.Dy(); y++ {
		for x := 0; x < gb.Dx(); x++ {
			g := got.RGBAAt(x, y)
			w := want.RGBAAt(x, y)
			d := maxDelta(g, w)
			sum += float64(absInt(int(g.R)-int(w.R))+
				absInt(int(g.G)-int(w.G))+
				absInt(int(g.B)-int(w.B))) / 3
			if d > MaxChannelDelta {
				res.Outliers++
			}
			if d > res.MaxDelta {
				res.MaxDelta = d
				res.WorstAt = image.Pt(x, y)
			}
		}
	}
	res.MeanDelta = sum / float64(res.Pixels)
	return res, nil
}

func maxDelta(a, b color.RGBA) int {
	d := absInt(int(a.R) - int(b.R))
	d = maxInt(d, absInt(int(a.G)-int(b.G)))
	d = maxInt(d, absInt(int(a.B)-int(b.B)))
	return maxInt(d, absInt(int(a.A)-int(b.A)))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// WriteDiff saves a visual diff highlighting where two renders disagree, so a
// failing baseline can be reviewed before it is regenerated.
func WriteDiff(path string, got, want *image.RGBA) error {
	b := got.Bounds()
	out := image.NewRGBA(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			g, w := got.RGBAAt(x, y), want.RGBAAt(x, y)
			d := maxDelta(g, w)
			switch {
			case d == 0:
				// Unchanged pixels stay, heavily dimmed.
				out.SetRGBA(x, y, color.RGBA{R: g.R / 5, G: g.G / 5, B: g.B / 5, A: 255})
			case d <= MaxChannelDelta:
				out.SetRGBA(x, y, color.RGBA{R: 0, G: 120, B: 60, A: 255})
			default:
				v := uint8(math.Min(255, 120+float64(d)))
				out.SetRGBA(x, y, color.RGBA{R: v, G: 40, B: 40, A: 255})
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}

// CopyPNG copies a rendered shot, used to refresh baselines and the visual
// diary under docs/shots.
func CopyPNG(dst, src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
