package main

// Tests with concrete invariants live in oom_invariants_test.go; integration
// load scenarios in oom_loadtest_test.go. This file holds only assertions
// that don't fit either bucket plus the shared readHeapAlloc helper.

import (
	"runtime"
	"testing"
)

func readHeapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestSeedCacheRandomEvictionKeepsWorkingSet checks that under a hot/cold
// access pattern the cache retains hot keys after being flooded with cold
// keys — the property of random eviction vs the old nuke-on-full behavior.
func TestSeedCacheRandomEvictionKeepsWorkingSet(t *testing.T) {
	s := NewServer()
	const hotCount = 50
	const coldCount = seedCacheMaxEntries + hotCount*10

	hotIDs := make([]ChunkID, hotCount)
	for i := range hotIDs {
		hotIDs[i] = ChunkID{X: int64(-1 - i)} // negative X avoids collision
	}
	for i := 0; i < coldCount; i++ {
		if i%5 == 0 {
			_ = s.generateChunkSeed(hotIDs[(i/5)%hotCount])
		}
		_ = s.generateChunkSeed(ChunkID{X: int64(i), Y: 42})
	}

	s.seedCacheMu.RLock()
	defer s.seedCacheMu.RUnlock()
	hits := 0
	for _, cid := range hotIDs {
		if _, ok := s.seedCache[cid]; ok {
			hits++
		}
	}
	if hits < hotCount/2 {
		t.Errorf("expected at least %d/%d hot keys resident post-eviction, got %d",
			hotCount/2, hotCount, hits)
	}
}
