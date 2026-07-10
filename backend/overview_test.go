package main

import (
	"bytes"
	"testing"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func overviewTestChunk(seed uint32) *ChunkBits {
	bits := &ChunkBits{}
	for i := seed % 11; i < 4096; i += 17 + seed%7 {
		bits[i>>6] |= 1 << (i & 63)
	}
	return bits
}

func TestOverviewPyramidAveragesColorWithoutSamplingPatterns(t *testing.T) {
	full := make([]byte, ChunkSize*ChunkSize)
	for y := 0; y < ChunkSize; y++ {
		for x := 0; x < ChunkSize; x++ {
			full[y*ChunkSize+x] = byte((x + y) & 1)
		}
	}
	tile := computeOverviewTileFromFull(full)
	for _, pixels := range [][]byte{tile.LOD1[:], tile.LOD2[:], tile.LOD4[:], tile.LOD8[:], tile.LOD16[:], tile.LOD32[:]} {
		for _, index := range pixels {
			if index != pixels[0] {
				t.Fatal("equal color mixtures produced a positional pattern")
			}
		}
	}
	index := tile.LOD8[0]
	if index < overviewColorBase {
		t.Fatalf("mixed block retained categorical palette index %d", index)
	}
	color := overviewPaletteRGB[index]
	if color[0] < 150 || color[1] < 150 || color[2] < 150 {
		t.Fatalf("linear-light average is unexpectedly dark: %v", color)
	}
}

func TestOverviewGlobalImagesMatchTilePyramid(t *testing.T) {
	s := NewServer()
	chunks := []ChunkID{{X: -2, Y: 3}, {X: 4, Y: -1}}
	for i, cid := range chunks {
		s.chunks[cid] = overviewTestChunk(uint32(i + 1))
	}
	s.rebuildOverviewLocked()

	for _, lod := range overviewGlobalLODs {
		image := s.overviewImages[lod]
		if image == nil {
			t.Fatalf("missing global LOD %d", lod)
		}
		if len(image.Pixels) > maxOverviewRegionPixels {
			t.Fatalf("LOD %d exceeds global image cap", lod)
		}
		for _, cid := range chunks {
			x := int(cid.X - image.OriginX)
			y := int(cid.Y - image.OriginY)
			width := image.WidthChunks * int(lod)
			got := make([]byte, lod*lod)
			for row := 0; row < int(lod); row++ {
				start := (y*int(lod)+row)*width + x*int(lod)
				copy(got[row*int(lod):(row+1)*int(lod)], image.Pixels[start:start+int(lod)])
			}
			if !bytes.Equal(got, s.overviewTiles[cid].pixels(lod)) {
				t.Fatalf("global LOD %d differs at chunk %v", lod, cid)
			}
		}
	}
}

func TestOverviewDirtyRefreshUpdatesEveryLOD(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: 1, Y: 2}
	s.chunks[cid] = overviewTestChunk(2)
	s.rebuildOverviewLocked()
	if len(s.overviewImages[8].Encoded) == 0 {
		t.Fatal("global LOD 8 snapshot was not pre-encoded")
	}
	previousRevision := s.overviewRevision
	s.chunks[cid][63] ^= 1 << 5
	s.overviewDirty[cid] = struct{}{}
	dirty, revision := s.refreshOverviewDirtyLocked()
	if len(dirty) != 1 || dirty[0] != cid || revision <= previousRevision {
		t.Fatalf("unexpected dirty refresh: dirty=%v revision=%d", dirty, revision)
	}
	for _, lod := range overviewGlobalLODs {
		image := s.overviewImages[lod]
		if image.Revision != revision {
			t.Fatalf("LOD %d image revision is %d, want %d", lod, image.Revision, revision)
		}
	}
	if s.overviewImages[8].Encoded != nil {
		t.Fatal("dirty global image retained stale encoded snapshot")
	}
}

