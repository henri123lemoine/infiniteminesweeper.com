package main

import (
	// "log"
	// "os"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// Cache key for minimap tiles
type minimapCacheKey struct {
	chunkID    ChunkID
	resolution uint32
}

// Async update for minimap processing
type minimapUpdate struct {
	chunkID ChunkID
	cell    uint32
}

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
			lrX := localX / 2 // 64->32 downsampling
			lrY := localY / 2
			lrPos := lrY*32 + lrX
			t32.Dirty[lrPos] = true
		}

		if t16, ok := s.minimapTiles[cid][16]; ok {
			localX := pos % ChunkSize
			localY := pos / ChunkSize
			lrX := localX / 4 // 64->16 downsampling
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

// LRU cache management for minimap tiles
func (s *Server) minimapCacheGet(cid ChunkID, resolution uint32) *MinimapTile {
	key := minimapCacheKey{chunkID: cid, resolution: resolution}
	if tile, ok := s.minimapCache[key]; ok {
		// Move to front of LRU order
		s.minimapCacheMoveToFront(key)
		return tile
	}
	return nil
}

func (s *Server) minimapCachePut(cid ChunkID, resolution uint32, tile *MinimapTile) {
	key := minimapCacheKey{chunkID: cid, resolution: resolution}

	// If already exists, just update and move to front
	if _, ok := s.minimapCache[key]; ok {
		s.minimapCache[key] = tile
		s.minimapCacheMoveToFront(key)
		return
	}

	// Evict oldest if at capacity
	if len(s.minimapCache) >= s.minimapCacheMaxSize {
		s.minimapCacheEvictOldest()
	}

	// Add new entry
	s.minimapCache[key] = tile
	s.minimapCacheOrder = append([]minimapCacheKey{key}, s.minimapCacheOrder...)
}

func (s *Server) minimapCacheInvalidate(cid ChunkID) {
	// Remove all resolutions for this chunk
	keysToRemove := make([]minimapCacheKey, 0, 3)
	for key := range s.minimapCache {
		if key.chunkID == cid {
			keysToRemove = append(keysToRemove, key)
		}
	}
	for _, key := range keysToRemove {
		delete(s.minimapCache, key)
		s.minimapCacheRemoveFromOrder(key)
	}
}

func (s *Server) minimapCacheMoveToFront(key minimapCacheKey) {
	// Find and remove from current position
	for i, k := range s.minimapCacheOrder {
		if k == key {
			s.minimapCacheOrder = append(s.minimapCacheOrder[:i], s.minimapCacheOrder[i+1:]...)
			break
		}
	}
	// Add to front
	s.minimapCacheOrder = append([]minimapCacheKey{key}, s.minimapCacheOrder...)
}

func (s *Server) minimapCacheRemoveFromOrder(key minimapCacheKey) {
	for i, k := range s.minimapCacheOrder {
		if k == key {
			s.minimapCacheOrder = append(s.minimapCacheOrder[:i], s.minimapCacheOrder[i+1:]...)
			break
		}
	}
}

func (s *Server) minimapCacheEvictOldest() {
	if len(s.minimapCacheOrder) == 0 {
		return
	}
	oldest := s.minimapCacheOrder[len(s.minimapCacheOrder)-1]
	delete(s.minimapCache, oldest)
	s.minimapCacheOrder = s.minimapCacheOrder[:len(s.minimapCacheOrder)-1]
}

// rebuild a full tile from current world state at the specified resolution
func (s *Server) minimapRebuildTile(cid ChunkID, resolution uint32) *MinimapTile {
	// Check cache first
	if cached := s.minimapCacheGet(cid, resolution); cached != nil {
		return cached
	}

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
		nonUnseenCount := 0
		for i := 0; i < ChunkSize*ChunkSize; i++ {
			idx := s.minimapPaletteFor(cid, uint32(i))
			if idx != mmUnseen {
				nonUnseenCount++
			}
			t.Data[i] = idx
			t.Dirty[i] = false
		}
		// if os.Getenv("MODE") == "development" && nonUnseenCount > 0 {
		// 	log.Printf("[DEBUG] Rebuilt tile (%d,%d) with %d/%d non-unseen cells",
		// 		cid.X, cid.Y, nonUnseenCount, ChunkSize*ChunkSize)
		// }
	} else {
		// Lower resolution - rebuild from full resolution using probabilistic downsampling
		fullTile := s.minimapRebuildTile(cid, 64) // Ensure full resolution exists
		blockSize := int(64 / resolution)         // 64/32=2, 64/16=4

		for y := 0; y < int(resolution); y++ {
			for x := 0; x < int(resolution); x++ {
				idx := probabilisticDownsample(cid, fullTile.Data, x, y, blockSize)
				pos := y*int(resolution) + x
				t.Data[pos] = idx
				t.Dirty[pos] = false
			}
		}
	}

	// Cache the rebuilt tile
	s.minimapCachePut(cid, resolution, t)
	return t
}

