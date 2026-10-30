package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWALFlushToDisk(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	dir := t.TempDir()
	s.dataDir = dir
	s.proximityRadius = -1

	cid := ChunkID{X: 0, Y: 0}
	// Do a reveal and a flag to generate WAL entries
	s.handleReveal(1, 1, cid, 0, false, false)
	s.handleReveal(1, 2, cid, 1, true, false) // may be wrong/correct depending on density

	if err := s.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	path := filepath.Join(dir, walFileName)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	types := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"type":"reveal"`) {
			types["reveal"] = true
		}
		if strings.Contains(line, `"type":"flag"`) {
			types["flag"] = true
		}
		if strings.Contains(line, `"type":"score_update"`) {
			types["score_update"] = true
		}
	}
	if len(types) == 0 {
		t.Fatalf("no WAL entries found")
	}
}

func TestReplayWALFromDiskWithTruncatedTail(t *testing.T) {
	src := NewServer()
	src.useS3 = false
	dir := t.TempDir()
	src.dataDir = dir
	src.proximityRadius = -1

	cid := ChunkID{X: 0, Y: 0}
	for cell := uint32(0); cell < 64; cell += 9 {
		src.handleReveal(1, uint64(cell)+1, cid, cell, false, false)
	}
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, walFileName), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if _, err := f.WriteString(`{"timestamp":"2026-01-01T00:00:00Z","type":"rev`); err != nil {
		t.Fatalf("append partial record: %v", err)
	}
	f.Close()

	dst := NewServer()
	dst.useS3 = false
	dst.dataDir = dir
	if err := dst.replayWAL(); err != nil {
		t.Fatalf("replayWAL: %v", err)
	}
	if dst.walSeq != src.walSeq {
		t.Fatalf("walSeq = %d, want %d", dst.walSeq, src.walSeq)
	}
	if src.chunks[cid] == nil || dst.chunks[cid] == nil || *dst.chunks[cid] != *src.chunks[cid] {
		t.Fatalf("replayed chunk bits differ from source")
	}
}
