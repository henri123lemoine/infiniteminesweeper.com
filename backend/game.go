package main

import (
	"math"
	"math/bits"
	"regexp"
	"sync"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// Helper Functions

func worldToChunk(x, y int) (ChunkID, uint32) {
	cx := int64(math.Floor(float64(x) / float64(ChunkSize)))
	cy := int64(math.Floor(float64(y) / float64(ChunkSize)))

	lx := x - int(cx)*ChunkSize
	ly := y - int(cy)*ChunkSize
	return ChunkID{X: cx, Y: cy}, uint32(ly*ChunkSize + lx)
}

// splitmix64 - deterministic PRNG for bomb placement (matches frontend)
func splitmix64(state uint64) uint64 {
	state += 0x9e3779b97f4a7c15
	state = (state ^ (state >> 30)) * 0xbf58476d1ce4e5b9
	state = (state ^ (state >> 27)) * 0x94d049bb133111eb
	return state ^ (state >> 31)
}

func cellIndexToXY(cell uint32) (int, int) {
	return int(cell % ChunkSize), int(cell / ChunkSize)
}

//  Adjacent-helper additions

// countAdjacentRevealedMines counts *already-revealed* mines around (chunkID,cell)
func (s *Server) countAdjacentRevealedMines(chunkID ChunkID, cell uint32) int {
	x, y := cellIndexToXY(cell)
	worldX, worldY := int(chunkID.X)*ChunkSize+x, int(chunkID.Y)*ChunkSize+y
	cnt := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			wx, wy := worldX+dx, worldY+dy
			cid, cidx := worldToChunk(wx, wy)
			if s.isCellRevealed(cid, cidx) && s.isMine(cid, cidx) {
				cnt++
			}
		}
	}
	return cnt
}

// applyScore clamps at 0 and returns the new total
func (s *Server) applyScore(playerID uint32, delta int32) int32 {
	newScore := s.scores[playerID] + delta
	if newScore < 0 {
		newScore = 0
	}
	s.scores[playerID] = newScore
	return newScore
}

// streakBonus: +5% per consecutive correct action, capped at 3x. Guessers
// can't sustain a streak. Caller must hold stateMu.
func (s *Server) streakBonus(playerID uint32) float64 {
	return 1.0 + math.Min(0.05*float64(s.streaks[playerID]), 2.0)
}

// getMineBitmap returns a cached 4096-bit mine bitmap for a chunk. Mines are
// pure functions of (seed, density); both are immutable per chunk, so the
// bitmap never needs invalidation.
func (s *Server) getMineBitmap(chunkID ChunkID) *[512]byte {
	s.mineBitmapsMu.Lock()
	if bm, ok := s.mineBitmaps[chunkID]; ok {
		s.mineBitmapsMu.Unlock()
		return bm
	}
	s.mineBitmapsMu.Unlock()

	seed := s.generateChunkSeed(chunkID)
	d32 := float32(s.getChunkDensity(chunkID))
	if d32 < 0 {
		d32 = 0
	} else if d32 > 1 {
		d32 = 1
	}
	threshold := uint64(math.Floor(float64(d32 * 100.0)))
	if threshold > 100 {
		threshold = 100
	}
	bm := &[512]byte{}
	for cell := uint32(0); cell < ChunkSize*ChunkSize; cell++ {
		if splitmix64(seed+uint64(cell))%100 < threshold {
			bm[cell>>3] |= 1 << (cell & 7)
		}
	}

	s.mineBitmapsMu.Lock()
	if len(s.mineBitmaps) >= mineBitmapCacheMaxEntries {
		for k := range s.mineBitmaps {
			delete(s.mineBitmaps, k)
			break
		}
	}
	s.mineBitmaps[chunkID] = bm
	s.mineBitmapsMu.Unlock()
	return bm
}

func (s *Server) isMine(chunkID ChunkID, cell uint32) bool {
	return s.getMineBitmap(chunkID)[cell>>3]&(1<<(cell&7)) != 0
}

