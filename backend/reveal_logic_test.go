package main

import (
    "testing"

    pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// findOneMineAndSafe scans a chunk to find at least one mined and one safe cell
func findOneMineAndSafe(s *Server, cid ChunkID) (mine uint32, safe uint32, ok bool) {
    // ensure density cache is populated deterministically
    _ = s.getChunkDensity(cid)
    foundMine, foundSafe := false, false
    for i := uint32(0); i < 4096 && !(foundMine && foundSafe); i++ {
        if s.isMine(cid, i) {
            if !foundMine { mine = i; foundMine = true }
        } else {
            if !foundSafe { safe = i; foundSafe = true }
        }
    }
    return mine, safe, foundMine && foundSafe
}

func TestRightClickFlaggingAndPenalty(t *testing.T) {
    s := NewServer()
    s.proximityRadius = -1 // disable proximity rule for unit test
    cid := ChunkID{X: 0, Y: 0}

    mine, safe, ok := findOneMineAndSafe(s, cid)
    if !ok { t.Fatalf("failed to find mine and safe cell") }

    // Correct flag on a mine
    before := s.scores[1]
    s.handleReveal(1, 1, cid, mine, true, false)
    after := s.scores[1]
    if after <= before { t.Fatalf("expected positive score change on correct flag: %d -> %d", before, after) }
    if !s.isCellFlagged(cid, mine) { t.Fatalf("mine not flagged") }

    // Wrong flag on a safe cell yields -20 and reveals (flood-fill)
    before2 := s.scores[1]
    s.handleReveal(1, 2, cid, safe, true, false)
    after2 := s.scores[1]
    if after2 >= before2 { t.Fatalf("expected penalty on wrong flag: %d -> %d", before2, after2) }
    if !s.isCellRevealed(cid, safe) { t.Fatalf("safe cell not revealed after wrong-flag flow") }
}

func TestChordValidationRevealsNeighbors(t *testing.T) {
    s := NewServer()
    s.proximityRadius = -1
    cid := ChunkID{X: 1, Y: 0}

    // Find a non-mine numbered cell
    var center uint32 = 0
    found := false
    for i := uint32(0); i < 4096; i++ {
        if !s.isMine(cid, i) {
            n := s.countAdjacentMines(cid, i)
            if n > 0 { center = i; found = true; break }
        }
    }
    if !found { t.Skip("no suitable numbered cell found in chunk") }

    // Reveal the center first (as chord requires revealed center)
    s.handleReveal(1, 10, cid, center, false, false)
    if !s.isCellRevealed(cid, center) { t.Fatalf("center not revealed") }

    // Prepare neighbors: flag mined neighbors; leave safe neighbors unflagged
    cx, cy := cellIndexToXY(center)
    worldX := int(cid.X)*ChunkSize + cx
    worldY := int(cid.Y)*ChunkSize + cy
    for dy := -1; dy <= 1; dy++ {
        for dx := -1; dx <= 1; dx++ {
            if dx == 0 && dy == 0 { continue }
            wx, wy := worldX+dx, worldY+dy
            ncid, ncell := worldToChunk(wx, wy)
            if s.isMine(ncid, ncell) {
                // place a flag directly into state
                s.stateMu.Lock()
                coll := make(map[ChunkID][]*pb.FlagPlacement)
                s.setCellFlagged(ncid, ncell, 1, 1, &coll)
                s.stateMu.Unlock()
            }
        }
    }

    // Now chord on center should be valid and reveal any unflagged safe neighbors
    s.handleReveal(1, 11, cid, center, false, true)

    // Verify at least one adjacent safe neighbor is revealed
    revealedAny := false
    for dy := -1; dy <= 1; dy++ {
        for dx := -1; dx <= 1; dx++ {
            if dx == 0 && dy == 0 { continue }
            wx, wy := worldX+dx, worldY+dy
            ncid, ncell := worldToChunk(wx, wy)
            if !s.isMine(ncid, ncell) && s.isCellRevealed(ncid, ncell) {
                revealedAny = true
            }
        }
    }
    if !revealedAny { t.Fatalf("chord did not reveal any safe neighbor") }
}
