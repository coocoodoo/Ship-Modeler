package model

import (
	"fmt"
	"math"
	"sort"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Direct edits and transforms (R10, R11, SPEC-GEOMETRY §7).
//
// Everything here moves vertices. A vertex move, an edge move, a face move and
// a body move are the same operation on different sets of indices, which is why
// selection resolves to a set of vertices before any of this runs: a vertex
// shared by two selected faces has to move exactly once, and the only way to
// guarantee that is to have decided what moves before deciding how.

// VertIndices resolves a selection into the vertices it would move, grouped by
// body (SPEC-GEOMETRY §7.1).
//
// The result is a set per body, deduplicated and sorted, so the same selection
// always produces the same move and a vertex named twice moves once.
func (s *Selection) VertIndices(doc *Document) map[uint32][]int {
	sets := map[uint32]map[int]bool{}
	add := func(body uint32, verts ...int) {
		set := sets[body]
		if set == nil {
			set = map[int]bool{}
			sets[body] = set
		}
		for _, v := range verts {
			set[v] = true
		}
	}

	for _, ref := range s.refs {
		b := doc.BodyByID(ref.Body)
		if b == nil || b.Mesh == nil {
			continue
		}
		switch ref.Kind {
		case SelVert:
			if ref.Vert >= 0 && ref.Vert < len(b.Mesh.Verts) {
				add(ref.Body, ref.Vert)
			}
		case SelEdge:
			t := b.Mesh.Topo()
			if ref.Edge >= 0 && ref.Edge < len(t.Edges) {
				e := t.Edges[ref.Edge]
				add(ref.Body, e.A, e.B)
			}
		case SelFace:
			if fi := faceIndex(b.Mesh, ref.Face); fi >= 0 {
				for _, loop := range b.Mesh.Faces[fi].Loops {
					add(ref.Body, loop...)
				}
			}
		case SelBody:
			all := make([]int, len(b.Mesh.Verts))
			for i := range all {
				all[i] = i
			}
			add(ref.Body, all...)
		}
	}

	out := make(map[uint32][]int, len(sets))
	for body, set := range sets {
		list := make([]int, 0, len(set))
		for v := range set {
			list = append(list, v)
		}
		sort.Ints(list)
		out[body] = list
	}
	return out
}

// Pivot is the centre of a selection, which is where a gizmo sits and what a
// rotation turns about.
func (s *Selection) Pivot(doc *Document) (geom.Vec3, bool) {
	// A marker has no vertices — it is a point in its own right, and the gizmo
	// belongs exactly on it (the user's request, 2026-08-28).
	if at, ok := s.markerPivot(doc); ok {
		return at, true
	}
	verts := s.VertIndices(doc)
	var sum geom.Vec3
	var n float64
	for body, list := range verts {
		b := doc.BodyByID(body)
		if b == nil || b.Mesh == nil {
			continue
		}
		for _, vi := range list {
			sum = sum.Add(b.Mesh.Verts[vi])
			n++
		}
	}
	if n == 0 {
		return geom.Vec3{}, false
	}
	return sum.Mul(1 / n), true
}

// markerPivot averages the selected dots, when dots are what is selected. A
// selection mixing dots with geometry has no single sensible pivot, so it
// falls through to the vertex answer and the dots simply do not move.
func (s *Selection) markerPivot(doc *Document) (geom.Vec3, bool) {
	var sum geom.Vec3
	n := 0.0
	for _, r := range s.refs {
		if r.Kind != SelMarker {
			return geom.Vec3{}, false
		}
		if r.Marker < 0 || r.Marker >= len(doc.Markers) {
			continue
		}
		sum = sum.Add(doc.Markers[r.Marker].At)
		n++
	}
	if n == 0 {
		return geom.Vec3{}, false
	}
	return sum.Mul(1 / n), true
}

// MarkerIndices lists the selected dots, which is what the gizmo moves when a
// dot is what it is anchored to.
func (s *Selection) MarkerIndices(doc *Document) []int {
	var out []int
	for _, r := range s.refs {
		if r.Kind == SelMarker && r.Marker >= 0 && r.Marker < len(doc.Markers) {
			out = append(out, r.Marker)
		}
	}
	return out
}

// vertEdit is the shared machinery of every direct edit: apply a point map to a
// set of vertices, remember what was there, and re-check what got bent.
type vertEdit struct {
	// Verts is what moves, grouped by body.
	Verts map[uint32][]int

	// FoldBent asks the edit to split any face it bends into planar pieces
	// along the crease between what moved and what stayed (the user's
	// request, 2026-08-28). The commit path sets it; drag frames leave it
	// off, because folding mid-drag would churn topology sixty times a
	// second for a crease that only the final position can place.
	FoldBent bool

	// before holds the original positions so undo is exact rather than
	// arithmetic run backwards — a rotation undone by rotating the other way
	// accumulates error, and this does not.
	before map[uint32][]geom.Vec3
	// bentBefore remembers which faces were already flagged, so undo restores
	// the flags rather than merely clearing them.
	bentBefore map[uint32][]bool
	// Paint anchors are snapshots too: drag replacement and undo must restore
	// them along with the vertices, without mutating shared mesh/CSG history.
	paintBefore map[uint32][]*mesh.FacePaint
	// facesBefore and seqBefore snapshot what folding rewrites: the face list
	// and the identity counter. Nil for bodies that did not fold.
	facesBefore map[uint32][]mesh.Face
	seqBefore   map[uint32]uint32
	bent        int
	folded      int
	leftGrid    bool
}

// Bent is how many faces the edit left non-planar (SPEC-GEOMETRY §7.2).
// With folding on, that is what remains after the fold — holed faces, or a
// loop no chord could flatten.
func (e *vertEdit) Bent() int { return e.bent }

// Folded is how many bent faces the edit split into planar pieces.
func (e *vertEdit) Folded() int { return e.folded }

// LeftTheGrid reports that this edit took vertices off the subunit lattice —
// that something which was on the grid no longer is.
//
// It asks about the change, not about the result. A body already off the grid
// from an earlier free rotation stays off it through a subsequent quarter turn,
// and saying "free rotation leaves the pixel grid" again there would be telling
// the user something they did not just do (SPEC-GEOMETRY §7.3).
func (e *vertEdit) LeftTheGrid() bool { return e.leftGrid }

// apply runs a point map over the selected vertices.
func (e *vertEdit) apply(doc *Document, move func(geom.Vec3) geom.Vec3, rotate func(geom.Vec3) geom.Vec3) error {
	if len(e.Verts) == 0 {
		return fmt.Errorf("nothing is selected to move")
	}
	// Validate everything before writing anything.
	type target struct {
		body  *Body
		verts []int
	}
	targets := make([]target, 0, len(e.Verts))
	for id, list := range e.Verts {
		b := doc.BodyByID(id)
		if b == nil || b.Mesh == nil {
			return fmt.Errorf("that body is no longer there")
		}
		for _, vi := range list {
			if vi < 0 || vi >= len(b.Mesh.Verts) {
				return fmt.Errorf("vert %d is not in %s any more", vi, b.Name)
			}
		}
		targets = append(targets, target{body: b, verts: list})
	}
	// A deterministic order keeps the flag counts and any future diagnostics
	// identical run to run.
	sort.Slice(targets, func(i, j int) bool { return targets[i].body.ID < targets[j].body.ID })

	e.before = make(map[uint32][]geom.Vec3, len(targets))
	e.bentBefore = make(map[uint32][]bool, len(targets))
	e.paintBefore = make(map[uint32][]*mesh.FacePaint, len(targets))
	e.facesBefore = make(map[uint32][]mesh.Face, len(targets))
	e.seqBefore = make(map[uint32]uint32, len(targets))
	e.bent = 0
	e.folded = 0
	e.leftGrid = false

	for _, t := range targets {
		m := t.body.Mesh
		flags := make([]bool, len(m.Faces))
		for i := range m.Faces {
			flags[i] = m.Faces[i].NonPlanar
		}
		e.bentBefore[t.body.ID] = flags
		paints := make([]*mesh.FacePaint, len(m.Faces))
		for i := range m.Faces {
			paints[i] = m.Faces[i].Paint
		}
		e.paintBefore[t.body.ID] = paints
		movedSet := make(map[int]bool, len(t.verts))
		for _, vi := range t.verts {
			movedSet[vi] = true
		}
		// Fully moved faces undergo the same rigid motion as their paint.
		// Partially moved faces retain their mapping and texel density while
		// their outline stretches. Shared allocations are copied once, so
		// stationary faces and undo snapshots keep their original anchors.
		movedPaint := map[*mesh.FacePaint]*mesh.FacePaint{}
		for fi := range m.Faces {
			f := &m.Faces[fi]
			if f.Paint == nil {
				continue
			}
			whole := true
			for _, loop := range f.Loops {
				for _, vi := range loop {
					whole = whole && movedSet[vi]
				}
			}
			if !whole {
				continue
			}
			p := movedPaint[f.Paint]
			if p == nil {
				cp := *f.Paint
				cp.Frame.O = move(cp.Frame.O)
				if rotate != nil {
					cp.Frame.U = rotate(cp.Frame.U)
					cp.Frame.V = rotate(cp.Frame.V)
					cp.Frame.N = rotate(cp.Frame.N)
				}
				p = &cp
				movedPaint[f.Paint] = p
			}
			f.Paint = p
		}

		saved := make([]geom.Vec3, len(t.verts))
		for i, vi := range t.verts {
			saved[i] = m.Verts[vi]
			p := move(m.Verts[vi])
			if onLattice(saved[i]) && !onLattice(p) {
				e.leftGrid = true
			}
			m.Verts[vi] = p
		}
		e.before[t.body.ID] = saved

		m.InvalidateCaches()
		bent := m.RecheckPlanarity()
		if e.FoldBent && bent > 0 {
			// Folding rewrites the face list and mints identities, so both
			// are snapshotted first; the copies are safe because the fold
			// only ever reads the old Face structs, never edits their loops.
			e.facesBefore[t.body.ID] = append([]mesh.Face(nil), m.Faces...)
			e.seqBefore[t.body.ID] = t.body.FaceSeq
			e.folded += mesh.FoldBent(m, movedSet, t.body.NextFaceUID)
			bent = m.RecheckPlanarity()
		}
		e.bent += bent
	}
	return nil
}

// undo puts every moved vertex back exactly where it was.
func (e *vertEdit) undo(doc *Document) {
	for id, saved := range e.before {
		b := doc.BodyByID(id)
		if b == nil || b.Mesh == nil {
			continue
		}
		if faces := e.facesBefore[id]; faces != nil {
			b.Mesh.Faces = faces
			b.FaceSeq = e.seqBefore[id]
		}
		for fi, p := range e.paintBefore[id] {
			b.Mesh.Faces[fi].Paint = p
		}
		for i, vi := range e.Verts[id] {
			if vi >= 0 && vi < len(b.Mesh.Verts) {
				b.Mesh.Verts[vi] = saved[i]
			}
		}
		b.Mesh.InvalidateCaches()
		if flags := e.bentBefore[id]; len(flags) == len(b.Mesh.Faces) {
			for i := range b.Mesh.Faces {
				b.Mesh.Faces[i].NonPlanar = flags[i]
			}
		} else {
			b.Mesh.RecheckPlanarity()
		}
	}
}

func (e *vertEdit) events() []Event {
	out := make([]Event, 0, len(e.Verts))
	for id := range e.Verts {
		out = append(out, Event{Kind: EvBodyChanged, BodyID: id})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BodyID < out[j].BodyID })
	return out
}

