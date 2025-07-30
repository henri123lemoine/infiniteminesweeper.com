package main

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"strings"

	pb "infinite-minesweeper/proto"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

//go:embed index.html proto/messages_pb.js bundle.js
var content embed.FS

const (
	ChunkSize        = 64
	SendBufSize      = 4096  // outbound msgs kept per player before back‑pressure
	MineCount        = 20    // mines per 100 cells (20% chance)
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
	Mailbox     chan func(*Player) // actor channel
	TokenBucket TokenBucket
	Name        string

	// Rate limiting
	Color             string
	RevealWindowStart time.Time
	RevealCount       int

	// Leaderboard version already sent (protected by mailbox now)
	LastLBVersion uint64

	// Suspicion metrics (for admin dashboards, nothing enforced server‑side)
	SusRevealOverflow int // # of extra reveals processed beyond MaxRevealsPerMin

	// Scoring
	FlagsInARow    int
	LastActionTime time.Time
	Score          int32

	// outbound-drop counter (closes WS after 32 consecutive drops)
	dropMisses int

	done chan struct{} // closed when player is fully removed
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

// mustProto marshals and gzips a protobuf message
func mustProto(m *pb.Msg) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	if _, err := gz.Write(b); err != nil {
		panic(err)
	}
	gz.Close()
	return buf.Bytes()
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
	lbProto      []byte
	lbDirty      bool
	playerColors map[int32]string               // playerID -> color
	playerViews  map[int32]struct{ X, Y int32 } // last known view position

	// Players
	playersMu    sync.RWMutex
	players      map[int32]map[*Player]struct{}
	playerNames  map[int32]string
	nextPlayerID int32

	// Seed cache for performance
	seedCache   map[ChunkID]uint64
	seedCacheMu sync.RWMutex

	// Persistence configuration
	useS3   bool
	dataDir string

	// AWS S3 client and WAL
	s3Client   *s3.S3
	bucketName string
	walBuffer  []WALEntry
	walMutex   sync.Mutex
	walSeq     uint64

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
		playerViews:  make(map[int32]struct{ X, Y int32 }),
		seedCache:    make(map[ChunkID]uint64),
		nextPlayerID: 1,
		dataDir:      "data",
		upgrader: websocket.Upgrader{
			// Reject cross-site WebSocket requests (prevents CSRF via <iframe>).
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				clean := func(h string) string {
					h = strings.TrimSuffix(h, ":80")
					h = strings.TrimSuffix(h, ":443")
					return h
				}
				return clean(u.Host) == clean(r.Host)
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

	s.lbVersion++
	lbMsg := &pb.Msg{Payload: &pb.Msg_Leaderboard{Leaderboard: &pb.Leaderboard{
		Version: s.lbVersion,
		Entries: make([]*pb.LeaderboardEntry, len(entries)),
	}}}
	for i, e := range entries {
		lbMsg.GetLeaderboard().Entries[i] = &pb.LeaderboardEntry{PlayerId: e.PlayerID, Name: e.Name, Score: e.Score}
	}
	s.lbProto = mustProto(lbMsg)
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
		delta = int32(len(reveals))
		s.lbDirty = true
	}

	for _, rv := range reveals {
		s.writeWALEntry("reveal", rv)
	}

	s.broadcastRevealBatch(reveals)

	return true
}

func (s *Server) sendScoreUpdate(playerID int32, score int32, worldX, worldY int, delta int32) {
	msg := &pb.Msg{Payload: &pb.Msg_ScoreUpdate{ScoreUpdate: &pb.ScoreUpdate{
		Score:  score,
		WorldX: int32(worldX),
		WorldY: int32(worldY),
		Delta:  delta,
	}}}
	s.sendToPlayer(playerID, mustProto(msg))
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
	hello := msg.GetHello()
	if hello == nil {
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
		Mailbox:           make(chan func(*Player), 64),
		TokenBucket:       TokenBucket{tokens: 200},
		RevealWindowStart: time.Now(),
		RevealCount:       0,
		Name:              hello.Name,
		Color:             hello.Color,
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

	welcomeMsg := &pb.Msg{Payload: &pb.Msg_Welcome{Welcome: &pb.Welcome{PlayerId: playerID, Name: hello.Name, Color: hello.Color, ViewX: view.X, ViewY: view.Y}}}
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
			X:       int32(fl.X), Y: int32(fl.Y), PlayerId: fl.PlayerID, Color: fl.Color,
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
			if server.lbDirty || server.lbProto == nil {
				server.buildLeaderboardUnsafe()
				server.lbDirty = false
			}
			lbJSON := server.lbProto
			lbVer := server.lbVersion
			server.stateMu.Unlock()

			// Fan‑out only to players with stale version
			server.playersMu.RLock()
			for _, set := range server.players {
				for p := range set {
					select {
					case <-p.done:
						continue // player gone
					default:
					}

					p.Mailbox <- func(pl *Player) {
						if pl.LastLBVersion == lbVer {
							return
						}
						// send outside mailbox so we don't block actor
						go func() { p.Send <- lbJSON }()
						pl.LastLBVersion = lbVer
					}
				}
			}
			server.playersMu.RUnlock()
		}
	}()

	fmt.Printf("Server running at: http://%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
