package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"math/bits"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func init() {
	// Register the concrete types once for gob so we don't have to do it each
	// encode().
	gob.Register(ChunkID{})
	gob.Register(ChunkBits{})
	gob.Register(PlayerStats{})
}

// PERSISTENCE CONSTANTS
const (
	// S3 configuration
	defaultBucketName = "infiniteminesweeper"
	snapshotKey       = "snapshot.gob.gz"

	// Multi-object WAL prefix. Each flush writes a unique key under this
	// prefix; truncation deletes the whole set. This avoids the old
	// read-append-rewrite cycle that turned every 2-minute flush into a full
	// O(WAL_size) memory spike on the server.
	walPrefix = "wal-segments/"

	// Local disk configuration
	snapshotFileName = "snapshot.gob.gz"
	snapshotTmpName  = "snapshot.tmp.gz"
	walFileName      = "wal.log"

	// Nudge the flush worker to drain the WAL once the in-memory buffer
	// holds this many entries, regardless of the wall-clock flush tick. Caps
	// walBuffer memory under heavy bursts — at ~200 bytes per entry this
	// bounds peak WAL memory near 2 MB, versus potentially tens of MB across
	// the whole 2-minute tick window under load.
	walBufferFlushThreshold = 10000
)

// WAL entry types
type WALEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "reveal", "flag", "score_update", "player", or "stats_update"
	Data      []byte    `json:"data"` // JSON encoded data
	Sequence  uint64    `json:"sequence"`
}

// walPlayerData persists identity changes (join, rename, flag change) so they
// survive a crash between snapshots. Without it, a crash loses every identity
// created since the last snapshot: scores replay from the WAL but names and
// session tokens vanish.
type walPlayerData struct {
	PlayerID     uint32 `json:"player_id"`
	Name         string `json:"name"`
	FlagID       uint32 `json:"flag_id"`
	SessionToken string `json:"session_token,omitempty"`
}

// snapshotData is what actually gets serialized. Gob can handle maps with
// struct keys, so we keep the exact types.
type snapshotData struct {
	Chunks       map[ChunkID]*ChunkBits
	Flags        map[ChunkID]map[uint32]Flag
	Scores       map[uint32]int32
	Streaks      map[uint32]uint32
	PlayerNames  map[uint32]string
	PlayerFlags  map[uint32]uint32
	PlayerViews  map[uint32]PlayerView
	NextPlayerID uint32
	// Persist session tokens so identities survive restarts
	SessionTokens map[string]uint32
	// Advancements. UnlockedFlags is intentionally not persisted here — it's
	// fully derivable from UnlockedAdvancements + achievementDefs, so we
	// rebuild it in restoreSnapshotData instead of storing it twice.
	PlayerStats          map[uint32]*PlayerStats
	UnlockedAdvancements map[uint32]map[string]bool
}

func (s *Server) initAWS() error {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"), // Default region, can be overridden by env
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %v", err)
	}

	s.s3Client = s3.New(sess)
	s.bucketName = os.Getenv("S3_BUCKET_NAME")
	if s.bucketName == "" {
		s.bucketName = defaultBucketName
	}

	log.Printf("Using S3 bucket: %s", s.bucketName)
	return nil
}

// walLogPlayerLocked records the player's current identity in the WAL.
// Caller must hold stateMu. Pass the session token only when it was just
// created; existing tokens are already persisted.
func (s *Server) walLogPlayerLocked(playerID uint32, sessionToken string) {
	s.writeWALEntry("player", walPlayerData{
		PlayerID:     playerID,
		Name:         s.playerNames[playerID],
		FlagID:       s.playerFlags[playerID],
		SessionToken: sessionToken,
	})
}

