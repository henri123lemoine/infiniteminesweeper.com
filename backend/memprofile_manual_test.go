//go:build memprofile

package main

import (
	"bytes"
	"encoding/gob"
	"runtime"
	"testing"
	"time"

	"github.com/klauspost/compress/gzip"
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
	flagsCopy := make(map[ChunkID]chunkFlags, len(s.flags))
	for cid, fm := range s.flags {
		flagsCopy[cid] = append(chunkFlags(nil), fm...)
	}
	flagsMB := heapMB() - before

	before = heapMB()
	chunksCopy := make(map[ChunkID]*ChunkBits, len(s.chunks))
	for cid, cb := range s.chunks {
		c := *cb
		chunksCopy[cid] = &c
	}
	chunksMB := heapMB() - before

	t.Logf("baseline heap:          %.1f MB", base)
	t.Logf("after full world load:  %.1f MB (delta %.1f)", afterLoad, afterLoad-base)
	t.Logf("flags: %d in %d chunks", nFlags, nChunksWithFlags)
	t.Logf("flag store copy:        %.1f MB  (%.1f B/flag)", flagsMB, flagsMB*1048576/float64(nFlags))
	t.Logf("chunks bitsets copy:    %.1f MB  (%d chunks)", chunksMB, len(s.chunks))
	t.Logf("players=%d names=%d tokens=%d", len(s.scores), len(s.playerNames), len(s.sessionTokens))
	runtime.KeepAlive(flagsCopy)
	runtime.KeepAlive(chunksCopy)
}

func TestSnapshotEncodeCost(t *testing.T) {
	s := NewServer()
	s.dataDir = "/Users/henrilemoine/minesweeper-world-backups/memprofile-data"
	if err := s.loadSnapshotFromDisk(); err != nil {
		t.Fatalf("load: %v", err)
	}

	t0 := time.Now()
	data := s.captureSnapshotData()
	tCopy := time.Since(t0)

	t0 = time.Now()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gz).Encode(data); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	tGob := time.Since(t0)

	t0 = time.Now()
	var bbuf bytes.Buffer
	if err := encodeSnapshot(&bbuf, &data); err != nil {
		t.Fatal(err)
	}
	tBinary := time.Since(t0)
	binarySize := bbuf.Len()

	t0 = time.Now()
	if _, err := decodeSnapshot(&bbuf); err != nil {
		t.Fatal(err)
	}
	tDecode := time.Since(t0)

	t.Logf("deep copy: %v", tCopy)
	t.Logf("gob+gzip encode:   %v -> %.1f MB", tGob, float64(buf.Len())/(1<<20))
	t.Logf("binary+lz4 encode: %v -> %.1f MB (decode %v)", tBinary, float64(binarySize)/(1<<20), tDecode)
}
