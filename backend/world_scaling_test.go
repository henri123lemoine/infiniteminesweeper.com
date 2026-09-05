package main

import (
	"bytes"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestSnapshotFlagsStayImmutableAcrossWrites(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: -1, Y: 2}
	for _, cell := range []uint32{1, 9, 20} {
		s.setFlagLocked(cid, cell, Flag{FlagID: 3, Owner: 7})
	}
	first := s.captureSnapshotData()
	wantFirst := append([]FlagEntry(nil), first.FlagsV2[cid]...)
	s.replayFlag(cid, 9, 6, 8)
	s.replayFlag(cid, 4, 2, 9)
	second := s.captureSnapshotData()
	wantSecond := append([]FlagEntry(nil), second.FlagsV2[cid]...)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for cell := uint32(0); cell < 100; cell++ {
			s.stateMu.Lock()
			s.setCellFlagged(cid, cell, 11, 5, &map[ChunkID][]*pb.FlagPlacement{})
			s.stateMu.Unlock()
		}
	}()
	for _, snapshot := range []snapshotData{first, second} {
		var encoded bytes.Buffer
		if err := encodeSnapshot(&encoded, &snapshot); err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeSnapshot(&encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded.FlagsV2, snapshot.FlagsV2) {
			t.Fatal("concurrent writes changed flags during snapshot encoding")
		}
	}
	<-done
	if !reflect.DeepEqual(first.FlagsV2[cid], wantFirst) || !reflect.DeepEqual(second.FlagsV2[cid], wantSecond) {
		t.Fatal("a later flag insertion or replacement mutated an earlier snapshot")
	}
	if len(s.flags[cid]) != 100 {
		t.Fatalf("live world has %d flags, want 100", len(s.flags[cid]))
	}
}

func TestOverviewDetailEvictionAndInvalidation(t *testing.T) {
	s := NewServer()
	for x := 0; x <= overviewDetailCacheMaxEntries; x++ {
		s.chunks[ChunkID{X: int64(x)}] = overviewTestChunk(uint32(x))
	}
	for x := 0; x < overviewDetailCacheMaxEntries; x++ {
		s.overviewPixelsLocked(ChunkID{X: int64(x)}, 32)
	}
	cid := ChunkID{}
	before := append([]byte(nil), s.overviewPixelsLocked(cid, 64)...)
	s.overviewPixelsLocked(ChunkID{X: overviewDetailCacheMaxEntries}, 32)
	if len(s.overviewDetails) != overviewDetailCacheMaxEntries || s.overviewDetailLRU.Len() != overviewDetailCacheMaxEntries {
		t.Fatal("detailed tile cache exceeded its cap")
	}
	if s.overviewDetails[cid] == nil || s.overviewDetails[ChunkID{X: 1}] != nil {
		t.Fatal("eviction did not preserve the recently used tile")
	}
	s.setCellFlagged(cid, 5, 1, 2, &map[ChunkID][]*pb.FlagPlacement{})
	if bytes.Equal(before, s.overviewPixelsLocked(cid, 64)) {
		t.Fatal("flagging retained stale detailed pixels")
	}
	s.setCellRevealed(cid, 7, 1, &map[ChunkID]*pb.RevealedCells{})
	s.refreshOverviewDirtyLocked()
	full := s.computeFullTileData(cid)
	for _, lod := range []uint32{1, 2, 4, 8, 12, 16, 32, 64} {
		want := full
		if lod != 64 {
			want = downsampleOverviewColor(full, int(lod))
		}
		if !bytes.Equal(s.overviewPixelsLocked(cid, lod), want) {
			t.Fatalf("LOD %d changed pixels after eviction or mutation", lod)
		}
	}
}

func TestFullTileMatchesCellPalette(t *testing.T) {
	s := NewServer()
	for _, cid := range []ChunkID{{}, {X: -2, Y: -3}, {X: 7, Y: 4}} {
		for _, dense := range []bool{false, true} {
			reveals := overviewTestChunk(3)
			if dense {
				for i := range reveals {
					reveals[i] = ^uint64(0)
				}
			}
			s.chunks[cid] = reveals
			for _, cell := range []uint32{0, 1, 63, 64, 65, 4095} {
				s.setFlagLocked(cid, cell, Flag{FlagID: 62 + cell%10})
			}
			got := s.computeFullTileData(cid)
			mines := s.getMineBitmap(cid)
			for cell := uint32(0); cell < 4096; cell++ {
				if want := s.paletteIndexAt(cid, cell, reveals, s.flags[cid], mines); got[cell] != want {
					t.Fatalf("chunk=%v dense=%v cell=%d palette=%d, want %d", cid, dense, cell, got[cell], want)
				}
			}
		}
	}
}