func (s *Server) writeWALEntry(entryType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("WAL: failed to marshal data: %v", err)
		return
	}

	s.walMutex.Lock()
	s.walSeq++
	entry := WALEntry{
		Timestamp: time.Now(),
		Type:      entryType,
		Data:      jsonData,
		Sequence:  s.walSeq,
	}
	s.walBuffer = append(s.walBuffer, entry)
	shouldSignal := len(s.walBuffer) >= walBufferFlushThreshold
	s.walMutex.Unlock()

	if shouldSignal {
		// Coalesced non-blocking signal — if a flush is already queued we
		// leave it be. The flusher drains whatever is in the buffer when it
		// runs, so a single wake-up covers any number of overshoots.
		select {
		case s.walFlushSignal <- struct{}{}:
		default:
		}
	}
}

func (s *Server) flushWAL() error {
	s.walMutex.Lock()
	if len(s.walBuffer) == 0 {
		s.walMutex.Unlock()
		return nil
	}

	// Hand ownership of the backing array to the flusher — no copy needed, so
	// we don't momentarily hold 2× the WAL in memory during the flush. The
	// buffer is reallocated fresh with modest capacity; a heavy burst will
	// grow it back naturally via append.
	entries := s.walBuffer
	s.walBuffer = make([]WALEntry, 0, 1024)
	s.walMutex.Unlock()

	if s.useS3 {
		// One S3 object per flush under walPrefix; truncate deletes the whole
		// set after a snapshot. Avoids pulling the cumulative WAL into memory
		// on every flush.
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		for _, entry := range entries {
			if err := encoder.Encode(entry); err != nil {
				return fmt.Errorf("failed to encode WAL entry: %v", err)
			}
		}

		// Segment key: zero-padded sequence range so lexicographic sort replays
		// in insertion order. End sequence is always the last entry's Sequence.
		endSeq := entries[len(entries)-1].Sequence
		startSeq := entries[0].Sequence
		key := fmt.Sprintf("%s%020d-%020d.jsonl", walPrefix, startSeq, endSeq)

		_, err := s.s3Client.PutObject(&s3.PutObjectInput{
			Bucket: aws.String(s.bucketName),
			Key:    aws.String(key),
			Body:   bytes.NewReader(buf.Bytes()),
		})
		if err != nil {
			return fmt.Errorf("failed to upload WAL segment to S3: %v", err)
		}

		log.Printf("WAL: flushed %d entries to S3 segment %s", len(entries), key)
	} else {
		if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(s.dataDir, walFileName)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		for _, entry := range entries {
			if err := enc.Encode(entry); err != nil {
				return err
			}
		}
		// Without fsync, a machine kill right after "flush" can still truncate
		// the tail of the file mid-record.
		if err := f.Sync(); err != nil {
			return err
		}
		log.Printf("WAL: flushed %d entries to disk", len(entries))
	}
	return nil
}

// readWALFromS3 returns all WAL entries for replay. Segments are sorted
// lexicographically (which equals sequence order because keys are zero-padded),
// so replay order matches write order.
func (s *Server) readWALFromS3() ([]WALEntry, error) {
	var allEntries []WALEntry

	segments, err := s.listWALSegments()
	if err != nil {
		return nil, fmt.Errorf("failed to list WAL segments: %v", err)
	}
	for _, key := range segments {
		entries, err := s.readWALObjectFromS3(key)
		if err != nil {
			return nil, fmt.Errorf("failed to read WAL segment %s: %v", key, err)
		}
		allEntries = append(allEntries, entries...)
	}
	return allEntries, nil
}

