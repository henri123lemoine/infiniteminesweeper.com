package main

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

func assertChunkFlagsMatch(t *testing.T, cf *chunkFlags, want map[uint32]Flag) {
	t.Helper()
	if cf.len() != len(want) {
		t.Fatalf("len = %d, want %d", cf.len(), len(want))
	}
	var cells []uint32
	for cell, f := range cf.all() {
		if w, ok := want[cell]; !ok || w != f {
			t.Fatalf("all() yielded cell %d = %+v, want %+v (present=%v)", cell, f, w, ok)
		}
		cells = append(cells, cell)
	}
	if !slices.IsSorted(cells) || len(cells) != len(want) {
		t.Fatalf("all() not in cell order or incomplete: %d cells", len(cells))
	}
	for cell := uint32(0); cell < ChunkSize*ChunkSize; cell++ {
		got, ok := cf.get(cell)
		w, wok := want[cell]
		if ok != wok || got != w || cf.has(cell) != wok {
			t.Fatalf("cell %d: get = %+v,%v has = %v; want %+v,%v", cell, got, ok, cf.has(cell), w, wok)
		}
	}
}

func TestChunkFlagsMatchesMapModel(t *testing.T) {
	for _, owners := range []int{3, 40, 600} {
		rng := rand.New(rand.NewSource(int64(owners)))
		cf := &chunkFlags{}
		want := map[uint32]Flag{}
		for i := 0; i < 3000; i++ {
			cell := uint32(rng.Intn(ChunkSize * ChunkSize))
			f := Flag{FlagID: uint32(rng.Intn(8)), Owner: uint32(rng.Intn(owners))}
			cf.set(cell, f)
			want[cell] = f
		}
		if cf.wide != (len(cf.palette) > 256) || (owners == 600 && !cf.wide) {
			t.Fatalf("owners=%d: wide=%v with %d palette entries", owners, cf.wide, len(cf.palette))
		}
		assertChunkFlagsMatch(t, cf, want)

		rebuilt := newChunkFlags(cf.entries())
		assertChunkFlagsMatch(t, rebuilt, want)
	}
}

func TestNewChunkFlagsSortsDedupesAndDropsOutOfRange(t *testing.T) {
	entries := []FlagEntry{
		{Cell: 900, FlagID: 1, Owner: 5},
		{Cell: 3, FlagID: 2, Owner: 6},
		{Cell: 900, FlagID: 7, Owner: 8},
		{Cell: 5000, FlagID: 1, Owner: 1},
	}
	original := slices.Clone(entries)
	cf := newChunkFlags(entries)
	if !reflect.DeepEqual(entries, original) {
		t.Fatal("newChunkFlags reordered the caller's slice")
	}
	want := []FlagEntry{{Cell: 3, FlagID: 2, Owner: 6}, {Cell: 900, FlagID: 7, Owner: 8}}
	if got := cf.entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestChunkFlagsCloneIsIndependent(t *testing.T) {
	cf := newChunkFlags([]FlagEntry{{Cell: 10, FlagID: 1, Owner: 1}})
	c := cf.clone()
	c.set(11, Flag{FlagID: 2, Owner: 2})
	c.set(10, Flag{FlagID: 3, Owner: 3})
	if f, _ := cf.get(10); cf.len() != 1 || f != (Flag{FlagID: 1, Owner: 1}) {
		t.Fatalf("clone writes leaked into the original: %v", cf.entries())
	}
}

func TestNilChunkFlagsReadsAsEmpty(t *testing.T) {
	var cf *chunkFlags
	if cf.len() != 0 || cf.has(0) {
		t.Fatal("nil chunkFlags not empty")
	}
	if _, ok := cf.get(0); ok {
		t.Fatal("nil chunkFlags returned a flag")
	}
	for range cf.all() {
		t.Fatal("nil chunkFlags yielded a flag")
	}
}
