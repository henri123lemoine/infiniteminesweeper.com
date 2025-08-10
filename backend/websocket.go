package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func debugLogMessage(msg *pb.Msg, playerID uint32) {
	if os.Getenv("MODE") != "development" {
		return
	}

	switch payload := msg.Payload.(type) {
	case *pb.Msg_Hello:
		log.Printf("[DEBUG] Player %d -> Hello: Name=%s, SessionToken=%s",
			playerID, payload.Hello.Name, payload.Hello.SessionToken)
	case *pb.Msg_Reveal:
		log.Printf("[DEBUG] Player %d -> Reveal: ChunkId=(%d,%d), Cell=%d, IsRightClick=%t, IsChord=%t",
			playerID, payload.Reveal.ChunkId.X, payload.Reveal.ChunkId.Y, payload.Reveal.Cell, payload.Reveal.IsRightClick, payload.Reveal.IsChord)
	case *pb.Msg_Subscribe:
		log.Printf("[DEBUG] Player %d -> Subscribe: ChunkId=(%d,%d)",
			playerID, payload.Subscribe.ChunkId.X, payload.Subscribe.ChunkId.Y)
	case *pb.Msg_Unsubscribe:
		log.Printf("[DEBUG] Player %d -> Unsubscribe: ChunkId=(%d,%d)",
			playerID, payload.Unsubscribe.ChunkId.X, payload.Unsubscribe.ChunkId.Y)
	case *pb.Msg_ViewUpdate:
		log.Printf("[DEBUG] Player %d -> ViewUpdate: ChunkId=(%d,%d), Cell=%d, SizeCells=(%d x %d)",
			playerID, payload.ViewUpdate.ChunkId.X, payload.ViewUpdate.ChunkId.Y, payload.ViewUpdate.Cell, payload.ViewUpdate.WidthCells, payload.ViewUpdate.HeightCells)

	default:
		log.Printf("[DEBUG] Player %d -> Unknown message type: %T", playerID, payload)
	}
}

// mustProto marshals a protobuf message and gzips it
func mustProto(m *pb.Msg) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(b); err != nil {
		panic(err)
	}
	gz.Close()
	return buf.Bytes()
}

