package main

import (
	"fmt"
	"log"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
)

const (
	purgeRequestName     = "purge.txt"
	unpurgeRequestName   = "unpurge.txt"
	appliedRequestSuffix = ".applied"
	prePurgeSnapshotName = "snapshot.before-purge.gob.gz"
)

func playerIDsFromFile(path string) (map[uint32]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := newBanList()
	if err := parseBanList(f, b); err != nil {
		return nil, err
	}
	return b.players, nil
}

// applyPurgeRequests runs once at boot, before anyone can connect. A
// purge.txt in the data dir (player IDs, same format as bans.txt) removes
// those players' footprint; an unpurge.txt puts back what a purge of those
// players removed. A request file is renamed once applied, so it runs once.
func (s *Server) applyPurgeRequests() {
	for name, apply := range map[string]func(map[uint32]struct{}) error{
		purgeRequestName:   s.purgeAtBoot,
		unpurgeRequestName: s.unpurgeAtBoot,
	} {
		path := filepath.Join(s.dataDir, name)
		ids, err := playerIDsFromFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err == nil && len(ids) == 0 {
			err = fmt.Errorf("no player IDs listed")
		}
		if err == nil {
			err = apply(ids)
		}
		if err != nil {
			log.Printf("[purge] %s NOT applied: %v", name, err)
			continue
		}
		if err := os.Rename(path, path+appliedRequestSuffix); err != nil {
			log.Printf("[purge] %s applied but could not be renamed, it will run again next boot: %v", name, err)
		}
	}
}

// purgeAtBoot first saves the world exactly as it is and keeps that file as
// snapshot.before-purge.gob.gz, which is what unpurgeAtBoot restores from.
// It refuses to run while an earlier backup is still there.
func (s *Server) purgeAtBoot(ids map[uint32]struct{}) error {
	backup := filepath.Join(s.dataDir, prePurgeSnapshotName)
	if _, err := os.Stat(backup); err == nil {
		return fmt.Errorf("%s already exists; move it away before purging again", prePurgeSnapshotName)
	}
	if err := s.saveSnapshotToDisk(); err != nil {
		return fmt.Errorf("saving the pre-purge snapshot: %w", err)
	}
	if err := os.Link(filepath.Join(s.dataDir, snapshotFileName), backup); err != nil {
		return fmt.Errorf("keeping the pre-purge snapshot: %w", err)
	}

	data := s.captureSnapshotData()
	rep := purgePlayers(&data, ids)
	s.restoreSnapshotData(data)
	var out strings.Builder
	printPurgeReport(&out, rep, data.PlayerNames)
	log.Printf("[purge] applied; undo by listing the same IDs in %s and restarting\n%s", unpurgeRequestName, out.String())

	if err := s.saveSnapshotToDisk(); err != nil {
		return fmt.Errorf("purged in memory but the snapshot was not saved: %w", err)
	}
	return nil
}

func (s *Server) unpurgeAtBoot(ids map[uint32]struct{}) error {
	f, err := os.Open(filepath.Join(s.dataDir, prePurgeSnapshotName))
	if err != nil {
		return err
	}
	before, err := decodeSnapshot(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("decoding %s: %w", prePurgeSnapshotName, err)
	}

	cur := s.captureSnapshotData()
	cells, flags, explosions := restorePurged(&cur, &before, ids)
	s.restoreSnapshotData(cur)
	log.Printf("[purge] undone: put back %d revealed cells, %d flags, %d explosion records", cells, flags, explosions)

	if err := s.saveSnapshotToDisk(); err != nil {
		return fmt.Errorf("restored in memory but the snapshot was not saved: %w", err)
	}
	return nil
}

// restorePurged puts back into cur everything purgePlayers would remove from
// before for these players. It only adds: cells revealed and flags placed
// since the purge stay as they are.
func restorePurged(cur, before *snapshotData, banned map[uint32]struct{}) (cells, flags, explosions int) {
	if cur.Territory == nil {
		cur.Territory = make(map[ChunkID]*chunkTerritory)
	}
	for cid, ct := range before.Territory {
		bcb := before.Chunks[cid]
		if ct == nil || bcb == nil {
			continue
		}
		for i := range ct {
			blk := ct[i]
			if _, ok := banned[blk.Owner]; !ok || blk.Votes == 0 || blockRevealed(bcb, i) != int(blk.Votes) {
				continue
			}
			ccb := cur.Chunks[cid]
			if ccb == nil {
				ccb = &ChunkBits{}
				cur.Chunks[cid] = ccb
			}
			row, mask := blockMask(i)
			for dy := range territoryBlockSize {
				add := bcb[row+dy] & mask &^ ccb[row+dy]
				cells += bits.OnesCount64(add)
				ccb[row+dy] |= add
			}
			cct := cur.Territory[cid]
			if cct == nil {
				cct = &chunkTerritory{}
				cur.Territory[cid] = cct
			}
			if cct[i].Votes == 0 {
				cct[i] = blk
			}
		}
	}

	if cur.flagSets == nil {
		cur.flagSets = make(map[ChunkID]*chunkFlags)
	}
	for cid, bcf := range before.flagSets {
		ccf, ccb := cur.flagSets[cid], cur.Chunks[cid]
		var add []FlagEntry
		for _, e := range bcf.entries() {
			if _, ok := banned[e.Owner]; !ok || ccf.has(uint32(e.Cell)) {
				continue
			}
			if ccb != nil && ccb[e.Cell/64]&(1<<(e.Cell%64)) != 0 {
				continue
			}
			add = append(add, e)
		}
		if len(add) == 0 {
			continue
		}
		flags += len(add)
		if ccf != nil {
			add = append(ccf.entries(), add...)
		}
		cur.flagSets[cid] = newChunkFlags(add)
	}

	if cur.Explosions == nil {
		cur.Explosions = make(map[ChunkID][]ExplosionEntry)
	}
	for cid, entries := range before.Explosions {
		have := make(map[uint16]struct{}, len(cur.Explosions[cid]))
		for _, e := range cur.Explosions[cid] {
			have[e.Cell] = struct{}{}
		}
		for _, e := range entries {
			if _, ok := banned[e.Owner]; !ok {
				continue
			}
			if _, dup := have[e.Cell]; dup {
				continue
			}
			cur.Explosions[cid] = append(cur.Explosions[cid], e)
			explosions++
		}
	}
	return cells, flags, explosions
}
