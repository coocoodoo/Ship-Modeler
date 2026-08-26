package geom

import "math"

// AABB is an axis-aligned bounding box in model space. The zero value is the
// empty box (Min > Max), so accumulation starts from AABB{} via Empty.
type AABB struct {
	Min, Max Vec3
}

// Empty returns an inverted box that absorbs the first point added to it.
func Empty() AABB {
	inf := math.Inf(1)
	return AABB{
		Min: Vec3{inf, inf, inf},
		Max: Vec3{-inf, -inf, -inf},
	}
}

// Valid reports whether the box contains at least one point.
func (b AABB) Valid() bool {
	return b.Min.X <= b.Max.X && b.Min.Y <= b.Max.Y && b.Min.Z <= b.Max.Z
}

// AddPoint returns the box grown to include p.
func (b AABB) AddPoint(p Vec3) AABB {
	return AABB{Min: b.Min.MinV(p), Max: b.Max.MaxV(p)}
}

// Union returns the box covering both inputs. An invalid box is ignored.
func (b AABB) Union(o AABB) AABB {
	if !o.Valid() {
		return b
	}
	if !b.Valid() {
		return o
	}
	return AABB{Min: b.Min.MinV(o.Min), Max: b.Max.MaxV(o.Max)}
}

// Expand grows the box by d in every direction.
func (b AABB) Expand(d float64) AABB {
	if !b.Valid() {
		return b
	}
	e := Vec3{d, d, d}
	return AABB{Min: b.Min.Sub(e), Max: b.Max.Add(e)}
}

func (b AABB) Center() Vec3 {
	return b.Min.Add(b.Max).Mul(0.5)
}

func (b AABB) Size() Vec3 {
	if !b.Valid() {
		return Vec3{}
	}
	return b.Max.Sub(b.Min)
}

// Diagonal is the corner-to-corner length, used for camera framing.
func (b AABB) Diagonal() float64 {
	return b.Size().Len()
}

// LongestSide is the largest extent, used for paint texel density (§8.2).
func (b AABB) LongestSide() float64 {
	s := b.Size()
	return math.Max(s.X, math.Max(s.Y, s.Z))
}

func (b AABB) Contains(p Vec3) bool {
	return p.X >= b.Min.X && p.X <= b.Max.X &&
		p.Y >= b.Min.Y && p.Y <= b.Max.Y &&
		p.Z >= b.Min.Z && p.Z <= b.Max.Z
}

// Intersects reports overlap including mere touching.
func (b AABB) Intersects(o AABB) bool {
	return b.Min.X <= o.Max.X && o.Min.X <= b.Max.X &&
		b.Min.Y <= o.Max.Y && o.Min.Y <= b.Max.Y &&
		b.Min.Z <= o.Max.Z && o.Min.Z <= b.Max.Z
}

// Corners returns the eight corners, ordered so bit 0 is X, bit 1 is Y and
// bit 2 is Z (0 = Min side).
func (b AABB) Corners() [8]Vec3 {
	var c [8]Vec3
	for i := 0; i < 8; i++ {
		p := b.Min
		if i&1 != 0 {
			p.X = b.Max.X
		}
		if i&2 != 0 {
			p.Y = b.Max.Y
		}
		if i&4 != 0 {
			p.Z = b.Max.Z
		}
		c[i] = p
	}
	return c
}

// AABBOf builds the bounding box of a point set.
func AABBOf(pts []Vec3) AABB {
	b := Empty()
	for _, p := range pts {
		b = b.AddPoint(p)
	}
	return b
}

// Transform returns the AABB of this box's corners under m. For rotations this
// grows the box, which is the correct conservative answer.
func (b AABB) Transform(m Mat4) AABB {
	if !b.Valid() {
		return b
	}
	out := Empty()
	for _, c := range b.Corners() {
		out = out.AddPoint(m.TransformPoint(c))
	}
	return out
}
