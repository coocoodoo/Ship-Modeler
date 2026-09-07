// Package io owns file formats: the .ship project container, exports, settings
// and the op-script format that drives headless runs and end-to-end tests.
// It is raylib-free so its tests need no window.
package io

import (
	"encoding/json"
	"fmt"
	"os"
)

// Op is one step of an op script (SPEC-DATA §7). The struct is a flat union of
// every op's fields; Parse validates that the ones an op needs are present.
//
// Ops are executed through the real command bus and camera controller, so a
// script and a user session go down identical code paths.
type Op struct {
	Op string `json:"op"`

	// Sketch ops.
	Plane string      `json:"plane,omitempty"`
	From  *[2]float64 `json:"from,omitempty"`
	To    *[2]float64 `json:"to,omitempty"`
	A     *[2]float64 `json:"a,omitempty"`
	B     *[2]float64 `json:"b,omitempty"`
	C     *[2]float64 `json:"c,omitempty"`
	R     float64     `json:"r,omitempty"`
	Segs  int         `json:"segs,omitempty"`
	Step  float64     `json:"step,omitempty"`
	// Indices name entities within a sketch, for the ops that convert or
	// modify what is already drawn.
	Indices []int `json:"indices,omitempty"`
	// Pts is a run of sketch positions, for the curves built from a list.
	Pts [][2]float64 `json:"pts,omitempty"`
	// Closed joins a curve's last point back to its first.
	Closed bool `json:"closed,omitempty"`
	// On is a generic flag for ops that turn something on or off.
	On *bool `json:"on,omitempty"`

	// Extrude and boolean ops.
	Sketch  string   `json:"sketch,omitempty"`
	Regions []int    `json:"regions,omitempty"`
	Depth   float64  `json:"depth,omitempty"`
	Draft   float64  `json:"draft,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Through bool     `json:"through,omitempty"`
	Result  string   `json:"result,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Target  string   `json:"target,omitempty"`
	Tools   []string `json:"tools,omitempty"`

	// Selection and transform ops.
	Body  string      `json:"body,omitempty"`
	Face  int         `json:"face,omitempty"`
	Vert  int         `json:"vert,omitempty"`
	Edge  int         `json:"edge,omitempty"`
	Delta *[3]float64 `json:"delta,omitempty"`
	// Marker ops: where the dot goes and the face normal it carries. (At is
	// taken by the pick op, and a marker IS a dot.)
	Dot     *[3]float64 `json:"dot,omitempty"`
	Normal  *[3]float64 `json:"normal,omitempty"`
	Axis    string      `json:"axis,omitempty"`
	Degrees float64     `json:"degrees,omitempty"`
	// Rect is a box-select rectangle in window pixels.
	Rect *[4]float64 `json:"rect,omitempty"`

	// Paint ops. UV is a texel index, and Points is a run of them: a real
	// stroke rather than a series of dabs, so a script exercises the same
	// interpolation the pointer does.
	Res int     `json:"res,omitempty"`
	Hex string  `json:"hex,omitempty"`
	UV  *[2]int `json:"uv,omitempty"`
	// Tolerance is the wand's colour latitude, 0..255.
	Tolerance int      `json:"tolerance,omitempty"`
	Points    [][2]int `json:"points,omitempty"`
	Size      int      `json:"size,omitempty"`
	// Frames is how many frames a wait lets pass. Nothing else in a script
	// spends time on purpose: settle stops the moment nothing is animating,
	// and a toast is not an animation.
	Frames int `json:"frames,omitempty"`

	// Tile ops (Tile_paint.md TP2). Path is shared with the file ops below;
	// Tile is the selected tile's index in the sheet, named so because the
	// parser owns the field called Index.
	TileW       int `json:"tileW,omitempty"`
	TileH       int `json:"tileH,omitempty"`
	TileMargin  int `json:"tileMargin,omitempty"`
	TileSpacing int `json:"tileSpacing,omitempty"`
	Tile        int `json:"tile,omitempty"`
	Rot         int `json:"rot,omitempty"`
	// Strength is view.ao's 0..1 ambient-occlusion scale.
	Strength *float64 `json:"strength,omitempty"`
	FlipTile bool     `json:"flip,omitempty"`

	// Modifier keys held for the next pointer op. They matter as much as the
	// position does: Shift adds to a selection, Ctrl snaps fine, Alt is free.
	Shift bool `json:"shift,omitempty"`
	Ctrl  bool `json:"ctrl,omitempty"`
	Alt   bool `json:"alt,omitempty"`

	// File ops. Path is explicit rather than dialog-driven: a headless run has
	// nobody to answer a dialog, and a test that named its own file is clearer
	// than one that guessed where a dialog would have put it.
	Path string `json:"path,omitempty"`
	// Scale is the mesh import's units-per-file-unit (V-144). A pointer so a
	// script can say "use the fitted default" by leaving it out.
	Scale *float64 `json:"scale,omitempty"`

	// Visibility, camera and capture ops.
	Visible *bool  `json:"visible,omitempty"`
	View    string `json:"view,omitempty"`
	Name    string `json:"name,omitempty"`

	// At is a window pixel, used by the pick op to interrogate the ID pass.
	At *[2]float64 `json:"at,omitempty"`

	// Index is the op's position in the script, filled in by Parse so error
	// messages can name it (SPEC-RENDER §9).
	Index int `json:"-"`
}

