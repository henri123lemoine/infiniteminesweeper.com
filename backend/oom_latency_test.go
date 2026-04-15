package main

import (
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// TestMassSubscribeLockHoldTime measures end-to-end time for a mass
// minimap subscribe. With the batched implementation the WRITE lock is only
// held for ~minimapSubscribeBatchSize * a-few-hundred-ns at a time (Phase 1);
// the rest of the work (Phase 2: palette compute + send) runs under read
// locks that don't block other readers and only briefly gate writers.
func TestMassSubscribeLockHoldTime(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	pid := uint32(1)
	s.playerNames[pid] = "p"
	s.scores[pid] = 0
	s.playerFlags[pid] = 1

	const nStateful = 5000
	for i := 0; i < nStateful; i++ {
		cid := ChunkID{X: int64(i % 100), Y: int64(i / 100)}
		bits := &ChunkBits{}
		bits[0] = 0xFFFFFFFFFFFFFFFF
		bits[1] = 0xFFFFFFFFFFFFFFFF
		s.chunks[cid] = bits
	}
	s.totalRevealed = uint64(nStateful) * 128

	const nTiles = 32400
	tiles := make([]*pb.TileRef, 0, nTiles)
	for i := 0; i < nTiles; i++ {
		tiles = append(tiles, &pb.TileRef{X: int32(i % 200), Y: int32(i / 200)})
	}

	start := time.Now()
	s.handleMinimapSubscribe(pid, &pb.SubscribeTiles{Tiles: tiles, Resolution: 16})
	elapsed := time.Since(start)
	t.Logf("batched mass-subscribe %d tiles (%d stateful): %v total wall-clock",
		nTiles, nStateful, elapsed)
}

// TestMassSubscribeWriteLockBursts measures the worst-case continuous write-
// lock hold during a mass subscribe — i.e. the longest gap during which a
// concurrent Reveal would block. With batching this should be < 10 ms even
// for 32 k tiles.
func TestMassSubscribeWriteLockBursts(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	pid := uint32(1)

	const nTiles = 32400
	tiles := make([]*pb.TileRef, 0, nTiles)
	for i := 0; i < nTiles; i++ {
		tiles = append(tiles, &pb.TileRef{X: int32(i % 200), Y: int32(i / 200)})
	}

	// Kick off the subscribe in a goroutine; from the main goroutine, race
	// to acquire the write lock and record the longest wait to get it —
	// that's the worst-case block experienced by a concurrent writer.
	done := make(chan struct{})
	go func() {
		s.handleMinimapSubscribe(pid, &pb.SubscribeTiles{Tiles: tiles, Resolution: 64})
		close(done)
	}()

	var worstWait time.Duration
	for {
		select {
		case <-done:
			t.Logf("worst-case concurrent-writer wait for WLock during 32k-tile subscribe: %v",
				worstWait)
			return
		default:
		}
		start := time.Now()
		s.stateMu.Lock()
		wait := time.Since(start)
		s.stateMu.Unlock()
		if wait > worstWait {
			worstWait = wait
		}
	}
}