func (s *Server) sendToPlayer(playerID uint32, data []byte) {
	s.playersMu.RLock()
	conns, exists := s.players[playerID]
	s.playersMu.RUnlock()
	if !exists {
		return
	}

	for p := range conns {
		select {
		case <-p.done:
			continue // player gone
		default:
		}

		select {
		case p.Send <- data:
			p.dropMisses = 0
		default:
			p.dropMisses++
			if p.dropMisses > 32 {
				p.Conn.Close() // force disconnect on sustained back-pressure
			}
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Connection hygiene
	conn.SetReadLimit(1 << 20)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(35 * time.Second))
		return nil
	})

	// 1. Receive and Decode the Hello Message
	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		conn.Close()
		return
	}
	pbBytes, err := io.ReadAll(gz)
	gz.Close()
	if err != nil {
		conn.Close()
		return
	}

	var msg pb.Msg
	if err := proto.Unmarshal(pbBytes, &msg); err != nil {
		conn.Close()
		return
	}
	debugLogMessage(&msg, 0) // PlayerID is not known yet

	hello := msg.GetHello()
	if hello == nil {
		conn.Close()
		return
	}

	// 2. Execute Authentication Logic from the Spec
	s.stateMu.Lock()
	var playerID uint32
	var sessionToken string
	isNewPlayer := false

	if hello.SessionToken != "" {
		pid, ok := s.sessionTokens[hello.SessionToken]
		if ok {
			playerID = pid
			sessionToken = hello.SessionToken
		} else {
			// Invalid or expired token; treat as a new player
			isNewPlayer = true
		}
	} else {
		// No token provided; treat as a new player
		isNewPlayer = true
	}

	// Handle identity: create new or update existing (name/flag)
	if isNewPlayer {
		// If name is missing/invalid, auto-assign a unique default.
		chosenName := hello.Name
		if !isValidUsername(chosenName) {
			// Use nextPlayerID and current time to craft a likely-unique default, then ensure uniqueness.
			for {
				candidate := fmt.Sprintf("User%05d", s.nextPlayerID)
				if _, ok := s.nameToPlayerID[candidate]; !ok {
					chosenName = candidate
					break
				}
				candidate = fmt.Sprintf("User%05d", time.Now().UnixNano()%100000)
				if _, ok := s.nameToPlayerID[candidate]; !ok {
					chosenName = candidate
					break
				}
			}
		} else {
			// Valid name provided by client: reject if taken by someone else.
			if _, ok := s.nameToPlayerID[chosenName]; ok {
				s.stateMu.Unlock()
				conn.Close()
				return
			}
		}

		// Create a brand new identity
		playerID = s.nextPlayerID
		s.nextPlayerID++
		sessionToken = generateSessionToken()
		s.sessionTokens[sessionToken] = playerID
		s.playerNames[playerID] = chosenName
		s.nameToPlayerID[chosenName] = playerID
		s.playerFlags[playerID] = hello.FlagID
		s.scores[playerID] = 0 // New players always start with a score of 0
		log.Printf("New player identity created: ID=%d, Name=%s", playerID, chosenName)
	} else {
		// Existing player via valid session token. Allow updating name/flag if provided.
		if hello.Name != "" && isValidUsername(hello.Name) {
			currentName := s.playerNames[playerID]
			newName := hello.Name
			if newName != currentName {
				if other, exists := s.nameToPlayerID[newName]; !exists || other == playerID {
					if currentName != "" {
						delete(s.nameToPlayerID, currentName)
					}
					s.nameToPlayerID[newName] = playerID
					s.playerNames[playerID] = newName
					s.lbDirty = true
				}
			}
		}
		// Update flag (allow zero) if different
		if s.playerFlags[playerID] != hello.FlagID {
			s.playerFlags[playerID] = hello.FlagID
			s.lbDirty = true
		}
	}

	// Read player state while still under the lock
	playerName := s.playerNames[playerID]
	playerFlag := s.playerFlags[playerID]
	initScore := s.scores[playerID]
	s.lbDirty = true
	s.stateMu.Unlock()

	// 3. Create the Player Actor and Register its Connection
	s.playersMu.Lock()
	if s.players[playerID] == nil {
		s.players[playerID] = make(map[*Player]struct{})
	}
	player := &Player{
		ID:          playerID,
		Conn:        conn,
		Send:        make(chan []byte, SendBufSize),
		Mailbox:     make(chan func(*Player), 64),
		TokenBucket: TokenBucket{tokens: 200},
		Name:        playerName,
		FlagID:      playerFlag,
		Score:       initScore,
		done:        make(chan struct{}),
	}
	s.players[playerID][player] = struct{}{}
	s.playersMu.Unlock()

	go func(p *Player) {
		for fn := range p.Mailbox {
			fn(p)
		}
	}(player)

	go s.writePump(player)
	go s.readPump(player)

	// 4. Send the Welcome Message with the Authoritative Session Token
	welcomeMsg := &pb.Msg{Payload: &pb.Msg_Welcome{Welcome: &pb.Welcome{
		SessionToken: sessionToken,
		Name:         playerName,
		Score:        initScore,
		FlagID:       playerFlag,
	}}}
	s.sendToPlayer(playerID, mustProto(welcomeMsg))

	// 5. Send Initial Leaderboard State
	s.stateMu.RLock()
	lbBytes := s.lbProto
	lbVer := s.lbVersion
	s.stateMu.RUnlock()

	if lbBytes != nil {
		s.sendToPlayer(playerID, lbBytes)
		player.LastLBVersion = lbVer
	}

	log.Printf("Player %d (%s) connected.", playerID, playerName)
}

