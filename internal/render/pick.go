package render

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"modeler/internal/geom"
	"modeler/internal/geom/mesh"
)

// ID-based picking (SPEC-RENDER §6). Every pickable thing drawn in a pick pass
// appends an entry to a table; its table index is rendered as a flat RGBA color
// into a small render texture around the cursor, and the readback is resolved
// center-out with the forgiveness order verts > edges > faces.

const (
	// PickRTSize is the pick render texture's edge length in texels.
	PickRTSize = 64
	// PickRegionPx is the screen-space neighbourhood mapped onto it: the
	// cursor plus or minus 12 pixels (SPEC-RENDER §6.1).
	PickRegionPx = 24

	// Resolve radii in screen pixels (SPEC-UX §12.1).
	PickVertRadius  = 4.5
	PickEdgeRadius  = 2.5
	PickFaceRadius  = 1.5
	PickGizmoRadius = 12.0
)

// PickKind is what a pick-table entry refers to.
type PickKind uint8

const (
	PickNone PickKind = iota
	PickFace
	PickEdge
	PickVert
	PickRegion
	PickSketchEntity
	PickPlane
	PickGizmo
)

func (k PickKind) String() string {
	switch k {
	case PickFace:
		return "face"
	case PickEdge:
		return "edge"
	case PickVert:
		return "vert"
	case PickRegion:
		return "region"
	case PickSketchEntity:
		return "sketchEntity"
	case PickPlane:
		return "plane"
	case PickGizmo:
		return "gizmo"
	default:
		return "none"
	}
}

// PickRef identifies one pickable element. A table, rather than bit packing,
// keeps every field addressable without cramming them into 24 bits.
type PickRef struct {
	Kind      PickKind
	BodyID    uint32
	FaceUID   mesh.FaceUID
	Edge      int
	Vert      int
	Plane     geom.PlaneKind
	EntityID  uint32
	RegionIdx int
	GizmoPart int
}

// PickTable maps rendered IDs to references. Index 0 is reserved for "nothing",
// so a cleared black pick buffer reads as empty.
type PickTable struct {
	refs []PickRef
}

// Reset empties the table for a new pass.
func (t *PickTable) Reset() { t.refs = t.refs[:0] }

// Add appends a reference and returns its rendered ID (always at least 1).
func (t *PickTable) Add(r PickRef) int {
	t.refs = append(t.refs, r)
	return len(t.refs)
}

// Next returns the ID the next Add would produce, which is what a batch of
// elements uses as its idBase.
func (t *PickTable) Next() int { return len(t.refs) + 1 }

// Get resolves an ID back to its reference.
func (t *PickTable) Get(id int) (PickRef, bool) {
	if id <= 0 || id > len(t.refs) {
		return PickRef{}, false
	}
	return t.refs[id-1], true
}

// Len reports how many pickables the last pass registered.
func (t *PickTable) Len() int { return len(t.refs) }

// PickResult is what the cursor is over.
type PickResult struct {
	PickRef
	// DistancePx is how far the winning pixel was from the cursor.
	DistancePx float64
	Hit        bool
}

// pickMatrix is the classic gluPickMatrix: an NDC translate and scale that
// blows a small neighbourhood of the cursor up to the whole render target.
//
// cursor is in viewport-local pixels with the origin at the top-left.
func pickMatrix(cursor geom.Vec2, vp Viewport, regionPx float64) geom.Mat4 {
	w, h := float64(vp.W), float64(vp.H)
	// GL window coordinates put the origin at the bottom-left.
	x := cursor.X
	y := h - cursor.Y
	tx := (w - 2*x) / regionPx
	ty := (h - 2*y) / regionPx
	return geom.Translate(geom.Vec3{X: tx, Y: ty}).
		Mul(geom.Scale(geom.Vec3{X: w / regionPx, Y: h / regionPx, Z: 1}))
}

// Pick renders the ID pass around the cursor and resolves what is under it.
// cursor is in window pixels; a cursor outside the viewport returns no hit.
func (r *Renderer) Pick(s *Scene, vp Viewport, windowX, windowY float64) PickResult {
	if vp.W <= 0 || vp.H <= 0 || !vp.Contains(int(windowX), int(windowY)) {
		return PickResult{}
	}
	local := vp.Local(windowX, windowY)
	r.Table.Reset()

	proj := pickMatrix(local, vp, PickRegionPx).Mul(s.Camera.Proj(vp.Aspect()))

	rl.BeginTextureMode(r.pickRT)
	rl.ClearBackground(color.RGBA{A: 255})
	rl.DrawRenderBatchActive()

	rl.Viewport(0, 0, PickRTSize, PickRTSize)
	savedProj := rl.GetMatrixProjection()
	savedView := rl.GetMatrixModelview()
	rl.SetMatrixProjection(toRLMatrix(proj))
	rl.SetMatrixModelview(toRLMatrix(s.Camera.View()))

	rl.EnableDepthTest()
	rl.DisableColorBlend()
	rl.SetTexture(r.whiteTex.ID)

	r.pickBodies(s)
	r.pickPlanes(s)
	r.pickEdgesAndVerts(s, vp)

	rl.SetTexture(0)
	rl.DrawRenderBatchActive()
	rl.EnableColorBlend()
	rl.DisableDepthTest()
	rl.SetMatrixProjection(savedProj)
	rl.SetMatrixModelview(savedView)
	rl.EndTextureMode()
	rl.Viewport(0, 0, int32(r.fbW), int32(r.fbH))

	return r.resolvePick()
}

