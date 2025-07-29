package main

import (
	"math"

	pb "infinite-minesweeper/proto"
)

const (
	DefaultMineCount = 20 // baseline mine density (20%)
	RegionSize       = 16 // chunks per noise region
)

// noise generates deterministic noise in [0,1)
func noise(x, y int32) float64 {
	state := uint64(uint32(x))<<32 ^ uint64(uint32(y))
	v := splitmix64(state)
	return float64(v>>1) / float64(1<<63)
}

// mineCountForChunk computes bomb density for a given chunk
func (s *Server) mineCountForChunk(id ChunkID) int {
	if id.X == 0 && id.Y == 0 {
		return DefaultMineCount
	}
	rx := id.X / RegionSize
	ry := id.Y / RegionSize
	fx := float64((id.X%RegionSize+RegionSize)%RegionSize) / float64(RegionSize)
	fy := float64((id.Y%RegionSize+RegionSize)%RegionSize) / float64(RegionSize)

	n00 := noise(rx, ry)
	n10 := noise(rx+1, ry)
	n01 := noise(rx, ry+1)
	n11 := noise(rx+1, ry+1)

	nx0 := n00*(1-fx) + n10*fx
	nx1 := n01*(1-fx) + n11*fx
	n := nx0*(1-fy) + nx1*fy

	return 15 + int(math.Round(n*20))
}

// playerDensityMultiplier returns a score multiplier based on nearby players
func (s *Server) playerDensityMultiplier(id ChunkID) float64 {
	count := 0
	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			count += len(s.subs[ChunkID{id.X + dx, id.Y + dy}])
		}
	}
	if count <= 1 {
		return 1.0
	}
	return 1.0 + 0.1*float64(count-1)
}

// isMine determines if a cell contains a mine using chunk-specific density
func (s *Server) isMine(id ChunkID, seed uint64, x, y int) bool {
	cellSeed := splitmix64(seed + uint64(y*ChunkSize+x))
	return int(cellSeed%100) < s.mineCountForChunk(id)
}

// sendScoreUpdate notifies a player of their current score
func (s *Server) sendScoreUpdate(playerID int32, score int32) {
	msg := &pb.Msg{Payload: &pb.Msg_ScoreUpdate{ScoreUpdate: &pb.ScoreUpdate{Score: score}}}
	s.sendToPlayer(playerID, mustProto(msg))
}
