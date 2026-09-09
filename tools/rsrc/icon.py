"""Cuts every size the launcher needs out of one 256x256 source.

Nothing here runs during a build.  The two things it writes are checked in --
`sacred.ico`, which `main.go` turns into the executable's icon resource, and
`ui/web/icon.png`, which the page carries as a data URI, because the build has
to work on a machine with only Go on it, and because a picture regenerated on
every build is a picture nobody can review in a diff.

It is checked in for the same reason no address in this project is typed by
hand: the sizes below came from one source through one filter, and the next
person to need a 40px copy should get it from here rather than from a paint
program.

    python tools/rsrc/icon.py            # from the repository root
    python tools/rsrc/icon.py <source>   # from another 256x256 PNG

Needs Pillow (`pip install pillow`).  Lanczos on the way down; 256 is the
source itself, copied rather than resampled.
"""

import sys
from pathlib import Path

from PIL import Image

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent

# What Windows asks an .ico for.  16/32/48 are the shell's own sizes, 256 is
# what a large-icon view and the Alt-Tab switcher use, and the rest are the
# steps in between that keep a 125% or 150% display from scaling one of those.
ICO_SIZES = (16, 20, 24, 32, 40, 48, 64, 96, 128, 256)

# The page draws the mark at 46 CSS pixels and a mod's icon at 40, so 128
# covers both to nearly 3x without carrying a quarter of a megabyte of base64
# through the HTML string the window is loaded from.
PAGE_SIZE = 128


def scaled(source: Image.Image, size: int) -> Image.Image:
    if size == source.width == source.height:
        return source.copy()
    return source.resize((size, size), Image.Resampling.LANCZOS)


def main() -> int:
    source = Path(sys.argv[1]) if len(sys.argv) > 1 else HERE / "sacred.png"
    image = Image.open(source).convert("RGBA")
    if image.width != image.height:
        print(f"{source} is {image.width}x{image.height}. The source image must be square")
        return 1

    icons = [scaled(image, size) for size in ICO_SIZES]
    ico = HERE / "sacred.ico"
    # Both arguments are needed and they do different things. sizes= is the list
    # of frames to write, and its default is a fixed seven that silently drops
    # the rest; append_images= is what makes each of those frames the one this
    # script resampled rather than one Pillow resampled for itself.
    icons[-1].save(
        ico,
        format="ICO",
        sizes=[(size, size) for size in ICO_SIZES],
        append_images=icons[:-1],
    )
    print(f"{ico}  {', '.join(str(s) for s in ICO_SIZES)}")

    page = ROOT / "ui" / "web" / "icon.png"
    scaled(image, PAGE_SIZE).save(page, format="PNG", optimize=True)
    print(f"{page}  {PAGE_SIZE}x{PAGE_SIZE}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