// Script is a parsed op script.
type Script struct {
	Ops []Op
}

// OpError names the failing op by index and kind, which is what golden tests
// print when a script fails.
type OpError struct {
	Index int
	Op    string
	Err   error
}

func (e *OpError) Error() string {
	return fmt.Sprintf("op %d (%s): %v", e.Index, e.Op, e.Err)
}

func (e *OpError) Unwrap() error { return e.Err }

// Errorf builds an OpError for an op.
func (o Op) Errorf(format string, args ...any) error {
	return &OpError{Index: o.Index, Op: o.Op, Err: fmt.Errorf(format, args...)}
}

// Wrap attaches op context to an existing error.
func (o Op) Wrap(err error) error {
	if err == nil {
		return nil
	}
	return &OpError{Index: o.Index, Op: o.Op, Err: err}
}

// ParseScript decodes an op script and checks that every op is one we know and
// carries the fields it needs. Unknown ops are an error rather than a silent
// skip, so a stale script fails loudly.
func ParseScript(data []byte) (*Script, error) {
	var ops []Op
	dec := json.NewDecoder(newTrimReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ops); err != nil {
		return nil, fmt.Errorf("parse op script: %w", err)
	}
	for i := range ops {
		ops[i].Index = i
		if err := ops[i].validate(); err != nil {
			return nil, err
		}
	}
	return &Script{Ops: ops}, nil
}

// LoadScript reads and parses a script file.
func LoadScript(path string) (*Script, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read op script: %w", err)
	}
	return ParseScript(data)
}

// knownOps is the set of ops the format defines. Ops whose tools have not
// landed yet still parse, so scripts can be written ahead of the milestone that
// executes them; the executor reports the unimplemented op at run time.
var knownOps = map[string]bool{
	"sketch.begin": true, "sketch.line": true, "sketch.rect": true,
	"sketch.circle": true, "sketch.finish": true, "sketch.tool": true,
	"sketch.face": true, "sketch.project": true, "sketch.grid": true,
	"sketch.point": true, "sketch.midline": true, "sketch.centerrect": true,
	"sketch.alignedrect": true, "sketch.construction": true,
	"sketch.circle3": true, "sketch.arc": true, "sketch.ellipse": true,
	"sketch.polygon": true, "sketch.slot": true,
	"sketch.spline": true, "sketch.bezier": true,
	"sketch.fillet": true, "sketch.chamfer": true, "sketch.offset": true,
	"sketch.mirror": true, "sketch.pattern": true, "sketch.select": true,
	"pushpull":      true,
	"chamfer.begin": true, "chamfer.distance": true, "chamfer.commit": true, "chamfer.cancel": true,
	"extrude": true, "extrude.begin": true, "extrude.commit": true,
	"extrude.cancel": true,
	"extrude.pick":   true, "body.copy": true, "body.paste": true,
	"library.testdir": true, "library.browse": true, "ui.type": true,
	"ui.text": true, "ui.key": true,
	"library.insert": true, "library.edit": true, "library.save": true, "library.update": true,
	"boolean": true, "boolean.begin": true,
	"boolean.commit": true, "boolean.cancel": true,
	"select": true, "move": true, "rotate": true,
	"marker.front": true, "marker.top": true, "marker.thruster": true,
	"marker.attachment": true,
	"marker.clear":      true, "marker.move": true,
	"duplicate": true, "box.select": true,
	"paint.begin": true, "paint.exit": true, "paint.res": true,
	"paint.color": true, "paint.tool": true, "paint.size": true,
	"paint.pixel": true, "paint.stroke": true, "paint.resample": true,
	"paint.pickedge": true,
	"tile.import":    true, "tile.grid": true, "tile.select": true,
	"tile.orient": true, "tile.stamp": true,
	"paint.textures": true, "paint.faceview": true,
	"paint.color2": true, "paint.swap": true, "paint.dither": true,
	"paint.shapefill": true, "paint.lock": true, "paint.unlock": true,
	"paint.edges": true, "paint.edgewidth": true, "paint.creases": true,
	"paint.wand": true, "paint.wandclear": true,
	"paint.alpha": true,
	"file.new":    true, "file.save": true, "file.open": true,
	"file.importmesh": true, "import.scale": true, "import.center": true,
	"import.commit": true, "import.cancel": true,
	"file.export": true, "file.autosave": true, "file.recover": true,
	"file.discard": true, "export.begin": true, "export.format": true,
	"export.cancel": true, "export.scale": true, "export.alpha": true,
	"body.visible": true, "plane.visible": true, "sketch.visible": true,
	"deselect": true, "delete": true, "undo": true, "redo": true,
	"hover": true, "click": true, "drag": true, "drag.release": true,
	"wheel":       true,
	"ui.tree":     true,
	"camera.view": true, "camera.frame": true, "camera.orbit": true,
	"camera.lookat": true,
	"camera.zoom":   true, "camera.project": true,
	"settle": true, "wait": true, "shot": true, "pick": true, "dump": true,
	"view.ao": true, "view.shading": true,
	"palette.browse": true, "palette.search": true, "palette.apply": true,
}

