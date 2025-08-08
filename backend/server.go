package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Server struct {
	secret []byte

	// Single mutex for all world state
	stateMu sync.RWMutex

	// World state - just what cells are revealed and by whom
	chunks        map[ChunkID]*ChunkBits          // Which cells are revealed (bitset)
	flags         map[ChunkID]map[uint32]Flag     // cellIndex -> Flag (with id)
	scores        map[uint32]int32                // playerID -> score
	subs          map[ChunkID]map[uint32]struct{} // who wants reveals for each chunk
	totalRevealed uint64

	// leaderboard cache
	lbVersion   uint64
	lbProto     []byte
	lbDirty     bool
	playerFlags map[uint32]uint32     // playerID -> flagID
	playerViews map[uint32]PlayerView // last known view position (chunk, cell)

	// Players
	playersMu   sync.RWMutex
	players     map[uint32]map[*Player]struct{}
	playerNames map[uint32]string
	// Fast lookup to reuse identity by name if token is missing/invalid
	nameToPlayerID map[string]uint32
	nextPlayerID   uint32
	sessionTokens  map[string]uint32 // session_token -> playerID

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

	// Gameplay rules
	// Chebyshev distance for proximity-limited actions (reveal/flag).
	// Negative value disables the restriction (used in tests).
	proximityRadius int

	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		secret:          []byte("minesweeper-secret-key"),
		chunks:          make(map[ChunkID]*ChunkBits),
		flags:           make(map[ChunkID]map[uint32]Flag),
		scores:          make(map[uint32]int32),
		subs:            make(map[ChunkID]map[uint32]struct{}),
		players:         make(map[uint32]map[*Player]struct{}),
		playerNames:     make(map[uint32]string),
		nameToPlayerID:  make(map[string]uint32),
		playerFlags:     make(map[uint32]uint32),
		playerViews:     make(map[uint32]PlayerView),
		sessionTokens:   make(map[string]uint32), // Initialize the new map
		seedCache:       make(map[ChunkID]uint64),
		nextPlayerID:    1,
		dataDir:         "data",
		proximityRadius: 2, // default behavior: must be within distance <= 2 of any revealed cell
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

// generateSessionToken creates a new, cryptographically secure session token.
func generateSessionToken() string {
	return uuid.New().String()
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

// handleHotspot returns the chunk with the highest number of active subscribers (players nearby).
// Falls back to {X:0, Y:0} when there is no activity yet.
func (s *Server) handleHotspot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.stateMu.RLock()
	var (
		bestChunk ChunkID
		bestCount int
	)
	for cid, set := range s.subs {
		c := len(set)
		if c > bestCount {
			bestCount = c
			bestChunk = cid
		}
	}
	s.stateMu.RUnlock()

	// Conservative default when no one is around
	resp := struct {
		X     int64 `json:"X"`
		Y     int64 `json:"Y"`
		Count int   `json:"count"`
	}{X: bestChunk.X, Y: bestChunk.Y, Count: bestCount}

	// Encode JSON (ignore write errors deliberately)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleLeaderboardHTTP serves a deduplicated, full leaderboard (best score per name), sorted desc.
func (s *Server) handleLeaderboardHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type entry struct {
		Name   string `json:"name"`
		Score  int32  `json:"score"`
		FlagID uint32 `json:"flagID"`
	}

	s.stateMu.RLock()
	bestByName := make(map[string]entry)
	for pid, sc := range s.scores {
		name := s.playerNames[pid]
		e := entry{Name: name, Score: sc, FlagID: s.playerFlags[pid]}
		if prev, ok := bestByName[name]; !ok || e.Score > prev.Score {
			bestByName[name] = e
		}
	}
	// Move to slice and sort
	out := make([]entry, 0, len(bestByName))
	for _, e := range bestByName {
		out = append(out, e)
	}
	s.stateMu.RUnlock()

	// Sort descending by score
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	_ = json.NewEncoder(w).Encode(out)
}

type profileUpdateReq struct {
	Name   *string `json:"name"`
	FlagID *uint32 `json:"flagID"`
}

type profileUpdateResp struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Name     string `json:"name"`
	FlagID   uint32 `json:"flagID"`
	Score    int32  `json:"score"`
	PlayerID uint32 `json:"playerID"`
}

// handleProfileUpdate updates the current player's display name and/or flag without
// changing their identity or resetting the score. Identifies the player via session token.
func (s *Server) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(profileUpdateResp{OK: false, Error: "method not allowed"})
		return
	}

	// Extract token from headers (X-Session-Token or Authorization: Bearer ...)
	token := r.Header.Get("X-Session-Token")
	if token == "" {
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, prefix) {
			token = strings.TrimPrefix(auth, prefix)
		}
	}
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(profileUpdateResp{OK: false, Error: "missing session token"})
		return
	}

	var req profileUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(profileUpdateResp{OK: false, Error: "invalid json"})
		return
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	pid, ok := s.sessionTokens[token]
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(profileUpdateResp{OK: false, Error: "invalid session token"})
		return
	}

	// Update name if provided
	if req.Name != nil {
		newName := strings.TrimSpace(*req.Name)
		if !isValidUsername(newName) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(profileUpdateResp{OK: false, Error: "invalid username"})
			return
		}
		current := s.playerNames[pid]
		if newName != current {
			if other, exists := s.nameToPlayerID[newName]; exists && other != pid {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(profileUpdateResp{OK: false, Error: "username taken"})
				return
			}
			if current != "" {
				delete(s.nameToPlayerID, current)
			}
			s.nameToPlayerID[newName] = pid
			s.playerNames[pid] = newName
			s.lbDirty = true
		}
	}

	// Update flag if provided
	if req.FlagID != nil {
		s.playerFlags[pid] = *req.FlagID
		s.lbDirty = true
	}

	resp := profileUpdateResp{
		OK:       true,
		Name:     s.playerNames[pid],
		FlagID:   s.playerFlags[pid],
		Score:    s.scores[pid],
		PlayerID: pid,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
