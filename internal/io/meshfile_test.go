package io

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Reading STL and OBJ (the user's request, 2026-08-28). Both are trivial
// formats with one trap each: STL comes in two encodings that must be told
// apart without trusting the header, and OBJ indexes from 1 and allows
// negative indices that count back from the end.

func write(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const asciiSTL = `solid tri
facet normal 0 0 1
  outer loop
    vertex 0 0 0
    vertex 1 0 0
    vertex 0 1 0
  endloop
endfacet
facet normal 0 0 1
  outer loop
    vertex 1 0 0
    vertex 1 1 0
    vertex 0 1 0
  endloop
endfacet
endsolid tri
`

func TestReadsAsciiSTL(t *testing.T) {
	tris, err := ReadMeshFile(write(t, "a.stl", []byte(asciiSTL)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(tris) != 2 {
		t.Fatalf("read %d triangles, want 2", len(tris))
	}
	if tris[0].B.X != 1 || tris[0].C.Y != 1 {
		t.Errorf("first triangle came back as %+v", tris[0])
	}
}

// binarySTL writes the 84-byte header plus 50 bytes a triangle.
func binarySTL(n int) []byte {
	var b bytes.Buffer
	// A binary file is allowed to begin with the word "solid", which is why
	// the reader must not use that to decide.
	head := make([]byte, 80)
	copy(head, "solid not-really-ascii")
	b.Write(head)
	binary.Write(&b, binary.LittleEndian, uint32(n))
	for i := 0; i < n; i++ {
		for _, f := range []float32{0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0} {
			binary.Write(&b, binary.LittleEndian, f)
		}
		binary.Write(&b, binary.LittleEndian, uint16(0))
	}
	return b.Bytes()
}

func TestReadsBinarySTLEvenWhenItSaysSolid(t *testing.T) {
	tris, err := ReadMeshFile(write(t, "b.stl", binarySTL(3)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(tris) != 3 {
		t.Fatalf("read %d triangles, want 3 — the reader trusted the word "+
			"\"solid\" instead of the file's size", len(tris))
	}
	if tris[0].B.X != 1 {
		t.Errorf("triangle came back as %+v", tris[0])
	}
}

func TestATruncatedSTLIsRefusedNotGuessedAt(t *testing.T) {
	data := binarySTL(3)[:100]
	if _, err := ReadMeshFile(write(t, "short.stl", data)); err == nil {
		t.Error("a truncated binary STL was read without complaint")
	}
}

const objQuad = `# a quad and a triangle
v 0 0 0
v 2 0 0
v 2 2 0
v 0 2 0
vn 0 0 1
vt 0 0
f 1/1/1 2/1/1 3/1/1 4/1/1
f -4 -3 -2
`

func TestReadsOBJIncludingPolygonsAndNegativeIndices(t *testing.T) {
	tris, err := ReadMeshFile(write(t, "q.obj", []byte(objQuad)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// The quad fans into two triangles, and the negative-index face is one
	// more — pointing at the same first three vertices.
	if len(tris) != 3 {
		t.Fatalf("read %d triangles, want 3", len(tris))
	}
	if tris[2].A.X != 0 || tris[2].B.X != 2 || tris[2].C.Y != 2 {
		t.Errorf("negative indices resolved to %+v", tris[2])
	}
	// The fan must keep the polygon's winding, or the face comes out inside-out.
	n := tris[0].B.Sub(tris[0].A).Cross(tris[0].C.Sub(tris[0].A))
	if n.Z <= 0 {
		t.Errorf("the quad fanned into a triangle wound the wrong way: %v", n)
	}
}

func TestAnUnknownExtensionSaysSo(t *testing.T) {
	_, err := ReadMeshFile(write(t, "thing.step", []byte("ISO-10303-21;")))
	if err == nil {
		t.Fatal("a .step file was accepted by the mesh reader")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("STL")) {
		t.Errorf("the refusal does not say what it does read: %v", err)
	}
}

func TestOBJIgnoresWhatItDoesNotUnderstand(t *testing.T) {
	src := "mtllib thing.mtl\nusemtl red\no group\ns off\n" + objQuad
	tris, err := ReadMeshFile(write(t, "m.obj", []byte(src)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(tris) != 3 {
		t.Errorf("material and group lines cost %d triangles", 3-len(tris))
	}
}

func TestNaNInAnSTLIsRejected(t *testing.T) {
	var b bytes.Buffer
	b.Write(make([]byte, 80))
	binary.Write(&b, binary.LittleEndian, uint32(1))
	for i := 0; i < 12; i++ {
		f := float32(0)
		if i == 3 {
			f = float32(math.NaN())
		}
		binary.Write(&b, binary.LittleEndian, f)
	}
	binary.Write(&b, binary.LittleEndian, uint16(0))
	if _, err := ReadMeshFile(write(t, "nan.stl", b.Bytes())); err == nil {
		t.Error("a triangle with a NaN corner was accepted")
	}
}
