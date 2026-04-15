package main

import "testing"

func TestMinimapSetAndCollectRects(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: 0, Y: 0}

	s.stateMu.Lock()
	s.minimapSubs[cid] = map[uint32]struct{}{1: {}}
	// Mark three adjacent cells dirty in first row and two in second row aligned.
	s.minimapMarkDirty(cid, 0)
	s.minimapMarkDirty(cid, 1)
	s.minimapMarkDirty(cid, 2)
	s.minimapMarkDirty(cid, uint32(ChunkSize+0))
	s.minimapMarkDirty(cid, uint32(ChunkSize+1))
	tile := s.minimapTiles[cid]
	s.stateMu.Unlock()

	if tile == nil {
		t.Fatalf("expected minimap tile to exist after mark dirty")
	}
	if !tile.hasAnyDirty() {
		t.Fatalf("expected tile to have dirty bits set")
	}
	dirty := tile.deriveDirtyAtResolution(64)
	data := make([]byte, 64*64) // all zeros is fine for rect collection test
	rects, bytesDelta := collectRectsFromDirty(dirty, data, 64)
	if bytesDelta == 0 || len(rects) == 0 {
		t.Fatalf("expected non-empty rects")
	}
}
