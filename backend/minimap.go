package main

import (
	"bytes"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// MinimapTile tracks which cells in a chunk have pending updates for minimap
// subscribers. Palette values are always recomputed from authoritative world
// state on demand — caching them would roughly 16× the memory budget for a
// fully-zoomed-out player without adding correctness.
type MinimapTile struct {
	Version uint32
	// Dirty bitset at full 64×64 resolution: 4096 bits / 64 uint64 = 512 bytes.
	// A 1 bit means the cell changed since the last broadcast for at least one
	// subscribed resolution.
	Dirty [64]uint64
}

// palette indices (<=10 colors total)
const (
	mmUnseen   = 0 // background
	mmMine     = 1 // revealed mine
	mmEmpty0   = 2 // revealed no-adjacent
	mmNum1     = 3 // numbers map 1..6 → 3..8
	mmNum2     = 4
	mmNum3     = 5
	mmNum4     = 6
	mmNum5     = 7
	mmNum6     = 8
	mmNum7Plus = 9 // 7 or 8

	mmFlagBase = 10 // 10 buckets for flags: 10..19
)

// Maximum number of minimap tile subscriptions per player. Measured costs:
//   - ~163 bytes of sub-map overhead per subscription (see TestSubscriptionOnlyMemory)
//   - ~560 bytes per *active* tile (only allocated when cells actually change
//     in that chunk — zoomed-out views mostly hit all-unseen chunks that never
//     allocate a tile)
//
// 100k subs = ~16 MB of sub-map overhead per player, which is fine even with
// ten players simultaneously pathologically zoomed out. The cap exists as a
// DoS guardrail for clients that send millions of tiles in one request — not
// as a UX-limiting throttle. The frontend's max overlay zoom (0.125 at 1920×
// 1080) requests ~32 k tiles, so 100 k is 3× real-world demand.
const maxMinimapSubsPerPlayer = 100000

func (t *MinimapTile) clearDirty() { clear(t.Dirty[:]) }

func (t *MinimapTile) setDirty(pos uint32) {
	if pos >= 4096 {
		return
	}
	t.Dirty[pos>>6] |= uint64(1) << (pos & 63)
}

func (t *MinimapTile) hasAnyDirty() bool {
	for _, w := range t.Dirty {
		if w != 0 {
			return true
		}
	}
	return false
}

// deriveDirtyAtResolution OR-reduces the 64-res Dirty bitset into a dense
// []bool sized for the requested output resolution. Block size is 64/res
// (1, 2, or 4); each output cell is 1 if any source cell in its block is
// dirty.
func (t *MinimapTile) deriveDirtyAtResolution(res uint32) []bool {
	out := make([]bool, res*res)
	block := int(64 / res)
	for by := 0; by < int(res); by++ {
		for bx := 0; bx < int(res); bx++ {
			for dy := 0; dy < block; dy++ {
				sy := by*block + dy
				rowBase := sy * 64
				for dx := 0; dx < block; dx++ {
					pos := rowBase + bx*block + dx
					if t.Dirty[pos>>6]&(uint64(1)<<(uint(pos)&63)) != 0 {
						out[by*int(res)+bx] = true
						goto next
					}
				}
			}
		next:
		}
	}
	return out
}

// hash64 is a small 64-bit mixer (xorshift/murmur-inspired) for deterministic PRNG
func hash64(x uint64) uint64 {
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

// probabilisticDownsample chooses a representative palette index for a lower-res pixel
// by sampling from the per-block histogram with probability proportional to counts.
// The RNG is deterministic per (chunk, blockX, blockY) so the image is stable over time.
func probabilisticDownsample(cid ChunkID, fullData []byte, blockX, blockY, blockSize int) byte {
	// Build histogram of palette indices in this block
	var counts [256]uint32
	total := uint32(0)
	for dy := 0; dy < blockSize; dy++ {
		sy := blockY*blockSize + dy
		if sy >= ChunkSize {
			continue
		}
		base := sy * ChunkSize
		for dx := 0; dx < blockSize; dx++ {
			sx := blockX*blockSize + dx
			if sx >= ChunkSize {
				continue
			}
			idx := fullData[base+sx]
			counts[idx]++
			total++
		}
	}
	if total == 0 {
		return mmUnseen
	}

	// Bayer 8x8 threshold matrix to decorrelate patterns across blocks (0..63)
	var bayer8 = [64]uint8{
		0, 32, 8, 40, 2, 34, 10, 42,
		48, 16, 56, 24, 50, 18, 58, 26,
		12, 44, 4, 36, 14, 46, 6, 38,
		60, 28, 52, 20, 62, 30, 54, 22,
		3, 35, 11, 43, 1, 33, 9, 41,
		51, 19, 59, 27, 49, 17, 57, 25,
		15, 47, 7, 39, 13, 45, 5, 37,
		63, 31, 55, 23, 61, 29, 53, 21,
	}

	// Deterministic base in [0,total)
	seed := hash64(uint64(cid.X)) ^ hash64((uint64(cid.Y) << 1)) ^ hash64((uint64(blockX) << 2)) ^ hash64((uint64(blockY) << 3))
	base := uint32(seed % uint64(total))
	// Deterministic offset from Bayer cell scaled to [0,total)
	bn := bayer8[(blockY&7)*8+(blockX&7)]
	offset := (uint32(bn) * total) >> 6 // divide by 64
	r := (base + offset) % total

	// Walk cumulative distribution to pick index proportionally to counts
	cum := uint32(0)
	for idx := 0; idx < len(counts); idx++ {
		c := counts[idx]
		if c == 0 {
			continue
		}
		cum += c
		if r < cum {
			return byte(idx)
		}
	}
	return mmUnseen
}

func minimapFlagBucket(flagID uint32) int {
	// Map sprite IDs to one of 10 color buckets.
	// Note: newer flag ID series are offset by +1 compared to the original 0..9 mapping.
	// Normalize so buckets align to: 0:light_gray, 1:red, 2:green, 3:blue, 4:yellow,
	// 5:orange, 6:purple, 7:cyan, 8:pink, 9:dark_gray.
	b := int(flagID % 10)
	if flagID >= 62 { // IDs from later series are shifted by +1 relative to early sets
		return (b + 9) % 10 // equivalent to (b - 1 + 10) % 10
	}
	return b
}

// chunkHasAnyState is a fast-path check: a chunk with no revealed cells and no
// flags renders as all-unseen and does not need computation.
func (s *Server) chunkHasAnyState(cid ChunkID) bool {
	if s.chunks[cid] != nil {
		for _, w := range *s.chunks[cid] {
			if w != 0 {
				return true
			}
		}
	}
	if len(s.flags[cid]) > 0 {
		return true
	}
	return false
}

// minimapMarkDirty marks a cell as changed in the tile's 64-res bitset. Caller
// must hold s.stateMu.
func (s *Server) minimapMarkDirty(cid ChunkID, cell uint32) {
	if len(s.minimapSubs[cid]) == 0 {
		return
	}
	t := s.getOrCreateMinimapTile(cid)
	t.setDirty(cell)
	s.minimapDirtyTiles[cid] = struct{}{}
}

func (s *Server) getOrCreateMinimapTile(cid ChunkID) *MinimapTile {
	t := s.minimapTiles[cid]
	if t == nil {
		t = &MinimapTile{}
		s.minimapTiles[cid] = t
	}
	return t
}

// computePaletteFor computes the palette index for a single cell directly from
// authoritative world state. Cheap — a couple of map lookups and a local seed
// lookup per isMine call.
//
// Palette layout depends on mmEmpty0..mmNum7Plus being consecutive: mmEmpty0
// (n=0) … mmNum6 (n=6), then mmNum7Plus for n ≥ 7. If you renumber the
// palette constants, update this accordingly.
func (s *Server) computePaletteFor(cid ChunkID, cell uint32) byte {
	if fl, ok := s.flags[cid][cell]; ok {
		return byte(mmFlagBase + minimapFlagBucket(fl.FlagID))
	}
	if !s.isCellRevealed(cid, cell) {
		return mmUnseen
	}
	if s.isMine(cid, cell) {
		return mmMine
	}
	n := s.countAdjacentMines(cid, cell)
	if n >= 7 {
		return mmNum7Plus
	}
	return byte(mmEmpty0 + n)
}

// computeFullTileData renders a chunk's full 64×64 palette from authoritative
// world state. Returns nil if the chunk is all unseen.
func (s *Server) computeFullTileData(cid ChunkID) []byte {
	if !s.chunkHasAnyState(cid) {
		return nil
	}
	data := make([]byte, ChunkSize*ChunkSize)
	for i := uint32(0); i < ChunkSize*ChunkSize; i++ {
		data[i] = s.computePaletteFor(cid, i)
	}
	return data
}

// computeTileDataAtResolution returns palette data sized for `res`. Returns nil
// when the chunk has no state (all-unseen).
func (s *Server) computeTileDataAtResolution(cid ChunkID, res uint32) []byte {
	full := s.computeFullTileData(cid)
	if full == nil {
		return nil
	}
	if res == 64 {
		return full
	}
	blockSize := int(64 / res)
	data := make([]byte, res*res)
	for y := 0; y < int(res); y++ {
		for x := 0; x < int(res); x++ {
			data[y*int(res)+x] = probabilisticDownsample(cid, full, x, y, blockSize)
		}
	}
	return data
}

// minimapOnReveal / minimapOnFlag are called from the reveal/flag logic under
// stateMu write-lock.
func (s *Server) minimapOnReveal(cid ChunkID, cell uint32) {
	s.minimapMarkDirty(cid, cell)
}

func (s *Server) minimapOnFlag(cid ChunkID, cell uint32) {
	s.minimapMarkDirty(cid, cell)
}

// collectRectsFromDirty groups dirty cells into rectangles of identical
// horizontal runs and writes the current palette byte for each. Caller supplies
// the data slice (already sized for resolution).
type deltaRect struct {
	x, y, w, h int
	rows       []byte
}

func collectRectsFromDirty(dirty []bool, data []byte, resolution int) (rects []deltaRect, deltaBytes int) {
	// Find horizontal runs per row
	type run struct{ x0, x1 int }
	runs := make([][]run, resolution)
	for y := 0; y < resolution; y++ {
		row := dirty[y*resolution : (y+1)*resolution]
		x := 0
		for x < resolution {
			for x < resolution && !row[x] {
				x++
			}
			if x >= resolution {
				break
			}
			start := x
			for x < resolution && row[x] {
				x++
			}
			runs[y] = append(runs[y], run{start, x - 1})
		}
	}
	// Merge identical runs vertically into rectangles
	used := make([][]bool, resolution)
	for y := 0; y < resolution; y++ {
		used[y] = make([]bool, len(runs[y]))
	}
	for y := 0; y < resolution; y++ {
		for i, r := range runs[y] {
			if used[y][i] {
				continue
			}
			h := 1
			for yy := y + 1; yy < resolution; yy++ {
				foundJ := -1
				for j, rr := range runs[yy] {
					if !used[yy][j] && rr.x0 == r.x0 && rr.x1 == r.x1 {
						foundJ = j
						break
					}
				}
				if foundJ == -1 {
					break
				}
				used[yy][foundJ] = true
				h++
			}
			w := r.x1 - r.x0 + 1
			var buf bytes.Buffer
			for yy := y; yy < y+h; yy++ {
				start := yy*resolution + r.x0
				buf.Write(data[start : start+w])
			}
			rects = append(rects, deltaRect{x: r.x0, y: y, w: w, h: h, rows: buf.Bytes()})
			deltaBytes += w * h
		}
	}
	return
}

// send a FullTile for cid to a specific player (stateMu held by caller)
func (s *Server) minimapSendFullTo(playerID uint32, cid ChunkID) {
	resolution := uint32(64)
	if playerRes, ok := s.minimapPlayerRes[playerID]; ok {
		resolution = playerRes
	}

	data := s.computeTileDataAtResolution(cid, resolution)
	if data == nil {
		return // chunk is all-unseen; no point sending
	}

	// Bump version if a tile exists, otherwise leave it at 0 (new subscribers
	// start at 0 too).
	var version uint32
	if t := s.minimapTiles[cid]; t != nil {
		version = t.Version
	}

	msg := &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
		Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
		Version:    version,
		Data:       data,
		Resolution: resolution,
	}}}
	s.sendToPlayer(playerID, mustProto(msg))
}

