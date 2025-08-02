# script.sh
#!/usr/bin/env bash
set -euo pipefail

# Directory where this script lives (and where pixel_resize.py / pixel_graft.py live)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESIZER="$SCRIPT_DIR/pixel_resize.py"
GRAFTER="$SCRIPT_DIR/pixel_graft.py"
# Adjust this if your frontend path is different:
BG="$SCRIPT_DIR/../../frontend/assets/raw/flag0.png"

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 INPUT_32x18.png OUTPUT_32x32.png"
  exit 1
fi

INPUT="$1"
OUTPUT="$2"
TMP="$(mktemp /tmp/flagXXXXXX)".png

# 1) Resize down to 25×18
uv run --script "$RESIZER" -i "$INPUT" -o "$TMP" -w 25 -H 18

# 2) Graft + draw border, default offset (5,5)
uv run --script "$GRAFTER" -b "$BG" -i "$TMP" -o "$OUTPUT" --x 5 --y 5

rm "$TMP"
echo "✅ Done: $OUTPUT"