// onLattice reports whether a point sits exactly on the subunit grid.
func onLattice(p geom.Vec3) bool {
	for _, c := range [3]float64{p.X, p.Y, p.Z} {
		if c*geom.Unit != math.Trunc(c*geom.Unit) {
			return false
		}
	}
	return true
}

// MoveVerts translates a set of vertices (R10, SPEC-GEOMETRY §7.1).
type MoveVerts struct {
	vertEdit
	Delta geom.Vec3
	// What names the selection for the undo history, e.g. "face" or "3 verts".
	What string
}

// NewMoveVerts is how a caller builds a move: the vertices come from a
// selection, and What is what the undo history will call it.
func NewMoveVerts(verts map[uint32][]int, delta geom.Vec3, what string) *MoveVerts {
	return &MoveVerts{vertEdit: vertEdit{Verts: verts}, Delta: delta, What: what}
}

func (c *MoveVerts) Name() string {
	if c.What == "" {
		return "Move"
	}
	return "Move " + c.What
}

func (c *MoveVerts) Do(doc *Document) error {
	return c.apply(doc, func(p geom.Vec3) geom.Vec3 { return p.Add(c.Delta) }, nil)
}

func (c *MoveVerts) Undo(doc *Document) { c.undo(doc) }
func (c *MoveVerts) Events() []Event    { return c.events() }

