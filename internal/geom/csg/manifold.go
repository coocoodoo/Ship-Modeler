package csg

// The Manifold binding (SPEC-GEOMETRY §6.2). Small on purpose: construct a
// solid from triangles, run one boolean, read the triangles back, ask for a
// volume, free everything. Nothing else crosses.
//
// Two rules hold throughout. Every handle is freed explicitly by its owner —
// no finalizers, because a finalizer frees at a time nobody chose and these are
// C++ objects holding real memory. And nothing panics: a bad input or a refused
// operation comes back as an error, because this runs behind a user's click.

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/manifold/include
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/manifold/lib -lmanifoldc -lmanifold -lClipper2 -lstdc++

#include <stdlib.h>
#include <manifold/manifoldc.h>

// Defined in shim.cpp: C++ operator new/delete, which is the allocator
// Manifold's own destructors pair with.
void* csgAlloc(size_t n);
void csgFree(void* p);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// glMesh is the triangle soup Manifold speaks, with the run table that carries
// provenance: triangles [runIndex[i], runIndex[i+1]) all came from the source
// face whose reserved id is runOriginalID[i].
type glMesh struct {
	verts []float64 // x,y,z per vertex
	tris  []uint64  // three vertex indices per triangle
	runs  []uint64  // start triangle of each run, plus a final total
	ids   []uint32  // reserved Manifold id per run
	// mergeFrom and mergeTo are Manifold's own verdict on which duplicated
	// vertices are topologically the same point. Only set on output.
	mergeFrom []uint64
	mergeTo   []uint64
}

// solid is a live Manifold handle. Close is mandatory.
//
// The C API constructs into a buffer the caller supplies and hands back a
// reinterpret-cast of it, so the handle and the buffer are the same address.
// That makes `manifold_delete_manifold` the whole of the cleanup: it runs the
// destructor and releases the memory. Freeing the buffer afterwards as well is
// a double free, which is a lesson this file learned the hard way.
type solid struct {
	ptr *C.ManifoldManifold
}

// errStatus turns a Manifold status code into an error a person can read.
func errStatus(code C.ManifoldError) error {
	switch code {
	case C.MANIFOLD_NO_ERROR:
		return nil
	case C.MANIFOLD_NON_FINITE_VERTEX:
		return fmt.Errorf("a vertex is not a finite number")
	case C.MANIFOLD_NOT_MANIFOLD:
		return fmt.Errorf("the solid is not closed: some edge does not have exactly two faces")
	case C.MANIFOLD_VERTEX_INDEX_OUT_OF_BOUNDS:
		return fmt.Errorf("a triangle refers to a vertex that is not there")
	case C.MANIFOLD_PROPERTIES_WRONG_LENGTH:
		return fmt.Errorf("the vertex property array is the wrong length")
	case C.MANIFOLD_MERGE_VECTORS_DIFFERENT_LENGTHS,
		C.MANIFOLD_MERGE_INDEX_OUT_OF_BOUNDS:
		return fmt.Errorf("the vertex merge table is inconsistent")
	case C.MANIFOLD_RUN_INDEX_WRONG_LENGTH,
		C.MANIFOLD_FACE_ID_WRONG_LENGTH:
		return fmt.Errorf("the face provenance table is the wrong length")
	case C.MANIFOLD_TRANSFORM_WRONG_LENGTH:
		return fmt.Errorf("the transform array is the wrong length")
	case C.MANIFOLD_INVALID_CONSTRUCTION:
		return fmt.Errorf("the solid could not be constructed")
	case C.MANIFOLD_RESULT_TOO_LARGE:
		return fmt.Errorf("the result is too large to build")
	case C.MANIFOLD_CANCELLED:
		return fmt.Errorf("the operation was cancelled")
	default:
		return fmt.Errorf("manifold error %d", int(code))
	}
}

// reserveIDs claims n consecutive original-ids from Manifold and returns the
// first. Ids reserved this way survive splits and merges, which is what makes
// provenance work at all (SPEC-GEOMETRY §6.3).
func reserveIDs(n int) uint32 {
	if n <= 0 {
		return 0
	}
	return uint32(C.manifold_reserve_ids(C.uint32_t(n)))
}