// pickBodies draws every pickable body with the ID shader. Each body is one
// draw call; the per-vertex face index picks the entry within its ID range.
func (r *Renderer) pickBodies(s *Scene) {
	rl.EnableBackfaceCulling()
	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || !b.GPU.uploaded || !b.Pickable {
			continue
		}
		base := r.Table.Next()
		for _, uid := range b.GPU.FaceUIDs {
			r.Table.Add(PickRef{Kind: PickFace, BodyID: b.BodyID, FaceUID: uid})
		}
		rl.SetShaderValue(r.pick, r.locPickIDBase, []float32{float32(base)}, rl.ShaderUniformFloat)
		rl.DrawMesh(*b.GPU.rlMesh, r.pickMat, toRLMatrix(b.Transform))
	}
}

// pickPlanes draws the default planes into the ID buffer so they can be
// clicked in the viewport (SPEC-UX §5).
func (r *Renderer) pickPlanes(s *Scene) {
	rl.DisableBackfaceCulling()
	for i := range s.Planes {
		p := &s.Planes[i]
		if !p.Pickable {
			continue
		}
		id := r.Table.Add(PickRef{Kind: PickPlane, Plane: p.Kind})
		quad := planeQuad(p.Frame, p.HalfSize)
		drawPolyFan(quad[:], encodeID(id))
	}
	rl.DrawRenderBatchActive()
	rl.EnableBackfaceCulling()
}

// pickEdgesAndVerts draws wider pick ribbons and vertex billboards on top of
// the faces, biased toward the eye so they win the depth test where they
// overlap the surfaces they belong to (SPEC-RENDER §6.1).
func (r *Renderer) pickEdgesAndVerts(s *Scene, vp Viewport) {
	if s.PickFacesOnly || s.PickOnly == PickFace {
		return
	}
	c := r.ribbonCtx(s.Camera, vp)
	rl.DisableBackfaceCulling()

	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || !b.Pickable {
			continue
		}
		for _, e := range b.GPU.Edges {
			if s.PickOnly == PickVert {
				continue
			}
			id := r.Table.Add(PickRef{Kind: PickEdge, BodyID: b.BodyID, Edge: e.Index})
			a := b.Transform.TransformPoint(e.A)
			z := b.Transform.TransformPoint(e.B)
			r.drawRibbon(c, a, z, EdgePickWidth, encodeID(id), EyeShrinkEdgeFactor)
		}
	}
	rl.DrawRenderBatchActive()

	for i := range s.Bodies {
		b := &s.Bodies[i]
		if b.GPU == nil || !b.Pickable {
			continue
		}
		for vi, p := range b.GPU.Verts {
			if s.PickOnly == PickEdge {
				continue
			}
			id := r.Table.Add(PickRef{Kind: PickVert, BodyID: b.BodyID, Vert: vi})
			r.drawBillboardQuad(c, b.Transform.TransformPoint(p),
				VertPickScreenSize, encodeID(id), EyeShrinkVertFactor)
		}
	}
	rl.DrawRenderBatchActive()
	rl.EnableBackfaceCulling()
}

// resolvePick reads the ID buffer back and applies the priority rules.
func (r *Renderer) resolvePick() PickResult {
	img := rl.LoadImageFromTexture(r.pickRT.Texture)
	if img == nil {
		return PickResult{}
	}
	defer rl.UnloadImage(img)
	pixels := rl.LoadImageColors(img)
	if len(pixels) < PickRTSize*PickRTSize {
		return PickResult{}
	}

	// One render-target texel spans this many screen pixels.
	const texelToPx = float64(PickRegionPx) / float64(PickRTSize)
	const center = (PickRTSize - 1) / 2.0

	best := map[PickKind]PickResult{}
	for y := 0; y < PickRTSize; y++ {
		for x := 0; x < PickRTSize; x++ {
			c := pixels[y*PickRTSize+x]
			id := decodeID(c)
			if id == 0 {
				continue
			}
			ref, ok := r.Table.Get(id)
			if !ok {
				continue
			}
			dx := (float64(x) - center) * texelToPx
			dy := (float64(y) - center) * texelToPx
			d := math.Hypot(dx, dy)
			if cur, ok := best[ref.Kind]; !ok || d < cur.DistancePx {
				best[ref.Kind] = PickResult{PickRef: ref, DistancePx: d, Hit: true}
			}
		}
	}

	// Priority order: gizmos, then vertices, edges and finally surfaces
	// (SPEC-UX §12.1). Each has its own forgiveness radius.
	order := []struct {
		kind   PickKind
		radius float64
	}{
		{PickGizmo, PickGizmoRadius},
		{PickVert, PickVertRadius},
		{PickEdge, PickEdgeRadius},
		{PickFace, PickFaceRadius},
		{PickRegion, PickFaceRadius},
		{PickSketchEntity, PickEdgeRadius},
		{PickPlane, PickFaceRadius},
	}
	for _, o := range order {
		if res, ok := best[o.kind]; ok && res.DistancePx <= o.radius {
			return res
		}
	}
	return PickResult{}
}
