import os
import subprocess
import sys

import requests
from bs4 import BeautifulSoup

# Base URL of the PixelFlags page
BASE_URL = "https://r74n.com/pixelflags/"
URL = BASE_URL

# Directories for raw inputs and generated outputs
DOWNLOAD_DIR = "flags_raw"
OUTPUT_DIR = "flags_32x32"

# Path to the makeflag.sh script
SCRIPT_PATH = os.path.join(os.path.abspath(os.path.dirname(__file__)), "makeflag.sh")


def ensure_dir(path):
    if not os.path.exists(path):
        os.makedirs(path)


def fetch_html(url):
    resp = requests.get(url)
    resp.raise_for_status()
    return resp.text


def parse_all_flags(html):
    soup = BeautifulSoup(html, "html.parser")
    content = soup.find("div", class_="content")
    if not content:
        print("ERROR: Couldn't find content div")
        sys.exit(1)

    imgs = content.find_all("img")
    srcs = [img["src"] for img in imgs if img.get("src")]
    # Deduplicate while preserving order
    seen = set()
    unique = []
    for src in srcs:
        if src not in seen:
            seen.add(src)
            unique.append(src)
    return unique


def download_image(src, download_dir):
    full_url = requests.compat.urljoin(BASE_URL, src)
    # Prefix category to filename to avoid collisions
    parts = src.split("/")
    if len(parts) >= 2:
        category = parts[-2]
        filename = parts[-1]
        local_name = f"{category}_{filename}"
    else:
        local_name = parts[-1]
    local_path = os.path.join(download_dir, local_name)

    if not os.path.exists(local_path):
        print(f"Downloading {full_url} → {local_path}")
        r = requests.get(full_url)
        r.raise_for_status()
        with open(local_path, "wb") as f:
            f.write(r.content)
    return local_path


def generate_flag(input_path, output_dir):
    base = os.path.basename(input_path)
    name, ext = os.path.splitext(base)
    output_path = os.path.join(output_dir, f"{name}-32x32{ext}")
    print(f"Generating 32×32: {input_path} → {output_path}")
    subprocess.run(["bash", SCRIPT_PATH, input_path, output_path], check=True)


def main():
    ensure_dir(DOWNLOAD_DIR)
    ensure_dir(OUTPUT_DIR)

    html = fetch_html(URL)
    srcs = parse_all_flags(html)

    for src in srcs:
        inp = download_image(src, DOWNLOAD_DIR)
        generate_flag(inp, OUTPUT_DIR)

    print("All flags processed!")


if __name__ == "__main__":
    main()
