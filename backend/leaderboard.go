package main

import (
	"fmt"
	pb "infiniteminesweeper/backend/gen/proto"
	"sort"
	"strconv"
	"time"
)

// Leaderboard helpers

type lbEntry struct {
	PlayerID uint32 `json:"playerID"`
	Name     string `json:"name"`
	Score    int32  `json:"score"`
	FlagID   uint32 `json:"flagID"`
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
	// Deduplicate by name at the source to avoid duplicates caused by identity glitches
	bestByName := make(map[string]lbEntry)
	for pid, sc := range s.scores {
		name := s.playerNames[pid]
		e := lbEntry{PlayerID: pid, Name: name, Score: sc, FlagID: s.playerFlags[pid]}
		if prev, ok := bestByName[name]; !ok || e.Score > prev.Score {
			bestByName[name] = e
		}
	}
	for _, e := range bestByName {
		entries = append(entries, e)
	}
	// Sort by the raw score
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Score > entries[j].Score
	})
	if len(entries) > 10 {
		entries = entries[:10]
	}

	s.lbVersion++
	lbMsg := &pb.Msg{Payload: &pb.Msg_Leaderboard{Leaderboard: &pb.Leaderboard{
		Version: s.lbVersion,
		Entries: make([]*pb.LeaderboardEntry, len(entries)),
	}}}
	for i, e := range entries {
		lbMsg.GetLeaderboard().Entries[i] = &pb.LeaderboardEntry{
			Name:   e.Name,
			Score:  e.Score,
			FlagID: e.FlagID,
		}
	}
	s.lbProto = mustProto(lbMsg)
}

// Leaderboard broadcast loop (1 s cadence, only on version mismatch)
func (s *Server) runLeaderboardBroadcaster() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Re-compute leaderboard if dirty
		s.stateMu.Lock()
		if s.lbDirty || s.lbProto == nil {
			s.buildLeaderboardUnsafe()
			s.lbDirty = false
		}
		lbJSON := s.lbProto
		lbVer := s.lbVersion
		s.stateMu.Unlock()

		// Send to every connection whose LastLBVersion is stale
		s.playersMu.RLock()
		for _, set := range s.players {
			for p := range set {
				select {
				case <-p.done:
					continue
				default:
				}

				p.Mailbox <- func(pl *Player) {
					if pl.LastLBVersion == lbVer {
						return
					}
					// ship bytes outside the actor to avoid blocking it
					go func() { p.Send <- lbJSON }()
					pl.LastLBVersion = lbVer
				}
			}
		}
		s.playersMu.RUnlock()
	}
}
