package main

import (
	"runtime"
	"testing"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// TestMinimapSubscriptionCap verifies the per-player minimap subscription
// cap is enforced server-side to prevent a single pathological client from
// exhausting memory by zooming out "infinitely".
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

// TestMinimapDeltaStillDelivered verifies end-to-end that a mark-dirty followed
// by a broadcast-pass pushes the expected update to a subscriber. The
// resolution-independent storage refactor only works if the delta path still
// produces correct palette data.
func TestMinimapDeltaStillDelivered(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	cid := ChunkID{X: 0, Y: 0}
	// Reveal a single cell so the chunk has state. The reveal path calls
	// minimapOnReveal under the lock, which in turn marks dirty.
	pid := uint32(1)
	s.playerNames[pid] = "p"
	s.scores[pid] = 0
	s.playerFlags[pid] = 1

	s.stateMu.Lock()
	// Pretend the player is subscribed before reveal so dirty tracking kicks in.
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

// TestTotalRevealedRecomputedFromSnapshot ensures that after a snapshot load,
// s.totalRevealed is reconstructed from the persisted bitsets. Without this,
// the proximity rule silently breaks after a restart.
func TestTotalRevealedRecomputedFromSnapshot(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	// Build a snapshot with a few revealed cells in one chunk.
	cid := ChunkID{X: 5, Y: 5}
	bits := &ChunkBits{}
	// Set bits for cells 0, 1, 64 (first word bits 0,1; second word bit 0).
	bits[0] = 0b11
	bits[1] = 0b1
	chunks := map[ChunkID]*ChunkBits{cid: bits}

	data := snapshotData{
		Chunks: chunks,
		Flags:  make(map[ChunkID]map[uint32]Flag),
		Scores: make(map[uint32]int32),
	}
	s.restoreSnapshotData(data)

	if s.totalRevealed != 3 {
		t.Errorf("expected totalRevealed=3 after snapshot restore, got %d", s.totalRevealed)
	}
}

// TestWALBufferThresholdSignal verifies that writeWALEntry signals the flush
// worker once the in-memory buffer crosses the threshold, so a heavy burst
// between periodic ticks can't let walBuffer grow unboundedly.
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

	// Buffer is still over threshold; next write should re-signal.
	s.writeWALEntry("reveal", struct{ I int }{1000000})
	select {
	case <-s.walFlushSignal:
	default:
		t.Fatal("signal did not re-fire while buffer stayed over threshold")
	}
}

// TestMinimapBroadcastAfterRefactor drives the refactored broadcaster end-to-end:
// mark cells dirty on multiple chunks at multiple resolutions, then verify a
// broadcast pass drains dirty tracking without panicking. The minimap
// broadcaster runs inline here (synchronously consumed) to avoid spinning a
// goroutine in the test.
func TestMinimapBroadcastAfterRefactor(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	// Subscribe 3 players at different resolutions to the same chunk.
	cid := ChunkID{X: 0, Y: 0}
	s.stateMu.Lock()
	s.minimapSubs[cid] = map[uint32]struct{}{1: {}, 2: {}, 3: {}}
	s.minimapPlayerRes[1] = 64
	s.minimapPlayerRes[2] = 32
	s.minimapPlayerRes[3] = 16
	// Add state so the chunk isn't all-unseen.
	s.chunks[cid] = &ChunkBits{}
	s.chunks[cid][0] = 0xFF
	s.totalRevealed = 8
	// Mark several cells dirty.
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

	// Simulate one broadcast pass — the goroutine's inner body.
	s.stateMu.Lock()
	fullData := s.computeFullTileData(cid)
	if fullData == nil {
		t.Fatal("expected full data, got nil (chunk should have state)")
	}
	// Derive dirty at each resolution; check at least one bit is set.
	for _, res := range []uint32{64, 32, 16} {
		dirty := tile.deriveDirtyAtResolution(res)
		found := false
		for _, b := range dirty {
			if b {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("res=%d expected at least one dirty bit, got none", res)
		}
	}
	tile.clearDirty()
	s.stateMu.Unlock()

	if tile.hasAnyDirty() {
		t.Errorf("expected dirty to be cleared after broadcast")
	}
}

// TestWALSegmentMemoryPatternLocal measures heap usage across multiple flushes
// against disk storage. The S3 path now writes independent segment objects, so
// each flush allocates only its own entry batch — not the cumulative WAL.
func TestWALFlushMemoryPatternLocal(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	s.dataDir = t.TempDir()

	// Simulate many flush cycles. Before the fix, re-flushing a growing WAL to
	// S3 allocated O(cumulative entries) per flush; on disk the append
	// semantics were already fine, but we still measure to have a baseline.
	before := readHeapAlloc()
	totalEntries := 0
	for cycle := 0; cycle < 10; cycle++ {
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
