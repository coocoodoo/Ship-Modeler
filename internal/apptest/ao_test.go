package apptest

import (
	"image"
	"path/filepath"
	"testing"
)

// Screen-space ambient occlusion: the stepped
// shape with the strength at its default and at zero. The pair is the
// assertion — the concave junction reads darker on the first shot and the
// two must differ, which the baselines pin without any tolerance games.
func TestGoldenAmbientOcclusion(t *testing.T) {
	_, outDir := runScript(t, "ao_step")
	checkGolden(t, "ao_step", outDir)
}

// Two separate convex bodies cannot self-occlude. Their contact shadow must
// come from the shared scene, and it must follow the actual bottom-left toggle.
func TestAmbientOcclusionAcrossBodiesAndViewChanges(t *testing.T) {
	_, dir := runScript(t, "ao_contact")
	load := func(name string) image.Image {
		im, err := LoadPNG(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		return im
	}
	region := image.Rect(250, 70, 1010, 540)
	diff := func(a, b image.Image) (dark, light int) {
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				r1, g1, b1, _ := a.At(x, y).RGBA()
				r2, g2, b2, _ := b.At(x, y).RGBA()
				d := int(r2+g2+b2) - int(r1+g1+b1)
				if d > 3*257*3 {
					dark++
				}
				if d < -3*257*3 {
					light++
				}
			}
		}
		return
	}
	for _, pair := range [][2]string{{"contact_on", "contact_off"}, {"contact_toggle_on", "contact_toggle_off"}, {"perspective_on", "perspective_off"}, {"flat_toggle_restores_lighting", "perspective_off"}} {
		dark, light := diff(load(pair[0]), load(pair[1]))
		if dark < 100 || light > 10 {
			t.Errorf("%s: AO darkened %d pixels and brightened %d; want a contact shadow without brightening", pair[0], dark, light)
		}
	}
	for _, pair := range [][2]string{{"contact_on", "contact_toggle_on"}, {"contact_off", "contact_toggle_off"}, {"flat_on", "flat_off"}} {
		dark, light := diff(load(pair[0]), load(pair[1]))
		if dark+light != 0 {
			t.Errorf("%s and %s disagree at %d pixels", pair[0], pair[1], dark+light)
		}
	}
}
