package main

import (
	"bytes"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// MinimapTile represents a 64x64 palette-indexed tile with a monotonic version.
type MinimapTile struct {
	Version uint32
	Data    [ChunkSize * ChunkSize]byte
	Dirty   [ChunkSize * ChunkSize]bool
}

// palette indices (<=10 colors total)
const (
	mmUnseen   = 0 // background
	mmMine     = 1 // revealed mine
	mmEmpty0   = 2 // revealed no-adjacent
	mmNum1     = 3 // numbers map 1..6 → 3..8
	mmNum2     = 4
	mmNum3     = 5
	mmNum4     = 6
	mmNum5     = 7
	mmNum6     = 8
	mmNum7Plus = 9 // 7 or 8

	mmFlagBase = 10 // 10 buckets for flags: 10..19
)

func minimapFlagBucket(flagID uint32) int {
	// Map sprite IDs to one of 10 color buckets.
	// Note: newer flag ID series are offset by +1 compared to the original 0..9 mapping.
	// Normalize so buckets align to: 0:light_gray, 1:red, 2:green, 3:blue, 4:yellow,
	// 5:orange, 6:purple, 7:cyan, 8:pink, 9:dark_gray.
	b := int(flagID % 10)
	if flagID >= 62 { // IDs from later series are shifted by +1 relative to early sets
		return (b + 9) % 10 // equivalent to (b - 1 + 10) % 10
	}
	return b
}

// mark a cell dirty in the minimap and set its palette index
func (s *Server) minimapSetCell(cid ChunkID, cell uint32, idx byte) {
	t := s.minimapTiles[cid]
	if t == nil {
		t = &MinimapTile{}
		s.minimapTiles[cid] = t
	}
	pos := int(cell)
	if pos < 0 || pos >= ChunkSize*ChunkSize {
		return
	}
	if t.Data[pos] != idx {
		t.Data[pos] = idx
		t.Dirty[pos] = true
		s.minimapDirtyTiles[cid] = struct{}{}
	}
}

// compute palette index for a cell given current authoritative state
func (s *Server) minimapPaletteFor(cid ChunkID, cell uint32) byte {
	if s.isCellFlagged(cid, cell) {
		// Use the stored flag's ID to choose one of 10 palette buckets
		if fl, ok := s.flags[cid][cell]; ok {
			b := minimapFlagBucket(fl.FlagID)
			return byte(mmFlagBase + b)
		}
		return byte(mmFlagBase)
	}
	if s.isCellRevealed(cid, cell) {
		if s.isMine(cid, cell) {
			return mmMine
		}
		n := s.countAdjacentMines(cid, cell)
		switch n {
		case 0:
			return mmEmpty0
		case 1:
			return mmNum1
		case 2:
			return mmNum2
		case 3:
			return mmNum3
		case 4:
			return mmNum4
		case 5:
			return mmNum5
		case 6:
			return mmNum6
		default:
			return mmNum7Plus
		}
	}
	return mmUnseen
}

// rebuild a full tile from current world state
func (s *Server) minimapRebuildTile(cid ChunkID) *MinimapTile {
	t := s.minimapTiles[cid]
	if t == nil {
		t = &MinimapTile{}
		s.minimapTiles[cid] = t
	}
	for i := 0; i < ChunkSize*ChunkSize; i++ {
		idx := s.minimapPaletteFor(cid, uint32(i))
		t.Data[i] = idx
		t.Dirty[i] = false
	}
	return t
}

// called from reveal/flag logic under stateMu write-lock
func (s *Server) minimapOnReveal(cid ChunkID, cell uint32) {
	s.minimapSetCell(cid, cell, s.minimapPaletteFor(cid, cell))
}

func (s *Server) minimapOnFlag(cid ChunkID, cell uint32) {
	s.minimapSetCell(cid, cell, s.minimapPaletteFor(cid, cell))
}

// gather dirty rectangles for a tile; also computes total bytes of deltas
func minimapCollectRects(t *MinimapTile) (rects []struct {
	x, y, w, h int
	rows       []byte
}, deltaBytes int) {
	// First, find horizontal runs per row
	type run struct{ x0, x1 int }
	runs := make([][]run, ChunkSize)
	for y := 0; y < ChunkSize; y++ {
		row := t.Dirty[y*ChunkSize : (y+1)*ChunkSize]
		x := 0
		for x < ChunkSize {
			// skip clean
			for x < ChunkSize && !row[x] {
				x++
			}
			if x >= ChunkSize {
				break
			}
			start := x
			for x < ChunkSize && row[x] {
				x++
			}
			runs[y] = append(runs[y], run{start, x - 1})
		}
	}
	// Merge identical runs vertically into rectangles
	used := make([][]bool, ChunkSize)
	for y := 0; y < ChunkSize; y++ {
		used[y] = make([]bool, len(runs[y]))
	}
	for y := 0; y < ChunkSize; y++ {
		for i, r := range runs[y] {
			if used[y][i] {
				continue
			}
			h := 1
			// try to extend downward while there exists an identical run
			for yy := y + 1; yy < ChunkSize; yy++ {
				// find a matching run in runs[yy]
				foundJ := -1
				for j, rr := range runs[yy] {
					if !used[yy][j] && rr.x0 == r.x0 && rr.x1 == r.x1 {
						foundJ = j
						break
					}
				}
				if foundJ == -1 {
					break
				}
				used[yy][foundJ] = true
				h++
			}
			// Build rows payload
			w := r.x1 - r.x0 + 1
			var buf bytes.Buffer
			for yy := y; yy < y+h; yy++ {
				start := yy*ChunkSize + r.x0
				buf.Write(t.Data[start : start+w])
			}
			rects = append(rects, struct {
				x, y, w, h int
				rows       []byte
			}{x: r.x0, y: y, w: w, h: h, rows: buf.Bytes()})
			deltaBytes += w * h
		}
	}
	return
}

// send a FullTile for cid to a specific player (stateMu may be held by caller)
func (s *Server) minimapSendFullTo(playerID uint32, cid ChunkID) {
	// Rebuild from authoritative state to avoid stale or missing tiles
	t := s.minimapRebuildTile(cid)
	isAllUnseen := true
	for i := 0; i < ChunkSize*ChunkSize; i++ {
		if t.Data[i] != mmUnseen {
			isAllUnseen = false
			break
		}
	}
	if isAllUnseen {
		return
	}
	// prepare bytes
	data := make([]byte, len(t.Data))
	copy(data, t.Data[:])
	msg := &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
		Tile:    &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
		Version: t.Version,
		Data:    data,
	}}}
	s.sendToPlayer(playerID, mustProto(msg))
}

