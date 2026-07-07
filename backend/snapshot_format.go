package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"

	"github.com/klauspost/compress/gzip"
	"github.com/pierrec/lz4/v4"
)

// Binary snapshot format: an 8-byte magic followed by an lz4 frame holding
// hand-encoded big sections (chunks, flags, territory) and a gob tail for
// the small player maps. gob's reflection over millions of map entries was
// ~90% of snapshot encode time; lz4 replaces gzip because the encode is an
// order of magnitude faster for a modestly larger file. Legacy gzip+gob
// snapshots (no magic; gzip's 0x1f8b header) are still read transparently.
var snapshotMagic = [8]byte{'I', 'M', 'S', 'N', 'A', 'P', '0', '1'}

const (
	snapMaxChunks      = 50_000_000
	snapMaxPlayers     = 100_000_000
	snapChunkBitsBytes = ChunkSize * ChunkSize / 8
	snapTerritoryBytes = territoryBlocksTotal * 8
	snapFlagEntryBytes = 8
)

func encodeSnapshot(w io.Writer, data *snapshotData) error {
	if _, err := w.Write(snapshotMagic[:]); err != nil {
		return err
	}
	lw := lz4.NewWriter(w)
	bw := bufio.NewWriterSize(lw, 1<<16)

	var scratch [16]byte
	putU32 := func(v uint32) error {
		binary.LittleEndian.PutUint32(scratch[:4], v)
		_, err := bw.Write(scratch[:4])
		return err
	}
	putChunkID := func(cid ChunkID) error {
		binary.LittleEndian.PutUint64(scratch[:8], uint64(cid.X))
		binary.LittleEndian.PutUint64(scratch[8:16], uint64(cid.Y))
		_, err := bw.Write(scratch[:16])
		return err
	}

	// Section 1: revealed-cell bitsets
	if err := putU32(uint32(len(data.Chunks))); err != nil {
		return err
	}
	var bits [snapChunkBitsBytes]byte
	for cid, cb := range data.Chunks {
		if cb == nil {
			cb = &ChunkBits{}
		}
		if err := putChunkID(cid); err != nil {
			return err
		}
		for i, word := range cb {
			binary.LittleEndian.PutUint64(bits[i*8:], word)
		}
		if _, err := bw.Write(bits[:]); err != nil {
			return err
		}
	}

	// Section 2: flags
	if err := putU32(uint32(len(data.FlagsV2))); err != nil {
		return err
	}
	var fe [snapFlagEntryBytes]byte
	for cid, entries := range data.FlagsV2 {
		if err := putChunkID(cid); err != nil {
			return err
		}
		if err := putU32(uint32(len(entries))); err != nil {
			return err
		}
		for _, e := range entries {
			binary.LittleEndian.PutUint16(fe[0:2], e.Cell)
			binary.LittleEndian.PutUint16(fe[2:4], e.FlagID)
			binary.LittleEndian.PutUint32(fe[4:8], e.Owner)
			if _, err := bw.Write(fe[:]); err != nil {
				return err
			}
		}
	}

	// Section 3: territory blocks
	if err := putU32(uint32(len(data.Territory))); err != nil {
		return err
	}
	var tb [snapTerritoryBytes]byte
	for cid, ct := range data.Territory {
		if ct == nil {
			ct = &chunkTerritory{}
		}
		if err := putChunkID(cid); err != nil {
			return err
		}
		for i, b := range ct {
			binary.LittleEndian.PutUint32(tb[i*8:], b.Owner)
			binary.LittleEndian.PutUint32(tb[i*8+4:], b.Votes)
		}
		if _, err := bw.Write(tb[:]); err != nil {
			return err
		}
	}

	// Tail: everything else via gob (a few thousand entries; cost is trivial)
	tail := *data
	tail.Chunks = nil
	tail.Flags = nil
	tail.FlagsV2 = nil
	tail.Territory = nil
	if err := gob.NewEncoder(bw).Encode(&tail); err != nil {
		return err
	}

	if err := bw.Flush(); err != nil {
		return err
	}
	return lw.Close()
}

