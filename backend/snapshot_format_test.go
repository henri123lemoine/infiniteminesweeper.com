package main

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"

	"github.com/klauspost/compress/gzip"
)

func sampleSnapshotData() snapshotData {
	cb := &ChunkBits{}
	cb[0] = 0xDEADBEEF
	cb[63] = 1 << 63
	ct := &chunkTerritory{}
	ct[0] = TerritoryBlock{Owner: 7, Votes: 3}
	ct[63] = TerritoryBlock{Owner: 9, Votes: 1}
	return snapshotData{
		Chunks: map[ChunkID]*ChunkBits{
			{X: 0, Y: 0}:       cb,
			{X: -5, Y: 900000}: {},
		},
		FlagsV2: map[ChunkID][]FlagEntry{
			{X: 1, Y: -1}: {
				{Cell: 0, FlagID: 3, Owner: 42},
				{Cell: 4095, FlagID: 160, Owner: 0},
			},
		},
		Territory: map[ChunkID]*chunkTerritory{{X: 2, Y: 2}: ct},
		Scores:    map[uint32]int32{1: 100, 2: -0 + 5},
		Streaks:   map[uint32]uint32{1: 7},
		PlayerNames: map[uint32]string{
			1: "Alice",
			2: "Böb 泊",
		},
		PlayerFlags:  map[uint32]uint32{1: 9},
		PlayerViews:  map[uint32]PlayerView{1: {Chunk: ChunkID{X: 3, Y: 4}, Cell: 17, RectWChunks: 2, RectHChunks: 3}},
		NextPlayerID: 12345,
		SessionTokens: map[string]uint32{
			"tok-abc": 1,
		},
		PlayerStats: map[uint32]*PlayerStats{
			1: {CellsRevealed: 10, MaxSafeStreak: 4},
		},
		UnlockedAdvancements: map[uint32]map[string]bool{1: {"land_surveyor": true}},
	}
}

func assertSnapshotEqual(t *testing.T, want, got snapshotData) {
	t.Helper()
	if !reflect.DeepEqual(want.Chunks, got.Chunks) {
		t.Errorf("Chunks mismatch")
	}
	if !reflect.DeepEqual(want.FlagsV2, got.FlagsV2) {
		t.Errorf("FlagsV2 mismatch: want %v got %v", want.FlagsV2, got.FlagsV2)
	}
	if !reflect.DeepEqual(want.Territory, got.Territory) {
		t.Errorf("Territory mismatch")
	}
	if !reflect.DeepEqual(want.Scores, got.Scores) ||
		!reflect.DeepEqual(want.Streaks, got.Streaks) ||
		!reflect.DeepEqual(want.PlayerNames, got.PlayerNames) ||
		!reflect.DeepEqual(want.PlayerFlags, got.PlayerFlags) ||
		!reflect.DeepEqual(want.PlayerViews, got.PlayerViews) ||
		!reflect.DeepEqual(want.SessionTokens, got.SessionTokens) ||
		!reflect.DeepEqual(want.PlayerStats, got.PlayerStats) ||
		!reflect.DeepEqual(want.UnlockedAdvancements, got.UnlockedAdvancements) ||
		want.NextPlayerID != got.NextPlayerID {
		t.Errorf("tail mismatch")
	}
}

func TestSnapshotBinaryRoundtrip(t *testing.T) {
	want := sampleSnapshotData()
	var buf bytes.Buffer
	if err := encodeSnapshot(&buf, &want); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeSnapshot(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertSnapshotEqual(t, want, got)
}

func TestSnapshotLegacyGobStillReadable(t *testing.T) {
	want := sampleSnapshotData()
	want.FlagsV2 = nil // legacy snapshots carried the map form instead
	want.Territory = nil
	want.Flags = map[ChunkID]map[uint32]Flag{
		{X: 1, Y: -1}: {7: {FlagID: 3, Owner: 42}},
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gz).Encode(&want); err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	gz.Close()

	got, err := decodeSnapshot(&buf)
	if err != nil {
		t.Fatalf("decode legacy: %v", err)
	}
	if !reflect.DeepEqual(want.Flags, got.Flags) {
		t.Errorf("legacy Flags mismatch")
	}
	if !reflect.DeepEqual(want.Chunks, got.Chunks) {
		t.Errorf("legacy Chunks mismatch")
	}
	if got.PlayerNames[2] != "Böb 泊" {
		t.Errorf("legacy tail mismatch")
	}
}

func TestSnapshotDecodeGarbageFails(t *testing.T) {
	if _, err := decodeSnapshot(bytes.NewReader([]byte("IMSNAP01garbage everywhere"))); err == nil {
		t.Fatal("garbage after magic should fail")
	}
	if _, err := decodeSnapshot(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("short junk should fail")
	}
}