// broadcast minimap changes periodically; should be run in a goroutine
func (s *Server) runMinimapBroadcaster() {
	// simple ticker-less loop driven by small sleeps to keep latency low
	for {
		// snapshot dirty set
		s.stateMu.Lock()
		if len(s.minimapDirtyTiles) == 0 {
			s.stateMu.Unlock()
			// sleep a bit to avoid busy loop
			time.Sleep(20 * time.Millisecond)
			continue
		}
		dirty := make([]ChunkID, 0, len(s.minimapDirtyTiles))
		for cid := range s.minimapDirtyTiles {
			dirty = append(dirty, cid)
		}
		// reset set
		s.minimapDirtyTiles = make(map[ChunkID]struct{})

		// For each tile, compute delta rectangles and send to subscribers
		for _, cid := range dirty {
			t := s.minimapTiles[cid]
			if t == nil {
				continue
			}
			rects, deltaBytes := minimapCollectRects(t)
			// Heuristic: if over half of tile, send full
			if deltaBytes > (ChunkSize*ChunkSize)/2 {
				// clear dirties, bump version, send full
				for i := range t.Dirty {
					t.Dirty[i] = false
				}
				t.Version++
				data := make([]byte, len(t.Data))
				copy(data, t.Data[:])
				msg := &pb.Msg{Payload: &pb.Msg_MinimapFullTile{MinimapFullTile: &pb.FullTile{
					Tile:    &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
					Version: t.Version,
					Data:    data,
				}}}
				subs := s.minimapSubs[cid]
				s.stateMu.Unlock()
				for pid := range subs {
					s.sendToPlayer(pid, mustProto(msg))
				}
				s.stateMu.Lock()
				continue
			}
			if len(rects) == 0 {
				continue
			}
			// build TileDelta
			pRects := make([]*pb.DeltaRect, 0, len(rects))
			for _, r := range rects {
				// clear dirty marks for cells in rect
				for yy := r.y; yy < r.y+r.h; yy++ {
					for xx := r.x; xx < r.x+r.w; xx++ {
						t.Dirty[yy*ChunkSize+xx] = false
					}
				}
				pRects = append(pRects, &pb.DeltaRect{
					X:    uint32(r.x),
					Y:    uint32(r.y),
					W:    uint32(r.w),
					H:    uint32(r.h),
					Rows: append([]byte(nil), r.rows...),
				})
			}
			t.Version++
			msg := &pb.Msg{Payload: &pb.Msg_MinimapTileDelta{MinimapTileDelta: &pb.TileDelta{
				Tile:    &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
				Version: t.Version,
				Rects:   pRects,
			}}}
			subs := s.minimapSubs[cid]
			s.stateMu.Unlock()
			for pid := range subs {
				s.sendToPlayer(pid, mustProto(msg))
			}
			s.stateMu.Lock()
		}
		s.stateMu.Unlock()
	}
}
