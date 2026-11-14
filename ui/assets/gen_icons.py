"""Generate AdaKaki's icon assets from the persimmon mark.

The mark is three shapes on a 1024 grid: a terminal prompt's chevron and
its cursor dot, which doubles as the persimmon. Sizes below 32 drop the
dot and use a solid chevron, so the prompt still reads at 16 px.

After changing icon.ico, refresh the Windows executable resource with:
    rsrc -ico ui/assets/icon.ico -arch amd64 -o rsrc_windows_amd64.syso
    rsrc -ico ui/assets/icon.ico -arch arm64 -o rsrc_windows_arm64.syso
"""

import io
import struct
from pathlib import Path

from PIL import Image, ImageDraw

ASSETS = Path(__file__).resolve().parent

ACCENT = (228, 99, 60, 255)  # #E4633C, the dark-theme persimmon
INK = (26, 15, 11, 255)  # #1A0F0B, the mark's glyph
PAPER = (243, 241, 236, 255)  # #F3F1EC, the light single-color tile

# The chevron: down to the apex, back out; the dot sits on its baseline.
CHEVRON = [(312, 320), (500, 512), (312, 704)]
# Below 24 px the dot goes and the chevron grows into the room it leaves,
# so the prompt still fills the tile at 16.
# A solid chevron: at 16 px a stroked one turns into a diagonal smudge, so
# the small sizes use a filled wedge instead.
CHEVRON_SMALL = [(240, 280), (680, 512), (240, 744), (240, 574), (510, 512), (240, 450)]
STROKE = 78
STROKE_SMALL = 150
DOT = (650, 704, 64)
DOT_MIN_SIZE = 32


def round_line(draw, points, width, fill):
    """A polyline with round caps and joints, like Lucide's strokes."""
    draw.line(points, fill=fill, width=int(round(width)), joint="curve")
    r = width / 2
    for x, y in (points[0], points[-1]):
        draw.ellipse((x - r, y - r, x + r, y + r), fill=fill)


def render(size, tile=ACCENT, ink=INK, supersample=4):
    """Draw the mark at size, supersampled then resampled down."""
    s = size * supersample
    img = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    k = s / 1024
    if tile is not None:
        d.rounded_rectangle(
            (64 * k, 64 * k, 960 * k, 960 * k), radius=200 * k, fill=tile
        )
    small = size < DOT_MIN_SIZE
    if small:
        d.polygon([(x * k, y * k) for x, y in CHEVRON_SMALL], fill=ink)
    else:
        round_line(d, [(x * k, y * k) for x, y in CHEVRON], STROKE * k, ink)
    if not small:
        cx, cy, r = DOT
        d.ellipse(
            ((cx - r) * k, (cy - r) * k, (cx + r) * k, (cy + r) * k), fill=ink
        )
    return img.resize((size, size), Image.Resampling.LANCZOS)


def save_ico(path, sizes):
    """Write an .ico whose entries are per-size drawings, PNG encoded.

    Pillow resamples one image for every entry, which would keep the dot at
    16 px where it turns to mush; Windows takes PNG entries per size, so
    each size gets its own drawing.
    """
    images = [(sz, render(sz)) for sz in sizes]
    blobs = []
    for _, img in images:
        buf = io.BytesIO()
        img.save(buf, format="PNG", optimize=True)
        blobs.append(buf.getvalue())

    header = struct.pack("<HHH", 0, 1, len(images))
    offset = 6 + 16 * len(images)
    entries, data = b"", b""
    for (sz, _), blob in zip(images, blobs):
        side = 0 if sz >= 256 else sz
        entries += struct.pack(
            "<BBBBHHII", side, side, 0, 0, 1, 32, len(blob), offset
        )
        offset += len(blob)
        data += blob
    path.write_bytes(header + entries + data)


def main():
    render(1024).save(ASSETS / "icon.png", optimize=True)
    save_ico(
        ASSETS / "icon.ico",
        [16, 20, 24, 32, 48, 64, 128, 256],
    )
    render(32).save(ASSETS / "tray.png", optimize=True)
    render(32, tile=None, ink=(0, 0, 0, 255)).save(
        ASSETS / "tray-template.png", optimize=True
    )
    render(512, tile=PAPER).save(ASSETS / "icon-light.png", optimize=True)


if __name__ == "__main__":
    main()
