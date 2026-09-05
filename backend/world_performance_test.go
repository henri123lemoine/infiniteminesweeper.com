package main

import (
	"fmt"
	"os"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
	"runtime"
	"testing"
	"time"
)

func TestWorldMemory(t *testing.T) {
	dir := os.Getenv("IMS_BENCH_DATA_DIR")
	if dir == "" {
		t.Skip("set IMS_BENCH_DATA_DIR to a directory containing a world snapshot")
	}
	s := NewServer()
	s.dataDir = dir
	started := time.Now()
	if err := s.loadSnapshotFromDisk(); err != nil {
		t.Fatal(err)
	}
	loadTime := time.Since(started)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started = time.Now()
	snapshot := s.captureSnapshotData()
	copyTime := time.Since(started)
	runtime.ReadMemStats(&after)
	t.Logf("chunks=%d revealed=%d overview_tiles=%d load=%v heap=%.2f MiB snapshot_copy=%v snapshot_alloc=%.2f MiB", len(s.chunks), s.totalRevealed, len(s.overviewTiles), loadTime, float64(before.HeapAlloc)/(1<<20), copyTime, float64(after.TotalAlloc-before.TotalAlloc)/(1<<20))
	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(s)
}

func BenchmarkOverviewTile(b *testing.B) {
	s := NewServer()
	cid := ChunkID{}
	s.chunks[cid] = &ChunkBits{}
	for i := range s.chunks[cid] {
		s.chunks[cid][i] = ^uint64(0)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.computeOverviewTile(cid)
	}
}

func BenchmarkOverviewRegion(b *testing.B) {
	s := NewServer()
	for y := int64(0); y < 16; y++ {
		for x := int64(0); x < 16; x++ {
			s.chunks[ChunkID{X: x, Y: y}] = overviewTestChunk(uint32(x + y))
		}
	}
	s.rebuildOverviewLocked()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.assembleOverviewRegionLocked(32, 0, 0, 16, 16)
	}
}

func BenchmarkOverviewFanout(b *testing.B) {
	for _, viewers := range []int{1, 100, 500} {
		b.Run(fmt.Sprint(viewers), func(b *testing.B) {
			s := NewServer()
			cid := ChunkID{}
			s.chunks[cid] = overviewTestChunk(3)
			s.rebuildOverviewLocked()
			players := make([]*Player, viewers)
			for i := range players {
				players[i] = &Player{Send: make(chan []byte, 1), done: make(chan struct{})}
				s.overviewSubs[players[i]] = map[uint32]overviewSubscription{8: {Global: true, Ready: true}}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.overviewDirty[cid] = struct{}{}
				s.broadcastOverview()
				for _, player := range players {
					<-player.Send
				}
			}
		})
	}
}

func BenchmarkFlagSnapshot(b *testing.B) {
	s := NewServer()
	for x := int64(0); x < 1000; x++ {
		flags := make(chunkFlags, 2048)
		for i := range flags {
			flags[i] = FlagEntry{Cell: uint16(i), FlagID: 3, Owner: 1}
		}
		s.flags[ChunkID{X: x}] = flags
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snapshot := s.captureSnapshotData()
		s.setCellFlagged(ChunkID{X: int64(i % 1000)}, 100, 1, 4, &map[ChunkID][]*pb.FlagPlacement{})
		runtime.KeepAlive(snapshot)
	}
}

func BenchmarkChunkRegionPan(b *testing.B) {
	s := NewServer()
	player := &Player{ID: 1, Send: make(chan []byte, 1), done: make(chan struct{})}
	s.players[1] = map[*Player]struct{}{player: {}}
	ids := make([]ChunkID, 49)
	for i := range ids {
		ids[i] = ChunkID{X: int64(i % 7), Y: int64(i / 7)}
		s.chunks[ids[i]] = overviewTestChunk(uint32(i))
		flags := make(chunkFlags, 1024)
		for j := range flags {
			flags[j] = FlagEntry{Cell: uint16(j * 4), FlagID: uint16(j % 16)}
		}
		s.flags[ids[i]] = flags
	}
	s.sendChunkRegionSync(1, ids)
	wireBytes := len(<-player.Send)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.sendChunkRegionSync(1, ids)
		<-player.Send
	}
	b.ReportMetric(float64(wireBytes), "wire_bytes/op")
}
