package geom

import (
	"encoding/json"
	"fmt"
)

// Vectors serialize as arrays, not objects (SPEC-DATA §4).
//
// A saved ship is mostly vertices, and `[0,1,2]` against `{"X":0,"Y":1,"Z":2}`
// is a third of the bytes and rather easier to read in a diff. The document
// format is the only reason these exist; nothing in the geometry cares.

// MarshalJSON writes a 3-vector as [x,y,z].
func (v Vec3) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]float64{v.X, v.Y, v.Z})
}

// UnmarshalJSON reads [x,y,z], and also accepts the object form so a file
// written by an older build still loads (SPEC-DATA §4's tolerant reader).
func (v *Vec3) UnmarshalJSON(data []byte) error {
	var arr [3]float64
	if err := json.Unmarshal(data, &arr); err == nil {
		v.X, v.Y, v.Z = arr[0], arr[1], arr[2]
		return nil
	}
	var obj struct{ X, Y, Z float64 }
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("read a 3-vector: %w", err)
	}
	v.X, v.Y, v.Z = obj.X, obj.Y, obj.Z
	return nil
}

// MarshalJSON writes a sketch point as [x,y] in subunits.
func (v Vec2i) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]int64{v.X, v.Y})
}

// UnmarshalJSON reads [x,y], or the object form.
func (v *Vec2i) UnmarshalJSON(data []byte) error {
	var arr [2]int64
	if err := json.Unmarshal(data, &arr); err == nil {
		v.X, v.Y = arr[0], arr[1]
		return nil
	}
	var obj struct{ X, Y int64 }
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("read a 2-vector: %w", err)
	}
	v.X, v.Y = obj.X, obj.Y
	return nil
}
