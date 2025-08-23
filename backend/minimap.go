package main

import (
	"bytes"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// MinimapTile represents a resolution×resolution palette-indexed tile with a monotonic version.
type MinimapTile struct {
	Version    uint32
	Resolution uint32 // 16, 32, or 64
	Data       []byte // resolution*resolution palette indices
	Dirty      []bool // resolution*resolution dirty flags
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

// Create a new MinimapTile with the specified resolution
func newMinimapTile(resolution uint32) *MinimapTile {
	size := resolution * resolution
	return &MinimapTile{
		Resolution: resolution,
		Data:       make([]byte, size),
		Dirty:      make([]bool, size),
	}
}

// Simple pixel averaging that preserves overall density and visual appearance
// This maintains the intuitive look of the minimap at all scales
func simplePixelAverage(fullData []byte, blockX, blockY, blockSize int) byte {
	counts := make(map[byte]int)
	totalPixels := 0
	
	// Count each palette index in this averaging block
	for dy := 0; dy < blockSize; dy++ {
		for dx := 0; dx < blockSize; dx++ {
			srcX := blockX*blockSize + dx
			srcY := blockY*blockSize + dy
			if srcX >= ChunkSize || srcY >= ChunkSize {
				continue
			}
			idx := fullData[srcY*ChunkSize + srcX]
			counts[idx]++
			totalPixels++
		}
	}
	
	// Return the most frequent palette index (mode)
	// This preserves the overall density and visual character
	maxCount := 0
	var bestIdx byte = mmUnseen
	
	for idx, count := range counts {
		if count > maxCount {
			maxCount = count
			bestIdx = idx
		}
	}
	
	return bestIdx
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

// mark a cell dirty in the minimap and set its palette index for full resolution (64x64)
func (s *Server) minimapSetCell(cid ChunkID, cell uint32, idx byte) {
	if s.minimapTiles[cid] == nil {
		s.minimapTiles[cid] = make(map[uint32]*MinimapTile)
	}
	
	// Always maintain the full-resolution (64x64) tile as the master
	t := s.minimapTiles[cid][64]
	if t == nil {
		t = newMinimapTile(64)
		s.minimapTiles[cid][64] = t
	}
	
	pos := int(cell)
	if pos < 0 || pos >= ChunkSize*ChunkSize {
		return
	}
	if t.Data[pos] != idx {
		t.Data[pos] = idx
		t.Dirty[pos] = true
		s.minimapDirtyTiles[cid] = struct{}{}
		
		// Mark lower resolution tiles as dirty too
		if t32, ok := s.minimapTiles[cid][32]; ok {
			// Calculate which lower-res cell this affects
			localX := pos % ChunkSize
			localY := pos / ChunkSize
			lrX := localX / 2  // 64->32 downsampling
			lrY := localY / 2
			lrPos := lrY*32 + lrX
			t32.Dirty[lrPos] = true
		}
		
		if t16, ok := s.minimapTiles[cid][16]; ok {
			localX := pos % ChunkSize
			localY := pos / ChunkSize
			lrX := localX / 4  // 64->16 downsampling
			lrY := localY / 4
			lrPos := lrY*16 + lrX
			t16.Dirty[lrPos] = true
		}
	}
}

// compute palette index for a cell given current authoritative state
func (s *Server) minimapPaletteFor(cid ChunkID, cell uint32) byte {
	if s.isCellFlagged(cid, cell) {
		// Use the stored flag's ID to choose one of 10 palette buckets
		if fl, ok := s.flags[cid][cell]; ok {
			b := minimapFlagBucket(fl.FlagID)
			return byte(mmFlagBase + b)
		}
		return byte(mmFlagBase)
	}
	if s.isCellRevealed(cid, cell) {
		if s.isMine(cid, cell) {
			return mmMine
		}
		n := s.countAdjacentMines(cid, cell)
		switch n {
		case 0:
			return mmEmpty0
		case 1:
			return mmNum1
		case 2:
			return mmNum2
		case 3:
			return mmNum3
		case 4:
			return mmNum4
		case 5:
			return mmNum5
		case 6:
			return mmNum6
		default:
			return mmNum7Plus
		}
	}
	return mmUnseen
}

// rebuild a full tile from current world state at the specified resolution
func (s *Server) minimapRebuildTile(cid ChunkID, resolution uint32) *MinimapTile {
	if s.minimapTiles[cid] == nil {
		s.minimapTiles[cid] = make(map[uint32]*MinimapTile)
	}
	
	t := s.minimapTiles[cid][resolution]
	if t == nil {
		t = newMinimapTile(resolution)
		s.minimapTiles[cid][resolution] = t
	}
	
	if resolution == 64 {
		// Full resolution - direct mapping
		for i := 0; i < ChunkSize*ChunkSize; i++ {
			idx := s.minimapPaletteFor(cid, uint32(i))
			t.Data[i] = idx
			t.Dirty[i] = false
		}
	} else {
		// Lower resolution - need to rebuild from full resolution
		fullTile := s.minimapRebuildTile(cid, 64) // Ensure full resolution exists
		blockSize := int(64 / resolution)        // 64/32=2, 64/16=4
		
		for y := 0; y < int(resolution); y++ {
			for x := 0; x < int(resolution); x++ {
				idx := simplePixelAverage(fullTile.Data, x, y, blockSize)
				pos := y*int(resolution) + x
				t.Data[pos] = idx
				t.Dirty[pos] = false
			}
		}
	}
	return t
}

// called from reveal/flag logic under stateMu write-lock
func (s *Server) minimapOnReveal(cid ChunkID, cell uint32) {
	s.minimapSetCell(cid, cell, s.minimapPaletteFor(cid, cell))
}

func (s *Server) minimapOnFlag(cid ChunkID, cell uint32) {
	s.minimapSetCell(cid, cell, s.minimapPaletteFor(cid, cell))
}

// gather dirty rectangles for a tile; also computes total bytes of deltas
func minimapCollectRects(t *MinimapTile) (rects []struct {
	x, y, w, h int
	rows       []byte
}, deltaBytes int) {
	resolution := int(t.Resolution)
	
	// First, find horizontal runs per row
	type run struct{ x0, x1 int }
	runs := make([][]run, resolution)
	for y := 0; y < resolution; y++ {
		row := t.Dirty[y*resolution : (y+1)*resolution]
		x := 0
		for x < resolution {
			// skip clean
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
			// try to extend downward while there exists an identical run
			for yy := y + 1; yy < resolution; yy++ {
				// find a matching run in runs[yy]
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
			// Build rows payload
			w := r.x1 - r.x0 + 1
			var buf bytes.Buffer
			for yy := y; yy < y+h; yy++ {
				start := yy*resolution + r.x0
				buf.Write(t.Data[start : start+w])
			}
			rects = append(rects, struct {
				x, y, w, h int
				rows       []byte
			}{x: r.x0, y: y, w: w, h: h, rows: buf.Bytes()})
			deltaBytes += w * h
		}
	}
	return
}

// send a FullTile for cid to a specific player (stateMu may be held by caller)
func (s *Server) minimapSendFullTo(playerID uint32, cid ChunkID) {
	// Get player's resolution preference (default to 64 if not set)
	resolution := uint32(64)
	if playerRes, ok := s.minimapPlayerRes[playerID]; ok {
		resolution = playerRes
	}
	
	// Rebuild from authoritative state to avoid stale or missing tiles
	t := s.minimapRebuildTile(cid, resolution)
	isAllUnseen := true
	tileSize := int(resolution * resolution)
	for i := 0; i < tileSize; i++ {
		if t.Data[i] != mmUnseen {
			isAllUnseen = false
			break
		}
	}
	if isAllUnseen {
		return
	}
	// prepare bytes
	data := make([]byte, len(t.Data))
	copy(data, t.Data[:])
	msg := &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
		Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
		Version:    t.Version,
		Data:       data,
		Resolution: resolution,
	}}}
	s.sendToPlayer(playerID, mustProto(msg))
}

// broadcast minimap changes periodically; should be run in a goroutine
func (s *Server) runMinimapBroadcaster() {
	// simple ticker-less loop driven by small sleeps to keep latency low
	for {
		// snapshot dirty set
		s.stateMu.Lock()
		if len(s.minimapDirtyTiles) == 0 {
			s.stateMu.Unlock()
			// sleep a bit to avoid busy loop
			time.Sleep(20 * time.Millisecond)
			continue
		}
		dirty := make([]ChunkID, 0, len(s.minimapDirtyTiles))
		for cid := range s.minimapDirtyTiles {
			dirty = append(dirty, cid)
		}
		// reset set
		s.minimapDirtyTiles = make(map[ChunkID]struct{})

		// For each dirty tile, group subscribers by resolution and send appropriate updates
		for _, cid := range dirty {
			subs := s.minimapSubs[cid]
			if len(subs) == 0 {
				continue
			}
			
			// Group subscribers by resolution
			playersByRes := make(map[uint32][]uint32)
			for pid := range subs {
				res := uint32(64) // default
				if playerRes, ok := s.minimapPlayerRes[pid]; ok {
					res = playerRes
				}
				playersByRes[res] = append(playersByRes[res], pid)
			}
			
			// Process each resolution group
			for resolution, players := range playersByRes {
				tileMap := s.minimapTiles[cid]
				if tileMap == nil {
					continue
				}
				
				t := tileMap[resolution]
				if t == nil {
					// Need to create/rebuild this resolution
					t = s.minimapRebuildTile(cid, resolution)
				}
				
				rects, deltaBytes := minimapCollectRects(t)
				tileSize := int(resolution * resolution)
				
				// Heuristic: if over half of tile, send full
				if deltaBytes > tileSize/2 {
					// clear dirties, bump version, send full
					for i := range t.Dirty {
						t.Dirty[i] = false
					}
					t.Version++
					data := make([]byte, len(t.Data))
					copy(data, t.Data[:])
					msg := &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
						Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
						Version:    t.Version,
						Data:       data,
						Resolution: resolution,
					}}}
					s.stateMu.Unlock()
					for _, pid := range players {
						s.sendToPlayer(pid, mustProto(msg))
					}
					s.stateMu.Lock()
					continue
				}
				
				if len(rects) == 0 {
					continue
				}
				
				// build TileDelta
				pRects := make([]*pb.DeltaRect, 0, len(rects))
				for _, r := range rects {
					// clear dirty marks for cells in rect
					for yy := r.y; yy < r.y+r.h; yy++ {
						for xx := r.x; xx < r.x+r.w; xx++ {
							t.Dirty[yy*int(resolution)+xx] = false
						}
					}
					pRects = append(pRects, &pb.DeltaRect{
						X:    uint32(r.x),
						Y:    uint32(r.y),
						W:    uint32(r.w),
						H:    uint32(r.h),
						Rows: append([]byte(nil), r.rows...),
					})
				}
				t.Version++
				msg := &pb.Msg{Payload: &pb.Msg_MinimapTileDelta{MinimapTileDelta: &pb.TileDelta{
					Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
					Version:    t.Version,
					Rects:      pRects,
					Resolution: resolution,
				}}}
				s.stateMu.Unlock()
				for _, pid := range players {
					s.sendToPlayer(pid, mustProto(msg))
				}
				s.stateMu.Lock()
			}
		}
		s.stateMu.Unlock()
	}
}
