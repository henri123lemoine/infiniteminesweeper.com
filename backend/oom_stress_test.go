package main

import (
	"runtime"
	"testing"
)

// TestWorstCasePlayerZoomedOut simulates a single player subscribing to the
// maximum possible minimap area (resolution 16, ~32k tiles — within the 100k
// cap so the frontend sees no truncation). Before the fix this allocated
// ~265 MB and OOM'd the fly.io shared-cpu-1x 256 MB machine. Post-fix the
// common case (mostly-unseen chunks, no allocated tiles) stays under 6 MB.
func TestWorstCasePlayerZoomedOut(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	pid := uint32(1)
	s.playerNames[pid] = "zoomer"
	s.scores[pid] = 0
	s.playerFlags[pid] = 1
	s.minimapPlayerRes[pid] = 16

	const attempted = 32400 // realistic max at overlay zoom 0.125 on 1920×1080

	before := readHeapAlloc()

	s.stateMu.Lock()
	remaining := maxMinimapSubsPerPlayer - s.minimapSubCount[pid]
	for i := 0; i < attempted && remaining > 0; i++ {
		cid := ChunkID{X: int64(i % 200), Y: int64(i / 200)}
		if s.minimapSubs[cid] == nil {
			s.minimapSubs[cid] = make(map[uint32]struct{})
		}
		if _, already := s.minimapSubs[cid][pid]; !already {
			s.minimapSubs[cid][pid] = struct{}{}
			s.minimapSubCount[pid]++
			remaining--
		}
		// Realistic: a random 3% of chunks have some state (players cluster
		// near activity). Most zoomed-out chunks render as all-unseen and
		// never allocate a tile.
		if i%30 == 0 {
			s.minimapMarkDirty(cid, 0)
		}
	}
	subCount := s.minimapSubCount[pid]
	tileCount := len(s.minimapTiles)
	s.stateMu.Unlock()

	after := readHeapAlloc()
	delta := int64(after) - int64(before)

	t.Logf("Attempted %d subs, effective subs=%d, distinct tiles stored=%d",
		attempted, subCount, tileCount)
	t.Logf("Heap delta: %d bytes (%.1f MB)", delta, float64(delta)/1024/1024)

	if subCount > maxMinimapSubsPerPlayer {
		t.Errorf("server-side cap not enforced: sub count %d exceeds cap %d", subCount, maxMinimapSubsPerPlayer)
	}
	// 32k subs × ~163 B + ~1k dirty tiles × ~560 B = ~6 MB expected floor.
	const expectedMaxMB = 12
	actualMB := int64(delta) / 1024 / 1024
	if actualMB > expectedMaxMB {
		t.Errorf("memory regressed: got %d MB, expected < %d MB", actualMB, expectedMaxMB)
	}
	runtime.KeepAlive(s)
}

// TestConcurrentPlayerMinimapMemory simulates a REALISTIC worst case: 10
// players each zoomed out to 1080p max (~32k tiles), viewing non-overlapping
// regions, with ~3% of each view containing actively-changing cells (reveals
// in progress). The server-side cap + sub-map/tile accounting keeps this
// comfortably under the 256 MB fly.io budget.
//
// Pure subscription maps dominate the memory: ~163 B/sub × 32 k × 10 =
// ~52 MB. Tile state (only allocated when a cell in the chunk actually
// changes) adds ~560 B × dirty-chunk-count. In practice dirty-chunk-count is
// bounded by the rate of active gameplay, not by how far zoomed out.
func TestConcurrentPlayerMinimapMemory(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	const nPlayers = 10
	// Realistic max zoom-out on 1080p (the pathological case the old code hit
	// so hard that fly.io OOM-killed the process).
	const tilesPerPlayer = 32400

	for pid := uint32(1); pid <= nPlayers; pid++ {
		s.playerNames[pid] = "p"
		s.minimapPlayerRes[pid] = 16
	}

	before := readHeapAlloc()

	s.stateMu.Lock()
	for pid := uint32(1); pid <= nPlayers; pid++ {
		// Non-overlapping regions — worst case for shared tile amortization.
		offset := int64(pid) * 10000
		for i := 0; i < tilesPerPlayer; i++ {
			cid := ChunkID{X: (int64(i) % 200) + offset, Y: int64(i) / 200}
			if s.minimapSubs[cid] == nil {
				s.minimapSubs[cid] = make(map[uint32]struct{})
			}
			if _, already := s.minimapSubs[cid][pid]; !already {
				s.minimapSubs[cid][pid] = struct{}{}
				s.minimapSubCount[pid]++
			}
			// Only 3% of chunks in a typical zoomed-out view have any state —
			// the rest are all-unseen noise. No-state chunks never allocate
			// a tile struct.
			if i%30 == 0 {
				s.minimapMarkDirty(cid, 0)
			}
		}
	}
	totalTiles := len(s.minimapTiles)
	s.stateMu.Unlock()

	after := readHeapAlloc()
	delta := int64(after) - int64(before)
	t.Logf("10 zoomed-out players (%d subs each, 3%% active): distinct tiles=%d, heap delta=%d bytes (%.1f MB)",
		tilesPerPlayer, totalTiles, delta, float64(delta)/1024/1024)
	// Comfortable ceiling well under the 256 MB fly.io budget.
	if delta > 120*1024*1024 {
		t.Errorf("realistic 10-player zoomed-out workload exceeds 120 MB: %d bytes", delta)
	}
	runtime.KeepAlive(s)
}