func (s *Server) countAdjacentMines(chunkID ChunkID, cell uint32) int {
	x, y := cellIndexToXY(cell)
	// Fast path: when the 3×3 neighborhood stays inside the same chunk,
	// one bitmap lookup services all 8 neighbors.
	if x >= 1 && x <= ChunkSize-2 && y >= 1 && y <= ChunkSize-2 {
		bm := s.getMineBitmap(chunkID)
		count := 0
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nc := uint32((y+dy)*ChunkSize + (x + dx))
				if bm[nc>>3]&(1<<(nc&7)) != 0 {
					count++
				}
			}
		}
		return count
	}
	worldX := int(chunkID.X)*ChunkSize + x
	worldY := int(chunkID.Y)*ChunkSize + y
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			cid, cidx := worldToChunk(worldX+dx, worldY+dy)
			if s.isMine(cid, cidx) {
				count++
			}
		}
	}
	return count
}

// Bounds checking for reveal requests
func (s *Server) isValidCoordinate(x, y int) bool {
	return x >= 0 && x < ChunkSize && y >= 0 && y < ChunkSize
}

var usernameRegex = regexp.MustCompile(`^[A-Za-z0-9 ._'-]{1,30}$`)

func isValidUsername(name string) bool {
	return usernameRegex.MatchString(name)
}

// validateUsername checks if a username is valid and available for the given player
// Returns empty string on success, or error message on failure
func (s *Server) validateUsername(name string, currentPlayerID uint32) string {
	if !isValidUsername(name) {
		return "Invalid username"
	}

	if existingPlayerID, exists := s.nameToPlayerID[name]; exists && existingPlayerID != currentPlayerID {
		return "Username already taken"
	}

	return ""
}