func (s *Server) readWALObjectFromS3(key string) ([]WALEntry, error) {
	result, err := s.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	var entries []WALEntry
	decoder := json.NewDecoder(result.Body)
	for {
		var entry WALEntry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			// Best-effort: keep everything decoded so far rather than
			// discarding the whole segment over one corrupt record.
			log.Printf("[wal] segment %s corrupt after %d entries, replaying prefix: %v", key, len(entries), err)
			break
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// listWALSegments returns all WAL segment keys under walPrefix, sorted
// lexicographically.
func (s *Server) listWALSegments() ([]string, error) {
	var keys []string
	var continuationToken *string
	for {
		out, err := s.s3Client.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucketName),
			Prefix:            aws.String(walPrefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range out.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}
		continuationToken = out.NextContinuationToken
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *Server) readWALFromDisk() ([]WALEntry, error) {
	path := filepath.Join(s.dataDir, walFileName)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []WALEntry
	decoder := json.NewDecoder(f)
	for {
		var entry WALEntry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			// A crash mid-append leaves a truncated trailing record; replay
			// the intact prefix instead of dropping the whole log.
			log.Printf("[wal] log corrupt after %d entries, replaying prefix: %v", len(entries), err)
			break
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Server) truncateWAL() error {
	if s.useS3 {
		segments, err := s.listWALSegments()
		if err != nil {
			return fmt.Errorf("failed to list WAL segments for truncate: %v", err)
		}
		if len(segments) == 0 {
			return nil
		}
		// DeleteObjects in batches of up to 1000 (API limit).
		const batch = 1000
		for i := 0; i < len(segments); i += batch {
			end := i + batch
			if end > len(segments) {
				end = len(segments)
			}
			objs := make([]*s3.ObjectIdentifier, 0, end-i)
			for _, k := range segments[i:end] {
				kCopy := k
				objs = append(objs, &s3.ObjectIdentifier{Key: aws.String(kCopy)})
			}
			if _, err := s.s3Client.DeleteObjects(&s3.DeleteObjectsInput{
				Bucket: aws.String(s.bucketName),
				Delete: &s3.Delete{Objects: objs, Quiet: aws.Bool(true)},
			}); err != nil {
				return fmt.Errorf("failed to delete WAL segments: %v", err)
			}
		}
		return nil
	}
	path := filepath.Join(s.dataDir, walFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NotFound")
}

func (s *Server) periodicWALFlush() {
	ticker := time.NewTicker(walFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-s.walFlushSignal:
		}
		if err := s.flushWAL(); err != nil {
			log.Printf("[wal] flush error: %v", err)
		}
	}
}

func (s *Server) replayWAL() error {
	var (
		entries []WALEntry
		err     error
	)
	if s.useS3 {
		entries, err = s.readWALFromS3()
		if err != nil {
			if isS3NotFound(err) {
				return nil // No WAL to replay
			}
			return err
		}
	} else {
		entries, err = s.readWALFromDisk()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
	}

	log.Printf("[wal] replaying %d WAL entries", len(entries))

	for _, entry := range entries {
		switch entry.Type {
		case "reveal":
			var revealData struct {
				ChunkID ChunkID  `json:"chunk_id"`
				Cells   []uint32 `json:"cells"`
			}
			if err := json.Unmarshal(entry.Data, &revealData); err != nil {
				log.Printf("[wal] failed to unmarshal reveal: %v", err)
				continue
			}
			s.replayReveal(revealData.ChunkID, revealData.Cells)

		case "flag":
			var flagData struct {
				ChunkID ChunkID `json:"chunk_id"`
				Cell    uint32  `json:"cell"`
				FlagID  uint32  `json:"flag_id"`
				Owner   uint32  `json:"owner"`
			}
			if err := json.Unmarshal(entry.Data, &flagData); err != nil {
				log.Printf("[wal] failed to unmarshal flag: %v", err)
				continue
			}
			s.replayFlag(flagData.ChunkID, flagData.Cell, flagData.FlagID, flagData.Owner)

		case "score_update":
			var scoreData struct {
				PlayerID uint32 `json:"player_id"`
				Score    int32  `json:"score"`
				Streak   uint32 `json:"streak"`
			}
			if err := json.Unmarshal(entry.Data, &scoreData); err != nil {
				log.Printf("[wal] failed to unmarshal score update: %v", err)
				continue
			}
			s.replayScoreUpdate(scoreData.PlayerID, scoreData.Score, scoreData.Streak)

		case "player":
			var playerData walPlayerData
			if err := json.Unmarshal(entry.Data, &playerData); err != nil {
				log.Printf("[wal] failed to unmarshal player: %v", err)
				continue
			}
			s.replayPlayer(playerData)

		case "stats_update":
			var statsData struct {
				PlayerID   uint32      `json:"player_id"`
				Stats      PlayerStats `json:"stats"`
				NewUnlocks []string    `json:"new_unlocks,omitempty"`
			}
			if err := json.Unmarshal(entry.Data, &statsData); err != nil {
				log.Printf("[wal] failed to unmarshal stats update: %v", err)
				continue
			}
			s.replayStatsUpdate(statsData.PlayerID, statsData.Stats, statsData.NewUnlocks)
		}

		// Update WAL sequence number
		if entry.Sequence > s.walSeq {
			s.walSeq = entry.Sequence
		}
	}

	return nil
}

func (s *Server) replayReveal(chunkID ChunkID, cells []uint32) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.chunks[chunkID] == nil {
		s.chunks[chunkID] = &ChunkBits{}
	}

	for _, cell := range cells {
		bitIndex := cell
		mask := uint64(1) << (bitIndex % 64)
		if (s.chunks[chunkID][bitIndex/64] & mask) == 0 {
			s.chunks[chunkID][bitIndex/64] |= mask
			s.totalRevealed++
		}
	}
}

func (s *Server) replayFlag(chunkID ChunkID, cell uint32, flagID uint32, owner uint32) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.flags[chunkID] == nil {
		s.flags[chunkID] = make(map[uint32]Flag)
	}
	s.flags[chunkID][cell] = Flag{FlagID: flagID, Owner: owner}
}

func (s *Server) replayScoreUpdate(playerID uint32, score int32, streak uint32) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.scores[playerID] = score
	s.streaks[playerID] = streak
}

