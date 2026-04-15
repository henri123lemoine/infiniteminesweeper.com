package main

import (
	"runtime"
	"testing"
	"time"
)

// TestSubscriptionOnlyMemory measures the "just subscribed, no tiles dirty
// yet" memory floor. This is the REAL cost of letting a player subscribe to
// many tiles at once — the tile objects themselves only appear when cells
// actually change.
func TestSubscriptionOnlyMemory(t *testing.T) {
	cases := []int{10_000, 50_000, 200_000}
	for _, n := range cases {
		t.Run("", func(t *testing.T) {
			s := NewServer()
			s.proximityRadius = -1
			const pid = uint32(1)
			s.playerNames[pid] = "p"
			s.minimapPlayerRes[pid] = 16

			before := readHeapAlloc()

			s.stateMu.Lock()
			for i := 0; i < n; i++ {
				cid := ChunkID{X: int64(i % 1000), Y: int64(i / 1000)}
				if s.minimapSubs[cid] == nil {
					s.minimapSubs[cid] = make(map[uint32]struct{})
				}
				if _, already := s.minimapSubs[cid][pid]; !already {
					s.minimapSubs[cid][pid] = struct{}{}
					s.minimapSubCount[pid]++
				}
				// NOTE: intentionally NOT calling minimapMarkDirty — this
				// measures pure subscription overhead with no active tiles.
			}
			s.stateMu.Unlock()

			after := readHeapAlloc()
			delta := int64(after) - int64(before)
			t.Logf("subs=%d tiles=%d sub-map heap_delta=%d bytes (%.1f bytes/sub, %.1f MB total)",
				n, len(s.minimapTiles), delta,
				float64(delta)/float64(n), float64(delta)/1024/1024)
			runtime.KeepAlive(s)
		})
	}
}

// TestBroadcastCPUCost measures the CPU cost of a broadcast pass covering
// many dirty tiles. This is the extra work the new "recompute palette on
// send" design pays each broadcast cycle, vs. the old cached-Data design.
func TestBroadcastCPUCost(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const pid = uint32(1)
	s.playerNames[pid] = "p"
	s.minimapPlayerRes[pid] = 64

	// Build a world with meaningful state across many chunks so the palette
	// computation is non-trivial.
	s.stateMu.Lock()
	const nChunks = 200
	for i := 0; i < nChunks; i++ {
		cid := ChunkID{X: int64(i % 20), Y: int64(i / 20)}
		bits := &ChunkBits{}
		// Partially reveal ~half the cells in this chunk.
		for w := 0; w < 32; w++ {
			bits[w] = 0xFFFFFFFFFFFFFFFF
		}
		s.chunks[cid] = bits

		// Subscribe and mark some cells dirty to simulate a broadcast backlog
		s.minimapSubs[cid] = map[uint32]struct{}{pid: {}}
		for k := uint32(0); k < 16; k++ {
			s.minimapMarkDirty(cid, k*113%4096)
		}
	}
	s.totalRevealed = uint64(nChunks * 32 * 64)
	s.stateMu.Unlock()

	// Run a broadcast pass and time it.
	const iterations = 5
	var total time.Duration
	for iter := 0; iter < iterations; iter++ {
		s.stateMu.Lock()
		// Reset dirty bits between iterations by re-marking
		for cid := range s.minimapSubs {
			for k := uint32(0); k < 16; k++ {
				s.minimapMarkDirty(cid, k*113%4096)
			}
		}
		start := time.Now()
		for cid := range s.minimapDirtyTiles {
			_ = s.computeFullTileData(cid)
		}
		elapsed := time.Since(start)
		s.stateMu.Unlock()
		total += elapsed
	}
	avg := total / iterations
	t.Logf("broadcast pass CPU (%d chunks with partial state): %v avg, %v per chunk",
		nChunks, avg, avg/time.Duration(nChunks))
}
