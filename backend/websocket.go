package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

var gzipWriterPool = sync.Pool{
	New: func() any {
		// BestSpeed (level 1) is ~2-3× faster than DefaultCompression (6) for
		// our workload (protobuf, already-packed ints) at ~5-15% larger output.
		// On a single-CPU fly machine, CPU is the binding resource; a few
		// extra KB/s of network traffic is free.
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

var gzipReaderPool = sync.Pool{}

var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func debugLogMessage(msg *pb.Msg, playerID uint32) {
	if os.Getenv("MODE") != "development" {
		return
	}

	switch payload := msg.Payload.(type) {
	case *pb.Msg_Join:
		log.Printf("[DEBUG] Player %d -> Join: Name=%s, FlagID=%d, SessionToken=%s",
			playerID, payload.Join.Name, payload.Join.FlagID, payload.Join.SessionToken)
	case *pb.Msg_UpdateProfile:
		log.Printf("[DEBUG] Player %d -> UpdateProfile: Name=%s, FlagID=%d",
			playerID, payload.UpdateProfile.Name, payload.UpdateProfile.FlagID)
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
	case *pb.Msg_MinimapSubscribe:
		log.Printf("[DEBUG] Player %d -> MinimapSubscribe: %d tiles", playerID, len(payload.MinimapSubscribe.Tiles))
	case *pb.Msg_MinimapUnsubscribe:
		log.Printf("[DEBUG] Player %d -> MinimapUnsubscribe: %d tiles", playerID, len(payload.MinimapUnsubscribe.Tiles))
	case *pb.Msg_MinimapResendFull:
		tr := payload.MinimapResendFull.Tile
		if tr != nil {
			log.Printf("[DEBUG] Player %d -> MinimapResendFull: (%d,%d)", playerID, tr.X, tr.Y)
		}
	case *pb.Msg_SeedRequest:
		log.Printf("[DEBUG] Player %d -> SeedRequest: %d chunks", playerID, len(payload.SeedRequest.ChunkIds))

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

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	gz := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(buf)
	if _, err := gz.Write(b); err != nil {
		panic(err)
	}
	gz.Close()
	gzipWriterPool.Put(gz)

	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	bufferPool.Put(buf)
	return result
}

func (s *Server) sendToPlayer(playerID uint32, data []byte) {
	s.playersMu.RLock()
	conns, exists := s.players[playerID]
	if !exists {
		s.playersMu.RUnlock()
		return
	}

	// Create a slice to hold connections so we can release the lock
	var activeConns []*Player
	for p := range conns {
		select {
		case <-p.done:
			continue // player gone
		default:
			activeConns = append(activeConns, p)
		}
	}
	s.playersMu.RUnlock()

	// Send to connections without holding the lock
	for _, p := range activeConns {
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

	// All connections start in SPECTATOR state
	s.stateMu.Lock()
	// Allocate spectator ID using the high bit to avoid collisions with player IDs
	const baseSpectatorID = uint32(1 << 31)
	sid := baseSpectatorID + s.nextSpectatorID
	s.nextSpectatorID++
	s.stateMu.Unlock()

	// Create player in SPECTATOR state (no identity, view-only access)
	s.playersMu.Lock()
	if s.players[sid] == nil {
		s.players[sid] = make(map[*Player]struct{})
	}
	player := &Player{
		ID:          sid,
		Conn:        conn,
		Send:        make(chan []byte, SendBufSize),
		Mailbox:     make(chan func(*Player), 64),
		TokenBucket: TokenBucket{tokens: 200},
		Name:        "", // no identity in spectator mode
		FlagID:      0,
		Score:       0,
		State:       ClientStateSpectator, // FSM: start in spectator state
		done:        make(chan struct{}),
	}
	s.players[sid][player] = struct{}{}
	s.playersMu.Unlock()

	go func(p *Player) {
		for fn := range p.Mailbox {
			fn(p)
		}
	}(player)

	go s.writePump(player)

	// Use unified readPump that handles both spectator and player messages based on state
	go s.readPump(player)

	log.Printf("Client %d connected in SPECTATOR state", sid)
}

// handleJoin processes a Join message, transitioning client from SPECTATOR to PLAYER state
func (s *Server) handleJoin(player *Player, join *pb.Join) {
	if player.State != ClientStateSpectator {
		// Already a player or invalid state - ignore
		return
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	var playerID uint32
	var sessionToken string
	var isNewPlayer bool

	if join.SessionToken != "" {
		// Try to reconnect to existing identity
		pid, ok := s.sessionTokens[join.SessionToken]
		if ok {
			playerID = pid
			sessionToken = join.SessionToken
			isNewPlayer = false
		} else {
			// Invalid or expired token; treat as a new player
			isNewPlayer = true
		}
	} else {
		// No token provided; treat as a new player
		isNewPlayer = true
	}

	var chosenName string
	var errorMsg string

	if isNewPlayer {
		// Validate and assign name for new player
		chosenName = join.Name
		errorMsg = s.validateUsername(chosenName, 0) // Use 0 for new players since they don't have an ID yet

		if errorMsg != "" && errorMsg == "Invalid username" {
			// Auto-assign a unique default name for invalid usernames only
			for {
				candidate := fmt.Sprintf("User%05d", s.nextPlayerID)
				if _, ok := s.nameToPlayerID[candidate]; !ok {
					chosenName = candidate
					errorMsg = "" // Clear error since we found a valid auto-assigned name
					break
				}
				candidate2 := fmt.Sprintf("User%05d", time.Now().UnixNano()%100000)
				if _, ok := s.nameToPlayerID[candidate2]; !ok {
					chosenName = candidate2
					errorMsg = "" // Clear error since we found a valid auto-assigned name
					break
				}
			}
		}
		// If username taken errorMsg, we keep the error and reject the join

		if errorMsg == "" {
			// Create a brand new identity
			playerID = s.nextPlayerID
			s.nextPlayerID++
			sessionToken = generateSessionToken()
			s.sessionTokens[sessionToken] = playerID
			s.playerNames[playerID] = chosenName
			s.nameToPlayerID[chosenName] = playerID
			s.playerFlags[playerID] = join.FlagID
			s.scores[playerID] = 0 // New players always start with a score of 0
			log.Printf("New player identity created: ID=%d, Name=%s", playerID, chosenName)
		}
	} else {
		// Existing player via valid session token. Allow updating name/flag if provided.
		chosenName = s.playerNames[playerID]
		if join.Name != "" {
			currentName := s.playerNames[playerID]
			newName := join.Name
			if newName != currentName {
				errorMsg = s.validateUsername(newName, playerID)
				if errorMsg == "" {
					if currentName != "" {
						delete(s.nameToPlayerID, currentName)
					}
					s.nameToPlayerID[newName] = playerID
					s.playerNames[playerID] = newName
					chosenName = newName
					s.lbDirty = true
				}
			}
		}
		// Update flag (allow zero) if different
		if s.playerFlags[playerID] != join.FlagID {
			s.playerFlags[playerID] = join.FlagID
			s.lbDirty = true
		}
	}

	// Send response
	if errorMsg != "" {
		// Join failed
		ackMsg := &pb.Msg{Payload: &pb.Msg_JoinAck{JoinAck: &pb.JoinAck{
			Ok:       false,
			Error:    errorMsg,
			NewState: ClientStateSpectator.ToPB(),
		}}}
		s.sendToPlayer(player.ID, mustProto(ackMsg))
		return
	}

	// Join succeeded - transition to PLAYER state
	// Update player identity and state
	player.Name = chosenName
	player.FlagID = s.playerFlags[playerID]
	player.Score = s.scores[playerID]
	player.State = ClientStatePlayer

	// Move player from spectator ID space to player ID space and drop any
	// spectator-ID-keyed state. We don't migrate it — the frontend re-sends
	// ViewUpdate on every pan, so subs rebuild on the next tick.
	oldID := player.ID
	s.playersMu.Lock()
	if set := s.players[oldID]; set != nil {
		delete(set, player)
		if len(set) == 0 {
			delete(s.players, oldID)
		}
	}
	if s.players[playerID] == nil {
		s.players[playerID] = make(map[*Player]struct{})
	}
	s.players[playerID][player] = struct{}{}
	player.ID = playerID
	s.playersMu.Unlock()

	// Clear spectator ID's accumulated state (we hold stateMu here).
	delete(s.playerViews, oldID)
	delete(s.playerSubs, oldID)
	delete(s.playerSubLastSeen, oldID)
	delete(s.minimapPlayerRes, oldID)
	delete(s.minimapSubCount, oldID)
	for cid, set := range s.subs {
		if _, ok := set[oldID]; ok {
			delete(set, oldID)
			if len(set) == 0 {
				delete(s.subs, cid)
			}
		}
	}
	for cid, set := range s.minimapSubs {
		if _, ok := set[oldID]; ok {
			delete(set, oldID)
			if len(set) == 0 {
				delete(s.minimapSubs, cid)
				delete(s.minimapTiles, cid)
			}
		}
	}

	s.lbDirty = true

	// Send successful join response
	ackMsg := &pb.Msg{Payload: &pb.Msg_JoinAck{JoinAck: &pb.JoinAck{
		Ok:           true,
		SessionToken: sessionToken,
		Name:         chosenName,
		Score:        player.Score,
		FlagID:       player.FlagID,
		NewState:     ClientStatePlayer.ToPB(),
	}}}
	s.sendToPlayer(playerID, mustProto(ackMsg))

	// Send spawn hint pointing to most populated chunk
	best := s.findMostPopulatedChunk()
	centerCell := uint32(32 + 32*ChunkSize) // center of 64x64 chunk
	spawnMsg := &pb.Msg{Payload: &pb.Msg_SpawnHint{SpawnHint: &pb.SpawnHint{
		ChunkId: &pb.ChunkID{X: best.X, Y: best.Y},
		Cell:    centerCell,
	}}}
	s.sendToPlayer(playerID, mustProto(spawnMsg))

	// Send initial leaderboard state
	lbBytes := s.lbProto
	lbVer := s.lbVersion
	if lbBytes != nil {
		s.sendToPlayer(playerID, lbBytes)
		player.LastLBVersion = lbVer
	}

	log.Printf("Client %d transitioned to PLAYER state: ID=%d, Name=%s", player.ID, playerID, chosenName)
}

// handleUpdateProfile processes an UpdateProfile message from a PLAYER state client
func (s *Server) handleUpdateProfile(player *Player, update *pb.UpdateProfile) {
	if player.State != ClientStatePlayer {
		// Not a player - ignore
		return
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	var errorMsg string
	playerID := player.ID

	// Update name if provided
	if update.Name != "" {
		currentName := s.playerNames[playerID]
		newName := update.Name
		if newName != currentName {
			if validationError := s.validateUsername(newName, playerID); validationError != "" {
				errorMsg = validationError
			} else {
				if currentName != "" {
					delete(s.nameToPlayerID, currentName)
				}
				s.nameToPlayerID[newName] = playerID
				s.playerNames[playerID] = newName
				player.Name = newName
				s.lbDirty = true
			}
		}
	}

	// Update flag if different
	if s.playerFlags[playerID] != update.FlagID {
		s.playerFlags[playerID] = update.FlagID
		player.FlagID = update.FlagID
		s.lbDirty = true
	}

	// Send response
	ackMsg := &pb.Msg{Payload: &pb.Msg_UpdateAck{UpdateAck: &pb.UpdateAck{
		Ok:     errorMsg == "",
		Error:  errorMsg,
		Name:   player.Name,
		FlagID: player.FlagID,
		Score:  player.Score,
	}}}
	s.sendToPlayer(playerID, mustProto(ackMsg))

	if errorMsg == "" {
		log.Printf("Player %d (%s) updated profile", playerID, player.Name)
	}
}

// handleSeedRequest processes a SeedRequest message, returning seeds and densities for requested chunks
func (s *Server) handleSeedRequest(playerID uint32, chunkIds []*pb.ChunkID) {
	if len(chunkIds) == 0 {
		return
	}

	s.stateMu.Lock()

	seeds := make([]*pb.ChunkSeed, 0, len(chunkIds))

	for _, chunkIdPB := range chunkIds {
		if chunkIdPB == nil {
			continue
		}

		chunkID := ChunkID{X: chunkIdPB.X, Y: chunkIdPB.Y}
		seed64 := s.generateChunkSeed(chunkID)
		var seedBytes [8]byte
		binary.LittleEndian.PutUint64(seedBytes[:], seed64)

		density := s.getChunkDensity(chunkID)

		seeds = append(seeds, &pb.ChunkSeed{
			ChunkId: &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
			Seed:    seedBytes[:],
			Density: float32(density),
		})
	}

	s.stateMu.Unlock()

	if len(seeds) > 0 {
		msg := &pb.Msg{Payload: &pb.Msg_SeedResponse{SeedResponse: &pb.SeedResponse{
			Seeds: seeds,
		}}}
		s.sendToPlayer(playerID, mustProto(msg))
	}
}

func (s *Server) readPump(player *Player) {
	defer func() {
		s.removePlayer(player)
		player.Conn.Close()
	}()

	// The connection is now established; reset the read deadline.
	player.Conn.SetReadDeadline(time.Time{})

	readBuf := bufferPool.Get().(*bytes.Buffer)
	defer bufferPool.Put(readBuf)

	for {
		_, data, err := player.Conn.ReadMessage()
		if err != nil {
			break
		}

		var gz *gzip.Reader
		if pooled := gzipReaderPool.Get(); pooled != nil {
			gz = pooled.(*gzip.Reader)
			if err := gz.Reset(bytes.NewReader(data)); err != nil {
				gzipReaderPool.Put(gz)
				continue
			}
		} else {
			gz, err = gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				continue
			}
		}

		readBuf.Reset()
		_, err = readBuf.ReadFrom(gz)
		gz.Close()
		gzipReaderPool.Put(gz)
		if err != nil {
			continue
		}

		var msg pb.Msg
		if err := proto.Unmarshal(readBuf.Bytes(), &msg); err != nil {
			continue
		}

		debugLogMessage(&msg, player.ID)

		switch t := msg.Payload.(type) {
		case *pb.Msg_Join:
			// FSM: SPECTATOR -> PLAYER transition
			s.handleJoin(player, t.Join)

		case *pb.Msg_UpdateProfile:
			// FSM: Update profile while PLAYER
			s.handleUpdateProfile(player, t.UpdateProfile)

		case *pb.Msg_Reveal:
			// FSM: Only PLAYER state can send Reveal messages
			if player.State != ClientStatePlayer {
				continue
			}
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

			// Use the chunk coordinates directly from the frontend
			centerChunkX := vu.ChunkId.X
			centerChunkY := vu.ChunkId.Y

			// Compute rectangle of chunk IDs
			// Use floor division to match frontend Math.floor() behavior
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

		case *pb.Msg_MinimapSubscribe:
			m := t.MinimapSubscribe
			if m != nil {
				s.handleMinimapSubscribe(player.ID, m)
			}
		case *pb.Msg_MinimapUnsubscribe:
			m := t.MinimapUnsubscribe
			if m != nil {
				s.stateMu.Lock()
				for _, tr := range m.Tiles {
					cid := ChunkID{X: int64(tr.X), Y: int64(tr.Y)}
					if subs, ok := s.minimapSubs[cid]; ok {
						if _, had := subs[player.ID]; had {
							delete(subs, player.ID)
							if s.minimapSubCount[player.ID] > 0 {
								s.minimapSubCount[player.ID]--
							}
						}
						if len(subs) == 0 {
							delete(s.minimapSubs, cid)
							delete(s.minimapTiles, cid)
							delete(s.minimapDirtyTiles, cid)
						}
					}
				}
				s.stateMu.Unlock()
			}
		case *pb.Msg_MinimapResendFull:
			m := t.MinimapResendFull
			if m != nil && m.Tile != nil {
				cid := ChunkID{X: int64(m.Tile.X), Y: int64(m.Tile.Y)}
				s.stateMu.Lock()
				s.minimapSendFullTo(player.ID, cid)
				s.stateMu.Unlock()
			}
		case *pb.Msg_SeedRequest:
			m := t.SeedRequest
			if m != nil {
				s.handleSeedRequest(player.ID, m.ChunkIds)
			}
		}
	}
}

func (s *Server) serializeChunk(chunkID ChunkID) *pb.ChunkSync {
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
		x := int(cell % ChunkSize)
		y := int(cell / ChunkSize)
		if y >= 0 && y < ChunkSize && x >= 0 && x < ChunkSize {
			bits[y] |= (1 << uint(x))
		}
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

	return &pb.ChunkSync{
		ChunkId:    &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
		Seed:       seedBytes[:],
		Reveals:    revealsBytes,
		FlagGroups: flagGroups,
		Density:    float32(density),
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

// handleMinimapSubscribe processes a (potentially huge — ~32k tiles for an
// overlay-max-zoom client) MinimapSubscribe in two phases:
//   - Phase 1: register subs under stateMu in batches of 2000 (~1ms WLock per
//     batch) so concurrent reveals don't get blocked for the full duration.
//   - Phase 2: compute palette + send each tile under RLock per-tile.
// Cell changes between the two phases self-heal because Phase 2 reads from
// authoritative world state.
func (s *Server) handleMinimapSubscribe(playerID uint32, m *pb.SubscribeTiles) {
	resolution := m.Resolution
	if resolution != 16 && resolution != 32 && resolution != 64 {
		resolution = 64
	}

	const batchSize = 2000
	toSend := make([]ChunkID, 0, len(m.Tiles))
	for i := 0; i < len(m.Tiles); i += batchSize {
		end := min(i+batchSize, len(m.Tiles))

		s.stateMu.Lock()
		if i == 0 {
			s.minimapPlayerRes[playerID] = resolution
		}
		remaining := maxMinimapSubsPerPlayer - s.minimapSubCount[playerID]
		for _, tr := range m.Tiles[i:end] {
			if remaining <= 0 {
				break
			}
			cid := ChunkID{X: int64(tr.X), Y: int64(tr.Y)}
			if s.minimapSubs[cid] == nil {
				s.minimapSubs[cid] = make(map[uint32]struct{})
			}
			if _, already := s.minimapSubs[cid][playerID]; !already {
				s.minimapSubs[cid][playerID] = struct{}{}
				s.minimapSubCount[playerID]++
				remaining--
				toSend = append(toSend, cid)
			}
		}
		s.stateMu.Unlock()
		if remaining <= 0 {
			break
		}
	}

	for _, cid := range toSend {
		s.stateMu.RLock()
		data := s.computeTileDataAtResolution(cid, resolution)
		var version uint32
		if t := s.minimapTiles[cid]; t != nil {
			version = t.Version
		}
		s.stateMu.RUnlock()

		if data == nil {
			continue // all-unseen chunk; skip
		}
		msg := &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
			Tile:       &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
			Version:    version,
			Data:       data,
			Resolution: resolution,
		}}}
		s.sendToPlayer(playerID, mustProto(msg))
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

	cs := &pb.Msg{Payload: &pb.Msg_ChunkSync{ChunkSync: s.serializeChunk(chunkID)}}

	// Unlock the state mutex *before* sending data to avoid deadlocks.
	s.stateMu.Unlock()
	s.sendToPlayer(playerID, mustProto(cs))
}

// sendChunkRegionSync gathers the state of multiple chunks and transmits them
// in a single compressed message. The provided chunkIDs should describe a
// rectangular region.
//
// Only reads s.chunks/s.flags (stateMu), plus seed+density caches (their own
// mutexes). RLock lets multiple concurrent region-syncs from different
// panning players run in parallel — huge win because this function dominated
// the lock-contention profile (57% of mutex delay).
func (s *Server) sendChunkRegionSync(playerID uint32, chunkIDs []ChunkID) {
	if len(chunkIDs) == 0 {
		return
	}

	s.stateMu.RLock()

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
		chunks = append(chunks, s.serializeChunk(chunkID))
	}

	s.stateMu.RUnlock()

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
	hasRemainingConnections := false
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
			} else {
				hasRemainingConnections = true
			}
		}
	}
	s.playersMu.Unlock()

	// Only clear subscriptions if no other connections remain for this playerID
	if !hasRemainingConnections {
		s.stateMu.Lock()
		for chunkID, subs := range s.subs {
			if _, exists := subs[p.ID]; exists {
				delete(subs, p.ID)
				if len(subs) == 0 {
					delete(s.subs, chunkID)
				}
			}
		}
		for chunkID, subs := range s.minimapSubs {
			if _, exists := subs[p.ID]; exists {
				delete(subs, p.ID)
				if len(subs) == 0 {
					delete(s.minimapSubs, chunkID)
					delete(s.minimapTiles, chunkID)
					delete(s.minimapDirtyTiles, chunkID)
				}
			}
		}
		// Clear reverse index for this player so next connection starts fresh
		delete(s.playerSubs, p.ID)
		delete(s.playerSubLastSeen, p.ID)
		delete(s.minimapPlayerRes, p.ID)
		delete(s.minimapSubCount, p.ID)
		delete(s.playerViews, p.ID)
		s.stateMu.Unlock()
	}

	log.Printf("Player %d disconnected", p.ID)
}

// runPlayerPositionBroadcaster periodically sends nearby player positions to each client.
func (s *Server) runPlayerPositionBroadcaster() {
	const broadcastInterval = 100 * time.Millisecond
	const chunkRadius = 10

	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.broadcastPlayerPositions(chunkRadius)
	}
}

