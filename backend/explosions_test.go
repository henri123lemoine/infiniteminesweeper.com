package main

import (
	"os"
	"testing"
)

// findMineAndSafeCell scans chunks until it finds one containing both a mine
// and a safe cell.
func findMineAndSafeCell(t *testing.T, s *Server) (ChunkID, uint32, uint32) {
	t.Helper()
	for x := int64(0); x < 50; x++ {
		cid := ChunkID{X: x, Y: 0}
		bm := s.getMineBitmap(cid)
		mine, safe := int64(-1), int64(-1)
		for cell := uint32(0); cell < ChunkSize*ChunkSize; cell++ {
			if bm[cell>>3]&(1<<(cell&7)) != 0 {
				if mine < 0 {
					mine = int64(cell)
				}
			} else if safe < 0 {
				safe = int64(cell)
			}
			if mine >= 0 && safe >= 0 {
				return cid, uint32(mine), uint32(safe)
			}
		}
	}
	t.Fatal("no chunk with both a mine and a safe cell found")
	return ChunkID{}, 0, 0
}

func TestExplosionRecordedOnMineReveal(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const pid = uint32(11)

	cid, mineCell, safeCell := findMineAndSafeCell(t, s)

	s.handleReveal(pid, 1, cid, mineCell, false, false)
	if owner, ok := s.explosions[cid].get(mineCell); !ok || owner != pid {
		t.Fatalf("mine reveal not attributed: owner=%d ok=%v, want %d", owner, ok, pid)
	}

	s.handleReveal(pid, 2, cid, safeCell, false, false)
	if _, ok := s.explosions[cid].get(safeCell); ok {
		t.Fatalf("safe reveal must not record an explosion")
	}
}

func TestExplosionWALReplay(t *testing.T) {
	s := NewServer()
	cid, mineCell, safeCell := findMineAndSafeCell(t, s)

	s.replayReveal(cid, []uint32{mineCell, safeCell}, 21)
	if owner, ok := s.explosions[cid].get(mineCell); !ok || owner != 21 {
		t.Fatalf("replayed explosion owner = %d ok=%v, want 21", owner, ok)
	}
	if _, ok := s.explosions[cid].get(safeCell); ok {
		t.Fatalf("safe cell must not be recorded as an explosion")
	}

	// Pre-provenance entries (owner 0) record nothing.
	s2 := NewServer()
	s2.replayReveal(cid, []uint32{mineCell}, 0)
	if _, ok := s2.explosions[cid].get(mineCell); ok {
		t.Fatalf("owner-0 replay must not record an explosion")
	}
}

func TestExplosionSnapshotRoundtrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "explosion-snap")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s := NewServer()
	s.dataDir = dir
	cid := ChunkID{X: -7, Y: 9}
	s.stateMu.Lock()
	s.recordExplosionLocked(cid, 123, 42)
	s.recordExplosionLocked(cid, 7, 43)
	s.stateMu.Unlock()

	if err := s.saveSnapshotToDisk(); err != nil {
		t.Fatalf("save: %v", err)
	}

	s2 := NewServer()
	s2.dataDir = dir
	if err := s2.loadSnapshotFromDisk(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if owner, ok := s2.explosions[cid].get(123); !ok || owner != 42 {
		t.Fatalf("restored explosion owner = %d ok=%v, want 42", owner, ok)
	}
	if owner, ok := s2.explosions[cid].get(7); !ok || owner != 43 {
		t.Fatalf("restored explosion owner = %d ok=%v, want 43", owner, ok)
	}
}

func TestAdvancementSyncThrottle(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	const pid = uint32(5)

	cid, _, safeCell := findMineAndSafeCell(t, s)

	// First action: zero advSyncLast is beyond the throttle window, so a
	// sync goes out and stamps the time.
	s.handleReveal(pid, 1, cid, safeCell, false, false)
	first := s.advSyncLast[pid]
	if first.IsZero() {
		t.Fatalf("first action should have sent an AdvancementSync")
	}

	// An immediate second action with no new unlock is throttled: the stamp
	// must not move.
	var second uint32
	for cell := safeCell + 1; cell < ChunkSize*ChunkSize; cell++ {
		if !s.isMine(cid, cell) && !s.isCellRevealed(cid, cell) {
			second = cell
			break
		}
	}
	if second == 0 {
		t.Skip("no second safe cell in chunk")
	}
	s.handleReveal(pid, 2, cid, second, false, false)
	if !s.advSyncLast[pid].Equal(first) {
		t.Fatalf("stat-only sync within the throttle window must be suppressed")
	}
}
