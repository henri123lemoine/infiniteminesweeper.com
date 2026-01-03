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
