package paint

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// The mapping tests of TESTING §2 ("Paint mapping"), written before the mapping
// exists. Everything the brush, the renderer and the texel cursor do rests on
// one function; if it drifts, paint lands somewhere other than where it was
// aimed and nothing else in the mode can be trusted.

// plate is a flat-topped box. Its +Y face is 8 units in X by 4 in Z, so the
// long side is unambiguous and a resolution chip has an obvious meaning.
func plate() (*mesh.Mesh, int) {
	m := mesh.Box(geom.Vec3{X: -4, Y: -1, Z: -2}, geom.Vec3{X: 4, Y: 1, Z: 2}, 1)
	return m, faceAlong(m, geom.Vec3{X: 0, Y: 1, Z: 0})
}

// faceAlong finds the face whose outward normal points along n.
func faceAlong(m *mesh.Mesh, n geom.Vec3) int {
	for i := range m.Faces {
		if m.FaceNormal(i).Dot(n) > 0.999 {
			return i
		}
	}
	panic("no face along that normal")
}

func TestAllocateTakesItsDensityFromTheChipAlone(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	// "4" means four texels to the unit — a density, not a count, so the face's
	// own size has no say in it (GEOM §8.2, V-128).
	if want := 1.0 / 4; math.Abs(p.Texel-want) > 1e-12 {
		t.Errorf("texel size = %v, want %v", p.Texel, want)
	}
	if p.Res != 4 {
		t.Errorf("res = %d, want 4", p.Res)
	}
	// The image covers the face's bbox plus a one-texel margin all round.
	b := p.Img.Bounds()
	// The face is 8 x 4 units, so at four texels to the unit it is 32 x 16.
	if b.Dx() != 34 || b.Dy() != 18 {
		t.Errorf("image is %dx%d, want 34x18 (32x16 plus a texel each side)", b.Dx(), b.Dy())
	}
	if p.Off != (image.Point{X: -1, Y: -1}) {
		t.Errorf("origin texel = %v, want (-1,-1): the margin sits outside the face", p.Off)
	}
	// Every corner of the face must land inside the image.
	for _, vi := range m.Faces[fi].Outer() {
		if tx := Texel(p, m.Verts[vi]); !tx.In(Bounds(p)) {
			t.Errorf("corner %v maps to texel %v, outside %v", m.Verts[vi], tx, Bounds(p))
		}
	}
}

func TestUVRoundTripsThroughWorld(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	for _, tx := range []image.Point{{X: 0, Y: 0}, {X: 31, Y: 15}, {X: 7, Y: 9}, {X: -1, Y: -1}} {
		w := World(p, tx)
		if got := Texel(p, w); got != tx {
			t.Errorf("texel %v -> world %v -> texel %v", tx, w, got)
		}
		// The world point of a texel centre is on the face's plane.
		if h := math.Abs(p.Frame.Height(w)); h > 1e-9 {
			t.Errorf("texel %v lifted %v off the paint plane", tx, h)
		}
	}
	// Continuous round trip: an arbitrary point on the plane keeps its uv.
	for _, uv := range []geom.Vec2{{X: 0.25, Y: 3.75}, {X: 12.5, Y: 0.5}} {
		w := p.Frame.ToWorld(uv.Mul(p.Texel))
		back := UV(p, w)
		if math.Abs(back.X-uv.X) > 1e-9 || math.Abs(back.Y-uv.Y) > 1e-9 {
			t.Errorf("uv %v -> world %v -> uv %v", uv, w, back)
		}
	}
}

func TestTexelSizeSurvivesTranslateAndQuarterTurn(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	m.Faces[fi].Paint = p
	Set(p, image.Point{X: 3, Y: 4}, color.RGBA{R: 200, G: 40, B: 40, A: 255})
	before := World(p, image.Point{X: 3, Y: 4})
	texelBefore := p.Texel

	x := geom.Translate(geom.Vec3{X: 5, Y: 0, Z: -2}).Mul(geom.RotateY(math.Pi / 2))
	mesh.Transform(m, x)

	if p.Texel != texelBefore {
		t.Errorf("texel size changed under a rigid motion: %v -> %v", texelBefore, p.Texel)
	}
	// The painted texel is exactly where the rigid motion put it.
	want := x.TransformPoint(before)
	if got := World(p, image.Point{X: 3, Y: 4}); !got.NearEq(want, 1e-9) {
		t.Errorf("painted texel moved to %v, want %v", got, want)
	}
	// And it is still the texel you get by pointing at that spot.
	if got := Texel(p, want); got != (image.Point{X: 3, Y: 4}) {
		t.Errorf("world %v now maps to texel %v", want, got)
	}
	if !p.Frame.Orthonormal() || !p.Frame.IsRightHanded() {
		t.Error("the paint frame stopped being an orthonormal right-handed basis")
	}
}

