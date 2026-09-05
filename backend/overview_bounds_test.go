package main

import (
	"bytes"
	"math"
	"runtime"
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestOverviewRegionLimits(t *testing.T) {
	for _, dims := range [][3]int{{64, math.MaxUint32, math.MaxUint32}, {8, 1, 9000}, {64, 0, 1}, {3, 1, 1}, {8, 1000, 1000}} {
		if validOverviewRegion(uint32(dims[0]), dims[1], dims[2]) {
			t.Fatalf("accepted oversized/invalid region: %v", dims)
		}
	}
	if !validOverviewRegion(8, 256, 256) {
		t.Fatal("rejected 4 MiB region")
	}
}

func TestOverviewSparseWorldRegionalSnapshot(t *testing.T) {
	s := NewServer()
	s.chunks[ChunkID{}] = overviewTestChunk(0)
	s.chunks[ChunkID{X: 10000, Y: 10000}] = overviewTestChunk(0)
	s.rebuildOverviewLocked()
	if len(s.overviewImages) != 0 {
		t.Fatal("unbounded sparse world allocated a global image")
	}
	p := &Player{Send: make(chan []byte, 8), done: make(chan struct{})}
	for i, lod := range []uint32{4, 64, 8} {
		s.handleOverviewRequest(p, &pb.OverviewRequest{Lod: lod, OriginX: -2, OriginY: -2, WidthChunks: 8, HeightChunks: 8, Subscribe: true, ReplaceSubscription: true, RequestId: uint32(i + 1)})
		snapshot := decodePBMsg(t, <-p.Send).GetOverviewSnapshot()
		if snapshot == nil || snapshot.Global || snapshot.RequestId != uint32(i+1) || len(snapshot.Pixels) != 8*8*int(lod*lod) {
			t.Fatalf("invalid regional snapshot: %v", snapshot)
		}
		expected := s.assembleOverviewRegionLocked(lod, -2, -2, 8, 8)
		if !bytes.Equal(snapshot.Pixels, expected) {
			t.Fatal("regional snapshot lost tile pixels")
		}
		if len(s.overviewSubs[p]) != 1 || !s.overviewSubs[p][lod].Ready {
			t.Fatal("old detail levels remain subscribed")
		}
	}
}

func TestOverviewSnapshotYieldsAndCatchesUp(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(oldProcs)
	s := NewServer()
	cid := ChunkID{}
	s.chunks[cid] = overviewTestChunk(0)
	s.rebuildOverviewLocked()
	p := &Player{Send: make(chan []byte, 8), done: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		s.handleOverviewRequest(p, &pb.OverviewRequest{Lod: 8, WidthChunks: 256, HeightChunks: 256, Subscribe: true, RequestId: 1})
		close(done)
	}()
	deadline := time.After(5 * time.Second)
	for {
		s.stateMu.Lock()
		sub, exists := s.overviewSubs[p][8]
		if exists && !sub.Ready {
			s.chunks[cid][0] |= 2
			s.overviewDirty[cid] = struct{}{}
			s.stateMu.Unlock()
			s.broadcastOverview()
			break
		}
		s.stateMu.Unlock()
		select {
		case <-done:
			t.Fatal("snapshot monopolized the game lock")
		case <-deadline:
			t.Fatal("snapshot did not start")
		default:
			runtime.Gosched()
		}
	}
	<-done
	first := decodePBMsg(t, <-p.Send).GetOverviewSnapshot()
	second := decodePBMsg(t, <-p.Send).GetOverviewPatch()
	if first == nil || second == nil || second.Revision <= first.Revision || len(second.Tiles) != 1 {
		t.Fatal("missing ordered catch-up after yielding snapshot")
	}
	if !bytes.Equal(second.Tiles[0].Data, s.overviewTiles[cid].LOD8[:]) {
		t.Fatal("catch-up differs from current world")
	}
}

func TestOverviewEmptyWorldReturnsPixels(t *testing.T) {
	s := NewServer()
	p := &Player{Send: make(chan []byte, 1), done: make(chan struct{})}
	s.handleOverviewRequest(p, &pb.OverviewRequest{Lod: 4, WidthChunks: 8, HeightChunks: 8, RequestId: 1})
	snapshot := decodePBMsg(t, <-p.Send).GetOverviewSnapshot()
	if snapshot == nil || snapshot.Unchanged || len(snapshot.Pixels) != 1024 {
		t.Fatal("first request to an empty world has no image")
	}
}
