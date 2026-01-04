package main

import (
	"fmt"
	"math/rand"
	"time"
)

func (s *Server) devRevealOriginArea(radius int) {
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
func (s *Server) devAddTestUsers(n int) {
	r := rand.New(rand.NewSource(64))

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	// Add n test users with random scores between 50 and 200
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("Name%d", i+1)

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
		s.botIDs[pid] = true
		delete(s.playerViews, pid)
		return pid
	}

	pid := s.nextPlayerID
	s.nextPlayerID++
	s.playerNames[pid] = name
	s.playerFlags[pid] = flagID
	s.scores[pid] = 0
	s.nameToPlayerID[name] = pid
	s.botIDs[pid] = true
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
	// Human-like behavior knobs
	GuessProbability float64 // chance [0..1] to make a reasoned guess when no certainties are found
	FocusRadius      int     // search radius (in cells) around last action before global sweep (default 8)
	JumpProbability  float64 // chance [0..1] to abandon current focus and roam elsewhere
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
	if cfg.GuessProbability < 0 {
		cfg.GuessProbability = 0
	}
	if cfg.GuessProbability > 1 {
		cfg.GuessProbability = 1
	}
	if cfg.FocusRadius <= 0 {
		cfg.FocusRadius = 8
	}
	if cfg.JumpProbability < 0 {
		cfg.JumpProbability = 0
	}
	if cfg.JumpProbability > 1 {
		cfg.JumpProbability = 1
	}

	botID := s.devEnsureBotUser(cfg.Name, cfg.FlagID)
	seed := cfg.RandomSeed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	s.stateMu.Lock()
	s.playerViews[botID] = PlayerView{Chunk: ChunkID{X: 0, Y: 0}, Cell: uint32(32 + 32*ChunkSize)}
	s.stateMu.Unlock()

	go func() {
		var requestID uint64 = 1
		// Focus on the last action's location (world coordinates via cid+cell)
		var focusCID ChunkID
		var focusCell uint32
		hasFocus := false

		rate := float64(cfg.ActionsPerSecond)
		if rate <= 0 {
			rate = 10
		}

		for {
			// Exponential inter-arrival → natural bursts
			dt := rng.ExpFloat64() / rate
			if dt < 0.02 { // clamp to avoid hot loop
				dt = 0.02
			}
			time.Sleep(time.Duration(dt * float64(time.Second)))

			// Occasionally abandon current area to roam
			if hasFocus && cfg.JumpProbability > 0 && rng.Float64() < cfg.JumpProbability {
				hasFocus = false
			}

			// Human-ish counting action near focus first
			cid, cell, rightClick, isChord, ok := s.devPickCountingActionHumanish(rng, hasFocus, focusCID, focusCell, cfg.FocusRadius, cfg.GuessProbability)
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

			// Update focus to where we just acted
			focusCID, focusCell = cid, cell
			hasFocus = true

			// Update playerViews so bots appear on minimap
			s.stateMu.Lock()
			s.playerViews[botID] = PlayerView{Chunk: cid, Cell: cell}
			s.stateMu.Unlock()
		}
	}()
}

// devStartCountingBots starts many bots at once using given configs.
func (s *Server) devStartCountingBots(cfgs []BotConfig) {
	for _, cfg := range cfgs {
		s.devStartCountingBot(cfg)
	}
}

