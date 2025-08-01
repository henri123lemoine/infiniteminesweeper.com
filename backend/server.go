package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gorilla/websocket"
)

type Server struct {
	secret []byte

	// Single mutex for all world state
	stateMu sync.RWMutex

	// World state - just what cells are revealed and by whom
	chunks     map[ChunkID]*ChunkBits         // Which cells are revealed (bitset)
	cellOwners map[ChunkID]map[int]int32      // bitIndex -> playerID
	flags      map[ChunkID]map[int]Flag       // bitIndex -> Flag (with id)
	scores     map[int32]int32                // playerID -> score
	subs       map[ChunkID]map[int32]struct{} // who wants reveals for each chunk

	// leaderboard cache
	lbVersion   uint64
	lbProto     []byte
	lbDirty     bool
	playerFlags map[int32]uint32               // playerID -> flagID
	playerViews map[int32]struct{ X, Y int32 } // last known view position

	// Players
	playersMu   sync.RWMutex
	players     map[int32]map[*Player]struct{}
	playerNames map[int32]string

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
		secret:      []byte("minesweeper-secret-key"),
		chunks:      make(map[ChunkID]*ChunkBits),
		cellOwners:  make(map[ChunkID]map[int]int32),
		flags:       make(map[ChunkID]map[int]Flag),
		scores:      make(map[int32]int32),
		subs:        make(map[ChunkID]map[int32]struct{}),
		players:     make(map[int32]map[*Player]struct{}),
		playerNames: make(map[int32]string),
		playerFlags: make(map[int32]uint32),
		playerViews: make(map[int32]struct{ X, Y int32 }),
		seedCache:   make(map[ChunkID]uint64),
		dataDir:     "data",
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

func (s *Server) generatePlayerID() int32 {
	for {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic("rand failure: " + err.Error())
		}
		id := int32(binary.LittleEndian.Uint32(b[:]) & 0x7fffffff)
		if id == 0 {
			continue
		}
		s.stateMu.RLock()
		_, existsName := s.playerNames[id]
		s.stateMu.RUnlock()
		s.playersMu.RLock()
		_, existsPlayer := s.players[id]
		s.playersMu.RUnlock()
		if !existsName && !existsPlayer {
			return id
		}
	}
}
