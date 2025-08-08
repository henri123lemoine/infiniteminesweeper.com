package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
)

//go:embed dist/*
var content embed.FS

const (
	ChunkSize        = 64
	SendBufSize      = 4096  // outbound msgs kept per player before back‑pressure
	MineCount        = 20    // mines per 100 cells (20% chance)
	MaxRevealsPerMin = 10000 // relaxed flood‑fill budget
)

// Reveal a starter patch around (0,0) in dev mode for faster testing
func (s *Server) devRevealOriginArea() {
	const radius = 40 // cells
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			cid, cell := worldToChunk(x, y)
			if s.chunks[cid] == nil {
				s.chunks[cid] = &ChunkBits{}
			}
			bit := cell
			(*s.chunks[cid])[bit/64] |= 1 << (bit % 64)
		}
	}
}

func (s *Server) devAddTestUsers() {
	r := rand.New(rand.NewSource(64))

	testNames := []string{
		"Name1", "Name2", "Name3", "Name4", "Name5",
		"Name6", "Name7", "Name8", "Name9", "Name10",
		"Name11", "Name12", "Name13", "Name14", "Name15",
		"Name16", "Name17", "Name18", "Name19", "Name20",
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	// Add 20 test users with random scores between 50 and 200
	for _, name := range testNames {
		playerID := s.nextPlayerID
		s.nextPlayerID++

		// Random score between 50 and 200
		score := r.Int31n(151) + 50 // 50 to 200 inclusive

		// Random flag ID between 0 and 16 (assuming 17 flags total)
		flagID := uint32(r.Intn(80))

		// Add to server state
		s.scores[playerID] = score
		s.playerNames[playerID] = name
		s.playerFlags[playerID] = flagID
		s.nameToPlayerID[name] = playerID

		fmt.Printf("Added test user: %s (ID: %d, Score: %d, Flag: %d)\n", name, playerID, score, flagID)
	}

	// Mark leaderboard as dirty to trigger rebuild
	s.lbDirty = true
}

func main() {
	mustLoadEnv()

	runtime.GOMAXPROCS(1)

	// honour $PORT for Heroku/Fly style deploys
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := "0.0.0.0:" + port

	server := NewServer()
	server.initPersistence()

	if os.Getenv("MODE") == "development" {
		server.devRevealOriginArea()
		server.devAddTestUsers()
	}

	http.HandleFunc("/ws", server.handleWebSocket)
	http.HandleFunc("/hotspot", server.handleHotspot)
	http.HandleFunc("/leaderboard", server.handleLeaderboardHTTP)
	http.HandleFunc("/profile/update", server.handleProfileUpdate)
	distFS, _ := fs.Sub(content, "dist")
	http.Handle("/", http.FileServer(http.FS(distFS)))

	go server.runLeaderboardBroadcaster()

	fmt.Printf("Server running at: http://%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))

}
