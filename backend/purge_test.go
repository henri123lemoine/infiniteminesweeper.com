package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	botID   = uint32(7)
	otherID = uint32(8)
)

func cellAt(x, y int) uint32 { return uint32(y*ChunkSize + x) }

func setRevealed(cb *ChunkBits, cells ...uint32) {
	for _, c := range cells {
		cb[c/64] |= 1 << (c % 64)
	}
}

func isRevealed(cb *ChunkBits, cell uint32) bool {
	return cb != nil && cb[cell/64]&(1<<(cell%64)) != 0
}

// purgeFixture: chunk A holds two blocks only the bot revealed, (0,0) and
// (7,7), a block (1,0) owned by another player, and a block (2,0) the bot
// dominates but someone else also revealed in.
func purgeFixture() snapshotData {
	a := ChunkID{X: 0, Y: 0}
	cb := &ChunkBits{}
	setRevealed(cb,
		cellAt(0, 0), cellAt(7, 7), cellAt(3, 4), // bot block (0,0)
		cellAt(8, 0), cellAt(15, 7), // other player's block (1,0)
		cellAt(63, 63),                              // bot block (7,7), last cell
		cellAt(16, 0), cellAt(17, 0), cellAt(18, 0), // shared block (2,0)
	)
	ct := &chunkTerritory{}
	ct[0] = TerritoryBlock{Owner: botID, Votes: 3}
	ct[2] = TerritoryBlock{Owner: botID, Votes: 1}
	ct[1] = TerritoryBlock{Owner: otherID, Votes: 2}
	ct[63] = TerritoryBlock{Owner: botID, Votes: 1}

	// Chunk B only ever saw the bot.
	b := ChunkID{X: 5, Y: -5}
	bb := &ChunkBits{}
	setRevealed(bb, cellAt(10, 10))
	bt := &chunkTerritory{}
	bt[1*territoryBlocksSide+1] = TerritoryBlock{Owner: botID, Votes: 1}

	return snapshotData{
		Chunks:    map[ChunkID]*ChunkBits{a: cb, b: bb},
		Territory: map[ChunkID]*chunkTerritory{a: ct, b: bt},
		FlagsV2: map[ChunkID][]FlagEntry{
			a: {
				{Cell: uint16(cellAt(20, 20)), FlagID: 1, Owner: botID},
				{Cell: uint16(cellAt(21, 20)), FlagID: 2, Owner: otherID},
				{Cell: uint16(cellAt(22, 20)), FlagID: 3, Owner: 0},
			},
			{X: 9, Y: 9}: {{Cell: 5, FlagID: 1, Owner: botID}},
		},
		Explosions: map[ChunkID][]ExplosionEntry{
			a: {
				{Cell: uint16(cellAt(3, 4)), Owner: botID},
				{Cell: uint16(cellAt(15, 7)), Owner: otherID},
			},
		},
		Scores:      map[uint32]int32{botID: 9000, otherID: 10},
		PlayerNames: map[uint32]string{botID: "Bot", otherID: "Human"},
	}
}

func TestPurgeUnrevealsOnlyDominatedBlocks(t *testing.T) {
	data := purgeFixture()
	a, b := ChunkID{}, ChunkID{X: 5, Y: -5}

	rep := purgePlayers(&data, map[uint32]struct{}{botID: {}})

	cb := data.Chunks[a]
	for _, c := range []uint32{cellAt(0, 0), cellAt(7, 7), cellAt(3, 4), cellAt(63, 63)} {
		if isRevealed(cb, c) {
			t.Errorf("cell %d in a bot block is still revealed", c)
		}
	}
	for _, c := range []uint32{cellAt(8, 0), cellAt(15, 7), cellAt(16, 0), cellAt(17, 0), cellAt(18, 0)} {
		if !isRevealed(cb, c) {
			t.Errorf("cell %d in a block someone else revealed in was unrevealed", c)
		}
	}
	if ct := data.Territory[a]; ct[2] != (TerritoryBlock{Owner: botID, Votes: 1}) {
		t.Errorf("shared block territory changed: %+v", ct[2])
	}
	if ct := data.Territory[a]; ct[0] != (TerritoryBlock{}) || ct[63] != (TerritoryBlock{}) || ct[1].Owner != otherID {
		t.Errorf("territory not reset correctly: %+v %+v %+v", ct[0], ct[63], ct[1])
	}

	st := rep.Players[botID]
	if st == nil || st.Blocks != 3 || st.Shared != 1 || st.Revealed != 5 || st.Flags != 2 || st.Explosions != 1 {
		t.Errorf("bot stats = %+v, want blocks=3 shared=1 revealed=5 flags=2 explosions=1", st)
	}
	if _, ok := rep.Players[otherID]; ok {
		t.Error("report lists a player that was not purged")
	}

	if _, ok := data.Chunks[b]; ok {
		t.Error("chunk that only the bot touched should be removed")
	}
	if _, ok := data.Territory[b]; ok {
		t.Error("territory of the removed chunk should be removed")
	}
	if _, ok := data.FlagsV2[ChunkID{X: 9, Y: 9}]; ok {
		t.Error("flag-only chunk entry should be removed")
	}
	if rep.EmptyChunks != 2 {
		t.Errorf("EmptyChunks = %d, want 2 (the reveal-only chunk and the flag-only chunk)", rep.EmptyChunks)
	}

	flags := data.FlagsV2[a]
	if len(flags) != 2 || flags[0].Owner != otherID || flags[1].Owner != 0 {
		t.Errorf("remaining flags = %+v, want the other player's and the unowned one", flags)
	}

	ex := data.Explosions[a]
	if len(ex) != 1 || ex[0].Cell != uint16(cellAt(15, 7)) {
		t.Errorf("remaining explosions = %+v, want only the untouched one", ex)
	}
	if data.Scores[botID] != 9000 {
		t.Error("purge must not alter scores")
	}
}