// RotateVerts turns a set of vertices about an axis through a pivot
// (R11, SPEC-GEOMETRY §7.3).
type RotateVerts struct {
	vertEdit
	Pivot   geom.Vec3
	Axis    geom.Vec3
	Degrees float64
	What    string
}

// NewRotateVerts builds a rotation about an axis through a pivot.
func NewRotateVerts(verts map[uint32][]int, pivot, axis geom.Vec3, degrees float64, what string) *RotateVerts {
	return &RotateVerts{
		vertEdit: vertEdit{Verts: verts},
		Pivot:    pivot, Axis: axis, Degrees: degrees, What: what,
	}
}

func (c *RotateVerts) Name() string {
	if c.What == "" {
		return "Rotate"
	}
	return "Rotate " + c.What
}

func (c *RotateVerts) Do(doc *Document) error {
	if turn, exact := quarterTurns(c.Axis, c.Degrees); exact {
		// A quarter turn about a world axis is a permutation of coordinates and
		// a sign flip. Doing it that way rather than through a rotation matrix
		// is not an optimisation: sin(pi/2) is 1 but cos(pi/2) is 6.1e-17, and
		// that is enough to take every vertex off the grid and keep it off.
		return c.apply(doc, func(p geom.Vec3) geom.Vec3 {
			return c.Pivot.Add(turn(p.Sub(c.Pivot)))
		}, turn)
	}
	rot := geom.RotateAxis(c.Axis, c.Degrees*math.Pi/180)
	return c.apply(doc, func(p geom.Vec3) geom.Vec3 {
		return c.Pivot.Add(rot.TransformDir(p.Sub(c.Pivot)))
	}, rot.TransformDir)
}