// Unified reveal / flag / chord handler
// – All state mutations happen while the write-lock is held
// – Actual broadcasts happen **after** the lock is released to avoid writer->reader self-dead-locks.
func (s *Server) handleReveal(
	playerID uint32,
	requestID uint64,
	chunkID ChunkID,
	cell uint32,
	isRightClick bool,
	isChord bool,
) {
	// phase 1: mutate shared state under the write-lock and collect the deltas
	s.stateMu.Lock()

	// 1. Initial State & Validation
	x, y := cellIndexToXY(cell)
	if x < 0 || x >= ChunkSize || y < 0 || y >= ChunkSize {
		s.stateMu.Unlock()
		return // Invalid cell index.
	}

	isRevealed := s.isCellRevealed(chunkID, cell)
	isFlagged := s.isCellFlagged(chunkID, cell)

	// collectors that will be handed to the broadcaster later
	allRevealedCells := make(map[ChunkID]*pb.RevealedCells)
	allPlacedFlags := make(map[ChunkID][]*pb.FlagPlacement)
	var scoreDelta int32

	// 2. Process command intent
	if isRightClick {
		// FLAG INTENT
		if isRevealed || isFlagged {
			s.sendRevealAck(playerID, requestID, false, nil, 0, chunkID, cell)
			s.stateMu.Unlock()
			return
		}

		// Proximity rule: only allow flagging within distance <= 2 of any revealed cell
		worldX := int(chunkID.X)*ChunkSize + x
		worldY := int(chunkID.Y)*ChunkSize + y
		if !s.hasRevealedWithinTwo(worldX, worldY) {
			s.sendRevealAck(playerID, requestID, false, nil, 0, chunkID, cell)
			s.stateMu.Unlock()
			return
		}

		playerFlagID := s.playerFlags[playerID]
		if s.isMine(chunkID, cell) {
			// correct flag
			m := s.getScoreMultiplier(chunkID)
			scoreDelta = int32(math.Round(10 * m * s.streakBonus(playerID)))
			s.applyScore(playerID, scoreDelta)
			s.streaks[playerID]++
			s.statsForLocked(playerID).CorrectFlags++
			s.bumpSafeStreakLocked(playerID)
			s.setCellFlagged(chunkID, cell, playerID, playerFlagID, &allPlacedFlags)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_FlaggedCell{FlaggedCell: &pb.FlagPlacement{Cell: cell, FlagID: playerFlagID}}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		} else {
			// Wrong flag: deduct points, and perform a flood-fill reveal like a normal reveal.
			// Penalty scales with the same multiplier as rewards — a flat
			// penalty made blind flag-spam positive-EV in hot sectors.
			m := s.getScoreMultiplier(chunkID)
			scoreDelta = -int32(math.Round(20 * m))
			s.applyScore(playerID, scoreDelta)
			s.streaks[playerID] = 0
			s.resetSafeStreakLocked(playerID)
			s.floodFillReveal(chunkID, cell, playerID, &allRevealedCells)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_RevealedCells{RevealedCells: allRevealedCells[chunkID]}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		}
	} else if isChord {
		// CHORD INTENT WITH FLOOD FILL
		if !isRevealed {
			s.sendRevealAck(playerID, requestID, false, nil, 0, chunkID, cell)
			s.stateMu.Unlock()
			return
		}

		need := s.countAdjacentMines(chunkID, cell)
		have := s.countAdjacentFlags(chunkID, cell) +
			s.countAdjacentRevealedMines(chunkID, cell) // *** revealed mines now count ***
		if need != have {
			s.sendRevealAck(playerID, requestID, false, nil, 0, chunkID, cell)
			s.stateMu.Unlock()
			return
		}

		// BFS queue seeded with all adjacent cells
		type qItem struct {
			Cid  ChunkID
			Cidx uint32
		}
		queue := []qItem{}
		visited := make(map[qItem]struct{})
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				wx := int(chunkID.X)*ChunkSize + x + dx
				wy := int(chunkID.Y)*ChunkSize + y + dy
				cid, cidx := worldToChunk(wx, wy)
				queue = append(queue, qItem{cid, cidx})
			}
		}

		// Process flood-fill: reveal neighbors; if zero, enqueue its neighbors
		minesHit := 0
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if _, seen := visited[curr]; seen {
				continue
			}
			visited[curr] = struct{}{}

			// skip if already revealed or flagged
			if s.isCellRevealed(curr.Cid, curr.Cidx) || s.isCellFlagged(curr.Cid, curr.Cidx) {
				continue
			}

			if s.isMine(curr.Cid, curr.Cidx) {
				// mine hit under chord
				scoreDelta -= 100
				minesHit++
				s.setCellRevealed(curr.Cid, curr.Cidx, playerID, &allRevealedCells)
				continue
			}

			// reveal this cell
			s.setCellRevealed(curr.Cid, curr.Cidx, playerID, &allRevealedCells)
			scoreDelta += 1 // safe cell revealed via chord
			adj := s.countAdjacentMines(curr.Cid, curr.Cidx)
			if adj == 0 {
				// expand flood-fill around this zero cell
				cx, cy := cellIndexToXY(curr.Cidx)
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						wx := int(curr.Cid.X)*ChunkSize + cx + dx
						wy := int(curr.Cid.Y)*ChunkSize + cy + dy
						ncid, ncidx := worldToChunk(wx, wy)
						queue = append(queue, qItem{ncid, ncidx})
					}
				}
			}
		}

		// Scale the net either way — exempting negative nets would make chord
		// mine-hits nearly free in high-multiplier sectors. The streak bonus
		// only applies to flawless (no mine) chords.
		m := s.getScoreMultiplier(chunkID)
		if minesHit == 0 && scoreDelta > 0 {
			scoreDelta = int32(math.Round(float64(scoreDelta) * m * s.streakBonus(playerID)))
			s.streaks[playerID]++
			s.bumpSafeStreakLocked(playerID)
		} else {
			scoreDelta = int32(math.Round(float64(scoreDelta) * m))
			if minesHit > 0 {
				s.streaks[playerID] = 0
				s.resetSafeStreakLocked(playerID)
			}
		}
		s.applyScore(playerID, scoreDelta)
		ack := &pb.RevealAck{
			Outcome: &pb.RevealAck_RevealedCells{
				RevealedCells: allRevealedCells[chunkID],
			},
		}
		s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
	} else {
		// STANDARD REVEAL INTENT
		if isRevealed || isFlagged {
			s.sendRevealAck(playerID, requestID, false, nil, 0, chunkID, cell)
			s.stateMu.Unlock()
			return
		}

		// Proximity rule: only allow revealing within distance <= 2 of any revealed cell
		worldX := int(chunkID.X)*ChunkSize + x
		worldY := int(chunkID.Y)*ChunkSize + y
		if !s.hasRevealedWithinTwo(worldX, worldY) {
			s.sendRevealAck(playerID, requestID, false, nil, 0, chunkID, cell)
			s.stateMu.Unlock()
			return
		}

		if s.isMine(chunkID, cell) {
			// Penalty scales with the multiplier, matching the reward side.
			m := s.getScoreMultiplier(chunkID)
			scoreDelta = -int32(math.Round(100 * m))
			s.applyScore(playerID, scoreDelta)
			s.streaks[playerID] = 0
			s.resetSafeStreakLocked(playerID)
			s.setCellRevealed(chunkID, cell, playerID, &allRevealedCells)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_RevealedCells{RevealedCells: allRevealedCells[chunkID]}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		} else if s.countAdjacentMines(chunkID, cell) > 0 {
			m := s.getScoreMultiplier(chunkID)
			scoreDelta = int32(math.Round(1 * m * s.streakBonus(playerID)))
			s.applyScore(playerID, scoreDelta)
			s.streaks[playerID]++
			s.bumpSafeStreakLocked(playerID)
			s.setCellRevealed(chunkID, cell, playerID, &allRevealedCells)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_RevealedCells{RevealedCells: allRevealedCells[chunkID]}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		} else {
			// Flood fill logic for empty cells
			startCount := countAllRevealedCells(allRevealedCells)
			s.floodFillReveal(chunkID, cell, playerID, &allRevealedCells)
			scoreDelta = int32(countAllRevealedCells(allRevealedCells) - startCount)
			if scoreDelta > 0 {
				m := s.getScoreMultiplier(chunkID)
				scoreDelta = int32(math.Round(float64(scoreDelta) * m * s.streakBonus(playerID)))
				s.streaks[playerID]++
				s.bumpSafeStreakLocked(playerID)
			}
			s.applyScore(playerID, scoreDelta)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_RevealedCells{RevealedCells: allRevealedCells[chunkID]}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		}
	}

	// WAL: persist all state changes for crash-recovery
	// 1) Revealed cells per chunk
	for cid, cells := range allRevealedCells {
		if cells == nil || len(cells.Cells) == 0 {
			continue
		}
		s.writeWALEntry("reveal", struct {
			ChunkID ChunkID  `json:"chunk_id"`
			Cells   []uint32 `json:"cells"`
			Owner   uint32   `json:"owner,omitempty"`
		}{ChunkID: cid, Cells: cells.Cells, Owner: playerID})
	}
	// 2) Flags placed
	for cid, placements := range allPlacedFlags {
		for _, p := range placements {
			s.writeWALEntry("flag", struct {
				ChunkID ChunkID `json:"chunk_id"`
				Cell    uint32  `json:"cell"`
				FlagID  uint32  `json:"flag_id"`
				Owner   uint32  `json:"owner,omitempty"`
			}{ChunkID: cid, Cell: p.Cell, FlagID: p.FlagID, Owner: playerID})
		}
	}
	// 3) Score update (only if changed this action)
	if scoreDelta != 0 {
		s.writeWALEntry("score_update", struct {
			PlayerID uint32 `json:"player_id"`
			Score    int32  `json:"score"`
			Streak   uint32 `json:"streak"`
		}{PlayerID: playerID, Score: s.scores[playerID], Streak: s.streaks[playerID]})
	}

	// 4) Advancement stats + newly-crossed thresholds (full-value snapshot,
	// mirroring score_update's pattern)
	var newUnlocks []*pb.AdvancementUnlocked
	var advSyncBytes []byte
	if stats := s.playerStats[playerID]; stats != nil {
		newUnlocks = s.evaluateAdvancementsLocked(playerID)
		unlockedIDs := make([]string, len(newUnlocks))
		for i, u := range newUnlocks {
			unlockedIDs[i] = u.Id
		}
		s.writeWALEntry("stats_update", struct {
			PlayerID   uint32      `json:"player_id"`
			Stats      PlayerStats `json:"stats"`
			NewUnlocks []string    `json:"new_unlocks,omitempty"`
		}{PlayerID: playerID, Stats: *stats, NewUnlocks: unlockedIDs})
		if len(newUnlocks) > 0 {
			// Rewards unlock whole shapes; a full sync keeps the client authoritative.
			advSyncBytes = s.buildAdvancementSyncLocked(playerID)
		}
	}

	s.lbDirty = true

	// phase 2: hand the mutations to the outer scope and release the lock
	revealsToSend := allRevealedCells
	flagsToSend := allPlacedFlags
	s.stateMu.Unlock()

	// phase 3: broadcast while *no* lock is held, preventing dead-locks
	if len(revealsToSend) > 0 || len(flagsToSend) > 0 {
		s.broadcastUpdates(revealsToSend, flagsToSend, playerID)
	}
	for _, u := range newUnlocks {
		s.sendToPlayer(playerID, mustProto(&pb.Msg{Payload: &pb.Msg_AdvancementUnlocked{AdvancementUnlocked: u}}))
	}
	if advSyncBytes != nil {
		s.sendToPlayer(playerID, advSyncBytes)
	}
}

