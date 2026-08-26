// Package render owns every GPU operation: the viewport passes, the ID pick
// pass, camera math and headless capture (SPEC-RENDER). It is one of the few
// packages allowed to import raylib (PLAN §4).
package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
)

// Conversions between our float64 model math and raylib's float32 types.
//
// raylib's Matrix declares its fields row by row (M0, M4, M8, M12 are the first
// row), so the struct's memory order matches our row-major Mat4 index order
// one for one. The mapping below is therefore a straight positional copy.

func toRLMatrix(m geom.Mat4) rl.Matrix {
	return rl.Matrix{
		M0: float32(m[0]), M4: float32(m[1]), M8: float32(m[2]), M12: float32(m[3]),
		M1: float32(m[4]), M5: float32(m[5]), M9: float32(m[6]), M13: float32(m[7]),
		M2: float32(m[8]), M6: float32(m[9]), M10: float32(m[10]), M14: float32(m[11]),
		M3: float32(m[12]), M7: float32(m[13]), M11: float32(m[14]), M15: float32(m[15]),
	}
}

func toRLVec3(v geom.Vec3) rl.Vector3 {
	return rl.Vector3{X: float32(v.X), Y: float32(v.Y), Z: float32(v.Z)}
}

func fromRLVec3(v rl.Vector3) geom.Vec3 {
	return geom.Vec3{X: float64(v.X), Y: float64(v.Y), Z: float64(v.Z)}
}

func toRLVec2(v geom.Vec2) rl.Vector2 {
	return rl.Vector2{X: float32(v.X), Y: float32(v.Y)}
}

// colorToVec4 converts a color to the normalised float vector shaders expect.
func colorToVec4(c color.RGBA) []float32 {
	return []float32{
		float32(c.R) / 255,
		float32(c.G) / 255,
		float32(c.B) / 255,
		float32(c.A) / 255,
	}
}

// encodeID packs a pick-table index into an RGBA color. Index 0 means "nothing"
// and is never assigned, so a cleared (black) pick buffer reads as empty.
func encodeID(id int) color.RGBA {
	return color.RGBA{
		R: uint8(id & 0xFF),
		G: uint8((id >> 8) & 0xFF),
		B: uint8((id >> 16) & 0xFF),
		A: 255,
	}
}

// decodeID unpacks a pick-buffer pixel back into a table index.
func decodeID(c color.RGBA) int {
	return int(c.R) | int(c.G)<<8 | int(c.B)<<16
}
