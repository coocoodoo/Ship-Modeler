// Package model owns the document: bodies, sketches, planes, the command bus
// that mutates them, and the change events derived caches listen to
// (SPEC-DATA §2, §3).
//
// It is raylib-free and cgo-free, so its tests run without a window.
package model

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
	"time"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
	"modeler/internal/geom/sketch2d"
)

// FormatVersion is the .ship document version this build writes (SPEC-DATA §4).
const FormatVersion = 1

// Sequences are the per-kind monotonic counters. They never reuse a number and
// they survive save and load, which is what makes FaceUIDs stable references
// for paint and boolean provenance (SPEC-DATA §1).
type Sequences struct {
	NotePin uint32 `json:"notePin,omitempty"`
	Body    uint32 `json:"body"`
	Sketch  uint32 `json:"sketch"`
}

// NextBody returns the next body id.
func (s *Sequences) NextBody() uint32 { s.Body++; return s.Body }

// NextSketch returns the next sketch id.
func (s *Sequences) NextSketch() uint32 { s.Sketch++; return s.Sketch }

// PlaneState is all a default plane carries. The three planes always exist and
// can only be hidden, never deleted or renamed (R1), so there is nothing else
// to store.
type PlaneState struct {
	Visible bool `json:"visible"`
}

// Body is one modelled solid.
//
// Mesh is a pointer so a command can swap a whole new mesh in as a single
// assignment: that is what makes "a failed boolean changes nothing" true by
// construction (SPEC-DATA §3.1).
type Body struct {
	ID      uint32     `json:"id"`
	Name    string     `json:"name"`
	Color   color.RGBA `json:"color"`
	Visible bool       `json:"visible"`
	Mesh    *mesh.Mesh `json:"mesh"`

	// FaceSeq is the per-body face counter behind FaceUID (SPEC-DATA §1).
	FaceSeq uint32 `json:"faceSeq"`
}

// NextFaceUID mints a fresh, never-reused face identity for this body.
func (b *Body) NextFaceUID() mesh.FaceUID {
	b.FaceSeq++
	return mesh.MakeFaceUID(b.ID, b.FaceSeq)
}

// TriangleCount reports the body's rendered triangle count for the stats line.
func (b *Body) TriangleCount() int {
	if b.Mesh == nil {
		return 0
	}
	return b.Mesh.TriangleCount()
}

// Sketch is a 2D profile on a plane. Its entities are stored in the logical
// form the user drew — a rectangle stays a rectangle — and expand to segments
// only for the region engine (SPEC-GEOMETRY §3).
type Sketch struct {
	ID      uint32 `json:"id"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`

	// Plane is the default plane a sketch was drawn on, meaningful only when
	// the sketch is not anchored to a face.
	Plane geom.PlaneKind `json:"plane"`

	// OnFace anchors the sketch to a body's face instead (R9, SPEC-UX §10).
	// Body and FaceID say which one, and FrameSnap is that face's plane as it
	// was at the moment the sketch started.
	//
	// The snapshot is the point. A face is a transient thing — the next boolean
	// can cut it in half, move it, or remove it — but a sketch drawn on it is
	// work somebody did, and losing that work because the surface underneath it
	// changed would be indefensible. The sketch keeps its plane; only the snap
	// references go stale (SPEC-GEOMETRY §3).
	OnFace    bool         `json:"onFace,omitempty"`
	Body      uint32       `json:"body,omitempty"`
	FaceID    mesh.FaceUID `json:"face,omitempty"`
	FrameSnap geom.Frame   `json:"frame,omitempty"`

	// Entities are what was drawn, in draw order.
	Entities []Entity `json:"entities"`

	// Consumed records that an extrude has already used this sketch, which the
	// UI must say out loud rather than pretend otherwise (SPEC-UX §8.8).
	Consumed bool `json:"consumed"`

	// Derived arrangement cache, never serialized. stamp advances on every
	// edit; arrStamp records which stamp the cache was built from.
	stamp       uint64
	arrStamp    uint64
	arrCached   bool
	arrangement sketch2d.Arrangement
}

