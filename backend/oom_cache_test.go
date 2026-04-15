package main

import (
	"runtime"
	"testing"
)

// TestSeedDensityCacheMemory measures the memory footprint of the full seed
// and density caches at their 200 k cap. These are constant overhead paid by
// the server regardless of player activity once enough chunks have been seen.
func TestSeedDensityCacheMemory(t *testing.T) {
	s := NewServer()

	before := readHeapAlloc()
	// Warm both caches to their cap.
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

// TestSeedCacheRandomEvictionKeepsWorkingSet checks that under a typical
// hot/cold access pattern, the cache retains hot keys even after being flooded
// with cold keys — the key property of random eviction vs the old nuke-on-full
// behavior. With the nuke-on-full approach, this test would frequently find
// zero hot keys resident (whenever the last nuke happened after the last hot
// access).
func TestSeedCacheRandomEvictionKeepsWorkingSet(t *testing.T) {
	s := NewServer()

	const hotCount = 50
	// Enough cold keys to overflow the cap several times over.
	const coldCount = seedCacheMaxEntries + hotCount*10

	hotIDs := make([]ChunkID, hotCount)
	for i := range hotIDs {
		// Use negative X so hot keys can't collide with cold keys below.
		hotIDs[i] = ChunkID{X: int64(-1 - i), Y: 0}
	}

	// Seed hot keys, then flood with cold keys — but re-touch hot keys
	// every 5 iterations so they can be re-inserted after any random
	// eviction.
	for i := 0; i < coldCount; i++ {
		if i%5 == 0 {
			// (i/5) cycles through every hotID, not just multiples of 5.
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