func (s *Server) readPump(player *Player) {
	defer func() {
		s.removePlayer(player)
		player.Conn.Close()
	}()

	// The connection is now established; reset the read deadline.
	player.Conn.SetReadDeadline(time.Time{})

	for {
		_, data, err := player.Conn.ReadMessage()
		if err != nil {
			break
		}

		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		pbData, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			continue
		}

		var msg pb.Msg
		if err := proto.Unmarshal(pbData, &msg); err != nil {
			continue
		}

		debugLogMessage(&msg, player.ID)

		switch t := msg.Payload.(type) {
		case *pb.Msg_Reveal:
			r := t.Reveal
			if r.ChunkId == nil {
				continue
			}
			chunkID := ChunkID{X: r.ChunkId.X, Y: r.ChunkId.Y}

			// Serialize all actions from this player through the per-player mailbox for ordering.
			player.Mailbox <- func(*Player) {
				s.handleReveal(
					player.ID,
					r.RequestId,
					chunkID,
					r.Cell,
					r.IsRightClick,
					r.IsChord,
				)
			}
		case *pb.Msg_Subscribe:
			m := t.Subscribe
			if m.ChunkId == nil {
				continue
			}
			s.subscribeToChunk(player.ID, ChunkID{X: m.ChunkId.X, Y: m.ChunkId.Y})

		case *pb.Msg_Unsubscribe:
			m := t.Unsubscribe
			if m.ChunkId == nil {
				continue
			}
			s.unsubscribeFromChunk(player.ID, ChunkID{X: m.ChunkId.X, Y: m.ChunkId.Y})

		case *pb.Msg_ViewUpdate:
			vu := t.ViewUpdate
			if vu.ChunkId == nil {
				continue
			}
			// Record player's view
			view := PlayerView{Chunk: ChunkID{X: vu.ChunkId.X, Y: vu.ChunkId.Y}, Cell: vu.Cell}
			s.stateMu.Lock()
			s.playerViews[player.ID] = view
			s.stateMu.Unlock()

			// Determine a proportional region around the viewport.
			// Convert width/height in world cells to chunk dimensions and add a small margin.
			widthCells := int(vu.WidthCells)
			heightCells := int(vu.HeightCells)
			if widthCells <= 0 || heightCells <= 0 {
				// Fallback: default to 3x3 chunks around the center if client didn't send dims
				widthCells, heightCells = ChunkSize*3, ChunkSize*3
			}
			chunksWide := (widthCells + ChunkSize - 1) / ChunkSize
			chunksHigh := (heightCells + ChunkSize - 1) / ChunkSize
			// Aim to cover at least the viewport, plus one chunk margin around for smoother panning
			chunksWide += 2
			chunksHigh += 2

			// Center of the view in world coords
			cellX := int(vu.Cell % uint32(ChunkSize))
			cellY := int(vu.Cell / uint32(ChunkSize))
			centerWorldX := int(vu.ChunkId.X)*ChunkSize + cellX
			centerWorldY := int(vu.ChunkId.Y)*ChunkSize + cellY

			// Convert center to chunk coords
			centerChunkX := int64(centerWorldX / ChunkSize)
			centerChunkY := int64(centerWorldY / ChunkSize)

			// Compute rectangle of chunk IDs
			halfW := chunksWide / 2
			halfH := chunksHigh / 2
			startX := centerChunkX - int64(halfW)
			startY := centerChunkY - int64(halfH)
			endX := startX + int64(chunksWide) - 1
			endY := startY + int64(chunksHigh) - 1

			// Build new desired set (viewport)
			newSet := make(map[ChunkID]struct{}, chunksWide*chunksHigh)
			for cy := startY; cy <= endY; cy++ {
				for cx := startX; cx <= endX; cx++ {
					newSet[ChunkID{X: cx, Y: cy}] = struct{}{}
				}
			}

			s.stateMu.Lock()
			// Ensure per-player maps exist
			current := s.playerSubs[player.ID]
			if current == nil {
				current = make(map[ChunkID]struct{})
				s.playerSubs[player.ID] = current
			}
			lastSeen := s.playerSubLastSeen[player.ID]
			if lastSeen == nil {
				lastSeen = make(map[ChunkID]uint64)
				s.playerSubLastSeen[player.ID] = lastSeen
			}

			// Determine which chunks are new (not already subscribed)
			adds := make([]ChunkID, 0)
			for cid := range newSet {
				if _, ok := current[cid]; !ok {
					// Subscribe to this chunk
					if s.subs[cid] == nil {
						s.subs[cid] = make(map[uint32]struct{})
					}
					s.subs[cid][player.ID] = struct{}{}
					current[cid] = struct{}{}
					adds = append(adds, cid)
				}
				// Update recency for everything in view
				s.subTick++
				lastSeen[cid] = s.subTick
			}

			// Enforce LRU capacity by evicting least-recent from outside the viewport first
			over := len(current) - s.maxPlayerSubs
			if over > 0 {
				// Collect candidates not in the current view
				type pair struct {
					id ChunkID
					t  uint64
				}
				candidates := make([]pair, 0)
				for cid := range current {
					if _, inView := newSet[cid]; inView {
						continue
					}
					candidates = append(candidates, pair{id: cid, t: lastSeen[cid]})
				}
				// Sort by oldest first
				sort.Slice(candidates, func(i, j int) bool { return candidates[i].t < candidates[j].t })
				// Evict from candidates first
				for i := 0; i < len(candidates) && over > 0; i++ {
					cid := candidates[i].id
					if subs, ok := s.subs[cid]; ok {
						delete(subs, player.ID)
						if len(subs) == 0 {
							delete(s.subs, cid)
						}
					}
					delete(current, cid)
					delete(lastSeen, cid)
					over--
				}
				// If still over capacity (rare), evict oldest including in-view
				if over > 0 {
					any := make([]pair, 0, len(current))
					for cid := range current {
						any = append(any, pair{id: cid, t: lastSeen[cid]})
					}
					sort.Slice(any, func(i, j int) bool { return any[i].t < any[j].t })
					for i := 0; i < len(any) && over > 0; i++ {
						cid := any[i].id
						if subs, ok := s.subs[cid]; ok {
							delete(subs, player.ID)
							if len(subs) == 0 {
								delete(s.subs, cid)
							}
						}
						delete(current, cid)
						delete(lastSeen, cid)
						over--
					}
				}
			}

			// Only send newly added chunks; existing subs are not resent
			chunkIDs := adds
			s.stateMu.Unlock()

			if len(chunkIDs) > 0 {
				if os.Getenv("MODE") == "development" {
					log.Printf("[DEBUG] Player %d <- ChunkRegionSync add %d chunks (rect %dx%d)", player.ID, len(chunkIDs), chunksWide, chunksHigh)
				}
				s.sendChunkRegionSync(player.ID, chunkIDs)
			}
		}
	}
}

