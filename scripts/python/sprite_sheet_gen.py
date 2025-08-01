#!/usr/bin/env -S uv run --script
# /// script
# requires-python = "~=3.12"
# dependencies = [
#   "pillow",
#   "pathlib",
#   "pyyaml",
# ]
# ///

import os
import json
import colorsys
import yaml
from PIL import Image, ImageEnhance
from pathlib import Path
import numpy as np

def load_sprite_config(config_path):
    """Load sprite configuration from YAML file."""
    if not os.path.exists(config_path):
        print(f"⚠️  Config file not found: {config_path}")
        print("Creating example config file...")

        # Create example config
        example_config = {
            'default_settings': {
                'generate_colors': True,
                'cost': 0,
                'description': 'A basic flag',
                'category': 'standard'
            },
            'color_palette': [
                0,    # Red
                30,   # Orange
                60,   # Yellow
                120,  # Green
                180,  # Cyan
                210,  # Light Blue
                240,  # Blue
                270,  # Purple
                300,  # Magenta
                330,  # Pink
                0,    # Light Gray (will be converted to ~210 with low saturation)
            ],
            'sprites': {
                'flag1': {
                    'name': 'Wavy Flag',
                    'description': 'A flag that waves in the wind',
                    'cost': 10,
                    'category': 'animated',
                    'generate_colors': True
                },
                'flag2': {
                    'name': 'Simple Flag',
                    'description': 'A basic rectangular flag',
                    'cost': 5,
                    'category': 'basic',
                    'generate_colors': True
                },
                'flag14dragoneye': {
                    'name': 'Dragon Eye Flag',
                    'description': 'A mystical flag with dragon eyes',
                    'cost': 50,
                    'category': 'special',
                    'generate_colors': False,  # Don't color-shift this one
                    'rarity': 'legendary'
                }
            }
        }

        with open(config_path, 'w') as f:
            yaml.dump(example_config, f, default_flow_style=False, indent=2)

        print(f"✅ Created example config: {config_path}")
        return example_config

    with open(config_path, 'r') as f:
        return yaml.safe_load(f)

def create_color_variations(image, base_hue_range=(0, 20), target_hues=None):
    """
    Create color variations of an image by shifting hues.
    """
    if target_hues is None:
        target_hues = [0, 30, 60, 120, 180, 210, 240, 270, 300, 330, 'light_gray']

    # Convert PIL image to numpy array
    img_array = np.array(image)

    if img_array.shape[2] == 4:  # RGBA
        rgb = img_array[:, :, :3]
        alpha = img_array[:, :, 3]
    else:  # RGB
        rgb = img_array
        alpha = None

    # Convert RGB to HSV
    rgb_normalized = rgb / 255.0
    hsv = np.zeros_like(rgb_normalized)

    for i in range(rgb_normalized.shape[0]):
        for j in range(rgb_normalized.shape[1]):
            r, g, b = rgb_normalized[i, j]
            h, s, v = colorsys.rgb_to_hsv(r, g, b)
            hsv[i, j] = [h * 360, s, v]  # Convert hue to degrees

    variations = []

    for target_hue in target_hues:
        # Create a copy of HSV
        new_hsv = hsv.copy()

        # Find pixels in the red hue range and shift them
        hue_mask = ((hsv[:, :, 0] >= base_hue_range[0]) & (hsv[:, :, 0] <= base_hue_range[1])) | \
                   ((hsv[:, :, 0] >= 340) & (hsv[:, :, 0] <= 360))  # Handle wrap-around for red

        # Also include darker reds (burgundy, etc.) by checking saturation and value
        red_like_mask = (
            (hue_mask) |  # Pure reds
            ((hsv[:, :, 0] >= 340) | (hsv[:, :, 0] <= 40)) & (hsv[:, :, 1] > 0.3)  # Red-ish colors with decent saturation
        )

        if target_hue == 'light_gray':
            # For light gray, desaturate and lighten
            new_hsv[red_like_mask, 0] = 210  # Slight blue tint
            new_hsv[red_like_mask, 1] *= 0.1  # Almost no saturation
            new_hsv[red_like_mask, 2] = np.minimum(new_hsv[red_like_mask, 2] * 1.3, 0.9)  # Lighten but not pure white
        else:
            # Shift the hue
            new_hsv[red_like_mask, 0] = target_hue

        # Convert back to RGB
        new_rgb = np.zeros_like(rgb_normalized)
        for i in range(new_hsv.shape[0]):
            for j in range(new_hsv.shape[1]):
                h, s, v = new_hsv[i, j]
                r, g, b = colorsys.hsv_to_rgb(h / 360.0, s, v)  # Convert hue back to 0-1 range
                new_rgb[i, j] = [r, g, b]

        # Convert back to 0-255 range
        new_rgb_int = (new_rgb * 255).astype(np.uint8)

        # Reconstruct the image
        if alpha is not None:
            new_img_array = np.dstack([new_rgb_int, alpha])
        else:
            new_img_array = new_rgb_int

        new_image = Image.fromarray(new_img_array)
        variations.append((new_image, target_hue))

    return variations

def get_color_name(hue):
    """Convert hue to a readable color name."""
    if hue == 'light_gray':
        return 'lightgray'

    color_names = {
        0: "red", 30: "orange", 60: "yellow", 90: "lime", 120: "green",
        150: "teal", 180: "cyan", 210: "lightblue", 240: "blue", 270: "purple",
        300: "magenta", 330: "pink"
    }

    # Find closest color name
    closest_hue = min(color_names.keys(), key=lambda x: abs(x - hue))
    return color_names[closest_hue]