// called from reveal/flag logic under stateMu write-lock
func (s *Server) minimapOnReveal(cid ChunkID, cell uint32) {
	// Queue update for async processing to minimize lock time
	select {
	case s.minimapUpdateQueue <- minimapUpdate{chunkID: cid, cell: cell}:
	default:
		// Channel full - drop update to avoid blocking game logic
	}
}

func (s *Server) minimapOnFlag(cid ChunkID, cell uint32) {
	// Queue update for async processing to minimize lock time
	select {
	case s.minimapUpdateQueue <- minimapUpdate{chunkID: cid, cell: cell}:
	default:
		// Channel full - drop update to avoid blocking game logic
	}
}

// gather dirty rectangles for a tile using efficient span-based algorithm
func minimapCollectRects(t *MinimapTile) (rects []struct {
	x, y, w, h int
	rows       []byte
}, deltaBytes int) {
	resolution := int(t.Resolution)

	// Single-pass span finding with minimal allocations
	type span struct{ x0, x1 int }
	spans := make([]span, 0, resolution/4) // pre-allocate reasonable capacity

	for y := 0; y < resolution; y++ {
		spans = spans[:0] // reuse slice

		// Find horizontal spans in this row
		x := 0
		for x < resolution {
			// Skip clean cells
			for x < resolution && !t.Dirty[y*resolution+x] {
				x++
			}
			if x >= resolution {
				break
			}
			start := x
			// Find end of dirty run
			for x < resolution && t.Dirty[y*resolution+x] {
				x++
			}
			spans = append(spans, span{start, x - 1})
		}

		// Convert each span to a 1-row rectangle
		for _, s := range spans {
			w := s.x1 - s.x0 + 1
			rows := make([]byte, w)
			start := y*resolution + s.x0
			copy(rows, t.Data[start:start+w])

			rects = append(rects, struct {
				x, y, w, h int
				rows       []byte
			}{x: s.x0, y: y, w: w, h: 1, rows: rows})
			deltaBytes += w
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

// process queued minimap updates to apply changes without blocking game logic
func (s *Server) processMinimapUpdates() {
	// Drain all pending updates in a tight loop
	for {
		select {
		case update := <-s.minimapUpdateQueue:
			s.stateMu.Lock()
			s.minimapCacheInvalidate(update.chunkID)
			s.minimapSetCell(update.chunkID, update.cell, s.minimapPaletteFor(update.chunkID, update.cell))
			s.stateMu.Unlock()
		default:
			// No more updates pending
			return
		}
	}
}

// broadcast minimap changes using batched updates to reduce message overhead
func (s *Server) runMinimapBroadcaster() {
	const batchIntervalMs = 50 // Collect updates for 50ms before sending

	for {
		// Process queued updates first
		s.processMinimapUpdates()

		// snapshot dirty set
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
		// if os.Getenv("MODE") == "development" && len(dirty) > 0 {
		// 	log.Printf("[DEBUG] Processing %d dirty minimap tiles", len(dirty))
		// }
		s.minimapDirtyTiles = make(map[ChunkID]struct{})
		s.stateMu.Unlock()

		// Group all updates by (playerID, resolution) to batch them
		type batchKey struct {
			playerID   uint32
			resolution uint32
		}
		batches := make(map[batchKey]*pb.MinimapBatch)

		s.stateMu.Lock()
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
				var t *MinimapTile

				if tileMap == nil || tileMap[resolution] == nil {
					// No existing tile - rebuild from scratch and send as full
					t = s.minimapRebuildTile(cid, resolution)

					// Check if tile has any non-unseen data before sending
					isAllUnseen := true
					tileSize := int(resolution * resolution)
					for i := 0; i < tileSize; i++ {
						if t.Data[i] != mmUnseen {
							isAllUnseen = false
							break
						}
					}
					if isAllUnseen {
						continue // Skip sending empty tiles
					}

					// For new tiles, send full tile to all subscribers
					for _, pid := range players {
						key := batchKey{playerID: pid, resolution: resolution}
						if batches[key] == nil {
							batches[key] = &pb.MinimapBatch{}
						}
						data := make([]byte, len(t.Data))
						copy(data, t.Data[:])
						fullTile := &pb.FullTile{
							Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
							Version:    t.Version,
							Data:       data,
							Resolution: resolution,
						}
						batches[key].FullTiles = append(batches[key].FullTiles, fullTile)
						// if os.Getenv("MODE") == "development" {
						// 	log.Printf("[DEBUG] Adding full tile (%d,%d) res=%d with %d bytes to batch for player %d",
						// 		cid.X, cid.Y, resolution, len(data), pid)
						// }
					}
					continue
				}

				t = tileMap[resolution]
				rects, deltaBytes := minimapCollectRects(t)
				tileSize := int(resolution * resolution)

				// Decide between full tile or delta
				if deltaBytes > tileSize/2 {
					// Send full tile
					for i := range t.Dirty {
						t.Dirty[i] = false
					}
					t.Version++
					data := make([]byte, len(t.Data))
					copy(data, t.Data[:])

					// Add to batch for each player at this resolution (create separate instances)
					for _, pid := range players {
						key := batchKey{playerID: pid, resolution: resolution}
						if batches[key] == nil {
							batches[key] = &pb.MinimapBatch{}
						}
						// Create separate FullTile instance for each player
						fullTile := &pb.FullTile{
							Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
							Version:    t.Version,
							Data:       append([]byte(nil), data...), // deep copy data
							Resolution: resolution,
						}
						batches[key].FullTiles = append(batches[key].FullTiles, fullTile)
					}
				} else if len(rects) > 0 {
					// Send delta
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

					// Add to batch for each player at this resolution (create separate instances)
					for _, pid := range players {
						key := batchKey{playerID: pid, resolution: resolution}
						if batches[key] == nil {
							batches[key] = &pb.MinimapBatch{}
						}
						// Create separate TileDelta instance for each player
						tileDelta := &pb.TileDelta{
							Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
							Version:    t.Version,
							Rects:      append([]*pb.DeltaRect(nil), pRects...), // deep copy rects
							Resolution: resolution,
						}
						batches[key].TileDeltas = append(batches[key].TileDeltas, tileDelta)
					}
				}
			}
		}
		s.stateMu.Unlock()

		// Send batches to players
		for key, batch := range batches {
			if len(batch.FullTiles) > 0 || len(batch.TileDeltas) > 0 {
				// if os.Getenv("MODE") == "development" {
				// 	log.Printf("[DEBUG] Player %d <- MinimapBatch: %d full tiles, %d deltas at res %d",
				// 		key.playerID, len(batch.FullTiles), len(batch.TileDeltas), key.resolution)
				// }
				msg := &pb.Msg{Payload: &pb.Msg_MinimapBatch{MinimapBatch: batch}}
				s.sendToPlayer(key.playerID, mustProto(msg))
			}
		}
	}
}
