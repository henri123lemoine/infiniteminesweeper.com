package main

import (
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// TestMassSubscribeLockHoldTime measures how long a mass-subscribe holds the
// write lock. This is the main CPU cost introduced by the refactor: we now
// compute palette data from world state on subscribe (old code cached it but
// that's what was blowing up memory). Most chunks are all-unseen so they
// short-circuit; the cost is dominated by chunks with actual reveals.
func TestMassSubscribeLockHoldTime(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	pid := uint32(1)
	s.playerNames[pid] = "p"
	s.scores[pid] = 0
	s.playerFlags[pid] = 1
	s.minimapPlayerRes[pid] = 16

	// Populate some chunks with state so palette computation isn't trivial.
	const nStateful = 5000
	for i := 0; i < nStateful; i++ {
		cid := ChunkID{X: int64(i % 100), Y: int64(i / 100)}
		bits := &ChunkBits{}
		bits[0] = 0xFFFFFFFFFFFFFFFF // first row revealed
		bits[1] = 0xFFFFFFFFFFFFFFFF // second row revealed
		s.chunks[cid] = bits
	}
	s.totalRevealed = uint64(nStateful) * 128

	// Simulate a mass subscribe of 32k tiles (1080p max zoom).
	const nTiles = 32400
	tiles := make([]*pb.TileRef, 0, nTiles)
	for i := 0; i < nTiles; i++ {
		tiles = append(tiles, &pb.TileRef{X: int32(i % 200), Y: int32(i / 200)})
	}

	start := time.Now()
	s.stateMu.Lock()
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
		s.minimapSendFullTo(pid, cid)
	}
	s.stateMu.Unlock()
	elapsed := time.Since(start)
	t.Logf("mass-subscribe %d tiles (%d stateful): %v", nTiles, nStateful, elapsed)
}