// Logic Helpers

type bfsItem struct {
	CId  ChunkID
	CIdx uint32
}

func countAllRevealedCells(m map[ChunkID]*pb.RevealedCells) int {
	total := 0
	for _, cells := range m {
		total += len(cells.GetCells())
	}
	return total
}

func (s *Server) floodFillReveal(startChunk ChunkID, startCell uint32, playerID uint32, collector *map[ChunkID]*pb.RevealedCells) {
	queue := []bfsItem{{startChunk, startCell}}
	visited := make(map[bfsItem]struct{})

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if _, exists := visited[curr]; exists || s.isCellRevealed(curr.CId, curr.CIdx) || s.isCellFlagged(curr.CId, curr.CIdx) {
			continue
		}
		visited[curr] = struct{}{}
		s.setCellRevealed(curr.CId, curr.CIdx, playerID, collector)

		if s.countAdjacentMines(curr.CId, curr.CIdx) == 0 {
			currX, currY := cellIndexToXY(curr.CIdx)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					wX := int(curr.CId.X)*ChunkSize + currX + dx
					wY := int(curr.CId.Y)*ChunkSize + currY + dy
					nCId, nCIdx := worldToChunk(wX, wY)
					queue = append(queue, bfsItem{nCId, nCIdx})
				}
			}
		}
	}
}