func (s *Server) replayStatsUpdate(playerID uint32, stats PlayerStats, newUnlocks []string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	copied := stats
	s.playerStats[playerID] = &copied

	if len(newUnlocks) == 0 {
		return
	}
	unlocked := s.unlockedAdvancements[playerID]
	if unlocked == nil {
		unlocked = make(map[string]bool)
		s.unlockedAdvancements[playerID] = unlocked
	}
	for _, id := range newUnlocks {
		unlocked[id] = true
	}
	s.rebuildUnlockedFlagsLocked(playerID)
}

func (s *Server) replayPlayer(d walPlayerData) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if prev, ok := s.playerNames[d.PlayerID]; ok && prev != "" && prev != d.Name {
		delete(s.nameToPlayerID, prev)
	}
	s.playerNames[d.PlayerID] = d.Name
	if d.Name != "" {
		s.nameToPlayerID[d.Name] = d.PlayerID
	}
	s.playerFlags[d.PlayerID] = d.FlagID
	if d.SessionToken != "" {
		s.sessionTokens[d.SessionToken] = d.PlayerID
	}
	if d.PlayerID >= s.nextPlayerID {
		s.nextPlayerID = d.PlayerID + 1
	}
}

func (s *Server) initPersistence() {
	s.bucketName = os.Getenv("S3_BUCKET_NAME")
	if s.bucketName != "" {
		s.useS3 = true
		if err := s.initAWS(); err != nil {
			log.Fatalf("Failed to initialize AWS: %v", err)
		}

		if err := s.loadSnapshotFromS3(); err != nil {
			log.Printf("[snapshot] no previous snapshot loaded from S3: %v (starting fresh)", err)
			// Even without a snapshot, attempt to recover state from WAL
			if err := s.replayWAL(); err != nil {
				log.Printf("[wal] failed to replay WAL: %v", err)
			} else {
				log.Printf("[wal] successfully replayed WAL")
				// After successful WAL replay, persist a snapshot so one always exists
				if err := s.saveSnapshotToS3(); err != nil {
					log.Printf("[snapshot] failed to save initial snapshot to S3: %v", err)
				} else {
					log.Printf("[snapshot] wrote initial snapshot to S3 after WAL replay")
				}
				s.logStartupSummary("boot (S3, WAL only)")
			}
		} else {
			log.Printf("[snapshot] loaded snapshot from S3")

			if err := s.replayWAL(); err != nil {
				log.Printf("[wal] failed to replay WAL: %v", err)
			} else {
				log.Printf("[wal] successfully replayed WAL")
				s.logStartupSummary("boot (S3, snapshot + WAL)")
			}
		}
	} else {
		if dir := os.Getenv("DATA_DIR"); dir != "" {
			s.dataDir = dir
		}
		if err := s.loadSnapshotFromDisk(); err != nil {
			log.Printf("[snapshot] no previous snapshot loaded from disk: %v (starting fresh)", err)
			// Even without a snapshot, attempt to recover state from WAL
			if err := s.replayWAL(); err != nil {
				log.Printf("[wal] failed to replay WAL: %v", err)
			} else {
				log.Printf("[wal] successfully replayed WAL")
				// After successful WAL replay, persist a snapshot so one always exists
				if err := s.saveSnapshotToDisk(); err != nil {
					log.Printf("[snapshot] failed to save initial snapshot to disk: %v", err)
				} else {
					log.Printf("[snapshot] wrote initial snapshot to disk after WAL replay")
				}
				s.logStartupSummary("boot (disk, WAL only)")
			}
		} else {
			log.Printf("[snapshot] loaded snapshot from disk")

			if err := s.replayWAL(); err != nil {
				log.Printf("[wal] failed to replay WAL: %v", err)
			} else {
				log.Printf("[wal] successfully replayed WAL")
				s.logStartupSummary("boot (disk, snapshot + WAL)")
			}
		}
	}

	go s.periodicSnapshotLoop() // fire‑and‑forget
	go s.periodicWALFlush()     // flush WAL every 30 seconds
}

