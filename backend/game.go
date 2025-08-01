package main

import (
	"encoding/binary"
	pb "infinite-minesweeper/backend/proto"
	"regexp"
	"time"
)

// Helpers

func worldToChunk(x, y int) (ChunkID, int, int) {
	cx := x / ChunkSize
	if x < 0 && x%ChunkSize != 0 {
		cx--
	}
	cy := y / ChunkSize
	if y < 0 && y%ChunkSize != 0 {
		cy--
	}
	lx := x - cx*ChunkSize
	ly := y - cy*ChunkSize
	return ChunkID{int32(cx), int32(cy)}, lx, ly
}

// splitmix64 - deterministic PRNG for bomb placement (matches frontend)
func splitmix64(state uint64) uint64 {
	state += 0x9e3779b97f4a7c15
	state = (state ^ (state >> 30)) * 0xbf58476d1ce4e5b9
	state = (state ^ (state >> 27)) * 0x94d049bb133111eb
	return state ^ (state >> 31)
}

// Game logic

// isMine determines if a cell contains a mine using the same logic as frontend
func (s *Server) isMine(seed uint64, x, y int) bool {
	cellSeed := splitmix64(seed + uint64(y*ChunkSize+x))
	return (cellSeed % 100) < MineCount
}

func (s *Server) countAdjacentMines(chunkID ChunkID, x, y int) int {
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
			cid, lx, ly := worldToChunk(wx, wy)
			seed := s.generateChunkSeed(cid)
			if s.isMine(seed, lx, ly) {
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

var usernameRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,20}$`)

func isValidUsername(name string) bool {
	return usernameRegex.MatchString(name)
}

// Player action handlers

// TODO: validate, and possibly add speed score
// New scoring system:
// - reveals = 1pt
// - correct flags = +flag-in-a-row multiplier (capped at 20x)
// - wrong flags = -20pts (and resets multiplier)
// - bombs = -100pts
// - minimum score = 0

func (s *Server) flag(playerID int32, chunkID ChunkID, x, y int) bool {
	// Bounds check
	if !s.isValidCoordinate(x, y) {
		return false
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	// Check if cell is already revealed
	chunk := s.chunks[chunkID]
	if chunk != nil {
		bitIndex := y*ChunkSize + x
		wordIndex := bitIndex / 64
		bitOffset := bitIndex % 64
		if chunk[wordIndex]&(1<<bitOffset) != 0 {
			return false // Can't flag revealed cells
		}
	}

	// Check if cell is already flagged
	bitIndex := y*ChunkSize + x
	if s.flags[chunkID] != nil {
		if _, exists := s.flags[chunkID][bitIndex]; exists {
			return false // Already flagged
		}
	}

	// Get player info (atomically under the same read‑lock)
	s.playersMu.RLock()
	var player *Player
	playerColor := s.playerColors[playerID]
	if conns, ok := s.players[playerID]; ok {
		for p := range conns {
			player = p
			break
		}
	}
	s.playersMu.RUnlock()
	if player == nil {
		return false
	}

	// Determine if this cell contains a mine
	seed := s.generateChunkSeed(chunkID)
	isMine := s.isMine(seed, x, y)

	res := make(chan int32, 1)
	player.Mailbox <- func(pl *Player) {
		if isMine {
			// Correct flag: award points with multiplier
			pl.FlagsInARow++
			multiplier := pl.FlagsInARow
			if multiplier > 20 {
				multiplier = 20
			}
			pl.Score += int32(multiplier)

			// Store the flag
			if s.flags[chunkID] == nil {
				s.flags[chunkID] = make(map[int]Flag)
			}
			flag := Flag{
				ChunkID:  chunkID,
				X:        x,
				Y:        y,
				PlayerID: playerID,
				Color:    playerColor,
			}
			s.flags[chunkID][bitIndex] = flag

			// Write to WAL
			s.writeWALEntry("flag", flag)

			// Broadcast flag to subscribers
			s.broadcastFlagTo3x3(flag)

		} else {
			// Wrong flag: penalize and auto-reveal
			pl.Score -= 20
			pl.FlagsInARow = 0 // Reset multiplier

			// Auto-reveal the cell (like a normal reveal but with penalty already applied)
			if chunk == nil {
				chunk = &ChunkBits{}
				s.chunks[chunkID] = chunk
			}
			wordIndex := bitIndex / 64
			bitOffset := bitIndex % 64
			chunk[wordIndex] |= 1 << bitOffset

			// Track who revealed it
			if s.cellOwners[chunkID] == nil {
				s.cellOwners[chunkID] = make(map[int]int32)
			}
			s.cellOwners[chunkID][bitIndex] = playerID

			// Write to WAL (auto-reveal from wrong flag)
			reveal := Reveal{ChunkID: chunkID, X: x, Y: y, PlayerID: playerID}
			s.writeWALEntry("reveal", reveal)

			// Broadcast the auto-reveal to all players who can see this chunk
			s.broadcastRevealTo3x3(reveal)
		}

		pl.LastActionTime = time.Now()

		// Cap score at 0
		if pl.Score < 0 {
			pl.Score = 0
		}

		res <- pl.Score
	}
	score := <-res

	old := s.scores[playerID]
	delta := score - old
	s.scores[playerID] = score

	// Mark leaderboard as dirty for persistence
	s.lbDirty = true

	// Send score update to player
	worldX := int(chunkID.X)*ChunkSize + x
	worldY := int(chunkID.Y)*ChunkSize + y
	s.sendScoreUpdate(playerID, score, worldX, worldY, delta)

	return true
}

func (s *Server) reveal(playerID int32, chunkID ChunkID, x, y int) bool {
	// Bounds check
	if !s.isValidCoordinate(x, y) {
		return false
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	// Get or create chunk
	chunk, exists := s.chunks[chunkID]
	if !exists {
		chunk = &ChunkBits{}
		s.chunks[chunkID] = chunk
	}

	// Check if already revealed
	bitIndex := y*ChunkSize + x
	wordIndex := bitIndex / 64
	bitOffset := bitIndex % 64

	if chunk[wordIndex]&(1<<bitOffset) != 0 {
		return false // Already revealed
	}

	// Set the bit (mark as revealed)
	chunk[wordIndex] |= 1 << bitOffset

	// Determine if this cell is a mine
	seed := s.generateChunkSeed(chunkID)
	isMine := s.isMine(seed, x, y)

	// Update player score based on reveal
	s.playersMu.RLock()
	var player *Player
	if conns, ok := s.players[playerID]; ok {
		for p := range conns { // pick first connection while lock is held
			player = p
			break
		}
	}
	s.playersMu.RUnlock()

	if player != nil {
		res := make(chan int32, 1)
		oldScore := make(chan int32, 1)
		player.Mailbox <- func(pl *Player) {
			oldScore <- pl.Score // Send old score before changes
			if isMine {
				pl.Score -= 100 // bomb penalty
				pl.FlagsInARow = 0
			} else {
				pl.Score += 1 // normal reveal
			}
			if pl.Score < 0 {
				pl.Score = 0
			}
			pl.LastActionTime = time.Now()
			res <- pl.Score
		}
		prevScore := <-oldScore
		newScore := <-res
		worldX := int(chunkID.X)*ChunkSize + x
		worldY := int(chunkID.Y)*ChunkSize + y
		delta := newScore - prevScore
		s.sendScoreUpdate(playerID, newScore, worldX, worldY, delta)
	}

	// Track who revealed it
	if s.cellOwners[chunkID] == nil {
		s.cellOwners[chunkID] = make(map[int]int32)
	}
	s.cellOwners[chunkID][bitIndex] = playerID

	// Update leaderboard score
	if player != nil {
		s.scores[playerID] = player.Score
	} else {
		// For tests or cases where player is not registered, just increment score
		s.scores[playerID]++
	}
	s.lbDirty = true

	// Create reveal message
	reveal := Reveal{
		ChunkID:  chunkID,
		X:        x,
		Y:        y,
		PlayerID: playerID,
	}

	// Write to WAL
	s.writeWALEntry("reveal", reveal)

	// Broadcast to 3x3 neighborhood
	s.broadcastRevealTo3x3(reveal)

	// persistence: mark dirty so that next snapshot loop rewrites file
	s.lbDirty = true
	return true
}

func (s *Server) floodReveal(playerID int32, chunkID ChunkID, x, y int) bool {
	if !s.isValidCoordinate(x, y) {
		return false
	}

	worldX := int(chunkID.X)*ChunkSize + x
	worldY := int(chunkID.Y)*ChunkSize + y

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	chunk, exists := s.chunks[chunkID]
	if !exists {
		chunk = &ChunkBits{}
		s.chunks[chunkID] = chunk
	}
	bitIndex := y*ChunkSize + x
	wordIndex := bitIndex / 64
	bitOffset := bitIndex % 64
	if chunk[wordIndex]&(1<<bitOffset) != 0 {
		return false
	}

	seed := s.generateChunkSeed(chunkID)
	if s.isMine(seed, x, y) {
		return false
	}
	if s.countAdjacentMines(chunkID, x, y) != 0 {
		return false
	}

	type cell struct{ wx, wy int }
	queue := []cell{{worldX, worldY}}
	visited := make(map[cell]struct{})
	var reveals []Reveal

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if _, ok := visited[c]; ok {
			continue
		}
		visited[c] = struct{}{}

		cid, lx, ly := worldToChunk(c.wx, c.wy)
		ch := s.chunks[cid]
		if ch == nil {
			ch = &ChunkBits{}
			s.chunks[cid] = ch
		}
		bIdx := ly*ChunkSize + lx
		wIdx := bIdx / 64
		bOff := bIdx % 64
		if ch[wIdx]&(1<<bOff) != 0 {
			continue
		}

		seed := s.generateChunkSeed(cid)
		if s.isMine(seed, lx, ly) {
			continue
		}

		ch[wIdx] |= 1 << bOff

		if s.cellOwners[cid] == nil {
			s.cellOwners[cid] = make(map[int]int32)
		}
		s.cellOwners[cid][bIdx] = playerID

		reveals = append(reveals, Reveal{ChunkID: cid, X: lx, Y: ly, PlayerID: playerID})

		if s.countAdjacentMines(cid, lx, ly) == 0 {
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					queue = append(queue, cell{c.wx + dx, c.wy + dy})
				}
			}
		}
	}

	if len(reveals) == 0 {
		return false
	}

	s.playersMu.RLock()
	var player *Player
	if conns, ok := s.players[playerID]; ok {
		for p := range conns {
			player = p
			break
		}
	}
	s.playersMu.RUnlock()

	var delta int32
	if player != nil {
		res := make(chan int32, 1)
		old := make(chan int32, 1)
		player.Mailbox <- func(pl *Player) {
			old <- pl.Score
			pl.Score += int32(len(reveals))
			if pl.Score < 0 {
				pl.Score = 0
			}
			pl.LastActionTime = time.Now()
			res <- pl.Score
		}
		prev := <-old
		newScore := <-res
		delta = newScore - prev
		s.scores[playerID] = newScore
		s.lbDirty = true
		s.sendScoreUpdate(playerID, newScore, worldX, worldY, delta)
	} else {
		s.scores[playerID] += int32(len(reveals))
		s.lbDirty = true
	}

	for _, rv := range reveals {
		s.writeWALEntry("reveal", rv)
	}

	s.broadcastRevealBatch(reveals)

	return true
}

// Broadcast helpers

func (s *Server) broadcastFlagTo3x3(flag Flag) {
	// Caller already holds stateMu
	pbFlag := &pb.Msg{Payload: &pb.Msg_Flag{Flag: &pb.Flag{
		ChunkId: &pb.ChunkID{X: flag.ChunkID.X, Y: flag.ChunkID.Y},
		X:       int32(flag.X), Y: int32(flag.Y),
		PlayerId: flag.PlayerID, Color: flag.Color,
	}}}
	payload := mustProto(pbFlag)
	sent := make(map[int32]struct{})

	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			chk := ChunkID{flag.ChunkID.X + dx, flag.ChunkID.Y + dy}
			for pid := range s.subs[chk] {
				if _, dup := sent[pid]; dup {
					continue
				}
				s.sendToPlayer(pid, payload)
				sent[pid] = struct{}{}
			}
		}
	}
}

func (s *Server) broadcastRevealTo3x3(reveal Reveal) {
	// Caller already holds stateMu
	pbReveal := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
		ChunkId: &pb.ChunkID{X: reveal.ChunkID.X, Y: reveal.ChunkID.Y},
		X:       int32(reveal.X), Y: int32(reveal.Y), PlayerId: reveal.PlayerID,
	}}}
	payload := mustProto(pbReveal)
	sent := make(map[int32]struct{})

	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			chk := ChunkID{reveal.ChunkID.X + dx, reveal.ChunkID.Y + dy}
			for pid := range s.subs[chk] {
				if _, dup := sent[pid]; dup {
					continue
				}
				s.sendToPlayer(pid, payload)
				sent[pid] = struct{}{}
			}
		}
	}
}

func (s *Server) broadcastRevealBatch(reveals []Reveal) {
	// Caller already holds stateMu
	byChunk := make(map[ChunkID][]Reveal)
	for _, r := range reveals {
		byChunk[r.ChunkID] = append(byChunk[r.ChunkID], r)
	}

	for chunkID, list := range byChunk {
		seed := s.generateChunkSeed(chunkID)
		var seedBytes [8]byte
		binary.LittleEndian.PutUint64(seedBytes[:], seed)
		msg := &pb.Msg{Payload: &pb.Msg_ChunkSync{ChunkSync: &pb.ChunkSync{
			ChunkId: &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
			Seed:    seedBytes[:],
		}}}
		for _, r := range list {
			msg.GetChunkSync().Reveals = append(msg.GetChunkSync().Reveals, &pb.Reveal{
				ChunkId:  &pb.ChunkID{X: r.ChunkID.X, Y: r.ChunkID.Y},
				X:        int32(r.X),
				Y:        int32(r.Y),
				PlayerId: r.PlayerID,
			})
		}
		payload := mustProto(msg)
		sent := make(map[int32]struct{})
		for dy := int32(-1); dy <= 1; dy++ {
			for dx := int32(-1); dx <= 1; dx++ {
				chk := ChunkID{chunkID.X + dx, chunkID.Y + dy}
				for pid := range s.subs[chk] {
					if _, dup := sent[pid]; dup {
						continue
					}
					s.sendToPlayer(pid, payload)
					sent[pid] = struct{}{}
				}
			}
		}
	}
}