func (s *Server) writePump(player *Player) {
	defer player.Conn.Close()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	for {
		select {
		case message, ok := <-player.Send:
			if !ok {
				return
			}
			if err := player.Conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				return
			}
		case <-ping.C:
			if err := player.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) subscribeToChunk(playerID uint32, chunkID ChunkID) {
	s.stateMu.Lock()

	if s.subs[chunkID] == nil {
		s.subs[chunkID] = make(map[uint32]struct{})
	}
	s.subs[chunkID][playerID] = struct{}{}

	// Maintain reverse index and recency for LRU
	if s.playerSubs[playerID] == nil {
		s.playerSubs[playerID] = make(map[ChunkID]struct{})
	}
	if s.playerSubLastSeen[playerID] == nil {
		s.playerSubLastSeen[playerID] = make(map[ChunkID]uint64)
	}
	s.playerSubs[playerID][chunkID] = struct{}{}
	s.subTick++
	s.playerSubLastSeen[playerID][chunkID] = s.subTick

	// Enforce capacity with LRU eviction (oldest first)
	if len(s.playerSubs[playerID]) > s.maxPlayerSubs {
		type pair struct {
			id ChunkID
			t  uint64
		}
		items := make([]pair, 0, len(s.playerSubs[playerID]))
		for cid := range s.playerSubs[playerID] {
			items = append(items, pair{id: cid, t: s.playerSubLastSeen[playerID][cid]})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].t < items[j].t })
		over := len(s.playerSubs[playerID]) - s.maxPlayerSubs
		for i := 0; i < len(items) && over > 0; i++ {
			ev := items[i].id
			if ev == chunkID { // avoid evicting the one we just added if possible
				continue
			}
			if subs, ok := s.subs[ev]; ok {
				delete(subs, playerID)
				if len(subs) == 0 {
					delete(s.subs, ev)
				}
			}
			delete(s.playerSubs[playerID], ev)
			delete(s.playerSubLastSeen[playerID], ev)
			over--
		}
	}

	chunk, chunkExists := s.chunks[chunkID]
	flagsMap := s.flags[chunkID]
	seed64 := s.generateChunkSeed(chunkID)
	var seedBytes [8]byte
	binary.LittleEndian.PutUint64(seedBytes[:], seed64)

	var bits ChunkBits
	if chunkExists {
		bits = *chunk
	}

	// Treat flags as revealed for transmission (compression-friendly):
	// overlay flag positions into the reveals bitset we send.
	for cell := range flagsMap {
		// cell is 0..4095; map to (x,y) and set bit in row y at column x
		x := int(cell % ChunkSize)
		y := int(cell / ChunkSize)
		if y >= 0 && y < ChunkSize && x >= 0 && x < ChunkSize {
			bits[y] |= (1 << uint(x))
		}
	}

	// Group flags by their flagID (appearance)
	groups := make(map[uint32]*pb.RevealedCells)
	for cell, fl := range flagsMap {
		if groups[fl.FlagID] == nil {
			groups[fl.FlagID] = &pb.RevealedCells{Cells: make([]uint32, 0)}
		}
		groups[fl.FlagID].Cells = append(groups[fl.FlagID].Cells, cell)
	}

	flagGroups := make([]*pb.FlagGroup, 0, len(groups))
	for flagID, cells := range groups {
		flagGroups = append(flagGroups, &pb.FlagGroup{
			FlagID: flagID,
			Cells:  cells,
		})
	}

	revealsBytes := make([]byte, len(bits)*8)
	for i, word := range bits {
		binary.LittleEndian.PutUint64(revealsBytes[i*8:], word)
	}

	density := s.getChunkDensity(chunkID)

	cs := &pb.Msg{Payload: &pb.Msg_ChunkSync{ChunkSync: &pb.ChunkSync{
		ChunkId:    &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
		Seed:       seedBytes[:],
		Reveals:    revealsBytes,
		FlagGroups: flagGroups,
		Density:    float32(density),
	}}}

	// Unlock the state mutex *before* sending data to avoid deadlocks.
	s.stateMu.Unlock()
	s.sendToPlayer(playerID, mustProto(cs))
}

