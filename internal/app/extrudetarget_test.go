package app

import (
	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/tools"
	"testing"
)

func TestExtrudeTargetInvalidatedByHistoryChange(t *testing.T) {
	a := bareApp()
	a.Mode = ModeExtrude
	a.extrude.tool = tools.NewExtrudeTool(1, []int{0}, geom.Vec3{}, geom.Vec3{Z: 1})
	a.extrude.tool.SetExtent(tools.ExtentVertex)
	if err := a.extrude.tool.SetTarget(geom.Vec3{Z: 4}, geom.Vec3{}); err != nil {
		t.Fatal(err)
	}
	a.extrude.targetRef = render.PickRef{Kind: render.PickVert, BodyID: 7, Vert: 9}
	a.onDocumentEvent(model.Event{Kind: model.EvBodyChanged, BodyID: 8})
	if !a.extrude.tool.TargetReady {
		t.Fatal("unrelated body invalidated target")
	}
	a.onDocumentEvent(model.Event{Kind: model.EvBodyChanged, BodyID: 7})
	if a.extrude.tool.TargetReady || a.extrude.targetRef.BodyID != 0 || a.extrude.previewErr == "" {
		t.Fatal("history change left a stale target")
	}
	if ok, _ := a.extrude.tool.Valid(); ok {
		t.Fatal("stale target can commit")
	}
}
