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
	ChunkSize   = 64
	SendBufSize = 4096 // outbound msgs kept per player before back‑pressure
)

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
		server.devRevealOriginArea(30)
		server.devAddTestUsers(100)

		cfgs := []BotConfig{
			{
				Name:             "CalmBot",
				FlagID:           56,
				ActionsPerSecond: 5,
			},
			{
				Name:                "SpeedBot",
				FlagID:              38,
				ActionsPerSecond:    12,
				FailFlagProbability: 0.08,
			},
			{
				Name:                "BBot",
				FlagID:              69,
				ActionsPerSecond:    40,
				FailFlagProbability: 0.14,
			},
			{
				Name:             "OPBot",
				FlagID:           80,
				ActionsPerSecond: 40,
			},
		}
		server.devStartCountingBots(cfgs)
	}

	http.HandleFunc("/ws", server.handleWebSocket)
	http.HandleFunc("/hotspot", server.handleHotspot)
	http.HandleFunc("/leaderboard", server.handleLeaderboardHTTP)
	http.HandleFunc("/profile/update", server.handleProfileUpdate)
	distFS, _ := fs.Sub(content, "dist")
	http.Handle("/", http.FileServer(http.FS(distFS)))

	go server.runLeaderboardBroadcaster()
	go server.runMinimapBroadcaster()
	go server.runPlayerPositionBroadcaster()

	fmt.Printf("Server running at: http://%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
