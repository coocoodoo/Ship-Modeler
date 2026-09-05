# Viewport ambient occlusion

The renderer now evaluates AO on the GPU each frame, using the depth of all
visible opaque bodies together. It replaces the CPU vertex bake and removes
the render-only tessellation that existed to interpolate that bake.

The pass packs depth into RGB24 on a GL 3.3 render target, reconstructs view
positions and surface normals, samples a 32-direction hemisphere, then applies
a depth-aware 5×5 filter. A fixed interleaved sampling pattern avoids temporal
random flicker. AO attenuates ambient lighting; direct lighting, paint data,
selection outlines, and transparent previews remain separate.

The radius follows model dimensions, with bounds of 0.2–5 world units. It does
not change when zooming. Targets resize with the viewport and restore the
previous framebuffer and matrices, including during transparent PNG capture.
Turning AO off or entering flat view skips the depth and filtering passes.

The bottom-left control is always present, including with the scene panel
collapsed. Clicking it from flat view restores lighting and enables AO. The
existing `ao` setting stores the preference.

This is screen-space AO, not hardware ray tracing or global illumination.
Occluders outside the screen or behind the visible depth surface cannot
contribute, and very thin or subpixel contacts may disappear with distance.

The depth/normal sampling and edge-aware filtering approach follows established
screen-space techniques; see the [AMD CACAO technique documentation](https://gpuopen.com/manuals/fidelityfx_sdk/techniques/combined-adaptive-compute-ambient-occlusion/).
This implementation uses custom GL 3.3 shaders and does not incorporate CACAO code.

Regression coverage compares actual rendered pixels for contact between two
separate convex bodies, on/off clicks, orthographic and perspective views,
viewport resizing, and flat-view behavior. Separate tests cover persistence,
unchanged mesh coverage, and icon path handling.
