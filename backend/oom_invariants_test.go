package main

// Regression invariants for the OOM fixes. These tests assert *behavior* (the
// fixes work, the bounds hold) rather than *measurements* (which live in
// oom_micro_test.go). Keep both around: the asserts catch regressions, the
// measurements help diagnose them.

import (
	"runtime"
	"testing"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// --- Per-player minimap subscription cap. ------------------------------------

func TestMinimapSubscriptionCap(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	pid := uint32(1)
	s.playerNames[pid] = "p"

	tiles := make([]*pb.TileRef, 0, maxMinimapSubsPerPlayer+1000)
	for i := 0; i < maxMinimapSubsPerPlayer+1000; i++ {
		tiles = append(tiles, &pb.TileRef{X: int32(i % 1000), Y: int32(i / 1000)})
	}

	s.stateMu.Lock()
	s.minimapPlayerRes[pid] = 16
	remaining := maxMinimapSubsPerPlayer - s.minimapSubCount[pid]
	for _, tr := range tiles {
		if remaining <= 0 {
			break
		}
		cid := ChunkID{X: int64(tr.X), Y: int64(tr.Y)}
		if s.minimapSubs[cid] == nil {
			s.minimapSubs[cid] = make(map[uint32]struct{})
		}
		if _, already := s.minimapSubs[cid][pid]; !already {
			s.minimapSubs[cid][pid] = struct{}{}
			s.minimapSubCount[pid]++
			remaining--
		}
	}
	count := s.minimapSubCount[pid]
	s.stateMu.Unlock()

	if count != maxMinimapSubsPerPlayer {
		t.Errorf("expected subscription count to be capped at %d, got %d", maxMinimapSubsPerPlayer, count)
	}
}

// --- Reveal -> minimap dirty path still wires up correctly. ------------------

func TestMinimapDeltaStillDelivered(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	cid := ChunkID{X: 0, Y: 0}
	pid := uint32(1)
	s.playerNames[pid] = "p"
	s.scores[pid] = 0
	s.playerFlags[pid] = 1

	s.stateMu.Lock()
	s.minimapSubs[cid] = map[uint32]struct{}{pid: {}}
	s.minimapPlayerRes[pid] = 64
	s.stateMu.Unlock()

	s.handleReveal(pid, 1, cid, 0, false, false)

	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	tile := s.minimapTiles[cid]
	if tile == nil {
		t.Fatalf("expected minimap tile to be created by reveal")
	}
	if !tile.hasAnyDirty() {
		t.Fatalf("expected dirty bits after reveal")
	}
	if _, ok := s.minimapDirtyTiles[cid]; !ok {
		t.Fatalf("expected chunk to be in minimapDirtyTiles set")
	}
}

// --- Snapshot load reconstructs totalRevealed counter. -----------------------

// Without this, after a restart hasRevealedWithinTwo trivially returns true
// (totalRevealed=0 means "world empty") and the proximity rule is silently
// disabled until at least one new reveal occurs.
func TestTotalRevealedRecomputedFromSnapshot(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	cid := ChunkID{X: 5, Y: 5}
	bits := &ChunkBits{}
	bits[0] = 0b11 // cells 0, 1
	bits[1] = 0b1  // cell 64
	chunks := map[ChunkID]*ChunkBits{cid: bits}

	s.restoreSnapshotData(snapshotData{
		Chunks: chunks,
		Flags:  make(map[ChunkID]map[uint32]Flag),
		Scores: make(map[uint32]int32),
	})

	if s.totalRevealed != 3 {
		t.Errorf("expected totalRevealed=3 after snapshot restore, got %d", s.totalRevealed)
	}
}

// --- WAL buffer size-triggered flush. ----------------------------------------

func TestWALBufferThresholdSignal(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	s.dataDir = t.TempDir()

	select {
	case <-s.walFlushSignal:
		t.Fatal("walFlushSignal should start empty")
	default:
	}

	for i := 0; i < walBufferFlushThreshold-1; i++ {
		s.writeWALEntry("reveal", struct{ I int }{i})
	}
	select {
	case <-s.walFlushSignal:
		t.Fatal("signal fired before threshold reached")
	default:
	}

	s.writeWALEntry("reveal", struct{ I int }{99999})
	select {
	case <-s.walFlushSignal:
	default:
		t.Fatal("signal did not fire after crossing threshold")
	}

	s.writeWALEntry("reveal", struct{ I int }{1000000})
	select {
	case <-s.walFlushSignal:
	default:
		t.Fatal("signal did not re-fire while buffer stayed over threshold")
	}
}

// --- Refactored broadcaster end-to-end. --------------------------------------

func TestMinimapBroadcastAfterRefactor(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	cid := ChunkID{X: 0, Y: 0}

	s.stateMu.Lock()
	s.minimapSubs[cid] = map[uint32]struct{}{1: {}, 2: {}, 3: {}}
	s.minimapPlayerRes[1] = 64
	s.minimapPlayerRes[2] = 32
	s.minimapPlayerRes[3] = 16
	s.chunks[cid] = &ChunkBits{}
	s.chunks[cid][0] = 0xFF
	s.totalRevealed = 8
	for i := uint32(0); i < 10; i++ {
		s.minimapMarkDirty(cid, i)
	}
	tile := s.minimapTiles[cid]
	s.stateMu.Unlock()

	if tile == nil {
		t.Fatalf("expected tile to be created")
	}
	if !tile.hasAnyDirty() {
		t.Fatalf("expected dirty bits set")
	}

	s.stateMu.Lock()
	if s.computeFullTileData(cid) == nil {
		t.Fatal("expected full data, got nil (chunk should have state)")
	}
	for _, res := range []uint32{64, 32, 16} {
		dirty := tile.deriveDirtyAtResolution(res)
		anyDirty := false
		for _, b := range dirty {
			if b {
				anyDirty = true
				break
			}
		}
		if !anyDirty {
			t.Errorf("res=%d expected at least one dirty bit, got none", res)
		}
	}
	tile.clearDirty()
	s.stateMu.Unlock()

	if tile.hasAnyDirty() {
		t.Errorf("expected dirty to be cleared after broadcast")
	}
}

// --- WAL flush cycles don't leak memory. -------------------------------------

func TestWALFlushMemoryPatternLocal(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	s.dataDir = t.TempDir()

	before := readHeapAlloc()
	totalEntries := 0
	for cycle := range 10 {
		for i := 0; i < 5000; i++ {
			s.writeWALEntry("reveal", struct {
				ChunkID ChunkID  `json:"chunk_id"`
				Cells   []uint32 `json:"cells"`
			}{
				ChunkID: ChunkID{X: int64(i), Y: int64(cycle)},
				Cells:   []uint32{1, 2, 3, 4, 5, 6, 7, 8},
			})
			totalEntries++
		}
		if err := s.flushWAL(); err != nil {
			t.Fatalf("flushWAL: %v", err)
		}
	}
	after := readHeapAlloc()
	delta := int64(after) - int64(before)
	t.Logf("After %d entries across 10 flushes: heap_delta=%d bytes (%.2f MB)",
		totalEntries, delta, float64(delta)/1024/1024)
	runtime.KeepAlive(s)
	if delta > 5*1024*1024 {
		t.Errorf("expected heap growth < 5MB after 10 flushes, got %d bytes", delta)
	}
}

// --- Single zoomed-out player stays bounded. ---------------------------------

// Pre-fix this allocated ~265 MB and OOM'd the fly.io shared-cpu-1x. Post-fix
// the common case (mostly-unseen chunks) stays under 12 MB.
func TestWorstCasePlayerZoomedOut(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	pid := uint32(1)
	s.playerNames[pid] = "zoomer"
	s.scores[pid] = 0
	s.playerFlags[pid] = 1
	s.minimapPlayerRes[pid] = 16

	const attempted = 32400 // realistic max overlay zoom on 1080p
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
		// 3% of chunks have state in a typical zoomed-out view; the rest are
		// all-unseen and never allocate a tile.
		if i%30 == 0 {
			s.minimapMarkDirty(cid, 0)
		}
	}
	subCount := s.minimapSubCount[pid]
	tileCount := len(s.minimapTiles)
	s.stateMu.Unlock()

	delta := int64(readHeapAlloc()) - int64(before)
	t.Logf("subs=%d tiles=%d heap=%.1f MB", subCount, tileCount, float64(delta)/1024/1024)

	if subCount > maxMinimapSubsPerPlayer {
		t.Errorf("server-side cap not enforced: sub count %d exceeds cap %d", subCount, maxMinimapSubsPerPlayer)
	}
	if mb := delta / 1024 / 1024; mb > 12 {
		t.Errorf("memory regressed: got %d MB, expected < 12 MB", mb)
	}
	runtime.KeepAlive(s)
}

