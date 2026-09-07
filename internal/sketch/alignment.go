package sketch

import (
	"math"
	"modeler/internal/geom"
	"modeler/internal/model"
)

// Track actual endpoints/corners rather than every tessellation point of a
// curve. A remote endpoint can align one coordinate without pulling the
// pointer all the way to that endpoint.
func alignmentPoints(ents []model.Entity) []geom.Vec2i {
	var points []geom.Vec2i
	for _, e := range ents {
		switch e.Kind {
		case model.EntLine, model.EntPoint, model.EntRect, model.EntPolygon:
			points = append(points, e.Points()...)
		case model.EntArc, model.EntSpline, model.EntBezier:
			if e.Closed() {
				continue
			}
			p := e.Points()
			if len(p) > 0 {
				points = append(points, p[0], p[len(p)-1])
			}
		}
	}
	return points
}

func alignEndpoints(s Snap, raw geom.Vec2i, ents []model.Entity, from *geom.Vec2i, cfg Config) Snap {
	// Only a point-placement gesture acquires tracking lines. Hovering with
	// the selection tool must not shift its region/geometry hit test.
	if from == nil || cfg.SubunitsPerPixel <= 0 {
		return s
	}
	points := append(alignmentPoints(ents), referenceCorners(cfg.Reference)...)
	radius := AlignmentRadiusPx * cfg.SubunitsPerPixel
	for axis := 0; axis < 2; axis++ {
		if (axis == 0 && s.Infer == InferVertical) || (axis == 1 && s.Infer == InferHorizontal) {
			continue
		}
		found := false
		var best geom.Vec2i
		bestOffset, bestDistance := math.Inf(1), math.Inf(1)
		for _, p := range points {
			if p == *from {
				continue
			}
			d := p.Sub(raw)
			offset := math.Abs(float64(d.X))
			if axis == 1 {
				offset = math.Abs(float64(d.Y))
			}
			if offset > radius {
				continue
			}
			distance := math.Hypot(float64(d.X), float64(d.Y))
			if !found || offset < bestOffset || (offset == bestOffset && (distance < bestDistance || (distance == bestDistance && lexLess(p, best)))) {
				best, bestOffset, bestDistance, found = p, offset, distance, true
			}
		}
		if !found {
			continue
		}
		guide := AlignmentGuide{From: best, Axis: InferVertical}
		if axis == 0 {
			s.Point.X = best.X
		} else {
			s.Point.Y = best.Y
			guide.Axis = InferHorizontal
		}
		s.Guides[axis] = guide
		s.Kind = SnapAlignment
	}
	return s
}