def create_sprite_sheet_with_config(input_folder, config_path="sprites.yaml",
                                  output_image="spritesheet.png", output_json="spritesheet.json"):
    """
    Creates sprite sheet using configuration file for metadata and color settings.
    """

    # Load configuration
    print(f"Loading config from {config_path}…")
    config = load_sprite_config(config_path)
    default_settings = config.get('default_settings', {})
    sprite_configs = config.get('sprites', {})
    color_palette = config.get('color_palette', [0, 30, 60, 120, 180, 210, 240, 270, 300, 330, 'light_gray'])

    # Get all image files
    image_extensions = {'.png', '.jpg', '.jpeg', '.gif', '.bmp'}
    image_files = []

    for file_path in Path(input_folder).iterdir():
        if file_path.suffix.lower() in image_extensions:
            image_files.append(file_path)

    if not image_files:
        print("No image files found!")
        return

    # Load images and create variations
    all_sprites = []

    for file_path in image_files:
        img = Image.open(file_path)
        base_name = file_path.stem

        # Get sprite-specific config, fall back to defaults
        sprite_config = sprite_configs.get(base_name, {})

        # Merge with defaults
        final_config = {**default_settings, **sprite_config}

        should_generate_colors = final_config.get('generate_colors', True)

        if should_generate_colors:
            variations = create_color_variations(img, target_hues=color_palette)

            for var_img, hue in variations:
                color_name = get_color_name(hue)
                sprite_name = f"{base_name}_{color_name}" if (hue != 0 and hue != color_palette[0]) else base_name

                all_sprites.append({
                    'image': var_img,
                    'name': sprite_name,
                    'width': var_img.width,
                    'height': var_img.height,
                    'original_name': base_name,
                    'color': color_name,
                    'hue': hue,
                    'config': final_config
                })
        else:
            # Original only, no color variations
            all_sprites.append({
                'image': img,
                'name': base_name,
                'width': img.width,
                'height': img.height,
                'original_name': base_name,
                'color': 'original',
                'hue': 0,
                'config': final_config
            })

    print(f"📊 Generated {len(all_sprites)} sprites from {len(image_files)} original images")

    # Calculate sprite sheet dimensions
    if not all_sprites:
        print("No sprites to process!")
        return

    max_width = max(sprite['width'] for sprite in all_sprites)
    max_height = max(sprite['height'] for sprite in all_sprites)

    # Calculate grid size
    import math
    grid_size = math.ceil(math.sqrt(len(all_sprites)))
    sheet_width = grid_size * max_width
    sheet_height = grid_size * max_height

    # Create sprite sheet
    sprite_sheet = Image.new('RGBA', (sheet_width, sheet_height), (0, 0, 0, 0))

    # Place images and collect metadata
    metadata = {
        'texture': output_image,
        'frames': {},
        'meta': {
            'size': {'w': sheet_width, 'h': sheet_height},
            'scale': '1',
            'color_palette': color_palette,
            'total_sprites': len(all_sprites),
            'original_count': len(image_files)
        }
    }

    for i, sprite_data in enumerate(all_sprites):
        # Calculate position in grid
        col = i % grid_size
        row = i // grid_size
        x = col * max_width
        y = row * max_height

        # Paste image (centered in its cell)
        img = sprite_data['image']
        offset_x = x + (max_width - img.width) // 2
        offset_y = y + (max_height - img.height) // 2

        sprite_sheet.paste(img, (offset_x, offset_y), img if img.mode == 'RGBA' else None)

        # Add to metadata with all config info
        sprite_config = sprite_data['config']
        metadata['frames'][sprite_data['name']] = {
            'frame': {
                'x': offset_x,
                'y': offset_y,
                'w': img.width,
                'h': img.height
            },
            'rotated': False,
            'trimmed': False,
            'spriteSourceSize': {
                'x': 0,
                'y': 0,
                'w': img.width,
                'h': img.height
            },
            'sourceSize': {
                'w': img.width,
                'h': img.height
            },
            'originalName': sprite_data['original_name'],
            'color': sprite_data['color'],
            'hue': sprite_data['hue'],
            # Include all the config metadata
            'displayName': sprite_config.get('name', sprite_data['original_name']),
            'description': sprite_config.get('description', 'A flag'),
            'cost': sprite_config.get('cost', 0),
            'category': sprite_config.get('category', 'standard'),
            'rarity': sprite_config.get('rarity', 'common'),
            'unlockLevel': sprite_config.get('unlock_level', 1),
            'generateColors': sprite_config.get('generate_colors', True)
        }

        img.close()

    # Save sprite sheet and metadata
    sprite_sheet.save(output_image)

    with open(output_json, 'w') as f:
        json.dump(metadata, f, indent=2)

    print(f"✅ Created sprite sheet: {output_image}")
    print(f"✅ Created metadata: {output_json}")
    print(f"📏 Final sprite sheet size: {sheet_width}x{sheet_height}")

    # Print summary by category
    categories = {}
    for sprite in all_sprites:
        cat = sprite['config'].get('category', 'standard')
        categories[cat] = categories.get(cat, 0) + 1

    print("📂 Sprites by category:")
    for cat, count in categories.items():
        print(f"   {cat}: {count}")

if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("Usage: python sprite_generator.py <input_folder> [config_file] [output_image] [output_json]")
        print("Example: python sprite_generator.py ./sprites sprites.yaml spritesheet.png spritesheet.json")
        sys.exit(1)

    input_folder = sys.argv[1]
    config_path = sys.argv[2] if len(sys.argv) > 2 else "sprites.yaml"
    output_image = sys.argv[3] if len(sys.argv) > 3 else "spritesheet.png"
    output_json = sys.argv[4] if len(sys.argv) > 4 else "spritesheet.json"

    create_sprite_sheet_with_config(input_folder, config_path, output_image, output_json)