// sendChunkRegionSync gathers the state of multiple chunks and transmits them
// in a single compressed message. The provided chunkIDs should describe a
// rectangular region.
func (s *Server) sendChunkRegionSync(playerID uint32, chunkIDs []ChunkID) {
	if len(chunkIDs) == 0 {
		return
	}

	s.stateMu.Lock()

	chunks := make([]*pb.ChunkSync, 0, len(chunkIDs))
	minX, maxX := chunkIDs[0].X, chunkIDs[0].X
	minY, maxY := chunkIDs[0].Y, chunkIDs[0].Y

	for _, chunkID := range chunkIDs {
		if chunkID.X < minX {
			minX = chunkID.X
		}
		if chunkID.X > maxX {
			maxX = chunkID.X
		}
		if chunkID.Y < minY {
			minY = chunkID.Y
		}
		if chunkID.Y > maxY {
			maxY = chunkID.Y
		}

		chunk, chunkExists := s.chunks[chunkID]
		flagsMap := s.flags[chunkID]
		seed64 := s.generateChunkSeed(chunkID)
		var seedBytes [8]byte
		binary.LittleEndian.PutUint64(seedBytes[:], seed64)

		var bits ChunkBits
		if chunkExists {
			bits = *chunk
		}

		groups := make(map[uint32]*pb.RevealedCells)
		for cell, fl := range flagsMap {
			if groups[fl.FlagID] == nil {
				groups[fl.FlagID] = &pb.RevealedCells{Cells: make([]uint32, 0)}
			}
			groups[fl.FlagID].Cells = append(groups[fl.FlagID].Cells, cell)
		}

		flagGroups := make([]*pb.FlagGroup, 0, len(groups))
		for flagID, cells := range groups {
			flagGroups = append(flagGroups, &pb.FlagGroup{
				FlagID: flagID,
				Cells:  cells,
			})
		}

		revealsBytes := make([]byte, len(bits)*8)
		for i, word := range bits {
			binary.LittleEndian.PutUint64(revealsBytes[i*8:], word)
		}

		density := s.getChunkDensity(chunkID)
		chunks = append(chunks, &pb.ChunkSync{
			ChunkId:    &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
			Seed:       seedBytes[:],
			Reveals:    revealsBytes,
			FlagGroups: flagGroups,
			Density:    float32(density),
		})
	}

	s.stateMu.Unlock()

	region := &pb.ChunkRegion{Chunks: chunks}
	raw, err := proto.Marshal(region)
	if err != nil {
		return
	}

	msg := &pb.Msg{Payload: &pb.Msg_ChunkRegionSync{ChunkRegionSync: &pb.ChunkRegionSync{
		Origin: &pb.ChunkID{X: minX, Y: minY},
		Width:  uint32(maxX - minX + 1),
		Height: uint32(maxY - minY + 1),
		Chunks: raw,
	}}}

	s.sendToPlayer(playerID, mustProto(msg))
}

