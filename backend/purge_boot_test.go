package main

import (
	"os"
	"path/filepath"
	"testing"
)

func bootFixtureServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	data := purgeFixture()
	data.flagSets = map[ChunkID]*chunkFlags{}
	for cid, entries := range data.FlagsV2 {
		data.flagSets[cid] = newChunkFlags(entries)
	}
	data.FlagsV2 = nil
	data.NextPlayerID = 10
	if err := writeSnapshotFile(filepath.Join(dir, snapshotFileName), &data); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.dataDir = dir
	if err := s.loadSnapshotFromDisk(); err != nil {
		t.Fatal(err)
	}
	return s
}

func request(t *testing.T, s *Server, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.dataDir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(s *Server, name string) bool {
	_, err := os.Stat(filepath.Join(s.dataDir, name))
	return err == nil
}

func TestPurgeAtBootThenUndoRestoresEverything(t *testing.T) {
	s := bootFixtureServer(t)
	a := ChunkID{}
	botCells := []uint32{cellAt(0, 0), cellAt(7, 7), cellAt(3, 4), cellAt(63, 63)}
	const total = 10

	s.applyPurgeRequests()
	if s.totalRevealed != total || exists(s, prePurgeSnapshotName) {
		t.Fatal("boot without a request file changed something")
	}

	request(t, s, purgeRequestName, "7 # the bot\n")
	s.applyPurgeRequests()
	for _, c := range botCells {
		if s.isCellRevealed(a, c) {
			t.Errorf("cell %d still revealed after purge", c)
		}
	}
	if !s.isCellRevealed(a, cellAt(8, 0)) || !s.isCellRevealed(a, cellAt(16, 0)) {
		t.Error("another player's cell or a shared block was purged")
	}
	if s.isCellFlagged(a, cellAt(20, 20)) || !s.isCellFlagged(a, cellAt(21, 20)) {
		t.Error("flags after purge are wrong")
	}
	if !exists(s, prePurgeSnapshotName) || exists(s, purgeRequestName) || !exists(s, purgeRequestName+appliedRequestSuffix) {
		t.Error("backup or request files are not in the expected state after purge")
	}

	// What survives a restart is the purged world.
	r := NewServer()
	r.dataDir = s.dataDir
	if err := r.loadSnapshotFromDisk(); err != nil {
		t.Fatal(err)
	}
	if r.totalRevealed != total-5 {
		t.Errorf("reloaded totalRevealed = %d, want %d", r.totalRevealed, total-5)
	}

	// Play since the purge must survive the undo.
	r.replayReveal(a, []uint32{cellAt(40, 40)}, otherID)

	request(t, r, unpurgeRequestName, "7\n")
	r.applyPurgeRequests()
	for _, c := range append(botCells, cellAt(40, 40), cellAt(8, 0)) {
		if !r.isCellRevealed(a, c) {
			t.Errorf("cell %d not revealed after undo", c)
		}
	}
	if !r.isCellRevealed(ChunkID{X: 5, Y: -5}, cellAt(10, 10)) {
		t.Error("chunk the purge dropped was not restored")
	}
	if r.totalRevealed != total+1 {
		t.Errorf("totalRevealed after undo = %d, want %d", r.totalRevealed, total+1)
	}
	if !r.isCellFlagged(a, cellAt(20, 20)) || !r.isCellFlagged(ChunkID{X: 9, Y: 9}, 5) {
		t.Error("the bot's flags were not restored")
	}
	if r.territoryOwner(a, cellAt(0, 0)) != botID {
		t.Error("territory of a restored block was not restored")
	}
	if len(r.explosions[a]) != 2 {
		t.Errorf("explosions after undo = %v, want both records", r.explosions[a])
	}
	if !exists(r, unpurgeRequestName+appliedRequestSuffix) || !exists(r, prePurgeSnapshotName) {
		t.Error("undo should rename its request and keep the backup")
	}
}

func TestPurgeAtBootRefusesWhenABackupExists(t *testing.T) {
	s := bootFixtureServer(t)
	request(t, s, prePurgeSnapshotName, "an earlier backup")
	request(t, s, purgeRequestName, "7\n")

	s.applyPurgeRequests()

	if !s.isCellRevealed(ChunkID{}, cellAt(0, 0)) {
		t.Error("purged despite an existing backup")
	}
	if !exists(s, purgeRequestName) {
		t.Error("a refused request must stay in place")
	}
	if b, _ := os.ReadFile(filepath.Join(s.dataDir, prePurgeSnapshotName)); string(b) != "an earlier backup" {
		t.Error("the existing backup was overwritten")
	}
}
