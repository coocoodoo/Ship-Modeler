# Architecture Decision Records

These decisions were made deliberately at planning time (2026-08-26). The executing agent should **not relitigate** them mid-build. If reality contradicts one, record the evidence under § Deviations log, implement the named reversal path, and continue.

> **Superseding user directive (2026-08-26):** an earlier draft of this plan was pure-Go-only. The user explicitly lifted that: *"Don't use pure Go — feel free to use libraries as needed to accelerate development."* The records below reflect the library-friendly stack. cgo is fully allowed (WinLibs mingw64 gcc is installed and on PATH).

---

## D-01 — raylib-go as the engine layer (window, input, GPU 3D/2D)

**Decision:** Use `github.com/gen2brain/raylib-go/raylib` for the OS window, input polling, GPU rendering (depth buffer, MSAA, render textures, shaders via rlgl), 2D drawing, and text. The viewport is our own set of render passes over raylib (SPEC-RENDER); the UI is our own widget kit drawn with raylib 2D.

**Why:**
- One dependency covers the entire platform/engine surface. raylib-go vendors the raylib C sources and compiles them through cgo with the gcc already on this machine — no SDKs, no DLL hunting, no CMake for the app itself.
- Real GPU: z-buffer, MSAA 4×, render-to-texture, custom GLSL — the "very polished" pillar (smooth 60 fps at any resolution, clean AA) comes essentially free compared to a software rasterizer.
- Battle-tested, huge example corpus (agent-friendly), simple C-style API.
- Render textures give us the ID pick pass (exact per-pixel picking) with a few lines.

**Alternatives rejected:**
- *Ebiten + in-house CPU rasterizer* (the old pure-Go plan): more code to write (a whole rasterizer), no MSAA, CPU-bound at 4K. Its one real advantage — byte-deterministic goldens — is recovered well enough via tolerance-based GPU goldens (TESTING §5).
- *go-gl + GLFW raw:* all the plumbing raylib already does, with none of the batteries.
- *G3N / engines:* scene-graph frameworks fight a CAD tool's custom passes; smaller communities.

**Reversal path:** none anticipated; `internal/render` isolates raylib so a different backend remains a bounded rewrite.

## D-02 — In-house immediate-mode widget kit (not Dear ImGui, not raygui, not ebitenui)

**Decision:** Small IMGUI in `internal/ui` drawn with raylib primitives, implementing exactly the frozen widget list of SPEC-UX §4, themed per SPEC-UX §3.

**Why:** "Very polished" is a pillar and the UI is bespoke (floating cards near gizmos, hint bar, tree with eye/swatch rows, chip groups). Dear ImGui (cimgui-go/giu) would accelerate raw widget count but fights custom look/feel and adds a heavyweight dependency for ~15 widget kinds; raygui is too crude for the polish bar. The costly widgets (text input with caret, drag-number) are bounded and specced.

**Risk control:** widget list frozen at M1 — creep requires a DECISIONS entry. If widget work overruns badly (>2 sessions past M1 estimate), reconsider cimgui-go here with a Deviations entry.

## D-03 — Numeric model: exact integer sketch space, float64 model space with snap-on-author

**Decision:**
- **Sketch space (2D):** `int64` subunits, 256 per world unit. The arrangement/region engine (the "is it closed?" question, R3) is integer-exact — no epsilon tuning, deterministic forever.
- **Model space (3D):** `float64`. Every *authored* coordinate is grid-snapped at input time (extrude depths, moves, 90° rotations are exact permutations), so user geometry lives on the 1/256-u grid; only boolean intersection vertices may land off-grid, which is geometrically correct. Tolerances centralized in GEOM §1.3.

**Why:** Integer exactness is cheap and valuable in 2D where closedness is decided. In 3D, Manifold (D-04) owns robustness internally, so exact int predicates + int128 machinery are no longer needed — float64 + snap-on-author keeps the pixel-native contract with far less code.

## D-04 — Booleans via Manifold (vendored static lib + ~15-function cgo binding)

**Decision:** Use **Manifold** (github.com/elalish/manifold, Apache-2.0) through its C API (`manifoldc`) for union/difference/intersect. Built once with CMake + the installed mingw64, vendored as static libs in `third_party/manifold/` (see D-12). Face provenance via `manifold_reserve_ids` + per-run `runOriginalID` maps result triangles back to source faces (paint/selection continuity). Our code keeps: prism construction, polygon-face reassembly from result triangles, and a validation gate.