func TestPurgeDoesNotTouchAnythingWithoutMatch(t *testing.T) {
	data := purgeFixture()
	want := purgeFixture()

	rep := purgePlayers(&data, map[uint32]struct{}{999: {}})

	if len(rep.Players) != 0 || rep.EmptyChunks != 0 {
		t.Errorf("unexpected report: %+v", rep)
	}
	assertSnapshotEqual(t, want, data)
}

func TestPurgeHandlesFlagSetsAndLegacyMaps(t *testing.T) {
	cid := ChunkID{X: 1, Y: 1}
	data := snapshotData{
		Chunks:   map[ChunkID]*ChunkBits{cid: {}},
		flagSets: map[ChunkID]*chunkFlags{cid: newChunkFlags([]FlagEntry{{Cell: 1, FlagID: 1, Owner: botID}, {Cell: 2, FlagID: 1, Owner: otherID}})},
		Flags:    map[ChunkID]map[uint32]Flag{cid: {3: {FlagID: 1, Owner: botID}, 4: {FlagID: 1, Owner: otherID}}},
	}

	rep := purgePlayers(&data, map[uint32]struct{}{botID: {}})

	if got := data.flagSets[cid].entries(); len(got) != 1 || got[0].Owner != otherID {
		t.Errorf("flagSets after purge = %+v", got)
	}
	if len(data.Flags[cid]) != 1 || data.Flags[cid][4].Owner != otherID {
		t.Errorf("legacy flags after purge = %+v", data.Flags[cid])
	}
	if rep.Players[botID].Flags != 2 {
		t.Errorf("flags removed = %d, want 2", rep.Players[botID].Flags)
	}
}