// captureSnapshotData deep-copies all state under the lock. The gob encoding
// happens after the lock is released, so handing out references to the live
// maps would let concurrent reveals mutate them mid-encode — a fatal
// "concurrent map iteration and map write" panic.
func (s *Server) captureSnapshotData() snapshotData {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	chunks := make(map[ChunkID]*ChunkBits, len(s.chunks))
	for cid, cb := range s.chunks {
		if cb == nil {
			continue
		}
		copied := *cb
		chunks[cid] = &copied
	}
	flags := make(map[ChunkID]map[uint32]Flag, len(s.flags))
	for cid, fm := range s.flags {
		inner := make(map[uint32]Flag, len(fm))
		maps.Copy(inner, fm)
		flags[cid] = inner
	}
	scores := make(map[uint32]int32, len(s.scores))
	maps.Copy(scores, s.scores)
	streaks := make(map[uint32]uint32, len(s.streaks))
	maps.Copy(streaks, s.streaks)
	playerNames := make(map[uint32]string, len(s.playerNames))
	maps.Copy(playerNames, s.playerNames)
	playerFlags := make(map[uint32]uint32, len(s.playerFlags))
	maps.Copy(playerFlags, s.playerFlags)
	playerViews := make(map[uint32]PlayerView, len(s.playerViews))
	maps.Copy(playerViews, s.playerViews)
	sessionTokens := make(map[string]uint32, len(s.sessionTokens))
	maps.Copy(sessionTokens, s.sessionTokens)

	playerStats := make(map[uint32]*PlayerStats, len(s.playerStats))
	for pid, ps := range s.playerStats {
		if ps == nil {
			continue
		}
		copied := *ps
		playerStats[pid] = &copied
	}
	unlockedAdvancements := make(map[uint32]map[string]bool, len(s.unlockedAdvancements))
	for pid, set := range s.unlockedAdvancements {
		inner := make(map[string]bool, len(set))
		maps.Copy(inner, set)
		unlockedAdvancements[pid] = inner
	}

	return snapshotData{
		Chunks:               chunks,
		Flags:                flags,
		Scores:               scores,
		Streaks:              streaks,
		PlayerNames:          playerNames,
		PlayerFlags:          playerFlags,
		PlayerViews:          playerViews,
		NextPlayerID:         s.nextPlayerID,
		SessionTokens:        sessionTokens,
		PlayerStats:          playerStats,
		UnlockedAdvancements: unlockedAdvancements,
	}
}