// devPickCountingActionHumanish finds one action with a human-like pattern:
// 1) Prefer certain moves (flag/chord) near the recent focus within radius.
// 2) Randomize scan order to avoid straight line sweeps.
// 3) If no certainties exist, occasionally make a reasoned guess adjacent to a number.
func (s *Server) devPickCountingActionHumanish(rng *rand.Rand, hasFocus bool, focusCID ChunkID, focusCell uint32, focusRadius int, guessProb float64) (ChunkID, uint32, bool, bool, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	// Helper to evaluate a revealed numbered cell and maybe produce an action.
	evalCenter := func(cid ChunkID, row, col int) (ChunkID, uint32, bool, bool, bool) {
		centerCell := uint32(row*ChunkSize + col)
		n := s.countAdjacentMines(cid, centerCell)
		f := s.countAdjacentFlags(cid, centerCell)
		r := s.countAdjacentRevealedMines(cid, centerCell)

		worldX := int(cid.X)*ChunkSize + col
		worldY := int(cid.Y)*ChunkSize + row

		// Gather unknown neighbors in random order
		neighbors := [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}
		// Shuffle neighbor order
		for i := range neighbors {
			j := rng.Intn(i + 1)
			neighbors[i], neighbors[j] = neighbors[j], neighbors[i]
		}
		unknown := make([]struct {
			ncid  ChunkID
			ncell uint32
		}, 0, 8)
		for _, d := range neighbors {
			dx, dy := d[0], d[1]
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
		if len(unknown) == 0 {
			return ChunkID{}, 0, false, false, false
		}

		minesLeft := n - f - r
		switch {
		case minesLeft == len(unknown):
			// all unknown are mines → flag a random one
			choice := unknown[rng.Intn(len(unknown))]
			return choice.ncid, choice.ncell, true, false, true
		case minesLeft == 0:
			// all unknown are safe → chord the center
			return cid, centerCell, false, true, true
		default:
			// Optional guess: pick one unknown with small probability
			if guessProb > 0 && rng.Float64() < guessProb {
				choice := unknown[rng.Intn(len(unknown))]
				return choice.ncid, choice.ncell, false, false, true
			}
			return ChunkID{}, 0, false, false, false
		}
	}

	// Strategy A: scan within a small radius of focus first
	if hasFocus {
		fx := int(focusCID.X)*ChunkSize + int(focusCell%ChunkSize)
		fy := int(focusCID.Y)*ChunkSize + int(focusCell/ChunkSize)

		// Iterate chunks near focus (map order is randomized)
		for cid, bits := range s.chunks {
			if bits == nil {
				continue
			}
			// Quick reject using chunk bounding box
			minX := int(cid.X) * ChunkSize
			minY := int(cid.Y) * ChunkSize
			maxX := minX + ChunkSize - 1
			maxY := minY + ChunkSize - 1
			if fx < minX-focusRadius || fx > maxX+focusRadius || fy < minY-focusRadius || fy > maxY+focusRadius {
				continue
			}

			// Randomize row/col start to avoid lines
			rowStart := rng.Intn(ChunkSize)
			for ro := 0; ro < ChunkSize; ro++ {
				row := (rowStart + ro) % ChunkSize
				rowBits := (*bits)[row]
				if rowBits == 0 {
					continue
				}
				colStart := rng.Intn(ChunkSize)
				for co := 0; co < ChunkSize; co++ {
					col := (colStart + co) % ChunkSize
					if (rowBits & (1 << uint(col))) == 0 {
						continue
					}
					if actCID, actCell, rc, chord, ok := evalCenter(cid, row, col); ok {
						return actCID, actCell, rc, chord, true
					}
				}
			}
		}
	}

	// Strategy B: global sweep with randomized row/col order
	for cid, bits := range s.chunks {
		if bits == nil {
			continue
		}
		rowStart := rng.Intn(ChunkSize)
		for ro := 0; ro < ChunkSize; ro++ {
			row := (rowStart + ro) % ChunkSize
			rowBits := (*bits)[row]
			if rowBits == 0 {
				continue
			}
			colStart := rng.Intn(ChunkSize)
			for co := 0; co < ChunkSize; co++ {
				col := (colStart + co) % ChunkSize
				if (rowBits & (1 << uint(col))) == 0 {
					continue
				}
				if actCID, actCell, rc, chord, ok := evalCenter(cid, row, col); ok {
					return actCID, actCell, rc, chord, true
				}
			}
		}
	}

	return ChunkID{}, 0, false, false, false
}
