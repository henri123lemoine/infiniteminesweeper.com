
#!/usr/bin/env -S uv run --script
# /// script
# requires-python = "~=3.12"
# dependencies = [
#   "pillow",
# ]
# ///

from PIL import Image

im = Image.open('/Users/henrilemoine/Downloads/abkhazia.png')
print(im.size)
