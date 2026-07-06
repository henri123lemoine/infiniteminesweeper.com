package main

import (
	"os"
	"testing"
)

func TestTerritoryBlockIndex(t *testing.T) {
	cases := []struct {
		cell uint32
		want int
	}{
		{0, 0},               // top-left cell -> block 0
		{7, 0},               // still block 0 (x=7)
		{8, 1},               // x=8 -> block 1
		{63, 7},              // top-right cell -> block 7
		{64 * 8, 8},          // y=8 -> second block row
		{64*63 + 63, 63},     // bottom-right cell -> block 63
		{64*9 + 9, 8 + 1},    // (9,9) -> block (1,1)
		{64*17 + 42, 16 + 5}, // (42,17) -> block (5,2)
		{64*56 + 16, 56 + 2}, // (16,56) -> block (2,7)
		{64*31 + 31, 24 + 3}, // (31,31) -> block (3,3)
		{64*32 + 32, 32 + 4}, // (32,32) -> block (4,4)
		{64*15 + 55, 8 + 6},  // (55,15) -> block (6,1)
		{64*40 + 63, 40 + 7}, // (63,40) -> block (7,5)
		{64*8 + 0, 8},        // (0,8) -> block (0,1)
		{64*62 + 1, 56},      // (1,62) -> block (0,7)
		{64*23 + 24, 16 + 3}, // (24,23) -> block (3,2)
	}
	for _, c := range cases {
		if got := territoryBlockIndex(c.cell); got != c.want {
			t.Errorf("territoryBlockIndex(%d) = %d, want %d", c.cell, got, c.want)
		}
	}
}

func TestTerritoryMajorityVote(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: 1, Y: 2}

	s.stateMu.Lock()
	// Player 7 reveals 5 cells of block 0, player 9 reveals 3 -> 7 dominates
	for i := uint32(0); i < 5; i++ {
		s.recordTerritoryLocked(cid, i, 7)
	}
	for i := uint32(64); i < 64+3; i++ { // second row, still block 0
		s.recordTerritoryLocked(cid, i, 9)
	}
	s.stateMu.Unlock()

	if got := s.territoryOwner(cid, 0); got != 7 {
		t.Fatalf("block 0 owner = %d, want 7", got)
	}

	// A sustained takeover flips the block: 9 needs to out-reveal 7's lead
	s.stateMu.Lock()
	for i := uint32(128); i < 128+6; i++ { // third row, still block 0
		s.recordTerritoryLocked(cid, i, 9)
	}
	s.stateMu.Unlock()

	if got := s.territoryOwner(cid, 0); got != 9 {
		t.Fatalf("block 0 owner after takeover = %d, want 9", got)
	}

	// Owner 0 (pre-provenance replay) records nothing
	if got := s.territoryOwner(cid, 63*64+63); got != 0 {
		t.Fatalf("untouched block owner = %d, want 0", got)
	}
	s.stateMu.Lock()
	s.recordTerritoryLocked(cid, 63*64+63, 0)
	s.stateMu.Unlock()
	if s.territory[cid][territoryBlockIndex(63*64+63)].Votes != 0 {
		t.Fatalf("owner-0 vote should not be recorded")
	}
}

func TestTerritorySnapshotRoundtrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "territory-snap")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s := NewServer()
	s.dataDir = dir
	cid := ChunkID{X: -3, Y: 4}
	s.stateMu.Lock()
	for i := uint32(0); i < 10; i++ {
		s.recordTerritoryLocked(cid, i, 42)
	}
	s.recordTerritoryLocked(cid, 64*8, 43) // block (0,1)
	s.stateMu.Unlock()

	if err := s.saveSnapshotToDisk(); err != nil {
		t.Fatalf("save: %v", err)
	}

	s2 := NewServer()
	s2.dataDir = dir
	if err := s2.loadSnapshotFromDisk(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := s2.territoryOwner(cid, 5); got != 42 {
		t.Fatalf("restored block 0 owner = %d, want 42", got)
	}
	if got := s2.territoryOwner(cid, 64*8); got != 43 {
		t.Fatalf("restored block (0,1) owner = %d, want 43", got)
	}
}

func TestTerritoryWALReplay(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: 5, Y: 5}
	s.replayReveal(cid, []uint32{0, 1, 2}, 21)
	s.replayReveal(cid, []uint32{3, 4}, 0) // pre-provenance entry: no votes

	if got := s.territoryOwner(cid, 1); got != 21 {
		t.Fatalf("replayed owner = %d, want 21", got)
	}
	// Re-replaying already-set bits must not double-vote
	votes := s.territory[cid][0].Votes
	s.replayReveal(cid, []uint32{0, 1, 2}, 21)
	if s.territory[cid][0].Votes != votes {
		t.Fatalf("idempotent replay changed votes: %d -> %d", votes, s.territory[cid][0].Votes)
	}
}
