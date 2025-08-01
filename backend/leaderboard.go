package main

import (
	"fmt"
	pb "infinite-minesweeper/backend/proto"
	"sort"
	"strconv"
	"time"
)

// Leaderboard helpers

type lbEntry struct {
	PlayerID int32  `json:"playerId"`
	Name     string `json:"name"`
	Score    string `json:"score"`
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
	for pid, sc := range s.scores {
		entries = append(entries, lbEntry{PlayerID: pid, Name: s.playerNames[pid], Score: formatScore(sc)})
	}
	// Sort by the real uint32 score
	sort.Slice(entries, func(i, j int) bool {
		return s.scores[entries[i].PlayerID] > s.scores[entries[j].PlayerID]
	})
	if len(entries) > 20 {
		entries = entries[:20]
	}

	s.lbVersion++
	lbMsg := &pb.Msg{Payload: &pb.Msg_Leaderboard{Leaderboard: &pb.Leaderboard{
		Version: s.lbVersion,
		Entries: make([]*pb.LeaderboardEntry, len(entries)),
	}}}
	for i, e := range entries {
		lbMsg.GetLeaderboard().Entries[i] = &pb.LeaderboardEntry{PlayerId: e.PlayerID, Name: e.Name, Score: e.Score}
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

func (s *Server) sendScoreUpdate(playerID int32, score int32, worldX, worldY int, delta int32) {
	msg := &pb.Msg{Payload: &pb.Msg_ScoreUpdate{ScoreUpdate: &pb.ScoreUpdate{
		Score:  score,
		WorldX: int32(worldX),
		WorldY: int32(worldY),
		Delta:  delta,
	}}}
	s.sendToPlayer(playerID, mustProto(msg))
}