func TestPurgedSnapshotRoundtripsAndLoadsIntoServer(t *testing.T) {
	data := purgeFixture()
	data.flagSets = map[ChunkID]*chunkFlags{}
	for cid, entries := range data.FlagsV2 {
		data.flagSets[cid] = newChunkFlags(entries)
	}
	data.FlagsV2 = nil
	data.NextPlayerID = 10
	a := ChunkID{}

	purgePlayers(&data, map[uint32]struct{}{botID: {}})

	var buf bytes.Buffer
	if err := encodeSnapshot(&buf, &data); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeSnapshot(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	s := NewServer()
	s.restoreSnapshotData(got)

	if s.isCellRevealed(a, cellAt(3, 4)) {
		t.Error("purged cell is revealed after load")
	}
	if !s.isCellRevealed(a, cellAt(8, 0)) {
		t.Error("legitimate cell lost after load")
	}
	if s.totalRevealed != 5 {
		t.Errorf("totalRevealed = %d, want 5", s.totalRevealed)
	}
	if s.isCellFlagged(a, cellAt(20, 20)) || !s.isCellFlagged(a, cellAt(21, 20)) {
		t.Error("flags after load do not match the purge")
	}
}

func writeFixtureSnapshot(t *testing.T, dir string) {
	t.Helper()
	data := purgeFixture()
	f, err := os.Create(filepath.Join(dir, snapshotFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := encodeSnapshot(f, &data); err != nil {
		t.Fatal(err)
	}
}

func runPurgeCmd(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = runPurge(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunPurgeDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSnapshot(t, dir)

	code, stdout, stderr := runPurgeCmd("-data", dir, "-ids", "7")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Bot") || !strings.Contains(stdout, "dry run") {
		t.Errorf("unexpected output:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, purgedSnapshotName)); !os.IsNotExist(err) {
		t.Error("dry run wrote an output file")
	}
}

func TestRunPurgeApplyLeavesInputUntouched(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSnapshot(t, dir)
	inPath := filepath.Join(dir, snapshotFileName)
	before, _ := os.ReadFile(inPath)

	code, _, stderr := runPurgeCmd("-data", dir, "-ids", "7", "-apply")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}

	after, _ := os.ReadFile(inPath)
	if !bytes.Equal(before, after) {
		t.Error("input snapshot was modified")
	}
	f, err := os.Open(filepath.Join(dir, purgedSnapshotName))
	if err != nil {
		t.Fatalf("no output written: %v", err)
	}
	defer f.Close()
	got, err := decodeSnapshot(f)
	if err != nil {
		t.Fatalf("output does not decode: %v", err)
	}
	if isRevealed(got.Chunks[ChunkID{}], cellAt(3, 4)) {
		t.Error("output still has the bot's cell")
	}
}

func TestRunPurgeReadsBanFiles(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSnapshot(t, dir)
	if err := os.WriteFile(filepath.Join(dir, bansFileName), []byte("7 # the bot\n203.0.113.0/24\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runPurgeCmd("-data", dir, "-banned")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Bot") || strings.Contains(stdout, "Human") {
		t.Errorf("expected only the banned player in the report:\n%s", stdout)
	}
}

func TestRunPurgeRefusesApplyWithPendingWAL(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSnapshot(t, dir)
	if err := os.WriteFile(filepath.Join(dir, walFileName), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runPurgeCmd("-data", dir, "-ids", "7", "-apply")
	if code == 0 || !strings.Contains(stderr, "wal.log") {
		t.Fatalf("exit %d, stderr %q: want refusal mentioning wal.log", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, purgedSnapshotName)); !os.IsNotExist(err) {
		t.Error("output written despite pending WAL")
	}

	if code, _, stderr := runPurgeCmd("-data", dir, "-ids", "7"); code != 0 || !strings.Contains(stderr, "warning") {
		t.Errorf("dry run should warn but succeed, got exit %d, stderr %q", code, stderr)
	}
}

func TestRunPurgeRejectsBadUsage(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSnapshot(t, dir)

	cases := map[string][]string{
		"no ids":         {"-data", dir},
		"bad id":         {"-data", dir, "-ids", "abc"},
		"zero id":        {"-data", dir, "-ids", "0"},
		"out is input":   {"-data", dir, "-ids", "7", "-out", filepath.Join(dir, snapshotFileName)},
		"missing source": {"-data", filepath.Join(dir, "nope"), "-ids", "7"},
	}
	for name, args := range cases {
		if code, _, _ := runPurgeCmd(args...); code == 0 {
			t.Errorf("%s: expected a non-zero exit", name)
		}
	}

	if _, stdout, stderr := runPurgeCmd("-data", dir, "-ids", "7,4242"); !strings.Contains(stderr, "4242") || stdout == "" {
		t.Errorf("unknown ID should warn but proceed; stderr=%q", stderr)
	}
}

func TestRunPurgePreviewMarksRemovedBlocks(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSnapshot(t, dir)
	out := filepath.Join(dir, "preview")

	if code, _, stderr := runPurgeCmd("-data", dir, "-ids", "7", "-preview", out); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	// The fixture's chunks (0,0) and (5,-5) fall in different regions.
	if _, err := os.Stat(filepath.Join(out, "region_0_-96.png")); err != nil {
		t.Errorf("no image for the second changed region: %v", err)
	}
	f, err := os.Open(filepath.Join(out, "region_0_0.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	const panel = previewRegionChunks*territoryBlocksSide + previewPanelGap
	lit := func(p, bx, by int) bool {
		r, _, _, _ := img.At(p*panel+bx, by).RGBA()
		return r>>8 > 20
	}
	isRed := func(bx, by int) bool {
		r, g, _, _ := img.At(2*panel+bx, by).RGBA()
		return r>>8 == 230 && g>>8 == 60
	}
	if !isRed(0, 0) || !isRed(7, 7) {
		t.Error("blocks only the bot revealed are not marked as removed")
	}
	if isRed(1, 0) || isRed(2, 0) {
		t.Error("another player's block or the shared block is marked as removed")
	}
	if !lit(0, 0, 0) || lit(1, 0, 0) || !lit(1, 1, 0) {
		t.Error("before/after panels do not show the bot block vanishing and the other block staying")
	}

	// The whole-world map spans chunks (0,-5) to (5,0) at one pixel per block.
	mf, err := os.Open(filepath.Join(out, "map_removed.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer mf.Close()
	m, err := png.Decode(mf)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Bounds().Size(); got.X != 6*territoryBlocksSide || got.Y != 6*territoryBlocksSide {
		t.Errorf("map size = %v, want 48x48", got)
	}
	if r, g, _, _ := m.At(0, 5*territoryBlocksSide).RGBA(); r>>8 != 230 || g>>8 != 60 {
		t.Error("map does not mark the bot block of chunk (0,0) as removed")
	}
	for _, name := range []string{"map_before.png", "map_after.png"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}

	if code, _, _ := runPurgeCmd("-data", dir, "-ids", "7", "-preview", out, "-view", "1,2,3"); code == 0 {
		t.Error("a malformed -view should fail")
	}
	small := filepath.Join(dir, "small")
	if code, _, stderr := runPurgeCmd("-data", dir, "-ids", "7", "-preview", small, "-view", "0,0,0,0"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	sf, err := os.Open(filepath.Join(small, "map_before.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer sf.Close()
	if cfg, err := png.DecodeConfig(sf); err != nil || cfg.Width != territoryBlocksSide {
		t.Errorf("-view 0,0,0,0 map width = %d (%v), want %d", cfg.Width, err, territoryBlocksSide)
	}
}