// CameraState is the saved view. It mirrors render.Camera without importing it:
// the dependency runs the other way (PLAN §4).
type CameraState struct {
	Target      [3]float64 `json:"target"`
	Azimuth     float64    `json:"azimuth"`
	Elevation   float64    `json:"elevation"`
	Dist        float64    `json:"dist"`
	OrthoScale  float64    `json:"orthoScale"`
	Perspective bool       `json:"perspective"`
}

// FeatureRec is one entry of the append-only feature log. v1 never replays it;
// it exists for debugging and forward compatibility (D-05, SPEC-DATA §2).
type FeatureRec struct {
	Kind   string          `json:"kind"`
	Params json.RawMessage `json:"params,omitempty"`
	Time   time.Time       `json:"time"`
}

// Document is everything a .ship file holds.
type Document struct {
	FormatVersion int                         `json:"formatVersion"`
	Seq           Sequences                   `json:"seq"`
	Planes        [geom.PlaneCount]PlaneState `json:"planes"`
	Sketches      []*Sketch                   `json:"sketches"`
	Bodies        []*Body                     `json:"bodies"`
	Features      []FeatureRec                `json:"features"`
	Camera        CameraState                 `json:"camera"`
	// Markers are the orientation dots a game engine reads: front, top and
	// thruster positions (the user's request, 2026-08-28). Absent in files
	// saved before they existed, which the tolerant reader treats as none.
	Markers []Marker `json:"markers,omitempty"`
	// NotePins are authoring requests, never game attachment markers.
	NotePins []NotePin `json:"notePins,omitempty"`

	// DirtySinceSave drives the autosave timer and the close prompt.
	DirtySinceSave bool `json:"-"`
}

// NewDocument returns an empty document with all three planes visible.
func NewDocument() *Document {
	d := &Document{FormatVersion: FormatVersion}
	for i := range d.Planes {
		d.Planes[i].Visible = true
	}
	return d
}

// PlaneVisible reports whether a default plane is shown.
func (d *Document) PlaneVisible(p geom.PlaneKind) bool {
	if p < 0 || int(p) >= geom.PlaneCount {
		return false
	}
	return d.Planes[p].Visible
}

// BodyByID returns a body, or nil.
func (d *Document) BodyByID(id uint32) *Body {
	for _, b := range d.Bodies {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// BodyByName returns a body by its exact display name, or nil.
func (d *Document) BodyByName(name string) *Body {
	for _, b := range d.Bodies {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// SketchByID returns a sketch, or nil.
func (d *Document) SketchByID(id uint32) *Sketch {
	for _, s := range d.Sketches {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// SketchByName returns a sketch by its exact display name, or nil.
func (d *Document) SketchByName(name string) *Sketch {
	for _, s := range d.Sketches {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// bodyIndex returns a body's position in the slice, or -1.
func (d *Document) bodyIndex(id uint32) int {
	for i, b := range d.Bodies {
		if b.ID == id {
			return i
		}
	}
	return -1
}

func (d *Document) sketchIndex(id uint32) int {
	for i, s := range d.Sketches {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// IsEmpty reports whether the document has nothing in it yet, which the welcome
// and empty states key off (SPEC-UX §14).
func (d *Document) IsEmpty() bool {
	return len(d.Bodies) == 0 && len(d.Sketches) == 0
}

// Stats summarises the document for the tree panel's footer line (SPEC-UX §7).
type Stats struct {
	Bodies    int
	Sketches  int
	Triangles int
}

// Stats counts what the footer line reports.
func (d *Document) Stats() Stats {
	s := Stats{Bodies: len(d.Bodies), Sketches: len(d.Sketches)}
	for _, b := range d.Bodies {
		s.Triangles += b.TriangleCount()
	}
	return s
}

// String renders the footer line, e.g. "3 bodies · 1,204 tris".
func (s Stats) String() string {
	return fmt.Sprintf("%s · %s", plural(s.Bodies, "body", "bodies"), commas(s.Triangles)+" tris")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// commas groups thousands so large triangle counts stay readable.
func commas(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// CleanName trims a user-entered name and reports whether it is usable.
// Rename is free text but never empty (SPEC-DATA §1).
func CleanName(s string) (string, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", fmt.Errorf("name can't be empty")
	}
	const maxNameLen = 120
	if len(t) > maxNameLen {
		t = t[:maxNameLen]
	}
	return t, nil
}