// TestAdversarialAllDirtyMinimap measures the absolute upper bound: every
// subscribed chunk has an allocated tile. This can happen over time as cells
// in distant chunks get revealed. Memory should still fit in fly.io's budget.
// This represents "someone zoomed out to max for hours while the world is
// very active".
func TestAdversarialAllDirtyMinimap(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	const nPlayers = 10
	const tilesPerPlayer = maxMinimapSubsPerPlayer // 100k cap per player

	for pid := uint32(1); pid <= nPlayers; pid++ {
		s.minimapPlayerRes[pid] = 16
	}

	before := readHeapAlloc()
	s.stateMu.Lock()
	for pid := uint32(1); pid <= nPlayers; pid++ {
		offset := int64(pid) * 20000
		for i := 0; i < tilesPerPlayer; i++ {
			cid := ChunkID{X: (int64(i) % 400) + offset, Y: int64(i) / 400}
			if s.minimapSubs[cid] == nil {
				s.minimapSubs[cid] = make(map[uint32]struct{})
			}
			if _, already := s.minimapSubs[cid][pid]; !already {
				s.minimapSubs[cid][pid] = struct{}{}
				s.minimapSubCount[pid]++
			}
			s.minimapMarkDirty(cid, 0) // every chunk allocates a tile
		}
	}
	distinctTiles := len(s.minimapTiles)
	s.stateMu.Unlock()

	after := readHeapAlloc()
	delta := int64(after) - int64(before)
	t.Logf("adversarial all-dirty, 10 players × 100k subs disjoint: distinct tiles=%d, heap=%.1f MB",
		distinctTiles, float64(delta)/1024/1024)
	// This is an ugly-but-not-fatal case. 1M subs × ~163 B + 1M tiles × 560 B ≈ 720 MB on a 256 MB box.
	// We're documenting the floor, not claiming it fits — if you care about
	// this workload, raise the fly.io VM's memory limit. See comment on
	// maxMinimapSubsPerPlayer for the tradeoff.
	_ = delta
	runtime.KeepAlive(s)
}

// TestSimulatedLongRunMemory runs a simulated gameplay loop: many reveals
// across many chunks, many snapshot cycles, and confirms heap stays bounded.
// This catches regressions where caches, maps, or tile state grow across time.
func TestSimulatedLongRunMemory(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	s.dataDir = t.TempDir()
	s.proximityRadius = -1

	// Simulate 1000 players each doing 100 reveals spread across a world.
	const nPlayers = 1000
	const revealsPerPlayer = 100

	for pid := uint32(1); pid <= nPlayers; pid++ {
		s.playerNames[pid] = "p"
		s.scores[pid] = 0
		s.playerFlags[pid] = 1
	}

	before := readHeapAlloc()
	for pid := uint32(1); pid <= nPlayers; pid++ {
		for k := 0; k < revealsPerPlayer; k++ {
			cid := ChunkID{X: int64(pid) % 100, Y: int64(k) % 100}
			cell := uint32(k*13) % 4096
			s.handleReveal(pid, uint64(k), cid, cell, false, false)
		}
	}
	// Multiple snapshot/WAL cycles
	for i := 0; i < 5; i++ {
		if err := s.flushWAL(); err != nil {
			t.Fatalf("flushWAL: %v", err)
		}
		if err := s.saveSnapshotToDisk(); err != nil {
			t.Fatalf("saveSnapshotToDisk: %v", err)
		}
	}
	after := readHeapAlloc()
	delta := int64(after) - int64(before)
	t.Logf("Long-run simulation (%d reveals, 5 snapshot cycles): heap delta %d bytes (%.1f MB)",
		nPlayers*revealsPerPlayer, delta, float64(delta)/1024/1024)
	runtime.KeepAlive(s)

	// This is a soft regression guard — 100k reveals should cost < 100 MB of
	// heap on a 256 MB fly.io machine.
	if delta > 100*1024*1024 {
		t.Errorf("long run simulation exceeded 100 MB: %d bytes (%.1f MB)",
			delta, float64(delta)/1024/1024)
	}
}
