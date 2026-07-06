//go:build memprofile

package main

import (
	"runtime"
	"testing"
)

func heapMB() float64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func TestMemoryBreakdown(t *testing.T) {
	base := heapMB()
	s := NewServer()
	s.dataDir = "/Users/henrilemoine/minesweeper-world-backups/memprofile-data"
	if err := s.loadSnapshotFromDisk(); err != nil {
		t.Fatalf("load: %v", err)
	}
	afterLoad := heapMB()

	nFlags, nChunksWithFlags := 0, 0
	for _, fm := range s.flags {
		nFlags += len(fm)
		nChunksWithFlags++
	}

	// Cost attribution: rebuild each structure fresh and measure the delta.
	before := heapMB()
	flagsCopy := make(map[ChunkID]map[uint32]Flag, len(s.flags))
	for cid, fm := range s.flags {
		inner := make(map[uint32]Flag, len(fm))
		for c, f := range fm {
			inner[c] = f
		}
		flagsCopy[cid] = inner
	}
	flagsMB := heapMB() - before

	before = heapMB()
	chunksCopy := make(map[ChunkID]*ChunkBits, len(s.chunks))
	for cid, cb := range s.chunks {
		c := *cb
		chunksCopy[cid] = &c
	}
	chunksMB := heapMB() - before

	// Compact flag alternative: per-chunk sorted slices of 8-byte entries
	type flagEntry struct {
		Cell   uint16
		FlagID uint16
		Owner  uint32
	}
	before = heapMB()
	compact := make(map[ChunkID][]flagEntry, len(s.flags))
	for cid, fm := range s.flags {
		sl := make([]flagEntry, 0, len(fm))
		for c, f := range fm {
			sl = append(sl, flagEntry{Cell: uint16(c), FlagID: uint16(f.FlagID), Owner: f.Owner})
		}
		compact[cid] = sl
	}
	compactMB := heapMB() - before

	t.Logf("baseline heap:          %.1f MB", base)
	t.Logf("after full world load:  %.1f MB (delta %.1f)", afterLoad, afterLoad-base)
	t.Logf("flags: %d in %d chunks", nFlags, nChunksWithFlags)
	t.Logf("flags map copy:         %.1f MB  (%.1f B/flag)", flagsMB, flagsMB*1048576/float64(nFlags))
	t.Logf("chunks bitsets copy:    %.1f MB  (%d chunks)", chunksMB, len(s.chunks))
	t.Logf("compact flag slices:    %.1f MB  (%.1f B/flag) -> saves %.1f MB",
		compactMB, compactMB*1048576/float64(nFlags), flagsMB-compactMB)
	t.Logf("players=%d names=%d tokens=%d", len(s.scores), len(s.playerNames), len(s.sessionTokens))
	runtime.KeepAlive(flagsCopy)
	runtime.KeepAlive(chunksCopy)
	runtime.KeepAlive(compact)
}
