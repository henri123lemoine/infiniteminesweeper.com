package main

// Territory ownership: who "owns" each 8×8 block of a chunk, for the future
// click-a-region-on-the-map feature. Block granularity keeps this at 512B per
// chunk (vs a 33× blowup for per-cell owners) and is indistinguishable from
// per-cell at map zoom. Recording started 2026-07; reveals before that have
// no owner, mirroring flag provenance.

const (
	territoryBlockSize   = 8 // cells per block side
	territoryBlocksSide  = ChunkSize / territoryBlockSize
	territoryBlocksTotal = territoryBlocksSide * territoryBlocksSide
)

// TerritoryBlock tracks the dominant revealer of one 8×8 block using the
// Boyer-Moore majority-vote scheme: same owner strengthens the claim, a
// different owner weakens it and takes over when it hits zero. Exact counts
// per player would cost unbounded memory; this converges to the majority
// revealer without any.
type TerritoryBlock struct {
	Owner uint32
	Votes uint32
}

type chunkTerritory [territoryBlocksTotal]TerritoryBlock

func territoryBlockIndex(cell uint32) int {
	x := int(cell) % ChunkSize / territoryBlockSize
	y := int(cell) / ChunkSize / territoryBlockSize
	return y*territoryBlocksSide + x
}

// recordTerritoryLocked votes playerID as the revealer of cell's block.
// Caller must hold stateMu (write). playerID 0 (replays of pre-provenance
// WAL entries) records nothing.
func (s *Server) recordTerritoryLocked(chunkID ChunkID, cell uint32, playerID uint32) {
	if playerID == 0 {
		return
	}
	ct := s.territory[chunkID]
	if ct == nil {
		ct = &chunkTerritory{}
		s.territory[chunkID] = ct
	}
	b := &ct[territoryBlockIndex(cell)]
	switch {
	case b.Votes == 0:
		b.Owner = playerID
		b.Votes = 1
	case b.Owner == playerID:
		b.Votes++
	default:
		b.Votes--
	}
}

// territoryOwner returns the dominant revealer of the block containing cell,
// or 0 if unrecorded. Caller must hold stateMu (read).
func (s *Server) territoryOwner(chunkID ChunkID, cell uint32) uint32 {
	ct := s.territory[chunkID]
	if ct == nil {
		return 0
	}
	b := ct[territoryBlockIndex(cell)]
	if b.Votes == 0 {
		return 0
	}
	return b.Owner
}