**Why:** Robust mesh booleans are *the* graveyard of projects like this one — and blocky modeling makes coplanar contact (the hardest case) the common case. Manifold is the modern, actively-maintained, guaranteed-manifold-output kernel (adopted by OpenSCAD and others). Binding ~15 C functions beats months of BSP debugging; its provenance IDs map exactly onto our paint-persistence design.

**Fallback (keep honest):** if Manifold can't be built/bound after two honest timeboxed attempts, implement the in-house BSP CSG fully specced in **SPEC-GEOMETRY Appendix A** against the same test matrix. The matrix, not the library's reputation, is the acceptance bar either way.

## D-05 — Pseudo-parametric document (direct meshes + feature log), not full history regen

**Decision:** Bodies are polygon meshes edited in place. Every operation is recorded in an append-only feature log (debug/undo/forward-compat), but editing an old sketch does not regenerate downstream bodies in v1.

**Why:** Full parametric regen (OnShape's actual core) is a multi-month kernel problem (topological naming, reference healing). The user's workflow — sketch, extrude, tweak verts, paint — is served by direct modeling plus persistent, re-usable sketches. The UI is explicit about this (SPEC-UX §8.8) so it never *feels* broken. M10 revisits parametric-lite.

## D-06 — Sketch regions via integer planar arrangement; no 2D boolean library

**Decision:** Closed regions come from a planar-graph face walk over quantized, split, welded segments (SPEC-GEOMETRY §4). Multi-region extrudes union in 3D via Manifold (which we have anyway). Draft uses an in-house miter offset with validity clamp (GEOM §5.3) — small, fully specced, no dependency roulette on Clipper ports.

## D-07 — Polygon faces with holes as the mesh face primitive (triangulate only internally)

**Decision:** `Face = planar loops (outer + holes) + stable FaceUID + optional paint + srcFace lineage`. Triangulation happens for rendering and for MeshGL handoff; result triangles are merged back into polygon faces (grouped by originalID + plane) after booleans.

**Why:** Faces are the user-facing unit: you sketch on one, push/pull one, paint one. Stable identity is what keeps paint glued and picking meaningful. Triangle soup would leak into every tool's UX.

## D-08 — Orthographic default projection

**Decision:** Ortho by default; `O` toggles perspective; view-cube face clicks read like blueprints. (User may override — PLAN Q1.)

## D-09 — `.ship` project container = zip(JSON + PNGs)

**Decision:** `manifest.json` (format version), `document.json` (sketch ints in subunits, model floats, human-diffable), `paint/<faceUID>.png`, `thumbnail.png`. Tolerant reader (unknown fields ignored, never crash on old/new versions).

## D-10 — File dialogs via ncruces/zenity

**Decision:** Open/Save/Export dialogs through `github.com/ncruces/zenity`. Color picking uses our own in-app picker. **Fallback:** minimal in-app file browser panel if zenity misbehaves; note in Deviations.

## D-11 — Fonts embedded, icons procedural

**Decision:** UI text = Go Regular TTF bytes (`golang.org/x/image/font/gofont/goregular`) loaded via raylib `LoadFontFromMemory` at the needed pixel sizes per DPI scale; icons are stroke-vector drawings in code (`ui/icons.go`). No binary art assets the executor can't author or regenerate.

## D-12 — Vendoring policy for Manifold

**Decision:** Build once in M4: pin a v3.x release tag (record the exact tag here when done), CMake flags `-G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release -DMANIFOLD_PAR=OFF -DMANIFOLD_TEST=OFF -DBUILD_SHARED_LIBS=OFF` (add `-DMANIFOLD_CROSS_SECTION=OFF` if the tag supports skipping it — we don't use cross sections). Copy `libmanifold*.a` + `include/manifold/*.h` + `LICENSE` into `third_party/manifold/`; commit them. cgo flags live only in `internal/geom/csg` (`CFLAGS: -I…`, `LDFLAGS: -L… -lmanifoldc -lmanifold -lstdc++ -static-libgcc -static-libstdc++`). The clone/build tree itself is NOT committed — only the vendored outputs, so later sessions never need CMake again.

---

## Deviations log

*(Executor appends here: date, spec section, what changed, why, evidence.)*