func (o Op) validate() error {
	if o.Op == "" {
		return o.Errorf("missing op name")
	}
	if !knownOps[o.Op] {
		return o.Errorf("unknown op %q", o.Op)
	}
	switch o.Op {
	case "ui.key":
		if o.Name == "" {
			return o.Errorf("needs a key name")
		}
	case "library.insert", "library.edit", "library.update":
		if o.Target == "" {
			return o.Errorf("needs the library part ID in target")
		}
	case "sketch.begin":
		if o.Plane == "" {
			return o.Errorf("needs a plane")
		}
	case "sketch.line":
		if o.From == nil || o.To == nil {
			return o.Errorf("needs from and to")
		}
	case "sketch.rect":
		if o.A == nil || o.B == nil {
			return o.Errorf("needs a and b")
		}
	case "sketch.circle":
		if o.C == nil || o.R <= 0 {
			return o.Errorf("needs c and a positive r")
		}
	case "camera.view":
		if o.View == "" {
			return o.Errorf("needs a view name")
		}
	case "camera.project":
		if o.Kind != "ortho" && o.Kind != "perspective" {
			return o.Errorf("kind must be ortho or perspective")
		}
	case "shot":
		if o.Name == "" {
			return o.Errorf("needs a name")
		}
	case "pick", "hover", "click":
		if o.At == nil {
			return o.Errorf("needs at [x,y]")
		}
	case "drag":
		if o.From == nil || o.To == nil {
			return o.Errorf("needs from [x,y] and to [x,y] in window pixels")
		}
	case "view.ao":
		if o.Strength == nil || *o.Strength < 0 || *o.Strength > 1 {
			return o.Errorf("needs strength between 0 and 1")
		}
	case "view.shading":
		if o.On == nil {
			return o.Errorf("needs on (true for shaded, false for flat)")
		}
	case "wheel":
		if o.Degrees == 0 {
			return o.Errorf("needs degrees: the notches to scroll, positive is up")
		}
	case "wait":
		if o.Frames <= 0 {
			return o.Errorf("needs frames: how many 1/60 s frames to let pass")
		}
	case "palette.browse":
		if o.On == nil {
			return o.Errorf("needs on (true to open the library, false to close)")
		}
	case "palette.apply":
		if o.Name == "" {
			return o.Errorf("needs the name of a palette")
		}
	case "tile.import":
		if o.Path == "" {
			return o.Errorf("needs a path")
		}
	case "tile.grid":
		if o.TileW <= 0 || o.TileH <= 0 {
			return o.Errorf("needs tileW and tileH")
		}
	case "tile.stamp":
		if o.UV == nil {
			return o.Errorf("needs uv [x,y]")
		}
	case "marker.move":
		if o.Delta == nil {
			return o.Errorf("needs delta [x,y,z]")
		}
	case "plane.visible":
		if o.Plane == "" || o.Visible == nil {
			return o.Errorf("needs plane and visible")
		}
	case "sketch.visible":
		if o.Sketch == "" || o.Visible == nil {
			return o.Errorf("needs sketch and visible")
		}
	case "ui.tree":
		if o.Visible == nil {
			return o.Errorf("needs visible")
		}
	case "body.visible":
		if o.Body == "" || o.Visible == nil {
			return o.Errorf("needs body and visible")
		}
	case "paint.res", "paint.resample":
		if o.Res == 0 {
			return o.Errorf("needs a res")
		}
	case "paint.color", "paint.color2":
		if o.Hex == "" {
			return o.Errorf("needs a hex colour")
		}
	case "paint.dither":
		if o.Kind == "" {
			return o.Errorf("needs a mode: None, 2x2, 4x4 or 8x8")
		}
	case "paint.shapefill":
		if o.Visible == nil {
			return o.Errorf("needs visible")
		}
	case "file.save", "file.open", "file.export":
		if o.Path == "" {
			return o.Errorf("needs a path")
		}
	case "export.format":
		if o.Kind == "" {
			return o.Errorf("needs a format extension, without the dot")
		}
	case "export.alpha":
		if o.Visible == nil {
			return o.Errorf("needs visible")
		}
	case "paint.tool":
		if o.Kind == "" {
			return o.Errorf("needs a tool: pencil, eraser, fill or pick")
		}
	case "paint.size":
		if o.Size == 0 {
			return o.Errorf("needs a size")
		}
	case "paint.alpha":
		if o.Size == 0 {
			return o.Errorf("needs an alpha, 1..255")
		}
	case "paint.pixel":
		if o.Body == "" || o.UV == nil {
			return o.Errorf("needs body and uv [x,y] in texels")
		}
	case "paint.stroke":
		if o.Body == "" || len(o.Points) == 0 {
			return o.Errorf("needs body and points [[x,y],...] in texels")
		}
	case "paint.textures":
		if o.Visible == nil {
			return o.Errorf("needs visible")
		}
	}
	return nil
}
