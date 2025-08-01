#!/usr/bin/env -S uv run --script
# /// script
# requires-python = "~=3.12"
# dependencies = [
#   "pillow",
#   "pathlib",
#   "pyyaml",
#   "numpy",
# ]
# ///

"""Generate a spritesheet (PNG + JSON) from a folder of source images
and a YAML description file.

- Each source sprite that has `generate_colors: true` (default) is recoloured for **every** entry in `palette` below.  The original art is assumed to be red-tinted so that a simple hue-rotate recolours it cleanly.

Call:
    make spritesheet
"""

import colorsys
import json
import math
import re
import sys
from pathlib import Path

import numpy as np
import yaml
from PIL import Image

### Palette - name -> hex.  Add / remove colours freely.
palette: dict[str, str] = {
    "Red": "#E6194B",
    "Green": "#3CB44B",
    "Blue": "#0082C8",
    "Yellow": "#FFE119",
    "Orange": "#F58231",
    "Purple": "#911EB4",
    "Cyan": "#42D4F4",
    "Pink": "#F032E6",
    "Dark Gray": "#1E1E1E",
    "Light Gray": "#C8C8C8",
}

### Utility functions


def hex_to_rgb(hex_colour: str) -> tuple[float, float, float]:
    r = int(hex_colour[1:3], 16) / 255.0
    g = int(hex_colour[3:5], 16) / 255.0
    b = int(hex_colour[5:7], 16) / 255.0
    return r, g, b


def hex_to_hsv(hex_colour: str) -> tuple[float, float, float]:
    return colorsys.rgb_to_hsv(*hex_to_rgb(hex_colour))


def slug(name: str) -> str:
    """File-system-friendly id (lower-snake-case)."""
    return re.sub(r"[^a-z0-9]+", "_", name.lower()).strip("_")


### Pre-compute palette hues
# name -> hue° / sat / val
palette_hsv: dict[str, tuple[float, float, float]] = {
    n: (h * 360, s, v)
    for n, (h, s, v) in ((k, hex_to_hsv(v)) for k, v in palette.items())
}

### Colour-shift routine (handles greyscale specially)


def colour_variants(
    img: Image.Image, *, base_hue_range=(0, 20)
) -> list[tuple[Image.Image, str]]:
    arr = np.array(img)
    if arr.shape[2] == 4:
        rgb, alpha = arr[:, :, :3], arr[:, :, 3]
    else:
        rgb, alpha = arr, None

    rgb_n = rgb / 255.0
    hsv = np.zeros_like(rgb_n)
    for i in range(rgb_n.shape[0]):
        for j in range(rgb_n.shape[1]):
            hsv[i, j] = list(colorsys.rgb_to_hsv(*rgb_n[i, j]))
    hsv[:, :, 0] *= 360  # hue degrees

    red_mask = (
        ((hsv[:, :, 0] >= base_hue_range[0]) & (hsv[:, :, 0] <= base_hue_range[1]))
        | (hsv[:, :, 0] >= 340)
        | (hsv[:, :, 0] <= 40)
    ) & (hsv[:, :, 1] > 0.2)  # ensure some saturation

    variants: list[tuple[Image.Image, str]] = []
    for colour_name, (target_h, target_s, target_v) in palette_hsv.items():
        new_hsv = hsv.copy()
        new_hsv[red_mask, 0] = target_h

        if target_s < 0.05:  # greys (incl. black / white)
            # desaturate completely
            new_hsv[red_mask, 1] = 0.0
            # scale brightness so the mean matches target_v
            current_v = new_hsv[red_mask, 2]
            if current_v.size > 0:
                factor = target_v / (np.mean(current_v) + 1e-6)
                new_hsv[red_mask, 2] = np.clip(current_v * factor, 0, 1)
        else:
            # adjust saturation towards target, but keep relative shading
            current_s = new_hsv[red_mask, 1]
            factor = target_s / (np.mean(current_s) + 1e-6)
            new_hsv[red_mask, 1] = np.clip(current_s * factor, 0, 1)

        # Convert back to RGB
        h_ = new_hsv[:, :, 0] / 360.0
        s_ = new_hsv[:, :, 1]
        v_ = new_hsv[:, :, 2]
        out_rgb = np.zeros_like(rgb_n)
        for i in range(out_rgb.shape[0]):
            for j in range(out_rgb.shape[1]):
                out_rgb[i, j] = list(colorsys.hsv_to_rgb(h_[i, j], s_[i, j], v_[i, j]))
        out_arr = (out_rgb * 255).astype(np.uint8)
        if alpha is not None:
            out_arr = np.dstack([out_arr, alpha])
        variants.append((Image.fromarray(out_arr), colour_name))
    return variants


