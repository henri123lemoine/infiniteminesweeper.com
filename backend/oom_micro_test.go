package main

// Microbenchmarks and component-level measurements for the OOM fixes.
// Bigger end-to-end scenarios live in oom_loadtest_test.go (build-tagged
// integration). Cross-cutting regression invariants are in
// oom_invariants_test.go.

import (
	"runtime"
	"testing"
	"time"

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

// --- Per-tile minimap memory cost across resolutions. ------------------------

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
			side := 1
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
				// Marking dirty forces tile allocation — measures realistic
				// "active subscription" memory.
				s.minimapMarkDirty(cid, 0)
			}
			s.stateMu.Unlock()

			after := readHeapAlloc()
			delta := int64(after) - int64(before)
			t.Logf("res=%d tiles=%d heap_delta=%d bytes (%.1f KB/tile, %.1f MB total)",
				tc.resolution, tc.nTiles, delta,
				float64(delta)/float64(tc.nTiles)/1024,
				float64(delta)/1024/1024)
			runtime.KeepAlive(s)
		})
	}
}

// --- Subscription overhead (no tiles allocated). -----------------------------

func TestSubscriptionOnlyMemory(t *testing.T) {
	for _, n := range []int{10_000, 50_000, 200_000} {
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
				// Pure subscription overhead — no minimapMarkDirty here.
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

// --- WAL in-memory buffer footprint. -----------------------------------------

func TestWALReplayMemory(t *testing.T) {
	s := NewServer()

	const nEntries = 50000
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

// --- Seed/density caches at cap. ---------------------------------------------

func TestSeedDensityCacheMemory(t *testing.T) {
	s := NewServer()

	before := readHeapAlloc()
	for i := 0; i < seedCacheMaxEntries; i++ {
		cid := ChunkID{X: int64(i), Y: int64(i * 7)}
		_ = s.generateChunkSeed(cid)
		_ = s.getChunkDensity(cid)
	}
	after := readHeapAlloc()
	delta := int64(after) - int64(before)
	t.Logf("seed+density caches at cap (%d entries each): heap delta %.1f MB (%.1f bytes/entry combined)",
		seedCacheMaxEntries, float64(delta)/1024/1024,
		float64(delta)/float64(2*seedCacheMaxEntries))
	runtime.KeepAlive(s)
}

// TestSeedCacheRandomEvictionKeepsWorkingSet checks that under a hot/cold
// access pattern, the cache retains hot keys after being flooded with cold
// keys — the key property of random eviction vs the old nuke-on-full behavior.
func TestSeedCacheRandomEvictionKeepsWorkingSet(t *testing.T) {
	s := NewServer()
	const hotCount = 50
	const coldCount = seedCacheMaxEntries + hotCount*10

	hotIDs := make([]ChunkID, hotCount)
	for i := range hotIDs {
		hotIDs[i] = ChunkID{X: int64(-1 - i), Y: 0} // negative X avoids collision
	}

	for i := 0; i < coldCount; i++ {
		if i%5 == 0 {
			_ = s.generateChunkSeed(hotIDs[(i/5)%hotCount])
		}
		_ = s.generateChunkSeed(ChunkID{X: int64(i), Y: 42})
	}

	s.seedCacheMu.RLock()
	hits := 0
	for _, cid := range hotIDs {
		if _, ok := s.seedCache[cid]; ok {
			hits++
		}
	}
	s.seedCacheMu.RUnlock()

	if hits < hotCount/2 {
		t.Errorf("expected at least %d/%d hot keys resident post-eviction, got %d",
			hotCount/2, hotCount, hits)
	} else {
		t.Logf("hot keys resident after cold-key flood: %d/%d", hits, hotCount)
	}
}

// --- Broadcast CPU cost per dirty chunk. -------------------------------------

func TestBroadcastCPUCost(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const pid = uint32(1)
	s.playerNames[pid] = "p"
	s.minimapPlayerRes[pid] = 64

	s.stateMu.Lock()
	const nChunks = 200
	for i := 0; i < nChunks; i++ {
		cid := ChunkID{X: int64(i % 20), Y: int64(i / 20)}
		bits := &ChunkBits{}
		for w := 0; w < 32; w++ {
			bits[w] = 0xFFFFFFFFFFFFFFFF
		}
		s.chunks[cid] = bits
		s.minimapSubs[cid] = map[uint32]struct{}{pid: {}}
		for k := uint32(0); k < 16; k++ {
			s.minimapMarkDirty(cid, k*113%4096)
		}
	}
	s.totalRevealed = uint64(nChunks * 32 * 64)
	s.stateMu.Unlock()

	const iterations = 5
	var total time.Duration
	for range iterations {
		s.stateMu.Lock()
		for cid := range s.minimapSubs {
			for k := uint32(0); k < 16; k++ {
				s.minimapMarkDirty(cid, k*113%4096)
			}
		}
		start := time.Now()
		for cid := range s.minimapDirtyTiles {
			_ = s.computeFullTileData(cid)
		}
		total += time.Since(start)
		s.stateMu.Unlock()
	}
	avg := total / iterations
	t.Logf("broadcast pass CPU (%d chunks with partial state): %v avg, %v per chunk",
		nChunks, avg, avg/time.Duration(nChunks))
}

// --- Mass-subscribe latency. -------------------------------------------------

func TestMassSubscribeLockHoldTime(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const pid = uint32(1)

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
	t.Logf("batched mass-subscribe %d tiles (%d stateful): %v total wall-clock",
		nTiles, nStateful, time.Since(start))
}

// TestMassSubscribeWriteLockBursts measures the worst-case continuous write-
// lock hold during a mass subscribe — the longest gap during which a
// concurrent reveal would block. Should be <10 ms even for 32 k tiles.
func TestMassSubscribeWriteLockBursts(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const pid = uint32(1)

	const nTiles = 32400
	tiles := make([]*pb.TileRef, 0, nTiles)
	for i := 0; i < nTiles; i++ {
		tiles = append(tiles, &pb.TileRef{X: int32(i % 200), Y: int32(i / 200)})
	}

	done := make(chan struct{})
	go func() {
		s.handleMinimapSubscribe(pid, &pb.SubscribeTiles{Tiles: tiles, Resolution: 64})
		close(done)
	}()

	var worstWait time.Duration
	for {
		select {
		case <-done:
			t.Logf("worst-case concurrent-writer wait during 32k-tile subscribe: %v", worstWait)
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
