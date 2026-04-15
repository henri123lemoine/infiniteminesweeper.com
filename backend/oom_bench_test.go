package main

import (
	"runtime"
	"testing"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// readHeapAlloc returns HeapAlloc in bytes (live + reachable allocated memory).
func readHeapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestMinimapMemoryPerTile measures bytes held per subscribed minimap tile for
// realistic subscription patterns (res=16, res=32, res=64).
func TestMinimapMemoryPerTile(t *testing.T) {
	cases := []struct {
		name       string
		resolution uint32
		nTiles     int
	}{
		{"res64-1k", 64, 1000},
		{"res32-5k", 32, 5000},
		{"res16-30k", 16, 30000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			s.proximityRadius = -1

			const fakePlayerID = uint32(1)
			s.playerNames[fakePlayerID] = "p"
			s.minimapPlayerRes[fakePlayerID] = tc.resolution

			before := readHeapAlloc()

			tiles := make([]*pb.TileRef, 0, tc.nTiles)
			side := int(1)
			for side*side < tc.nTiles {
				side++
			}
			for y := 0; y < side && len(tiles) < tc.nTiles; y++ {
				for x := 0; x < side && len(tiles) < tc.nTiles; x++ {
					tiles = append(tiles, &pb.TileRef{X: int32(x), Y: int32(y)})
				}
			}

			s.stateMu.Lock()
			for _, tr := range tiles {
				cid := ChunkID{X: int64(tr.X), Y: int64(tr.Y)}
				if s.minimapSubs[cid] == nil {
					s.minimapSubs[cid] = make(map[uint32]struct{})
				}
				s.minimapSubs[cid][fakePlayerID] = struct{}{}
				// Simulate a live tile by marking one cell dirty — this forces
				// creation of the MinimapTile (the struct a subscribed chunk
				// holds).
				s.minimapMarkDirty(cid, 0)
			}
			s.stateMu.Unlock()

			after := readHeapAlloc()
			delta := int64(after) - int64(before)
			t.Logf("res=%d tiles=%d heap_delta=%d bytes (%.1f KB/tile, %.1f MB total)",
				tc.resolution, tc.nTiles, delta,
				float64(delta)/float64(tc.nTiles)/1024,
				float64(delta)/1024/1024)
			// Keep the server alive so allocations aren't collected before measurement.
			runtime.KeepAlive(s)
		})
	}
}

// TestWALReplayMemory measures memory pressure of the WAL flush path when the
// WAL grows between snapshots. It does NOT exercise S3 directly but shows the
// shape of the in-memory WAL slice.
func TestWALReplayMemory(t *testing.T) {
	s := NewServer()

	nEntries := 50000
	before := readHeapAlloc()
	for i := range nEntries {
		s.writeWALEntry("reveal", struct {
			ChunkID ChunkID  `json:"chunk_id"`
			Cells   []uint32 `json:"cells"`
		}{
			ChunkID: ChunkID{X: int64(i % 100), Y: int64(i / 100)},
			Cells:   []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		})
	}
	after := readHeapAlloc()
	delta := int64(after) - int64(before)
	t.Logf("WAL entries=%d heap_delta=%d bytes (%.1f bytes/entry, %.1f MB total)",
		nEntries, delta, float64(delta)/float64(nEntries), float64(delta)/1024/1024)
	runtime.KeepAlive(s)
}

// TestTilesNoResolutionDuplication verifies the post-fix invariant: tiles no
// longer allocate per-resolution Data arrays; only a small dirty bitset per
// chunk is retained.
func TestTilesNoResolutionDuplication(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1

	cid := ChunkID{X: 0, Y: 0}
	s.minimapSubs[cid] = map[uint32]struct{}{1: {}}
	s.minimapPlayerRes[1] = 16
	s.minimapMarkDirty(cid, 0)

	tile := s.minimapTiles[cid]
	if tile == nil {
		t.Fatalf("expected a tile to exist after marking dirty")
	}
	// The per-tile dirty bitset is exactly 64 uint64 words = 512 bytes;
	// everything else (e.g. 4 KB Data) is gone.
	if sz := len(tile.Dirty); sz != 64 {
		t.Errorf("expected 64-word dirty bitset, got %d", sz)
	}
}
