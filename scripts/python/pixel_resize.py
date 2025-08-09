"""
A tiny command-line tool to resize pixel-art images (e.g. flags) with no blur,
using nearest-neighbor sampling.

Dependencies:
    pip install pillow
"""

import argparse
import sys

from PIL import Image


def resize_image(input_path: str, output_path: str, width: int, height: int) -> None:
    """
    Open an image, convert to RGBA to preserve any transparency, resize with
    nearest-neighbor, and save.
    """
    try:
        img = Image.open(input_path)
    except IOError as e:
        print(f"Error: cannot open input file {input_path}: {e}", file=sys.stderr)
        sys.exit(1)

    # Ensure alpha channel (in case of transparency), and convert palette modes
    img = img.convert("RGBA")

    # Perform the resize—crisp pixels guaranteed
    resized = img.resize((width, height), resample=Image.Resampling.NEAREST)

    try:
        resized.save(output_path)
        print(f"✅ Saved {output_path} ({width}×{height})")
    except IOError as e:
        print(f"Error: cannot save output file {output_path}: {e}", file=sys.stderr)
        sys.exit(1)


def parse_args():
    p = argparse.ArgumentParser(
        description="Resize pixel-art images with nearest-neighbor (no blur)."
    )
    p.add_argument(
        "--input", "-i", required=True, help="Path to source PNG (e.g. 32×18 flag)."
    )
    p.add_argument(
        "--output", "-o", required=True, help="Path for resized PNG (e.g. 27×16 flag)."
    )
    p.add_argument(
        "--width", "-w", type=int, required=True, help="Target width in pixels."
    )
    # Use -H instead of -h to avoid conflict with argparse's help flag
    p.add_argument(
        "--height", "-H", type=int, required=True, help="Target height in pixels."
    )
    return p.parse_args()


if __name__ == "__main__":
    args = parse_args()
    resize_image(args.input, args.output, args.width, args.height)
