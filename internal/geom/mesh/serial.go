package mesh

import (
	"encoding/json"
	"fmt"
	"image"

	"modeler/internal/geom"
)

// How a mesh is written into a .ship document (SPEC-DATA §4).
//
// The mesh marshals itself rather than being mirrored by a struct in package
// io, because of one invariant only it knows about: fragments of a cut face
// share a single FacePaint by pointer (SPEC-GEOMETRY §8.4). Marshalling face by
// face would write that picture once per fragment and read back a copy each,
// and painting one fragment would stop showing on its siblings — a contract
// broken by having been saved. So the pictures go in a table and the faces hold
// an index into it.
//
// The pixels themselves are not here. They travel as PNGs beside the document
// (`paint/<owner face uid>.png`); a texture spelled out as a JSON array of
// bytes would dwarf the geometry it belongs to.

// PaintEntry is one distinct picture in a mesh, and the face that introduced it.
//
// The owner is what names the file beside the document. It is a face identity
// rather than a running number so the name means something, and it is stable:
// identities are never reused (SPEC-DATA §1).
type PaintEntry struct {
	Owner FaceUID
	Paint *FacePaint
}

// PaintTable lists the mesh's distinct pictures in the order the faces first
// reference them, which is the order they are written in.
func (m *Mesh) PaintTable() []PaintEntry {
	var out []PaintEntry
	seen := map[*FacePaint]bool{}
	for i := range m.Faces {
		p := m.Faces[i].Paint
		if p == nil || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, PaintEntry{Owner: m.Faces[i].ID, Paint: p})
	}
	return out
}

// faceJSON is one face on disk. Paint is a 1-based index into the mesh's paint
// table, or zero for an unpainted face.
type faceJSON struct {
	ID        FaceUID `json:"id"`
	Loops     [][]int `json:"loops"`
	SrcFace   FaceUID `json:"src,omitempty"`
	NonPlanar bool    `json:"nonPlanar,omitempty"`
	KeepEdges bool    `json:"keepEdges,omitempty"`
	Paint     int     `json:"paint,omitempty"`
}

// paintJSON is one picture's mapping. The image is absent by design.
type paintJSON struct {
	Owner FaceUID     `json:"owner"`
	Res   int         `json:"res"`
	Texel float64     `json:"texel"`
	Frame geom.Frame  `json:"frame"`
	Off   image.Point `json:"off"`
}

type meshJSON struct {
	Verts  []geom.Vec3 `json:"verts"`
	Faces  []faceJSON  `json:"faces"`
	Paints []paintJSON `json:"paints,omitempty"`
}

// MarshalJSON writes the mesh, its faces and its paint table.
func (m *Mesh) MarshalJSON() ([]byte, error) {
	table := m.PaintTable()
	index := make(map[*FacePaint]int, len(table))
	out := meshJSON{
		Verts: m.Verts,
		Faces: make([]faceJSON, len(m.Faces)),
	}
	for i, e := range table {
		index[e.Paint] = i + 1
		out.Paints = append(out.Paints, paintJSON{
			Owner: e.Owner,
			Res:   e.Paint.Res,
			Texel: e.Paint.Texel,
			Frame: e.Paint.Frame,
			Off:   e.Paint.Off,
		})
	}
	for i := range m.Faces {
		f := &m.Faces[i]
		out.Faces[i] = faceJSON{
			ID:        f.ID,
			Loops:     f.Loops,
			SrcFace:   f.SrcFace,
			NonPlanar: f.NonPlanar,
			KeepEdges: f.KeepEdges,
			Paint:     index[f.Paint],
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads a mesh back, rebuilding the sharing between faces that
// reference one picture.
func (m *Mesh) UnmarshalJSON(data []byte) error {
	var in meshJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("read a mesh: %w", err)
	}
	paints := make([]*FacePaint, len(in.Paints))
	for i, p := range in.Paints {
		paints[i] = &FacePaint{Res: p.Res, Texel: p.Texel, Frame: p.Frame, Off: p.Off}
	}

	out := Mesh{
		Verts: in.Verts,
		Faces: make([]Face, len(in.Faces)),
	}
	for i, f := range in.Faces {
		if f.Paint < 0 || f.Paint > len(paints) {
			return fmt.Errorf("face %d refers to picture %d, and there are %d",
				f.ID, f.Paint, len(paints))
		}
		out.Faces[i] = Face{
			ID:        f.ID,
			Loops:     f.Loops,
			SrcFace:   f.SrcFace,
			NonPlanar: f.NonPlanar,
			// Older saved chamfers used construction body zero for their
			// tool faces. Promote that durable lineage to the explicit flag.
			KeepEdges: f.KeepEdges || (f.SrcFace != NoFace && f.SrcFace.BodyID() == 0),
		}
		if f.Paint > 0 {
			out.Faces[i].Paint = paints[f.Paint-1]
		}
	}
	*m = out
	return nil
}

// PaintOwners returns the owning face identity of each picture, positionally
// matching PaintTable. Package io names the PNGs from it.
func (m *Mesh) PaintOwners() []FaceUID {
	table := m.PaintTable()
	out := make([]FaceUID, len(table))
	for i, e := range table {
		out[i] = e.Owner
	}
	return out
}
