package main

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWALFlushToDisk(t *testing.T) {
	s := NewServer()
	s.useS3 = false
	dir := t.TempDir()
	s.dataDir = dir
	s.proximityRadius = -1

	cid := ChunkID{X: 0, Y: 0}
	// Do a reveal and a flag to generate WAL entries
	s.handleReveal(1, 1, cid, 0, false, false)
	s.handleReveal(1, 2, cid, 1, true, false) // may be wrong/correct depending on density

	if err := s.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	path := filepath.Join(dir, walFileName)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	types := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"type":"reveal"`) {
			types["reveal"] = true
		}
		if strings.Contains(line, `"type":"flag"`) {
			types["flag"] = true
		}
		if strings.Contains(line, `"type":"score_update"`) {
			types["score_update"] = true
		}
	}
	if len(types) == 0 {
		t.Fatalf("no WAL entries found")
	}
}

func TestReplayWALFromDiskWithTruncatedTail(t *testing.T) {
	src := NewServer()
	src.useS3 = false
	dir := t.TempDir()
	src.dataDir = dir
	src.proximityRadius = -1

	cid := ChunkID{X: 0, Y: 0}
	for cell := uint32(0); cell < 64; cell += 9 {
		src.handleReveal(1, uint64(cell)+1, cid, cell, false, false)
	}
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, walFileName), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if _, err := f.WriteString(`{"timestamp":"2026-01-01T00:00:00Z","type":"rev`); err != nil {
		t.Fatalf("append partial record: %v", err)
	}
	f.Close()

	dst := NewServer()
	dst.useS3 = false
	dst.dataDir = dir
	if err := dst.replayWAL(); err != nil {
		t.Fatalf("replayWAL: %v", err)
	}
	if dst.walSeq != src.walSeq {
		t.Fatalf("walSeq = %d, want %d", dst.walSeq, src.walSeq)
	}
	if src.chunks[cid] == nil || dst.chunks[cid] == nil || *dst.chunks[cid] != *src.chunks[cid] {
		t.Fatalf("replayed chunk bits differ from source")
	}
}

func newDiskServer(dir string) *Server {
	s := NewServer()
	s.useS3 = false
	s.dataDir = dir
	s.proximityRadius = -1
	return s
}

func appendToWAL(t *testing.T, dir, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, walFileName), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// A record torn by a hard kill must not hide what later flushes append.
func TestReplayWALContinuesPastTornRecord(t *testing.T) {
	dir := t.TempDir()
	src := newDiskServer(dir)
	cid := ChunkID{X: 0, Y: 0}

	src.handleReveal(1, 1, cid, 0, false, false)
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	appendToWAL(t, dir, `{"timestamp":"2026-01-01T00:00:00Z","type":"rev`)
	for cell := uint32(9); cell < 640; cell += 9 {
		src.handleReveal(1, uint64(cell)+1, cid, cell, false, false)
	}
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}

	dst := newDiskServer(dir)
	if err := dst.replayWAL(); err != nil {
		t.Fatalf("replayWAL: %v", err)
	}
	if dst.chunks[cid] == nil || *dst.chunks[cid] != *src.chunks[cid] {
		t.Fatalf("reveals appended after the torn record were not replayed")
	}
	if dst.scores[1] != src.scores[1] {
		t.Fatalf("score = %d, want %d", dst.scores[1], src.scores[1])
	}
}

// Entries flushed while a snapshot is being written are not in it and must
// stay in the WAL.
func TestSnapshotTrimKeepsLaterWALEntries(t *testing.T) {
	dir := t.TempDir()
	src := newDiskServer(dir)
	cid := ChunkID{X: 0, Y: 0}

	src.handleReveal(1, 1, cid, 0, false, false)
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	covered := src.walDiskSize()
	data := src.captureSnapshotData()
	if err := writeSnapshotFile(filepath.Join(dir, snapshotFileName), &data); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	for cell := uint32(9); cell < 640; cell += 9 {
		src.handleReveal(1, uint64(cell)+1, cid, cell, false, false)
	}
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	if err := src.trimWALPrefix(covered); err != nil {
		t.Fatalf("trimWALPrefix: %v", err)
	}

	dst := newDiskServer(dir)
	if err := dst.loadSnapshotFromDisk(); err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := dst.replayWAL(); err != nil {
		t.Fatalf("replayWAL: %v", err)
	}
	if *dst.chunks[cid] != *src.chunks[cid] {
		t.Fatalf("reveals flushed during the snapshot were lost")
	}
	if dst.scores[1] != src.scores[1] {
		t.Fatalf("score = %d, want %d", dst.scores[1], src.scores[1])
	}
}

