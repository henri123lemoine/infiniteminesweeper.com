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
	MineCount = 10 // 10% of cells are mines (cellSeed % 100 < 10)
)

type ChunkID struct {
	X, Y int32 // Actually fits in 8 bytes, not 4 as comment claimed
}

type ChunkBits [64]uint64

type Reveal struct {
	ChunkID       ChunkID `json:"chunkId"`
	X             int     `json:"x"`
	Y             int     `json:"y"`
	IsMine        bool    `json:"isMine"`
	AdjacentMines int     `json:"adjacentMines"`
	PlayerID      int32   `json:"playerId"`
}

type Player struct {
	ID          int32
	Conn        *websocket.Conn
	Send        chan []byte
	TokenBucket TokenBucket
	// Separate tracking for reveal rate limiting
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

	// Single mutex for all world state to prevent deadlocks
	stateMu sync.RWMutex

	// World state - all protected by stateMu
	chunks     map[ChunkID]*ChunkBits
	cellOwners map[ChunkID]map[int]int32 // Direct int32 instead of *int32
	scores     map[int32]uint32
	subs       map[ChunkID]map[int32]chan Reveal

	// Players - separate mutex since they have different access patterns
	playersMu    sync.RWMutex
	players      map[int32]*Player
	nextPlayerID int32

	// Seed cache for performance optimization
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

func splitmix64(state uint64) uint64 {
	state += 0x9e3779b97f4a7c15
	state = (state ^ (state >> 30)) * 0xbf58476d1ce4e5b9
	state = (state ^ (state >> 27)) * 0x94d049bb133111eb
	return state ^ (state >> 31)
}

// Pass seed to avoid repeated HMAC calls
func (s *Server) isMineWithSeed(seed uint64, x, y int) bool {
	cellSeed := splitmix64(seed + uint64(y*ChunkSize+x))
	return (cellSeed % 100) < MineCount
}

func (s *Server) isMine(chunkID ChunkID, x, y int) bool {
	seed := s.generateChunkSeed(chunkID)
	return s.isMineWithSeed(seed, x, y)
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

	// Single lock for all state mutations
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

	// Atomic state update - set bit, track ownership, update score all in one critical section
	chunk[wordIndex] |= 1 << bitOffset

	// Track cell ownership
	if s.cellOwners[chunkID] == nil {
		s.cellOwners[chunkID] = make(map[int]int32)
	}
	s.cellOwners[chunkID][bitIndex] = playerID // Direct value, not pointer

	// Update score
	s.scores[playerID]++

	// Calculate mine info with cached seed
	seed := s.generateChunkSeed(chunkID)
	isMine := s.isMineWithSeed(seed, x, y)
	adjacentMines := 0
	if !isMine {
		adjacentMines = s.countAdjacentMinesWithSeed(chunkID, x, y, seed)
	}

	// Create reveal message
	reveal := Reveal{
		ChunkID:       chunkID,
		X:             x,
		Y:             y,
		IsMine:        isMine,
		AdjacentMines: adjacentMines,
		PlayerID:      playerID,
	}

	// Broadcast to 3x3 neighborhood (still under stateMu)
	s.broadcastRevealTo3x3(reveal)

	return true
}

// Optimized version that reuses seeds and avoids repeated HMAC
func (s *Server) countAdjacentMinesWithSeed(chunkID ChunkID, x, y int, centerSeed uint64) int {
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}

			newX, newY := x+dx, y+dy
			currentChunk := chunkID

			// Handle cross-chunk boundaries
			if newX < 0 {
				currentChunk.X--
				newX += ChunkSize
			} else if newX >= ChunkSize {
				currentChunk.X++
				newX -= ChunkSize
			}

			if newY < 0 {
				currentChunk.Y--
				newY += ChunkSize
			} else if newY >= ChunkSize {
				currentChunk.Y++
				newY -= ChunkSize
			}

			var neighborSeed uint64
			if currentChunk == chunkID {
				neighborSeed = centerSeed
			} else {
				neighborSeed = s.generateChunkSeed(currentChunk)
			}

			if s.isMineWithSeed(neighborSeed, newX, newY) {
				count++
			}
		}
	}
	return count
}

func (s *Server) countAdjacentMines(chunkID ChunkID, x, y int) int {
	seed := s.generateChunkSeed(chunkID)
	return s.countAdjacentMinesWithSeed(chunkID, x, y, seed)
}

// This now assumes stateMu is already held by caller
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

// Helper to safely send to a player
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
			tokens: 200, // Start with full bucket
		},
		RevealWindowStart: time.Now(),
		RevealCount:       0,
	}
	s.players[playerID] = player
	s.playersMu.Unlock()

	// Send initial leaderboard on connect
	leaderboard := s.getLeaderboard()
	s.sendToPlayer(playerID, mustJSON(leaderboard))

	// Start goroutines for this player
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

			// Reveal rate limiting with separate window tracking
			now := time.Now()
			if now.Sub(player.RevealWindowStart) > time.Minute {
				// Reset window
				player.RevealWindowStart = now
				player.RevealCount = 0
			}

			if player.RevealCount >= 100 {
				continue // Drop if over limit in current window
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
				continue // Rate limit exceeded, drop request
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

	// Create buffered channel for this player's subscription
	if _, exists := s.subs[chunkID][playerID]; !exists {
		s.subs[chunkID][playerID] = make(chan Reveal, 100)

		// Start a goroutine to handle reveals for this subscription
		go s.handleSubscription(playerID, chunkID, s.subs[chunkID][playerID])
	}

	// Prepare chunk sync data while holding the lock
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
					isMine := s.isMineWithSeed(seed, x, y)
					adjacentMines := 0
					if !isMine {
						adjacentMines = s.countAdjacentMinesWithSeed(chunkID, x, y, seed)
					}

					// Include proper player attribution
					var ownerID int32 = 0 // Default for historical cells without tracking
					if owners != nil {
						if owner, exists := owners[bitIndex]; exists {
							ownerID = owner
						}
					}

					reveals = append(reveals, Reveal{
						ChunkID:       chunkID,
						X:             x,
						Y:             y,
						IsMine:        isMine,
						AdjacentMines: adjacentMines,
						PlayerID:      ownerID,
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

	// Clean up subscriptions under proper locking
	s.stateMu.Lock()
	for chunkID, subs := range s.subs {
		if ch, exists := subs[playerID]; exists {
			close(ch)
			delete(subs, playerID)

			// Clean up empty subscription maps
			if len(subs) == 0 {
				delete(s.subs, chunkID)
			}
		}
	}
	s.stateMu.Unlock()

	log.Printf("Player %d disconnected", playerID)
}

// Copy scores map to prevent data races during JSON marshaling
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
	runtime.GOMAXPROCS(1) // Single core as specified

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
					// Drop if channel full
				}
			}
			server.playersMu.RUnlock()
		}
	}()

	fmt.Println("Server running at: http://localhost:8001/")
	log.Fatal(http.ListenAndServe("0.0.0.0:8001", nil))
}