func (s *Server) saveSnapshotToS3() error {
	data := s.captureSnapshotData()

	// Create compressed snapshot in memory. bytes.Buffer + bytes.NewReader is
	// important here: strings.Builder + strings.NewReader(buf.String()) copied
	// the whole buffer just to satisfy the reader contract, doubling peak
	// memory during a snapshot.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(&data); err != nil {
		return fmt.Errorf("failed to encode snapshot: %v", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %v", err)
	}

	_, err := s.s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(snapshotKey),
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("failed to upload snapshot to S3: %v", err)
	}

	// Truncate WAL after successful snapshot
	if err := s.truncateWAL(); err != nil {
		log.Printf("[wal] failed to truncate WAL after snapshot: %v", err)
		// Don't fail the snapshot operation for this
	}

	return nil
}

func (s *Server) saveSnapshotToDisk() error {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return err
	}

	data := s.captureSnapshotData()

	tmpPath := filepath.Join(s.dataDir, snapshotTmpName)
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(&data); err != nil {
		f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	finalPath := filepath.Join(s.dataDir, snapshotFileName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}

	if err := s.truncateWAL(); err != nil {
		log.Printf("[wal] failed to truncate WAL after snapshot: %v", err)
	}
	return nil
}

func (s *Server) restoreSnapshotData(data snapshotData) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.chunks = data.Chunks
	if data.Flags != nil {
		s.flags = data.Flags
	} else {
		s.flags = make(map[ChunkID]map[uint32]Flag)
	}
	s.scores = data.Scores
	if data.Streaks != nil {
		s.streaks = data.Streaks
	} else {
		s.streaks = make(map[uint32]uint32)
	}
	if data.PlayerNames != nil {
		s.playerNames = data.PlayerNames
	} else {
		s.playerNames = make(map[uint32]string)
	}
	if data.PlayerFlags != nil {
		s.playerFlags = data.PlayerFlags
	} else {
		s.playerFlags = make(map[uint32]uint32)
	}
	if data.PlayerViews != nil {
		s.playerViews = data.PlayerViews
		// Clean up orphaned playerViews entries
		for pid := range s.playerViews {
			if _, hasName := s.playerNames[pid]; !hasName {
				delete(s.playerViews, pid)
			}
		}
	} else {
		s.playerViews = make(map[uint32]PlayerView)
	}
	if data.NextPlayerID != 0 {
		s.nextPlayerID = data.NextPlayerID
	}
	if data.SessionTokens != nil {
		s.sessionTokens = data.SessionTokens
	} else if s.sessionTokens == nil {
		s.sessionTokens = make(map[string]uint32)
	}
	if data.PlayerStats != nil {
		s.playerStats = data.PlayerStats
	} else {
		s.playerStats = make(map[uint32]*PlayerStats)
	}
	if data.UnlockedAdvancements != nil {
		s.unlockedAdvancements = data.UnlockedAdvancements
	} else {
		s.unlockedAdvancements = make(map[uint32]map[string]bool)
	}
	// unlockedFlags is derived, not persisted: rebuild it from the unlocked
	// achievement IDs + their reward shapes so old snapshots (pre-advancements)
	// restore cleanly with empty unlocks.
	s.unlockedFlags = make(map[uint32]map[uint32]bool, len(s.unlockedAdvancements))
	for pid := range s.unlockedAdvancements {
		s.rebuildUnlockedFlagsLocked(pid)
	}
	// Rebuild fast name lookup index
	idx := make(map[string]uint32, len(s.playerNames))
	for pid, name := range s.playerNames {
		if name != "" {
			idx[name] = pid
		}
	}
	s.nameToPlayerID = idx

	// Recompute totalRevealed from persisted bitsets. Without this, after a
	// restart hasRevealedWithinTwo trivially returns true and the proximity
	// rule is silently disabled until at least one new reveal occurs.
	var totalRevealed uint64
	for _, cb := range s.chunks {
		if cb == nil {
			continue
		}
		for _, word := range *cb {
			totalRevealed += uint64(bits.OnesCount64(word))
		}
	}
	s.totalRevealed = totalRevealed

	s.lbDirty = true
}

