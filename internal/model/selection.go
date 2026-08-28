package model

import (
	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Selection is app state, not document state: it is deliberately outside the
// undo history and is cleared or remapped when a structural change lands
// (SPEC-DATA §2). It lives in this package so tools and the tree can share one
// vocabulary for "what is selected".

// SelKind is what a selection reference points at.
type SelKind uint8

const (
	SelNone SelKind = iota
	SelBody
	SelFace
	SelEdge
	SelVert
	SelPlane
	SelSketch
	SelMarker
)

func (k SelKind) String() string {
	switch k {
	case SelBody:
		return "body"
	case SelFace:
		return "face"
	case SelEdge:
		return "edge"
	case SelVert:
		return "vertex"
	case SelPlane:
		return "plane"
	case SelSketch:
		return "sketch"
	case SelMarker:
		return "dot"
	default:
		return "nothing"
	}
}

// Ref is one selected element.
type Ref struct {
	Kind   SelKind
	Body   uint32
	Face   mesh.FaceUID
	Edge   int
	Vert   int
	Plane  geom.PlaneKind
	Sketch uint32
	// Marker is an index into Document.Markers. Markers have no stable id of
	// their own, so this shifts when one is deleted — which is why Prune
	// checks it, exactly as it does for edge and vertex indices.
	Marker int
}

// BodyRef, PlaneRef and SketchRef are the constructors the tree panel uses.
func BodyRef(id uint32) Ref                { return Ref{Kind: SelBody, Body: id} }
func PlaneRef(p geom.PlaneKind) Ref        { return Ref{Kind: SelPlane, Plane: p} }
func SketchRef(id uint32) Ref              { return Ref{Kind: SelSketch, Sketch: id} }
func FaceRef(b uint32, f mesh.FaceUID) Ref { return Ref{Kind: SelFace, Body: b, Face: f} }
func EdgeRef(b uint32, e int) Ref          { return Ref{Kind: SelEdge, Body: b, Edge: e} }
func VertRef(b uint32, v int) Ref          { return Ref{Kind: SelVert, Body: b, Vert: v} }
func MarkerRef(i int) Ref                  { return Ref{Kind: SelMarker, Marker: i} }

// Selection is an ordered set of references. Order matters: the first entry is
// the primary selection, which is what the boolean tool keeps and what a gizmo
// anchors to.
type Selection struct {
	refs []Ref
}

// Len reports how many elements are selected.
func (s *Selection) Len() int { return len(s.refs) }

// Empty reports whether nothing is selected.
func (s *Selection) Empty() bool { return len(s.refs) == 0 }

// Refs returns the selected elements. The slice must not be modified.
func (s *Selection) Refs() []Ref { return s.refs }

// Primary returns the first selected element.
func (s *Selection) Primary() (Ref, bool) {
	if len(s.refs) == 0 {
		return Ref{}, false
	}
	return s.refs[0], true
}

// Kind reports the kind of a homogeneous selection, or SelNone when the
// selection is empty or mixed. Tools use it to decide which gizmo to show.
func (s *Selection) Kind() SelKind {
	if len(s.refs) == 0 {
		return SelNone
	}
	k := s.refs[0].Kind
	for _, r := range s.refs[1:] {
		if r.Kind != k {
			return SelNone
		}
	}
	return k
}

// Contains reports whether a reference is selected.
func (s *Selection) Contains(r Ref) bool {
	for _, x := range s.refs {
		if x == r {
			return true
		}
	}
	return false
}

// Set replaces the selection with a single reference.
func (s *Selection) Set(r Ref) {
	s.refs = append(s.refs[:0], r)
}

// SetAll replaces the selection wholesale.
func (s *Selection) SetAll(refs []Ref) {
	s.refs = append(s.refs[:0], refs...)
}

// Add appends a reference if it is not already selected (Shift-click).
func (s *Selection) Add(r Ref) {
	if !s.Contains(r) {
		s.refs = append(s.refs, r)
	}
}

// Toggle adds or removes a reference (Ctrl-click).
func (s *Selection) Toggle(r Ref) {
	for i, x := range s.refs {
		if x == r {
			s.refs = append(s.refs[:i], s.refs[i+1:]...)
			return
		}
	}
	s.refs = append(s.refs, r)
}

// Remove drops a reference if present.
func (s *Selection) Remove(r Ref) {
	for i, x := range s.refs {
		if x == r {
			s.refs = append(s.refs[:i], s.refs[i+1:]...)
			return
		}
	}
}

// Clear empties the selection.
func (s *Selection) Clear() { s.refs = s.refs[:0] }

// Bodies returns the ids of every selected body, in selection order.
func (s *Selection) Bodies() []uint32 {
	var out []uint32
	for _, r := range s.refs {
		if r.Kind == SelBody {
			out = append(out, r.Body)
		}
	}
	return out
}

// Prune drops references to things the document no longer contains, which is
// what a structural change (delete, undo of an add) requires.
func (s *Selection) Prune(doc *Document) {
	kept := s.refs[:0]
	for _, r := range s.refs {
		switch r.Kind {
		case SelBody, SelFace, SelEdge, SelVert:
			if doc.BodyByID(r.Body) == nil {
				continue
			}
		case SelSketch:
			if doc.SketchByID(r.Sketch) == nil {
				continue
			}
		case SelPlane:
			if r.Plane < 0 || int(r.Plane) >= geom.PlaneCount {
				continue
			}
		case SelMarker:
			if r.Marker < 0 || r.Marker >= len(doc.Markers) {
				continue
			}
		case SelNone:
			continue
		}
		kept = append(kept, r)
	}
	s.refs = kept
}

// Describe names the selection for the hint bar, e.g. "Body 2" or
// "3 vertices".
func (s *Selection) Describe(doc *Document) string {
	switch len(s.refs) {
	case 0:
		return ""
	case 1:
		r := s.refs[0]
		switch r.Kind {
		case SelBody:
			if b := doc.BodyByID(r.Body); b != nil {
				return b.Name
			}
		case SelSketch:
			if sk := doc.SketchByID(r.Sketch); sk != nil {
				return sk.Name
			}
		case SelPlane:
			return r.Plane.String() + " plane"
		case SelFace:
			// A face is only meaningful with the body it belongs to: "a face"
			// on its own tells you nothing about what you clicked.
			if b := doc.BodyByID(r.Body); b != nil {
				return "a face of " + b.Name
			}
		case SelMarker:
			return doc.MarkerLabel(r.Marker)
		}
		return r.Kind.String()
	default:
		k := s.Kind()
		if k == SelNone {
			return plural(len(s.refs), "item", "items")
		}
		return plural(len(s.refs), k.String(), pluralWord(k))
	}
}

func pluralWord(k SelKind) string {
	switch k {
	case SelBody:
		return "bodies"
	case SelVert:
		return "vertices"
	case SelMarker:
		return "dots"
	default:
		return k.String() + "s"
	}
}