// runMinimapBroadcaster drains the dirty tile set and sends deltas (or full
// tiles when delta would exceed half the tile size). Runs in its own goroutine.
func (s *Server) runMinimapBroadcaster() {
	for {
		s.stateMu.Lock()
		if len(s.minimapDirtyTiles) == 0 {
			s.stateMu.Unlock()
			time.Sleep(20 * time.Millisecond)
			continue
		}
		dirty := make([]ChunkID, 0, len(s.minimapDirtyTiles))
		for cid := range s.minimapDirtyTiles {
			dirty = append(dirty, cid)
		}
		s.minimapDirtyTiles = make(map[ChunkID]struct{})

		// Collect per-resolution messages to send OUTSIDE the lock.
		type pendingSend struct {
			data []byte
			pids []uint32
		}
		var pending []pendingSend

		for _, cid := range dirty {
			subs := s.minimapSubs[cid]
			if len(subs) == 0 {
				// Tile became unsubscribed while we were queued; drop state.
				delete(s.minimapTiles, cid)
				continue
			}
			t := s.minimapTiles[cid]
			if t == nil || !t.hasAnyDirty() {
				continue
			}

			// Group subscribers by their chosen resolution.
			playersByRes := make(map[uint32][]uint32)
			for pid := range subs {
				res := uint32(64)
				if pr, ok := s.minimapPlayerRes[pid]; ok {
					res = pr
				}
				playersByRes[res] = append(playersByRes[res], pid)
			}

			// Precompute full data once; we'll downsample as needed.
			fullData := s.computeFullTileData(cid)
			t.Version++
			version := t.Version

			for res, players := range playersByRes {
				var data []byte
				if res == 64 {
					data = fullData
				} else if fullData != nil {
					blockSize := int(64 / res)
					data = make([]byte, res*res)
					for y := 0; y < int(res); y++ {
						for x := 0; x < int(res); x++ {
							data[y*int(res)+x] = probabilisticDownsample(cid, fullData, x, y, blockSize)
						}
					}
				}
				if data == nil {
					continue // chunk has no state; nothing to send
				}
				dirtyLR := t.deriveDirtyAtResolution(res)
				rects, deltaBytes := collectRectsFromDirty(dirtyLR, data, int(res))
				tileSize := int(res * res)

				var payload *pb.Msg
				if deltaBytes > tileSize/2 {
					payload = &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
						Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
						Version:    version,
						Data:       data,
						Resolution: res,
					}}}
				} else if len(rects) > 0 {
					pRects := make([]*pb.DeltaRect, 0, len(rects))
					for _, r := range rects {
						pRects = append(pRects, &pb.DeltaRect{
							X:    uint32(r.x),
							Y:    uint32(r.y),
							W:    uint32(r.w),
							H:    uint32(r.h),
							Rows: r.rows,
						})
					}
					payload = &pb.Msg{Payload: &pb.Msg_MinimapTileDelta{MinimapTileDelta: &pb.TileDelta{
						Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
						Version:    version,
						Rects:      pRects,
						Resolution: res,
					}}}
				}
				if payload != nil {
					pending = append(pending, pendingSend{data: mustProto(payload), pids: players})
				}
			}
			t.clearDirty()
		}
		s.stateMu.Unlock()

		for _, p := range pending {
			for _, pid := range p.pids {
				s.sendToPlayer(pid, p.data)
			}
		}
	}
}