func (s *Server) loadSnapshotFromS3() error {
	result, err := s.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(snapshotKey),
	})
	if err != nil {
		return fmt.Errorf("failed to download snapshot from S3: %v", err)
	}
	defer result.Body.Close()

	gz, err := gzip.NewReader(result.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	dec := gob.NewDecoder(gz)
	var data snapshotData
	if err := dec.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode snapshot: %v", err)
	}

	s.restoreSnapshotData(data)
	return nil
}

func (s *Server) loadSnapshotFromDisk() error {
	path := filepath.Join(s.dataDir, snapshotFileName)
	fmt.Println("Loading snapshot from disk:", path)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	dec := gob.NewDecoder(gz)
	var data snapshotData
	if err := dec.Decode(&data); err != nil {
		return err
	}

	s.restoreSnapshotData(data)
	log.Printf("[snapshot] loaded from disk: %d chunks", len(s.chunks))
	return nil
}

// persistOnShutdown makes a best-effort attempt to durably persist all state
// before the process exits. WAL flush first — it is fast and alone guarantees
// no data loss; the snapshot just makes the next boot faster.
func (s *Server) persistOnShutdown() {
	if err := s.flushWAL(); err != nil {
		log.Printf("[shutdown] WAL flush failed: %v", err)
	} else {
		log.Printf("[shutdown] WAL flushed")
	}
	var err error
	if s.useS3 {
		err = s.saveSnapshotToS3()
	} else {
		err = s.saveSnapshotToDisk()
	}
	if err != nil {
		log.Printf("[shutdown] snapshot failed: %v", err)
	} else {
		log.Printf("[shutdown] snapshot saved")
	}
}

func (s *Server) periodicSnapshotLoop() {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for range ticker.C {
		var err error
		if s.useS3 {
			err = s.saveSnapshotToS3()
		} else {
			err = s.saveSnapshotToDisk()
		}
		if err != nil {
			log.Printf("[snapshot] save error: %v", err)
		} else if s.useS3 {
			log.Printf("[snapshot] saved to S3")
		} else {
			log.Printf("[snapshot] saved to disk")
		}
	}
}

// logStartupSummary prints a one-line summary of recovered state after boot.
func (s *Server) logStartupSummary(context string) {
	s.stateMu.RLock()
	// Count chunks
	chunkCount := len(s.chunks)
	// Count revealed cells by popcount across all chunk bitmaps
	var revealed uint64
	for _, cb := range s.chunks {
		for _, word := range *cb {
			revealed += uint64(bits.OnesCount64(word))
		}
	}
	// Count flags across all chunks
	var flagsTotal int
	for _, fm := range s.flags {
		flagsTotal += len(fm)
	}
	players := len(s.scores)
	walSeq := s.walSeq
	s.stateMu.RUnlock()

	log.Printf("[startup] %s: chunks=%d revealed=%d flags=%d players=%d walSeq=%d", context, chunkCount, revealed, flagsTotal, players, walSeq)
}
