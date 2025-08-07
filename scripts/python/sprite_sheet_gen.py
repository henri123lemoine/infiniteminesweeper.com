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
and a YAML description file with stable IDs and category-based defaults.

Key improvements:
- Explicit stable IDs for each sprite variant
- Category-based default settings
- Robust sprite ordering and ID management
- Easy extensibility for new sprite types
"""

import colorsys
import json
import math
import re
import sys
from pathlib import Path
from typing import Dict, List, Tuple, Any, Optional

import numpy as np
import yaml
from PIL import Image, ImageDraw

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

### Category-based defaults
CATEGORY_DEFAULTS = {
    "flag": {
        "generate_colors": True,
        "unlocked": True,
        "wavy": False,
        "broken": False,
        "pointing": "side",
        "cost": 0,
    },
    "mine": {
        "generate_colors": False,
        "unlocked": True,
        "cost": 0,
    },
    "advancement": {
        "generate_colors": False,
        "unlocked": False,
        "cost": 0,
    },
    # Add more categories as needed
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

def draw_3d_cell(width: int, height: int) -> Image.Image:
    """Draws a 3D-style unrevealed cell background, similar to CanvasRenderer.js."""
    img = Image.new("RGB", (width, height))
    draw = ImageDraw.Draw(img)
    size = min(width, height)
    borderWidth = max(1, round(size * 0.08))

    # Mimic the JS version for an "unrevealed" cell's 3D appearance
    # Top and left highlights
    draw.rectangle((0, 0, width, borderWidth), fill="#d4d4d4")
    draw.rectangle((0, 0, borderWidth, height), fill="#d4d4d4")

    # Bottom and right shadows
    draw.rectangle((borderWidth, height - borderWidth, width, height), fill="#808080")
    draw.rectangle((width - borderWidth, borderWidth, width, height), fill="#808080")

    # Inner area
    draw.rectangle(
        (borderWidth, borderWidth, width - borderWidth, height - borderWidth),
        fill="#c0c0c0",
    )

    # Dark corner for depth
    draw.rectangle(
        (width - borderWidth, height - borderWidth, width, height), fill="#606060"
    )

    return img

### Pre-compute palette hues
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

def get_sprite_defaults(category: str) -> Dict[str, Any]:
    """Get default settings for a sprite category."""
    return CATEGORY_DEFAULTS.get(category, {
        "generate_colors": False,
        "unlocked": True,
        "cost": 0,
    })

def resolve_sprite_config(base_name: str, sprite_config: Dict[str, Any]) -> Dict[str, Any]:
    """Resolve final sprite configuration with category-based defaults."""
    category = sprite_config.get("category", "unknown")

    # Start with category defaults
    final_config = get_sprite_defaults(category).copy()

    # Override with sprite-specific settings
    final_config.update(sprite_config)

    # Ensure category is set
    final_config["category"] = category

    return final_config

class SpriteRecord:
    """Represents a single sprite variant with stable ID."""
    def __init__(self, sprite_id: int, base_name: str, color_name: str,
                 image: Image.Image, config: Dict[str, Any]):
        self.sprite_id = sprite_id
        self.base_name = base_name
        self.color_name = color_name
        self.image = image
        self.config = config

    @property
    def display_name(self) -> str:
        """Human-readable name for this sprite variant."""
        base_name = self.config.get("name", self.base_name)
        if self.color_name == "Original" or not self.config.get("generate_colors", False):
            return base_name
        return f"{self.color_name} {base_name}"

### ID Management System

class IDManager:
    """Manages stable sprite IDs across spritesheet generations."""

    def __init__(self, id_file: str = "sprite_ids.yaml"):
        self.id_file = id_file
        self.id_map: Dict[str, int] = {}
        self.next_id = 1
        self.load_ids()

    def load_ids(self):
        """Load existing ID mappings."""
        if Path(self.id_file).exists():
            with open(self.id_file, 'r') as f:
                data = yaml.safe_load(f) or {}
                self.id_map = data.get('mappings', {})
                self.next_id = data.get('next_id', 1)

    def save_ids(self):
        """Save ID mappings to file."""
        with open(self.id_file, 'w') as f:
            yaml.dump({
                'mappings': self.id_map,
                'next_id': self.next_id,
            }, f, default_flow_style=False)

    def get_or_create_id(self, sprite_key: str) -> int:
        """Get existing ID or create new one for sprite variant."""
        if sprite_key in self.id_map:
            return self.id_map[sprite_key]

        # Create new ID
        new_id = self.next_id
        self.id_map[sprite_key] = new_id
        self.next_id += 1
        return new_id

    def make_sprite_key(self, base_name: str, color_name: str,
                       generate_colors: bool) -> str:
        """Create a stable key for a sprite variant."""
        if not generate_colors or color_name == "Original":
            return base_name
        return f"{base_name}_{slug(color_name)}"

### Sprite-sheet packing

def make_sheet(
    input_dir: Path,
    cfg_path: str = "sprites.yaml",
    out_png: str = "spritesheet.png",
    out_json: str = "spritesheet.json",
    id_file: str = "sprite_ids.yaml"
):
    cfg = load_config(cfg_path)
    sprite_cfgs: dict = cfg.get("sprites", {})
    id_manager = IDManager(id_file)

    images = [
        p for p in input_dir.iterdir() if p.suffix.lower() in {".png", ".jpg", ".jpeg"}
    ]
    if not images:
        print("❌ No image files in", input_dir)
        return

    # Sort images by name for consistent ordering
    images.sort(key=lambda p: p.name)

    records: List[SpriteRecord] = []

    for img_path in images:
        base_name = img_path.stem
        raw_config = sprite_cfgs.get(base_name, {})

        # Resolve configuration with category defaults
        final_config = resolve_sprite_config(base_name, raw_config)

        with Image.open(img_path) as original:
            if final_config.get("generate_colors", False):
                variants = colour_variants(original)
            else:
                variants = [(original.copy(), "Original")]

            for var_img, color_name in variants:
                sprite_key = id_manager.make_sprite_key(
                    base_name, color_name, final_config.get("generate_colors", False)
                )
                sprite_id = id_manager.get_or_create_id(sprite_key)

                records.append(SpriteRecord(
                    sprite_id=sprite_id,
                    base_name=base_name,
                    color_name=color_name,
                    image=var_img,
                    config=final_config
                ))

    # Sort records by ID for consistent spritesheet layout
    records.sort(key=lambda r: r.sprite_id)

    # Save updated ID mappings
    id_manager.save_ids()

    # Create spritesheet
    w_max = max(r.image.width for r in records)
    h_max = max(r.image.height for r in records)
    grid = math.ceil(math.sqrt(len(records)))
    sheet_w, sheet_h = grid * w_max, grid * h_max
    sheet = Image.new("RGBA", (sheet_w, sheet_h), (0, 0, 0, 0))
    grey_sheet = Image.new("RGBA", (sheet_w, sheet_h))

    meta = {
        "texture": out_png,
        "frames": {},
        "meta": {
            "size": {"w": sheet_w, "h": sheet_h},
            "scale": "1",
            "palette": palette,
            "total_sprites": len(records),
            "categories": list(CATEGORY_DEFAULTS.keys()),
            "id_file": id_file,
        },
    }

    for idx, record in enumerate(records):
        col, row = idx % grid, idx // grid
        x, y = col * w_max, row * h_max
        ox = x + (w_max - record.image.width) // 2
        oy = y + (h_max - record.image.height) // 2
        sheet.paste(record.image, (ox, oy), record.image)

        # Create grey background version
        cell_bg = draw_3d_cell(w_max, h_max)
        grey_sheet.paste(cell_bg, (x, y))
        grey_sheet.paste(record.image, (ox, oy), record.image)

        # Create frame metadata
        frame_data = {
            "id": record.sprite_id,  # Stable uint32 ID
            "frame": {"x": ox, "y": oy, "w": record.image.width, "h": record.image.height},
            "rotated": False,
            "trimmed": False,
            "spriteSourceSize": {
                "x": 0,
                "y": 0,
                "w": record.image.width,
                "h": record.image.height,
            },
            "sourceSize": {"w": record.image.width, "h": record.image.height},
            "baseName": record.base_name,
            "colorName": record.color_name,
            "displayName": record.display_name,
            "hex": (
                palette.get(record.color_name) or
                rgb_to_hex(average_visible_rgb(record.image))
            ),
        }

        # Add all config properties
        frame_data.update(record.config)

        # Store by both ID and legacy string key for backwards compatibility
        meta["frames"][str(record.sprite_id)] = frame_data

        # Also store by legacy key if different
        legacy_key = id_manager.make_sprite_key(
            record.base_name, record.color_name,
            record.config.get("generate_colors", False)
        )
        if legacy_key != str(record.sprite_id):
            meta["frames"][legacy_key] = frame_data

    sheet.save(out_png)
    with open(out_json, "w") as fh:
        json.dump(meta, fh, indent=2)

    out_path = Path(out_png)
    grey_out_path = out_path.parent / f"grey-{out_path.name}"
    grey_sheet.save(grey_out_path)

    print(f"✅ {len(records)} sprites ➜ {out_png} ({sheet_w}×{sheet_h}) + {out_json}")
    print(f"✅ Grey background version ➜ {grey_out_path}")
    print(f"✅ ID mappings saved to {id_file}")
    print(f"✅ Categories: {', '.join(CATEGORY_DEFAULTS.keys())}")

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: sprite_sheet_gen.py <folder> [cfg] [out.png] [out.json] [id_file]")
        sys.exit(1)

    inp = Path(sys.argv[1])
    cfg = sys.argv[2] if len(sys.argv) > 2 else "sprites.yaml"
    out_png = sys.argv[3] if len(sys.argv) > 3 else "spritesheet.png"
    out_json = sys.argv[4] if len(sys.argv) > 4 else "spritesheet.json"
    id_file = sys.argv[5] if len(sys.argv) > 5 else "sprite_ids.yaml"

    make_sheet(inp, cfg, out_png, out_json, id_file)
