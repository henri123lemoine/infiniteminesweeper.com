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
