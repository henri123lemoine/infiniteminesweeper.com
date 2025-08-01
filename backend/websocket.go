package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	pb "infinite-minesweeper/backend/gen/proto"
)

func debugLogMessage(msg *pb.Msg, playerID int32) {
	if os.Getenv("MODE") != "development" {
		return
	}

	switch payload := msg.Payload.(type) {
	case *pb.Msg_Hello:
		log.Printf("[DEBUG] Player %d -> Hello: Name=%s, PlayerId=%d",
			playerID, payload.Hello.Name, payload.Hello.PlayerId)
	case *pb.Msg_Reveal:
		log.Printf("[DEBUG] Player %d -> Reveal: ChunkId=(%d,%d), X=%d, Y=%d",
			playerID, payload.Reveal.ChunkId.X, payload.Reveal.ChunkId.Y, payload.Reveal.X, payload.Reveal.Y)
	case *pb.Msg_Flag:
		log.Printf("[DEBUG] Player %d -> Flag: ChunkId=(%d,%d), X=%d, Y=%d",
			playerID, payload.Flag.ChunkId.X, payload.Flag.ChunkId.Y, payload.Flag.X, payload.Flag.Y)
	case *pb.Msg_Subscribe:
		log.Printf("[DEBUG] Player %d -> Subscribe: ChunkX=%d, ChunkY=%d",
			playerID, payload.Subscribe.ChunkX, payload.Subscribe.ChunkY)
	case *pb.Msg_Unsubscribe:
		log.Printf("[DEBUG] Player %d -> Unsubscribe: ChunkX=%d, ChunkY=%d",
			playerID, payload.Unsubscribe.ChunkX, payload.Unsubscribe.ChunkY)
	case *pb.Msg_ViewUpdate:
		log.Printf("[DEBUG] Player %d -> ViewUpdate: ViewX=%d, ViewY=%d",
			playerID, payload.ViewUpdate.ViewX, payload.ViewUpdate.ViewY)
	default:
		log.Printf("[DEBUG] Player %d -> Unknown message type: %T", playerID, payload)
	}
}

// mustProto marshals a protobuf message and gzips it only if the
// serialized size exceeds CompressThreshold.
func mustProto(m *pb.Msg) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	if len(b) < CompressThreshold {
		// Send small payloads uncompressed to avoid gzip overhead.
		return b
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(b); err != nil {
		panic(err)
	}
	gz.Close()
	return buf.Bytes()
}