func TestGrowKeepsExistingPixelsWhereTheyAre(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	red := color.RGBA{R: 220, G: 30, B: 30, A: 255}
	marks := []image.Point{{X: 0, Y: 0}, {X: 5, Y: 6}, {X: 31, Y: 15}}
	worlds := make([]geom.Vec3, len(marks))
	for i, tx := range marks {
		Set(p, tx, red)
		worlds[i] = World(p, tx)
	}
	beforeOff, beforeSize := p.Off, p.Img.Bounds().Size()

	// Paint well outside the current image, which has to grow to hold it.
	far := image.Point{X: -20, Y: 40}
	if !Set(p, far, color.RGBA{G: 255, A: 255}) {
		t.Fatal("painting outside the image should grow it, not fail")
	}
	if p.Img.Bounds().Size() == beforeSize && p.Off == beforeOff {
		t.Fatal("the image did not grow")
	}
	for i, tx := range marks {
		if got := At(p, tx); got != red {
			t.Errorf("texel %v is %v after growing, want %v", tx, got, red)
		}
		if got := World(p, tx); !got.NearEq(worlds[i], 1e-12) {
			t.Errorf("texel %v moved from %v to %v when the image grew", tx, worlds[i], got)
		}
	}
	if got := At(p, far); got != (color.RGBA{G: 255, A: 255}) {
		t.Errorf("the far texel is %v, want green", got)
	}
}

func TestGrowRefusesBeyondTheCap(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if Set(p, image.Point{X: MaxTextureSize + 10, Y: 0}, color.RGBA{A: 255}) {
		t.Error("a texel past the size cap should be refused, not allocated")
	}
	if p.Img.Bounds().Dx() > MaxTextureSize || p.Img.Bounds().Dy() > MaxTextureSize {
		t.Errorf("image grew to %v, past the %d cap", p.Img.Bounds().Size(), MaxTextureSize)
	}
}

func TestAllocateRejectsAnUnknownResolution(t *testing.T) {
	m, fi := plate()
	if _, err := Allocate(m, fi, 33); err == nil {
		t.Error("33 is not a resolution chip; allocate should say so")
	}
	for _, res := range Resolutions {
		if _, err := Allocate(m, fi, res); err != nil {
			t.Errorf("res %d: %v", res, err)
		}
	}
}

// A face that is not axis aligned still gets a usable anchor: the canonical
// face frame with its origin dropped on the bbox corner, and every corner of
// the face inside the image it allocates.
func TestAllocateOnASlantedFace(t *testing.T) {
	m := mesh.Box(geom.Vec3{X: -3, Y: -3, Z: -3}, geom.Vec3{X: 3, Y: 3, Z: 3}, 1)
	mesh.Transform(m, geom.RotateY(0.7).Mul(geom.RotateZ(0.4)))
	fi := 0
	p, err := Allocate(m, fi, 2)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if !p.Frame.Orthonormal() {
		t.Error("paint frame is not orthonormal")
	}
	n := m.FaceNormal(fi)
	if p.Frame.N.Dot(n) < 0.999 {
		t.Errorf("paint frame normal %v does not match the face normal %v", p.Frame.N, n)
	}
	for _, vi := range m.Faces[fi].Outer() {
		if tx := Texel(p, m.Verts[vi]); !tx.In(Bounds(p)) {
			t.Errorf("corner maps to %v, outside %v", tx, Bounds(p))
		}
	}
}