func TestOverviewFanoutSharesWireDataAndFiltersRegions(t *testing.T) {
	s := NewServer()
	cid := ChunkID{X: -2, Y: 3}
	s.chunks[cid] = overviewTestChunk(1)
	s.rebuildOverviewLocked()
	players := make([]*Player, 20)
	for i := range players {
		p := &Player{Send: make(chan []byte, 2), done: make(chan struct{})}
		players[i] = p
		s.overviewSubs[p] = map[uint32]overviewSubscription{8: {Global: true, Ready: true}}
	}
	outside := &Player{Send: make(chan []byte, 2), done: make(chan struct{})}
	s.overviewSubs[outside] = map[uint32]overviewSubscription{8: {OriginX: 100, WidthChunks: 1, HeightChunks: 1, Ready: true}}
	pending := &Player{Send: make(chan []byte, 2), done: make(chan struct{})}
	s.overviewSubs[pending] = map[uint32]overviewSubscription{8: {Global: true}}
	s.overviewDirty[cid] = struct{}{}
	s.broadcastOverview()
	var shared []byte
	for _, p := range players {
		data := <-p.Send
		if shared == nil {
			shared = data
		} else if &shared[0] != &data[0] {
			t.Fatal("identical subscriptions did not share encoded data")
		}
		patch := decodePBMsg(t, data).GetOverviewPatch()
		if patch == nil || len(patch.Tiles) != 1 || patch.Tiles[0].Tile.X != int32(cid.X) {
			t.Fatal("subscriber received incorrect patch")
		}
	}
	if len(outside.Send) != 0 || len(pending.Send) != 0 {
		t.Fatal("patch sent outside a region or before its initial snapshot")
	}
	if _, ok := s.overviewSubs[pending][8].Pending[cid]; !ok {
		t.Fatal("in-flight snapshot lost its pending update")
	}
	if s.overviewPatchBytes != uint64(len(players)*64) || s.overviewWireBytes != uint64(len(players)*len(shared)) {
		t.Fatal("fanout metrics did not account for every recipient")
	}
}

func TestChunkRegionFlagAllocationAndInvalidation(t *testing.T) {
	s := NewServer()
	player := &Player{ID: 1, Send: make(chan []byte, 1), done: make(chan struct{})}
	s.players[1] = map[*Player]struct{}{player: {}}
	ids := []ChunkID{{X: -3, Y: 2}, {X: -1, Y: 4}, {X: 2, Y: -5}}
	for _, cid := range ids {
		s.chunks[cid] = overviewTestChunk(4)
		for cell := uint32(0); cell < 2048; cell++ {
			s.setFlagLocked(cid, cell, Flag{FlagID: cell % 128})
		}
		entry := s.getChunkSyncEntry(cid)
		capacity := 0
		for _, group := range entry.cs.FlagGroups {
			capacity += cap(group.Cells.Cells)
		}
		if capacity > 2*len(s.flags[cid]) {
			t.Fatalf("flag groups reserved %d cells for %d flags", capacity, len(s.flags[cid]))
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 2 {
			s.setCellFlagged(ids[0], 3000, 1, 99, &map[ChunkID][]*pb.FlagPlacement{})
		}
		s.sendChunkRegionSync(1, ids)
		wire := <-player.Send
		regionSync := decodePBMsg(t, wire).GetChunkRegionSync()
		if regionSync == nil || regionSync.Origin.X != -3 || regionSync.Origin.Y != -5 || regionSync.Width != 6 || regionSync.Height != 10 {
			t.Fatalf("incorrect region envelope: %v", regionSync)
		}
		region := &pb.ChunkRegion{}
		if err := proto.Unmarshal(regionSync.Chunks, region); err != nil {
			t.Fatal(err)
		}
		if len(region.Chunks) != len(ids) {
			t.Fatalf("decoded %d chunks, want %d", len(region.Chunks), len(ids))
		}
		for i, cid := range ids {
			if !proto.Equal(region.Chunks[i], s.serializeChunk(cid)) {
				t.Fatalf("chunk %v changed during serialization (attempt %d)", cid, attempt)
			}
		}
	}
}