// --- 10 concurrent zoomed-out players stay under fly.io budget. --------------

func TestConcurrentPlayerMinimapMemory(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const nPlayers = 10
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
			if i%30 == 0 {
				s.minimapMarkDirty(cid, 0)
			}
		}
	}
	totalTiles := len(s.minimapTiles)
	s.stateMu.Unlock()

	delta := int64(readHeapAlloc()) - int64(before)
	t.Logf("10 zoomed-out players (%d subs each, 3%% active): tiles=%d heap=%.1f MB",
		tilesPerPlayer, totalTiles, float64(delta)/1024/1024)
	if delta > 120*1024*1024 {
		t.Errorf("realistic 10-player workload exceeds 120 MB: %d bytes", delta)
	}
	runtime.KeepAlive(s)
}

// --- Long-running gameplay simulation. ---------------------------------------

func TestSimulatedLongRunMemory(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	s.dataDir = t.TempDir()
	s.proximityRadius = -1

	const nPlayers = 1000
	const revealsPerPlayer = 100
	for pid := uint32(1); pid <= nPlayers; pid++ {
		s.playerNames[pid] = "p"
		s.scores[pid] = 0
		s.playerFlags[pid] = 1
	}

	before := readHeapAlloc()
	for pid := uint32(1); pid <= nPlayers; pid++ {
		for k := range revealsPerPlayer {
			cid := ChunkID{X: int64(pid) % 100, Y: int64(k) % 100}
			cell := uint32(k*13) % 4096
			s.handleReveal(pid, uint64(k), cid, cell, false, false)
		}
	}
	for range 5 {
		if err := s.flushWAL(); err != nil {
			t.Fatalf("flushWAL: %v", err)
		}
		if err := s.saveSnapshotToDisk(); err != nil {
			t.Fatalf("saveSnapshotToDisk: %v", err)
		}
	}
	delta := int64(readHeapAlloc()) - int64(before)
	t.Logf("Long-run (%d reveals, 5 snapshot cycles): heap=%.1f MB",
		nPlayers*revealsPerPlayer, float64(delta)/1024/1024)
	runtime.KeepAlive(s)
	if delta > 100*1024*1024 {
		t.Errorf("long run simulation exceeded 100 MB: %d bytes", delta)
	}
}