func TestFaceRectCoversTheFaceAndNotTheMargin(t *testing.T) {
	m, fi := plate()
	p, err := Allocate(m, fi, 4)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	r := FaceRect(m, fi, p)
	// The face is 8x4 units at 1/4 u per texel: 32 by 16 texels, starting at
	// the origin because Allocate put texel (0,0) on the bbox corner.
	want := image.Rect(0, 0, 32, 16)
	if r != want {
		t.Errorf("face rect = %v, want %v", r, want)
	}
	// It is strictly inside the image, which is what leaves the fill somewhere
	// to stop and the atlas a texel of padding.
	if !r.In(Bounds(p)) {
		t.Errorf("face rect %v is not inside the image %v", r, Bounds(p))
	}
	// Every corner of the face is in it.
	for _, vi := range m.Faces[fi].Outer() {
		tx := Texel(p, m.Verts[vi])
		// A corner sits exactly on the boundary, so clamp it inwards the way a
		// cursor on the edge is clamped.
		tx.X = clampInt(tx.X, r.Min.X, r.Max.X-1)
		tx.Y = clampInt(tx.Y, r.Min.Y, r.Max.Y-1)
		if !tx.In(r) {
			t.Errorf("corner %v maps outside the face rect", m.Verts[vi])
		}
	}
}

func TestFaceRectFollowsAGrownImage(t *testing.T) {
	m, fi := plate()
	p, _ := Allocate(m, fi, 4)
	before := FaceRect(m, fi, p)
	// Painting far outside grows the image; the face has not moved, so the
	// rectangle the face occupies must not move either.
	Set(p, image.Point{X: 40, Y: 40}, color.RGBA{R: 255, A: 255})
	if got := FaceRect(m, fi, p); got != before {
		t.Errorf("face rect moved from %v to %v when the image grew", before, got)
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// The point of the whole exercise (V-128): a pixel is the same size on every
// face, so an edge line is the same thickness on both faces it touches and a
// texel is a texel wherever you look.
//
// Before this, texel size was the face's longest side over the chip, so the
// short faces of any box that was not a cube had smaller pixels than the long
// ones — and nothing on screen said so.
func TestEveryFaceGetsThePixelSizeTheChipAsksFor(t *testing.T) {
	// Deliberately not a cube: 12 x 2 x 8 has three different face sizes, which
	// under the old rule gave three different pixel sizes.
	m := mesh.Box(geom.Vec3{X: -6, Y: -1, Z: -4}, geom.Vec3{X: 6, Y: 1, Z: 4}, 1)
	const res = 8
	want := 1.0 / res
	for fi := range m.Faces {
		p, err := Allocate(m, fi, res)
		if err != nil {
			t.Fatalf("allocating face %d: %v", fi, err)
		}
		if math.Abs(p.Texel-want) > 1e-12 {
			t.Errorf("face %d has %v-unit texels, want %v — a pixel has to be "+
				"the same size on every face", fi, p.Texel, want)
		}
		if d := Density(p); math.Abs(d-res) > 1e-9 {
			t.Errorf("face %d reports %v px/u, want %d", fi, d, res)
		}
	}
}

// A density means the picture grows with the face, so a big enough face can ask
// for more than the cap allows. That has to be refused rather than clamped: a
// clamped picture would not cover the face, and a face silently painted at some
// other density is the exact thing the density was brought in to stop.
func TestAFaceTooBigForItsChipIsRefusedWithAChipThatFits(t *testing.T) {
	// 400 units across at 32 px/u would be 12800 px, well past the 1024 cap.
	m := mesh.Box(geom.Vec3{X: -200, Y: -1, Z: -200}, geom.Vec3{X: 200, Y: 1, Z: 200}, 1)
	fi := faceAlong(m, geom.Vec3{X: 0, Y: 1, Z: 0})

	_, err := Allocate(m, fi, 32)
	if err == nil {
		t.Fatal("a 400-unit face was allocated at 32 px/u, past the texture cap")
	}
	// The refusal has to name a chip that works, or it is a dead end.
	if !strings.Contains(err.Error(), "try 2 px/u") {
		t.Errorf("the refusal does not say what would fit: %v", err)
	}
	if _, err := Allocate(m, fi, 2); err != nil {
		t.Errorf("the chip it recommended does not itself fit: %v", err)
	}
}
