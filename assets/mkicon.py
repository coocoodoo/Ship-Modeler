"""The program's own icon: pixel art painted onto an isometric solid.

Everything here comes from the app's own tokens - the panel ground, a body's
auto colour on its three faces, and the paint accents - so the icon is a
small picture of what the program is for rather than a generic cube.

Windows wants the mark twice, by two different mechanisms, so this writes
both from the one drawing:

    python assets/mkicon.py assets/modeler.ico
    go run github.com/akavel/rsrc -ico assets/modeler.ico -arch amd64 -o cmd/modeler/modeler_windows_amd64.syso

modeler.ico becomes the .syso the Go linker compiles into the executable,
which is what Explorer shows. window_icon.png is embedded and handed to
raylib at startup, which is what the running window shows. They are the same
picture on purpose: two marks would make one program look like two.
"""
import os
import sys
from PIL import Image, ImageDraw

OUT = sys.argv[1]
SS = 8

GROUND = (22, 25, 32)
EDGE   = (60, 68, 84)
TOP    = (186, 202, 214)
LEFT   = (120, 138, 154)
RIGHT  = (86, 100, 116)
GRID   = (156, 174, 189)
PINK   = (255, 111, 181)
BLUE   = (83, 164, 255)
GOLD   = (255, 217, 74)
TEAL   = (46, 208, 204)

N = 4  # texels across the top face


def lerp(a, b, t):
    return (a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t)


def draw(size, detail=True):
    n = size * SS
    im = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)

    pad = n * 0.045
    d.rounded_rectangle([pad, pad, n - pad, n - pad], radius=n * 0.21,
                        fill=GROUND + (255,), outline=EDGE + (255,),
                        width=max(1, int(n * 0.012)))

    cx, cy = n * 0.5, n * 0.40
    s, h = n * 0.30, n * 0.28
    top = (cx, cy - s * 0.5)          # far corner
    right = (cx + s, cy)
    left = (cx - s, cy)
    bot = (cx, cy + s * 0.5)          # near corner

    # Body: the two side faces first, so the lit top sits over them.
    d.polygon([left, bot, (bot[0], bot[1] + h), (left[0], left[1] + h)], fill=LEFT + (255,))
    d.polygon([bot, right, (right[0], right[1] + h), (bot[0], bot[1] + h)], fill=RIGHT + (255,))
    d.polygon([top, right, bot, left], fill=TOP + (255,))

    if detail:
        # The top face as a grid of texels, a few of them painted. This is the
        # whole idea of the program in one mark: pixels laid on a solid.
        paint = {(0, 1): PINK, (1, 1): PINK, (2, 0): BLUE, (1, 3): GOLD, (3, 2): TEAL}
        for u in range(N):
            for v in range(N):
                col = paint.get((u, v))
                if col is None:
                    continue
                a0 = lerp(top, right, u / N)
                a1 = lerp(top, right, (u + 1) / N)
                dv0 = ((left[0] - top[0]) * v / N, (left[1] - top[1]) * v / N)
                dv1 = ((left[0] - top[0]) * (v + 1) / N, (left[1] - top[1]) * (v + 1) / N)
                quad = [(a0[0] + dv0[0], a0[1] + dv0[1]),
                        (a1[0] + dv0[0], a1[1] + dv0[1]),
                        (a1[0] + dv1[0], a1[1] + dv1[1]),
                        (a0[0] + dv1[0], a0[1] + dv1[1])]
                d.polygon(quad, fill=col + (255,))
        # Faint grid lines over the whole top, so the unpainted cells read as
        # cells waiting rather than as blank metal.
        w = max(1, int(n * 0.006))
        for i in range(1, N):
            p0 = lerp(top, right, i / N)
            p1 = lerp(left, bot, i / N)
            d.line([p0, p1], fill=GRID + (255,), width=w)
            q0 = lerp(top, left, i / N)
            q1 = lerp(right, bot, i / N)
            d.line([q0, q1], fill=GRID + (255,), width=w)

    return im.resize((size, size), Image.LANCZOS)


sizes = [256, 128, 64, 48, 32, 24, 16]
imgs = [draw(s, detail=s >= 32) for s in sizes]
imgs[0].save(OUT, format="ICO", sizes=[(s, s) for s in sizes])
# The window icon raylib loads at startup, from the same drawing.
draw(64).save(os.path.join(os.path.dirname(OUT) or ".", "window_icon.png"))

# Review sheets go wherever the caller asks, never into the shipped assets.
if len(sys.argv) > 2:
    imgs[0].save(os.path.join(sys.argv[2], "icon_256.png"))
    strip = Image.new("RGBA", (sum(sizes) + 12 * len(sizes), 276), (235, 236, 240, 255))
    x = 6
    for im, sz in zip(imgs, sizes):
        strip.paste(im, (x, 6), im)
        x += sz + 12
    strip.save(os.path.join(sys.argv[2], "icon_strip.png"))

print("wrote", OUT)