func (s *Server) unsubscribeFromChunk(playerID uint32, chunkID ChunkID) {
	s.stateMu.Lock()
	if subs, ok := s.subs[chunkID]; ok {
		if _, exists := subs[playerID]; exists {
			delete(subs, playerID)
			if len(subs) == 0 {
				delete(s.subs, chunkID)
			}
		}
	}
	if m, ok := s.playerSubs[playerID]; ok {
		delete(m, chunkID)
		if last, ok2 := s.playerSubLastSeen[playerID]; ok2 {
			delete(last, chunkID)
			if len(last) == 0 {
				delete(s.playerSubLastSeen, playerID)
			}
		}
		if len(m) == 0 {
			delete(s.playerSubs, playerID)
		}
	}
	s.stateMu.Unlock()
}

func (s *Server) removePlayer(p *Player) {
	s.playersMu.Lock()
	if set, ok := s.players[p.ID]; ok {
		if _, exists := set[p]; exists {
			// signal goroutines to stop enqueueing work
			close(p.done)
			// give senders 1 tick to observe <-done
			time.AfterFunc(10*time.Millisecond, func() {
				close(p.Mailbox)
				close(p.Send)
			})
			delete(set, p)
			if len(set) == 0 {
				delete(s.players, p.ID)
			}
		}
	}
	s.playersMu.Unlock()

	s.stateMu.Lock()
	for chunkID, subs := range s.subs {
		if _, exists := subs[p.ID]; exists {
			delete(subs, p.ID)
			if len(subs) == 0 {
				delete(s.subs, chunkID)
			}
		}
	}
	// Clear reverse index for this player so next connection starts fresh
	delete(s.playerSubs, p.ID)
	delete(s.playerSubLastSeen, p.ID)
	s.stateMu.Unlock()

	log.Printf("Player %d disconnected", p.ID)
}
