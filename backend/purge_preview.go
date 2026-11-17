package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

const (
	previewRegionChunks = 96
	previewPanelGap     = 8
)

func cloneChunks(chunks map[ChunkID]*ChunkBits) map[ChunkID]*ChunkBits {
	out := make(map[ChunkID]*ChunkBits, len(chunks))
	for cid, cb := range chunks {
		if cb != nil {
			c := *cb
			out[cid] = &c
		}
	}
	return out
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// writePurgePreview draws every 96×96-chunk region the purge changes as
// region_<x>_<y>.png, named by the region's top-left chunk. Each holds three
// panels: before, after, and after with the removed blocks in red. One pixel
// is one 8×8 block, brighter the more of it is revealed.
func writePurgePreview(dir string, before, after map[ChunkID]*ChunkBits) (int, error) {
	type region struct{ X, Y int64 }
	regionOf := func(cid ChunkID) region {
		return region{floorDiv(cid.X, previewRegionChunks), floorDiv(cid.Y, previewRegionChunks)}
	}
	changed := make(map[region]struct{})
	for cid, b := range before {
		if a := after[cid]; a == nil || *a != *b {
			changed[regionOf(cid)] = struct{}{}
		}
	}
	if len(changed) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}

	const side = previewRegionChunks * territoryBlocksSide
	const cellsPerBlock = territoryBlockSize * territoryBlockSize
	shade := func(n int) color.RGBA {
		mix := func(lo, hi int) uint8 { return uint8(lo + (hi-lo)*n/cellsPerBlock) }
		return color.RGBA{mix(20, 205), mix(22, 210), mix(26, 218), 255}
	}
	removedColor := color.RGBA{230, 60, 50, 255}

	imgs := make(map[region]*image.RGBA, len(changed))
	for r := range changed {
		img := image.NewRGBA(image.Rect(0, 0, 3*side+2*previewPanelGap, side))
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
		for panel := range 3 {
			x0 := panel * (side + previewPanelGap)
			for y := range side {
				for x := range side {
					img.SetRGBA(x0+x, y, shade(0))
				}
			}
		}
		imgs[r] = img
	}
	for cid, b := range before {
		r := regionOf(cid)
		img := imgs[r]
		if img == nil {
			continue
		}
		a := after[cid]
		ox := int(cid.X-r.X*previewRegionChunks) * territoryBlocksSide
		oy := int(cid.Y-r.Y*previewRegionChunks) * territoryBlocksSide
		for blk := range territoryBlocksTotal {
			x, y := ox+blk%territoryBlocksSide, oy+blk/territoryBlocksSide
			nb, na := blockRevealed(b, blk), 0
			if a != nil {
				na = blockRevealed(a, blk)
			}
			img.SetRGBA(x, y, shade(nb))
			img.SetRGBA(side+previewPanelGap+x, y, shade(na))
			third := shade(na)
			if na < nb {
				third = removedColor
			}
			img.SetRGBA(2*(side+previewPanelGap)+x, y, third)
		}
	}

	for r, img := range imgs {
		name := fmt.Sprintf("region_%d_%d.png", r.X*previewRegionChunks, r.Y*previewRegionChunks)
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return 0, err
		}
		err = png.Encode(f, img)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return 0, err
		}
	}
	return len(imgs), nil
}
