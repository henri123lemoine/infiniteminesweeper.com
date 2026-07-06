package main

import (
	"math"
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

// isMine determines if a cell contains a mine using the chunk's seed and density.
func (s *Server) isMine(chunkID ChunkID, cell uint32) bool {
	seed := s.generateChunkSeed(chunkID)
	cellSeed := splitmix64(seed + uint64(cell))
	// Use per-chunk density mapped to 0..100 threshold.
	// Quantize to float32 to match what is sent to the client and used for its calculations.
	d32 := float32(s.getChunkDensity(chunkID)) // [0,1] as float32 like on the wire
	if d32 < 0 {
		d32 = 0
	} else if d32 > 1 {
		d32 = 1
	}
	threshold := uint64(math.Floor(float64(d32 * 100.0)))
	if threshold > 100 {
		threshold = 100
	}
	return (cellSeed % 100) < threshold
}

func (s *Server) countAdjacentMines(chunkID ChunkID, cell uint32) int {
	x, y := cellIndexToXY(cell)
	worldX := int(chunkID.X)*ChunkSize + x
	worldY := int(chunkID.Y)*ChunkSize + y
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			wx := worldX + dx
			wy := worldY + dy
			cid, cidx := worldToChunk(wx, wy)
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
			scoreDelta = int32(math.Round(10 * m))
			s.applyScore(playerID, scoreDelta)
			s.setCellFlagged(chunkID, cell, playerID, playerFlagID, &allPlacedFlags)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_FlaggedCell{FlaggedCell: &pb.FlagPlacement{Cell: cell, FlagID: playerFlagID}}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		} else {
			// Wrong flag: deduct points, and perform a flood-fill reveal like a normal reveal.
			scoreDelta = -20 // wrong flag penalty (no positive points even if many cells revealed)
			s.applyScore(playerID, scoreDelta)
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

		// Apply multiplier only if net positive
		if scoreDelta > 0 {
			m := s.getScoreMultiplier(chunkID)
			scoreDelta = int32(math.Round(float64(scoreDelta) * m))
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
			scoreDelta = -100
			s.applyScore(playerID, scoreDelta)
			s.setCellRevealed(chunkID, cell, playerID, &allRevealedCells)
			ack := &pb.RevealAck{Outcome: &pb.RevealAck_RevealedCells{RevealedCells: allRevealedCells[chunkID]}}
			s.sendRevealAck(playerID, requestID, true, ack, scoreDelta, chunkID, cell)
		} else if s.countAdjacentMines(chunkID, cell) > 0 {
			m := s.getScoreMultiplier(chunkID)
			scoreDelta = int32(math.Round(1 * m))
			s.applyScore(playerID, scoreDelta)
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
				scoreDelta = int32(math.Round(float64(scoreDelta) * m))
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
		}{ChunkID: cid, Cells: cells.Cells})
	}
	// 2) Flags placed
	for cid, placements := range allPlacedFlags {
		for _, p := range placements {
			s.writeWALEntry("flag", struct {
				ChunkID ChunkID `json:"chunk_id"`
				Cell    uint32  `json:"cell"`
				FlagID  uint32  `json:"flag_id"`
			}{ChunkID: cid, Cell: p.Cell, FlagID: p.FlagID})
		}
	}
	// 3) Score update (only if changed this action)
	if scoreDelta != 0 {
		s.writeWALEntry("score_update", struct {
			PlayerID uint32 `json:"player_id"`
			Score    int32  `json:"score"`
		}{PlayerID: playerID, Score: s.scores[playerID]})
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
	if s.chunks[chunkID] == nil {
		s.chunks[chunkID] = &ChunkBits{}
	}
	x, y := cellIndexToXY(cell)
	bitIndex := y*ChunkSize + x
	// only increment if this bit was previously 0
	mask := uint64(1) << (bitIndex % 64)
	if (s.chunks[chunkID][bitIndex/64] & mask) == 0 {
		s.chunks[chunkID][bitIndex/64] |= mask
		s.totalRevealed++
	}

	if (*collector)[chunkID] == nil {
		(*collector)[chunkID] = &pb.RevealedCells{Cells: make([]uint32, 0)}
	}
	(*collector)[chunkID].Cells = append((*collector)[chunkID].Cells, cell)

	// Update minimap tile (under the same lock)
	s.minimapOnReveal(chunkID, cell)
}

func (s *Server) setCellFlagged(chunkID ChunkID, cell uint32, playerID uint32, flagID uint32, collector *map[ChunkID][]*pb.FlagPlacement) {
	if s.flags[chunkID] == nil {
		s.flags[chunkID] = make(map[uint32]Flag)
	}
	s.flags[chunkID][cell] = Flag{FlagID: flagID}

	if (*collector)[chunkID] == nil {
		(*collector)[chunkID] = make([]*pb.FlagPlacement, 0)
	}
	placement := &pb.FlagPlacement{Cell: cell, FlagID: flagID}
	(*collector)[chunkID] = append((*collector)[chunkID], placement)

	// Update minimap tile (under the same lock)
	s.minimapOnFlag(chunkID, cell)
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
	chunkFlags, ok := s.flags[chunkID]
	if !ok {
		return false
	}
	_, exists := chunkFlags[cell]
	return exists
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

// broadcastUpdates fans chunk updates out to subscribers. actorID (if nonzero)
// always receives every touched chunk even without a subscription: flood-fills
// are unbounded and routinely spill past the actor's viewport-based
// subscription margin — without this, the actor's own reveal silently never
// reaches them for those chunks.
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
	for pid := range subscribers {
		subList = append(subList, pid)
	}
	s.stateMu.RUnlock()

	if alwaysInclude != 0 {
		if _, ok := subscribers[alwaysInclude]; !ok {
			subList = append(subList, alwaysInclude)
		}
	}

	for _, pid := range subList {
		s.sendToPlayer(pid, payload)
	}
}