func TestSnapshotToDiskEmptiesCoveredWAL(t *testing.T) {
	dir := t.TempDir()
	s := newDiskServer(dir)
	s.handleReveal(1, 1, ChunkID{}, 0, false, false)
	if err := s.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	if err := s.saveSnapshotToDisk(); err != nil {
		t.Fatalf("saveSnapshotToDisk: %v", err)
	}
	if size := s.walDiskSize(); size != 0 {
		t.Fatalf("WAL size after snapshot = %d, want 0", size)
	}
}

func walLineCounts(t *testing.T, dir string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	n, _, err := func() (int, int, error) {
		f, err := os.Open(filepath.Join(dir, walFileName))
		if err != nil {
			return 0, 0, err
		}
		defer f.Close()
		return decodeWAL(f, func(e WALEntry) { counts[e.Type]++ })
	}()
	if err != nil || n == 0 {
		t.Fatalf("read wal: n=%d err=%v", n, err)
	}
	return counts
}

// One player's burst of reveals must not write a score and a stats record
// per action: that is what filled the production volume.
func TestFlushKeepsOnlyLastScoreAndStatsPerPlayer(t *testing.T) {
	dir := t.TempDir()
	src := newDiskServer(dir)
	cid := ChunkID{X: 0, Y: 0}
	for cell := uint32(0); cell < 640; cell += 9 {
		src.handleReveal(1, uint64(cell)+1, cid, cell, false, false)
	}
	src.handleReveal(2, 1, cid, 700, false, false)
	if err := src.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}

	counts := walLineCounts(t, dir)
	// Stats records that unlock an advancement are kept on top of the last one.
	if counts["score_update"] != 2 || counts["stats_update"] > 6 {
		t.Fatalf("superseded records were written: %v", counts)
	}

	dst := newDiskServer(dir)
	if err := dst.replayWAL(); err != nil {
		t.Fatalf("replayWAL: %v", err)
	}
	if dst.chunks[cid] == nil || *dst.chunks[cid] != *src.chunks[cid] {
		t.Fatalf("reveals differ after replay")
	}
	for _, pid := range []uint32{1, 2} {
		if dst.scores[pid] != src.scores[pid] || dst.streaks[pid] != src.streaks[pid] {
			t.Fatalf("player %d: score/streak = %d/%d, want %d/%d", pid, dst.scores[pid], dst.streaks[pid], src.scores[pid], src.streaks[pid])
		}
		if (src.playerStats[pid] == nil) != (dst.playerStats[pid] == nil) ||
			(src.playerStats[pid] != nil && *dst.playerStats[pid] != *src.playerStats[pid]) {
			t.Fatalf("player %d: stats differ after replay", pid)
		}
	}
}

func TestCompactWALKeepsUnlocksAndOrder(t *testing.T) {
	stats := func(seq uint64, unlocks string) WALEntry {
		return WALEntry{Type: "stats_update", Sequence: seq, Data: []byte(`{"player_id":7,"stats":{}` + unlocks + `}`)}
	}
	in := []WALEntry{
		stats(1, ""),
		stats(2, `,"new_unlocks":["first_steps"]`),
		{Type: "reveal", Sequence: 3, Data: []byte(`{}`)},
		stats(4, ""),
		stats(5, ""),
	}
	var got []uint64
	for _, e := range compactWALEntries(in) {
		got = append(got, e.Sequence)
	}
	if want := []uint64{2, 3, 5}; !slices.Equal(got, want) {
		t.Fatalf("kept sequences %v, want %v", got, want)
	}
}

func TestOversizedWALRequestsSnapshot(t *testing.T) {
	old := walSnapshotTriggerBytes
	walSnapshotTriggerBytes = 1
	defer func() { walSnapshotTriggerBytes = old }()

	s := newDiskServer(t.TempDir())
	s.handleReveal(1, 1, ChunkID{}, 0, false, false)
	if err := s.flushWAL(); err != nil {
		t.Fatalf("flushWAL: %v", err)
	}
	select {
	case <-s.snapshotSignal:
	default:
		t.Fatalf("a WAL over the size trigger did not request a snapshot")
	}
}

func TestRevealBucketLimitsBursts(t *testing.T) {
	now := time.Now()
	tb := newTokenBucket(revealBucketCap, revealRefillPer)
	allowed := 0
	for range 10 * revealBucketCap {
		if tb.take(now) {
			allowed++
		}
	}
	if allowed != revealBucketCap {
		t.Fatalf("burst allowed %d reveals, want %d", allowed, revealBucketCap)
	}
	if !tb.take(now.Add(time.Second)) {
		t.Fatalf("bucket did not refill")
	}
	var unlimited TokenBucket
	if !unlimited.take(now) {
		t.Fatalf("zero-value bucket must not limit")
	}
}