func (c *RotateVerts) Undo(doc *Document) { c.undo(doc) }
func (c *RotateVerts) Events() []Event    { return c.events() }

// quarterTurns returns the exact coordinate shuffle for a multiple of 90
// degrees about a world axis, and whether the request is one.
func quarterTurns(axis geom.Vec3, degrees float64) (func(geom.Vec3) geom.Vec3, bool) {
	if math.Mod(math.Abs(degrees), 90) != 0 {
		return nil, false
	}
	// Quarters, normalised to 0..3.
	q := int(math.Round(degrees/90)) % 4
	if q < 0 {
		q += 4
	}

	var about int
	switch {
	case math.Abs(axis.X) == 1 && axis.Y == 0 && axis.Z == 0:
		about, q = 0, orient(q, axis.X)
	case axis.X == 0 && math.Abs(axis.Y) == 1 && axis.Z == 0:
		about, q = 1, orient(q, axis.Y)
	case axis.X == 0 && axis.Y == 0 && math.Abs(axis.Z) == 1:
		about, q = 2, orient(q, axis.Z)
	default:
		return nil, false
	}

	return func(p geom.Vec3) geom.Vec3 {
		for i := 0; i < q; i++ {
			switch about {
			case 0: // X: Y -> Z -> -Y
				p = geom.Vec3{X: p.X, Y: -p.Z, Z: p.Y}
			case 1: // Y: Z -> X -> -Z
				p = geom.Vec3{X: p.Z, Y: p.Y, Z: -p.X}
			default: // Z: X -> Y -> -X
				p = geom.Vec3{X: -p.Y, Y: p.X, Z: p.Z}
			}
		}
		return p
	}, true
}

// orient flips the quarter count when the axis points the other way.
func orient(q int, sign float64) int {
	if sign < 0 {
		return (4 - q) % 4
	}
	return q
}

// DuplicateBody copies a body one unit along X (SPEC-UX §12.4).
type DuplicateBody struct {
	ID uint32

	copy *Body
}

func (c *DuplicateBody) Name() string {
	if c.copy != nil {
		return "Duplicate " + c.copy.Name
	}
	return "Duplicate"
}

// Copy returns the body the command created.
func (c *DuplicateBody) Copy() *Body { return c.copy }

func (c *DuplicateBody) Do(doc *Document) error {
	src := doc.BodyByID(c.ID)
	if src == nil || src.Mesh == nil {
		return fmt.Errorf("that body is no longer there")
	}
	if c.copy == nil {
		id := doc.Seq.NextBody()
		m := src.Mesh.Clone()
		// Face identities belong to the body that owns them: a copy that kept
		// the original's would have paint and selection following it home.
		for i := range m.Faces {
			m.Faces[i].ID = mesh.MakeFaceUID(id, uint32(i))
		}
		mesh.Translate(m, geom.Vec3{X: 1})
		c.copy = &Body{
			ID: id, Name: fmt.Sprintf("Body %d", id), Color: src.Color,
			Visible: true, Mesh: m, FaceSeq: uint32(len(m.Faces)),
		}
	}
	doc.Bodies = append(doc.Bodies, c.copy)
	return nil
}

func (c *DuplicateBody) Undo(doc *Document) {
	if i := doc.bodyIndex(c.copy.ID); i >= 0 {
		doc.Bodies = append(doc.Bodies[:i], doc.Bodies[i+1:]...)
	}
}

func (c *DuplicateBody) Events() []Event {
	return []Event{{Kind: EvBodyAdded, BodyID: c.copy.ID}}
}
