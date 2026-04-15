package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
)

// WAL entry types
type WALEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "reveal", "flag", or "score_update"
	Data      []byte    `json:"data"` // JSON encoded data
	Sequence  uint64    `json:"sequence"`
}

// snapshotData is what actually gets serialized. Gob can handle maps with
// struct keys, so we keep the exact types.
type snapshotData struct {
	Chunks       map[ChunkID]*ChunkBits
	Flags        map[ChunkID]map[uint32]Flag
	Scores       map[uint32]int32
	PlayerNames  map[uint32]string
	PlayerFlags  map[uint32]uint32
	PlayerViews  map[uint32]PlayerView
	NextPlayerID uint32
	// Persist session tokens so identities survive restarts
	SessionTokens map[string]uint32
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
	s.walMutex.Unlock()
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
		// Encode ONLY the new batch into a unique segment object. We avoid the
		// legacy read-append-rewrite flow because it pulled the entire WAL into
		// memory on every flush — which is the primary WAL-related OOM
		// contributor under load.
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
			return nil, fmt.Errorf("failed to decode WAL entry: %v", err)
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
			return nil, err
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

	for range ticker.C {
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
			}
			if err := json.Unmarshal(entry.Data, &flagData); err != nil {
				log.Printf("[wal] failed to unmarshal flag: %v", err)
				continue
			}
			s.replayFlag(flagData.ChunkID, flagData.Cell, flagData.FlagID)

		case "score_update":
			var scoreData struct {
				PlayerID uint32 `json:"player_id"`
				Score    int32  `json:"score"`
			}
			if err := json.Unmarshal(entry.Data, &scoreData); err != nil {
				log.Printf("[wal] failed to unmarshal score update: %v", err)
				continue
			}
			s.replayScoreUpdate(scoreData.PlayerID, scoreData.Score)
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

func (s *Server) replayFlag(chunkID ChunkID, cell uint32, flagID uint32) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.flags[chunkID] == nil {
		s.flags[chunkID] = make(map[uint32]Flag)
	}
	s.flags[chunkID][cell] = Flag{FlagID: flagID}
}

func (s *Server) replayScoreUpdate(playerID uint32, score int32) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.scores[playerID] = score
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

func (s *Server) captureSnapshotData() snapshotData {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return snapshotData{
		Chunks:        s.chunks,
		Flags:         s.flags,
		Scores:        s.scores,
		PlayerNames:   s.playerNames,
		PlayerFlags:   s.playerFlags,
		PlayerViews:   s.playerViews,
		NextPlayerID:  s.nextPlayerID,
		SessionTokens: s.sessionTokens,
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
	// Rebuild fast name lookup index
	idx := make(map[string]uint32, len(s.playerNames))
	for pid, name := range s.playerNames {
		if name != "" {
			idx[name] = pid
		}
	}
	s.nameToPlayerID = idx
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
