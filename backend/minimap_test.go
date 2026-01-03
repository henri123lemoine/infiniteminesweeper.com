package main

import "testing"

func TestMinimapSetAndCollectRects(t *testing.T) {
    s := NewServer()
    cid := ChunkID{X: 0, Y: 0}

    s.stateMu.Lock()
    s.minimapSubs[cid] = map[uint32]struct{}{1: {}}
    // Mark three adjacent cells dirty in first row and two in second row aligned, expecting a 2x3 rect
    s.minimapSetCell(cid, 0, 5)
    s.minimapSetCell(cid, 1, 5)
    s.minimapSetCell(cid, 2, 5)
    s.minimapSetCell(cid, uint32(ChunkSize+0), 6)
    s.minimapSetCell(cid, uint32(ChunkSize+1), 6)
    // Use the full-resolution (64x64) tile created by minimapSetCell
    t0 := s.minimapTiles[cid][64]
    s.stateMu.Unlock()

    rects, bytes := minimapCollectRects(t0)
    if bytes == 0 || len(rects) == 0 {
        t.Fatalf("expected non-empty rects")
    }
}
