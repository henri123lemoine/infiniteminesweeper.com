package main

import (
	"encoding/binary"
	"iter"
	"math/bits"
	"slices"
)

// chunkFlags holds one chunk's flags as a bitset of flagged cells plus, per
// flag in cell order, an index into palette: the chunk's distinct
// (design, owner) pairs. Chunks have few distinct flaggers (median 4 in the
// July 2026 world), so a flag costs ~1 byte here instead of an 8-byte entry.
// refs is one byte per flag, or two little-endian bytes once wide (>256
// palette entries).
type chunkFlags struct {
	cells   ChunkBits
	refs    []byte
	palette []Flag
	wide    bool
}

func flagEntryCmp(a, b FlagEntry) int { return int(a.Cell) - int(b.Cell) }

// newChunkFlags builds a chunk's flags from snapshot entries; it does not
// retain entries. Cells outside the chunk are dropped.
func newChunkFlags(entries []FlagEntry) *chunkFlags {
	if !slices.IsSortedFunc(entries, flagEntryCmp) {
		entries = slices.Clone(entries)
		slices.SortStableFunc(entries, flagEntryCmp)
	}
	cf := &chunkFlags{refs: make([]byte, 0, len(entries))}
	for _, e := range entries {
		cell := uint32(e.Cell)
		if cell >= ChunkSize*ChunkSize {
			continue
		}
		idx := cf.paletteIndex(Flag{FlagID: uint32(e.FlagID), Owner: e.Owner})
		if cf.has(cell) {
			cf.setRef(cf.len()-1, idx) // duplicate cell: last entry wins
			continue
		}
		cf.cells[cell>>6] |= 1 << (cell & 63)
		cf.insertRef(cf.len(), idx)
	}
	return cf
}

func (cf *chunkFlags) len() int {
	if cf == nil {
		return 0
	}
	if cf.wide {
		return len(cf.refs) / 2
	}
	return len(cf.refs)
}

func (cf *chunkFlags) has(cell uint32) bool {
	return cf != nil && cf.cells[cell>>6]&(1<<(cell&63)) != 0
}

func (cf *chunkFlags) get(cell uint32) (Flag, bool) {
	if !cf.has(cell) {
		return Flag{}, false
	}
	return cf.palette[cf.ref(cf.rank(cell))], true
}

// set places or replaces the flag at cell.
func (cf *chunkFlags) set(cell uint32, f Flag) {
	idx := cf.paletteIndex(f)
	r := cf.rank(cell)
	if cf.has(cell) {
		cf.setRef(r, idx)
		return
	}
	cf.cells[cell>>6] |= 1 << (cell & 63)
	cf.insertRef(r, idx)
}

// all yields flags in cell order.
func (cf *chunkFlags) all() iter.Seq2[uint32, Flag] {
	return func(yield func(uint32, Flag) bool) {
		if cf == nil {
			return
		}
		i := 0
		for w, word := range cf.cells {
			for word != 0 {
				cell := uint32(w*64 + bits.TrailingZeros64(word))
				word &= word - 1
				if !yield(cell, cf.palette[cf.ref(i)]) {
					return
				}
				i++
			}
		}
	}
}

func (cf *chunkFlags) entries() []FlagEntry {
	out := make([]FlagEntry, 0, cf.len())
	for cell, f := range cf.all() {
		out = append(out, FlagEntry{Cell: uint16(cell), FlagID: uint16(f.FlagID), Owner: f.Owner})
	}
	return out
}

func (cf *chunkFlags) clone() *chunkFlags {
	c := *cf
	c.refs = slices.Clone(cf.refs)
	c.palette = slices.Clone(cf.palette)
	return &c
}

// rank is the number of flagged cells before cell, i.e. its index in refs.
func (cf *chunkFlags) rank(cell uint32) int {
	w := cell >> 6
	n := bits.OnesCount64(cf.cells[w] & (1<<(cell&63) - 1))
	for _, word := range cf.cells[:w] {
		n += bits.OnesCount64(word)
	}
	return n
}

func (cf *chunkFlags) paletteIndex(f Flag) int {
	if i := slices.Index(cf.palette, f); i >= 0 {
		return i
	}
	cf.palette = append(cf.palette, f)
	if len(cf.palette) > 256 && !cf.wide {
		wide := make([]byte, 2*len(cf.refs), 2*cap(cf.refs))
		for i, r := range cf.refs {
			binary.LittleEndian.PutUint16(wide[2*i:], uint16(r))
		}
		cf.refs, cf.wide = wide, true
	}
	return len(cf.palette) - 1
}

func (cf *chunkFlags) ref(i int) int {
	if cf.wide {
		return int(binary.LittleEndian.Uint16(cf.refs[2*i:]))
	}
	return int(cf.refs[i])
}

func (cf *chunkFlags) setRef(i, idx int) {
	if cf.wide {
		binary.LittleEndian.PutUint16(cf.refs[2*i:], uint16(idx))
		return
	}
	cf.refs[i] = byte(idx)
}

func (cf *chunkFlags) insertRef(i, idx int) {
	if cf.wide {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], uint16(idx))
		cf.refs = slices.Insert(cf.refs, 2*i, b[0], b[1])
		return
	}
	cf.refs = slices.Insert(cf.refs, i, byte(idx))
}

// Snapshots share chunkFlags by pointer; a chunk captured by one is cloned
// before its first change afterwards. Caller must hold stateMu for writing.
func (s *Server) setFlagLocked(cid ChunkID, cell uint32, flag Flag) {
	cf := s.flags[cid]
	if cf == nil {
		cf = &chunkFlags{}
		s.flags[cid] = cf
	} else if _, shared := s.snapshotSharedFlags[cid]; shared {
		cf = cf.clone()
		s.flags[cid] = cf
		delete(s.snapshotSharedFlags, cid)
	}
	cf.set(cell, flag)
}