// newSolid uploads a triangle soup and returns a live Manifold handle.
//
// The Go slices are passed straight to C, so they must not move while the call
// runs. `runtime.KeepAlive` at the end is what guarantees that: the arguments
// are only reachable through unsafe.Pointer, which the collector does not
// follow.
func newSolid(g glMesh) (*solid, error) {
	if len(g.verts) == 0 || len(g.tris) == 0 {
		return nil, fmt.Errorf("a solid needs vertices and triangles")
	}
	if len(g.verts)%3 != 0 || len(g.tris)%3 != 0 {
		return nil, fmt.Errorf("vertex or triangle data is not a multiple of three")
	}
	if len(g.runs) != len(g.ids)+1 {
		return nil, fmt.Errorf("the run table has %d starts for %d ids",
			len(g.runs), len(g.ids))
	}

	// Everything handed over is copied into C memory first.
	//
	// cgo forbids passing a Go pointer that itself contains Go pointers, and the
	// options struct is exactly that: a struct of array pointers. Copying is not
	// a workaround for the rule, it is the rule's point — the collector is free
	// to move a Go slice while C is reading it. These are a few hundred kilobytes
	// at the very worst, next to a boolean that does real work.
	cVerts := (*C.double)(C.malloc(C.size_t(len(g.verts)) * C.sizeof_double))
	copy(unsafe.Slice((*float64)(unsafe.Pointer(cVerts)), len(g.verts)), g.verts)
	cTris := (*C.uint64_t)(C.malloc(C.size_t(len(g.tris)) * C.sizeof_uint64_t))
	copy(unsafe.Slice((*uint64)(unsafe.Pointer(cTris)), len(g.tris)), g.tris)
	// Manifold counts runs in triangle-vertex units; this package counts them in
	// triangles, because that is what the grouping downstream works in. The
	// conversion belongs here, at the boundary, and nowhere else.
	cRuns := (*C.uint64_t)(C.malloc(C.size_t(len(g.runs)) * C.sizeof_uint64_t))
	runsOut := unsafe.Slice((*uint64)(unsafe.Pointer(cRuns)), len(g.runs))
	for i, r := range g.runs {
		runsOut[i] = r * 3
	}
	cIDs := (*C.uint32_t)(C.malloc(C.size_t(len(g.ids)) * C.sizeof_uint32_t))
	copy(unsafe.Slice((*uint32)(unsafe.Pointer(cIDs)), len(g.ids)), g.ids)

	opts := (*C.ManifoldMeshGL64Options)(C.calloc(1, C.sizeof_ManifoldMeshGL64Options))
	opts.run_indices = cRuns
	opts.run_indices_length = C.size_t(len(g.runs))
	opts.run_original_ids = cIDs
	opts.run_original_ids_length = C.size_t(len(g.ids))

	glMem := C.csgAlloc(C.manifold_meshgl64_size())
	gl := C.manifold_meshgl64_w_options(glMem,
		cVerts, C.size_t(len(g.verts)/3), 3,
		cTris, C.size_t(len(g.tris)/3),
		opts)

	mem := C.csgAlloc(C.manifold_manifold_size())
	ptr := C.manifold_of_meshgl64(mem, gl)

	C.manifold_delete_meshgl64(gl)
	C.free(unsafe.Pointer(opts))
	C.free(unsafe.Pointer(cVerts))
	C.free(unsafe.Pointer(cTris))
	C.free(unsafe.Pointer(cRuns))
	C.free(unsafe.Pointer(cIDs))

	s := &solid{ptr: ptr}
	if err := errStatus(C.manifold_status(ptr)); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close frees the handle. Calling it twice is safe and does nothing.
func (s *solid) Close() {
	if s == nil || s.ptr == nil {
		return
	}
	C.manifold_delete_manifold(s.ptr)
	s.ptr = nil
}

// volume is Manifold's own measure of the solid.
func (s *solid) volume() float64 { return float64(C.manifold_volume(s.ptr)) }

// empty reports whether the solid has no volume at all, which several
// operations legally produce (SPEC-GEOMETRY §6.4).
func (s *solid) empty() bool { return C.manifold_is_empty(s.ptr) != 0 }

// apply runs one boolean and returns a new solid. The inputs are untouched.
func apply(op Op, a, b *solid) (*solid, error) {
	var cOp C.ManifoldOpType
	switch op {
	case Subtract:
		cOp = C.MANIFOLD_SUBTRACT
	case Intersect:
		cOp = C.MANIFOLD_INTERSECT
	default:
		cOp = C.MANIFOLD_ADD
	}
	mem := C.csgAlloc(C.manifold_manifold_size())
	ptr := C.manifold_boolean(mem, a.ptr, b.ptr, cOp)
	out := &solid{ptr: ptr}
	if err := errStatus(C.manifold_status(ptr)); err != nil {
		out.Close()
		return nil, err
	}
	return out, nil
}

// download reads the solid's triangles and its run table back out.
func (s *solid) download() glMesh {
	gl := C.manifold_get_meshgl64(C.csgAlloc(C.manifold_meshgl64_size()), s.ptr)
	defer C.manifold_delete_meshgl64(gl)

	nVerts := int(C.manifold_meshgl64_num_vert(gl))
	nTris := int(C.manifold_meshgl64_num_tri(gl))
	nProps := int(C.manifold_meshgl64_num_prop(gl))
	nRuns := int(C.manifold_meshgl64_num_run(gl))

	out := glMesh{
		verts: make([]float64, nVerts*3),
		tris:  make([]uint64, nTris*3),
		runs:  make([]uint64, nRuns+1),
		ids:   make([]uint32, nRuns),
	}

	// The accessors copy into memory we own, so each buffer is filled and then
	// read straight back into Go before anything else can touch it.
	if nVerts > 0 {
		buf := C.malloc(C.size_t(nVerts * nProps * C.sizeof_double))
		props := C.manifold_meshgl64_vert_properties(buf, gl)
		src := unsafe.Slice((*float64)(unsafe.Pointer(props)), nVerts*nProps)
		for i := 0; i < nVerts; i++ {
			// Position is always the first three properties (§6.3).
			copy(out.verts[i*3:i*3+3], src[i*nProps:i*nProps+3])
		}
		C.free(buf)
	}
	if nTris > 0 {
		buf := C.malloc(C.size_t(nTris * 3 * C.sizeof_uint64_t))
		tris := C.manifold_meshgl64_tri_verts(buf, gl)
		copy(out.tris, unsafe.Slice((*uint64)(unsafe.Pointer(tris)), nTris*3))
		C.free(buf)
	}
	if nRuns > 0 {
		idxLen := int(C.manifold_meshgl64_run_index_length(gl))
		buf := C.malloc(C.size_t(idxLen * C.sizeof_uint64_t))
		idx := C.manifold_meshgl64_run_index(buf, gl)
		src := unsafe.Slice((*uint64)(unsafe.Pointer(idx)), idxLen)
		// Manifold reports run starts in triangle-vertex units; the run table
		// this package keeps is in triangles, which is what the grouping needs.
		for i := 0; i < len(out.runs) && i < idxLen; i++ {
			out.runs[i] = src[i] / 3
		}
		C.free(buf)

		buf = C.malloc(C.size_t(nRuns * C.sizeof_uint32_t))
		ids := C.manifold_meshgl64_run_original_id(buf, gl)
		copy(out.ids, unsafe.Slice((*uint32)(unsafe.Pointer(ids)), nRuns))
		C.free(buf)
	}

	// The merge table says which duplicated vertices are really one point.
	// Deciding that by position instead would be a different question with a
	// different answer: two solids touching along an edge have coincident
	// vertices that Manifold has deliberately kept apart, and merging them
	// would turn a legal pair of shells into a non-manifold mess.
	if n := int(C.manifold_meshgl64_merge_length(gl)); n > 0 {
		out.mergeFrom = make([]uint64, n)
		out.mergeTo = make([]uint64, n)
		buf := C.malloc(C.size_t(n * C.sizeof_uint64_t))
		from := C.manifold_meshgl64_merge_from_vert(buf, gl)
		copy(out.mergeFrom, unsafe.Slice((*uint64)(unsafe.Pointer(from)), n))
		to := C.manifold_meshgl64_merge_to_vert(buf, gl)
		copy(out.mergeTo, unsafe.Slice((*uint64)(unsafe.Pointer(to)), n))
		C.free(buf)
	}
	return out
}
