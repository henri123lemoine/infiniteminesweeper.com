package main

import (
	"flag"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

const purgedSnapshotName = "snapshot.purged.gob.gz"

type purgeStats struct {
	Blocks     int
	Revealed   int
	Flags      int
	Explosions int
}

type purgeReport struct {
	Players         map[uint32]*purgeStats
	StaleExplosions int
	EmptyChunks     int
}

func (r *purgeReport) stats(id uint32) *purgeStats {
	st := r.Players[id]
	if st == nil {
		st = &purgeStats{}
		r.Players[id] = st
	}
	return st
}

// purgePlayers removes the banned players' flags and explosion records, and
// unreveals every 8×8 territory block they dominate. Reveals have no per-cell
// author, so block ownership (a majority vote) is the closest attribution.
func purgePlayers(data *snapshotData, banned map[uint32]struct{}) purgeReport {
	rep := purgeReport{Players: make(map[uint32]*purgeStats)}
	touched := make(map[ChunkID]struct{})

	for cid, ct := range data.Territory {
		if ct == nil {
			continue
		}
		for i := range ct {
			blk := &ct[i]
			if _, ok := banned[blk.Owner]; !ok || blk.Votes == 0 {
				continue
			}
			st := rep.stats(blk.Owner)
			st.Blocks++
			if cb := data.Chunks[cid]; cb != nil {
				bx, by := i%territoryBlocksSide, i/territoryBlocksSide
				mask := uint64(1<<territoryBlockSize-1) << (bx * territoryBlockSize)
				for dy := range territoryBlockSize {
					row := &cb[by*territoryBlockSize+dy]
					st.Revealed += bits.OnesCount64(*row & mask)
					*row &^= mask
				}
			}
			*blk = TerritoryBlock{}
			touched[cid] = struct{}{}
		}
	}

	purgeFlags(data, banned, &rep, touched)

	for cid, entries := range data.Explosions {
		_, wasTouched := touched[cid]
		cb := data.Chunks[cid]
		kept := slices.DeleteFunc(slices.Clone(entries), func(e ExplosionEntry) bool {
			if _, ok := banned[e.Owner]; ok {
				rep.stats(e.Owner).Explosions++
				return true
			}
			if wasTouched && (cb == nil || cb[e.Cell/64]&(1<<(e.Cell%64)) == 0) {
				rep.StaleExplosions++
				return true
			}
			return false
		})
		if len(kept) == len(entries) {
			continue
		}
		touched[cid] = struct{}{}
		if len(kept) == 0 {
			delete(data.Explosions, cid)
		} else {
			data.Explosions[cid] = kept
		}
	}

	for cid := range touched {
		if chunkIsEmpty(data, cid) {
			delete(data.Chunks, cid)
			delete(data.Territory, cid)
			rep.EmptyChunks++
		}
	}
	return rep
}

// purgeFlags handles all three snapshot flag encodings; old snapshots carry
// the map form, hand-built ones FlagsV2.
func purgeFlags(data *snapshotData, banned map[uint32]struct{}, rep *purgeReport, touched map[ChunkID]struct{}) {
	drop := func(owner uint32) bool {
		if _, ok := banned[owner]; !ok {
			return false
		}
		rep.stats(owner).Flags++
		return true
	}

	for cid, cf := range data.flagSets {
		entries := cf.entries()
		kept := slices.DeleteFunc(slices.Clone(entries), func(e FlagEntry) bool { return drop(e.Owner) })
		if len(kept) == len(entries) {
			continue
		}
		touched[cid] = struct{}{}
		if len(kept) == 0 {
			delete(data.flagSets, cid)
		} else {
			data.flagSets[cid] = newChunkFlags(kept)
		}
	}
	for cid, entries := range data.FlagsV2 {
		kept := slices.DeleteFunc(slices.Clone(entries), func(e FlagEntry) bool { return drop(e.Owner) })
		if len(kept) == len(entries) {
			continue
		}
		touched[cid] = struct{}{}
		if len(kept) == 0 {
			delete(data.FlagsV2, cid)
		} else {
			data.FlagsV2[cid] = kept
		}
	}
	for cid, cells := range data.Flags {
		before := len(cells)
		for cell, f := range cells {
			if drop(f.Owner) {
				delete(cells, cell)
			}
		}
		if len(cells) == before {
			continue
		}
		touched[cid] = struct{}{}
		if len(cells) == 0 {
			delete(data.Flags, cid)
		}
	}
}

func chunkIsEmpty(data *snapshotData, cid ChunkID) bool {
	if cb := data.Chunks[cid]; cb != nil && *cb != (ChunkBits{}) {
		return false
	}
	return data.flagSets[cid].len() == 0 && len(data.FlagsV2[cid]) == 0 && len(data.Flags[cid]) == 0
}

func bannedIDsFromFiles(dir string) (map[uint32]struct{}, error) {
	b := newBanList()
	for _, name := range []string{bansFileName, autoBansFileName} {
		f, err := os.Open(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		err = parseBanList(f, b)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return b.players, nil
}

func parsePlayerIDs(list string) (map[uint32]struct{}, error) {
	ids := make(map[uint32]struct{})
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("invalid player ID %q", part)
		}
		ids[uint32(n)] = struct{}{}
	}
	return ids, nil
}

func writeSnapshotFile(path string, data *snapshotData) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := encodeSnapshot(f, data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func printPurgeReport(w io.Writer, rep purgeReport, names map[uint32]string) {
	ids := make([]uint32, 0, len(rep.Players))
	for id := range rep.Players {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tBLOCKS\tREVEALED CELLS\tFLAGS\tEXPLOSIONS")
	var total purgeStats
	for _, id := range ids {
		st := rep.Players[id]
		fmt.Fprintf(tw, "%d\t%s\t%d\t%d\t%d\t%d\n", id, names[id], st.Blocks, st.Revealed, st.Flags, st.Explosions)
		total.Blocks += st.Blocks
		total.Revealed += st.Revealed
		total.Flags += st.Flags
		total.Explosions += st.Explosions
	}
	fmt.Fprintf(tw, "total\t\t%d\t%d\t%d\t%d\n", total.Blocks, total.Revealed, total.Flags, total.Explosions)
	tw.Flush()
	fmt.Fprintf(w, "also dropped %d explosion records on newly unrevealed cells and %d emptied chunks\n", rep.StaleExplosions, rep.EmptyChunks)
}

// runPurge implements `run-app purge`. It never modifies the input snapshot.
func runPurge(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("data", envOr("DATA_DIR", "data"), "data directory holding snapshot.gob.gz")
	idList := fs.String("ids", "", "comma-separated player IDs to purge")
	useBans := fs.Bool("banned", false, "also purge every player ID listed in bans.txt and bans-auto.txt")
	apply := fs.Bool("apply", false, "write the purged snapshot (default: dry run)")
	out := fs.String("out", "", "output path (default: <data>/"+purgedSnapshotName+")")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: run-app purge [-data DIR] [-ids 1,2] [-banned] [-apply] [-out PATH]")
		fmt.Fprintln(stderr, "Unreveals the 8x8 blocks the given players dominate and removes their flags and")
		fmt.Fprintln(stderr, "explosion records. Reveals from before territory tracking cannot be attributed.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	banned, err := parsePlayerIDs(*idList)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *useBans {
		fromFiles, err := bannedIDsFromFiles(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "reading ban files: %v\n", err)
			return 1
		}
		for id := range fromFiles {
			banned[id] = struct{}{}
		}
	}
	if len(banned) == 0 {
		fmt.Fprintln(stderr, "no player IDs given: pass -ids and/or -banned")
		return 2
	}

	outPath := *out
	if outPath == "" {
		outPath = filepath.Join(*dir, purgedSnapshotName)
	}
	inPath := filepath.Join(*dir, snapshotFileName)
	if a, b := mustAbs(inPath), mustAbs(outPath); a == b {
		fmt.Fprintln(stderr, "refusing to overwrite the input snapshot; choose another -out")
		return 2
	}

	if fi, err := os.Stat(filepath.Join(*dir, walFileName)); err == nil && fi.Size() > 0 {
		msg := "wal.log is not empty: its reveals would be replayed over the purged snapshot. " +
			"Stop the server (it snapshots and truncates the WAL on shutdown) and rerun."
		if *apply {
			fmt.Fprintln(stderr, msg)
			return 1
		}
		fmt.Fprintln(stderr, "warning: "+msg)
	}

	f, err := os.Open(inPath)
	if err != nil {
		fmt.Fprintf(stderr, "opening snapshot: %v\n", err)
		return 1
	}
	data, err := decodeSnapshot(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(stderr, "decoding snapshot: %v\n", err)
		return 1
	}

	for id := range banned {
		if _, known := data.PlayerNames[id]; !known {
			fmt.Fprintf(stderr, "warning: player ID %d is not in the snapshot\n", id)
		}
	}

	rep := purgePlayers(&data, banned)
	printPurgeReport(stdout, rep, data.PlayerNames)

	if !*apply {
		fmt.Fprintln(stdout, "dry run: nothing written; rerun with -apply to write the purged snapshot")
		return 0
	}
	if err := writeSnapshotFile(outPath, &data); err != nil {
		fmt.Fprintf(stderr, "writing %s: %v\n", outPath, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", outPath)
	fmt.Fprintf(stdout, "to use it: stop the server, back up %s, then move the new file over it and restart\n", inPath)
	return 0
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustAbs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