func (s *Server) setCellRevealed(chunkID ChunkID, cell uint32, playerID uint32, collector *map[ChunkID]*pb.RevealedCells) {
	stats := s.statsForLocked(playerID)

	founded := s.chunks[chunkID] == nil
	if founded {
		s.chunks[chunkID] = &ChunkBits{}
		stats.ChunksFounded++
	}
	x, y := cellIndexToXY(cell)
	bitIndex := y*ChunkSize + x
	// only increment if this bit was previously 0
	mask := uint64(1) << (bitIndex % 64)
	if (s.chunks[chunkID][bitIndex/64] & mask) == 0 {
		s.chunks[chunkID][bitIndex/64] |= mask
		s.totalRevealed++
		stats.CellsRevealed++
		s.recordTerritoryLocked(chunkID, cell, playerID)

		// Detect the reveal that completes the chunk (all 4096 cells set).
		var revealedCount int
		for _, word := range s.chunks[chunkID] {
			revealedCount += bits.OnesCount64(word)
		}
		if revealedCount == ChunkSize*ChunkSize {
			stats.ChunksCleared++
		}

		if s.getChunkDensity(chunkID) >= 0.32 {
			stats.HighDensityReveals++
		}
	}

	if dist := chebyshevChunkDistance(chunkID); dist > stats.FurthestChunkDistance {
		stats.FurthestChunkDistance = dist
	}

	if (*collector)[chunkID] == nil {
		(*collector)[chunkID] = &pb.RevealedCells{Cells: make([]uint32, 0)}
	}
	(*collector)[chunkID].Cells = append((*collector)[chunkID].Cells, cell)

	// Update minimap tile (under the same lock)
	s.minimapOnReveal(chunkID, cell)
	s.invalidateChunkSync(chunkID)
}

