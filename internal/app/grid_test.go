package app

import (
	"testing"

	"modeler/internal/geom"
	"modeler/internal/model"
	"modeler/internal/render"
	"modeler/internal/sketch"
)

// The grid step drives both the drawn grid and the snap, from one setting.
// These pin the snap half; the drawn half is pinned by TestGoldenGridStep.

func TestSnapFollowsTheGridStepSetting(t *testing.T) {
	a := bareApp()
	vp := render.Viewport{W: 1280, H: 720}
	in := NewInputFrame()

	a.Settings.GridStep = 0.5
	if got := a.snapConfig(in, vp).GridStep; got != geom.ToSubunits(0.5) {
		t.Errorf("grid 0.5 u snapped at %d subunits, want %d", got, geom.ToSubunits(0.5))
	}

	// A broken setting falls back to one unit rather than snapping to nothing.
	a.Settings.GridStep = 0
	if got := a.snapConfig(in, vp).GridStep; got != geom.SubunitsPerUnit {
		t.Errorf("grid 0 snapped at %d subunits, want the 1 u fallback %d", got, geom.SubunitsPerUnit)
	}

	// Ctrl is the fine override whatever the chips say (SPEC-UX §16).
	a.Settings.GridStep = 2
	in.Ctrl = true
	if got := a.snapConfig(in, vp).GridStep; got != geom.SubunitsFine {
		t.Errorf("Ctrl snapped at %d subunits, want the fine step %d", got, geom.SubunitsFine)
	}
}

func TestGridStaysAlignedBetweenDrawings(t *testing.T) {
	for _, plane := range []geom.PlaneKind{geom.PlaneTop, geom.PlaneFront, geom.PlaneRight} {
		t.Run(plane.String(), func(t *testing.T) {
			a := bareApp()
			base := geom.PlaneFrame(plane)
			// These are existing face-centered frames, including a distant face
			// and the opposite side of a solid. No saved geometry may move.
			frames := []geom.Frame{
				base,
				geom.FrameFromNormal(base.ToWorld(geom.Vec2{X: 2.5, Y: -1.25}), base.N),
				geom.FrameFromNormal(base.ToWorld(geom.Vec2{X: -35.25, Y: 19.5}), base.N.Neg()),
			}
			vp := render.Viewport{W: 1280, H: 720}
			for _, frame := range frames {
				s := &model.Sketch{ID: uint32(len(a.Doc().Sketches) + 1), Visible: true,
					OnFace: true, FrameSnap: frame,
					Entities: []model.Entity{model.NewLine(geom.Vec2i{}, geom.Vec2i{X: 256})}}
				a.Doc().Sketches = append(a.Doc().Sketches, s)
				for _, step := range SketchGridSteps {
					for _, fine := range []bool{false, true} {
						a.EditSketch(s)
						a.Settings.GridStep = step
						a.sketch.fineGrid = fine
						in := NewInputFrame()
						in.Ctrl = fine
						cfg := a.snapConfig(in, vp)
						g := a.BuildScene().Grid
						if g == nil || g.Frame != frame || g.Origin != cfg.GridOrigin.Units() || geom.ToSubunits(g.MinorStep) != cfg.GridStep {
							t.Fatal("visible grid and snapping use different coordinates or spacing")
						}
						world := base.ToWorld(geom.Vec2{X: 4.0625, Y: -2.0625})
						raw := geom.Vec2iFromUnits(frame.ToLocal(world))
						point := sketch.Resolve(raw, nil, nil, cfg).Point
						got := frame.LiftSub(point, 0)
						want := base.ToWorld(geom.Vec2{X: 4, Y: -2})
						if got.Sub(want).Len() > 1e-9 {
							t.Fatalf("step %v fine %v: snapped world point %v, want %v", step, fine, got, want)
						}
						a.ExitSketch(false)
						if s.Frame() != frame || len(s.Entities) != 1 || s.Entities[0].Points()[0] != (geom.Vec2i{}) {
							t.Fatal("switching drawings changed existing geometry")
						}
					}
				}
			}
		})
	}
}
