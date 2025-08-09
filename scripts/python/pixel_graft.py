"""
Composite a 25×18 pixel-art overlay onto a 32×32 background,
adding a 1-pixel black border around the overlay (so it occupies 27×20).
"""

import argparse
import sys

from PIL import Image, ImageDraw


def graft_image(
    background_path: str, overlay_path: str, x: int, y: int, output_path: str
) -> None:
    # Load background and overlay (RGBA to preserve transparency)
    try:
        bg = Image.open(background_path).convert("RGBA")
    except IOError as e:
        print(
            f"Error: cannot open background file {background_path}: {e}",
            file=sys.stderr,
        )
        sys.exit(1)
    try:
        overlay = Image.open(overlay_path).convert("RGBA")
    except IOError as e:
        print(f"Error: cannot open overlay file {overlay_path}: {e}", file=sys.stderr)
        sys.exit(1)

    ov_w, ov_h = overlay.size
    # Check bounds
    bg_w, bg_h = bg.size
    if not (
        0 <= x - 1 < bg_w
        and 0 <= y - 1 < bg_h
        and x + ov_w <= bg_w
        and y + ov_h <= bg_h
    ):
        print("Warning: overlay+border may exceed background bounds", file=sys.stderr)

    # Create result and draw border
    result = bg.copy()
    draw = ImageDraw.Draw(result)
    # Draw 1px black rectangle around the overlay region
    border_box = [x - 1, y - 1, x + ov_w, y + ov_h]
    draw.rectangle(border_box, outline=(0, 0, 0, 255))

    # Paste the overlay on top
    result.paste(overlay, (x, y), overlay)

    try:
        result.save(output_path)
        print(f"✅ Saved composite with border to {output_path}")
    except IOError as e:
        print(f"Error: cannot save output file {output_path}: {e}", file=sys.stderr)
        sys.exit(1)


def parse_args():
    p = argparse.ArgumentParser(
        description="Composite a pixel-art overlay onto a background with a 1px black border."
    )
    p.add_argument("--background", "-b", required=True, help="32×32 background PNG")
    p.add_argument("--overlay", "-i", required=True, help="25×18 overlay PNG")
    p.add_argument(
        "--output", "-o", required=True, help="Where to write the 32×32 output PNG"
    )
    p.add_argument("--x", type=int, default=6, help="Horizontal offset (default: 6)")
    p.add_argument("--y", type=int, default=7, help="Vertical offset (default: 7)")
    return p.parse_args()


if __name__ == "__main__":
    args = parse_args()
    graft_image(args.background, args.overlay, args.x, args.y, args.output)
