package main

import (
	"testing"
)

func TestGetUserRank(t *testing.T) {
	s := NewServer()

	// Add some test players with different scores
	s.stateMu.Lock()
	s.scores[1] = 1000
	s.playerNames[1] = "Player1"
	s.scores[2] = 500
	s.playerNames[2] = "Player2"
	s.scores[3] = 2000
	s.playerNames[3] = "Player3"
	s.scores[4] = 500 // Tie with Player2
	s.playerNames[4] = "Player4"
	s.stateMu.Unlock()

	// Test rank calculation
	s.stateMu.RLock()
	rank1 := s.getUserRankUnsafe(1000) // Player1
	rank2 := s.getUserRankUnsafe(500)  // Player2/Player4 (tie)
	rank3 := s.getUserRankUnsafe(2000) // Player3
	rank4 := s.getUserRankUnsafe(500)  // Player4 (same as Player2)
	s.stateMu.RUnlock()

	// Player3 should be rank 1 (highest score)
	if rank3 != 1 {
		t.Errorf("Expected rank 1 for score 2000, got %d", rank3)
	}

	// Player1 should be rank 2
	if rank1 != 2 {
		t.Errorf("Expected rank 2 for score 1000, got %d", rank1)
	}

	// Player2 and Player4 should both be rank 3 (tie)
	if rank2 != 3 {
		t.Errorf("Expected rank 3 for score 500, got %d", rank2)
	}
	if rank4 != 3 {
		t.Errorf("Expected rank 3 for score 500 (tie), got %d", rank4)
	}

	// A score above everyone ranks first (rank = 1 + players strictly above)
	s.stateMu.RLock()
	rankTop := s.getUserRankUnsafe(9999)
	s.stateMu.RUnlock()

	if rankTop != 1 {
		t.Errorf("Expected rank 1 for a score above all players, got %d", rankTop)
	}
}

func TestStableSorting(t *testing.T) {
	s := NewServer()

	// Add players with identical scores but different names
	s.stateMu.Lock()
	s.scores[1] = 1000
	s.playerNames[1] = "Alice"
	s.playerFlags[1] = 1
	s.scores[2] = 1000
	s.playerNames[2] = "Bob"
	s.playerFlags[2] = 2
	s.scores[3] = 1000
	s.playerNames[3] = "Charlie"
	s.playerFlags[3] = 3
	s.stateMu.Unlock()

	// Build leaderboard multiple times to ensure stable ordering
	var firstOrder []string
	var secondOrder []string

	s.stateMu.Lock()
	s.buildLeaderboardUnsafe()
	lb1 := s.lbProto
	s.stateMu.Unlock()

	// Decode and extract names from first build
	msg1, err := decodeMsg(lb1)
	if err != nil {
		t.Fatalf("Failed to decode first leaderboard: %v", err)
	}
	lb1Entries := msg1.GetLeaderboard().Entries
	for _, entry := range lb1Entries {
		firstOrder = append(firstOrder, entry.Name)
	}

	// Build leaderboard again
	s.stateMu.Lock()
	s.buildLeaderboardUnsafe()
	lb2 := s.lbProto
	s.stateMu.Unlock()

	// Decode and extract names from second build
	msg2, err := decodeMsg(lb2)
	if err != nil {
		t.Fatalf("Failed to decode second leaderboard: %v", err)
	}
	lb2Entries := msg2.GetLeaderboard().Entries
	for _, entry := range lb2Entries {
		secondOrder = append(secondOrder, entry.Name)
	}

	// Verify the order is stable (no flickering)
	if len(firstOrder) != len(secondOrder) {
		t.Errorf("Leaderboard size changed: %d vs %d", len(firstOrder), len(secondOrder))
	}

	for i := 0; i < len(firstOrder); i++ {
		if firstOrder[i] != secondOrder[i] {
			t.Errorf("Order changed at position %d: %s vs %s", i, firstOrder[i], secondOrder[i])
		}
	}

	// Verify alphabetical ordering for tied scores
	expectedOrder := []string{"Alice", "Bob", "Charlie"}
	for i, name := range firstOrder {
		if name != expectedOrder[i] {
			t.Errorf("Expected %s at position %d, got %s", expectedOrder[i], i, name)
		}
	}
}
