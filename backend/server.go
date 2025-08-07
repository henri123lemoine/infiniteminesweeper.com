package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"net/url"
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
	chunks map[ChunkID]*ChunkBits          // Which cells are revealed (bitset)
	flags  map[ChunkID]map[uint32]Flag     // cellIndex -> Flag (with id)
	scores map[uint32]int32                // playerID -> score
	subs   map[ChunkID]map[uint32]struct{} // who wants reveals for each chunk

	// leaderboard cache
	lbVersion   uint64
	lbProto     []byte
	lbDirty     bool
	playerFlags map[uint32]uint32     // playerID -> flagID
	playerViews map[uint32]PlayerView // last known view position (chunk, cell)

	// Players
	playersMu     sync.RWMutex
	players       map[uint32]map[*Player]struct{}
	playerNames   map[uint32]string
	nextPlayerID  uint32
	sessionTokens map[string]uint32 // session_token -> playerID

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
		secret:        []byte("minesweeper-secret-key"),
		chunks:        make(map[ChunkID]*ChunkBits),
		flags:         make(map[ChunkID]map[uint32]Flag),
		scores:        make(map[uint32]int32),
		subs:          make(map[ChunkID]map[uint32]struct{}),
		players:       make(map[uint32]map[*Player]struct{}),
		playerNames:   make(map[uint32]string),
		playerFlags:   make(map[uint32]uint32),
		playerViews:   make(map[uint32]PlayerView),
		sessionTokens: make(map[string]uint32), // Initialize the new map
		seedCache:     make(map[ChunkID]uint64),
		nextPlayerID:  1,
		dataDir:       "data",
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