// statsForLocked returns (creating if necessary) the PlayerStats for pid.
// Caller must hold stateMu (write).
func (s *Server) statsForLocked(pid uint32) *PlayerStats {
	stats := s.playerStats[pid]
	if stats == nil {
		stats = &PlayerStats{}
		s.playerStats[pid] = stats
	}
	return stats
}

// chebyshevChunkDistance returns max(|X|,|Y|) for a chunk relative to the
// origin, used as the "how far explored" advancement metric.
func chebyshevChunkDistance(chunkID ChunkID) uint32 {
	abs := func(v int64) int64 {
		if v < 0 {
			return -v
		}
		return v
	}
	ax, ay := abs(chunkID.X), abs(chunkID.Y)
	if ax > ay {
		return uint32(ax)
	}
	return uint32(ay)
}

func (s *Server) setCellFlagged(chunkID ChunkID, cell uint32, playerID uint32, flagID uint32, collector *map[ChunkID][]*pb.FlagPlacement) {
	s.flags[chunkID] = s.flags[chunkID].set(cell, Flag{FlagID: flagID, Owner: playerID})

	if (*collector)[chunkID] == nil {
		(*collector)[chunkID] = make([]*pb.FlagPlacement, 0)
	}
	placement := &pb.FlagPlacement{Cell: cell, FlagID: flagID}
	(*collector)[chunkID] = append((*collector)[chunkID], placement)

	// Update minimap tile (under the same lock)
	s.minimapOnFlag(chunkID, cell)
	s.invalidateChunkSync(chunkID)
}

// invalidateChunkSync drops a cached serialized ChunkSync for chunkID after
// a mutation. Always called under stateMu.Lock; readers (serializeChunk) are
// blocked, so no one can repopulate a stale entry before we release the
// state lock. Safe to call for chunks not currently cached.
func (s *Server) invalidateChunkSync(chunkID ChunkID) {
	s.chunkSyncCacheMu.Lock()
	delete(s.chunkSyncCache, chunkID)
	s.chunkSyncCacheMu.Unlock()
}

func (s *Server) isCellRevealed(chunkID ChunkID, cell uint32) bool {
	chunk, ok := s.chunks[chunkID]
	if !ok || chunk == nil {
		return false
	}
	bitIndex := cell
	return (chunk[bitIndex/64] & (1 << (bitIndex % 64))) != 0
}

func (s *Server) isCellFlagged(chunkID ChunkID, cell uint32) bool {
	_, ok := s.flags[chunkID].get(cell)
	return ok
}

func (s *Server) countAdjacentFlags(chunkID ChunkID, cell uint32) int {
	x, y := cellIndexToXY(cell)
	worldX, worldY := int(chunkID.X)*ChunkSize+x, int(chunkID.Y)*ChunkSize+y
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			wx, wy := worldX+dx, worldY+dy
			cid, cidx := worldToChunk(wx, wy)
			if s.isCellFlagged(cid, cidx) {
				count++
			}
		}
	}
	return count
}