// broadcastPlayerPositions sends nearby player positions to each connected client.
func (s *Server) broadcastPlayerPositions(chunkRadius int64) {
	// First get the set of connected player IDs
	s.playersMu.RLock()
	connectedIDs := make(map[uint32]bool)
	for pid := range s.players {
		connectedIDs[pid] = true
	}
	s.playersMu.RUnlock()

	// Collect positions only for connected players or bots
	s.stateMu.RLock()
	type playerPos struct {
		id     uint32
		view   PlayerView
		flagID uint32
	}
	positions := make([]playerPos, 0, len(s.playerViews))
	for pid, view := range s.playerViews {
		// Only include if player is still connected or is a bot
		if !connectedIDs[pid] && !s.botIDs[pid] {
			continue
		}
		flagID := s.playerFlags[pid]
		positions = append(positions, playerPos{id: pid, view: view, flagID: flagID})
	}
	s.stateMu.RUnlock()

	// For each player, find nearby players and send their positions
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()

	for _, set := range s.players {
		for p := range set {
			select {
			case <-p.done:
				continue
			default:
			}

			// Find this player's position
			var myView *PlayerView
			for i := range positions {
				if positions[i].id == p.ID {
					myView = &positions[i].view
					break
				}
			}
			if myView == nil {
				continue // Player has no known view position
			}

			// Collect nearby players (excluding self)
			nearby := make([]*pb.PlayerPosition, 0)
			for _, other := range positions {
				if other.id == p.ID {
					continue // Skip self
				}
				dx := other.view.Chunk.X - myView.Chunk.X
				dy := other.view.Chunk.Y - myView.Chunk.Y
				if dx < 0 {
					dx = -dx
				}
				if dy < 0 {
					dy = -dy
				}
				if dx <= chunkRadius && dy <= chunkRadius {
					nearby = append(nearby, &pb.PlayerPosition{
						PlayerId: other.id,
						ChunkId:  &pb.ChunkID{X: other.view.Chunk.X, Y: other.view.Chunk.Y},
						Cell:     other.view.Cell,
						FlagId:   other.flagID,
					})
				}
			}

			// Always send (even if empty) so clients can clear stale positions
			msg := &pb.Msg{Payload: &pb.Msg_PlayerPositions{PlayerPositions: &pb.PlayerPositions{
				Players: nearby,
			}}}
			s.sendToPlayer(p.ID, mustProto(msg))
		}
	}
}
