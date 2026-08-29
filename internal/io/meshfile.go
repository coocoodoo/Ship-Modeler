package io

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// Reading mesh files (the user's request, 2026-08-28).
//
// STL and OBJ, which between them are what every other tool can write. Both
// are read into plain triangle soup and handed to mesh.Assemble, which does
// the work that makes the result usable here — welding, orienting, and merging
// coplanar triangles back into polygons.
//
// Neither format carries units, and neither carries anything this program
// wants beyond geometry: OBJ materials, normals and texture coordinates are
// read past rather than stored, because a face here gets its normal from its
// own plane and its colour from the body.

// MeshExtensions are the mesh files the importer opens.
var MeshExtensions = []string{".stl", ".obj"}

// ReadMeshFile reads a mesh file into triangles, choosing the reader by
// extension.
func ReadMeshFile(path string) ([]mesh.Tri3, error) {
	switch strings.ToLower(ext(path)) {
	case ".stl":
		return readSTL(path)
	case ".obj":
		return readOBJ(path)
	default:
		return nil, fmt.Errorf("%s is not a mesh file this reads — STL and OBJ are",
			ext(path))
	}
}

func ext(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i:]
	}
	return ""
}

// stlHeader is the fixed part of a binary STL: 80 bytes of anything, then the
// triangle count.
const stlHeader = 84

// stlTriangle is a normal and three corners as float32, plus the attribute
// short nobody uses.
const stlTriangle = 50

// readSTL reads either encoding.
//
// Which one is decided by arithmetic, not by the header: a binary STL may
// legally begin with the word "solid" — plenty do, because the writer put its
// own name there — so sniffing that text is how importers end up reading
// binary files as ASCII and finding nothing.
func readSTL(path string) ([]mesh.Tri3, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() >= stlHeader {
		var count uint32
		if _, err := f.Seek(80, 0); err != nil {
			return nil, err
		}
		if err := binary.Read(f, binary.LittleEndian, &count); err != nil {
			return nil, fmt.Errorf("reading the triangle count: %w", err)
		}
		if want := int64(stlHeader) + int64(count)*stlTriangle; want == info.Size() {
			return readBinarySTL(f, int(count))
		} else if count > 0 && info.Size() > stlHeader && !looksASCII(f) {
			return nil, fmt.Errorf("this looks like a binary STL of %d triangles, "+
				"which needs %d bytes, but the file is %d — it is truncated",
				count, want, info.Size())
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	return readASCIISTL(f)
}

// looksASCII reports whether the file's opening reads as text, which is the
// tie-breaker when the size arithmetic did not settle it.
func looksASCII(f *os.File) bool {
	if _, err := f.Seek(0, 0); err != nil {
		return false
	}
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	for _, b := range buf[:n] {
		if b == 0 {
			return false
		}
	}
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(string(buf[:n]))), "solid")
}

func readBinarySTL(f *os.File, count int) ([]mesh.Tri3, error) {
	if _, err := f.Seek(stlHeader, 0); err != nil {
		return nil, err
	}
	r := bufio.NewReaderSize(f, 1<<16)
	buf := make([]byte, stlTriangle)
	out := make([]mesh.Tri3, 0, count)
	for i := 0; i < count; i++ {
		if _, err := readFull(r, buf); err != nil {
			return nil, fmt.Errorf("triangle %d of %d: %w", i+1, count, err)
		}
		// Bytes 0..11 are the facet normal, which is ignored: it is advisory,
		// frequently wrong, and the winding decides anyway (mesh.Assemble
		// re-derives it).
		var t mesh.Tri3
		for k, p := range []*geom.Vec3{&t.A, &t.B, &t.C} {
			o := 12 + k*12
			p.X = float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[o:])))
			p.Y = float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[o+4:])))
			p.Z = float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[o+8:])))
		}
		if err := finite(t, i+1); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		k, err := r.Read(buf[n:])
		n += k
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func readASCIISTL(f *os.File) ([]mesh.Tri3, error) {
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 1<<16), 1<<20)
	var (
		out  []mesh.Tri3
		pts  []geom.Vec3
		line int
	)
	for s.Scan() {
		line++
		fields := strings.Fields(s.Text())
		if len(fields) == 0 || !strings.EqualFold(fields[0], "vertex") {
			continue
		}
		if len(fields) < 4 {
			return nil, fmt.Errorf("line %d: a vertex needs three numbers", line)
		}
		p, err := vec3Of(fields[1:4], line)
		if err != nil {
			return nil, err
		}
		pts = append(pts, p)
		if len(pts) == 3 {
			t := mesh.Tri3{A: pts[0], B: pts[1], C: pts[2]}
			if err := finite(t, len(out)+1); err != nil {
				return nil, err
			}
			out = append(out, t)
			pts = pts[:0]
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no triangles found — this may not be an STL file")
	}
	return out, nil
}

// readOBJ reads vertices and faces and ignores the rest.
//
// Faces may be polygons, and are fanned from their first corner. That fan is
// safe here for the reason it usually is not: mesh.Assemble merges coplanar
// triangles straight back into the polygon, so a flat face makes the round
// trip exactly. A non-planar OBJ face is the one case where the fan is a
// choice rather than a formality, and any triangulation of one is a choice.
func readOBJ(path string) ([]mesh.Tri3, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 1<<16), 1<<20)
	var (
		verts []geom.Vec3
		out   []mesh.Tri3
		line  int
	)
	for s.Scan() {
		line++
		text := s.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "v":
			if len(fields) < 4 {
				return nil, fmt.Errorf("line %d: a vertex needs three numbers", line)
			}
			p, err := vec3Of(fields[1:4], line)
			if err != nil {
				return nil, err
			}
			verts = append(verts, p)
		case "f":
			idx := make([]int, 0, len(fields)-1)
			for _, spec := range fields[1:] {
				// "v", "v/vt", "v//vn" and "v/vt/vn" all start with the one
				// number this cares about.
				if i := strings.IndexByte(spec, '/'); i >= 0 {
					spec = spec[:i]
				}
				n, err := strconv.Atoi(spec)
				if err != nil {
					return nil, fmt.Errorf("line %d: %q is not a vertex index", line, spec)
				}
				if n < 0 {
					n = len(verts) + 1 + n // negative counts back from the end
				}
				if n < 1 || n > len(verts) {
					return nil, fmt.Errorf("line %d: vertex %d of %d", line, n, len(verts))
				}
				idx = append(idx, n-1)
			}
			for k := 2; k < len(idx); k++ {
				out = append(out, mesh.Tri3{
					A: verts[idx[0]], B: verts[idx[k-1]], C: verts[idx[k]],
				})
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no faces found — this may not be an OBJ file")
	}
	return out, nil
}

func vec3Of(fields []string, line int) (geom.Vec3, error) {
	var out [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return geom.Vec3{}, fmt.Errorf("line %d: %q is not a number", line, fields[i])
		}
		out[i] = v
	}
	return geom.Vec3{X: out[0], Y: out[1], Z: out[2]}, nil
}

// finite refuses NaN and infinity at the door. One of them anywhere in the
// mesh would poison every bounding box, snap and boolean downstream, and the
// place to catch that is the file it came from.
func finite(t mesh.Tri3, n int) error {
	for _, p := range []geom.Vec3{t.A, t.B, t.C} {
		for _, v := range []float64{p.X, p.Y, p.Z} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("triangle %d has a corner that is not a number", n)
			}
		}
	}
	return nil
}
