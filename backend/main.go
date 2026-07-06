package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

//go:embed dist/*
var content embed.FS

const (
	ChunkSize   = 64
	SendBufSize = 4096 // outbound msgs kept per player before back‑pressure
)

// findAvailablePort scans ports 8080-8099 and returns the first available one.
func findAvailablePort() string {
	for p := 8080; p <= 8099; p++ {
		addr := fmt.Sprintf(":%d", p)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return fmt.Sprintf("%d", p)
		}
	}
	log.Fatal("No available port in range 8080-8099")
	return ""
}

func main() {
	mustLoadEnv()

	runtime.GOMAXPROCS(1)

	var port string
	if os.Getenv("MODE") == "development" {
		port = findAvailablePort()
	} else {
		// honour $PORT for Heroku/Fly style deploys
		port = os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
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
				ActionsPerSecond: 2,
				FocusRadius:      4,
			},
			{
				Name:                "SpeedBot",
				FlagID:              38,
				ActionsPerSecond:    3,
				FailFlagProbability: 0.08,
				FocusRadius:         6,
			},
			{
				Name:                "BBot",
				FlagID:              69,
				ActionsPerSecond:    2,
				FailFlagProbability: 0.14,
				FocusRadius:         5,
			},
			{
				Name:             "OPBot",
				FlagID:           80,
				ActionsPerSecond: 3,
				FocusRadius:      6,
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

	// Optional Prometheus metrics endpoint on a separate port — kept off the
	// public mux so it's reachable from fly's internal scraper but not the
	// public internet. See fly.toml's [[metrics]] block.
	if metricsPort := os.Getenv("METRICS_PORT"); metricsPort != "" {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/metrics", server.handleMetrics)
			metricsAddr := "0.0.0.0:" + metricsPort
			log.Printf("Metrics server on http://%s/metrics", metricsAddr)
			if err := http.ListenAndServe(metricsAddr, mux); err != nil {
				log.Printf("metrics server exited: %v", err)
			}
		}()
	}

	// Fly sends SIGINT on auto-stop and deploys. Without this handler, every
	// stop silently dropped the in-memory WAL buffer — up to a full flush
	// interval of recent play.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[shutdown] received %v, flushing WAL + snapshot", sig)
		server.persistOnShutdown()
		os.Exit(0)
	}()

	fmt.Printf("Server running at: http://%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
