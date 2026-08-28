package model

import (
	"fmt"

	"modeler/internal/geom"
)

// Orientation markers (the user's request, 2026-08-28).
//
// A ship model means nothing to a game engine until it knows which way the
// ship points. The markers are that answer, authored where the answer is
// obvious — on the model itself: one dot for the front, one for the top, and
// one per thruster. The document stores them, the .pxm file carries them, and
// the engine reads orientation and effect anchors instead of guessing from
// geometry or keeping hand-measured tables per hull.

// MarkerKind is what a marker names.
type MarkerKind uint8

const (
	// MarkerFront is the nose: the direction the ship flies.
	MarkerFront MarkerKind = iota
	// MarkerTop is the ship's up, which pins the roll.
	MarkerTop
	// MarkerThruster is one engine port: where an exhaust effect plays.
	MarkerThruster
)

func (k MarkerKind) String() string {
	switch k {
	case MarkerFront:
		return "front"
	case MarkerTop:
		return "top"
	default:
		return "thruster"
	}
}

// Label is the marker's name in the tree.
func (k MarkerKind) Label() string {
	switch k {
	case MarkerFront:
		return "Front"
	case MarkerTop:
		return "Top"
	default:
		return "Thruster"
	}
}

// DefaultThrusterRadius is the bell radius a thruster marker starts with, in
// units. Engines scale it with the model, so it is a proportion, not a size.
const DefaultThrusterRadius = 0.25

// Marker is one authored dot on the model.
type Marker struct {
	Kind MarkerKind `json:"kind"`
	// At is the dot's world position, on the face it was placed on.
	At geom.Vec3 `json:"at"`
	// Dir is the face's outward normal where the dot was placed. For a
	// thruster that is the exhaust direction; for front and top it is a hint
	// the basis math can fall back on when the model's centre is degenerate.
	Dir geom.Vec3 `json:"dir"`
	// R is a thruster's bell radius in units; zero for the other kinds.
	R float64 `json:"r,omitempty"`
}

// FrontMarker and TopMarker return the singleton markers, if placed.
func (d *Document) FrontMarker() (Marker, bool) { return d.markerOf(MarkerFront) }
func (d *Document) TopMarker() (Marker, bool)   { return d.markerOf(MarkerTop) }

func (d *Document) markerOf(k MarkerKind) (Marker, bool) {
	for _, m := range d.Markers {
		if m.Kind == k {
			return m, true
		}
	}
	return Marker{}, false
}

// Thrusters lists the thruster markers in the order they were placed.
func (d *Document) Thrusters() []Marker {
	var out []Marker
	for _, m := range d.Markers {
		if m.Kind == MarkerThruster {
			out = append(out, m)
		}
	}
	return out
}

// PlaceMarker sets a marker. Front and top are singletons — placing again
// moves the existing dot, because a ship with two fronts is not a thing the
// engine could obey. Thrusters accumulate.
type PlaceMarker struct {
	Marker Marker

	// replaced remembers the singleton this displaced, if any.
	replaced    Marker
	hadPrevious bool
	// at is where the marker landed in the slice, for undo.
	at int
}

func (c *PlaceMarker) Name() string {
	if c.Marker.Kind == MarkerThruster {
		return "Add thruster dot"
	}
	return "Set " + c.Marker.Kind.String() + " dot"
}

func (c *PlaceMarker) Do(doc *Document) error {
	if c.Marker.Dir == (geom.Vec3{}) {
		return fmt.Errorf("a marker needs the direction of the face it sits on")
	}
	if c.Marker.Kind == MarkerThruster && c.Marker.R <= 0 {
		c.Marker.R = DefaultThrusterRadius
	}
	if c.Marker.Kind != MarkerThruster {
		for i, m := range doc.Markers {
			if m.Kind == c.Marker.Kind {
				c.replaced, c.hadPrevious, c.at = m, true, i
				doc.Markers[i] = c.Marker
				return nil
			}
		}
	}
	c.hadPrevious = false
	c.at = len(doc.Markers)
	doc.Markers = append(doc.Markers, c.Marker)
	return nil
}

func (c *PlaceMarker) Undo(doc *Document) {
	if c.at < 0 || c.at >= len(doc.Markers) {
		return
	}
	if c.hadPrevious {
		doc.Markers[c.at] = c.replaced
		return
	}
	doc.Markers = append(doc.Markers[:c.at], doc.Markers[c.at+1:]...)
}

func (c *PlaceMarker) Events() []Event {
	return []Event{{Kind: EvMarkersChanged}}
}

// DeleteMarker removes one marker by index.
type DeleteMarker struct {
	Index int

	removed Marker
}

func (c *DeleteMarker) Name() string { return "Delete " + c.removed.Kind.String() + " dot" }

func (c *DeleteMarker) Do(doc *Document) error {
	if c.Index < 0 || c.Index >= len(doc.Markers) {
		return fmt.Errorf("no marker %d", c.Index)
	}
	c.removed = doc.Markers[c.Index]
	doc.Markers = append(doc.Markers[:c.Index], doc.Markers[c.Index+1:]...)
	return nil
}

func (c *DeleteMarker) Undo(doc *Document) {
	if c.Index < 0 || c.Index > len(doc.Markers) {
		return
	}
	doc.Markers = append(doc.Markers, Marker{})
	copy(doc.Markers[c.Index+1:], doc.Markers[c.Index:])
	doc.Markers[c.Index] = c.removed
}

func (c *DeleteMarker) Events() []Event {
	return []Event{{Kind: EvMarkersChanged}}
}
