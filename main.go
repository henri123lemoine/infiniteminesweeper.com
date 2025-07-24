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
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ChunkSize        = 64
	SendBufSize      = 4096  // outbound msgs kept per player before back‑pressure
	MaxRevealsPerMin = 10000 // relaxed flood‑fill budget
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

	// Leaderboard version the player has already received
	LastLBVersion uint64

	// Suspicion metrics (for admin dashboards, nothing enforced server‑side)
	SusRevealOverflow int // # of extra reveals processed beyond MaxRevealsPerMin
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

type UnsubscribeMessage struct {
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
	chunks     map[ChunkID]*ChunkBits         // Which cells are revealed (bitset)
	cellOwners map[ChunkID]map[int]int32      // bitIndex -> playerID
	scores     map[int32]uint32               // playerID -> reveal count
	subs       map[ChunkID]map[int32]struct{} // who wants reveals for each chunk

	// --- leaderboard cache ---
	lbVersion uint64
	lbJSON    []byte
	lbDirty   bool

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
		subs:         make(map[ChunkID]map[int32]struct{}),
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

// Leaderboard helpers

type lbEntry struct {
	PlayerID int32  `json:"playerId"`
	Score    string `json:"score"`
}

func formatScore(n uint32) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return strconv.Itoa(int(n))
	}
}

// Assumes caller holds s.stateMu (write lock)
func (s *Server) buildLeaderboardUnsafe() {
	// Collect & sort
	entries := make([]lbEntry, 0, len(s.scores))
	for pid, sc := range s.scores {
		entries = append(entries, lbEntry{PlayerID: pid, Score: formatScore(sc)})
	}
	// Sort by the real uint32 score
	sort.Slice(entries, func(i, j int) bool {
		return s.scores[entries[i].PlayerID] > s.scores[entries[j].PlayerID]
	})
	if len(entries) > 20 {
		entries = entries[:20]
	}

	lb := struct {
		Type    string    `json:"type"`
		Version uint64    `json:"version"`
		Entries []lbEntry `json:"entries"`
	}{
		Type:    "leaderboard",
		Version: s.lbVersion + 1,
		Entries: entries,
	}

	s.lbVersion++
	s.lbJSON = mustJSON(lb)
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

	// Update score & mark leaderboard dirty
	s.scores[playerID]++
	s.lbDirty = true

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
	// Caller already holds stateMu (inside reveal())
	payload := mustJSON(reveal) // marshal once
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

func (s *Server) sendToPlayer(playerID int32, data []byte) {
	s.playersMu.RLock()
	player, exists := s.players[playerID]
	s.playersMu.RUnlock()
	if !exists {
		return
	}

	// Non‑blocking send; drop the message if the client's buffer is full.
	// Avoids stalling the entire server when a slow client back‑pressures.
	select {
	case player.Send <- data:
	default:
		// TODO: increment metric / log dropped message
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
		Send: make(chan []byte, SendBufSize),
		TokenBucket: TokenBucket{
			tokens: 200,
		},
		RevealWindowStart: time.Now(),
		RevealCount:       0,
	}
	s.players[playerID] = player
	s.playersMu.Unlock()

	// Send initial leaderboard
	s.stateMu.Lock()
	if s.lbJSON == nil {
		s.buildLeaderboardUnsafe()
		s.lbDirty = false
	}
	lbBytes := s.lbJSON
	lbVer := s.lbVersion
	s.stateMu.Unlock()

	s.sendToPlayer(playerID, lbBytes)
	player.LastLBVersion = lbVer

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

			if player.RevealCount >= MaxRevealsPerMin {
				player.SusRevealOverflow++
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

		case "unsubscribe":
			var msg UnsubscribeMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}
			s.unsubscribeFromChunk(player.ID, ChunkID{X: msg.ChunkX, Y: msg.ChunkY})
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
		s.subs[chunkID] = make(map[int32]struct{})
	}
	s.subs[chunkID][playerID] = struct{}{}

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
		if _, exists := subs[playerID]; exists {
			delete(subs, playerID)
			if len(subs) == 0 {
				delete(s.subs, chunkID)
			}
		}
	}
	s.stateMu.Unlock()

	log.Printf("Player %d disconnected", playerID)
}

func main() {
	runtime.GOMAXPROCS(1)

	server := NewServer()

	http.HandleFunc("/ws", server.handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./")))

	// Leaderboard broadcast loop (1 s cadence, only on version mismatch)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Re‑compute if dirty
			server.stateMu.Lock()
			if server.lbDirty || server.lbJSON == nil {
				server.buildLeaderboardUnsafe()
				server.lbDirty = false
			}
			lbJSON := server.lbJSON
			lbVer := server.lbVersion
			server.stateMu.Unlock()

			// Fan‑out only to players with stale version
			server.playersMu.RLock()
			for _, p := range server.players {
				if p.LastLBVersion == lbVer {
					continue
				}
				select {
				case p.Send <- lbJSON:
					p.LastLBVersion = lbVer
				default:
					// full buffer; skip, they'll get it next tick
				}
			}
			server.playersMu.RUnlock()
		}
	}()

	fmt.Println("Server running at: http://localhost:8001/")
	log.Fatal(http.ListenAndServe("0.0.0.0:8001", nil))
}