### YAML helpers


def load_config(path: str):
    with open(path, "r") as fh:
        return yaml.safe_load(fh)


### helpers for a safe hex value on every sprite


def rgb_to_hex(rgb: tuple[float, float, float]) -> str:
    r, g, b = (int(round(c * 255)) for c in rgb)
    return f"#{r:02X}{g:02X}{b:02X}"

def average_visible_rgb(img: Image.Image) -> tuple[float, float, float]:
    """Mean RGB (0-1) of all non-transparent pixels."""
    arr = np.asarray(img.convert("RGBA"))
    rgb = arr[..., :3].astype(np.float32) / 255.0
    alpha = arr[..., 3] > 0
    if not alpha.any():
        return (1.0, 1.0, 1.0)  # solid fallback (pure white)
    mean = rgb[alpha].mean(axis=0)
    return tuple(mean)


### Sprite-sheet packing


def make_sheet(
    input_dir: Path,
    cfg_path: str = "sprites.yaml",
    out_png: str = "spritesheet.png",
    out_json: str = "spritesheet.json",
):
    cfg = load_config(cfg_path)
    defaults: dict = cfg.get("default_settings", {})
    sprite_cfgs: dict = cfg.get("sprites", {})

    images = [
        p for p in input_dir.iterdir() if p.suffix.lower() in {".png", ".jpg", ".jpeg"}
    ]
    if not images:
        print("❌ No image files in", input_dir)
        return

    records = []
    for img_path in images:
        base = img_path.stem
        spec = {**defaults, **sprite_cfgs.get(base, {})}
        with Image.open(img_path) as original:
            if spec.get("generate_colors", True):
                variants = colour_variants(original)
            else:
                variants = [(original.copy(), "Original")]
            for var_img, cname in variants:
                records.append(
                    {
                        "id": f"{base}_{slug(cname)}" if cname != "Vivid Red" else base,
                        "base": base,
                        "colour": cname,
                        "img": var_img,
                        "cfg": spec,
                    }
                )

    w_max = max(r["img"].width for r in records)
    h_max = max(r["img"].height for r in records)
    grid = math.ceil(math.sqrt(len(records)))
    sheet_w, sheet_h = grid * w_max, grid * h_max
    sheet = Image.new("RGBA", (sheet_w, sheet_h), (0, 0, 0, 0))

    meta = {
        "texture": out_png,
        "frames": {},
        "meta": {
            "size": {"w": sheet_w, "h": sheet_h},
            "scale": "1",
            "palette": palette,
            "total_sprites": len(records),
            "original_count": len(images),
        },
    }

    for idx, rec in enumerate(records):
        col, row = idx % grid, idx // grid
        x, y = col * w_max, row * h_max
        ox = x + (w_max - rec["img"].width) // 2
        oy = y + (h_max - rec["img"].height) // 2
        sheet.paste(rec["img"], (ox, oy), rec["img"])

        meta["frames"][rec["id"]] = {
            "frame": {"x": ox, "y": oy, "w": rec["img"].width, "h": rec["img"].height},
            "rotated": False,
            "trimmed": False,
            "spriteSourceSize": {
                "x": 0,
                "y": 0,
                "w": rec["img"].width,
                "h": rec["img"].height,
            },
            "sourceSize": {"w": rec["img"].width, "h": rec["img"].height},
            "originalName": rec["base"],
            "color": rec["colour"],
            "hex": (
                palette.get(rec["colour"]) or  # named entry
                rgb_to_hex(average_visible_rgb(rec["img"]))  # fallback
            ),
            **{
                k: rec["cfg"].get(k)
                for k in (
                    "name",
                    "description",
                    "cost",
                    "category",
                    "rarity",
                    "unlock_level",
                    "generate_colors",
                )
            },
        }

    sheet.save(out_png)
    with open(out_json, "w") as fh:
        json.dump(meta, fh, indent=2)

    print(f"✅ {len(records)} sprites ➜ {out_png} ({sheet_w}×{sheet_h}) + {out_json}")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: sprite_sheet_gen.py <folder> [cfg] [out.png] [out.json]")
        sys.exit(1)

    inp = Path(sys.argv[1])
    cfg = sys.argv[2] if len(sys.argv) > 2 else "sprites.yaml"
    out_png = sys.argv[3] if len(sys.argv) > 3 else "spritesheet.png"
    out_json = sys.argv[4] if len(sys.argv) > 4 else "spritesheet.json"

    make_sheet(inp, cfg, out_png, out_json)