func (s *Server) sendToPlayer(playerID int32, data []byte) {
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

	// connection hygiene
	conn.SetReadLimit(1 << 20)                             // max 1 MiB frame
	conn.SetReadDeadline(time.Now().Add(35 * time.Second)) // liveness timer
	conn.SetPongHandler(func(string) error {               // refresh on pong
		conn.SetReadDeadline(time.Now().Add(35 * time.Second))
		return nil
	})

	// Expect hello message with optional playerId and name
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}

	var pbBytes []byte
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			conn.Close()
			return
		}
		pbBytes, err = io.ReadAll(gz)
		gz.Close()
		if err != nil {
			conn.Close()
			return
		}
	} else {
		pbBytes = data
	}
	var msg pb.Msg
	if err := proto.Unmarshal(pbBytes, &msg); err != nil {
		conn.Close()
		return
	}
	debugLogMessage(&msg, 0)

	hello := msg.GetHello()
	if hello == nil || !isValidUsername(hello.Name) {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	playerID := hello.PlayerId
	s.stateMu.Lock()
	if playerID <= 0 || playerID >= s.nextPlayerID {
		playerID = s.nextPlayerID
		s.nextPlayerID++
	}
	// Grab any previously‑saved score before we touch the player map
	initScore := s.scores[playerID]
	s.playerNames[playerID] = hello.Name
	s.playerFlags[playerID] = hello.FlagID
	s.lbDirty = true
	s.stateMu.Unlock()

	// Create player and register connection under this id
	s.playersMu.Lock()
	if s.players[playerID] == nil {
		s.players[playerID] = make(map[*Player]struct{})
	}
	player := &Player{
		ID:                playerID,
		Conn:              conn,
		Send:              make(chan []byte, SendBufSize),
		Mailbox:           make(chan func(*Player), 64),
		TokenBucket:       TokenBucket{tokens: 200},
		RevealWindowStart: time.Now(),
		RevealCount:       0,
		Name:              hello.Name,
		FlagID:            hello.FlagID,
		Score:             initScore, // preserve previous score
		done:              make(chan struct{}),
	}
	s.players[playerID][player] = struct{}{}
	s.playersMu.Unlock()

	// start actor loop
	go func(p *Player) {
		for fn := range p.Mailbox {
			fn(p)
		}
	}(player)

	// Send initial leaderboard and welcome after goroutines start
	s.stateMu.Lock()
	if s.lbProto == nil {
		s.buildLeaderboardUnsafe()
		s.lbDirty = false
	}
	lbBytes := s.lbProto
	lbVer := s.lbVersion
	view := s.playerViews[playerID]
	s.stateMu.Unlock()

	go s.writePump(player)
	go s.readPump(player)

	welcomeMsg := &pb.Msg{Payload: &pb.Msg_Welcome{Welcome: &pb.Welcome{PlayerId: playerID, Name: hello.Name, FlagID: hello.FlagID, ViewX: view.X, ViewY: view.Y}}}
	s.sendToPlayer(playerID, mustProto(welcomeMsg))

	// auto-subscribe to surrounding chunks
	cx := int(view.X) / ChunkSize
	cy := int(view.Y) / ChunkSize
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			s.subscribeToChunk(playerID, ChunkID{X: int32(cx + dx), Y: int32(cy + dy)})
		}
	}
	s.sendToPlayer(playerID, lbBytes)
	player.LastLBVersion = lbVer

	// Send initial score (keeps existing progress)
	// Use dummy coordinates since this is not associated with a specific cell action
	s.sendScoreUpdate(playerID, initScore, 0, 0, 0)

	log.Printf("Player %d connected", playerID)
}

