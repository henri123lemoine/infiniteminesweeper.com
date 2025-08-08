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
	"time"
)

//go:embed dist/*
var content embed.FS

const (
	ChunkSize        = 64
	SendBufSize      = 4096  // outbound msgs kept per player before back‑pressure
	MineCount        = 20    // mines per 100 cells (20% chance)
	MaxRevealsPerMin = 10000 // relaxed flood‑fill budget
)

func (s *Server) devRevealOriginArea() {
	const radius = 10 // cells
	r2 := radius * radius
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y > r2 {
				continue
			}
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

// devEnsureBotUser creates (or returns) a single bot identity for dev mode.
func (s *Server) devEnsureBotUser(name string, flagID uint32) uint32 {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if pid, ok := s.nameToPlayerID[name]; ok {
		return pid
	}

	pid := s.nextPlayerID
	s.nextPlayerID++
	s.playerNames[pid] = name
	s.playerFlags[pid] = flagID
	s.scores[pid] = 0
	s.nameToPlayerID[name] = pid
	s.lbDirty = true
	return pid
}

// BotConfig controls a counting-bot instance.
type BotConfig struct {
	Name                string
	FlagID              uint32
	ActionsPerSecond    int
	FailFlagProbability float64 // chance [0..1] to mis-handle a certain-flag by revealing instead
	RandomSeed          int64   // seed for RNG (0 → time-based)
}

// devStartCountingBot starts one counting-based bot with the provided config.
func (s *Server) devStartCountingBot(cfg BotConfig) {
	if cfg.ActionsPerSecond <= 0 {
		cfg.ActionsPerSecond = 10
	}
	if cfg.Name == "" {
		cfg.Name = "DevBot"
	}
	if cfg.FailFlagProbability < 0 {
		cfg.FailFlagProbability = 0
	}
	if cfg.FailFlagProbability > 1 {
		cfg.FailFlagProbability = 1
	}

	botID := s.devEnsureBotUser(cfg.Name, cfg.FlagID)
	seed := cfg.RandomSeed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(cfg.ActionsPerSecond))
		defer ticker.Stop()
		var requestID uint64 = 1

		for range ticker.C {
			// Counting-based deterministic action (no mine peeking).
			cid, cell, rightClick, isChord, ok := s.devPickCountingAction()
			if !ok {
				continue
			}

			// Inject failure only for certain-flag actions by clicking instead of flagging
			if rightClick && cfg.FailFlagProbability > 0 && rng.Float64() < cfg.FailFlagProbability {
				rightClick = false
				isChord = false
			}

			s.handleReveal(botID, requestID, cid, cell, rightClick, isChord)
			requestID++
		}
	}()
}

// devStartCountingBots starts many bots at once using given configs.
func (s *Server) devStartCountingBots(cfgs []BotConfig) {
	for _, cfg := range cfgs {
		s.devStartCountingBot(cfg)
	}
}

// devPickCountingAction scans revealed numbered cells and applies basic counting:
// - If N - F - R == U: all unknown neighbors are mines → flag one of them
// - If N - F - R == 0 and U > 0: neighbors are safe → chord the center cell
// Returns a single action per call.
func (s *Server) devPickCountingAction() (ChunkID, uint32, bool, bool, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	for cid, bits := range s.chunks {
		if bits == nil {
			continue
		}
		for row := 0; row < ChunkSize; row++ {
			rowBits := (*bits)[row]
			if rowBits == 0 {
				continue
			}
			for col := 0; col < ChunkSize; col++ {
				if (rowBits & (1 << uint(col))) == 0 {
					continue
				}

				centerCell := uint32(row*ChunkSize + col)
				// Count numbers around the center (this equals the displayed number for revealed cells).
				n := s.countAdjacentMines(cid, centerCell)
				f := s.countAdjacentFlags(cid, centerCell)
				r := s.countAdjacentRevealedMines(cid, centerCell)

				// Collect unknown neighbors
				worldX := int(cid.X)*ChunkSize + col
				worldY := int(cid.Y)*ChunkSize + row
				var unknown []struct {
					ncid  ChunkID
					ncell uint32
				}
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						wx, wy := worldX+dx, worldY+dy
						ncid, ncell := worldToChunk(wx, wy)
						if s.isCellRevealed(ncid, ncell) || s.isCellFlagged(ncid, ncell) {
							continue
						}
						unknown = append(unknown, struct {
							ncid  ChunkID
							ncell uint32
						}{ncid, ncell})
					}
				}

				if len(unknown) == 0 {
					continue
				}

				minesLeft := n - f - r
				if minesLeft == len(unknown) {
					// Certainty: all unknown are mines → flag one now
					choice := unknown[0]
					return choice.ncid, choice.ncell, true, false, true
				}
				if minesLeft == 0 {
					// Certainty: all unknown are safe → chord the center to reveal them
					return cid, centerCell, false, true, true
				}
			}
		}
	}
	return ChunkID{}, 0, false, false, false
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

		cfgs := []BotConfig{
			{
				Name:             "CalmBot",
				FlagID:           56,
				ActionsPerSecond: 5,
			},
			{
				Name:                "SpeedBot",
				FlagID:              80,
				ActionsPerSecond:    15,
				FailFlagProbability: 0.1,
			},
			{
				Name:                "BBot",
				FlagID:              69,
				ActionsPerSecond:    40,
				FailFlagProbability: 0.4,
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

	fmt.Printf("Server running at: http://%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))

}
