package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
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
	if os.Getenv("MODE") != "development" {
		return
	}
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
