package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	previewRegionChunks = 96
	previewPanelGap     = 8
	previewMaxSide      = 8192
)

// chunkRect is an inclusive rectangle of chunk coordinates.
type chunkRect struct{ X0, Y0, X1, Y1 int64 }

func (r chunkRect) contains(cid ChunkID) bool {
	return cid.X >= r.X0 && cid.X <= r.X1 && cid.Y >= r.Y0 && cid.Y <= r.Y1
}

func parseChunkRect(s string) (chunkRect, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return chunkRect{}, fmt.Errorf("want x0,y0,x1,y1 in chunk coordinates, got %q", s)
	}
	var v [4]int64
	for i, p := range parts {
		n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			return chunkRect{}, fmt.Errorf("bad chunk coordinate %q", p)
		}
		v[i] = n
	}
	r := chunkRect{min(v[0], v[2]), min(v[1], v[3]), max(v[0], v[2]), max(v[1], v[3])}
	return r, nil
}

func chunksBounds(chunks map[ChunkID]*ChunkBits) (chunkRect, bool) {
	var r chunkRect
	first := true
	for cid := range chunks {
		if first {
			r, first = chunkRect{cid.X, cid.Y, cid.X, cid.Y}, false
			continue
		}
		r.X0, r.X1 = min(r.X0, cid.X), max(r.X1, cid.X)
		r.Y0, r.Y1 = min(r.Y0, cid.Y), max(r.Y1, cid.Y)
	}
	return r, !first
}

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

// renderPurge draws rect three times: before, after, and after with removed
// blocks in red. One pixel is one 8×8 block, or a square of them when rect
// is wider than previewMaxSide blocks, brighter the more of it is revealed.
func renderPurge(before, after map[ChunkID]*ChunkBits, rect chunkRect) (imgs [3]*image.RGBA, blocksPerPixelSide int64) {
	blocksW := (rect.X1 - rect.X0 + 1) * territoryBlocksSide
	blocksH := (rect.Y1 - rect.Y0 + 1) * territoryBlocksSide
	scale := (max(blocksW, blocksH) + previewMaxSide - 1) / previewMaxSide
	w, h := int((blocksW+scale-1)/scale), int((blocksH+scale-1)/scale)

	type px struct{ before, after, removed int32 }
	pixels := make([]px, w*h)
	for cid, b := range before {
		if !rect.contains(cid) {
			continue
		}
		a := after[cid]
		for blk := range territoryBlocksTotal {
			x := ((cid.X-rect.X0)*territoryBlocksSide + int64(blk%territoryBlocksSide)) / scale
			y := ((cid.Y-rect.Y0)*territoryBlocksSide + int64(blk/territoryBlocksSide)) / scale
			p := &pixels[int(y)*w+int(x)]
			nb, na := int32(blockRevealed(b, blk)), int32(0)
			if a != nil {
				na = int32(blockRevealed(a, blk))
			}
			p.before += nb
			p.after += na
			if na < nb {
				p.removed++
			}
		}
	}

	cellsPerPixel := int32(scale * scale * territoryBlockSize * territoryBlockSize)
	mix := func(lo, hi color.RGBA, num, den int32) color.RGBA {
		ch := func(l, h uint8) uint8 { return uint8(int32(l) + (int32(h)-int32(l))*num/den) }
		return color.RGBA{ch(lo.R, hi.R), ch(lo.G, hi.G), ch(lo.B, hi.B), 255}
	}
	hidden, revealed := color.RGBA{20, 22, 26, 255}, color.RGBA{205, 210, 218, 255}
	removedColor := color.RGBA{230, 60, 50, 255}
	for i := range imgs {
		imgs[i] = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	for i, p := range pixels {
		x, y := i%w, i/w
		afterShade := mix(hidden, revealed, p.after, cellsPerPixel)
		imgs[0].SetRGBA(x, y, mix(hidden, revealed, p.before, cellsPerPixel))
		imgs[1].SetRGBA(x, y, afterShade)
		// A pixel with any removal is plain red: a faint blend would hide
		// small removals on a zoomed-out map.
		if p.removed > 0 {
			afterShade = removedColor
		}
		imgs[2].SetRGBA(x, y, afterShade)
	}
	return imgs, scale
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// writePurgeRegions draws every 96×96-chunk region the purge changes as
// region_<x>_<y>.png, named by the region's top-left chunk, with the three
// renderings side by side at full block resolution.
func writePurgeRegions(dir string, before, after map[ChunkID]*ChunkBits) (int, error) {
	changed := make(map[chunkRect]struct{})
	for cid, b := range before {
		if a := after[cid]; a == nil || *a != *b {
			x := floorDiv(cid.X, previewRegionChunks) * previewRegionChunks
			y := floorDiv(cid.Y, previewRegionChunks) * previewRegionChunks
			changed[chunkRect{x, y, x + previewRegionChunks - 1, y + previewRegionChunks - 1}] = struct{}{}
		}
	}
	const side = previewRegionChunks * territoryBlocksSide
	for rect := range changed {
		panels, _ := renderPurge(before, after, rect)
		img := image.NewRGBA(image.Rect(0, 0, 3*side+2*previewPanelGap, side))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
		for i, p := range panels {
			at := image.Pt(i*(side+previewPanelGap), 0)
			draw.Draw(img, p.Bounds().Add(at), p, image.Point{}, draw.Src)
		}
		if err := writePNG(filepath.Join(dir, fmt.Sprintf("region_%d_%d.png", rect.X0, rect.Y0)), img); err != nil {
			return 0, err
		}
	}
	return len(changed), nil
}

// writePurgeMap draws rect as map_before.png, map_after.png and
// map_removed.png: same size, so they can be flipped between.
func writePurgeMap(dir string, before, after map[ChunkID]*ChunkBits, rect chunkRect) (blocksPerPixelSide int64, err error) {
	imgs, scale := renderPurge(before, after, rect)
	for i, name := range []string{"map_before.png", "map_after.png", "map_removed.png"} {
		if err := writePNG(filepath.Join(dir, name), imgs[i]); err != nil {
			return 0, err
		}
	}
	return scale, nil
}
