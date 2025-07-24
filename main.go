package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ChunkSize = 64
)

type ChunkID struct {
	X, Y int32
}

type ChunkBits [64]uint64

type Reveal struct {
	ChunkID  ChunkID `json:"chunkId"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	PlayerID int32   `json:"playerId"`
}

type Player struct {
	ID          int32
	Conn        *websocket.Conn
	Send        chan []byte
	TokenBucket TokenBucket
	// Rate limiting for reveals
	RevealWindowStart time.Time
	RevealCount       int
}

// TokenBucket for rate limiting seed requests (200/min)
type TokenBucket struct {
	tokens     int
	lastRefill time.Time
}

func (tb *TokenBucket) Take() bool {
	now := time.Now()

	// Initialize on first use
	if tb.lastRefill.IsZero() {
		tb.lastRefill = now
		tb.tokens = 200
	}

	// Refill tokens: 200 per minute = ~3.33 per second
	elapsed := now.Sub(tb.lastRefill)
	refillAmount := int(elapsed.Seconds() * 200.0 / 60.0)

	if refillAmount > 0 {
		tb.tokens += refillAmount
		if tb.tokens > 200 {
			tb.tokens = 200
		}
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

type Message struct {
	Type string `json:"type"`
}

type RevealMessage struct {
	Type   string `json:"type"`
	ChunkX int32  `json:"chunkX"`
	ChunkY int32  `json:"chunkY"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

type SubscribeMessage struct {
	Type   string `json:"type"`
	ChunkX int32  `json:"chunkX"`
	ChunkY int32  `json:"chunkY"`
}

type SeedReq struct {
	Type   string `json:"type"`
	ChunkX int32  `json:"chunkX"`
	ChunkY int32  `json:"chunkY"`
}

type SeedResp struct {
	Type   string `json:"type"`
	ChunkX int32  `json:"chunkX"`
	ChunkY int32  `json:"chunkY"`
	Seed   uint64 `json:"seed"`
}

type ChunkSync struct {
	Type    string   `json:"type"`
	ChunkID ChunkID  `json:"chunkId"`
	Seed    uint64   `json:"seed"`
	Reveals []Reveal `json:"reveals"`
}

type RevealAck struct {
	Type    string  `json:"type"`
	ChunkID ChunkID `json:"chunkId"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	OK      bool    `json:"ok"`
	Scorer  int32   `json:"scorer,omitempty"`
}

// mustJSON helper for marshaling
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

type Server struct {
	secret []byte

	// Single mutex for all world state
	stateMu sync.RWMutex

	// World state - just what cells are revealed and by whom
	chunks     map[ChunkID]*ChunkBits    // Which cells are revealed (bitset)
	cellOwners map[ChunkID]map[int]int32 // bitIndex -> playerID
	scores     map[int32]uint32          // playerID -> reveal count
	subs       map[ChunkID]map[int32]chan Reveal

	// Players
	playersMu    sync.RWMutex
	players      map[int32]*Player
	nextPlayerID int32

	// Seed cache for performance
	seedCache   map[ChunkID]uint64
	seedCacheMu sync.RWMutex

	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		secret:       []byte("minesweeper-secret-key"),
		chunks:       make(map[ChunkID]*ChunkBits),
		cellOwners:   make(map[ChunkID]map[int]int32),
		scores:       make(map[int32]uint32),
		subs:         make(map[ChunkID]map[int32]chan Reveal),
		players:      make(map[int32]*Player),
		seedCache:    make(map[ChunkID]uint64),
		nextPlayerID: 1,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) generateChunkSeed(chunkID ChunkID) uint64 {
	// Cache seeds to avoid repeated HMAC calculations
	s.seedCacheMu.RLock()
	if seed, exists := s.seedCache[chunkID]; exists {
		s.seedCacheMu.RUnlock()
		return seed
	}
	s.seedCacheMu.RUnlock()

	h := hmac.New(sha256.New, s.secret)
	binary.Write(h, binary.LittleEndian, chunkID.X)
	binary.Write(h, binary.LittleEndian, chunkID.Y)
	hash := h.Sum(nil)
	seed := binary.LittleEndian.Uint64(hash[:8])

	s.seedCacheMu.Lock()
	s.seedCache[chunkID] = seed
	s.seedCacheMu.Unlock()

	return seed
}

// Bounds checking for reveal requests
func (s *Server) isValidCoordinate(x, y int) bool {
	return x >= 0 && x < ChunkSize && y >= 0 && y < ChunkSize
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

	// Track who revealed it
	if s.cellOwners[chunkID] == nil {
		s.cellOwners[chunkID] = make(map[int]int32)
	}
	s.cellOwners[chunkID][bitIndex] = playerID

	// Update score
	s.scores[playerID]++

	// Create reveal message
	reveal := Reveal{
		ChunkID:  chunkID,
		X:        x,
		Y:        y,
		PlayerID: playerID,
	}

	// Broadcast to 3x3 neighborhood
	s.broadcastRevealTo3x3(reveal)

	return true
}

func (s *Server) broadcastRevealTo3x3(reveal Reveal) {
	// Broadcast to 3x3 neighborhood of chunks
	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			neighborChunk := ChunkID{
				X: reveal.ChunkID.X + dx,
				Y: reveal.ChunkID.Y + dy,
			}

			if subs, exists := s.subs[neighborChunk]; exists {
				for _, ch := range subs {
					select {
					case ch <- reveal:
					default:
						// Non-blocking send, drop if channel full
					}
				}
			}
		}
	}
}

func (s *Server) sendToPlayer(playerID int32, data []byte) {
	s.playersMu.RLock()
	player, exists := s.players[playerID]
	s.playersMu.RUnlock()

	if exists {
		select {
		case player.Send <- data:
		default:
			// Drop if channel full
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Create new player
	s.playersMu.Lock()
	playerID := s.nextPlayerID
	s.nextPlayerID++
	player := &Player{
		ID:   playerID,
		Conn: conn,
		Send: make(chan []byte, 256),
		TokenBucket: TokenBucket{
			tokens: 200,
		},
		RevealWindowStart: time.Now(),
		RevealCount:       0,
	}
	s.players[playerID] = player
	s.playersMu.Unlock()

	// Send initial leaderboard
	leaderboard := s.getLeaderboard()
	s.sendToPlayer(playerID, mustJSON(leaderboard))

	// Start goroutines
	go s.writePump(player)
	go s.readPump(player)

	log.Printf("Player %d connected", playerID)
}

func (s *Server) readPump(player *Player) {
	defer func() {
		s.removePlayer(player.ID)
		player.Conn.Close()
	}()

	for {
		var rawMsg json.RawMessage
		err := player.Conn.ReadJSON(&rawMsg)
		if err != nil {
			break
		}

		var baseMsg Message
		if err := json.Unmarshal(rawMsg, &baseMsg); err != nil {
			continue
		}

		switch baseMsg.Type {
		case "reveal":
			var msg RevealMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}

			// Rate limiting
			now := time.Now()
			if now.Sub(player.RevealWindowStart) > time.Minute {
				player.RevealWindowStart = now
				player.RevealCount = 0
			}

			if player.RevealCount >= 100 {
				continue
			}

			player.RevealCount++

			chunkID := ChunkID{X: msg.ChunkX, Y: msg.ChunkY}
			ok := s.reveal(player.ID, chunkID, msg.X, msg.Y)

			ack := RevealAck{
				Type:    "revealAck",
				ChunkID: chunkID,
				X:       msg.X,
				Y:       msg.Y,
				OK:      ok,
			}

			if ok {
				ack.Scorer = player.ID
			}

			s.sendToPlayer(player.ID, mustJSON(ack))

		case "subscribe":
			var msg SubscribeMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}
			s.subscribeToChunk(player.ID, ChunkID{X: msg.ChunkX, Y: msg.ChunkY})

		case "seed":
			var msg SeedReq
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}

			if !player.TokenBucket.Take() {
				continue
			}

			seed := s.generateChunkSeed(ChunkID{msg.ChunkX, msg.ChunkY})
			resp := SeedResp{
				Type:   "seed",
				ChunkX: msg.ChunkX,
				ChunkY: msg.ChunkY,
				Seed:   seed,
			}

			s.sendToPlayer(player.ID, mustJSON(resp))
		}
	}
}

func (s *Server) writePump(player *Player) {
	defer player.Conn.Close()

	for message := range player.Send {
		if err := player.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

func (s *Server) subscribeToChunk(playerID int32, chunkID ChunkID) {
	s.stateMu.Lock()
	if s.subs[chunkID] == nil {
		s.subs[chunkID] = make(map[int32]chan Reveal)
	}

	if _, exists := s.subs[chunkID][playerID]; !exists {
		s.subs[chunkID][playerID] = make(chan Reveal, 100)
		go s.handleSubscription(playerID, chunkID, s.subs[chunkID][playerID])
	}

	// Prepare chunk sync - just revealed cells and their owners
	chunk, chunkExists := s.chunks[chunkID]
	owners := s.cellOwners[chunkID]
	seed := s.generateChunkSeed(chunkID)
	var reveals []Reveal

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
	s.stateMu.Unlock()

	msg := ChunkSync{
		Type:    "chunkSync",
		ChunkID: chunkID,
		Seed:    seed,
		Reveals: reveals,
	}

	s.sendToPlayer(playerID, mustJSON(msg))
}

func (s *Server) handleSubscription(playerID int32, chunkID ChunkID, revealCh chan Reveal) {
	for reveal := range revealCh {
		data, err := json.Marshal(reveal)
		if err != nil {
			continue
		}

		s.sendToPlayer(playerID, data)
	}
}

func (s *Server) removePlayer(playerID int32) {
	s.playersMu.Lock()
	player, exists := s.players[playerID]
	if exists {
		close(player.Send)
		delete(s.players, playerID)
	}
	s.playersMu.Unlock()

	s.stateMu.Lock()
	for chunkID, subs := range s.subs {
		if ch, exists := subs[playerID]; exists {
			close(ch)
			delete(subs, playerID)

			if len(subs) == 0 {
				delete(s.subs, chunkID)
			}
		}
	}
	s.stateMu.Unlock()

	log.Printf("Player %d disconnected", playerID)
}

func (s *Server) getLeaderboard() map[string]interface{} {
	s.stateMu.RLock()
	scoresCopy := make(map[int32]uint32, len(s.scores))
	for k, v := range s.scores {
		scoresCopy[k] = v
	}
	s.stateMu.RUnlock()

	return map[string]interface{}{
		"type":   "leaderboard",
		"scores": scoresCopy,
	}
}

func main() {
	runtime.GOMAXPROCS(1)

	server := NewServer()

	http.HandleFunc("/ws", server.handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./")))

	// Periodic leaderboard broadcast
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			leaderboard := server.getLeaderboard()
			data := mustJSON(leaderboard)

			server.playersMu.RLock()
			for _, player := range server.players {
				select {
				case player.Send <- data:
				default:
				}
			}
			server.playersMu.RUnlock()
		}
	}()

	fmt.Println("Server running at: http://localhost:8001/")
	log.Fatal(http.ListenAndServe("0.0.0.0:8001", nil))
}
