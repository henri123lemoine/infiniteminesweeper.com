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

	if isNewPlayer {
		if !isValidUsername(hello.Name) {
			s.stateMu.Unlock()
			conn.Close()
			return
		}

		// If a player with the same name already exists, reject this new identity.
		if _, ok := s.nameToPlayerID[hello.Name]; ok {
			// Username is taken – close the connection without creating a new identity.
			s.stateMu.Unlock()
			conn.Close()
			return
		} else {
			// Create a brand new identity
			playerID = s.nextPlayerID
			s.nextPlayerID++
			sessionToken = generateSessionToken()
			s.sessionTokens[sessionToken] = playerID
			s.playerNames[playerID] = hello.Name
			s.nameToPlayerID[hello.Name] = playerID
			s.playerFlags[playerID] = hello.FlagID
			s.scores[playerID] = 0 // New players always start with a score of 0
			log.Printf("New player identity created: ID=%d, Name=%s", playerID, hello.Name)
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

	chunk, chunkExists := s.chunks[chunkID]
	flagsMap := s.flags[chunkID]
	seed64 := s.generateChunkSeed(chunkID)
	var seedBytes [8]byte
	binary.LittleEndian.PutUint64(seedBytes[:], seed64)

	var bits ChunkBits
	if chunkExists {
		bits = *chunk
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

	cs := &pb.Msg{Payload: &pb.Msg_ChunkSync{ChunkSync: &pb.ChunkSync{
		ChunkId:    &pb.ChunkID{X: chunkID.X, Y: chunkID.Y},
		Seed:       seedBytes[:],
		Reveals:    revealsBytes,
		FlagGroups: flagGroups,
	}}}

	// Unlock the state mutex *before* sending data to avoid deadlocks.
	s.stateMu.Unlock()
	s.sendToPlayer(playerID, mustProto(cs))
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