// hasRevealedWithinTwo returns true if any cell within the configured Chebyshev
// distance of (worldX, worldY) is revealed. If proximity is disabled (radius < 0)
// or no cells are revealed yet, returns true to allow the first reveal.
func (s *Server) hasRevealedWithinTwo(worldX, worldY int) bool {
	// Allow the very first reveal to seed the exploration.
	if s.totalRevealed == 0 {
		return true
	}
	radius := s.proximityRadius
	if radius < 0 {
		return true
	}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			wx, wy := worldX+dx, worldY+dy
			cid, cidx := worldToChunk(wx, wy)
			if s.isCellRevealed(cid, cidx) {
				return true
			}
		}
	}
	return false
}

// Broadcast and Response Helpers

func (s *Server) sendRevealAck(playerID uint32, requestID uint64, ok bool, ack *pb.RevealAck, delta int32, chunkID ChunkID, cell uint32) {
	// If the provided ack is nil (which happens on failure), create a new one.
	if ack == nil {
		ack = &pb.RevealAck{}
	}

	ack.RequestId = requestID
	ack.Ok = ok

	if ok {
		ack.ScoreUpdate = &pb.ScoreUpdate{
			Score:   s.scores[playerID],
			Delta:   delta,
			ChunkId: &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
			Cell:    cell,
		}

		// Calculate and include user rank
		ack.UserRank = s.getUserRankUnsafe(s.scores[playerID])
	}
	s.sendToPlayer(playerID, mustProto(&pb.Msg{Payload: &pb.Msg_RevealAck{RevealAck: ack}}))
}

// broadcastUpdates fans chunk updates out to subscribers. actorID (if
// nonzero) always receives every touched chunk even without a subscription:
// flood-fills routinely spill past the viewport-based subscription margin.
func (s *Server) broadcastUpdates(reveals map[ChunkID]*pb.RevealedCells, flags map[ChunkID][]*pb.FlagPlacement, actorID uint32) {
	var wg sync.WaitGroup

	for chunkID, cells := range reveals {
		wg.Add(1)
		go func(cid ChunkID, c *pb.RevealedCells) {
			defer wg.Done()
			msg := &pb.Msg{Payload: &pb.Msg_ChunkUpdateBroadcast{ChunkUpdateBroadcast: &pb.ChunkUpdateBroadcast{
				ChunkId: &pb.ChunkID{X: cid.X, Y: cid.Y},
				Update:  &pb.ChunkUpdateBroadcast_RevealedCells{RevealedCells: c},
			}}}
			s.broadcastToChunkSubs(cid, mustProto(msg), actorID)
		}(chunkID, cells)
	}

	for chunkID, placements := range flags {
		for _, p := range placements {
			wg.Add(1)
			go func(cid ChunkID, placement *pb.FlagPlacement) {
				defer wg.Done()
				msg := &pb.Msg{Payload: &pb.Msg_ChunkUpdateBroadcast{ChunkUpdateBroadcast: &pb.ChunkUpdateBroadcast{
					ChunkId: &pb.ChunkID{X: cid.X, Y: cid.Y},
					Update:  &pb.ChunkUpdateBroadcast_FlaggedCell{FlaggedCell: placement},
				}}}
				s.broadcastToChunkSubs(cid, mustProto(msg), actorID)
			}(chunkID, p)
		}
	}

	wg.Wait()
}

func (s *Server) broadcastToChunkSubs(chunkID ChunkID, payload []byte, alwaysInclude uint32) {
	s.stateMu.RLock()
	subscribers := s.subs[chunkID]
	subList := make([]uint32, 0, len(subscribers)+1)
	actorSubscribed := false
	for pid := range subscribers {
		subList = append(subList, pid)
		if pid == alwaysInclude {
			actorSubscribed = true
		}
	}
	s.stateMu.RUnlock()

	if alwaysInclude != 0 && !actorSubscribed {
		subList = append(subList, alwaysInclude)
	}

	for _, pid := range subList {
		s.sendToPlayer(pid, payload)
	}
}