func (s *Server) readPump(player *Player) {
	defer func() {
		s.removePlayer(player)
		player.Conn.Close()
	}()

	for {
		_, data, err := player.Conn.ReadMessage()
		if err != nil {
			break
		}

		var pbData []byte
		if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
			gz, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				continue
			}
			pbData, err = io.ReadAll(gz)
			gz.Close()
			if err != nil {
				continue
			}
		} else {
			pbData = data
		}

		var msg pb.Msg
		if err := proto.Unmarshal(pbData, &msg); err != nil {
			continue
		}

		debugLogMessage(&msg, player.ID)

		switch t := msg.Payload.(type) {
		case *pb.Msg_Reveal:
			r := t.Reveal

			// Rate limiting
			player.Mailbox <- func(pl *Player) {
				now := time.Now()
				if now.Sub(pl.RevealWindowStart) > time.Minute {
					pl.RevealWindowStart = now
					pl.RevealCount = 0
				}
				if pl.RevealCount >= MaxRevealsPerMin {
					pl.SusRevealOverflow++
				}
				pl.RevealCount++
			}

			chunkID := ChunkID{X: r.ChunkId.X, Y: r.ChunkId.Y}
			var ok bool
			if r.Flow {
				ok = s.floodReveal(player.ID, chunkID, int(r.X), int(r.Y))
			} else {
				ok = s.reveal(player.ID, chunkID, int(r.X), int(r.Y))
			}

			ack := &pb.Msg{Payload: &pb.Msg_RevealAck{RevealAck: &pb.RevealAck{Ok: ok}}}
			s.sendToPlayer(player.ID, mustProto(ack))

		case *pb.Msg_Flag:
			m := t.Flag
			chunkID := ChunkID{X: m.ChunkId.X, Y: m.ChunkId.Y}
			ok := s.flag(player.ID, chunkID, int(m.X), int(m.Y))
			ack := &pb.Msg{Payload: &pb.Msg_FlagAck{FlagAck: &pb.FlagAck{Ok: ok}}}
			s.sendToPlayer(player.ID, mustProto(ack))

		case *pb.Msg_Subscribe:
			m := t.Subscribe
			s.subscribeToChunk(player.ID, ChunkID{X: m.ChunkX, Y: m.ChunkY})

		case *pb.Msg_Unsubscribe:
			m := t.Unsubscribe
			s.unsubscribeFromChunk(player.ID, ChunkID{X: m.ChunkX, Y: m.ChunkY})
		case *pb.Msg_ViewUpdate:
			m := t.ViewUpdate
			s.stateMu.Lock()
			s.playerViews[player.ID] = struct{ X, Y int32 }{X: m.ViewX, Y: m.ViewY}
			s.stateMu.Unlock()
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

func (s *Server) subscribeToChunk(playerID int32, chunkID ChunkID) {
	s.stateMu.Lock()
	if s.subs[chunkID] == nil {
		s.subs[chunkID] = make(map[int32]struct{})
	}
	s.subs[chunkID][playerID] = struct{}{}

	// Prepare chunk sync - just revealed cells and their owners
	chunk, chunkExists := s.chunks[chunkID]
	owners := s.cellOwners[chunkID]
	flagsMap := s.flags[chunkID]
	seed64 := s.generateChunkSeed(chunkID)
	var seedBytes [8]byte
	binary.LittleEndian.PutUint64(seedBytes[:], seed64)
	var reveals []Reveal
	var flags []Flag

	if chunkExists {
		for y := 0; y < ChunkSize; y++ {
			for x := 0; x < ChunkSize; x++ {
				bitIndex := y*ChunkSize + x
				wordIndex := bitIndex / 64
				bitOffset := bitIndex % 64

				if chunk[wordIndex]&(1<<bitOffset) != 0 {
					var ownerID int32 = 0
					if owners != nil {
						if owner, exists := owners[bitIndex]; exists {
							ownerID = owner
						}
					}

					reveals = append(reveals, Reveal{
						ChunkID:  chunkID,
						X:        x,
						Y:        y,
						PlayerID: ownerID,
					})
				}
			}
		}
	}

	// Add flags for this chunk
	for _, flag := range flagsMap {
		flags = append(flags, flag)
	}
	s.stateMu.Unlock()

	cs := &pb.Msg{Payload: &pb.Msg_ChunkSync{ChunkSync: &pb.ChunkSync{
		ChunkId: &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
		Seed:    seedBytes[:],
	}}}
	for _, rv := range reveals {
		cs.GetChunkSync().Reveals = append(cs.GetChunkSync().Reveals, &pb.Reveal{
			ChunkId: &pb.ChunkID{X: rv.ChunkID.X, Y: rv.ChunkID.Y},
			X:       int32(rv.X), Y: int32(rv.Y), PlayerId: rv.PlayerID,
		})
	}
	for _, fl := range flags {
		cs.GetChunkSync().Flags = append(cs.GetChunkSync().Flags, &pb.Flag{
			ChunkId: &pb.ChunkID{X: fl.ChunkID.X, Y: fl.ChunkID.Y},
			X:       int32(fl.X), Y: int32(fl.Y), PlayerId: fl.PlayerID, FlagID: fl.FlagID,
		})
	}
	s.sendToPlayer(playerID, mustProto(cs))
}

func (s *Server) unsubscribeFromChunk(playerID int32, chunkID ChunkID) {
	s.stateMu.Lock()
	if subs, ok := s.subs[chunkID]; ok {
		if _, exists := subs[playerID]; exists {
			delete(subs, playerID)
			if len(subs) == 0 {
				delete(s.subs, chunkID)
			}
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
	s.stateMu.Unlock()

	log.Printf("Player %d disconnected", p.ID)
}