func decodeSnapshot(r io.Reader) (snapshotData, error) {
	var head [8]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return snapshotData{}, fmt.Errorf("snapshot header: %w", err)
	}
	if head != snapshotMagic {
		return decodeLegacySnapshot(io.MultiReader(bytes.NewReader(head[:]), r))
	}

	br := bufio.NewReaderSize(lz4.NewReader(r), 1<<16)
	var scratch [16]byte
	getU32 := func() (uint32, error) {
		if _, err := io.ReadFull(br, scratch[:4]); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint32(scratch[:4]), nil
	}
	getChunkID := func() (ChunkID, error) {
		if _, err := io.ReadFull(br, scratch[:16]); err != nil {
			return ChunkID{}, err
		}
		return ChunkID{
			X: int64(binary.LittleEndian.Uint64(scratch[:8])),
			Y: int64(binary.LittleEndian.Uint64(scratch[8:16])),
		}, nil
	}

	var data snapshotData

	n, err := getU32()
	if err != nil || n > snapMaxChunks {
		return data, fmt.Errorf("snapshot chunks section: n=%d err=%w", n, errOr(err))
	}
	data.Chunks = make(map[ChunkID]*ChunkBits, n)
	var bits [snapChunkBitsBytes]byte
	for i := uint32(0); i < n; i++ {
		cid, err := getChunkID()
		if err != nil {
			return data, err
		}
		if _, err := io.ReadFull(br, bits[:]); err != nil {
			return data, err
		}
		cb := &ChunkBits{}
		for w := range cb {
			cb[w] = binary.LittleEndian.Uint64(bits[w*8:])
		}
		data.Chunks[cid] = cb
	}

	n, err = getU32()
	if err != nil || n > snapMaxChunks {
		return data, fmt.Errorf("snapshot flags section: n=%d err=%w", n, errOr(err))
	}
	data.FlagsV2 = make(map[ChunkID][]FlagEntry, n)
	var fe [snapFlagEntryBytes]byte
	for i := uint32(0); i < n; i++ {
		cid, err := getChunkID()
		if err != nil {
			return data, err
		}
		cnt, err := getU32()
		if err != nil || cnt > ChunkSize*ChunkSize {
			return data, fmt.Errorf("snapshot flag count: n=%d err=%w", cnt, errOr(err))
		}
		entries := make([]FlagEntry, cnt)
		for j := range entries {
			if _, err := io.ReadFull(br, fe[:]); err != nil {
				return data, err
			}
			entries[j] = FlagEntry{
				Cell:   binary.LittleEndian.Uint16(fe[0:2]),
				FlagID: binary.LittleEndian.Uint16(fe[2:4]),
				Owner:  binary.LittleEndian.Uint32(fe[4:8]),
			}
		}
		data.FlagsV2[cid] = entries
	}

	n, err = getU32()
	if err != nil || n > snapMaxChunks {
		return data, fmt.Errorf("snapshot territory section: n=%d err=%w", n, errOr(err))
	}
	data.Territory = make(map[ChunkID]*chunkTerritory, n)
	var tb [snapTerritoryBytes]byte
	for i := uint32(0); i < n; i++ {
		cid, err := getChunkID()
		if err != nil {
			return data, err
		}
		if _, err := io.ReadFull(br, tb[:]); err != nil {
			return data, err
		}
		ct := &chunkTerritory{}
		for b := range ct {
			ct[b].Owner = binary.LittleEndian.Uint32(tb[b*8:])
			ct[b].Votes = binary.LittleEndian.Uint32(tb[b*8+4:])
		}
		data.Territory[cid] = ct
	}

	var tail snapshotData
	if err := gob.NewDecoder(br).Decode(&tail); err != nil {
		return data, fmt.Errorf("snapshot tail: %w", err)
	}
	if len(tail.Scores) > snapMaxPlayers {
		return data, errors.New("snapshot tail: implausible player count")
	}
	tail.Chunks = data.Chunks
	tail.FlagsV2 = data.FlagsV2
	tail.Territory = data.Territory
	return tail, nil
}

func decodeLegacySnapshot(r io.Reader) (snapshotData, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return snapshotData{}, fmt.Errorf("legacy snapshot: %w", err)
	}
	defer gz.Close()
	var data snapshotData
	if err := gob.NewDecoder(gz).Decode(&data); err != nil {
		return snapshotData{}, fmt.Errorf("legacy snapshot decode: %w", err)
	}
	return data, nil
}

// errOr keeps the %w verb valid when the guard tripped on a bound, not an error.
func errOr(err error) error {
	if err != nil {
		return err
	}
	return errors.New("bound exceeded")
}
