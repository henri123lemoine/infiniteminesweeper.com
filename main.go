package main

import (
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed index.html
var content embed.FS

const (
	ChunkSize        = 64
	SendBufSize      = 4096  // outbound msgs kept per player before back‑pressure
	MineCount        = 20    // mines per 100 cells (20% chance)
	MaxRevealsPerMin = 10000 // relaxed flood‑fill budget
)

// SNAPSHOT / PERSISTENCE CONSTANTS
const (
	snapshotFile     = "data/snapshot.gob.gz"
	snapshotTmp      = "snapshot.tmp.gz"
	snapshotInterval = 10 * time.Minute
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

type Flag struct {
	ChunkID  ChunkID `json:"chunkId"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	PlayerID int32   `json:"playerId"`
	Color    string  `json:"color"`
}

type Player struct {
	ID          int32
	Conn        *websocket.Conn
	Send        chan []byte
	TokenBucket TokenBucket
	Name        string
	// protects everything below this line against concurrent readPump /
	// flag() / reveal() races (one player can have several connections)
	mu sync.Mutex

	// Rate limiting for reveals
	Color             string
	RevealWindowStart time.Time
	RevealCount       int

	// Leaderboard version the player has already received
	LastLBVersion uint64

	// Suspicion metrics (for admin dashboards, nothing enforced server‑side)
	SusRevealOverflow int // # of extra reveals processed beyond MaxRevealsPerMin

	// New scoring system state
	FlagsInARow    int // consecutive correct flags
	LastActionTime time.Time
	Score          int32 // capped at 0
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

type FlagMessage struct {
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

type ChunkSync struct {
	Type    string   `json:"type"`
	ChunkID ChunkID  `json:"chunkId"`
	Seed    string   `json:"seed"`
	Reveals []Reveal `json:"reveals"`
	Flags   []Flag   `json:"flags"`
}

type RevealAck struct {
	Type string `json:"type"`
	OK   bool   `json:"ok"`
}

type FlagAck struct {
	Type string `json:"type"`
	OK   bool   `json:"ok"`
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
	flags      map[ChunkID]map[int]Flag       // bitIndex -> Flag (with color)
	scores     map[int32]int32                // playerID -> score
	subs       map[ChunkID]map[int32]struct{} // who wants reveals for each chunk

	// leaderboard cache
	lbVersion    uint64
	lbJSON       []byte
	lbDirty      bool
	playerColors map[int32]string // playerID -> color

	// Players
	playersMu    sync.RWMutex
	players      map[int32]map[*Player]struct{}
	playerNames  map[int32]string
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
		flags:        make(map[ChunkID]map[int]Flag),
		scores:       make(map[int32]int32),
		subs:         make(map[ChunkID]map[int32]struct{}),
		players:      make(map[int32]map[*Player]struct{}),
		playerNames:  make(map[int32]string),
		playerColors: make(map[int32]string),
		seedCache:    make(map[ChunkID]uint64),
		nextPlayerID: 1,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) initPersistence() {
	if err := s.loadSnapshot(); err != nil {
		log.Printf("[snapshot] no previous snapshot loaded: %v (starting fresh)", err)
	}
	go s.periodicSnapshotLoop() // fire‑and‑forget
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

// splitmix64 - deterministic PRNG for bomb placement (matches frontend)
func splitmix64(state uint64) uint64 {
	state += 0x9e3779b97f4a7c15
	state = (state ^ (state >> 30)) * 0xbf58476d1ce4e5b9
	state = (state ^ (state >> 27)) * 0x94d049bb133111eb
	return state ^ (state >> 31)
}

// isMine determines if a cell contains a mine using the same logic as frontend
func (s *Server) isMine(seed uint64, x, y int) bool {
	cellSeed := splitmix64(seed + uint64(y*ChunkSize+x))
	return (cellSeed % 100) < MineCount
}

// Leaderboard helpers

type lbEntry struct {
	PlayerID int32  `json:"playerId"`
	Name     string `json:"name"`
	Score    string `json:"score"`
}

func formatScore(n int32) string {
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
		entries = append(entries, lbEntry{PlayerID: pid, Name: s.playerNames[pid], Score: formatScore(sc)})
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

	// Get player info
	s.playersMu.RLock()
	playerConns := s.players[playerID]
	playerColor := s.playerColors[playerID]
	s.playersMu.RUnlock()

	var player *Player
	for p := range playerConns {
		player = p
		break
	}
	if player == nil {
		return false
	}

	// Determine if this cell contains a mine
	seed := s.generateChunkSeed(chunkID)
	isMine := s.isMine(seed, x, y)

	player.mu.Lock()
	if isMine {
		// Correct flag: award points with multiplier
		player.FlagsInARow++
		multiplier := player.FlagsInARow
		if multiplier > 20 {
			multiplier = 20
		}
		player.Score += int32(multiplier)

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

		// Broadcast flag to subscribers
		s.broadcastFlagTo3x3(flag)

	} else {
		// Wrong flag: penalize and auto-reveal
		player.Score -= 20
		player.FlagsInARow = 0 // Reset multiplier

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

		// Broadcast the auto-reveal to all players who can see this chunk
		reveal := Reveal{
			ChunkID:  chunkID,
			X:        x,
			Y:        y,
			PlayerID: playerID,
		}
		s.broadcastRevealTo3x3(reveal)
	}

	player.LastActionTime = time.Now()

	// Cap score at 0
	if player.Score < 0 {
		player.Score = 0
	}

	score := player.Score
	player.mu.Unlock()

	// Update leaderboard score
	s.scores[playerID] = score

	// Mark leaderboard as dirty for persistence
	s.lbDirty = true

	// Send score update to player
	s.sendScoreUpdate(playerID, score)

	return true
}

func (s *Server) broadcastFlagTo3x3(flag Flag) {
	// Caller already holds stateMu
	payload := mustJSON(flag)
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
	payload := mustJSON(reveal)
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
	log.Printf("reveal: cell (%d, %d) isMine: %v", x, y, isMine)

	// Update player score based on reveal
	s.playersMu.RLock()
	playerConns, exists := s.players[playerID]
	s.playersMu.RUnlock()

	var player *Player
	if exists {
		for p := range playerConns {
			player = p
			break // Get any connection for this player
		}
	}

	if player != nil {
		player.mu.Lock()
		if isMine {
			player.Score -= 100 // bomb penalty
			player.FlagsInARow = 0
		} else {
			player.Score += 1 // normal reveal
		}
		if player.Score < 0 {
			player.Score = 0
		}
		player.LastActionTime = time.Now()
		newScore := player.Score // copy while still locked
		player.mu.Unlock()

		s.sendScoreUpdate(playerID, newScore)
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

	// Broadcast to 3x3 neighborhood
	s.broadcastRevealTo3x3(reveal)

	// persistence: mark dirty so that next snapshot loop rewrites file
	s.lbDirty = true
	return true
}

func (s *Server) sendScoreUpdate(playerID int32, score int32) {
	scoreMsg := struct {
		Type  string `json:"type"`
		Score int32  `json:"score"`
	}{
		Type:  "scoreUpdate",
		Score: score,
	}

	s.sendToPlayer(playerID, mustJSON(scoreMsg))
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
		case p.Send <- data:
		default:
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	// Expect hello message with optional playerId and name
	var hello struct {
		Type     string `json:"type"`
		PlayerID int32  `json:"playerId"`
		Name     string `json:"name"`
		Color    string `json:"color"`
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.ReadJSON(&hello); err != nil || hello.Type != "hello" {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	playerID := hello.PlayerID
	s.stateMu.Lock()
	if playerID <= 0 || playerID >= s.nextPlayerID {
		playerID = s.nextPlayerID
		s.nextPlayerID++
	}
	// Grab any previously‑saved score before we touch the player map
	initScore := s.scores[playerID]
	s.playerNames[playerID] = hello.Name
	s.playerColors[playerID] = hello.Color
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
		TokenBucket:       TokenBucket{tokens: 200},
		RevealWindowStart: time.Now(),
		RevealCount:       0,
		Name:              hello.Name,
		Color:             hello.Color,
		Score:             initScore, // preserve previous score
	}
	s.players[playerID][player] = struct{}{}
	s.playersMu.Unlock()

	// Send initial leaderboard and welcome after goroutines start
	s.stateMu.Lock()
	if s.lbJSON == nil {
		s.buildLeaderboardUnsafe()
		s.lbDirty = false
	}
	lbBytes := s.lbJSON
	lbVer := s.lbVersion
	s.stateMu.Unlock()

	go s.writePump(player)
	go s.readPump(player)

	welcome := struct {
		Type     string `json:"type"`
		PlayerID int32  `json:"playerId"`
		Name     string `json:"name"`
		Color    string `json:"color"`
	}{"welcome", playerID, hello.Name, hello.Color}
	s.sendToPlayer(playerID, mustJSON(welcome))
	s.sendToPlayer(playerID, lbBytes)
	player.LastLBVersion = lbVer

	// Send initial score (keeps existing progress)
	s.sendScoreUpdate(playerID, initScore)

	log.Printf("Player %d connected", playerID)
}

func (s *Server) readPump(player *Player) {
	defer func() {
		s.removePlayer(player)
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
			player.mu.Lock()
			now := time.Now()
			if now.Sub(player.RevealWindowStart) > time.Minute {
				player.RevealWindowStart = now
				player.RevealCount = 0
			}
			if player.RevealCount >= MaxRevealsPerMin {
				player.SusRevealOverflow++
			}
			player.RevealCount++
			player.mu.Unlock()

			chunkID := ChunkID{X: msg.ChunkX, Y: msg.ChunkY}
			ok := s.reveal(player.ID, chunkID, msg.X, msg.Y)

			ack := RevealAck{
				Type: "revealAck",
				OK:   ok,
			}

			s.sendToPlayer(player.ID, mustJSON(ack))

		case "flag":
			var msg FlagMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}

			chunkID := ChunkID{X: msg.ChunkX, Y: msg.ChunkY}
			ok := s.flag(player.ID, chunkID, msg.X, msg.Y)

			ack := FlagAck{
				Type: "flagAck",
				OK:   ok,
			}
			s.sendToPlayer(player.ID, mustJSON(ack))

		case "subscribe":
			var msg SubscribeMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}
			s.subscribeToChunk(player.ID, ChunkID{X: msg.ChunkX, Y: msg.ChunkY})

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
	flagsMap := s.flags[chunkID]
	seed := s.generateChunkSeed(chunkID)
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
	if flagsMap != nil {
		for _, flag := range flagsMap {
			flags = append(flags, flag)
		}
	}
	s.stateMu.Unlock()

	msg := ChunkSync{
		Type:    "chunkSync",
		ChunkID: chunkID,
		Seed:    strconv.FormatUint(seed, 10),
		Reveals: reveals,
		Flags:   flags,
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

func (s *Server) removePlayer(p *Player) {
	s.playersMu.Lock()
	if set, ok := s.players[p.ID]; ok {
		if _, exists := set[p]; exists {
			close(p.Send)
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

// snapshotData is what actually gets serialized. Gob can handle maps with
// struct keys, so we keep the exact types.
type snapshotData struct {
	Chunks       map[ChunkID]*ChunkBits
	CellOwners   map[ChunkID]map[int]int32
	Flags        map[ChunkID]map[int]Flag
	Scores       map[int32]int32
	PlayerNames  map[int32]string
	PlayerColors map[int32]string
	NextPlayerID int32
}

func (s *Server) saveSnapshot() error {
	s.stateMu.RLock()
	data := snapshotData{
		Chunks:       s.chunks,
		CellOwners:   s.cellOwners,
		Flags:        s.flags,
		Scores:       s.scores,
		PlayerNames:  s.playerNames,
		PlayerColors: s.playerColors,
		NextPlayerID: s.nextPlayerID,
	}
	s.stateMu.RUnlock()

	f, err := os.Create(snapshotTmp)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(&data); err != nil {
		f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(snapshotTmp, snapshotFile)
}

func (s *Server) loadSnapshot() error {
	f, err := os.Open(snapshotFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	dec := gob.NewDecoder(gz)
	var data snapshotData
	if err := dec.Decode(&data); err != nil {
		return err
	}

	s.stateMu.Lock()
	s.chunks = data.Chunks
	s.cellOwners = data.CellOwners
	if data.Flags != nil {
		s.flags = data.Flags
	} else {
		s.flags = make(map[ChunkID]map[int]Flag)
	}
	s.scores = data.Scores
	if data.PlayerNames != nil {
		s.playerNames = data.PlayerNames
	} else {
		s.playerNames = make(map[int32]string)
	}
	if data.PlayerColors != nil {
		s.playerColors = data.PlayerColors
	} else {
		s.playerColors = make(map[int32]string)
	}
	if data.NextPlayerID != 0 {
		s.nextPlayerID = data.NextPlayerID
	}
	s.lbDirty = true // force rebuild of leaderboard on first tick
	s.stateMu.Unlock()
	return nil
}

func (s *Server) periodicSnapshotLoop() {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.saveSnapshot(); err != nil {
			log.Printf("[snapshot] save error: %v", err)
		} else {
			log.Printf("[snapshot] wrote %s", snapshotFile)
		}
	}
}

func main() {
	runtime.GOMAXPROCS(1)

	// honour $PORT for Heroku/Fly style deploys
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := "0.0.0.0:" + port

	server := NewServer()
	server.initPersistence()

	http.HandleFunc("/ws", server.handleWebSocket)
	http.Handle("/", http.FileServer(http.FS(content)))

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
			for _, set := range server.players {
				for p := range set {
					if p.LastLBVersion == lbVer {
						continue
					}
					select {
					case p.Send <- lbJSON:
						p.LastLBVersion = lbVer
					default:
					}
				}
			}
			server.playersMu.RUnlock()
		}
	}()

	fmt.Printf("Server running at: http://%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
