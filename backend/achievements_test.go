package main

import (
	"testing"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestEvaluateAdvancementsThresholds(t *testing.T) {
	s := NewServer()
	const pid = uint32(1)

	s.playerStats[pid] = &PlayerStats{ChunksFounded: 1}
	unlocks := s.evaluateAdvancementsLocked(pid)
	if len(unlocks) != 1 || unlocks[0].Id != "first_foothold" {
		t.Fatalf("expected only first_foothold, got %+v", unlocks)
	}
	if unlocks[0].RewardFlagId != shapeWavyTriangle.variants[0] {
		t.Fatalf("expected first_foothold to reward a Wavy Triangle variant, got %d", unlocks[0].RewardFlagId)
	}
	if !s.unlockedAdvancements[pid]["first_foothold"] {
		t.Fatalf("first_foothold not recorded as unlocked")
	}
	// Shape unlocks grant every color variant, not just the representative.
	for _, v := range shapeWavyTriangle.variants {
		if !s.unlockedFlags[pid][v] {
			t.Fatalf("expected all Wavy Triangle variants unlocked, missing %d", v)
		}
	}

	// Re-evaluating with unchanged stats should not re-emit the same unlock.
	if again := s.evaluateAdvancementsLocked(pid); len(again) != 0 {
		t.Fatalf("expected no new unlocks on repeat evaluation, got %+v", again)
	}

	// Crossing a much higher threshold unlocks every intermediate tier at once.
	s.playerStats[pid].ChunksFounded = 2000
	unlocks = s.evaluateAdvancementsLocked(pid)
	want := map[string]bool{"homesteader": true, "frontier_baron": true, "empire_builder": true, "continental": true}
	got := make(map[string]bool)
	for _, u := range unlocks {
		got[u.Id] = true
	}
	for id := range want {
		if !got[id] {
			t.Errorf("expected %s to unlock at ChunksFounded=2000, got %+v", id, unlocks)
		}
	}
	if !s.unlockedFlags[pid][shapeDragon.variants[0]] {
		t.Fatalf("expected continental's reward shape (Dragon) unlocked")
	}
}

func TestEvaluateAdvancementsCollector(t *testing.T) {
	s := NewServer()
	const pid = uint32(1)

	// Nine flag-reward achievements is not enough for collector.
	s.playerStats[pid] = &PlayerStats{
		ChunksFounded:         2000, // first_foothold, homesteader, frontier_baron, empire_builder, continental (5)
		FurthestChunkDistance: 3000, // wanderer, pioneer, pathfinder, edge_of_the_map (4)
	}
	s.evaluateAdvancementsLocked(pid)
	if s.unlockedAdvancements[pid]["collector"] {
		t.Fatalf("collector should not unlock with only 9 flag-reward achievements")
	}

	// A tenth (land_surveyor, ChunksCleared>=1) should push it over the threshold.
	s.playerStats[pid].ChunksCleared = 1
	unlocks := s.evaluateAdvancementsLocked(pid)
	found := false
	for _, u := range unlocks {
		if u.Id == "collector" {
			found = true
		}
	}
	if !found || !s.unlockedAdvancements[pid]["collector"] {
		t.Fatalf("expected collector to unlock at 10 flag-reward achievements, unlocks=%+v", unlocks)
	}
	if s.unlockedAdvancements[pid]["collector"] && s.unlockedFlags[pid][0] {
		t.Fatalf("collector must not grant a flag reward (it's badge-only)")
	}
}

func TestSetCellRevealedChunkFoundedAndCleared(t *testing.T) {
	s := NewServer()
	const pid = uint32(1)
	chunkID := ChunkID{X: 5, Y: -3}
	collector := make(map[ChunkID]*pb.RevealedCells)

	s.setCellRevealed(chunkID, 0, pid, &collector)
	stats := s.playerStats[pid]
	if stats == nil {
		t.Fatalf("expected stats to be created for player")
	}
	if stats.ChunksFounded != 1 {
		t.Fatalf("expected ChunksFounded=1 after first reveal in a new chunk, got %d", stats.ChunksFounded)
	}
	if stats.CellsRevealed != 1 {
		t.Fatalf("expected CellsRevealed=1, got %d", stats.CellsRevealed)
	}
	if stats.ChunksCleared != 0 {
		t.Fatalf("expected ChunksCleared=0 with only 1/4096 cells revealed, got %d", stats.ChunksCleared)
	}

	// Revealing the same cell again must not double-count.
	s.setCellRevealed(chunkID, 0, pid, &collector)
	if stats.ChunksFounded != 1 || stats.CellsRevealed != 1 {
		t.Fatalf("re-revealing an already-revealed cell must be a no-op, got founded=%d cells=%d", stats.ChunksFounded, stats.CellsRevealed)
	}

	// Reveal the remaining 4095 cells to complete the chunk.
	for cell := uint32(1); cell < ChunkSize*ChunkSize; cell++ {
		s.setCellRevealed(chunkID, cell, pid, &collector)
	}
	if stats.CellsRevealed != ChunkSize*ChunkSize {
		t.Fatalf("expected CellsRevealed=%d, got %d", ChunkSize*ChunkSize, stats.CellsRevealed)
	}
	if stats.ChunksCleared != 1 {
		t.Fatalf("expected ChunksCleared=1 after revealing every cell, got %d", stats.ChunksCleared)
	}
	if stats.ChunksFounded != 1 {
		t.Fatalf("ChunksFounded must only increment once per chunk, got %d", stats.ChunksFounded)
	}
}

func TestSetCellRevealedFurthestChunkDistance(t *testing.T) {
	s := NewServer()
	const pid = uint32(1)
	collector := make(map[ChunkID]*pb.RevealedCells)

	s.setCellRevealed(ChunkID{X: 3, Y: -2}, 0, pid, &collector)
	if got := s.playerStats[pid].FurthestChunkDistance; got != 3 {
		t.Fatalf("expected Chebyshev distance 3, got %d", got)
	}

	// A closer chunk must not decrease the recorded maximum.
	s.setCellRevealed(ChunkID{X: 1, Y: 1}, 0, pid, &collector)
	if got := s.playerStats[pid].FurthestChunkDistance; got != 3 {
		t.Fatalf("furthest distance must not decrease, got %d", got)
	}

	// A farther chunk (Y dominates) must raise it.
	s.setCellRevealed(ChunkID{X: -2, Y: 7}, 0, pid, &collector)
	if got := s.playerStats[pid].FurthestChunkDistance; got != 7 {
		t.Fatalf("expected updated furthest distance 7, got %d", got)
	}
}

func TestFlagGatingPaidVsFreeVsGrandfathered(t *testing.T) {
	s := NewServer()
	const pid = uint32(1)

	dragonVariant := shapeDragon.variants[0]

	// A paid flag with no unlock and no prior equip is rejected.
	if s.isFlagUnlockedLocked(pid, dragonVariant) {
		t.Fatalf("Dragon variant should not be unlocked for a fresh player")
	}

	// Starter-tier (cost <= 5) variants are always usable.
	for _, v := range shapeTriangle.variants {
		if paidFlagIDs[v] {
			t.Fatalf("starter-tier variant %d must not be a paid flag", v)
		}
		if !s.isFlagUnlockedLocked(pid, v) {
			t.Fatalf("starter-tier variant %d must always be usable", v)
		}
	}

	// Once unlocked via an achievement, every variant of the rewarded shape
	// becomes usable.
	s.playerStats[pid] = &PlayerStats{ChunksFounded: 1} // unlocks first_foothold -> Wavy Triangle
	s.evaluateAdvancementsLocked(pid)
	for _, v := range shapeWavyTriangle.variants {
		if !s.isFlagUnlockedLocked(pid, v) {
			t.Fatalf("Wavy Triangle variant %d should be usable after first_foothold", v)
		}
	}
	if s.isFlagUnlockedLocked(pid, dragonVariant) {
		t.Fatalf("Dragon should still be locked; only Wavy Triangle was unlocked")
	}

	// Grandfathering: an already-equipped paid flag is usable without an unlock.
	brokenGuidonVariant := shapeBrokenGuidon.variants[0]
	s.playerFlags[pid] = brokenGuidonVariant
	if !s.isFlagUnlockedLocked(pid, brokenGuidonVariant) {
		t.Fatalf("currently-equipped paid flag must be grandfathered in")
	}
}

func TestSnapshotRoundTripStatsAndUnlocks(t *testing.T) {
	s := NewServer()
	const pid = uint32(7)

	s.playerStats[pid] = &PlayerStats{
		CellsRevealed:         12345,
		CorrectFlags:          10,
		ChunksFounded:         3,
		ChunksCleared:         1,
		CurrentSafeStreak:     4,
		MaxSafeStreak:         42,
		FurthestChunkDistance: 17,
		HighDensityReveals:    6,
	}
	s.evaluateAdvancementsLocked(pid) // ChunksFounded=3 unlocks first_foothold
	s.unlockedAdvancements[pid]["land_surveyor"] = true
	s.rebuildUnlockedFlagsLocked(pid)

	data := s.captureSnapshotData()

	restored := NewServer()
	restored.restoreSnapshotData(data)

	got := restored.playerStats[pid]
	if got == nil {
		t.Fatalf("expected player stats to survive snapshot round-trip")
	}
	if *got != *s.playerStats[pid] {
		t.Fatalf("stats mismatch after round-trip: got %+v, want %+v", *got, *s.playerStats[pid])
	}
	if !restored.unlockedAdvancements[pid]["land_surveyor"] {
		t.Fatalf("expected land_surveyor unlock to survive round-trip")
	}
	// unlockedFlags is derived: land_surveyor's reward shape must reappear
	// even though unlockedFlags itself was never copied.
	for _, v := range shapeVeryWavyGuidon.variants {
		if !restored.unlockedFlags[pid][v] {
			t.Fatalf("expected unlockedFlags rebuilt from unlockedAdvancements on restore (missing %d)", v)
		}
	}

	// Mutating the copy must not affect the original (deep copy check).
	got.CellsRevealed = 999999
	if s.playerStats[pid].CellsRevealed == 999999 {
		t.Fatalf("captureSnapshotData must deep-copy PlayerStats, not alias it")
	}
}
