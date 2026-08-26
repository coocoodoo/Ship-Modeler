// Package csg is the boolean kernel (SPEC-GEOMETRY §6): the only package that
// talks to Manifold, and the only one in the tree that links C++.
//
// Everything crossing this boundary is a `mesh.Mesh` with polygon faces. What
// happens in between — triangulating, handing Manifold float32 vertices,
// tracking which output triangle came from which input face, and putting the
// polygons back together — is this package's business and nobody else's.
package csg

import (
	"fmt"

	"modeler/internal/geom/mesh"
)

// Op is a boolean operation (SPEC-GEOMETRY §6.4).
type Op uint8

const (
	// Union merges the tools into the target.
	Union Op = iota
	// Subtract cuts each tool out of the target, in order.
	Subtract
	// Intersect keeps only the volume common to the target and every tool.
	Intersect
)

func (o Op) String() string {
	switch o {
	case Subtract:
		return "Subtract"
	case Intersect:
		return "Intersect"
	default:
		return "Union"
	}
}

// Result is what one boolean produced.
type Result struct {
	// Mesh is the solid that came out. Nil when Empty.
	Mesh *mesh.Mesh
	// Volume is Manifold's own report for the result, kept so callers and tests
	// can cross-check it against our `mesh.Volume` (SPEC-GEOMETRY §6.5).
	Volume float64
	// Empty reports a legal nothing: a subtract that swallowed the body, or an
	// intersect of solids that do not meet. Not an error (§6.4).
	Empty bool
	// Loose counts result triangles that could not be merged back into a
	// polygon face. They are valid geometry, just not tidy; tests drive this to
	// zero on the acceptance matrix.
	Loose int
	// NextFaceSeq is the face sequence the result's body should continue from,
	// so a later op keeps minting unused face ids.
	NextFaceSeq uint32
}

// Boolean runs op on a target and one or more tools (SPEC-GEOMETRY §6.4).
//
// Union folds every tool in, Subtract removes each in turn, and Intersect folds
// pairwise. The document is never touched: a failure returns an error and the
// caller still holds exactly the meshes it passed in.
func Boolean(op Op, target *mesh.Mesh, tools ...*mesh.Mesh) (Result, error) {
	if target == nil {
		return Result{}, fmt.Errorf("no target solid")
	}
	if len(tools) == 0 {
		return Result{}, fmt.Errorf("a boolean needs something to combine with")
	}

	// Every input's lineage goes into one table: after the fold, an output run
	// can descend from any of them.
	lineage := map[uint32]origin{}
	acc, err := uploadInto(target, lineage)
	if err != nil {
		return Result{}, fmt.Errorf("the target %w", err)
	}
	defer func() { acc.Close() }()

	for i, tool := range tools {
		if tool == nil {
			return Result{}, fmt.Errorf("tool %d is missing", i+1)
		}
		t, err := uploadInto(tool, lineage)
		if err != nil {
			return Result{}, fmt.Errorf("tool %d %w", i+1, err)
		}
		next, err := apply(op, acc, t)
		t.Close()
		if err != nil {
			return Result{}, err
		}
		acc.Close()
		acc = next

		// A fold that empties partway is finished: nothing left to subtract
		// from, nothing left to intersect with.
		if acc.empty() {
			break
		}
	}

	res := Result{Volume: acc.volume()}
	if acc.empty() {
		res.Empty = true
		return res, nil
	}

	bodyID := bodyOf(target)
	seq := freshSeq(target)
	out, loose, err := download(acc.download(), lineage, bodyID, &seq)
	if err != nil {
		return Result{}, err
	}
	if err := mesh.Validate(out); err != nil {
		return Result{}, fmt.Errorf("the %s produced an invalid solid: %w", op, err)
	}
	res.Mesh, res.Loose, res.NextFaceSeq = out, loose, seq
	return res, nil
}

// uploadInto hands one mesh to Manifold, adding its faces to the shared
// lineage table.
func uploadInto(m *mesh.Mesh, lineage map[uint32]origin) (*solid, error) {
	g, mine, err := upload(m)
	if err != nil {
		return nil, err
	}
	s, err := newSolid(g)
	if err != nil {
		return nil, fmt.Errorf("could not be read as a solid: %w", err)
	}
	for id, o := range mine {
		lineage[id] = o
	}
	return s, nil
}

// bodyOf is the body a mesh's faces belong to, which the result's fresh face
// ids continue from.
func bodyOf(m *mesh.Mesh) uint32 {
	if len(m.Faces) == 0 {
		return 0
	}
	return m.Faces[0].ID.BodyID()
}

// freshSeq is the first face sequence number not already used by the target, so
// no result face can collide with one the document still remembers.
func freshSeq(m *mesh.Mesh) uint32 {
	var max uint32
	body := bodyOf(m)
	for i := range m.Faces {
		if id := m.Faces[i].ID; id.BodyID() == body && id.Seq() >= max {
			max = id.Seq() + 1
		}
	}
	return max
}

// Volume reports Manifold's own volume for a mesh, which is the independent
// number the validation gate of SPEC-GEOMETRY §6.5 checks ours against.
func Volume(m *mesh.Mesh) (float64, error) {
	if m == nil {
		return 0, fmt.Errorf("no solid given")
	}
	g, _, err := upload(m)
	if err != nil {
		return 0, err
	}
	s, err := newSolid(g)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	return s.volume(), nil
}