func TestOverviewRequestUsesCoherentCachedSnapshot(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: 0, Y: 0}
	s.chunks[cid] = overviewTestChunk(3)
	s.rebuildOverviewLocked()
	player := &Player{ID: 1, Send: make(chan []byte, 2), done: make(chan struct{})}
	s.players[1] = map[*Player]struct{}{player: {}}

	s.handleOverviewRequest(player, &pb.OverviewRequest{Lod: 8, Global: true, Subscribe: true})
	msg := decodePBMsg(t, <-player.Send)
	snapshot := msg.GetOverviewSnapshot()
	if snapshot == nil || snapshot.Unchanged || len(snapshot.Pixels) == 0 {
		t.Fatalf("unexpected overview snapshot: %#v", snapshot)
	}
	if got, want := len(snapshot.Pixels), snapshot.GetWidthChunks()*snapshot.GetHeightChunks()*64; uint32(got) != want {
		t.Fatalf("snapshot pixels=%d, want=%d", got, want)
	}
	if sub := s.overviewSubs[player][8]; !sub.Ready {
		t.Fatal("overview subscription did not become ready after snapshot enqueue")
	}

	s.handleOverviewRequest(player, &pb.OverviewRequest{
		Lod: 8, Global: true, KnownRevision: snapshot.Revision, Subscribe: true,
	})
	unchanged := decodePBMsg(t, <-player.Send).GetOverviewSnapshot()
	if unchanged == nil || !unchanged.Unchanged || len(unchanged.Pixels) != 0 {
		t.Fatalf("unexpected unchanged response: %#v", unchanged)
	}
}

func TestOverviewPrefetchDoesNotKeepSubscription(t *testing.T) {
	s := NewServer()
	s.chunks[ChunkID{}] = overviewTestChunk(1)
	s.rebuildOverviewLocked()
	player := &Player{ID: 1, Send: make(chan []byte, 1), done: make(chan struct{})}
	s.handleOverviewRequest(player, &pb.OverviewRequest{Lod: 8, Global: true})
	if snapshot := decodePBMsg(t, <-player.Send).GetOverviewSnapshot(); snapshot == nil || len(snapshot.Pixels) == 0 {
		t.Fatalf("unexpected prefetch snapshot: %#v", snapshot)
	}
	if _, exists := s.overviewSubs[player]; exists {
		t.Fatal("one-shot prefetch retained a live subscription")
	}
}

func TestOverviewPatchUsesRequestedLOD(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: 2, Y: 5}
	s.chunks[cid] = overviewTestChunk(6)
	s.rebuildOverviewLocked()
	s.chunks[cid][10] ^= 1 << 3
	s.overviewDirty[cid] = struct{}{}
	dirty, revision := s.refreshOverviewDirtyLocked()

	for _, lod := range []uint32{8, 16, 32, 64} {
		msg := s.overviewPatchLocked(lod, overviewSubscription{Global: true}, dirty, revision)
		if msg == nil {
			t.Fatalf("missing LOD %d patch", lod)
		}
		patch := msg.GetOverviewPatch()
		if len(patch.Tiles) != 1 || len(patch.Tiles[0].Data) != int(lod*lod) {
			t.Fatalf("LOD %d patch has wrong shape: %#v", lod, patch.Tiles)
		}
	}
}

func BenchmarkOverviewPyramidTile(b *testing.B) {
	s := NewServer()
	cid := ChunkID{X: 7, Y: -3}
	s.chunks[cid] = overviewTestChunk(4)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.computeOverviewTile(cid)
	}
}

func BenchmarkOverviewRegionLOD8(b *testing.B) {
	s := NewServer()
	for y := int64(0); y < 100; y++ {
		for x := int64(0); x < 100; x++ {
			cid := ChunkID{X: x, Y: y}
			s.chunks[cid] = overviewTestChunk(uint32(x + y))
			s.overviewTiles[cid] = s.computeOverviewTile(cid)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.assembleOverviewRegionLocked(8, 0, 0, 100, 100)
	}
}
